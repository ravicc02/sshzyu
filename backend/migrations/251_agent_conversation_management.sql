-- Preserve messages, operations, assets and billing when hiding a conversation.
ALTER TABLE agent_conversations ADD COLUMN IF NOT EXISTS archived_at timestamptz;
ALTER TABLE agent_conversations ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS agent_conversations_visible ON agent_conversations(user_id,updated_at DESC) WHERE deleted_at IS NULL;
