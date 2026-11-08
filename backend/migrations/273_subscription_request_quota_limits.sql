ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS daily_request_limit BIGINT NULL,
    ADD COLUMN IF NOT EXISTS weekly_request_limit BIGINT NULL,
    ADD COLUMN IF NOT EXISTS monthly_request_limit BIGINT NULL;

ALTER TABLE user_subscriptions
    ADD COLUMN IF NOT EXISTS daily_usage_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS weekly_usage_requests BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS monthly_usage_requests BIGINT NOT NULL DEFAULT 0;
