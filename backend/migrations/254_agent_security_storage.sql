-- Add bounded private checkpoint accounting and distributed SSE leases.
CREATE TABLE IF NOT EXISTS agent_checkpoints (
 operation_id varchar(96) PRIMARY KEY REFERENCES agent_operations(id),
 user_id bigint NOT NULL REFERENCES users(id), storage_key varchar(160) NOT NULL UNIQUE,
 byte_size bigint NOT NULL DEFAULT 0 CHECK(byte_size>=0),
 reserved_bytes bigint NOT NULL DEFAULT 0 CHECK(reserved_bytes>=0),
 purged_at timestamptz, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_checkpoints_user ON agent_checkpoints(user_id) WHERE purged_at IS NULL;
INSERT INTO agent_checkpoints(operation_id,user_id,storage_key)
 SELECT o.id,r.user_id,o.id||'.result.json' FROM agent_operations o JOIN agent_runs r ON r.id=o.run_id
 ON CONFLICT(operation_id) DO NOTHING;
UPDATE agent_operations SET billing_request_id='agent:'||id WHERE billing_request_id='client:'||id;
CREATE TABLE IF NOT EXISTS agent_event_subscriptions (
 id varchar(64) PRIMARY KEY,user_id bigint NOT NULL REFERENCES users(id),
 run_id varchar(64) NOT NULL REFERENCES agent_runs(id),expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS agent_event_subscriptions_expiry ON agent_event_subscriptions(expires_at);
CREATE INDEX IF NOT EXISTS agent_event_subscriptions_user ON agent_event_subscriptions(user_id,expires_at);
