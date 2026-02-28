package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

func TestInternalHandler_GetAccountByZitadelSub(t *testing.T) {
	as := newMockAccountStore()
	acct := as.seed(entity.Account{ZitadelSub: "sub-123", Email: "a@b.com", DisplayName: "Alice"})

	h := NewInternalHandler(
		makeAccountServiceWith(as),
		makeSubService(),
		makeEntitlementService(),
		makeVIPService(),
	)

	r := testRouter()
	r.GET("/internal/v1/accounts/by-zitadel-sub/:sub", h.GetAccountByZitadelSub)

	tests := []struct {
		name   string
		sub    string
		status int
	}{
		{"found", "sub-123", http.StatusOK},
		{"not_found", "no-such-sub", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/internal/v1/accounts/by-zitadel-sub/"+tt.sub, nil)
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
			if tt.status == http.StatusOK {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				if resp["email"] != acct.Email {
					t.Errorf("email = %v, want %s", resp["email"], acct.Email)
				}
			}
		})
	}
}

func TestInternalHandler_UpsertAccount(t *testing.T) {
	as := newMockAccountStore()
	h := NewInternalHandler(
		makeAccountServiceWith(as),
		makeSubService(),
		makeEntitlementService(),
		makeVIPService(),
	)

	r := testRouter()
	r.POST("/internal/v1/accounts/upsert", h.UpsertAccount)

	tests := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{
			"valid_new_account",
			map[string]string{"zitadel_sub": "new-sub", "email": "new@b.com", "display_name": "Bob"},
			http.StatusOK,
		},
		{
			"missing_sub",
			map[string]string{"email": "c@b.com"},
			http.StatusBadRequest,
		},
		{
			"missing_email",
			map[string]string{"zitadel_sub": "sub-x"},
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal/v1/accounts/upsert", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestInternalHandler_GetEntitlements(t *testing.T) {
	h := NewInternalHandler(
		makeAccountService(),
		makeSubService(),
		makeEntitlementService(),
		makeVIPService(),
	)

	r := testRouter()
	r.GET("/internal/v1/accounts/:id/entitlements/:product_id", h.GetEntitlements)

	tests := []struct {
		name      string
		path      string
		status    int
		checkFree bool // expect default {"plan_code":"free"}
	}{
		{"valid_id_no_sub", "/internal/v1/accounts/1/entitlements/lurus_api", http.StatusOK, true},
		{"invalid_id", "/internal/v1/accounts/abc/entitlements/lurus_api", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
			if tt.checkFree {
				var resp map[string]string
				json.Unmarshal(w.Body.Bytes(), &resp)
				if resp["plan_code"] != "free" {
					t.Errorf("plan_code = %q, want \"free\"", resp["plan_code"])
				}
			}
		})
	}
}

func TestInternalHandler_GetSubscription(t *testing.T) {
	h := NewInternalHandler(
		makeAccountService(),
		makeSubService(),
		makeEntitlementService(),
		makeVIPService(),
	)

	r := testRouter()
	r.GET("/internal/v1/accounts/:id/subscription/:product_id", h.GetSubscription)

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{"no_sub", "/internal/v1/accounts/1/subscription/lurus_api", http.StatusNotFound},
		{"bad_id", "/internal/v1/accounts/abc/subscription/lurus_api", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
		})
	}
}

func TestInternalHandler_ReportUsage(t *testing.T) {
	h := NewInternalHandler(
		makeAccountService(),
		makeSubService(),
		makeEntitlementService(),
		makeVIPService(),
	)

	r := testRouter()
	r.POST("/internal/v1/usage/report", h.ReportUsage)

	tests := []struct {
		name   string
		body   map[string]interface{}
		status int
	}{
		{"valid", map[string]interface{}{"account_id": 1, "amount_cny": 10.5}, http.StatusOK},
		{"missing_account", map[string]interface{}{"amount_cny": 10.5}, http.StatusBadRequest},
		{"missing_amount", map[string]interface{}{"account_id": 1}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/internal/v1/usage/report", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}
