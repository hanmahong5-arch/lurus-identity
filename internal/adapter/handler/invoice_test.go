package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvoiceHandler_GenerateInvoice_MissingOrderNo(t *testing.T) {
	h := NewInvoiceHandler(makeInvoiceService())
	r := testRouter()
	r.POST("/api/v1/invoices", withAccountID(1), h.GenerateInvoice)

	body, _ := json.Marshal(map[string]string{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invoices", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestInvoiceHandler_ListInvoices(t *testing.T) {
	h := NewInvoiceHandler(makeInvoiceService())
	r := testRouter()
	r.GET("/api/v1/invoices", withAccountID(1), h.ListInvoices)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/invoices", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestInvoiceHandler_GetInvoice_NotFound(t *testing.T) {
	h := NewInvoiceHandler(makeInvoiceService())
	r := testRouter()
	r.GET("/api/v1/invoices/:invoice_no", withAccountID(1), h.GetInvoice)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/invoices/INV-NONE", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestInvoiceHandler_AdminList(t *testing.T) {
	h := NewInvoiceHandler(makeInvoiceService())
	r := testRouter()
	r.GET("/admin/v1/invoices", h.AdminList)

	tests := []struct {
		name  string
		query string
	}{
		{"no_filter", ""},
		{"with_account_filter", "?account_id=1"},
		{"pagination", "?page=2&page_size=5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/admin/v1/invoices"+tt.query, nil)
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}
		})
	}
}
