DO $$
BEGIN
    CREATE TYPE company_metric_type AS ENUM (
        'numeric', 'currency', 'percentage', 'count', 'ratio', 'score', 'text'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;

CREATE TABLE IF NOT EXISTS company_metric_settings (
    company_id TEXT NOT NULL,
    metric_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    metric_type company_metric_type NOT NULL,
    characteristic TEXT NOT NULL,
    unit TEXT NOT NULL DEFAULT '',
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (company_id, metric_id)
);
