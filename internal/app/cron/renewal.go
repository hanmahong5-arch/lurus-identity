// Package cron contains scheduled background tasks.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/app"
	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
	"github.com/redis/go-redis/v9"
)

const (
	// renewalLockKey is the Redis key used to ensure only one pod runs auto-renewal at a time.
	renewalLockKey = "cron:lock:subscription_renewal"
	// renewalLockTTL prevents a crashed pod from holding the lock forever.
	// Set slightly under the renewal interval to guarantee at most one concurrent run.
	renewalLockTTL = 55 * time.Minute

	// Exponential backoff intervals indexed by attempt number (0-based after failure).
	backoffAttempt1 = 1 * time.Hour
	backoffAttempt2 = 4 * time.Hour
	backoffAttempt3 = 12 * time.Hour

	// renewalTxType is the wallet transaction type used for subscription auto-renewal debits.
	renewalTxType = "subscription_renewal"
	// renewalRefundTxType is the wallet transaction type for compensating a failed activation after debit.
	renewalRefundTxType = "subscription_renewal_refund"
)

// renewalStore is the subset of subscriptionStore needed by RenewalJob.
// Declared locally to keep RenewalJob independent of app.SubscriptionService internals.
type renewalStore interface {
	ListDueForRenewal(ctx context.Context) ([]entity.Subscription, error)
	UpdateRenewalState(ctx context.Context, subID int64, attempts int, nextAt *time.Time) error
}

// renewalPlanStore is the subset of planStore needed to look up plan price.
type renewalPlanStore interface {
	GetPlanByID(ctx context.Context, id int64) (*entity.ProductPlan, error)
}

// RenewalJob scans subscriptions due for auto-renewal and processes them.
type RenewalJob struct {
	subs      *app.SubscriptionService
	store     renewalStore
	plans     renewalPlanStore
	wallets   *app.WalletService
	publisher EventPublisher
	rdb       *redis.Client
	interval  time.Duration
	outbox    outboxWriter
}

// NewRenewalJob creates a new RenewalJob.
func NewRenewalJob(
	subs *app.SubscriptionService,
	store renewalStore,
	plans renewalPlanStore,
	wallets *app.WalletService,
	publisher EventPublisher,
	rdb *redis.Client,
	interval time.Duration,
	outbox outboxWriter,
) *RenewalJob {
	return &RenewalJob{
		subs:      subs,
		store:     store,
		plans:     plans,
		wallets:   wallets,
		publisher: publisher,
		rdb:       rdb,
		interval:  interval,
		outbox:    outbox,
	}
}

// Run starts the recurring renewal job. It blocks until ctx is cancelled.
func (j *RenewalJob) Run(ctx context.Context) error {
	slog.Info("cron/renewal: starting", "interval", j.interval)
	// Run once immediately, then on interval.
	j.tick(ctx)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("cron/renewal: stopped")
			return nil
		case <-ticker.C:
			j.tick(ctx)
		}
	}
}

// tick performs a single renewal scan, protected by a distributed Redis lock.
func (j *RenewalJob) tick(ctx context.Context) {
	acquired, err := j.acquireLock(ctx)
	if err != nil {
		slog.Error("cron/renewal: lock error", "err", err)
		return
	}
	if !acquired {
		slog.Debug("cron/renewal: another pod holds the lock, skipping")
		return
	}
	defer j.releaseLock(ctx)

	start := time.Now()
	scanned, renewed, failed := j.runRenewal(ctx)
	slog.Info("cron/renewal: tick complete",
		"scanned", scanned,
		"renewed", renewed,
		"failed", failed,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// runRenewal iterates all subscriptions due for renewal, debits the wallet,
// and either activates the renewal or records a failed attempt with backoff.
func (j *RenewalJob) runRenewal(ctx context.Context) (scanned, renewed, failed int) {
	subs, err := j.store.ListDueForRenewal(ctx)
	if err != nil {
		slog.Error("cron/renewal: list due for renewal", "err", err)
		return
	}
	scanned = len(subs)

	for _, sub := range subs {
		if err := j.processOne(ctx, sub); err != nil {
			slog.Error("cron/renewal: process subscription",
				"sub_id", sub.ID,
				"account_id", sub.AccountID,
				"err", err,
			)
			failed++
		} else {
			renewed++
		}
	}
	return
}

// processOne attempts to renew a single subscription.
// On wallet debit success it calls SubscriptionService.Activate and resets the counter.
// On failure it increments the attempt counter and schedules an exponential backoff retry.
func (j *RenewalJob) processOne(ctx context.Context, sub entity.Subscription) error {
	plan, err := j.plans.GetPlanByID(ctx, sub.PlanID)
	if err != nil {
		return fmt.Errorf("get plan %d: %w", sub.PlanID, err)
	}
	if plan == nil {
		return fmt.Errorf("plan %d not found", sub.PlanID)
	}

	orderRef := fmt.Sprintf("renewal:sub:%d", sub.ID)
	_, debitErr := j.wallets.Debit(ctx,
		sub.AccountID,
		plan.PriceCNY,
		renewalTxType,
		fmt.Sprintf("Auto-renewal for plan %s", plan.Code),
		"subscription",
		orderRef,
		sub.ProductID,
	)

	if debitErr == nil {
		// Debit succeeded: activate a new subscription cycle.
		_, activateErr := j.subs.Activate(ctx,
			sub.AccountID,
			sub.ProductID,
			sub.PlanID,
			sub.PaymentMethod,
			sub.ExternalSubID,
		)
		if activateErr != nil {
			// Activation failed after debit — compensate by crediting back the funds.
			refundRef := fmt.Sprintf("refund:renewal:sub:%d", sub.ID)
			_, creditErr := j.wallets.Credit(ctx,
				sub.AccountID,
				plan.PriceCNY,
				renewalRefundTxType,
				fmt.Sprintf("Renewal refund: activation failed for plan %s", plan.Code),
				"subscription",
				refundRef,
				sub.ProductID,
			)
			if creditErr != nil {
				// CRITICAL: money deducted but refund failed — requires manual intervention.
				slog.Error("CRITICAL: cron/renewal: compensation credit failed after activation failure",
					"sub_id", sub.ID,
					"account_id", sub.AccountID,
					"plan_id", sub.PlanID,
					"amount", plan.PriceCNY,
					"debit_ref", orderRef,
					"activate_err", activateErr,
					"credit_err", creditErr,
				)
			} else {
				slog.Warn("cron/renewal: activation failed, funds refunded",
					"sub_id", sub.ID,
					"account_id", sub.AccountID,
					"amount", plan.PriceCNY,
				)
			}
			// Return error so the subscription enters backoff retry instead of being silently reset.
			return fmt.Errorf("activate after debit (funds refunded): %w", activateErr)
		}

		// Reset renewal state on the original subscription row.
		_ = j.store.UpdateRenewalState(ctx, sub.ID, 0, nil)

		j.publishRenewalEvent(ctx, event.SubjectSubscriptionActivated, sub.AccountID, sub.ProductID, map[string]any{
			"subscription_id": sub.ID,
			"plan_id":         sub.PlanID,
			"plan_code":       plan.Code,
			"event":           "renewal_success",
		})
		return nil
	}

	// Debit failed: increment attempts and schedule retry with exponential backoff.
	nextAttempts := sub.RenewalAttempts + 1
	nextAt := nextRenewalAt(nextAttempts)
	_ = j.store.UpdateRenewalState(ctx, sub.ID, nextAttempts, nextAt)

	j.publishRenewalEvent(ctx, event.SubjectSubscriptionExpired, sub.AccountID, sub.ProductID, map[string]any{
		"subscription_id":  sub.ID,
		"plan_id":          sub.PlanID,
		"renewal_attempts": nextAttempts,
		"next_renewal_at":  nextAt,
		"event":            "renewal_failed",
		"reason":           debitErr.Error(),
	})

	return fmt.Errorf("debit account %d: %w", sub.AccountID, debitErr)
}

// nextRenewalAt returns the retry timestamp for a given attempt count (1-based after failure).
// Backoff schedule: attempt 1 → +1h, attempt 2 → +4h, attempt 3+ → +12h.
func nextRenewalAt(attempts int) *time.Time {
	var d time.Duration
	switch attempts {
	case 1:
		d = backoffAttempt1
	case 2:
		d = backoffAttempt2
	default:
		d = backoffAttempt3
	}
	t := time.Now().Add(d)
	return &t
}

// publishRenewalEvent writes the event to the outbox for reliable delivery.
// Falls back to direct NATS publish if the outbox is unavailable.
func (j *RenewalJob) publishRenewalEvent(ctx context.Context, subject string, accountID int64, productID string, payload any) {
	ev, err := event.NewEvent(subject, accountID, "", productID, payload)
	if err != nil {
		slog.Error("cron/renewal: build event", "err", err)
		return
	}

	// Primary path: write to outbox (relay will publish to NATS).
	if j.outbox != nil {
		if err := j.outbox.Insert(ctx, ev); err != nil {
			slog.Error("cron/renewal: outbox insert failed, falling back to direct publish",
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
		slog.Error("cron/renewal: publish event", "subject", subject, "err", err)
	}
}

// acquireLock tries to set a Redis NX lock. Returns (true, nil) if acquired.
func (j *RenewalJob) acquireLock(ctx context.Context) (bool, error) {
	ok, err := j.rdb.SetNX(ctx, renewalLockKey, "1", renewalLockTTL).Result()
	if err != nil {
		return false, fmt.Errorf("redis SetNX: %w", err)
	}
	return ok, nil
}

// releaseLock deletes the lock key.
func (j *RenewalJob) releaseLock(ctx context.Context) {
	if err := j.rdb.Del(ctx, renewalLockKey).Err(); err != nil {
		slog.Warn("cron/renewal: release lock failed", "err", err)
	}
}
