-- Agent v1: additive, forward-only private workspace. No changes to existing bills.
CREATE TABLE IF NOT EXISTS agent_conversations (
 id varchar(64) PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id),
 config jsonb NOT NULL, title varchar(200) NOT NULL, version integer NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_conversations_user_time ON agent_conversations(user_id,updated_at DESC);
CREATE TABLE IF NOT EXISTS agent_runs (
 id varchar(64) PRIMARY KEY, conversation_id varchar(64) NOT NULL REFERENCES agent_conversations(id),
 user_id bigint NOT NULL REFERENCES users(id), idempotency_key varchar(255) NOT NULL,
 request_hash varchar(128) NOT NULL, snapshot jsonb NOT NULL,
 status varchar(32) NOT NULL, version bigint NOT NULL DEFAULT 0,
 owner varchar(64) NOT NULL DEFAULT '', fence_token bigint NOT NULL DEFAULT 0,
 lease_until timestamptz, cancel_requested boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,idempotency_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS agent_runs_one_active_user ON agent_runs(user_id)
 WHERE status NOT IN ('completed','partial_success','failed','cancelled','expired');
CREATE INDEX IF NOT EXISTS agent_runs_recovery ON agent_runs(status,lease_until);
CREATE TABLE IF NOT EXISTS agent_messages (
 id varchar(64) PRIMARY KEY, conversation_id varchar(64) NOT NULL REFERENCES agent_conversations(id),
 run_id varchar(64) REFERENCES agent_runs(id), role varchar(24) NOT NULL,
 content jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_messages_conversation_time ON agent_messages(conversation_id,created_at,id);
CREATE TABLE IF NOT EXISTS agent_operations (
 id varchar(96) PRIMARY KEY, run_id varchar(64) NOT NULL REFERENCES agent_runs(id),
 kind varchar(16) NOT NULL, step integer NOT NULL,
 request_hash varchar(128) NOT NULL, status varchar(32) NOT NULL DEFAULT 'prepared',
 quote numeric(20,8) NOT NULL CHECK(quote>=0), cost numeric(20,8),
 output jsonb, asset_id varchar(64), error_code varchar(128) NOT NULL DEFAULT '',
 billing_request_id varchar(128) NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(run_id,kind,step)
);
CREATE TABLE IF NOT EXISTS agent_tool_calls (
 id varchar(64) PRIMARY KEY, run_id varchar(64) NOT NULL REFERENCES agent_runs(id),
 model_operation_id varchar(96) NOT NULL REFERENCES agent_operations(id),
 provider_call_id varchar(128) NOT NULL, arguments_hash varchar(128) NOT NULL,
 arguments jsonb NOT NULL, result jsonb,
 UNIQUE(model_operation_id,provider_call_id)
);
CREATE TABLE IF NOT EXISTS agent_assets (
 id varchar(64) PRIMARY KEY, user_id bigint NOT NULL REFERENCES users(id),
 conversation_id varchar(64) NOT NULL REFERENCES agent_conversations(id),
 run_id varchar(64) REFERENCES agent_runs(id), operation_id varchar(96) UNIQUE REFERENCES agent_operations(id),
 storage_key varchar(160) NOT NULL, mime_type varchar(32) NOT NULL, byte_size bigint NOT NULL,
 width integer NOT NULL, height integer NOT NULL, sha256 varchar(64) NOT NULL,
 status varchar(24) NOT NULL DEFAULT 'ready', expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_assets_user_expiry ON agent_assets(user_id,expires_at);
CREATE TABLE IF NOT EXISTS agent_run_events (
 run_id varchar(64) NOT NULL REFERENCES agent_runs(id), sequence bigint NOT NULL,
 type varchar(64) NOT NULL, payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(run_id,sequence)
);
CREATE TABLE IF NOT EXISTS agent_task_outbox (
 run_id varchar(64) PRIMARY KEY REFERENCES agent_runs(id),
 ready_at timestamptz NOT NULL DEFAULT now(), completed boolean NOT NULL DEFAULT false,
 attempts integer NOT NULL DEFAULT 0
);
