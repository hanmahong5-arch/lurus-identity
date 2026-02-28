package payment

import (
	"testing"
)

// Epay tests are limited to constructor behavior because the epay.Client
// requires real gateway credentials for Purchase/Verify, and the library
// does not expose interfaces for mocking.

func TestNewEpayProvider_Disabled_EmptyPartnerID(t *testing.T) {
	p, err := NewEpayProvider("", "key", "https://epay.example.com", "https://notify.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil provider when partner ID empty")
	}
}

func TestNewEpayProvider_Disabled_EmptyKey(t *testing.T) {
	p, err := NewEpayProvider("12345", "", "https://epay.example.com", "https://notify.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil provider when key empty")
	}
}

func TestNewEpayProvider_Disabled_EmptyGateway(t *testing.T) {
	p, err := NewEpayProvider("12345", "key", "", "https://notify.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Error("expected nil provider when gateway URL empty")
	}
}

func TestNewEpayProvider_Valid(t *testing.T) {
	p, err := NewEpayProvider("12345", "testkey", "https://epay.example.com", "https://notify.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "epay_alipay" {
		t.Errorf("Name() = %q, want epay_alipay", p.Name())
	}
}
