ALTER TABLE IF EXISTS redeem_codes
  ADD COLUMN IF NOT EXISTS max_uses INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS used_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS redeem_code_claims (
  id BIGSERIAL PRIMARY KEY,
  redeem_code_id BIGINT NOT NULL REFERENCES redeem_codes(id) ON DELETE RESTRICT,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  email TEXT NOT NULL,
  type VARCHAR(20) NOT NULL,
  value DECIMAL(20,8) NOT NULL,
  group_id BIGINT,
  validity_days INTEGER NOT NULL DEFAULT 30,
  claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT redeem_code_claims_code_user_unique UNIQUE (redeem_code_id, user_id),
  CONSTRAINT redeem_code_claims_code_email_unique UNIQUE (redeem_code_id, email)
);

CREATE INDEX IF NOT EXISTS redeem_code_claims_user_id_idx ON redeem_code_claims(user_id);
