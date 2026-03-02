package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/idempotency"
)

func makeWebhookHandler() *WebhookHandler {
	return NewWebhookHandler(
		makeWalletService(),
		makeSubService(),
		nil, nil, nil, // all payment providers nil
		idempotency.New(nil, 0), // nil redis → dedup is a no-op
	)
}

// ---------- EpayNotify ----------

func TestWebhookHandler_EpayNotify_NilProvider(t *testing.T) {
	h := makeWebhookHandler()
	r := testRouter()
	r.GET("/webhook/epay", h.EpayNotify)

	req := httptest.NewRequest(http.MethodGet, "/webhook/epay?trade_no=T001&out_trade_no=ORD001", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// epay is nil → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", w.Code, w.Body.String())
	}
}

// ---------- StripeWebhook ----------

func TestWebhookHandler_StripeWebhook_NilProvider(t *testing.T) {
	h := makeWebhookHandler()
	r := testRouter()
	r.POST("/webhook/stripe", h.StripeWebhook)

	req := httptest.NewRequest(http.MethodPost, "/webhook/stripe", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// stripe is nil → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", w.Code, w.Body.String())
	}
}

// ---------- CreemWebhook ----------

func TestWebhookHandler_CreemWebhook_NilProvider(t *testing.T) {
	h := makeWebhookHandler()
	r := testRouter()
	r.POST("/webhook/creem", h.CreemWebhook)

	req := httptest.NewRequest(http.MethodPost, "/webhook/creem", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// creem is nil → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", w.Code, w.Body.String())
	}
}

// ---------- processOrderPaid ----------

func TestWebhookHandler_ProcessOrderPaid_OrderNotFound(t *testing.T) {
	h := makeWebhookHandler()
	r := testRouter()

	// Wrap processOrderPaid in a handler for testing
	r.POST("/test/process", func(c *gin.Context) {
		var req struct {
			OrderNo string `json:"order_no"`
		}
		_ = c.ShouldBindJSON(&req)
		if err := h.processOrderPaid(c, req.OrderNo); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	body, _ := json.Marshal(map[string]string{"order_no": "NONEXISTENT"})
	req := httptest.NewRequest(http.MethodPost, "/test/process", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
	}
}
