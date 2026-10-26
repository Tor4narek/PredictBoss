CREATE TYPE company_metric_type AS ENUM (
    'numeric', 'currency', 'percentage', 'count', 'ratio', 'score', 'text'
);

CREATE TABLE agent_conversations (
    conversation_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_type TEXT NOT NULL CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha', 'konstantin')),
    scope_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (agent_type, scope_id)
);

CREATE TABLE agent_calls (
    call_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES agent_conversations(conversation_id),
    agent_type TEXT NOT NULL CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha', 'konstantin')),
    request_json JSONB NOT NULL,
    hermes_run_id TEXT,
    runtime_instance_id TEXT,
    execution_deadline_at TIMESTAMPTZ NOT NULL,
    launch_started_at TIMESTAMPTZ,
    cancel_requested_at TIMESTAMPTZ,
    stop_attempted_at TIMESTAMPTZ,
    stop_failed_at TIMESTAMPTZ,
    stop_reason TEXT CHECK (stop_reason IN ('user', 'timeout')),
    event_sequence BIGINT NOT NULL DEFAULT 0 CHECK (event_sequence >= 0),
    cancel_sequence BIGINT,
    stop_failure_sequence BIGINT,
    terminal_sequence BIGINT,
    CONSTRAINT agent_calls_runtime_run_id_unique
        UNIQUE (agent_type, runtime_instance_id, hermes_run_id),
    idempotency_key TEXT UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('queued', 'submitting', 'started', 'running', 'processing', 'completed', 'failed', 'cancelled', 'interrupted')),
    output TEXT,
    error TEXT,
    retry_count SMALLINT NOT NULL DEFAULT 0 CONSTRAINT agent_calls_retry_count_check CHECK (retry_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX agent_calls_active_idx
    ON agent_calls (created_at)
    WHERE status IN ('queued', 'submitting', 'started', 'running', 'processing');

CREATE INDEX agent_calls_runtime_active_idx
    ON agent_calls (agent_type, runtime_instance_id, status)
    WHERE runtime_instance_id IS NOT NULL
      AND status IN ('submitting', 'started', 'running', 'processing');

CREATE INDEX agent_calls_deadline_idx
    ON agent_calls (execution_deadline_at, created_at)
    WHERE status IN ('queued', 'submitting', 'started', 'running', 'processing');

CREATE UNIQUE INDEX agent_calls_one_active_per_conversation_idx
    ON agent_calls (conversation_id)
    WHERE status IN ('submitting', 'started', 'running', 'processing');

CREATE TABLE agent_messages (
    message_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id UUID NOT NULL REFERENCES agent_conversations(conversation_id),
    call_id UUID REFERENCES agent_calls(call_id),
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX agent_messages_conversation_idx
    ON agent_messages (conversation_id, message_id);

CREATE TABLE company_metric_settings (
	metric_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	company_id UUID NOT NULL,
	type_code TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    metric_type company_metric_type NOT NULL,
    characteristic TEXT NOT NULL,
    unit TEXT NOT NULL DEFAULT '',
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (company_id, type_code)
);

CREATE TABLE company_profile_state (
    company_id UUID PRIMARY KEY,
    profile_revision BIGINT NOT NULL CHECK (profile_revision >= 0),
    profile_hash TEXT NOT NULL DEFAULT '',
    profile_request_hash TEXT NOT NULL DEFAULT '',
    profile_synced BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE company_news_sources (
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

CREATE INDEX company_news_sources_company_revision_idx
    ON company_news_sources (company_id, source_revision DESC);

CREATE TABLE konstantin_call_sources (
    call_id UUID NOT NULL REFERENCES agent_calls(call_id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES company_news_sources(source_id),
    PRIMARY KEY (call_id, source_id)
);

CREATE TABLE konstantin_messages (
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

CREATE INDEX konstantin_messages_company_freshness_idx
    ON konstantin_messages (company_id, created_at DESC)
    WHERE kind = 'analysis_result';

CREATE INDEX konstantin_messages_conversation_idx
    ON konstantin_messages (conversation_id, created_at, message_id);

CREATE INDEX konstantin_messages_company_history_idx
    ON konstantin_messages (company_id, created_at DESC, message_id DESC);

CREATE INDEX agent_calls_konstantin_question_history_idx
    ON agent_calls (conversation_id, created_at DESC, call_id DESC)
    WHERE agent_type = 'konstantin' AND request_json->>'request_kind' = 'question';

CREATE TABLE agent_context_summaries (
    conversation_id UUID PRIMARY KEY REFERENCES agent_conversations(conversation_id),
    agent_type TEXT NOT NULL CHECK (agent_type IN ('serezha', 'saveliy', 'valera', 'kesha', 'konstantin')),
    summary TEXT NOT NULL,
    through_call_created_at TIMESTAMPTZ,
    through_call_id UUID REFERENCES agent_calls(call_id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((through_call_created_at IS NULL) = (through_call_id IS NULL))
);

CREATE INDEX agent_context_summaries_agent_idx
    ON agent_context_summaries (agent_type, updated_at);
