-- Additive, admin-only upstream account balance monitoring. No business-data rewrite.
CREATE TABLE IF NOT EXISTS upstream_balance_sites (
 id TEXT PRIMARY KEY, base_url TEXT NOT NULL UNIQUE, provider_type TEXT NOT NULL CHECK(provider_type IN ('sub2api','newapi')),
 account_id BIGINT NOT NULL REFERENCES accounts(id), display_name TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT TRUE, warning_enabled BOOLEAN NOT NULL DEFAULT FALSE, threshold NUMERIC(36,12), recharge_url TEXT NOT NULL,
 auth_cipher TEXT NOT NULL DEFAULT '', upstream_user_id TEXT NOT NULL DEFAULT '',
 amount NUMERIC(36,12), currency TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', error_code TEXT NOT NULL DEFAULT '',
 last_success_at TIMESTAMPTZ, last_attempt_at TIMESTAMPTZ, next_check_at TIMESTAMPTZ DEFAULT now(), failure_count INTEGER NOT NULL DEFAULT 0,
 alert_active BOOLEAN NOT NULL DEFAULT FALSE, episode_id TEXT NOT NULL DEFAULT '', recovery_count INTEGER NOT NULL DEFAULT 0,
 revision BIGINT NOT NULL DEFAULT 1, deleted_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS upstream_balance_due ON upstream_balance_sites(next_check_at) WHERE enabled AND deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS upstream_balance_events (
 id BIGSERIAL PRIMARY KEY, site_id TEXT REFERENCES upstream_balance_sites(id), kind TEXT NOT NULL, level TEXT NOT NULL DEFAULT 'info',
 message TEXT NOT NULL, details JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS upstream_balance_event_time ON upstream_balance_events(created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS upstream_balance_event_site ON upstream_balance_events(site_id,created_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS upstream_balance_deliveries (
 id TEXT PRIMARY KEY, site_id TEXT REFERENCES upstream_balance_sites(id), episode_id TEXT NOT NULL, site_revision BIGINT NOT NULL,
 title TEXT NOT NULL, amount TEXT NOT NULL, currency TEXT NOT NULL, recharge_url TEXT NOT NULL, is_test BOOLEAN NOT NULL DEFAULT FALSE,
 status TEXT NOT NULL DEFAULT 'pending', attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 lease_until TIMESTAMPTZ, message_id TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(site_id,episode_id)
);
CREATE INDEX IF NOT EXISTS upstream_balance_delivery_due ON upstream_balance_deliveries(next_attempt_at) WHERE status IN ('pending','retry','sending');
CREATE TABLE IF NOT EXISTS upstream_balance_channel (
 id INTEGER PRIMARY KEY CHECK(id=1), app_id TEXT NOT NULL, chat_id TEXT NOT NULL, secret_cipher TEXT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
