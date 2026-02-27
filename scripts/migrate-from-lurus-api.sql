-- Migration script: lurus_api → identity + billing schemas
-- Run inside a maintenance window. Estimated time < 15min for < 10k users.
-- All steps are idempotent — safe to re-run after partial failure.
--
-- Usage:
--   psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrate-from-lurus-api.sql

BEGIN;

-- ─────────────────────────────────────────────────────────────
-- Step 1: Migrate accounts (lurus_api.users → identity.accounts)
-- ─────────────────────────────────────────────────────────────
INSERT INTO identity.accounts (
    lurus_id,
    zitadel_sub,
    display_name,
    avatar_url,
    email,
    email_verified,
    status,
    locale,
    aff_code,
    created_at,
    updated_at
)
SELECT
    'LU' || LPAD(u.id::text, 7, '0')  AS lurus_id,
    NULLIF(u.oidc_id, '')              AS zitadel_sub,
    COALESCE(NULLIF(u.display_name, ''), u.username, u.email) AS display_name,
    NULLIF(u.avatar_url, '')           AS avatar_url,
    u.email,
    true                               AS email_verified,
    CASE
        WHEN u.status = 1 THEN 1  -- enabled
        ELSE 2                    -- suspended/deleted
    END                                AS status,
    'zh-CN'                            AS locale,
    COALESCE(NULLIF(u.aff_code, ''), gen_random_uuid()::text)  AS aff_code,
    u.created_at,
    u.updated_at
FROM lurus_api.users u
WHERE u.deleted_at IS NULL
ON CONFLICT (email) DO NOTHING;

-- Back-fill lurus_id on newly inserted rows (lurus_id = 'LU' + padded numeric id)
UPDATE identity.accounts a
SET lurus_id = 'LU' || LPAD(
    (SELECT u.id::text FROM lurus_api.users u WHERE u.email = a.email LIMIT 1),
    7, '0'
)
WHERE a.lurus_id IS NULL OR a.lurus_id = '';

-- ─────────────────────────────────────────────────────────────
-- Step 2: Migrate OAuth bindings (github_id, discord_id, etc.)
-- ─────────────────────────────────────────────────────────────
INSERT INTO identity.account_oauth_bindings (account_id, provider, provider_id, provider_email)
SELECT a.id, 'github', u.github_id, u.email
FROM lurus_api.users u
JOIN identity.accounts a ON a.email = u.email
WHERE u.github_id IS NOT NULL AND u.github_id != ''
ON CONFLICT (provider, provider_id) DO NOTHING;

INSERT INTO identity.account_oauth_bindings (account_id, provider, provider_id, provider_email)
SELECT a.id, 'discord', u.discord_id::text, u.email
FROM lurus_api.users u
JOIN identity.accounts a ON a.email = u.email
WHERE u.discord_id IS NOT NULL AND u.discord_id != 0
ON CONFLICT (provider, provider_id) DO NOTHING;

INSERT INTO identity.account_oauth_bindings (account_id, provider, provider_id, provider_email)
SELECT a.id, 'linuxdo', u.linux_do_id::text, u.email
FROM lurus_api.users u
JOIN identity.accounts a ON a.email = u.email
WHERE u.linux_do_id IS NOT NULL AND u.linux_do_id != 0
ON CONFLICT (provider, provider_id) DO NOTHING;

INSERT INTO identity.account_oauth_bindings (account_id, provider, provider_id, provider_email)
SELECT a.id, 'telegram', u.telegram_id::text, u.email
FROM lurus_api.users u
JOIN identity.accounts a ON a.email = u.email
WHERE u.telegram_id IS NOT NULL AND u.telegram_id != 0
ON CONFLICT (provider, provider_id) DO NOTHING;

INSERT INTO identity.account_oauth_bindings (account_id, provider, provider_id, provider_email)
SELECT a.id, 'wechat', u.wechat_id, u.email
FROM lurus_api.users u
JOIN identity.accounts a ON a.email = u.email
WHERE u.wechat_id IS NOT NULL AND u.wechat_id != ''
ON CONFLICT (provider, provider_id) DO NOTHING;

-- ─────────────────────────────────────────────────────────────
-- Step 3: Migrate wallets (users.quota → billing.wallets)
-- Note: lurus_api quota is stored as tokens (not CNY).
--       Conversion: credits_cny = quota / 500000 (assuming 1 CNY = 500k tokens avg)
--       lifetime_topup is derived from sum of completed topup amounts.
-- ─────────────────────────────────────────────────────────────
INSERT INTO billing.wallets (account_id, balance, lifetime_topup, lifetime_spend)
SELECT
    a.id               AS account_id,
    GREATEST(0, COALESCE(u.quota::DECIMAL / 500000.0, 0))  AS balance,
    COALESCE(topups.total_topup, 0)                         AS lifetime_topup,
    0                                                        AS lifetime_spend
FROM lurus_api.users u
JOIN identity.accounts a ON a.email = u.email
LEFT JOIN (
    SELECT user_id, SUM(money) AS total_topup
    FROM lurus_api.topups
    WHERE status = 'completed'
    GROUP BY user_id
) topups ON topups.user_id = u.id
WHERE u.deleted_at IS NULL
ON CONFLICT (account_id) DO NOTHING;

-- ─────────────────────────────────────────────────────────────
-- Step 4: Migrate active subscriptions (lurus_api.subscriptions → identity.subscriptions)
-- ─────────────────────────────────────────────────────────────
INSERT INTO identity.subscriptions (
    account_id, product_id, plan_id, status,
    started_at, expires_at, auto_renew,
    payment_method, external_sub_id, created_at, updated_at
)
SELECT
    a.id                               AS account_id,
    'llm-api'                          AS product_id,
    pp.id                              AS plan_id,
    CASE s.status
        WHEN 'active'    THEN 'active'
        WHEN 'expired'   THEN 'expired'
        WHEN 'cancelled' THEN 'cancelled'
        ELSE 'expired'
    END                                AS status,
    s.started_at,
    s.expires_at,
    s.auto_renew,
    s.payment_method,
    s.payment_id                       AS external_sub_id,
    s.created_at,
    s.updated_at
FROM lurus_api.subscriptions s
JOIN lurus_api.users u ON u.id = s.user_id
JOIN identity.accounts a ON a.email = u.email
JOIN identity.product_plans pp
    ON pp.product_id = 'llm-api'
    AND pp.code = s.plan_code
WHERE u.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────
-- Step 5: Migrate topup/payment orders
-- ─────────────────────────────────────────────────────────────
INSERT INTO billing.payment_orders (
    account_id, order_no, order_type, product_id,
    amount_cny, currency, payment_method, status,
    external_id, paid_at, created_at, updated_at
)
SELECT
    a.id              AS account_id,
    COALESCE(NULLIF(t.trade_no, ''), 'MIGRATED-' || t.id::text)  AS order_no,
    'topup'           AS order_type,
    NULL              AS product_id,
    t.money           AS amount_cny,
    'CNY'             AS currency,
    t.channel         AS payment_method,
    CASE t.status
        WHEN 'completed' THEN 'paid'
        WHEN 'pending'   THEN 'cancelled'
        ELSE 'failed'
    END               AS status,
    NULLIF(t.trade_no, '')  AS external_id,
    CASE WHEN t.status = 'completed' THEN t.created_at END AS paid_at,
    t.created_at,
    t.updated_at
FROM lurus_api.topups t
JOIN lurus_api.users u ON u.id = t.user_id
JOIN identity.accounts a ON a.email = u.email
ON CONFLICT (order_no) DO NOTHING;

-- ─────────────────────────────────────────────────────────────
-- Step 6: Migrate redemption codes
-- ─────────────────────────────────────────────────────────────
INSERT INTO billing.redemption_codes (
    code, product_id, reward_type, reward_value,
    max_uses, used_count, expires_at, created_at
)
SELECT
    r.key              AS code,
    NULL               AS product_id,
    'credits'          AS reward_type,
    r.quota / 500000.0 AS reward_value,   -- convert token quota → credits
    1                  AS max_uses,
    CASE WHEN r.redeemed_user_ids IS NOT NULL AND r.redeemed_user_ids != '' THEN 1 ELSE 0 END AS used_count,
    NULL               AS expires_at,
    r.created_at
FROM lurus_api.redemptions r
ON CONFLICT (code) DO NOTHING;

-- ─────────────────────────────────────────────────────────────
-- Step 7: Initialise account_vip from lifetime_topup
-- ─────────────────────────────────────────────────────────────
INSERT INTO identity.account_vip (account_id, level, level_name, spend_grant)
SELECT
    w.account_id,
    COALESCE((
        SELECT MAX(vc.level)
        FROM identity.vip_level_configs vc
        WHERE vc.min_spend_cny <= w.lifetime_topup
    ), 0) AS level,
    COALESCE((
        SELECT vc.name
        FROM identity.vip_level_configs vc
        WHERE vc.min_spend_cny <= w.lifetime_topup
        ORDER BY vc.level DESC LIMIT 1
    ), 'Standard') AS level_name,
    COALESCE((
        SELECT MAX(vc.level)
        FROM identity.vip_level_configs vc
        WHERE vc.min_spend_cny <= w.lifetime_topup
    ), 0) AS spend_grant
FROM billing.wallets w
ON CONFLICT (account_id) DO NOTHING;

-- ─────────────────────────────────────────────────────────────
-- Step 8: Sync active subscription entitlements
-- ─────────────────────────────────────────────────────────────
INSERT INTO identity.account_entitlements (account_id, product_id, key, value, value_type, source, source_ref)
SELECT
    s.account_id,
    s.product_id,
    'plan_code',
    pp.code,
    'string',
    'subscription',
    s.id::text
FROM identity.subscriptions s
JOIN identity.product_plans pp ON pp.id = s.plan_id
WHERE s.status IN ('active', 'grace', 'trial')
ON CONFLICT (account_id, product_id, key) DO UPDATE
    SET value = EXCLUDED.value, source_ref = EXCLUDED.source_ref;

-- For free accounts (no active sub), seed plan_code=free
INSERT INTO identity.account_entitlements (account_id, product_id, key, value, value_type, source)
SELECT
    a.id,
    'llm-api',
    'plan_code',
    'free',
    'string',
    'system'
FROM identity.accounts a
WHERE NOT EXISTS (
    SELECT 1 FROM identity.account_entitlements e
    WHERE e.account_id = a.id AND e.product_id = 'llm-api' AND e.key = 'plan_code'
)
ON CONFLICT (account_id, product_id, key) DO NOTHING;

COMMIT;

-- Post-migration validation queries (run manually to verify):
-- SELECT COUNT(*) FROM identity.accounts;       -- should match lurus_api.users (non-deleted)
-- SELECT COUNT(*) FROM billing.wallets;
-- SELECT COUNT(*) FROM identity.subscriptions WHERE status='active';
-- SELECT COUNT(*) FROM identity.account_entitlements WHERE key='plan_code';
