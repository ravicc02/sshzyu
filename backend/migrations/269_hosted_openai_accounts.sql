CREATE TABLE IF NOT EXISTS hosted_openai_accounts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    subscription_id BIGINT NULL,
    proxy_id BIGINT NULL,
    model VARCHAR(120) NOT NULL,
    supports_messages BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    account_email VARCHAR(255) NOT NULL DEFAULT '',
    plan_type VARCHAR(40) NOT NULL DEFAULT '',
    last_quota_refreshed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT hosted_openai_accounts_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT hosted_openai_accounts_account_id_fkey
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    CONSTRAINT hosted_openai_accounts_group_id_fkey
        FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
    CONSTRAINT hosted_openai_accounts_subscription_id_fkey
        FOREIGN KEY (subscription_id) REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    CONSTRAINT hosted_openai_accounts_proxy_id_fkey
        FOREIGN KEY (proxy_id) REFERENCES proxies(id) ON DELETE SET NULL,
    CONSTRAINT hosted_openai_accounts_user_account_unique UNIQUE (user_id, account_id),
    CONSTRAINT hosted_openai_accounts_group_unique UNIQUE (group_id)
);

CREATE INDEX IF NOT EXISTS idx_hosted_openai_accounts_user_status
    ON hosted_openai_accounts (user_id, status);

CREATE INDEX IF NOT EXISTS idx_hosted_openai_accounts_status_updated
    ON hosted_openai_accounts (status, updated_at DESC);
