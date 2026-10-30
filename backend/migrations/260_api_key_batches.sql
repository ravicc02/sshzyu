-- Batch-generated API keys.
--
-- Batch keys use the normal authentication, quota, and billing path, but carry
-- a batch identifier so the main key page can exclude them and a dedicated page
-- can list, manage, copy, and export them independently.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS batch_id VARCHAR(64);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS batch_name VARCHAR(100) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_api_keys_user_batch
    ON api_keys(user_id, batch_id)
    WHERE batch_id IS NOT NULL AND deleted_at IS NULL;

COMMENT ON COLUMN api_keys.batch_id IS
    'Batch-generated API key group identifier; NULL for normal keys';
COMMENT ON COLUMN api_keys.batch_name IS
    'Display name shared by every key in the batch';
