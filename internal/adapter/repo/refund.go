package repo

import (
	"context"
	"errors"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"gorm.io/gorm"
)

// RefundRepo implements refundStore backed by PostgreSQL via GORM.
type RefundRepo struct {
	db *gorm.DB
}

// NewRefundRepo creates a new RefundRepo.
func NewRefundRepo(db *gorm.DB) *RefundRepo { return &RefundRepo{db: db} }

// Create inserts a new refund record.
func (r *RefundRepo) Create(ctx context.Context, ref *entity.Refund) error {
	return r.db.WithContext(ctx).Create(ref).Error
}

// GetByRefundNo returns a refund by its unique refund number, or nil if not found.
func (r *RefundRepo) GetByRefundNo(ctx context.Context, refundNo string) (*entity.Refund, error) {
	var ref entity.Refund
	err := r.db.WithContext(ctx).Where("refund_no = ?", refundNo).First(&ref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

// GetPendingByOrderNo returns a refund in pending or approved status for a given order.
// Returns nil if no in-progress refund exists.
func (r *RefundRepo) GetPendingByOrderNo(ctx context.Context, orderNo string) (*entity.Refund, error) {
	var ref entity.Refund
	err := r.db.WithContext(ctx).
		Where("order_no = ? AND status IN ('pending','approved')", orderNo).
		First(&ref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

// UpdateStatus updates the status, review metadata, and reviewer info of a refund.
func (r *RefundRepo) UpdateStatus(ctx context.Context, refundNo, status, reviewNote, reviewedBy string, reviewedAt *time.Time) error {
	return r.db.WithContext(ctx).
		Model(&entity.Refund{}).
		Where("refund_no = ?", refundNo).
		Updates(map[string]any{
			"status":      status,
			"review_note": reviewNote,
			"reviewed_by": reviewedBy,
			"reviewed_at": reviewedAt,
		}).Error
}

// MarkCompleted sets the refund status to completed and records the completion timestamp.
func (r *RefundRepo) MarkCompleted(ctx context.Context, refundNo string, completedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&entity.Refund{}).
		Where("refund_no = ?", refundNo).
		Updates(map[string]any{
			"status":       string(entity.RefundStatusCompleted),
			"completed_at": completedAt,
		}).Error
}

// ListByAccount returns paginated refunds for a given account, newest first.
func (r *RefundRepo) ListByAccount(ctx context.Context, accountID int64, page, pageSize int) ([]entity.Refund, int64, error) {
	var list []entity.Refund
	var total int64
	q := r.db.WithContext(ctx).Model(&entity.Refund{}).Where("account_id = ?", accountID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := q.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&list).Error
	return list, total, err
}
