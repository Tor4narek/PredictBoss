ALTER TABLE agent_conversations DROP CONSTRAINT IF EXISTS agent_conversations_agent_type_check;
ALTER TABLE agent_conversations ADD CONSTRAINT agent_conversations_agent_type_check
    CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha', 'konstantin'));
ALTER TABLE agent_calls DROP CONSTRAINT IF EXISTS agent_calls_agent_type_check;
ALTER TABLE agent_calls ADD CONSTRAINT agent_calls_agent_type_check
    CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha', 'konstantin'));
ALTER TABLE company_metric_settings ALTER COLUMN company_id TYPE UUID USING company_id::uuid;

CREATE TABLE IF NOT EXISTS company_profile_state (
    company_id UUID PRIMARY KEY,
    profile_revision BIGINT NOT NULL CHECK (profile_revision >= 0),
    profile_hash TEXT NOT NULL DEFAULT '',
    profile_request_hash TEXT NOT NULL DEFAULT '',
    profile_synced BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS company_news_sources (
    source_revision BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
    company_id UUID NOT NULL,
    source_id UUID NOT NULL UNIQUE,
    news_id TEXT NOT NULL,
    material_kind TEXT NOT NULL CHECK (material_kind IN ('external_news', 'alpha_analysis')),
    media_type TEXT NOT NULL CHECK (media_type IN ('application/pdf', 'text/html')),
    canonical_url TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS company_news_sources_company_revision_idx
    ON company_news_sources (company_id, source_revision DESC);

CREATE TABLE IF NOT EXISTS konstantin_messages (
    message_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL UNIQUE,
    call_id UUID NOT NULL UNIQUE REFERENCES agent_calls(call_id),
    company_id UUID NOT NULL,
    conversation_id UUID NOT NULL REFERENCES agent_conversations(conversation_id),
    analysis_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('analysis_result', 'assistant_answer')),
    text TEXT NOT NULL,
    evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    profile_revision BIGINT NOT NULL DEFAULT 0,
    source_revision BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS konstantin_messages_company_freshness_idx
    ON konstantin_messages (company_id, created_at DESC)
    WHERE kind = 'analysis_result';
CREATE INDEX IF NOT EXISTS konstantin_messages_conversation_idx
    ON konstantin_messages (conversation_id, created_at, message_id);
