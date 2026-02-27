package app

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
)

func TestGenerateAffCode(t *testing.T) {
	code, err := generateAffCode()
	if err != nil {
		t.Fatalf("generateAffCode returned error: %v", err)
	}
	if len(code) != 8 {
		t.Errorf("len(code)=%d, want 8; got %q", len(code), code)
	}
	if _, err := hex.DecodeString(code); err != nil {
		t.Errorf("aff code %q is not valid hex: %v", code, err)
	}
	if strings.ToLower(code) != code {
		t.Errorf("aff code %q is not lowercase", code)
	}
}

func TestGenerateAffCode_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		code, err := generateAffCode()
		if err != nil {
			t.Fatalf("generateAffCode error at iteration %d: %v", i, err)
		}
		if seen[code] {
			t.Errorf("duplicate aff code %q at iteration %d", code, i)
		}
		seen[code] = true
	}
}

// ── AccountService integration tests (using in-memory mocks) ─────────────────

func makeAccountService() *AccountService {
	return NewAccountService(newMockAccountStore(), newMockWalletStore(), newMockVIPStore(nil))
}

func TestAccountService_UpsertByZitadelSub_NewAccount(t *testing.T) {
	svc := makeAccountService()
	ctx := context.Background()

	a, err := svc.UpsertByZitadelSub(ctx, "sub-001", "alice@example.com", "Alice", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("expected non-nil account")
	}
	if a.Email != "alice@example.com" {
		t.Errorf("Email=%q, want alice@example.com", a.Email)
	}
	if a.ZitadelSub != "sub-001" {
		t.Errorf("ZitadelSub=%q, want sub-001", a.ZitadelSub)
	}
	if a.LurusID == "" {
		t.Error("LurusID should not be empty after creation")
	}
	if len(a.AffCode) != 8 {
		t.Errorf("AffCode len=%d, want 8", len(a.AffCode))
	}
}

func TestAccountService_UpsertByZitadelSub_ExistingSub(t *testing.T) {
	svc := makeAccountService()
	ctx := context.Background()

	// Create
	a1, _ := svc.UpsertByZitadelSub(ctx, "sub-002", "bob@example.com", "Bob", "")
	// Upsert again with same sub — should update display name, not create new
	a2, err := svc.UpsertByZitadelSub(ctx, "sub-002", "bob@example.com", "Bobby", "https://avatar.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a2.ID != a1.ID {
		t.Errorf("expected same account ID, got %d vs %d", a2.ID, a1.ID)
	}
	if a2.DisplayName != "Bobby" {
		t.Errorf("DisplayName=%q, want Bobby", a2.DisplayName)
	}
}

func TestAccountService_UpsertByZitadelSub_EmailMatchLinksSub(t *testing.T) {
	// Simulates: account exists with email but no ZitadelSub yet
	store := newMockAccountStore()
	svc := NewAccountService(store, newMockWalletStore(), newMockVIPStore(nil))
	ctx := context.Background()

	// Pre-create account with email but no sub
	a1, _ := svc.UpsertByZitadelSub(ctx, "", "carol@example.com", "Carol", "")
	if a1 == nil {
		t.Fatal("pre-create failed")
	}

	// Now upsert with same email + a zitadel sub
	a2, err := svc.UpsertByZitadelSub(ctx, "sub-003", "carol@example.com", "Carol Z", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a2.ZitadelSub != "sub-003" {
		t.Errorf("ZitadelSub=%q, want sub-003", a2.ZitadelSub)
	}
}

func TestAccountService_GetByID_NotFound(t *testing.T) {
	svc := makeAccountService()
	a, err := svc.GetByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != nil {
		t.Errorf("expected nil for unknown ID, got %+v", a)
	}
}

func TestAccountService_GetByZitadelSub(t *testing.T) {
	svc := makeAccountService()
	ctx := context.Background()
	created, _ := svc.UpsertByZitadelSub(ctx, "sub-zit", "dan@example.com", "Dan", "")

	got, err := svc.GetByZitadelSub(ctx, "sub-zit")
	if err != nil {
		t.Fatalf("GetByZitadelSub error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil account")
	}
	if got.ID != created.ID {
		t.Errorf("ID=%d, want %d", got.ID, created.ID)
	}
}

func TestAccountService_Update(t *testing.T) {
	svc := makeAccountService()
	ctx := context.Background()
	a, _ := svc.UpsertByZitadelSub(ctx, "sub-upd", "eve@example.com", "Eve", "")

	a.DisplayName = "Eve Updated"
	if err := svc.Update(ctx, a); err != nil {
		t.Fatalf("Update error: %v", err)
	}
	// Verify through GetByID
	got, _ := svc.GetByID(ctx, a.ID)
	if got == nil || got.DisplayName != "Eve Updated" {
		t.Errorf("DisplayName after update=%q, want Eve Updated", got.DisplayName)
	}
}

func TestAccountService_List(t *testing.T) {
	svc := makeAccountService()
	ctx := context.Background()
	_, _ = svc.UpsertByZitadelSub(ctx, "sub-a", "a@example.com", "A", "")
	_, _ = svc.UpsertByZitadelSub(ctx, "sub-b", "b@example.com", "B", "")

	accounts, total, err := svc.List(ctx, "", 1, 10)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if total < 2 {
		t.Errorf("total=%d, want ≥2", total)
	}
	if len(accounts) < 2 {
		t.Errorf("len(accounts)=%d, want ≥2", len(accounts))
	}
}

func TestAccountService_BindOAuth(t *testing.T) {
	svc := makeAccountService()
	ctx := context.Background()
	a, _ := svc.UpsertByZitadelSub(ctx, "sub-oauth", "frank@example.com", "Frank", "")

	err := svc.BindOAuth(ctx, a.ID, "github", "gh-12345", "frank@github.com")
	if err != nil {
		t.Fatalf("BindOAuth error: %v", err)
	}
}
