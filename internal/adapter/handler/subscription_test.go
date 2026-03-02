package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hanmahong5-arch/lurus-identity/internal/app"
	"github.com/hanmahong5-arch/lurus-identity/internal/domain/entity"
)

func makeSubHandler() *SubscriptionHandler {
	return NewSubscriptionHandler(
		makeSubService(),
		makeProductService(),
		makeWalletService(),
		nil, nil, nil, // all payment providers nil
	)
}

// ---------- ListSubscriptions ----------

func TestSubHandler_ListSubscriptions_OK(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.GET("/api/v1/subscriptions", withAccountID(1), h.ListSubscriptions)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["subscriptions"]; !ok {
		t.Error("response missing 'subscriptions' key")
	}
}

func TestSubHandler_ListSubscriptions_Empty(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.GET("/api/v1/subscriptions", withAccountID(999), h.ListSubscriptions)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
}

// ---------- GetSubscription ----------

func TestSubHandler_GetSubscription_NotFound(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.GET("/api/v1/subscriptions/:product_id", withAccountID(1), h.GetSubscription)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/llm-api", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

// ---------- Checkout ----------

func TestSubHandler_Checkout_MissingBody(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.POST("/api/v1/subscriptions/checkout", withAccountID(1), h.Checkout)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/checkout", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestSubHandler_Checkout_PlanNotFound_Wallet(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.POST("/api/v1/subscriptions/checkout", withAccountID(1), h.Checkout)

	body, _ := json.Marshal(map[string]any{
		"product_id":     "llm-api",
		"plan_id":        999,
		"payment_method": "wallet",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/checkout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestSubHandler_Checkout_PlanNotFound_External(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.POST("/api/v1/subscriptions/checkout", withAccountID(1), h.Checkout)

	body, _ := json.Marshal(map[string]any{
		"product_id":     "llm-api",
		"plan_id":        999,
		"payment_method": "stripe",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/checkout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func makeSubHandlerWithPlan(plan *entity.ProductPlan) *SubscriptionHandler {
	ps := newMockPlanStore()
	ps.plans[plan.ID] = plan
	subSvc := app.NewSubscriptionService(newMockSubStore(), ps, makeEntitlementService(), 3)
	return NewSubscriptionHandler(subSvc, app.NewProductService(ps), makeWalletService(), nil, nil, nil)
}

func TestSubHandler_Checkout_WalletPayment_FreePlan(t *testing.T) {
	h := makeSubHandlerWithPlan(&entity.ProductPlan{ID: 1, ProductID: "llm-api", PriceCNY: 0})
	r := testRouter()
	r.POST("/api/v1/subscriptions/checkout", withAccountID(1), h.Checkout)

	body, _ := json.Marshal(map[string]any{
		"product_id":     "llm-api",
		"plan_id":        1,
		"payment_method": "wallet",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/checkout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["subscription"]; !ok {
		t.Error("response missing 'subscription' key")
	}
}

func TestSubHandler_Checkout_WalletPayment_InsufficientBalance(t *testing.T) {
	h := makeSubHandlerWithPlan(&entity.ProductPlan{ID: 1, ProductID: "llm-api", PriceCNY: 99.0})
	r := testRouter()
	r.POST("/api/v1/subscriptions/checkout", withAccountID(1), h.Checkout)

	body, _ := json.Marshal(map[string]any{
		"product_id":     "llm-api",
		"plan_id":        1,
		"payment_method": "wallet",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/checkout", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status=%d, want 402; body=%s", w.Code, w.Body.String())
	}
}

// ---------- CancelSubscription ----------

func TestSubHandler_CancelSubscription_NoActive(t *testing.T) {
	h := makeSubHandler()
	r := testRouter()
	r.POST("/api/v1/subscriptions/:product_id/cancel", withAccountID(1), h.CancelSubscription)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/llm-api/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Cancel on non-existent subscription → error from service
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", w.Code, w.Body.String())
	}
}
