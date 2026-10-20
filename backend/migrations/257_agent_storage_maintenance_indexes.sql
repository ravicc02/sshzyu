-- Bounded content maintenance; no deletion of user messages or financial evidence.
CREATE INDEX IF NOT EXISTS agent_assets_pending_expiry ON agent_assets(expires_at) WHERE status IN ('ready','deleted','expired','uploading');
CREATE INDEX IF NOT EXISTS agent_stream_events_expiry ON agent_run_events(created_at,run_id,sequence) WHERE type NOT IN ('run.completed','run.partial_success','run.failed','run.cancelled','run.expired');
