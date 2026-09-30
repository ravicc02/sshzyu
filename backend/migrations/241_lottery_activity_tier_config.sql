-- 241_lottery_activity_tier_config.sql
--
-- Add activity-level tier configuration. Existing activities remain on the
-- default fixed schedule: $5, $55, $105, ... (incrementing by $50).
-- Custom mode stores strictly increasing unlock thresholds in cents.

ALTER TABLE lottery_activities
    ADD COLUMN IF NOT EXISTS tier_mode VARCHAR(20) NOT NULL DEFAULT 'fixed';

ALTER TABLE lottery_activities
    ADD COLUMN IF NOT EXISTS tier_thresholds JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN lottery_activities.tier_mode IS
    'Tier unlock mode: fixed ($5 then $50 increments) or custom (tier_thresholds).';

COMMENT ON COLUMN lottery_activities.tier_thresholds IS
    'Custom lottery unlock thresholds in cents, strictly increasing; used only when tier_mode=custom.';
