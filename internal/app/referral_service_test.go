package app

import (
	"context"
	"math"
	"testing"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

// approxEqual returns true if |a - b| < epsilon.
func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestReferralService_Rewards verifies all referral reward paths.
func TestReferralService_Rewards(t *testing.T) {
	ctx := context.Background()

	setup := func() (*ReferralService, *mockAccountStore, *mockWalletStore) {
		as := newMockAccountStore()
		ws := newMockWalletStore()
		svc := NewReferralServiceWithCodes(as, ws, newMockRedemptionCodeStore())
		return svc, as, ws
	}

	seedPair := func(as *mockAccountStore) (referrer, referee entity.Account) {
		referrer = entity.Account{ZitadelSub: "ref-er", Email: "referrer@test.com", DisplayName: "Referrer"}
		_ = as.Create(ctx, &referrer)
		referee = entity.Account{ZitadelSub: "ref-ee", Email: "referee@test.com", DisplayName: "Referee"}
		_ = as.Create(ctx, &referee)
		return referrer, referee
	}

	t.Run("OnSignup_credits_5_LB", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		if err := svc.OnSignup(ctx, referee.ID, referrer.ID); err != nil {
			t.Fatalf("OnSignup error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		if w.Balance != RewardSignup {
			t.Errorf("balance = %.2f, want %.2f", w.Balance, RewardSignup)
		}
	})

	t.Run("OnFirstTopup_credits_10_LB", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		if err := svc.OnFirstTopup(ctx, referee.ID, referrer.ID); err != nil {
			t.Fatalf("OnFirstTopup error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		if w.Balance != RewardFirstTopup {
			t.Errorf("balance = %.2f, want %.2f", w.Balance, RewardFirstTopup)
		}
	})

	t.Run("OnFirstSubscription_credits_30_LB", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		if err := svc.OnFirstSubscription(ctx, referee.ID, referrer.ID); err != nil {
			t.Fatalf("OnFirstSubscription error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		if w.Balance != RewardFirstSubscription {
			t.Errorf("balance = %.2f, want %.2f", w.Balance, RewardFirstSubscription)
		}
	})

	t.Run("OnRenewal_credits_5pct_of_amount", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		const subAmount = 199.0
		if err := svc.OnRenewal(ctx, referee.ID, referrer.ID, subAmount, 1); err != nil {
			t.Fatalf("OnRenewal error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		want := subAmount * RewardRenewalRate
		if !approxEqual(w.Balance, want, 1e-9) {
			t.Errorf("balance = %v, want %v (diff=%v)", w.Balance, want, w.Balance-want)
		}
	})

	t.Run("OnRenewal_exceeds_6_no_reward", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		// Renewal count 7 should produce no reward
		if err := svc.OnRenewal(ctx, referee.ID, referrer.ID, 199.0, 7); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		if w.Balance != 0 {
			t.Errorf("balance = %.2f, want 0 (no reward after 6 renewals)", w.Balance)
		}
	})

	t.Run("OnRenewal_at_boundary_6_rewards", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		const subAmount = 100.0
		if err := svc.OnRenewal(ctx, referee.ID, referrer.ID, subAmount, 6); err != nil {
			t.Fatalf("OnRenewal(6) error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		want := subAmount * RewardRenewalRate
		if !approxEqual(w.Balance, want, 1e-9) {
			t.Errorf("balance = %v, want %v (renewal #6 should still be rewarded)", w.Balance, want)
		}
	})

	t.Run("OnRenewal_zero_amount_no_reward", func(t *testing.T) {
		svc, as, ws := setup()
		referrer, referee := seedPair(as)
		_, _ = ws.GetOrCreate(ctx, referrer.ID)

		if err := svc.OnRenewal(ctx, referee.ID, referrer.ID, 0, 1); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		w, _ := ws.GetByAccountID(ctx, referrer.ID)
		if w.Balance != 0 {
			t.Errorf("balance = %.2f, want 0 for zero-amount renewal", w.Balance)
		}
	})

	t.Run("unknown_referrer_returns_error", func(t *testing.T) {
		svc, _, _ := setup()
		err := svc.OnSignup(ctx, 2, 999) // referrer 999 does not exist
		if err == nil {
			t.Error("expected error for unknown referrer, got nil")
		}
	})
}

// TestReferralRewardConstants ensures constants are not accidentally lowered.
func TestReferralRewardConstants(t *testing.T) {
	if RewardSignup != 5.0 {
		t.Errorf("RewardSignup = %.1f, want 5.0", RewardSignup)
	}
	if RewardFirstTopup != 10.0 {
		t.Errorf("RewardFirstTopup = %.1f, want 10.0", RewardFirstTopup)
	}
	if RewardFirstSubscription < 30.0 {
		t.Errorf("RewardFirstSubscription = %.1f, must be >= 30.0", RewardFirstSubscription)
	}
	if RewardRenewalRate != 0.05 {
		t.Errorf("RewardRenewalRate = %.3f, want 0.05", RewardRenewalRate)
	}
}
