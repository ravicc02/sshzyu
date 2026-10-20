-- Durable, owner-scoped human review of visual work. Execution uses agent runs/operations.
CREATE TABLE agent_work_plans (
 id text PRIMARY KEY,
 user_id bigint NOT NULL REFERENCES users(id),
 conversation_id text NOT NULL REFERENCES agent_conversations(id),
 source_run_id text NOT NULL REFERENCES agent_runs(id),
 revision integer NOT NULL DEFAULT 1,
 status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','ready','submitted','superseded')),
 snapshot jsonb NOT NULL,
 run_id text REFERENCES agent_runs(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX agent_work_plans_conversation ON agent_work_plans(user_id,conversation_id,created_at);
