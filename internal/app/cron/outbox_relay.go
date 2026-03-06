package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
)

const (
	// outboxPollInterval is how often the relay checks for unpublished events.
	outboxPollInterval = 2 * time.Second
	// outboxBatchSize is the max number of events fetched per poll cycle.
	outboxBatchSize = 50
	// outboxMaxAttempts is the retry cap; events exceeding this are skipped (dead letter).
	outboxMaxAttempts = 10
	// outboxCleanupAge is how long published events are retained before deletion.
	outboxCleanupAge = 7 * 24 * time.Hour
	// outboxCleanupInterval is how often the cleanup sweep runs.
	outboxCleanupInterval = 6 * time.Hour
	// outboxPublishTimeout is the per-event NATS publish deadline.
	outboxPublishTimeout = 5 * time.Second
)

// outboxStore is the minimal interface the relay needs from the outbox repository.
type outboxStore interface {
	ListUnpublished(ctx context.Context, limit int) ([]entity.OutboxEvent, error)
	MarkPublished(ctx context.Context, id int64) error
	IncrementAttempts(ctx context.Context, id int64, lastErr string) error
	DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// OutboxRelay polls the outbox table and publishes events to NATS.
type OutboxRelay struct {
	store     outboxStore
	publisher EventPublisher
}

// NewOutboxRelay creates a new OutboxRelay.
func NewOutboxRelay(store outboxStore, publisher EventPublisher) *OutboxRelay {
	return &OutboxRelay{store: store, publisher: publisher}
}

// Run starts the relay loop. It blocks until ctx is cancelled.
func (r *OutboxRelay) Run(ctx context.Context) error {
	slog.Info("outbox/relay: starting",
		"poll_interval", outboxPollInterval,
		"batch_size", outboxBatchSize,
	)

	pollTicker := time.NewTicker(outboxPollInterval)
	defer pollTicker.Stop()

	cleanupTicker := time.NewTicker(outboxCleanupInterval)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox/relay: stopped")
			return nil
		case <-pollTicker.C:
			r.pollAndPublish(ctx)
		case <-cleanupTicker.C:
			r.cleanup(ctx)
		}
	}
}

// pollAndPublish fetches unpublished events and publishes them to NATS one by one.
func (r *OutboxRelay) pollAndPublish(ctx context.Context) {
	rows, err := r.store.ListUnpublished(ctx, outboxBatchSize)
	if err != nil {
		slog.Error("outbox/relay: list unpublished", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}

	published := 0
	for _, row := range rows {
		if row.Attempts >= outboxMaxAttempts {
			slog.Warn("outbox/relay: event exceeded max attempts, skipping (dead letter)",
				"outbox_id", row.ID,
				"event_id", row.EventID,
				"attempts", row.Attempts,
				"last_error", row.LastError,
			)
			continue
		}

		if err := r.publishOne(ctx, row); err != nil {
			slog.Error("outbox/relay: publish failed",
				"outbox_id", row.ID,
				"event_id", row.EventID,
				"err", err,
			)
			_ = r.store.IncrementAttempts(ctx, row.ID, err.Error())
			continue
		}

		if err := r.store.MarkPublished(ctx, row.ID); err != nil {
			slog.Error("outbox/relay: mark published failed",
				"outbox_id", row.ID,
				"err", err,
			)
			continue
		}
		published++
	}

	if published > 0 {
		slog.Info("outbox/relay: batch published", "count", published)
	}
}

// publishOne deserializes the outbox payload back into an IdentityEvent and publishes it.
func (r *OutboxRelay) publishOne(ctx context.Context, row entity.OutboxEvent) error {
	var ev event.IdentityEvent
	if err := json.Unmarshal(row.Payload, &ev); err != nil {
		return fmt.Errorf("unmarshal outbox payload: %w", err)
	}

	pubCtx, cancel := context.WithTimeout(ctx, outboxPublishTimeout)
	defer cancel()

	return r.publisher.Publish(pubCtx, &ev)
}

// cleanup removes published events older than the retention period.
func (r *OutboxRelay) cleanup(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-outboxCleanupAge)
	deleted, err := r.store.DeletePublishedBefore(ctx, cutoff)
	if err != nil {
		slog.Error("outbox/relay: cleanup failed", "err", err)
		return
	}
	if deleted > 0 {
		slog.Info("outbox/relay: cleanup complete", "deleted", deleted)
	}
}
