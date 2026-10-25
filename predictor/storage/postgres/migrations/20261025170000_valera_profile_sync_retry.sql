-- Persist the single automatic Hermes retry used when Valera completed without
-- confirming the requested profile revision in PostgreSQL and S3.
ALTER TABLE agent_calls
    ADD COLUMN IF NOT EXISTS retry_count SMALLINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'agent_calls_retry_count_check'
          AND conrelid = 'agent_calls'::regclass
    ) THEN
        ALTER TABLE agent_calls
            ADD CONSTRAINT agent_calls_retry_count_check CHECK (retry_count BETWEEN 0 AND 1);
    END IF;
END
$$;
