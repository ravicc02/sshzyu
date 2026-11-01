-- Allow each OpenAI group to opt out of the degradation factor without
-- disabling degradation probes or their history.

ALTER TABLE account_scheduler_group_settings
    ADD COLUMN IF NOT EXISTS degradation_factor_enabled BOOLEAN NOT NULL DEFAULT true;
