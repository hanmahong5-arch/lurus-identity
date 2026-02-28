package app

import (
	"context"
	"testing"
	"time"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/config"
)

// mockAdminSettingStore is an in-memory implementation of adminSettingStore for tests.
type mockAdminSettingStore struct {
	settings []entity.AdminSetting
	setErr   error
	setCalls []struct{ key, value, updatedBy string }
}

func (m *mockAdminSettingStore) GetAll(_ context.Context) ([]entity.AdminSetting, error) {
	return m.settings, nil
}

func (m *mockAdminSettingStore) Set(_ context.Context, key, value, updatedBy string) error {
	m.setCalls = append(m.setCalls, struct{ key, value, updatedBy string }{key, value, updatedBy})
	if m.setErr != nil {
		return m.setErr
	}
	for i := range m.settings {
		if m.settings[i].Key == key {
			m.settings[i].Value = value
			m.settings[i].UpdatedBy = updatedBy
			m.settings[i].UpdatedAt = time.Now()
			return nil
		}
	}
	m.settings = append(m.settings, entity.AdminSetting{
		Key: key, Value: value, UpdatedBy: updatedBy, UpdatedAt: time.Now(),
	})
	return nil
}

func TestAdminConfig_Load_PopulatesCache(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{
			{Key: "epay_key", Value: "sk_test_123"},
			{Key: "creem_api_key", Value: "creem_456"},
		},
	}
	svc := NewAdminConfigService(store)
	if err := svc.Load(context.Background()); err != nil {
		t.Fatalf("Load error: %v", err)
	}
	// Cache populated — GetEffective should return DB value without env fallback
	got := svc.GetEffective("epay_key", "env_default")
	if got != "sk_test_123" {
		t.Errorf("GetEffective = %q, want %q", got, "sk_test_123")
	}
}

func TestAdminConfig_GetEffective_DBWins(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{{Key: "epay_partner_id", Value: "db-value"}},
	}
	svc := NewAdminConfigService(store)
	_ = svc.Load(context.Background())

	got := svc.GetEffective("epay_partner_id", "env-value")
	if got != "db-value" {
		t.Errorf("GetEffective = %q, want %q (DB should win over env)", got, "db-value")
	}
}

func TestAdminConfig_GetEffective_EnvFallback(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{{Key: "epay_partner_id", Value: ""}},
	}
	svc := NewAdminConfigService(store)
	_ = svc.Load(context.Background())

	got := svc.GetEffective("epay_partner_id", "env-fallback")
	if got != "env-fallback" {
		t.Errorf("GetEffective = %q, want %q (empty DB value should fall back to env)", got, "env-fallback")
	}
}

func TestAdminConfig_Set_UpdatesCacheAndRepo(t *testing.T) {
	store := &mockAdminSettingStore{}
	svc := NewAdminConfigService(store)
	ctx := context.Background()

	if err := svc.Set(ctx, "epay_key", "new-value", "admin@test.com"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if len(store.setCalls) == 0 {
		t.Error("expected repo.Set to be called at least once")
	}
	// Cache should reflect the new value
	got := svc.GetEffective("epay_key", "")
	if got != "new-value" {
		t.Errorf("GetEffective after Set = %q, want %q", got, "new-value")
	}
}

func TestAdminConfig_LoadAll_ReturnsSettings(t *testing.T) {
	settings := []entity.AdminSetting{
		{Key: "key1", Value: "val1"},
		{Key: "key2", Value: "val2"},
	}
	store := &mockAdminSettingStore{settings: settings}
	svc := NewAdminConfigService(store)
	ctx := context.Background()

	got, err := svc.LoadAll(ctx)
	if err != nil {
		t.Fatalf("LoadAll error: %v", err)
	}
	if len(got) != len(settings) {
		t.Errorf("LoadAll returned %d settings, want %d", len(got), len(settings))
	}
}

func TestAdminConfig_ApplyToConfig_Override(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{
			{Key: "epay_partner_id", Value: "db-partner-id"},
		},
	}
	svc := NewAdminConfigService(store)
	_ = svc.Load(context.Background())

	cfg := &config.Config{EpayPartnerID: "env-partner-id"}
	svc.ApplyToConfig(cfg)

	if cfg.EpayPartnerID != "db-partner-id" {
		t.Errorf("cfg.EpayPartnerID = %q, want %q (DB should override env)", cfg.EpayPartnerID, "db-partner-id")
	}
}

func TestAdminConfig_ApplyToConfig_MultipleOverrides(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{
			{Key: "epay_partner_id", Value: "pid-db"},
			{Key: "epay_key", Value: "ekey-db"},
			{Key: "epay_gateway_url", Value: "https://gateway.db"},
			{Key: "epay_notify_url", Value: "https://notify.db"},
			{Key: "stripe_secret_key", Value: "stripe-sk-db"},
			{Key: "stripe_webhook_secret", Value: "stripe-wh-db"},
			{Key: "creem_api_key", Value: "creem-key-db"},
			{Key: "creem_webhook_secret", Value: "creem-wh-db"},
		},
	}
	svc := NewAdminConfigService(store)
	_ = svc.Load(context.Background())

	cfg := &config.Config{
		EpayPartnerID:       "pid-env",
		EpayKey:             "ekey-env",
		EpayGatewayURL:      "https://gateway.env",
		EpayNotifyURL:       "https://notify.env",
		StripeSecretKey:     "stripe-sk-env",
		StripeWebhookSecret: "stripe-wh-env",
		CreemAPIKey:         "creem-key-env",
		CreemWebhookSecret:  "creem-wh-env",
	}
	svc.ApplyToConfig(cfg)

	checks := map[string]string{
		"EpayPartnerID":       cfg.EpayPartnerID,
		"EpayKey":             cfg.EpayKey,
		"EpayGatewayURL":      cfg.EpayGatewayURL,
		"EpayNotifyURL":       cfg.EpayNotifyURL,
		"StripeSecretKey":     cfg.StripeSecretKey,
		"StripeWebhookSecret": cfg.StripeWebhookSecret,
		"CreemAPIKey":         cfg.CreemAPIKey,
		"CreemWebhookSecret":  cfg.CreemWebhookSecret,
	}
	want := map[string]string{
		"EpayPartnerID":       "pid-db",
		"EpayKey":             "ekey-db",
		"EpayGatewayURL":      "https://gateway.db",
		"EpayNotifyURL":       "https://notify.db",
		"StripeSecretKey":     "stripe-sk-db",
		"StripeWebhookSecret": "stripe-wh-db",
		"CreemAPIKey":         "creem-key-db",
		"CreemWebhookSecret":  "creem-wh-db",
	}
	for field, got := range checks {
		if got != want[field] {
			t.Errorf("cfg.%s = %q, want %q", field, got, want[field])
		}
	}
}

func TestAdminConfig_Get_CacheHit(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{{Key: "stripe_secret_key", Value: "sk_from_cache"}},
	}
	svc := NewAdminConfigService(store)
	// Load populates cache
	_ = svc.Load(context.Background())

	val, err := svc.Get(context.Background(), "stripe_secret_key")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "sk_from_cache" {
		t.Errorf("Get = %q, want %q", val, "sk_from_cache")
	}
}

func TestAdminConfig_Get_CacheMiss_DBHit(t *testing.T) {
	store := &mockAdminSettingStore{
		settings: []entity.AdminSetting{{Key: "creem_api_key", Value: "ck_from_db"}},
	}
	// Do NOT call Load — force cache miss
	svc := NewAdminConfigService(store)

	val, err := svc.Get(context.Background(), "creem_api_key")
	if err != nil {
		t.Fatalf("Get (cache miss) error: %v", err)
	}
	if val != "ck_from_db" {
		t.Errorf("Get = %q, want %q", val, "ck_from_db")
	}
}

func TestAdminConfig_Get_CacheMiss_NotFound(t *testing.T) {
	store := &mockAdminSettingStore{} // empty store
	svc := NewAdminConfigService(store)

	val, err := svc.Get(context.Background(), "nonexistent_key")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "" {
		t.Errorf("Get for missing key = %q, want empty string", val)
	}
}
