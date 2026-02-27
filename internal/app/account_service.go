// Package app contains use case orchestration — no framework types allowed here.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

// AccountService orchestrates account creation, lookup, and OAuth binding.
type AccountService struct {
	accounts accountStore
	wallets  walletStore
	vip      vipStore
}

func NewAccountService(accounts accountStore, wallets walletStore, vip vipStore) *AccountService {
	return &AccountService{accounts: accounts, wallets: wallets, vip: vip}
}

// UpsertByZitadelSub creates or updates the account linked to a Zitadel OIDC sub.
// This is called on every OIDC login callback.
func (s *AccountService) UpsertByZitadelSub(ctx context.Context, sub, email, displayName, avatarURL string) (*entity.Account, error) {
	// Try by sub first (fastest, indexed)
	a, err := s.accounts.GetByZitadelSub(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("lookup by sub: %w", err)
	}
	if a != nil {
		// Update mutable fields that may change in Zitadel
		a.DisplayName = displayName
		a.AvatarURL = avatarURL
		if err := s.accounts.Update(ctx, a); err != nil {
			return nil, fmt.Errorf("update account: %w", err)
		}
		return a, nil
	}

	// Fall back to email match (handles accounts created before Zitadel)
	a, err = s.accounts.GetByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("lookup by email: %w", err)
	}
	if a != nil {
		a.ZitadelSub = sub
		a.DisplayName = displayName
		a.AvatarURL = avatarURL
		if err := s.accounts.Update(ctx, a); err != nil {
			return nil, fmt.Errorf("link zitadel sub: %w", err)
		}
		return a, nil
	}

	// New account
	affCode, err := generateAffCode()
	if err != nil {
		return nil, fmt.Errorf("generate aff code: %w", err)
	}
	a = &entity.Account{
		ZitadelSub:  sub,
		Email:       email,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		AffCode:     affCode,
		Status:      entity.AccountStatusActive,
	}
	if err := s.accounts.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	// Assign human-readable LurusID after insert (needs auto-increment ID)
	a.LurusID = entity.GenerateLurusID(a.ID)
	if err := s.accounts.Update(ctx, a); err != nil {
		return nil, fmt.Errorf("set lurus_id: %w", err)
	}

	// Bootstrap wallet and VIP row
	if _, err := s.wallets.GetOrCreate(ctx, a.ID); err != nil {
		return nil, fmt.Errorf("create wallet: %w", err)
	}
	if _, err := s.vip.GetOrCreate(ctx, a.ID); err != nil {
		return nil, fmt.Errorf("create vip: %w", err)
	}

	return a, nil
}

// GetByID returns an account by its numeric ID.
func (s *AccountService) GetByID(ctx context.Context, id int64) (*entity.Account, error) {
	return s.accounts.GetByID(ctx, id)
}

// GetByZitadelSub returns an account by Zitadel OIDC sub.
func (s *AccountService) GetByZitadelSub(ctx context.Context, sub string) (*entity.Account, error) {
	return s.accounts.GetByZitadelSub(ctx, sub)
}

// Update persists account profile changes.
func (s *AccountService) Update(ctx context.Context, a *entity.Account) error {
	return s.accounts.Update(ctx, a)
}

// List returns paginated accounts for admin use.
func (s *AccountService) List(ctx context.Context, keyword string, page, pageSize int) ([]*entity.Account, int64, error) {
	return s.accounts.List(ctx, keyword, page, pageSize)
}

// BindOAuth links a third-party OAuth provider to an account.
func (s *AccountService) BindOAuth(ctx context.Context, accountID int64, provider, providerID, providerEmail string) error {
	return s.accounts.UpsertOAuthBinding(ctx, &entity.OAuthBinding{
		AccountID:     accountID,
		Provider:      provider,
		ProviderID:    providerID,
		ProviderEmail: providerEmail,
	})
}

// generateAffCode produces a random 8-character hex referral code.
func generateAffCode() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
