-- 264_account_scheduler_degradation.sql
-- Account-level degradation probes and per-group scheduling test settings.

CREATE TABLE IF NOT EXISTS account_scheduler_group_settings (
    group_id              BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    degradation_enabled   BOOLEAN NOT NULL DEFAULT false,
    interval_seconds      INT NOT NULL DEFAULT 600,
    test_model            VARCHAR(200) NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_scheduler_group_interval_check
        CHECK (interval_seconds BETWEEN 60 AND 86400)
);

CREATE TABLE IF NOT EXISTS account_degradation_probes (
    id              BIGSERIAL PRIMARY KEY,
    account_id      BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    group_id        BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    model           VARCHAR(200) NOT NULL DEFAULT '',
    status          VARCHAR(20) NOT NULL,
    response_text   TEXT NOT NULL DEFAULT '',
    error_message   TEXT NOT NULL DEFAULT '',
    latency_ms      BIGINT NOT NULL DEFAULT 0,
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_degradation_probe_status_check
        CHECK (status IN ('pass', 'degraded', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_account_degradation_probes_group_account_checked
    ON account_degradation_probes (group_id, account_id, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_account_degradation_probes_checked_at
    ON account_degradation_probes (checked_at DESC);

-- New scoring is the default routing policy. Administrators can still turn
-- the advanced scheduler off explicitly after this one-time upgrade.
INSERT INTO settings (key, value, updated_at)
VALUES ('openai_advanced_scheduler_enabled', 'true', NOW())
ON CONFLICT (key) DO UPDATE
SET value = 'true', updated_at = NOW()
WHERE settings.value IS DISTINCT FROM 'true';

INSERT INTO settings (key, value, updated_at)
VALUES ('openai_advanced_scheduler_weight_degradation', '1.5', NOW())
ON CONFLICT (key) DO NOTHING;
