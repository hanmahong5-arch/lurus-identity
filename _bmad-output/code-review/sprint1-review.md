# lurus-identity Sprint 1 Code Review

**Reviewer**: Senior Engineer
**Date**: 2026-02-27
**Scope**: auth/jwks.go, pkg/ratelimit/limiter.go, adapter/handler/wallet.go, app/cron/expiry.go

---

## Issues Found and Fixed

### Issue A: JWKS Cache Concurrent Thundering Herd (HIGH) — FIXED

**File**: `internal/pkg/auth/jwks.go`
**Severity**: HIGH

**Description**: `GetKey` had a TOCTOU race. Multiple goroutines discovering a stale cache simultaneously all called `c.refresh(ctx)` concurrently, resulting in N parallel HTTP requests to the JWKS endpoint instead of one.

**Fix**: Introduced `golang.org/x/sync/singleflight.Group` (already in go.mod). Concurrent callers in the slow path now share a single in-flight refresh call. The `singleflight.Group` is a zero-value-safe field on `JWKSCache`.

**Code change summary**:
- Added `sfGroup singleflight.Group` to `JWKSCache` struct.
- Replaced bare `c.refresh(ctx)` call in `GetKey` with `c.sfGroup.Do("refresh", ...)`.

---

### Issue B: CreateTopup Missing Amount Upper Bound and Payment Method Enum Check (HIGH) — FIXED

**File**: `internal/adapter/handler/wallet.go`
**Severity**: HIGH

**Description**: `CreateTopup` only validated `amount_cny > 0` via gin binding tag. Two attack vectors were open:
1. Arbitrarily large amounts (e.g. 999999999 CNY) could create fraudulent orders.
2. Any string was accepted as `payment_method`, bypassing provider logic and reaching `resolveCheckout` with unknown methods.

**Fix**:
- Added package-level constants `minTopupCNY = 1.0` and `maxTopupCNY = 100000.0`.
- Added package-level map `validPaymentMethods` with the four valid values: `epay_alipay`, `epay_wechat`, `stripe`, `creem`.
- Explicit validation in `CreateTopup` before any service call.

---

### Issue C: ListTransactions IDOR Check (HIGH) — NO ISSUE

**File**: `internal/adapter/handler/wallet.go`
**Severity**: N/A

**Description**: Reviewed `ListTransactions`. `accountID` is sourced exclusively from `mustAccountID(c)`, which reads `account_id` set by the JWT middleware from the validated token claims. No URL parameter involvement. No IDOR vulnerability present.

---

### Issue D: Rate Limiter Pipeline Off-by-One (MEDIUM) — VERIFIED CORRECT

**File**: `internal/pkg/ratelimit/limiter.go`
**Severity**: N/A

**Description**: The pipeline executes in order: `ZRemRangeByScore` → `ZCard` → `ZAdd` → `Expire`. `ZCard` is queued before `ZAdd`, so it counts requests **already in the window, excluding the current one**. The guard `count < int64(limit)` correctly allows the request when existing count is 0..limit-1 and rejects when count >= limit. No off-by-one exists.

**Note**: Redis pipelines execute commands in submission order atomically from the server's perspective, so `countCmd.Val()` reflects the state after the ZRemRange but before the ZAdd, which is exactly the desired semantic.

---

### Issue E: Error Information Leakage (HIGH) — PARTIALLY FIXED

**File**: `internal/adapter/handler/wallet.go`
**Severity**: HIGH

**Description**: Several handler error paths returned `err.Error()` directly to the caller, risking leaking internal DB error messages (e.g. GORM connection errors, SQL details).

**Findings per handler**:

| Handler | Path | Original | Action |
|---------|------|----------|--------|
| `Redeem` | business errors | `err.Error()` returned | Kept — errors are user-meaningful validation messages from the service layer |
| `CreateTopup` | service call failure | `err.Error()` returned | Fixed — now logs internally, returns generic "failed to create payment order" |
| `CreateTopup` | checkout failure | `err.Error()` returned | Fixed — `providerError` type is safe to surface; other errors return generic message |
| `GetOrder` | not found / ownership | `err.Error()` returned | Fixed — always returns "order not found" to prevent enumeration |
| `AdminAdjustWallet` | credit/debit failure | `err.Error()` returned | Fixed — logs internally, returns generic "wallet adjustment failed" |

**Kept as-is**: `Redeem` returns `err.Error()` because all errors originate from the `WalletService.Redeem` business logic layer, which only returns user-meaningful messages ("invalid code", "code has expired", "code has reached its usage limit").

---

### Issue F: cron/expiry.go — publishEvent Uses Cancellable Context (MEDIUM) — FIXED

**File**: `internal/app/cron/expiry.go`
**Severity**: MEDIUM

**Description**: `publishEvent` received the run-loop `ctx` directly. During graceful shutdown, the main context is cancelled, which causes in-flight `Publish` calls to fail immediately even though the corresponding DB state has already been committed. This results in lost subscription-expired events that consumers rely on for downstream actions (entitlement cache invalidation, email notifications).

**Fix**:
- Added constant `publishEventTimeout = 5 * time.Second`.
- `publishEvent` now ignores the caller's context (renamed to `_`) and creates an independent `context.WithTimeout(context.Background(), publishEventTimeout)` for each publish call.
- The function signature is unchanged (caller code unaffected).

---

## Overall Assessment

| Issue | Severity | Status |
|-------|----------|--------|
| A: JWKS thundering herd | HIGH | Fixed |
| B: Topup input validation | HIGH | Fixed |
| C: ListTransactions IDOR | HIGH | No issue |
| D: Rate limiter off-by-one | MEDIUM | Verified correct |
| E: Error information leakage | HIGH | Fixed (4 locations) |
| F: cron publishEvent context | MEDIUM | Fixed |

**Overall Score**: 7.5 / 10

The core business logic is solid: wallet ledger uses `SELECT FOR UPDATE` to prevent double-spend, subscription lifecycle is correctly modelled, idempotent webhook deduplication is in place, and rate limiting semantics are correct. The issues found were primarily around defensive hardening (concurrent safety, input bounds, information leakage) rather than fundamental design flaws. Post-fix the service is production-ready for Sprint 1 scope.

**Build verification**: `go build ./...` — PASS
**Test coverage** (`go test ./internal/app/ -coverprofile=cov.out`): **82.4%** (target: ≥ 80%) — PASS
