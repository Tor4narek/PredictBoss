CREATE TABLE IF NOT EXISTS agent_conversations (
    conversation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_type TEXT NOT NULL CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha')),
    scope_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_type, scope_id)
);

CREATE TABLE IF NOT EXISTS agent_calls (
    call_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES agent_conversations(conversation_id),
    agent_type TEXT NOT NULL CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha')),
    request_json JSONB NOT NULL,
    hermes_run_id TEXT UNIQUE,
    idempotency_key TEXT UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('queued', 'submitting', 'started', 'running', 'processing', 'completed', 'failed', 'cancelled', 'interrupted')),
    output TEXT,
    error TEXT,
    retry_count SMALLINT NOT NULL DEFAULT 0 CONSTRAINT agent_calls_retry_count_check CHECK (retry_count BETWEEN 0 AND 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS agent_calls_active_idx
    ON agent_calls (created_at)
    WHERE status IN ('queued', 'submitting', 'started', 'running', 'processing');

CREATE UNIQUE INDEX IF NOT EXISTS agent_calls_one_active_per_conversation_idx
    ON agent_calls (conversation_id)
    WHERE status IN ('submitting', 'started', 'running', 'processing');

CREATE TABLE IF NOT EXISTS agent_messages (
    message_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES agent_conversations(conversation_id),
    call_id UUID REFERENCES agent_calls(call_id),
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS agent_messages_conversation_idx
    ON agent_messages (conversation_id, message_id);
