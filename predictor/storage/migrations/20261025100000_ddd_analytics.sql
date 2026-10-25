-- Replace the initial draft analytics model with the domain model used by the ESB.
DROP TABLE IF EXISTS company_metric_predictions;
DROP TABLE IF EXISTS company_fact_matches;
DROP TABLE IF EXISTS facts;
DROP TABLE IF EXISTS events;

CREATE TABLE news
(
    news_id String,
    source_id String,
    source_url String,
    news_text String,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY (news_id);

CREATE TABLE facts
(
    fact_id UUID,
    news_id String,
    source_id String,
    source_url String,
    fact_text String,
    category LowCardinality(String),
    entities Array(String),
    evidence String,
    occurred_at Nullable(DateTime64(3, 'UTC')),
    confidence Float32,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY (news_id, fact_id);

CREATE TABLE fact_company
(
    company_id String,
    fact_id UUID,
    relevance String,
    confidence Float32,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY (company_id, fact_id);

CREATE TABLE metric_observations
(
    observation_id UUID,
    company_id String,
    metric_id String,
    news_id String,
    value Nullable(Float64),
    probability Float32,
    period_from Date,
    period_to Date,
    source_fact_ids Array(UUID),
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(inserted_at)
ORDER BY (company_id, metric_id, period_from, period_to, observation_id);

CREATE TABLE metric_totals
(
    company_id String,
    metric_id String,
    period_from Date,
    period_to Date,
    total Float64,
    observation_count UInt32,
    calculated_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(calculated_at)
ORDER BY (company_id, metric_id, period_from, period_to);

CREATE TABLE company_analyses
(
    analysis_id UUID,
    company_id String,
    period_from Date,
    period_to Date,
    probability Float32,
    metric_ids Array(String),
    analysis_text String,
    generated_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(generated_at)
ORDER BY (company_id, period_from, period_to, analysis_id);
