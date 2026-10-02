ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS collection_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS image_size VARCHAR(16),
    ADD COLUMN IF NOT EXISTS aspect_ratio VARCHAR(16),
    ADD COLUMN IF NOT EXISTS response_mime_type VARCHAR(64);

CREATE INDEX IF NOT EXISTS batch_image_jobs_collection_id_idx
    ON batch_image_jobs (user_id, api_key_id, collection_id, created_at)
    WHERE collection_id IS NOT NULL AND collection_id <> '';

COMMENT ON COLUMN batch_image_jobs.collection_id IS
    'Client submission collection ID used to group one user task across multiple technical batches.';
COMMENT ON COLUMN batch_image_jobs.image_size IS 'Normalized image billing tier snapshot submitted for this batch.';
COMMENT ON COLUMN batch_image_jobs.aspect_ratio IS 'Normalized aspect ratio snapshot submitted for this batch.';
COMMENT ON COLUMN batch_image_jobs.response_mime_type IS 'Normalized response image MIME type submitted for this batch.';
