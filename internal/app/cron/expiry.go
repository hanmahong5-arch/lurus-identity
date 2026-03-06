// Package cron contains scheduled background tasks.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/app"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
	"github.com/redis/go-redis/v9"
)

// outboxWriter is the subset of the outbox repository needed by event publishers.
type outboxWriter interface {
	Insert(ctx context.Context, ev *event.IdentityEvent) error
}

const (
	// publishEventTimeout is the deadline for a single best-effort event publish.
	// An independent context is used so a cancelled run-ctx does not abort in-flight
	// event delivery (the DB write has already succeeded at this point).
	publishEventTimeout = 5 * time.Second
)

const (
	// lockKey is the Redis key used to ensure only one pod runs expiry at a time.
	lockKey = "cron:lock:subscription_expiry"
	// lockTTL prevents a crashed pod from holding the lock forever.
	lockTTL = 10 * time.Minute
	// defaultInterval controls how often the expiry scan runs.
	defaultInterval = time.Hour
)

// EventPublisher is the subset of the NATS publisher needed by the cron job.
type EventPublisher interface {
	Publish(ctx context.Context, ev *event.IdentityEvent) error
}

// ExpiryJob scans for expired subscriptions and transitions them through the
// active → grace → expired lifecycle.
type ExpiryJob struct {
	subs      *app.SubscriptionService
	publisher EventPublisher
	rdb       *redis.Client
	interval  time.Duration
	outbox    outboxWriter
}

// NewExpiryJob creates a new ExpiryJob.
func NewExpiryJob(subs *app.SubscriptionService, publisher EventPublisher, rdb *redis.Client, outbox outboxWriter) *ExpiryJob {
	return &ExpiryJob{
		subs:      subs,
		publisher: publisher,
		rdb:       rdb,
		interval:  defaultInterval,
		outbox:    outbox,
	}
}

// Run starts the recurring expiry job. It blocks until ctx is cancelled.
func (j *ExpiryJob) Run(ctx context.Context) error {
	slog.Info("cron/expiry: starting", "interval", j.interval)
	// Run once immediately, then on interval.
	j.tick(ctx)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("cron/expiry: stopped")
			return nil
		case <-ticker.C:
			j.tick(ctx)
		}
	}
}

// tick performs a single expiry scan, protected by a distributed Redis lock.
func (j *ExpiryJob) tick(ctx context.Context) {
	acquired, err := j.acquireLock(ctx)
	if err != nil {
		slog.Error("cron/expiry: lock error", "err", err)
		return
	}
	if !acquired {
		slog.Debug("cron/expiry: another pod holds the lock, skipping")
		return
	}
	defer j.releaseLock(ctx)

	start := time.Now()
	scanned, processed, failed := j.runExpiry(ctx)
	slog.Info("cron/expiry: tick complete",
		"scanned", scanned,
		"processed", processed,
		"failed", failed,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// runExpiry performs the two-phase expiry scan and returns metrics.
func (j *ExpiryJob) runExpiry(ctx context.Context) (scanned, processed, failed int) {
	// Phase 1: active subscriptions past their expires_at → enter grace period.
	activeExpired, err := j.subs.ListActiveExpired(ctx)
	if err != nil {
		slog.Error("cron/expiry: list active-expired", "err", err)
		return
	}
	scanned += len(activeExpired)
	for _, sub := range activeExpired {
		if err := j.subs.Expire(ctx, sub.ID); err != nil {
			slog.Error("cron/expiry: expire subscription",
				"sub_id", sub.ID, "account_id", sub.AccountID, "err", err)
			failed++
			continue
		}
		j.publishEvent(ctx, event.SubjectSubscriptionExpired, sub.AccountID, "", sub.ProductID, map[string]any{
			"subscription_id": sub.ID,
			"phase":           "grace_entered",
		})
		processed++
	}

	// Phase 2: grace-period subscriptions past grace_until → permanent expiry.
	graceExpired, err := j.subs.ListGraceExpired(ctx)
	if err != nil {
		slog.Error("cron/expiry: list grace-expired", "err", err)
		return
	}
	scanned += len(graceExpired)
	for _, sub := range graceExpired {
		if err := j.subs.EndGrace(ctx, sub.ID); err != nil {
			slog.Error("cron/expiry: end grace",
				"sub_id", sub.ID, "account_id", sub.AccountID, "err", err)
			failed++
			continue
		}
		j.publishEvent(ctx, event.SubjectSubscriptionExpired, sub.AccountID, "", sub.ProductID, map[string]any{
			"subscription_id": sub.ID,
			"phase":           "expired_downgraded",
		})
		processed++
	}
	return
}

// publishEvent writes the event to the transactional outbox for reliable delivery.
// If the outbox is unavailable, it falls back to direct NATS publish (best-effort).
func (j *ExpiryJob) publishEvent(ctx context.Context, subject string, accountID int64, lurusID, productID string, payload any) {
	ev, err := event.NewEvent(subject, accountID, lurusID, productID, payload)
	if err != nil {
		slog.Error("cron/expiry: build event", "err", err)
		return
	}

	// Primary path: write to outbox (relay will publish to NATS).
	if j.outbox != nil {
		if err := j.outbox.Insert(ctx, ev); err != nil {
			slog.Error("cron/expiry: outbox insert failed, falling back to direct publish",
				"subject", subject, "err", err)
		} else {
			return
		}
	}

	// Fallback: direct NATS publish (best-effort).
	if j.publisher == nil {
		return
	}
	pubCtx, cancel := context.WithTimeout(context.Background(), publishEventTimeout)
	defer cancel()
	if err := j.publisher.Publish(pubCtx, ev); err != nil {
		slog.Error("cron/expiry: publish event", "subject", subject, "err", err)
	}
}

// acquireLock tries to set a Redis NX lock. Returns (true, nil) if acquired.
func (j *ExpiryJob) acquireLock(ctx context.Context) (bool, error) {
	ok, err := j.rdb.SetNX(ctx, lockKey, "1", lockTTL).Result()
	if err != nil {
		return false, fmt.Errorf("redis SetNX: %w", err)
	}
	return ok, nil
}

// releaseLock deletes the lock key.
func (j *ExpiryJob) releaseLock(ctx context.Context) {
	if err := j.rdb.Del(ctx, lockKey).Err(); err != nil {
		slog.Warn("cron/expiry: release lock failed", "err", err)
	}
}
