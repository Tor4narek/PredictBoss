CREATE TABLE IF NOT EXISTS facts
(
    fact_id UUID,
    news_id String,
    source_url String,
    fact_text String,
    category LowCardinality(String),
    entities Array(String),
    evidence String,
    occurred_at Nullable(DateTime64(3, 'UTC')),
    confidence Float32,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree
ORDER BY (news_id, fact_id);

CREATE TABLE IF NOT EXISTS company_fact_matches
(
    company_id String,
    fact_id UUID,
    relevance String,
    confidence Float32,
    matched_at DateTime64(3, 'UTC') DEFAULT now64(3),
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree
ORDER BY (company_id, fact_id);

CREATE TABLE IF NOT EXISTS company_metric_predictions
(
    prediction_id UUID,
    company_id String,
    metric String,
    direction LowCardinality(String),
    horizon String,
    value Nullable(Float64),
    confidence Float32,
    rationale String,
    source_fact_ids Array(UUID),
    predicted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree
ORDER BY (company_id, metric, prediction_id);
