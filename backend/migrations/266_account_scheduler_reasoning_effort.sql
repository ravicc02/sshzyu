-- Persist the reasoning effort used by account-scheduler probes.

ALTER TABLE account_scheduler_group_settings
    ADD COLUMN IF NOT EXISTS test_reasoning_effort VARCHAR(20) NOT NULL DEFAULT 'medium';

ALTER TABLE account_degradation_probes
    ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(20) NOT NULL DEFAULT '';
