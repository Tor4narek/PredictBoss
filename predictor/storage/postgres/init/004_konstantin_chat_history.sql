CREATE INDEX IF NOT EXISTS konstantin_messages_company_history_idx
    ON konstantin_messages (company_id, created_at DESC, message_id DESC);
CREATE INDEX IF NOT EXISTS agent_calls_konstantin_question_history_idx
    ON agent_calls (conversation_id, created_at DESC, call_id DESC)
    WHERE agent_type = 'konstantin' AND request_json->>'request_kind' = 'question';
