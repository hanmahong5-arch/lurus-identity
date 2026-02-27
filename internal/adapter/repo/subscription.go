package repo

import (
	"context"
	"errors"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"gorm.io/gorm"
)

// SubscriptionRepo manages subscription and entitlement persistence.
type SubscriptionRepo struct {
	db *gorm.DB
}

func NewSubscriptionRepo(db *gorm.DB) *SubscriptionRepo { return &SubscriptionRepo{db: db} }

func (r *SubscriptionRepo) Create(ctx context.Context, s *entity.Subscription) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *SubscriptionRepo) Update(ctx context.Context, s *entity.Subscription) error {
	return r.db.WithContext(ctx).Save(s).Error
}

func (r *SubscriptionRepo) GetByID(ctx context.Context, id int64) (*entity.Subscription, error) {
	var s entity.Subscription
	err := r.db.WithContext(ctx).First(&s, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &s, err
}

// GetActive returns the single live subscription for an account+product (active/grace/trial).
func (r *SubscriptionRepo) GetActive(ctx context.Context, accountID int64, productID string) (*entity.Subscription, error) {
	var s entity.Subscription
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND product_id = ? AND status IN ('active','grace','trial')", accountID, productID).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &s, err
}

// ListByAccount returns all subscriptions for an account.
func (r *SubscriptionRepo) ListByAccount(ctx context.Context, accountID int64) ([]entity.Subscription, error) {
	var list []entity.Subscription
	err := r.db.WithContext(ctx).Where("account_id = ?", accountID).Order("id DESC").Find(&list).Error
	return list, err
}

// ListExpiring returns active subscriptions expiring within the next interval.
func (r *SubscriptionRepo) ListExpiring(ctx context.Context, withinHours int) ([]entity.Subscription, error) {
	var list []entity.Subscription
	err := r.db.WithContext(ctx).
		Where("status = 'active' AND expires_at IS NOT NULL AND expires_at <= NOW() + (? || ' hours')::interval", withinHours).
		Find(&list).Error
	return list, err
}

// ListActiveExpired returns active subscriptions where expires_at < now.
// These should be transitioned to the grace period.
func (r *SubscriptionRepo) ListActiveExpired(ctx context.Context) ([]entity.Subscription, error) {
	var list []entity.Subscription
	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at IS NOT NULL AND expires_at < NOW()", entity.SubStatusActive).
		Find(&list).Error
	return list, err
}

// ListGraceExpired returns grace-period subscriptions where grace_until < now.
// These should be permanently expired and downgraded.
func (r *SubscriptionRepo) ListGraceExpired(ctx context.Context) ([]entity.Subscription, error) {
	var list []entity.Subscription
	err := r.db.WithContext(ctx).
		Where("status = ? AND grace_until IS NOT NULL AND grace_until < NOW()", entity.SubStatusGrace).
		Find(&list).Error
	return list, err
}

// UpsertEntitlement creates or updates a single entitlement row.
func (r *SubscriptionRepo) UpsertEntitlement(ctx context.Context, e *entity.AccountEntitlement) error {
	return r.db.WithContext(ctx).
		Where("account_id = ? AND product_id = ? AND key = ?", e.AccountID, e.ProductID, e.Key).
		Assign(entity.AccountEntitlement{
			Value: e.Value, ValueType: e.ValueType,
			Source: e.Source, SourceRef: e.SourceRef, ExpiresAt: e.ExpiresAt,
		}).
		FirstOrCreate(e).Error
}

// GetEntitlements returns all entitlement rows for an account+product.
func (r *SubscriptionRepo) GetEntitlements(ctx context.Context, accountID int64, productID string) ([]entity.AccountEntitlement, error) {
	var list []entity.AccountEntitlement
	q := r.db.WithContext(ctx).Where("account_id = ? AND product_id = ?", accountID, productID)
	q = q.Where("expires_at IS NULL OR expires_at > NOW()")
	err := q.Find(&list).Error
	return list, err
}

// DeleteEntitlements removes all entitlements for account+product (used on subscription expiry).
func (r *SubscriptionRepo) DeleteEntitlements(ctx context.Context, accountID int64, productID string) error {
	return r.db.WithContext(ctx).
		Where("account_id = ? AND product_id = ?", accountID, productID).
		Delete(&entity.AccountEntitlement{}).Error
}
