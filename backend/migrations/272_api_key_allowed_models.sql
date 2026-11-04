-- Optional per-key restrictions, intersected with the owning group's catalog.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS allowed_models JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Configuration edits invalidate auth snapshots durably, including Redis outages.
-- Usage counters are intentionally excluded from the change predicate.
CREATE OR REPLACE FUNCTION enqueue_api_key_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM enqueue_auth_cache_invalidation(OLD.key);
        RETURN OLD;
    END IF;

    IF OLD.key IS DISTINCT FROM NEW.key
       OR OLD.status IS DISTINCT FROM NEW.status
       OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at
       OR OLD.user_id IS DISTINCT FROM NEW.user_id
       OR OLD.group_id IS DISTINCT FROM NEW.group_id
       OR OLD.ip_whitelist IS DISTINCT FROM NEW.ip_whitelist
       OR OLD.ip_blacklist IS DISTINCT FROM NEW.ip_blacklist
       OR OLD.expires_at IS DISTINCT FROM NEW.expires_at
       OR OLD.allowed_models IS DISTINCT FROM NEW.allowed_models
       OR OLD.quota IS DISTINCT FROM NEW.quota
       OR OLD.quota_tokens IS DISTINCT FROM NEW.quota_tokens
       OR OLD.rate_limit_5h IS DISTINCT FROM NEW.rate_limit_5h
       OR OLD.rate_limit_1d IS DISTINCT FROM NEW.rate_limit_1d
       OR OLD.rate_limit_7d IS DISTINCT FROM NEW.rate_limit_7d
       OR OLD.rate_limit_5h_tokens IS DISTINCT FROM NEW.rate_limit_5h_tokens
       OR OLD.rate_limit_1d_tokens IS DISTINCT FROM NEW.rate_limit_1d_tokens
       OR OLD.rate_limit_7d_tokens IS DISTINCT FROM NEW.rate_limit_7d_tokens THEN
        PERFORM enqueue_auth_cache_invalidation(OLD.key);
        IF NEW.deleted_at IS NULL AND NEW.key IS DISTINCT FROM OLD.key THEN
            PERFORM enqueue_auth_cache_invalidation(NEW.key);
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
