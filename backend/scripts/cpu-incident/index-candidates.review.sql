-- REVIEW DRAFT ONLY. No DROP statement is executable in this file.
-- Production idx_scan, sizes, reset age and plans have NOT been collected.
-- The owner must confirm every candidate separately using audit-indexes.sql.
-- Never include a primary/unique/constraint index in this procedure.
-- Run CONCURRENTLY outside a transaction; do not put this file in auto migrations.

-- Candidate: idx_usage_logs_user_id
-- Covered prefix: idx_usage_logs_user_created(user_id,created_at)
-- DROP INDEX CONCURRENTLY IF EXISTS public.idx_usage_logs_user_id;
-- Rollback: CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_user_id ON public.usage_logs(user_id);

-- Candidate: idx_usage_logs_api_key_id
-- Covered prefix: idx_usage_logs_api_key_created_at(api_key_id,created_at)
-- DROP INDEX CONCURRENTLY IF EXISTS public.idx_usage_logs_api_key_id;
-- Rollback: CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_api_key_id ON public.usage_logs(api_key_id);

-- Candidate: idx_usage_logs_account_id
-- Covered prefix: idx_usage_logs_account_created_at(account_id,created_at)
-- DROP INDEX CONCURRENTLY IF EXISTS public.idx_usage_logs_account_id;
-- Rollback: CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_account_id ON public.usage_logs(account_id);

-- Candidate: idx_usage_logs_model
-- Covered prefix: idx_usage_logs_model_created_at(model,created_at)
-- DROP INDEX CONCURRENTLY IF EXISTS public.idx_usage_logs_model;
-- Rollback: CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_model ON public.usage_logs(model);

-- Candidate: idx_usage_logs_subscription_id
-- Covered prefix: idx_usage_logs_sub_created(subscription_id,created_at)
-- DROP INDEX CONCURRENTLY IF EXISTS public.idx_usage_logs_subscription_id;
-- Rollback: CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_subscription_id ON public.usage_logs(subscription_id);

-- created_at alone is a prefix of BOTH created_* indexes, but its narrower
-- tuples may be cheaper. Retain until representative range plans are measured.
-- effective_* keys use COALESCE/NULLIF/BTRIM expressions and model-first order.
-- They DO NOT cover raw created_* (time-first) indexes or each other.
