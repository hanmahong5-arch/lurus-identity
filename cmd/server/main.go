package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	natsgo "github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	otelgin "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/hanmahong5-arch/lurus-identity/internal/adapter/handler"
	identitynats "github.com/hanmahong5-arch/lurus-identity/internal/adapter/nats"
	"github.com/hanmahong5-arch/lurus-identity/internal/adapter/handler/router"
	"github.com/hanmahong5-arch/lurus-identity/internal/adapter/payment"
	"github.com/hanmahong5-arch/lurus-identity/internal/adapter/repo"
	"github.com/hanmahong5-arch/lurus-identity/internal/app"
	"github.com/hanmahong5-arch/lurus-identity/internal/app/cron"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/auth"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/cache"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/config"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/email"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/idempotency"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/metrics"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/ratelimit"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/tracing"
	lurusweb "github.com/hanmahong5-arch/lurus-identity/web"
)

func main() {
	_ = godotenv.Load()

	// Config validates required env vars — panics if missing (fast-fail on startup)
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	// Use JSON log handler in production for structured log ingestion.
	if cfg.Env == "production" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("fatal error", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.Config) error {
	// --- OpenTelemetry tracing ---
	tracingShutdown, err := tracing.Init(ctx, cfg.OtelServiceName, cfg.OtelEndpoint)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tracingShutdown(shutCtx)
	}()

	// --- Database ---
	db, err := gorm.Open(postgres.Open(cfg.DatabaseDSN), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	defer sqlDB.Close()

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer rdb.Close()

	// --- NATS ---
	nc, err := natsgo.Connect(cfg.NATSAddr,
		natsgo.RetryOnFailedConnect(true),
		natsgo.MaxReconnects(10),
		natsgo.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer nc.Close()

	// --- Repositories ---
	accountRepo := repo.NewAccountRepo(db)
	subRepo := repo.NewSubscriptionRepo(db)
	walletRepo := repo.NewWalletRepo(db)
	productRepo := repo.NewProductRepo(db)
	vipRepo := repo.NewVIPRepo(db)
	invoiceRepo := repo.NewInvoiceRepo(db)
	refundRepo := repo.NewRefundRepo(db)

	// --- Cache ---
	entCache := cache.NewEntitlementCache(rdb, cfg.CacheEntitlementTTL)

	// --- App Services ---
	vipSvc := app.NewVIPService(vipRepo, walletRepo)
	walletSvc := app.NewWalletService(walletRepo, vipSvc)
	productSvc := app.NewProductService(productRepo)
	entSvc := app.NewEntitlementService(subRepo, productRepo, entCache)
	subSvc := app.NewSubscriptionService(subRepo, productRepo, entSvc, cfg.GracePeriodDays)
	accountSvc := app.NewAccountService(accountRepo, walletRepo, vipRepo)
	invoiceSvc := app.NewInvoiceService(invoiceRepo, walletRepo)
	referralSvc := app.NewReferralServiceWithCodes(accountRepo, walletRepo, walletRepo)

	// --- NATS Publisher ---
	publisher, err := identitynats.NewPublisher(nc)
	if err != nil {
		return fmt.Errorf("nats publisher: %w", err)
	}
	refundSvc := app.NewRefundService(refundRepo, walletRepo, publisher)

	// --- NATS Consumer ---
	consumer, err := identitynats.NewConsumer(nc, vipSvc)
	if err != nil {
		return fmt.Errorf("nats consumer: %w", err)
	}

	// --- Payment Providers ---
	epayProvider, err := payment.NewEpayProvider(cfg.EpayPartnerID, cfg.EpayKey, cfg.EpayGatewayURL, cfg.EpayNotifyURL)
	if err != nil {
		return fmt.Errorf("init epay provider: %w", err)
	}
	stripeProvider := payment.NewStripeProvider(cfg.StripeSecretKey, cfg.StripeWebhookSecret)
	creemProvider, err := payment.NewCreemProvider(cfg.CreemAPIKey, cfg.CreemWebhookSecret)
	if err != nil {
		return fmt.Errorf("init creem provider: %w", err)
	}

	// --- Auth Middleware (Zitadel JWKS JWT) ---
	jwtValidator := auth.NewValidator(auth.ValidatorConfig{
		Issuer:     cfg.ZitadelIssuer,
		Audience:   cfg.ZitadelAudience,
		JWKSURL:    cfg.ZitadelJWKSURL,
		JWKSTTL:    time.Hour,
		AdminRoles: []string{cfg.ZitadelAdminRole},
	})
	// AccountLookup: resolve Zitadel sub → lurus account_id (Redis sub-cache → DB fallback).
	accountLookup := buildAccountLookup(rdb, accountSvc)
	jwtMiddleware := auth.NewJWTMiddleware(jwtValidator, accountLookup)

	// --- Rate Limiter ---
	rateLimiter := ratelimit.New(rdb, ratelimit.DefaultConfig(
		cfg.RateLimitIPPerMinute,
		cfg.RateLimitUserPerMinute,
	))

	// --- Webhook Idempotency Deduper ---
	webhookDeduper := idempotency.New(rdb, 24*time.Hour)

	// --- HTTP Handlers ---
	accountH := handler.NewAccountHandler(accountSvc, vipSvc, subSvc)
	subH := handler.NewSubscriptionHandler(subSvc, productSvc, walletSvc, epayProvider, stripeProvider, creemProvider)
	walletH := handler.NewWalletHandler(walletSvc, epayProvider, stripeProvider, creemProvider)
	productH := handler.NewProductHandler(productSvc)
	internalH := handler.NewInternalHandler(accountSvc, subSvc, entSvc, vipSvc)
	webhookH := handler.NewWebhookHandler(walletSvc, subSvc, epayProvider, stripeProvider, creemProvider, webhookDeduper)
	invoiceH := handler.NewInvoiceHandler(invoiceSvc)
	refundH := handler.NewRefundHandler(refundSvc)
	adminOpsH := handler.NewAdminOpsHandler(referralSvc)
	reportH := handler.NewReportHandler(db)

	engine := router.Build(router.Deps{
		Accounts:      accountH,
		Subscriptions: subH,
		Wallets:       walletH,
		Products:      productH,
		Internal:      internalH,
		Webhooks:      webhookH,
		Invoices:      invoiceH,
		Refunds:       refundH,
		AdminOps:      adminOpsH,
		Reports:       reportH,
		InternalKey:   cfg.InternalAPIKey,
		JWT:           jwtMiddleware,
		RateLimit:     rateLimiter,
	})

	// Prometheus /metrics endpoint (unauthenticated, scraped internally by Prometheus).
	engine.GET("/metrics", gin.WrapH(metrics.Handler()))

	// Instrument all routes with Prometheus HTTP metrics and OTel traces.
	engine.Use(metrics.HTTPMiddleware())
	engine.Use(otelgin.Middleware(cfg.OtelServiceName))

	// --- SPA static files (web/dist embedded) ---
	webFS, err := fs.Sub(lurusweb.Dist, "dist")
	if err != nil {
		return fmt.Errorf("embed web/dist: %w", err)
	}
	engine.NoRoute(func(c *gin.Context) {
		// Serve static assets (JS/CSS/fonts) when the file exists in dist/.
		// Fall back to index.html for SPA client-side routes (/wallet, /callback, etc.).
		reqPath := strings.TrimPrefix(c.Request.URL.Path, "/")
		if reqPath != "" {
			if _, err := webFS.Open(reqPath); err == nil {
				http.FileServerFS(webFS).ServeHTTP(c.Writer, c.Request)
				return
			}
		}
		c.FileFromFS("index.html", http.FS(webFS))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      engine,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// --- Email Sender ---
	var emailSender email.Sender
	if cfg.EmailSMTPHost != "" {
		emailSender = email.NewSMTPSender(cfg.EmailSMTPHost, cfg.EmailSMTPPort, cfg.EmailSMTPUser, cfg.EmailSMTPPass, cfg.EmailFrom)
	} else {
		emailSender = email.NoopSender{}
	}

	// --- Cron Jobs ---
	expiryJob := cron.NewExpiryJob(subSvc, publisher, rdb)
	renewalJob := cron.NewRenewalJob(subSvc, subRepo, productRepo, walletSvc, publisher, rdb, time.Hour)
	notifJob := cron.NewNotificationJob(subRepo, accountRepo, emailSender, rdb, 24*time.Hour)

	g, gctx := errgroup.WithContext(ctx)

	// HTTP server
	g.Go(func() error {
		slog.Info("lurus-identity starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	})

	// NATS consumer
	g.Go(func() error {
		return consumer.Run(gctx)
	})

	// Subscription expiry cron
	g.Go(func() error {
		return expiryJob.Run(gctx)
	})

	// Subscription auto-renewal cron
	g.Go(func() error {
		return renewalJob.Run(gctx)
	})

	// Expiry notification email cron
	g.Go(func() error {
		return notifJob.Run(gctx)
	})

	// Graceful shutdown trigger
	g.Go(func() error {
		<-gctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutCtx)
	})

	return g.Wait()
}

// buildAccountLookup creates an AccountLookup function that caches Zitadel sub → account_id
// in Redis (TTL 10min) to avoid a DB round-trip on every authenticated request.
func buildAccountLookup(rdb *redis.Client, accountSvc *app.AccountService) auth.AccountLookup {
	const subCacheTTL = 10 * time.Minute

	return func(ctx context.Context, sub string) (int64, error) {
		key := "sub:id:" + sub

		// Fast path: Redis cache.
		val, err := rdb.Get(ctx, key).Int64()
		if err == nil {
			return val, nil
		}

		// Slow path: DB lookup.
		account, err := accountSvc.GetByZitadelSub(ctx, sub)
		if err != nil {
			return 0, fmt.Errorf("account lookup: %w", err)
		}
		if account == nil {
			return 0, fmt.Errorf("account not found for sub %q", sub)
		}

		// Cache the result.
		_ = rdb.Set(ctx, key, account.ID, subCacheTTL).Err()

		return account.ID, nil
	}
}
