-- Content lifetime is independent of retained metadata and financial records.
ALTER TABLE agent_checkpoints ADD COLUMN content_created_at timestamptz;
ALTER TABLE agent_checkpoints ADD COLUMN content_expires_at timestamptz;
-- Legacy checkpoints lack first-response time: use the conservative request
-- admission time, never a later recovery/restart timestamp.
UPDATE agent_checkpoints cp SET content_created_at=o.created_at,
 content_expires_at=o.created_at+interval '24 hours'
 FROM agent_operations o WHERE o.id=cp.operation_id AND cp.byte_size>0;
UPDATE agent_assets a SET expires_at=least(a.expires_at,a.created_at+interval '24 hours',
 coalesce((SELECT cp.content_expires_at FROM agent_checkpoints cp WHERE cp.operation_id=a.operation_id),a.created_at+interval '24 hours'));
CREATE INDEX agent_checkpoints_content_expiry ON agent_checkpoints(content_expires_at) WHERE purged_at IS NULL;
-- A bounded hold permits only the already admitted job to finish using its exact inputs.
-- Public download and new tasks always respect the original expires_at.
CREATE TABLE agent_asset_holds (
 asset_id text NOT NULL REFERENCES agent_assets(id),
 run_id text NOT NULL REFERENCES agent_runs(id),
 until_at timestamptz NOT NULL,
 PRIMARY KEY(asset_id,run_id)
);
CREATE TABLE agent_draft_mutations (
 tool_id text PRIMARY KEY, arguments_hash text NOT NULL, user_id bigint NOT NULL REFERENCES users(id),
 plan_id text NOT NULL REFERENCES agent_work_plans(id), revision integer NOT NULL,
 snapshot jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE agent_execution_grants (
 id text PRIMARY KEY, request_hash text NOT NULL, user_id bigint NOT NULL REFERENCES users(id),
 plan_id text NOT NULL REFERENCES agent_work_plans(id),
 key_id bigint NOT NULL, model text NOT NULL, purpose text NOT NULL CHECK(purpose IN ('sample','output')),
 max_count integer NOT NULL CHECK(max_count BETWEEN 1 AND 32),
 max_cost numeric(20,8) NOT NULL CHECK(max_cost>0),
 used_count integer NOT NULL DEFAULT 0, reserved_cost numeric(20,8) NOT NULL DEFAULT 0,
 status text NOT NULL DEFAULT 'open' CHECK(status IN ('open','closed')),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX agent_execution_grants_plan ON agent_execution_grants(user_id,plan_id);
CREATE TABLE agent_grant_operations (
 operation_id text PRIMARY KEY REFERENCES agent_operations(id),
 grant_id text NOT NULL REFERENCES agent_execution_grants(id),
 reserved_cost numeric(20,8) NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);

-- A project budget survives model runs, grants and render chunks. Conservatively
-- charge shared decision calls to every project they mutate.
CREATE TABLE agent_project_operations (
 plan_id text NOT NULL REFERENCES agent_work_plans(id),
 operation_id text NOT NULL REFERENCES agent_operations(id),
 PRIMARY KEY(plan_id,operation_id)
);
UPDATE agent_work_plans p SET snapshot=jsonb_set(p.snapshot,'{task_budget_usd}',to_jsonb(c.config->>'default_max_cost_usd')) FROM agent_conversations c WHERE c.id=p.conversation_id;
