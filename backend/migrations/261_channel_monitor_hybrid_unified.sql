-- Unified Hybrid channel monitoring.
--
-- V2 passive aggregation and V1 active probes now run together. Per-monitor
-- switches control whether active probe history may be used as a no-traffic
-- fallback and whether the OpenAI reasoning probe runs.
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS probe_fallback_enabled BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS intelligence_enabled BOOLEAN NOT NULL DEFAULT TRUE;

COMMENT ON COLUMN channel_monitors.probe_fallback_enabled IS
    'Use active probe status as fallback when a linked group has no real V2 traffic';
COMMENT ON COLUMN channel_monitors.intelligence_enabled IS
    'Run the OpenAI reasoning/intelligence probe for this monitor';

UPDATE settings
SET value = 'hybrid'
WHERE key = 'channel_monitor_mode';

INSERT INTO settings (key, value)
VALUES ('channel_monitor_mode', 'hybrid')
ON CONFLICT (key) DO NOTHING;
