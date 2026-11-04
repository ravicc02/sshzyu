\set ON_ERROR_STOP on
-- Dedicated disposable benchmark database only. Does not connect to the app DB.
CREATE TABLE groups (id bigint PRIMARY KEY, name text, platform text, status text, model_allowlist jsonb, sort_order integer, deleted_at timestamptz);
CREATE TABLE usage_logs (id bigint PRIMARY KEY, group_id bigint, model text, actual_cost numeric(20,10), created_at timestamptz NOT NULL);
CREATE INDEX idx_usage_logs_created_at ON usage_logs(created_at);
CREATE INDEX idx_usage_logs_group_created_at_not_null ON usage_logs(group_id,created_at) WHERE group_id IS NOT NULL;
INSERT INTO groups SELECT i,'bench-'||i,'openai','active','{"models":[]}'::jsonb,i,NULL FROM generate_series(1,31)i;
INSERT INTO usage_logs SELECT i,1+((i*17)%31),'model-'||(i%11),0.001,now()-interval '30 days'+(i*interval '2.59 seconds') FROM generate_series(1,1000000)i;
ANALYZE groups; ANALYZE usage_logs;
SELECT COUNT(*) AS usage_rows FROM usage_logs;
