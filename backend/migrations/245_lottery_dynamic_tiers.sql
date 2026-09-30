-- Dynamic lottery tier definitions. Existing activities keep their legacy fields
-- and are normalized by the service until an administrator saves custom tiers.
ALTER TABLE lottery_activities
    ADD COLUMN IF NOT EXISTS tier_definitions JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN lottery_activities.tier_definitions IS
    'Custom tier definitions: [{"name":"青铜","threshold_cents":0}, ...].';
