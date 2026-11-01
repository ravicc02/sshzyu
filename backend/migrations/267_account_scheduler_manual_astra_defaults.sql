-- Degradation metrics are opt-in per group. The default test model is Astra.

ALTER TABLE account_scheduler_group_settings
    ALTER COLUMN degradation_factor_enabled SET DEFAULT false,
    ALTER COLUMN test_model SET DEFAULT 'gpt-6-astra';

UPDATE account_scheduler_group_settings
SET degradation_enabled = false,
    degradation_factor_enabled = false,
    test_reasoning_effort = CASE
        WHEN TRIM(test_reasoning_effort) = '' THEN 'medium'
        ELSE test_reasoning_effort
    END,
    test_model = CASE
        WHEN TRIM(test_model) IN ('', 'gpt-6-sol') THEN 'gpt-6-astra'
        ELSE test_model
    END,
    updated_at = NOW();
