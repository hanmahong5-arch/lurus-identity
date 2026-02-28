# lurus-identity

统一账号、产品订阅、钱包计费、VIP 会员服务。所有 Lurus 产品的用户层与计费核心。
Namespace: `lurus-identity` | Port: `18104` | DB schema: `identity`, `billing`

## Tech Stack

| Layer | Choice |
|-------|--------|
| Backend | Go 1.25, Gin, GORM (pgx driver) |
| Frontend | React 18 + TypeScript, Vite, Bun, Semi UI (embedded via `web/dist`) |
| DB | PostgreSQL (`search_path=identity,billing,public`), Redis DB 3 |
| Messaging | NATS JetStream (stream: `IDENTITY_EVENTS`) |
| Payment | Stripe, Creem, 易支付 (Epay) |
| Auth | Zitadel JWKS JWT; internal routes use bearer `INTERNAL_API_KEY` |
| Observability | Prometheus `/metrics`, OpenTelemetry (OTLP), `log/slog` JSON in production |

## Directory Structure

```
lurus-identity/
├── cmd/server/main.go          # Entry point, DI wiring
├── internal/
│   ├── domain/entity/          # Account, Subscription, Wallet, VIP, Product, Invoice, Refund
│   ├── app/                    # Use-case services (AccountSvc, SubSvc, WalletSvc, EntitlementSvc, ...)
│   │   └── cron/               # ExpiryJob, RenewalJob, NotificationJob
│   ├── adapter/
│   │   ├── handler/            # Gin handlers + router/router.go
│   │   ├── repo/               # GORM repositories
│   │   ├── nats/               # Publisher + Consumer
│   │   └── payment/            # Stripe, Creem, Epay providers
│   ├── lifecycle/              # Graceful shutdown helper
│   └── pkg/
│       ├── auth/               # JWKS validator + JWT middleware + AdminAuth
│       ├── cache/              # Redis entitlement cache (5min TTL)
│       ├── config/             # Env-var loader (fast-fail on missing required vars)
│       ├── email/              # SMTP sender + noop fallback
│       ├── idempotency/        # Webhook dedup (24h Redis key)
│       ├── metrics/            # Prometheus HTTP middleware
│       ├── ratelimit/          # IP + per-user rate limiter (Redis sliding window)
│       ├── tracing/            # OTel OTLP setup
│       └── audit/              # Structured audit log helpers
├── migrations/                 # SQL migrations (run in order, idempotent)
├── scripts/migrate-from-lurus-api.sql  # One-time data migration (maintenance only)
├── deploy/k8s/                 # Kustomize: deployment, configmap, secrets, hpa, servicemonitor
├── web/                        # React SPA (embedded in binary via web/embed.go)
└── _bmad-output/               # BMAD planning artifacts (do not edit manually)
```

## API Routes

| Group | Auth | Base Path |
|-------|------|-----------|
| Public user API | Zitadel JWT | `/api/v1/` |
| Internal service-to-service | Bearer `INTERNAL_API_KEY` | `/internal/v1/` |
| Admin | Zitadel JWT + `admin` role | `/admin/v1/` |
| Payment webhooks | Signature per provider | `/webhook/` |
| Health | None | `GET /health` |
| Metrics | None | `GET /metrics` |

Key internal endpoints (called by other services):
- `GET  /internal/v1/accounts/by-zitadel-sub/:sub`
- `POST /internal/v1/accounts/upsert`
- `GET  /internal/v1/accounts/:id/entitlements/:product_id`
- `POST /internal/v1/usage/report`

## Commands

```bash
# --- Backend ---
# Run locally (requires .env in service root)
go run ./cmd/server

# Build production binary
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o app ./cmd/server

# Test (all packages)
go test -v ./...

# Test with coverage
go test -coverprofile=cov.out ./... && go tool cover -html=cov.out

# --- Frontend (web/) ---
cd web && bun install
bun run dev        # dev server (Vite proxy to :18104)
bun run build      # outputs to web/dist (embedded in binary)

# --- Migrations (run in order; maintenance window) ---
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/001_identity_schema.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/002_billing_schema.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/003_seed_products.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/004_renewal_fields.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/005_invoice_refund.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/006_admin_settings.sql

# One-time lurus-api data migration (maintenance window only)
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f scripts/migrate-from-lurus-api.sql

# --- Deploy ---
ssh root@100.98.57.55 "kubectl rollout restart deployment/lurus-identity -n lurus-identity"
ssh root@100.98.57.55 "kubectl logs -f deployment/lurus-identity -n lurus-identity"
```

## Environment Variables

### Required (service will panic on startup if missing)

| Variable | Description |
|----------|-------------|
| `DATABASE_DSN` | PostgreSQL DSN (`search_path=identity,billing,public`) |
| `INTERNAL_API_KEY` | Bearer token for `/internal/v1/*` calls |
| `ZITADEL_ISSUER` | e.g. `https://auth.lurus.cn` |
| `ZITADEL_JWKS_URL` | e.g. `https://auth.lurus.cn/oauth/v2/keys` |

### Optional (defaults shown)

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `18104` | HTTP listen port |
| `ENV` | `production` | `production` = JSON log, `development` = text log |
| `REDIS_ADDR` | `redis.messaging.svc:6379` | Redis address |
| `REDIS_PASSWORD` | _(empty)_ | Redis password |
| `REDIS_DB` | `3` | Redis database number |
| `NATS_ADDR` | `nats://nats.messaging.svc:4222` | NATS server |
| `ZITADEL_AUDIENCE` | _(empty)_ | JWT audience (project ID) |
| `ZITADEL_ADMIN_ROLE` | `admin` | Role claim for admin access |
| `GRACE_PERIOD_DAYS` | `3` | Days before hard-expiry after subscription lapse |
| `CACHE_ENTITLEMENT_TTL` | `5m` | Redis TTL for entitlement cache |
| `RATE_LIMIT_IP_PER_MINUTE` | `120` | IP-level rate limit |
| `RATE_LIMIT_USER_PER_MINUTE` | `300` | Per-user rate limit |
| `SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown deadline |
| `STRIPE_SECRET_KEY` | _(empty)_ | Stripe payment |
| `STRIPE_WEBHOOK_SECRET` | _(empty)_ | Stripe webhook signature |
| `STRIPE_RETURN_URL` | _(empty)_ | Post-payment redirect |
| `EPAY_PARTNER_ID` | _(empty)_ | 易支付 partner ID |
| `EPAY_KEY` | _(empty)_ | 易支付 signing key |
| `EPAY_GATEWAY_URL` | _(empty)_ | 易支付 gateway URL |
| `EPAY_NOTIFY_URL` | _(empty)_ | 易支付 async callback URL |
| `EPAY_RETURN_URL` | _(empty)_ | 易支付 redirect URL |
| `CREEM_API_KEY` | _(empty)_ | Creem API key |
| `CREEM_WEBHOOK_SECRET` | _(empty)_ | Creem webhook secret |
| `CREEM_RETURN_URL` | _(empty)_ | Creem post-payment redirect |
| `WECHAT_SERVER_ADDRESS` | _(empty)_ | WeChat proxy base URL (empty = WeChat login disabled) |
| `WECHAT_SERVER_TOKEN` | _(empty)_ | Bearer token for WeChat proxy |
| `SESSION_SECRET` | _(empty)_ | HS256 secret for lurus session tokens (min 32 bytes; empty = disabled) |
| `EMAIL_SMTP_HOST` | _(empty)_ | SMTP host (empty = noop sender) |
| `EMAIL_SMTP_PORT` | `587` | SMTP port |
| `EMAIL_SMTP_USER` | _(empty)_ | SMTP username |
| `EMAIL_SMTP_PASS` | _(empty)_ | SMTP password |
| `EMAIL_FROM` | _(empty)_ | Sender address for notifications |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | _(empty)_ | OTel collector (empty = noop) |
| `OTEL_SERVICE_NAME` | `lurus-identity` | OTel service name |

## BMAD

| Resource | Path |
|----------|------|
| PRD | `./_bmad-output/planning-artifacts/prd.md` |
| Architecture | `./_bmad-output/planning-artifacts/architecture.md` |
| Epics | `./_bmad-output/planning-artifacts/epics.md` |
| Sprint Status | `./_bmad-output/planning-artifacts/sprint-status.yaml` |
| Dev Story DoD | `./_bmad-output/dev-story/checklist.md` |
| Code Review | `./_bmad-output/code-review/checklist.md` |
