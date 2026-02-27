package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

const (
	// bulkGenerateMaxCount is the maximum number of redemption codes per batch request.
	bulkGenerateMaxCount = 1000
	// codeLength is the number of random bytes used to produce each 8-char code.
	codeLength = 4
)

// Referral reward rates (in Credits = CNY).
const (
	RewardSignup            = 5.0
	RewardFirstTopup        = 10.0
	RewardFirstSubscription = 20.0
)

// ReferralService processes referral chain reward events and bulk code generation.
type ReferralService struct {
	accounts      accountStore
	wallets       walletStore
	redemptions   redemptionCodeStore
}

// NewReferralService creates a ReferralService without bulk-code support (legacy path).
func NewReferralService(accounts accountStore, wallets walletStore) *ReferralService {
	return &ReferralService{accounts: accounts, wallets: wallets}
}

// NewReferralServiceWithCodes creates a ReferralService with bulk-code support.
func NewReferralServiceWithCodes(accounts accountStore, wallets walletStore, redemptions redemptionCodeStore) *ReferralService {
	return &ReferralService{accounts: accounts, wallets: wallets, redemptions: redemptions}
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

// BulkGenerateCodes generates count unique redemption codes in a single batch.
// count must be in [1, 1000]. Each code is 8 uppercase alphanumeric characters.
// The returned slice is in the same order as the generated codes.
func (s *ReferralService) BulkGenerateCodes(
	ctx context.Context,
	productID, planCode string,
	durationDays int,
	expiresAt *time.Time,
	notes string,
	count int,
) ([]entity.RedemptionCode, error) {
	if count < 1 || count > bulkGenerateMaxCount {
		return nil, fmt.Errorf("count must be between 1 and %d, got %d", bulkGenerateMaxCount, count)
	}
	if s.redemptions == nil {
		return nil, fmt.Errorf("redemption code store not configured")
	}

	codes := make([]entity.RedemptionCode, 0, count)
	seen := make(map[string]struct{}, count)

	for len(codes) < count {
		code, err := generateCode()
		if err != nil {
			return nil, fmt.Errorf("generate code: %w", err)
		}
		if _, dup := seen[code]; dup {
			continue // retry on collision
		}
		seen[code] = struct{}{}
		rc := entity.RedemptionCode{
			Code:         code,
			ProductID:    productID,
			RewardType:   "subscription_trial",
			RewardValue:  float64(durationDays),
			MaxUses:      1,
			ExpiresAt:    expiresAt,
			BatchID:      "",
			RewardMetadata: []byte(`{"plan_code":"` + planCode + `","duration_days":` + fmt.Sprintf("%d", durationDays) + `}`),
		}
		// notes stored in description field via RewardMetadata or via Code notes workaround;
		// entity has no Notes field, so we embed notes into BatchID as a readable prefix.
		if notes != "" {
			rc.BatchID = notes
		}
		codes = append(codes, rc)
	}

	if err := s.redemptions.BulkCreate(ctx, codes); err != nil {
		return nil, fmt.Errorf("bulk create codes: %w", err)
	}
	return codes, nil
}

// generateCode returns a random 8-character uppercase alphanumeric code.
func generateCode() (string, error) {
	b := make([]byte, codeLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	raw := strings.ToUpper(hex.EncodeToString(b)) // 8 hex chars
	return raw, nil
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
