package app

import (
	"context"
	"testing"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

// mockOverviewCache implements overviewCache for testing.
type mockOverviewCache struct {
	data map[string][]byte
}

func newMockOverviewCache() *mockOverviewCache {
	return &mockOverviewCache{data: make(map[string][]byte)}
}

func (m *mockOverviewCache) Get(_ context.Context, accountID int64, productID string) ([]byte, error) {
	k := overviewCacheKey(accountID, productID)
	v, ok := m.data[k]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (m *mockOverviewCache) Set(_ context.Context, accountID int64, productID string, data []byte) error {
	m.data[overviewCacheKey(accountID, productID)] = data
	return nil
}

func (m *mockOverviewCache) Invalidate(_ context.Context, accountID int64, productID string) error {
	delete(m.data, overviewCacheKey(accountID, productID))
	return nil
}

func overviewCacheKey(accountID int64, productID string) string {
	return string(rune(accountID)) + ":" + productID
}

func makeOverviewService() (*OverviewService, *mockAccountStore, *mockOverviewCache) {
	as := newMockAccountStore()
	ws := newMockWalletStore()
	vs := NewVIPService(newMockVIPStore(nil), ws)
	ss := NewSubscriptionService(newMockSubStore(), newMockPlanStore(), NewEntitlementService(newMockSubStore(), newMockPlanStore(), newMockCache()), 3)
	ps := newMockPlanStore()
	oc := newMockOverviewCache()
	svc := NewOverviewService(as, vs, ws, ss, ps, oc)
	return svc, as, oc
}

func TestOverviewService_Get_CacheMiss(t *testing.T) {
	svc, as, _ := makeOverviewService()
	ctx := context.Background()

	// Seed an account
	_ = as.Create(ctx, &entity.Account{DisplayName: "Alice", ZitadelSub: "sub-1", LurusID: "LU0000001"})

	ov, err := svc.Get(ctx, 1, "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ov.Account.DisplayName != "Alice" {
		t.Errorf("DisplayName=%q, want Alice", ov.Account.DisplayName)
	}
	if ov.TopupURL != topupURL {
		t.Errorf("TopupURL=%q, want %q", ov.TopupURL, topupURL)
	}
}

func TestOverviewService_Get_CacheHit(t *testing.T) {
	svc, as, oc := makeOverviewService()
	ctx := context.Background()

	_ = as.Create(ctx, &entity.Account{DisplayName: "Bob", ZitadelSub: "sub-2", LurusID: "LU0000002"})

	// Warm the cache by calling Get
	_, _ = svc.Get(ctx, 1, "")

	// Cache should now be populated
	if len(oc.data) == 0 {
		t.Error("expected cache to be populated after first Get")
	}

	// Second call should use cache (no error even if underlying data is fine)
	ov, err := svc.Get(ctx, 1, "")
	if err != nil {
		t.Fatalf("cached Get: %v", err)
	}
	if ov.Account.DisplayName != "Bob" {
		t.Errorf("DisplayName=%q, want Bob", ov.Account.DisplayName)
	}
}

func TestOverviewService_Get_AccountNotFound(t *testing.T) {
	svc, _, _ := makeOverviewService()

	_, err := svc.Get(context.Background(), 999, "")
	if err == nil {
		t.Fatal("expected error for missing account")
	}
}

func TestOverviewService_Get_WithProductID(t *testing.T) {
	svc, as, _ := makeOverviewService()
	ctx := context.Background()

	_ = as.Create(ctx, &entity.Account{DisplayName: "Charlie", ZitadelSub: "sub-3", LurusID: "LU0000003"})

	// No active subscription → Subscription should be nil
	ov, err := svc.Get(ctx, 1, "llm-api")
	if err != nil {
		t.Fatalf("Get with productID: %v", err)
	}
	if ov.Subscription != nil {
		t.Error("expected nil Subscription when no active sub exists")
	}
}

