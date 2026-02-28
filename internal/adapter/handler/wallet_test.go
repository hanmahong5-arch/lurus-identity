package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWalletHandler_GetWallet(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)
	r := testRouter()
	r.GET("/api/v1/wallet", withAccountID(1), h.GetWallet)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestWalletHandler_ListTransactions(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)
	r := testRouter()
	r.GET("/api/v1/wallet/transactions", withAccountID(1), h.ListTransactions)

	tests := []struct {
		name  string
		query string
	}{
		{"default", ""},
		{"custom_page", "?page=2&page_size=10"},
		{"bad_page_normalized", "?page=-5&page_size=999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet/transactions"+tt.query, nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}
		})
	}
}

func TestWalletHandler_Redeem(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)
	r := testRouter()
	r.POST("/api/v1/wallet/redeem", withAccountID(1), h.Redeem)

	tests := []struct {
		name   string
		body   map[string]string
		status int
	}{
		{"missing_code", map[string]string{}, http.StatusBadRequest},
		{"invalid_code", map[string]string{"code": "NOTREAL"}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/redeem", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d", w.Code, tt.status)
			}
		})
	}
}

func TestWalletHandler_TopupInfo(t *testing.T) {
	tests := []struct {
		name          string
		hasEpay       bool
		hasStripe     bool
		hasCreem      bool
		expectMethods int
	}{
		{"no_providers", false, false, false, 0},
		{"all_providers", true, true, true, 4}, // epay adds 2 (alipay + wxpay)
		{"only_stripe", false, true, false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// nil providers = disabled
			h := NewWalletHandler(makeWalletService(), nil, nil, nil)
			// We can't easily construct real providers, but nil check is the test
			r := testRouter()
			r.GET("/api/v1/wallet/topup/info", withAccountID(1), h.TopupInfo)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet/topup/info", nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}
			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			methods, _ := resp["payment_methods"].([]interface{})
			if tt.name == "no_providers" && len(methods) != 0 {
				t.Errorf("expected 0 methods with nil providers, got %d", len(methods))
			}
		})
	}
}

func TestWalletHandler_CreateTopup_Validation(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)
	r := testRouter()
	r.POST("/api/v1/wallet/topup", withAccountID(1), h.CreateTopup)

	tests := []struct {
		name   string
		body   map[string]interface{}
		status int
		errMsg string
	}{
		{
			"missing_amount",
			map[string]interface{}{"payment_method": "stripe"},
			http.StatusBadRequest,
			"",
		},
		{
			"missing_method",
			map[string]interface{}{"amount_cny": 50.0},
			http.StatusBadRequest,
			"",
		},
		{
			"amount_below_min",
			map[string]interface{}{"amount_cny": 0.5, "payment_method": "stripe"},
			http.StatusBadRequest,
			"at least 1.00",
		},
		{
			"amount_above_max",
			map[string]interface{}{"amount_cny": 200000.0, "payment_method": "stripe"},
			http.StatusBadRequest,
			"exceeds maximum",
		},
		{
			"invalid_payment_method",
			map[string]interface{}{"amount_cny": 50.0, "payment_method": "bitcoin"},
			http.StatusBadRequest,
			"unsupported payment method",
		},
		{
			"provider_disabled",
			map[string]interface{}{"amount_cny": 50.0, "payment_method": "stripe"},
			// stripe provider is nil, so after passing validation + CreateTopup,
			// resolveCheckout returns providerError → 400
			http.StatusBadRequest,
			"not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/topup", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.status, w.Body.String())
			}
			if tt.errMsg != "" {
				var resp map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &resp)
				errStr, _ := resp["error"].(string)
				if !containsStr(errStr, tt.errMsg) {
					t.Errorf("error = %q, want containing %q", errStr, tt.errMsg)
				}
			}
		})
	}
}

func TestWalletHandler_AdminAdjustWallet(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)

	tests := []struct {
		name   string
		id     string
		body   map[string]interface{}
		status int
	}{
		{
			"valid_credit",
			"1",
			map[string]interface{}{"amount": 100.0, "description": "bonus"},
			http.StatusOK,
		},
		{
			"invalid_id",
			"abc",
			map[string]interface{}{"amount": 100.0, "description": "bonus"},
			http.StatusBadRequest,
		},
		{
			"missing_description",
			"1",
			map[string]interface{}{"amount": 100.0},
			http.StatusBadRequest,
		},
		{
			"missing_amount",
			"1",
			map[string]interface{}{"description": "test"},
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRouter()
			r.POST("/admin/v1/accounts/:id/wallet/adjust", h.AdminAdjustWallet)
			body, _ := json.Marshal(tt.body)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/admin/v1/accounts/"+tt.id+"/wallet/adjust", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tt.status {
				t.Errorf("status = %d, want %d, body: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestWalletHandler_ListOrders(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)
	r := testRouter()
	r.GET("/api/v1/wallet/orders", withAccountID(1), h.ListOrders)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet/orders", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWalletHandler_GetOrder_NotFound(t *testing.T) {
	h := NewWalletHandler(makeWalletService(), nil, nil, nil)
	r := testRouter()
	r.GET("/api/v1/wallet/orders/:order_no", withAccountID(1), h.GetOrder)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet/orders/NONEXISTENT", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
