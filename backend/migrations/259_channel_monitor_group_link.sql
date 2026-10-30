-- Durable group association for channel monitors.
--
-- Hybrid monitoring joins V1 active-probe results to V2 passive group rows by
-- ID. group_name remains for display and for legacy compatibility, but names
-- may be renamed and are not a safe join key.
ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id
    ON channel_monitors(group_id);

-- Backfill exact legacy name matches. Active group names are unique; soft-deleted
-- groups are intentionally excluded.
UPDATE channel_monitors AS cm
SET group_id = g.id
FROM groups AS g
WHERE cm.group_id IS NULL
  AND cm.group_name <> ''
  AND g.deleted_at IS NULL
  AND g.name = cm.group_name;

COMMENT ON COLUMN channel_monitors.group_id IS
    'Durable group link for hybrid monitor display; legacy group_name remains a display fallback';
