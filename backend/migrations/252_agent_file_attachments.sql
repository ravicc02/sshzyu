-- Keep original display names independent of private storage keys.
ALTER TABLE agent_assets ADD COLUMN IF NOT EXISTS filename varchar(512) NOT NULL DEFAULT '';
