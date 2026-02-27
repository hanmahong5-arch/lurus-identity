package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

// generateOrderNo creates a unique order number: "LO" + yyyyMMdd + 8-hex-chars.
func generateOrderNo(_ int64) string {
	return fmt.Sprintf("LO%s%s", time.Now().UTC().Format("20060102"), uuid.New().String()[:8])
}

// WalletService orchestrates topup, debit, and redemption use cases.
type WalletService struct {
	wallets walletStore
	vip     *VIPService
}

func NewWalletService(wallets walletStore, vip *VIPService) *WalletService {
	return &WalletService{wallets: wallets, vip: vip}
}

// GetWallet returns the wallet for an account (creates it if missing).
func (s *WalletService) GetWallet(ctx context.Context, accountID int64) (*entity.Wallet, error) {
	return s.wallets.GetOrCreate(ctx, accountID)
}

// Topup credits the wallet and triggers a VIP recalculation.
func (s *WalletService) Topup(ctx context.Context, accountID int64, amountCNY float64, orderNo string) (*entity.WalletTransaction, error) {
	tx, err := s.wallets.Credit(ctx, accountID, amountCNY,
		entity.TxTypeTopup,
		fmt.Sprintf("充值 %.2f CNY", amountCNY),
		"payment_order", orderNo, "")
	if err != nil {
		return nil, err
	}
	// Async-safe: VIP recalculation is idempotent
	_ = s.vip.RecalculateFromWallet(ctx, accountID)
	return tx, nil
}

// Credit adds a balance to the wallet (admin adjustments, bonuses, etc.).
func (s *WalletService) Credit(ctx context.Context, accountID int64, amount float64, txType, desc, refType, refID, productID string) (*entity.WalletTransaction, error) {
	return s.wallets.Credit(ctx, accountID, amount, txType, desc, refType, refID, productID)
}

// Debit charges the wallet for a product purchase or subscription.
func (s *WalletService) Debit(ctx context.Context, accountID int64, amount float64, txType, desc, productID, refType, refID string) (*entity.WalletTransaction, error) {
	return s.wallets.Debit(ctx, accountID, amount, txType, desc, refType, refID, productID)
}

// UpdatePaymentOrder persists changes to a payment order (e.g. storing the external ID).
func (s *WalletService) UpdatePaymentOrder(ctx context.Context, o *entity.PaymentOrder) error {
	return s.wallets.UpdatePaymentOrder(ctx, o)
}

// Redeem validates and applies a redemption code.
func (s *WalletService) Redeem(ctx context.Context, accountID int64, code string) error {
	rc, err := s.wallets.GetRedemptionCode(ctx, strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return fmt.Errorf("get redemption code: %w", err)
	}
	if rc == nil {
		return fmt.Errorf("invalid code")
	}
	if rc.ExpiresAt != nil && rc.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("code has expired")
	}
	if rc.UsedCount >= rc.MaxUses {
		return fmt.Errorf("code has reached its usage limit")
	}

	switch rc.RewardType {
	case "credits":
		if _, err := s.wallets.Credit(ctx, accountID, rc.RewardValue,
			entity.TxTypeRedemption,
			fmt.Sprintf("兑换码 %s 充值", rc.Code),
			"redemption_code", rc.Code, rc.ProductID); err != nil {
			return fmt.Errorf("credit wallet: %w", err)
		}
	default:
		return fmt.Errorf("unsupported reward type: %s", rc.RewardType)
	}

	rc.UsedCount++
	return s.wallets.UpdateRedemptionCode(ctx, rc)
}

// ListTransactions returns paginated wallet transactions.
func (s *WalletService) ListTransactions(ctx context.Context, accountID int64, page, pageSize int) ([]entity.WalletTransaction, int64, error) {
	return s.wallets.ListTransactions(ctx, accountID, page, pageSize)
}

// CreatePaymentOrder inserts a new pending order and returns it.
func (s *WalletService) CreatePaymentOrder(ctx context.Context, o *entity.PaymentOrder) error {
	return s.wallets.CreatePaymentOrder(ctx, o)
}

// CreateSubscriptionOrder creates a pending payment order for a subscription purchase.
func (s *WalletService) CreateSubscriptionOrder(ctx context.Context, o *entity.PaymentOrder) error {
	o.OrderNo = generateOrderNo(o.AccountID)
	o.Status = entity.OrderStatusPending
	return s.wallets.CreatePaymentOrder(ctx, o)
}

// CreateTopup creates a payment order for a wallet topup and returns the order.
// The caller is responsible for redirecting the user to the returned payURL.
func (s *WalletService) CreateTopup(ctx context.Context, accountID int64, amountCNY float64, method string) (*entity.PaymentOrder, error) {
	if amountCNY <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	o := &entity.PaymentOrder{
		AccountID:     accountID,
		OrderNo:       generateOrderNo(accountID),
		OrderType:     "topup",
		AmountCNY:     amountCNY,
		Currency:      "CNY",
		PaymentMethod: method,
		Status:        entity.OrderStatusPending,
	}
	if err := s.wallets.CreatePaymentOrder(ctx, o); err != nil {
		return nil, fmt.Errorf("create payment order: %w", err)
	}
	return o, nil
}

// ListOrders returns paginated payment orders for an account.
func (s *WalletService) ListOrders(ctx context.Context, accountID int64, page, pageSize int) ([]entity.PaymentOrder, int64, error) {
	return s.wallets.ListOrders(ctx, accountID, page, pageSize)
}

// GetOrderByNo returns a specific payment order, validating ownership.
func (s *WalletService) GetOrderByNo(ctx context.Context, accountID int64, orderNo string) (*entity.PaymentOrder, error) {
	o, err := s.wallets.GetPaymentOrderByNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fmt.Errorf("order %s not found", orderNo)
	}
	if o.AccountID != accountID {
		return nil, fmt.Errorf("order %s not found", orderNo) // obscure to prevent enumeration
	}
	return o, nil
}

// MarkOrderPaid marks an order as paid and credits the wallet.
func (s *WalletService) MarkOrderPaid(ctx context.Context, orderNo string) (*entity.PaymentOrder, error) {
	o, err := s.wallets.GetPaymentOrderByNo(ctx, orderNo)
	if err != nil || o == nil {
		return nil, fmt.Errorf("order %s not found", orderNo)
	}
	if o.Status == entity.OrderStatusPaid {
		return o, nil // idempotent
	}
	now := time.Now().UTC()
	o.Status = entity.OrderStatusPaid
	o.PaidAt = &now
	if err := s.wallets.UpdatePaymentOrder(ctx, o); err != nil {
		return nil, err
	}
	if o.OrderType == "topup" {
		if _, err := s.Topup(ctx, o.AccountID, o.AmountCNY, o.OrderNo); err != nil {
			return nil, fmt.Errorf("credit wallet: %w", err)
		}
	}
	return o, nil
}
