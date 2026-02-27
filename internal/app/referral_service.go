package app

import (
	"context"
	"fmt"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

// Referral reward rates (in Credits = CNY).
const (
	RewardSignup            = 5.0
	RewardFirstTopup        = 10.0
	RewardFirstSubscription = 20.0
)

// ReferralService processes referral chain reward events.
type ReferralService struct {
	accounts accountStore
	wallets  walletStore
}

func NewReferralService(accounts accountStore, wallets walletStore) *ReferralService {
	return &ReferralService{accounts: accounts, wallets: wallets}
}

// OnSignup awards a signup bonus to the referrer.
func (s *ReferralService) OnSignup(ctx context.Context, refereeID, referrerID int64) error {
	return s.reward(ctx, referrerID, refereeID, entity.ReferralEventSignup, RewardSignup)
}

// OnFirstTopup awards a topup bonus to the referrer.
func (s *ReferralService) OnFirstTopup(ctx context.Context, refereeID, referrerID int64) error {
	return s.reward(ctx, referrerID, refereeID, entity.ReferralEventFirstTopup, RewardFirstTopup)
}

// OnFirstSubscription awards a subscription bonus to the referrer.
func (s *ReferralService) OnFirstSubscription(ctx context.Context, refereeID, referrerID int64) error {
	return s.reward(ctx, referrerID, refereeID, entity.ReferralEventFirstSubscription, RewardFirstSubscription)
}

func (s *ReferralService) reward(ctx context.Context, referrerID, refereeID int64, eventType string, amount float64) error {
	referrer, err := s.accounts.GetByID(ctx, referrerID)
	if err != nil || referrer == nil {
		return fmt.Errorf("referrer %d not found", referrerID)
	}
	if _, err := s.wallets.GetOrCreate(ctx, referrerID); err != nil {
		return fmt.Errorf("ensure referrer wallet: %w", err)
	}
	_, err = s.wallets.Credit(ctx, referrerID, amount,
		entity.TxTypeReferralReward,
		fmt.Sprintf("推荐奖励 — %s", eventType),
		"referral_event",
		fmt.Sprintf("referee:%d", refereeID),
		"")
	return err
}
