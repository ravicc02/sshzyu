\set ON_ERROR_STOP on
CREATE TABLE usage_group_daily_rollups(bucket_date date NOT NULL,group_id bigint NOT NULL,actual_cost numeric(20,10) NOT NULL DEFAULT 0,computed_at timestamptz NOT NULL DEFAULT NOW(),PRIMARY KEY(bucket_date,group_id));
CREATE TABLE usage_group_rollup_state(id smallint PRIMARY KEY CHECK(id=1),closed_before date NOT NULL,retained_from timestamptz NOT NULL,timezone_name text NOT NULL,updated_at timestamptz NOT NULL DEFAULT NOW());
INSERT INTO usage_group_rollup_state SELECT 1,(MIN(created_at) AT TIME ZONE 'Asia/Shanghai')::date,MIN(created_at),'Asia/Shanghai',NOW() FROM usage_logs;
