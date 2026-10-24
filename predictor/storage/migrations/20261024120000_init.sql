-- Initial append-only event stream for analytical workloads.
CREATE TABLE IF NOT EXISTS events
(
    event_id UUID,
    occurred_at DateTime64(3, 'UTC'),
    source LowCardinality(String),
    event_type LowCardinality(String),
    payload String,
    inserted_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = MergeTree
ORDER BY (occurred_at, event_id);
