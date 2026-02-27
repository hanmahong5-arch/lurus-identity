// Package router wires all HTTP routes for lurus-identity.
package router

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/hanmahong5-arch/lurus-identity/internal/adapter/handler"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/auth"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/ratelimit"
)

// Deps holds all handler dependencies injected at startup.
type Deps struct {
	Accounts      *handler.AccountHandler
	Subscriptions *handler.SubscriptionHandler
	Wallets       *handler.WalletHandler
	Products      *handler.ProductHandler
	Internal      *handler.InternalHandler
	Webhooks      *handler.WebhookHandler
	Invoices      *handler.InvoiceHandler
	Refunds       *handler.RefundHandler
	AdminOps      *handler.AdminOpsHandler
	Reports       *handler.ReportHandler
	InternalKey   string // secret for /internal/* bearer auth
	JWT           *auth.JWTMiddleware
	RateLimit     *ratelimit.Limiter
}

// Build constructs and returns the root Gin engine.
func Build(deps Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	// Health check — unauthenticated
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "lurus-identity"})
	})

	// Public user API — requires Zitadel JWT
	v1 := r.Group("/api/v1")
	v1.Use(deps.JWT.Auth())
	if deps.RateLimit != nil {
		v1.Use(deps.RateLimit.PerUser())
	}
	{
		// Account
		v1.GET("/account/me", deps.Accounts.GetMe)
		v1.PUT("/account/me", deps.Accounts.UpdateMe)
		v1.GET("/account/me/services", deps.Accounts.GetServices)

		// Products (read-only, public)
		v1.GET("/products", deps.Products.ListProducts)
		v1.GET("/products/:id/plans", deps.Products.ListPlans)

		// Subscriptions
		v1.GET("/subscriptions", deps.Subscriptions.ListSubscriptions)
		v1.GET("/subscriptions/:product_id", deps.Subscriptions.GetSubscription)
		v1.POST("/subscriptions/checkout", deps.Subscriptions.Checkout)
		v1.POST("/subscriptions/:product_id/cancel", deps.Subscriptions.CancelSubscription)

		// Wallet
		v1.GET("/wallet", deps.Wallets.GetWallet)
		v1.GET("/wallet/transactions", deps.Wallets.ListTransactions)
		v1.POST("/wallet/redeem", deps.Wallets.Redeem)

		// Topup & Orders
		v1.GET("/wallet/topup/info", deps.Wallets.TopupInfo)
		v1.POST("/wallet/topup", deps.Wallets.CreateTopup)
		v1.GET("/wallet/orders", deps.Wallets.ListOrders)
		v1.GET("/wallet/orders/:order_no", deps.Wallets.GetOrder)

		// Invoices
		v1.POST("/invoices", deps.Invoices.GenerateInvoice)
		v1.GET("/invoices", deps.Invoices.ListInvoices)
		v1.GET("/invoices/:invoice_no", deps.Invoices.GetInvoice)

		// Refunds
		v1.POST("/refunds", deps.Refunds.RequestRefund)
		v1.GET("/refunds", deps.Refunds.ListRefunds)
		v1.GET("/refunds/:refund_no", deps.Refunds.GetRefund)
	}

	// Internal service-to-service API — bearer token auth
	internal := r.Group("/internal/v1")
	internal.Use(internalKeyAuth(deps.InternalKey))
	{
		internal.GET("/accounts/by-zitadel-sub/:sub", deps.Internal.GetAccountByZitadelSub)
		internal.POST("/accounts/upsert", deps.Internal.UpsertAccount)
		internal.GET("/accounts/:id/entitlements/:product_id", deps.Internal.GetEntitlements)
		internal.GET("/accounts/:id/subscription/:product_id", deps.Internal.GetSubscription)
		internal.POST("/usage/report", deps.Internal.ReportUsage)
	}

	// Admin API — requires admin JWT role
	admin := r.Group("/admin/v1")
	admin.Use(deps.JWT.AdminAuth())
	{
		admin.GET("/accounts", deps.Accounts.AdminListAccounts)
		admin.GET("/accounts/:id", deps.Accounts.AdminGetAccount)
		admin.POST("/accounts/:id/grant", deps.Accounts.AdminGrantEntitlement)
		admin.POST("/accounts/:id/wallet/adjust", deps.Wallets.AdminAdjustWallet)

		admin.POST("/products", deps.Products.AdminCreateProduct)
		admin.PUT("/products/:id", deps.Products.AdminUpdateProduct)
		admin.POST("/products/:id/plans", deps.Products.AdminCreatePlan)
		admin.PUT("/plans/:id", deps.Products.AdminUpdatePlan)

		// Admin Invoices
		admin.GET("/invoices", deps.Invoices.AdminList)

		// Admin Refunds
		admin.POST("/refunds/:refund_no/approve", deps.Refunds.AdminApprove)
		admin.POST("/refunds/:refund_no/reject", deps.Refunds.AdminReject)

		// Admin Ops: batch redemption code generation
		admin.POST("/redemption-codes/batch", deps.AdminOps.BatchGenerateCodes)

		// Admin Reports: financial reconciliation
		admin.GET("/reports/financial", deps.Reports.FinancialReport)
	}

	// Payment provider webhooks — signature-verified per-provider
	webhooks := r.Group("/webhook")
	if deps.RateLimit != nil {
		webhooks.Use(deps.RateLimit.PerIP())
	}
	{
		webhooks.GET("/epay", deps.Webhooks.EpayNotify)  // 易支付 uses GET callbacks
		webhooks.POST("/stripe", deps.Webhooks.StripeWebhook)
		webhooks.POST("/creem", deps.Webhooks.CreemWebhook)
	}

	return r
}

// internalKeyAuth validates the shared internal service API key.
func internalKeyAuth(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		bearer := c.GetHeader("Authorization")
		expected := "Bearer " + key
		if bearer != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid internal key"})
			return
		}
		c.Next()
	}
}
