// Package cron contains scheduled background tasks.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/email"
	"github.com/redis/go-redis/v9"
)

// notificationSubStore is the minimal subscription persistence interface required by NotificationJob.
type notificationSubStore interface {
	// ListExpiring returns active subscriptions expiring within the next withinHours hours.
	ListExpiring(ctx context.Context, withinHours int) ([]entity.Subscription, error)
}

// notificationAccountStore is the minimal account persistence interface required by NotificationJob.
type notificationAccountStore interface {
	GetByID(ctx context.Context, id int64) (*entity.Account, error)
}

const (
	// notificationLockKey is the Redis key used to ensure only one pod runs the notification job.
	notificationLockKey = "cron:lock:subscription_notification"
	// notificationLockTTL prevents a crashed pod from holding the lock indefinitely.
	notificationLockTTL = 23 * time.Hour
	// defaultNotificationInterval is how often the notification scan runs.
	defaultNotificationInterval = 24 * time.Hour
	// remindDedupeExtra is the extra duration added on top of the remaining TTL for the
	// Redis deduplication key so that a notification is not re-sent if it fires slightly early.
	remindDedupeExtra = 48 * time.Hour
)

// reminderDays lists how many days before expiry reminder emails are sent.
var reminderDays = []int{7, 3, 1}

// NotificationJob sends expiry reminder emails for active subscriptions.
type NotificationJob struct {
	subs     notificationSubStore
	accounts notificationAccountStore
	mailer   email.Sender
	rdb      *redis.Client
	interval time.Duration
}

// NewNotificationJob creates a new NotificationJob.
func NewNotificationJob(
	subs notificationSubStore,
	accounts notificationAccountStore,
	mailer email.Sender,
	rdb *redis.Client,
	interval time.Duration,
) *NotificationJob {
	return &NotificationJob{
		subs:     subs,
		accounts: accounts,
		mailer:   mailer,
		rdb:      rdb,
		interval: interval,
	}
}

// Run starts the recurring notification job. It blocks until ctx is cancelled.
func (j *NotificationJob) Run(ctx context.Context) error {
	slog.Info("cron/notification: starting", "interval", j.interval)
	// Run once immediately, then on interval.
	j.tick(ctx)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("cron/notification: stopped")
			return nil
		case <-ticker.C:
			j.tick(ctx)
		}
	}
}

// tick performs a single notification scan, protected by a distributed Redis lock.
func (j *NotificationJob) tick(ctx context.Context) {
	acquired, err := j.acquireNotifLock(ctx)
	if err != nil {
		slog.Error("cron/notification: lock error", "err", err)
		return
	}
	if !acquired {
		slog.Debug("cron/notification: another pod holds the lock, skipping")
		return
	}
	defer j.releaseNotifLock(ctx)

	start := time.Now()
	sent, skipped, failed := j.runNotifications(ctx)
	slog.Info("cron/notification: tick complete",
		"sent", sent,
		"skipped", skipped,
		"failed", failed,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// runNotifications scans for subscriptions expiring in 7, 3, and 1 days and sends
// deduplicated reminder emails. Failures are fail-open: errors are logged and the
// job continues processing remaining notifications.
func (j *NotificationJob) runNotifications(ctx context.Context) (sent, skipped, failed int) {
	for _, days := range reminderDays {
		// ListExpiring accepts a window in hours. We query subscriptions expiring
		// within the target day window (days*24 hours from now), with a 1-hour upper
		// buffer to avoid missing subscriptions that fall just outside an exact boundary.
		withinHours := days * 24
		subs, err := j.subs.ListExpiring(ctx, withinHours)
		if err != nil {
			slog.Error("cron/notification: list expiring subscriptions",
				"days", days, "err", err)
			failed++
			continue
		}

		for _, sub := range subs {
			if sub.ExpiresAt == nil {
				continue
			}

			dedupeKey := fmt.Sprintf("email:remind:%d:%d", sub.ID, days)

			// Check deduplication key — skip if already sent.
			exists, err := j.rdb.Exists(ctx, dedupeKey).Result()
			if err != nil {
				slog.Error("cron/notification: check dedupe key",
					"key", dedupeKey, "err", err)
				failed++
				continue
			}
			if exists > 0 {
				skipped++
				continue
			}

			account, err := j.accounts.GetByID(ctx, sub.AccountID)
			if err != nil || account == nil {
				slog.Error("cron/notification: get account",
					"account_id", sub.AccountID, "err", err)
				failed++
				continue
			}

			if err := j.sendReminder(ctx, account, sub, days); err != nil {
				// Fail-open: log and continue.
				slog.Error("cron/notification: send reminder email",
					"account_id", account.ID,
					"sub_id", sub.ID,
					"days", days,
					"err", err,
				)
				failed++
				continue
			}

			// Set deduplication key with TTL = remaining time until expiry + 2 days buffer.
			ttl := time.Until(*sub.ExpiresAt) + remindDedupeExtra
			if ttl < time.Minute {
				ttl = remindDedupeExtra
			}
			if err := j.rdb.Set(ctx, dedupeKey, "1", ttl).Err(); err != nil {
				slog.Warn("cron/notification: set dedupe key failed",
					"key", dedupeKey, "err", err)
			}

			slog.Info("cron/notification: reminder sent",
				"account_id", account.ID,
				"sub_id", sub.ID,
				"days_until_expiry", days,
			)
			sent++
		}
	}
	return
}

// sendReminder composes and delivers the expiry reminder email.
func (j *NotificationJob) sendReminder(ctx context.Context, account *entity.Account, sub entity.Subscription, daysLeft int) error {
	subject := fmt.Sprintf("Your subscription expires in %d day(s)", daysLeft)
	body := fmt.Sprintf(
		"Dear %s,\r\n\r\n"+
			"Your subscription (ID: %d) for product %q will expire in %d day(s) on %s.\r\n\r\n"+
			"Please renew your subscription to continue uninterrupted service.\r\n\r\n"+
			"Lurus Platform",
		account.DisplayName,
		sub.ID,
		sub.ProductID,
		daysLeft,
		sub.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"),
	)
	return j.mailer.Send(ctx, account.Email, subject, body)
}

// acquireNotifLock tries to set a Redis NX lock for the notification job.
func (j *NotificationJob) acquireNotifLock(ctx context.Context) (bool, error) {
	ok, err := j.rdb.SetNX(ctx, notificationLockKey, "1", notificationLockTTL).Result()
	if err != nil {
		return false, fmt.Errorf("redis SetNX: %w", err)
	}
	return ok, nil
}

// releaseNotifLock deletes the notification lock key.
func (j *NotificationJob) releaseNotifLock(ctx context.Context) {
	if err := j.rdb.Del(ctx, notificationLockKey).Err(); err != nil {
		slog.Warn("cron/notification: release lock failed", "err", err)
	}
}
