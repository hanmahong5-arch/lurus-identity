package app

import (
	"context"
	"time"

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
	// GetByOAuthBinding looks up an account via its OAuth provider binding.
	GetByOAuthBinding(ctx context.Context, provider, providerID string) (*entity.Account, error)
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
	// ListDueForRenewal returns active subscriptions that are due for auto-renewal.
	ListDueForRenewal(ctx context.Context) ([]entity.Subscription, error)
	// UpdateRenewalState persists renewal attempt counter and next retry time.
	UpdateRenewalState(ctx context.Context, subID int64, attempts int, nextAt *time.Time) error
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

// invoiceStore defines persistence operations for invoices.
type invoiceStore interface {
	Create(ctx context.Context, inv *entity.Invoice) error
	GetByOrderNo(ctx context.Context, orderNo string) (*entity.Invoice, error)
	GetByInvoiceNo(ctx context.Context, invoiceNo string) (*entity.Invoice, error)
	ListByAccount(ctx context.Context, accountID int64, page, pageSize int) ([]entity.Invoice, int64, error)
	AdminList(ctx context.Context, filterAccountID int64, page, pageSize int) ([]entity.Invoice, int64, error)
}

// redemptionCodeStore defines persistence operations for bulk redemption code creation.
type redemptionCodeStore interface {
	BulkCreate(ctx context.Context, codes []entity.RedemptionCode) error
}

// refundStore defines persistence operations for refunds.
type refundStore interface {
	Create(ctx context.Context, r *entity.Refund) error
	GetByRefundNo(ctx context.Context, refundNo string) (*entity.Refund, error)
	GetPendingByOrderNo(ctx context.Context, orderNo string) (*entity.Refund, error)
	UpdateStatus(ctx context.Context, refundNo, status, reviewNote, reviewedBy string, reviewedAt *time.Time) error
	MarkCompleted(ctx context.Context, refundNo string, completedAt time.Time) error
	ListByAccount(ctx context.Context, accountID int64, page, pageSize int) ([]entity.Refund, int64, error)
}

// orgStore is the minimal DB interface required by OrganizationService.
type orgStore interface {
	// organization CRUD
	Create(ctx context.Context, org *entity.Organization) error
	GetByID(ctx context.Context, id int64) (*entity.Organization, error)
	GetBySlug(ctx context.Context, slug string) (*entity.Organization, error)
	ListByAccountID(ctx context.Context, accountID int64) ([]entity.Organization, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	ListAll(ctx context.Context, limit, offset int) ([]entity.Organization, error)
	// members
	AddMember(ctx context.Context, m *entity.OrgMember) error
	RemoveMember(ctx context.Context, orgID, accountID int64) error
	GetMember(ctx context.Context, orgID, accountID int64) (*entity.OrgMember, error)
	ListMembers(ctx context.Context, orgID int64) ([]entity.OrgMember, error)
	// api keys
	CreateAPIKey(ctx context.Context, k *entity.OrgAPIKey) error
	GetAPIKeyByHash(ctx context.Context, hash string) (*entity.OrgAPIKey, error)
	ListAPIKeys(ctx context.Context, orgID int64) ([]entity.OrgAPIKey, error)
	RevokeAPIKey(ctx context.Context, id int64) error
	TouchAPIKey(ctx context.Context, id int64) error
	// wallet
	GetOrCreateWallet(ctx context.Context, orgID int64) (*entity.OrgWallet, error)
}
