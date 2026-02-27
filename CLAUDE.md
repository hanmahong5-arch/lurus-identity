# lurus-identity

统一账号、产品订阅、钱包计费服务。所有 Lurus 产品的用户层。

## Commands

```bash
# Build
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o app ./cmd/server

# Test
go test -v ./...

# Run migrations (maintenance window)
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/001_identity_schema.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/002_billing_schema.sql
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrations/003_seed_products.sql

# Migrate from lurus-api (maintenance window only)
psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f scripts/migrate-from-lurus-api.sql

# Deploy
ssh root@100.98.57.55 "kubectl rollout restart deployment/lurus-identity -n lurus-identity"
```

## BMAD

| Resource | Path |
|----------|------|
| PRD | `./_bmad-output/planning-artifacts/prd.md` |
| Architecture | `./_bmad-output/planning-artifacts/architecture.md` |
| Epics | `./_bmad-output/planning-artifacts/epics.md` |
| Sprint Status | `./_bmad-output/planning-artifacts/sprint-status.yaml` |
| Dev Story DoD | `./_bmad-output/dev-story/checklist.md` |
| Code Review | `./_bmad-output/code-review/checklist.md` |
