package app

import (
	"context"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

// accountStore is the minimal DB interface required by AccountService.
type accountStore interface {
	Create(ctx context.Context, a *entity.Account) error
	Update(ctx context.Context, a *entity.Account) error
	GetByID(ctx context.Context, id int64) (*entity.Account, error)
	GetByEmail(ctx context.Context, email string) (*entity.Account, error)
	GetByZitadelSub(ctx context.Context, sub string) (*entity.Account, error)
	GetByLurusID(ctx context.Context, lurusID string) (*entity.Account, error)
	List(ctx context.Context, keyword string, page, pageSize int) ([]*entity.Account, int64, error)
	UpsertOAuthBinding(ctx context.Context, b *entity.OAuthBinding) error
}

// walletStore is the minimal DB interface required by WalletService and VIPService.
type walletStore interface {
	GetOrCreate(ctx context.Context, accountID int64) (*entity.Wallet, error)
	GetByAccountID(ctx context.Context, accountID int64) (*entity.Wallet, error)
	Credit(ctx context.Context, accountID int64, amount float64, txType, desc, refType, refID, productID string) (*entity.WalletTransaction, error)
	Debit(ctx context.Context, accountID int64, amount float64, txType, desc, refType, refID, productID string) (*entity.WalletTransaction, error)
	ListTransactions(ctx context.Context, accountID int64, page, pageSize int) ([]entity.WalletTransaction, int64, error)
	CreatePaymentOrder(ctx context.Context, o *entity.PaymentOrder) error
	UpdatePaymentOrder(ctx context.Context, o *entity.PaymentOrder) error
	GetPaymentOrderByNo(ctx context.Context, orderNo string) (*entity.PaymentOrder, error)
	GetRedemptionCode(ctx context.Context, code string) (*entity.RedemptionCode, error)
	UpdateRedemptionCode(ctx context.Context, rc *entity.RedemptionCode) error
	ListOrders(ctx context.Context, accountID int64, page, pageSize int) ([]entity.PaymentOrder, int64, error)
}

// vipStore is the minimal DB interface required by VIPService.
type vipStore interface {
	GetOrCreate(ctx context.Context, accountID int64) (*entity.AccountVIP, error)
	Update(ctx context.Context, v *entity.AccountVIP) error
	ListConfigs(ctx context.Context) ([]entity.VIPLevelConfig, error)
}

// subscriptionStore is the minimal DB interface required by SubscriptionService.
type subscriptionStore interface {
	Create(ctx context.Context, s *entity.Subscription) error
	Update(ctx context.Context, s *entity.Subscription) error
	GetByID(ctx context.Context, id int64) (*entity.Subscription, error)
	GetActive(ctx context.Context, accountID int64, productID string) (*entity.Subscription, error)
	ListByAccount(ctx context.Context, accountID int64) ([]entity.Subscription, error)
	ListActiveExpired(ctx context.Context) ([]entity.Subscription, error)
	ListGraceExpired(ctx context.Context) ([]entity.Subscription, error)
	UpsertEntitlement(ctx context.Context, e *entity.AccountEntitlement) error
	GetEntitlements(ctx context.Context, accountID int64, productID string) ([]entity.AccountEntitlement, error)
	DeleteEntitlements(ctx context.Context, accountID int64, productID string) error
}

// planStore is the minimal DB interface for product/plan lookups.
type planStore interface {
	GetPlanByID(ctx context.Context, id int64) (*entity.ProductPlan, error)
	ListActive(ctx context.Context) ([]entity.Product, error)
	ListPlans(ctx context.Context, productID string) ([]entity.ProductPlan, error)
	GetByID(ctx context.Context, id string) (*entity.Product, error)
	Create(ctx context.Context, p *entity.Product) error
	Update(ctx context.Context, p *entity.Product) error
	CreatePlan(ctx context.Context, p *entity.ProductPlan) error
	UpdatePlan(ctx context.Context, p *entity.ProductPlan) error
}

// entitlementCache is the minimal cache interface for entitlement lookups.
// Uses map[string]string to avoid importing the cache package (cache.EntitlementMap is that type).
type entitlementCache interface {
	Get(ctx context.Context, accountID int64, productID string) (map[string]string, error)
	Set(ctx context.Context, accountID int64, productID string, em map[string]string) error
	Invalidate(ctx context.Context, accountID int64, productID string) error
}
