-- Supplementary migration: username + phone + checkin history backfill
-- Run after migrate-from-lurus-api.sql and after applying migrations 010 + 011.
-- All steps are idempotent — safe to re-run.
--
-- Usage:
--   psql $DATABASE_DSN -v ON_ERROR_STOP=1 -f migrate-supplementary.sql

BEGIN;

-- ─────────────────────────────────────────────────────────────
-- Step 1: Backfill username from lurus_api.users
-- ─────────────────────────────────────────────────────────────
UPDATE identity.accounts a
SET username = sub.username
FROM (
    SELECT u.username, u.email
    FROM lurus_api.users u
    WHERE u.deleted_at IS NULL
      AND u.username IS NOT NULL
      AND u.username != ''
) sub
WHERE a.email = sub.email
  AND (a.username IS NULL OR a.username = '');

-- ─────────────────────────────────────────────────────────────
-- Step 2: Backfill phone from lurus_api.users
-- ─────────────────────────────────────────────────────────────
UPDATE identity.accounts a
SET phone = sub.phone
FROM (
    SELECT u.phone, u.email
    FROM lurus_api.users u
    WHERE u.deleted_at IS NULL
      AND u.phone IS NOT NULL
      AND u.phone != ''
) sub
WHERE a.email = sub.email
  AND (a.phone IS NULL OR a.phone = '');

-- ─────────────────────────────────────────────────────────────
-- Step 3: Migrate checkin history
-- Conversion: lurus_api quota → credits (1 LB = 500,000 quota tokens)
-- ─────────────────────────────────────────────────────────────
INSERT INTO identity.checkins (account_id, checkin_date, reward_type, reward_value, created_at)
SELECT
    a.id                                    AS account_id,
    c.checkin_date,
    'credits'                               AS reward_type,
    GREATEST(0.01, c.quota_awarded::DECIMAL / 500000.0) AS reward_value,
    to_timestamp(c.created_at)              AS created_at
FROM lurus_api.checkins c
JOIN lurus_api.users u ON u.id = c.user_id
JOIN identity.accounts a ON a.email = u.email
WHERE u.deleted_at IS NULL
ON CONFLICT (account_id, checkin_date) DO NOTHING;

COMMIT;

-- Validation:
-- SELECT COUNT(*) FROM identity.accounts WHERE username IS NOT NULL;
-- SELECT COUNT(*) FROM identity.accounts WHERE phone IS NOT NULL AND phone != '';
-- SELECT COUNT(*) FROM identity.checkins;
