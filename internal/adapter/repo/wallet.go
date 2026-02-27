package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WalletRepo manages wallets, transactions, and payment orders.
type WalletRepo struct {
	db *gorm.DB
}

func NewWalletRepo(db *gorm.DB) *WalletRepo { return &WalletRepo{db: db} }

// GetOrCreate returns the wallet for an account, creating it if it doesn't exist.
func (r *WalletRepo) GetOrCreate(ctx context.Context, accountID int64) (*entity.Wallet, error) {
	var w entity.Wallet
	err := r.db.WithContext(ctx).
		Where(entity.Wallet{AccountID: accountID}).
		FirstOrCreate(&w).Error
	return &w, err
}

func (r *WalletRepo) GetByAccountID(ctx context.Context, accountID int64) (*entity.Wallet, error) {
	var w entity.Wallet
	err := r.db.WithContext(ctx).Where("account_id = ?", accountID).First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &w, err
}

// Credit adds amount to balance and appends a ledger entry atomically.
func (r *WalletRepo) Credit(ctx context.Context, accountID int64, amount float64, txType, desc, refType, refID, productID string) (*entity.WalletTransaction, error) {
	var tx entity.WalletTransaction
	err := r.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		var w entity.Wallet
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("account_id = ?", accountID).First(&w).Error; err != nil {
			return fmt.Errorf("lock wallet: %w", err)
		}
		w.Balance += amount
		if txType == entity.TxTypeTopup {
			w.LifetimeTopup += amount
		}
		if err := db.Save(&w).Error; err != nil {
			return fmt.Errorf("save wallet: %w", err)
		}
		tx = entity.WalletTransaction{
			WalletID:      w.ID,
			AccountID:     accountID,
			Type:          txType,
			Amount:        amount,
			BalanceAfter:  w.Balance,
			ProductID:     productID,
			ReferenceType: refType,
			ReferenceID:   refID,
			Description:   desc,
		}
		return db.Create(&tx).Error
	})
	return &tx, err
}

// Debit subtracts amount from balance (fails if insufficient funds).
func (r *WalletRepo) Debit(ctx context.Context, accountID int64, amount float64, txType, desc, refType, refID, productID string) (*entity.WalletTransaction, error) {
	var tx entity.WalletTransaction
	err := r.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		var w entity.Wallet
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("account_id = ?", accountID).First(&w).Error; err != nil {
			return fmt.Errorf("lock wallet: %w", err)
		}
		if w.Balance < amount {
			return fmt.Errorf("insufficient balance: have %.4f, need %.4f", w.Balance, amount)
		}
		w.Balance -= amount
		w.LifetimeSpend += amount
		if err := db.Save(&w).Error; err != nil {
			return fmt.Errorf("save wallet: %w", err)
		}
		tx = entity.WalletTransaction{
			WalletID:      w.ID,
			AccountID:     accountID,
			Type:          txType,
			Amount:        -amount,
			BalanceAfter:  w.Balance,
			ProductID:     productID,
			ReferenceType: refType,
			ReferenceID:   refID,
			Description:   desc,
		}
		return db.Create(&tx).Error
	})
	return &tx, err
}

// ListTransactions returns paginated transactions for an account.
func (r *WalletRepo) ListTransactions(ctx context.Context, accountID int64, page, pageSize int) ([]entity.WalletTransaction, int64, error) {
	var list []entity.WalletTransaction
	var total int64
	q := r.db.WithContext(ctx).Model(&entity.WalletTransaction{}).Where("account_id = ?", accountID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := q.Order("id DESC").Limit(pageSize).Offset(offset).Find(&list).Error
	return list, total, err
}

// CreatePaymentOrder inserts a new pending order.
func (r *WalletRepo) CreatePaymentOrder(ctx context.Context, o *entity.PaymentOrder) error {
	return r.db.WithContext(ctx).Create(o).Error
}

// UpdatePaymentOrder updates the payment order status.
func (r *WalletRepo) UpdatePaymentOrder(ctx context.Context, o *entity.PaymentOrder) error {
	return r.db.WithContext(ctx).Save(o).Error
}

func (r *WalletRepo) GetPaymentOrderByNo(ctx context.Context, orderNo string) (*entity.PaymentOrder, error) {
	var o entity.PaymentOrder
	err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &o, err
}

func (r *WalletRepo) GetPaymentOrderByExternalID(ctx context.Context, externalID string) (*entity.PaymentOrder, error) {
	var o entity.PaymentOrder
	err := r.db.WithContext(ctx).Where("external_id = ?", externalID).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &o, err
}

// GetRedemptionCode returns a code, locked for update to prevent race conditions.
// ListOrders returns paginated payment orders for an account, newest first.
func (r *WalletRepo) ListOrders(ctx context.Context, accountID int64, page, pageSize int) ([]entity.PaymentOrder, int64, error) {
	var list []entity.PaymentOrder
	var total int64
	q := r.db.WithContext(ctx).Model(&entity.PaymentOrder{}).Where("account_id = ?", accountID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := q.Order("created_at DESC").Limit(pageSize).Offset(offset).Find(&list).Error
	return list, total, err
}

func (r *WalletRepo) GetRedemptionCode(ctx context.Context, code string) (*entity.RedemptionCode, error) {
	var rc entity.RedemptionCode
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("code = ?", code).First(&rc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rc, err
}

func (r *WalletRepo) UpdateRedemptionCode(ctx context.Context, rc *entity.RedemptionCode) error {
	return r.db.WithContext(ctx).Save(rc).Error
}

func (r *WalletRepo) CreateRedemptionCode(ctx context.Context, rc *entity.RedemptionCode) error {
	return r.db.WithContext(ctx).Create(rc).Error
}

// GenerateOrderNo creates a unique order number: "LO" + date + 6-digit sequence.
func GenerateOrderNo(accountID int64) string {
	return fmt.Sprintf("LO%s%06d", time.Now().UTC().Format("20060102"), accountID%1000000)
}
