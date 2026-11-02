ALTER TABLE hosted_openai_accounts
    ADD COLUMN IF NOT EXISTS account_name VARCHAR(160) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS models JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE hosted_openai_accounts
SET
    account_name = CASE
        WHEN account_name = '' THEN COALESCE(NULLIF(account_email, ''), 'Hosted OpenAI Account')
        ELSE account_name
    END,
    models = CASE
        WHEN models = '[]'::jsonb AND model <> '' THEN jsonb_build_array(model)
        ELSE models
    END
WHERE account_name = '' OR models = '[]'::jsonb;
