// Package config loads and validates all service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all service configuration.
type Config struct {
	// Server
	Port int
	Env  string

	// Database
	DatabaseDSN string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// NATS
	NATSAddr string

	// Auth — Zitadel JWT validation
	ZitadelIssuer   string // ZITADEL_ISSUER (e.g. https://auth.lurus.cn)
	ZitadelAudience string // ZITADEL_AUDIENCE (project ID)
	ZitadelJWKSURL  string // ZITADEL_JWKS_URL (e.g. https://auth.lurus.cn/oauth/v2/keys)
	ZitadelAdminRole string // ZITADEL_ADMIN_ROLE (default: admin)

	// Auth (internal service key for /internal/* routes)
	InternalAPIKey string

	// Rate limiting
	RateLimitIPPerMinute   int // RATE_LIMIT_IP_PER_MINUTE (default: 120)
	RateLimitUserPerMinute int // RATE_LIMIT_USER_PER_MINUTE (default: 300)

	// Subscription automation
	GracePeriodDays int // GRACE_PERIOD_DAYS (default: 3)

	// Payment providers
	StripeSecretKey       string
	StripeWebhookSecret   string
	StripeReturnURL       string
	EpayPartnerID         string
	EpayKey               string
	EpayGatewayURL        string
	EpayNotifyURL         string
	EpayReturnURL         string
	CreemAPIKey           string
	CreemWebhookSecret    string
	CreemReturnURL        string

	// Email (SMTP)
	EmailSMTPHost string // EMAIL_SMTP_HOST (empty = noop sender)
	EmailSMTPPort int    // EMAIL_SMTP_PORT (default: 587)
	EmailSMTPUser string // EMAIL_SMTP_USER
	EmailSMTPPass string // EMAIL_SMTP_PASS
	EmailFrom     string // EMAIL_FROM

	// Timeouts
	ShutdownTimeout     time.Duration
	CacheEntitlementTTL time.Duration

	// OpenTelemetry tracing
	OtelEndpoint    string // OTEL_EXPORTER_OTLP_ENDPOINT (empty = noop)
	OtelServiceName string // OTEL_SERVICE_NAME (default: lurus-identity)
}

// Load reads config from environment variables and validates required fields.
// Fails fast on startup if any required field is missing.
func Load() (*Config, error) {
	cfg := &Config{
		Port:                parseInt("PORT", 18104),
		Env:                 getEnv("ENV", "production"),
		DatabaseDSN:         requireEnv("DATABASE_DSN"),
		RedisAddr:           getEnv("REDIS_ADDR", "redis.messaging.svc:6379"),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisDB:             parseInt("REDIS_DB", 3),
		NATSAddr:            getEnv("NATS_ADDR", "nats://nats.messaging.svc:4222"),
		ZitadelIssuer:       requireEnv("ZITADEL_ISSUER"),
		ZitadelAudience:     getEnv("ZITADEL_AUDIENCE", ""),
		ZitadelJWKSURL:      requireEnv("ZITADEL_JWKS_URL"),
		ZitadelAdminRole:    getEnv("ZITADEL_ADMIN_ROLE", "admin"),
		InternalAPIKey:      requireEnv("INTERNAL_API_KEY"),
		RateLimitIPPerMinute:   parseInt("RATE_LIMIT_IP_PER_MINUTE", 120),
		RateLimitUserPerMinute: parseInt("RATE_LIMIT_USER_PER_MINUTE", 300),
		GracePeriodDays:     parseInt("GRACE_PERIOD_DAYS", 3),
		StripeSecretKey:     getEnv("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret: getEnv("STRIPE_WEBHOOK_SECRET", ""),
		StripeReturnURL:     getEnv("STRIPE_RETURN_URL", ""),
		EpayPartnerID:       getEnv("EPAY_PARTNER_ID", ""),
		EpayKey:             getEnv("EPAY_KEY", ""),
		EpayGatewayURL:      getEnv("EPAY_GATEWAY_URL", ""),
		EpayNotifyURL:       getEnv("EPAY_NOTIFY_URL", ""),
		EpayReturnURL:       getEnv("EPAY_RETURN_URL", ""),
		CreemAPIKey:         getEnv("CREEM_API_KEY", ""),
		CreemWebhookSecret:  getEnv("CREEM_WEBHOOK_SECRET", ""),
		CreemReturnURL:      getEnv("CREEM_RETURN_URL", ""),
		EmailSMTPHost:       getEnv("EMAIL_SMTP_HOST", ""),
		EmailSMTPPort:       parseInt("EMAIL_SMTP_PORT", 587),
		EmailSMTPUser:       getEnv("EMAIL_SMTP_USER", ""),
		EmailSMTPPass:       getEnv("EMAIL_SMTP_PASS", ""),
		EmailFrom:           getEnv("EMAIL_FROM", ""),
		ShutdownTimeout:     parseDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		CacheEntitlementTTL: parseDuration("CACHE_ENTITLEMENT_TTL", 5*time.Minute),
		OtelEndpoint:        getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OtelServiceName:     getEnv("OTEL_SERVICE_NAME", "lurus-identity"),
	}

	return cfg, nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %q is not set", key))
	}
	return v
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseInt(key string, defaultVal int) int {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

func parseDuration(key string, defaultVal time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}
