package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hanmahong5-arch/lurus-identity/internal/app"
)

// InternalHandler serves /internal/v1/* endpoints for service-to-service calls.
type InternalHandler struct {
	accounts     *app.AccountService
	subs         *app.SubscriptionService
	entitlements *app.EntitlementService
	vip          *app.VIPService
}

func NewInternalHandler(
	accounts *app.AccountService,
	subs *app.SubscriptionService,
	ents *app.EntitlementService,
	vip *app.VIPService,
) *InternalHandler {
	return &InternalHandler{accounts: accounts, subs: subs, entitlements: ents, vip: vip}
}

// GetAccountByZitadelSub looks up an account by Zitadel OIDC sub.
// GET /internal/v1/accounts/by-zitadel-sub/:sub
func (h *InternalHandler) GetAccountByZitadelSub(c *gin.Context) {
	sub := c.Param("sub")
	a, err := h.accounts.GetByZitadelSub(c.Request.Context(), sub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	if a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return
	}
	c.JSON(http.StatusOK, a)
}

// UpsertAccount creates or updates an account from a Zitadel webhook payload.
// POST /internal/v1/accounts/upsert
func (h *InternalHandler) UpsertAccount(c *gin.Context) {
	var req struct {
		ZitadelSub  string `json:"zitadel_sub"  binding:"required"`
		Email       string `json:"email"        binding:"required"`
		DisplayName string `json:"display_name"`
		AvatarURL   string `json:"avatar_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a, err := h.accounts.UpsertByZitadelSub(c.Request.Context(), req.ZitadelSub, req.Email, req.DisplayName, req.AvatarURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, a)
}

// GetEntitlements returns entitlements for an account+product (Redis-cached).
// GET /internal/v1/accounts/:id/entitlements/:product_id
func (h *InternalHandler) GetEntitlements(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	productID := c.Param("product_id")
	em, err := h.entitlements.Get(c.Request.Context(), id, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get entitlements"})
		return
	}
	if em == nil {
		em = map[string]string{"plan_code": "free"}
	}
	c.JSON(http.StatusOK, em)
}

// GetSubscription returns the active subscription for an account+product.
// GET /internal/v1/accounts/:id/subscription/:product_id
func (h *InternalHandler) GetSubscription(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	productID := c.Param("product_id")
	sub, err := h.subs.GetActive(c.Request.Context(), id, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "lookup failed"})
		return
	}
	if sub == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active subscription"})
		return
	}
	c.JSON(http.StatusOK, sub)
}

// ReportUsage receives LLM usage reports from lurus-api for VIP accumulation.
// POST /internal/v1/usage/report
func (h *InternalHandler) ReportUsage(c *gin.Context) {
	var req struct {
		AccountID int64   `json:"account_id" binding:"required"`
		AmountCNY float64 `json:"amount_cny" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = h.vip.RecalculateFromWallet(c.Request.Context(), req.AccountID)
	c.JSON(http.StatusOK, gin.H{"accepted": true})
}
