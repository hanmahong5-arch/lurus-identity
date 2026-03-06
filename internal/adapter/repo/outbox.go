package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
	"gorm.io/gorm"
)

// OutboxRepo manages the transactional outbox table.
type OutboxRepo struct {
	db *gorm.DB
}

// NewOutboxRepo creates a new OutboxRepo.
func NewOutboxRepo(db *gorm.DB) *OutboxRepo { return &OutboxRepo{db: db} }

// Insert serializes an IdentityEvent and writes it to the outbox table.
// This should be called within the same DB transaction as the business state change
// to guarantee atomicity.
func (r *OutboxRepo) Insert(ctx context.Context, ev *event.IdentityEvent) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	row := entity.OutboxEvent{
		EventID: ev.EventID,
		Subject: ev.EventType,
		Payload: payload,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

// ListUnpublished returns up to limit outbox events that have not been published,
// ordered by ID ascending (FIFO).
func (r *OutboxRepo) ListUnpublished(ctx context.Context, limit int) ([]entity.OutboxEvent, error) {
	var rows []entity.OutboxEvent
	err := r.db.WithContext(ctx).
		Where("published_at IS NULL").
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// MarkPublished sets the published_at timestamp for the given outbox event ID.
func (r *OutboxRepo) MarkPublished(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&entity.OutboxEvent{}).
		Where("id = ?", id).
		Update("published_at", now).Error
}

// IncrementAttempts increments the attempts counter and records the last error message.
func (r *OutboxRepo) IncrementAttempts(ctx context.Context, id int64, lastErr string) error {
	return r.db.WithContext(ctx).
		Model(&entity.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"attempts":   gorm.Expr("attempts + 1"),
			"last_error": lastErr,
		}).Error
}

// DeletePublishedBefore removes outbox events that were published before the cutoff time.
// Returns the number of deleted rows.
func (r *OutboxRepo) DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("published_at IS NOT NULL AND published_at < ?", cutoff).
		Delete(&entity.OutboxEvent{})
	return result.RowsAffected, result.Error
}
