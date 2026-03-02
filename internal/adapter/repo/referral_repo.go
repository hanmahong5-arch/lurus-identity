package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// ReferralRepo provides read-only statistics over referral reward events.
type ReferralRepo struct {
	db *gorm.DB
}

// NewReferralRepo creates a new ReferralRepo.
func NewReferralRepo(db *gorm.DB) *ReferralRepo {
	return &ReferralRepo{db: db}
}

// GetReferralStats returns the total number of distinct referees and total LB rewarded
// for the given referrer account, based on the billing.wallet_transactions ledger.
//
// Each referral reward entry has type='referral_reward' and reference_id='referee:<id>'.
// Counting DISTINCT reference_id gives unique referees even when multiple events fire
// per referee (signup + first_topup + first_subscription).
func (r *ReferralRepo) GetReferralStats(ctx context.Context, referrerAccountID int64) (totalReferrals int, totalRewardedLB float64, err error) {
	var result struct {
		TotalReferrals  int     `gorm:"column:total_referrals"`
		TotalRewardedLB float64 `gorm:"column:total_rewarded_lb"`
	}
	err = r.db.WithContext(ctx).Raw(`
		SELECT
			COUNT(DISTINCT reference_id) AS total_referrals,
			COALESCE(SUM(amount), 0)     AS total_rewarded_lb
		FROM billing.wallet_transactions
		WHERE account_id = ? AND type = 'referral_reward'
	`, referrerAccountID).Scan(&result).Error
	if err != nil {
		return 0, 0, fmt.Errorf("query referral stats: %w", err)
	}
	return result.TotalReferrals, result.TotalRewardedLB, nil
}
