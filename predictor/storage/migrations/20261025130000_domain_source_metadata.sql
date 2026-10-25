-- Persist the source metadata resolved by ESB from Domain. The model sees
-- canonical source IDs and material kind for citations, while facts remain universal.
ALTER TABLE news ADD COLUMN IF NOT EXISTS company_id String DEFAULT '';
ALTER TABLE news ADD COLUMN IF NOT EXISTS material_kind LowCardinality(String) DEFAULT 'external_news';
ALTER TABLE news ADD COLUMN IF NOT EXISTS media_type LowCardinality(String) DEFAULT 'text/html';
ALTER TABLE news ADD COLUMN IF NOT EXISTS title String DEFAULT '';
ALTER TABLE news ADD COLUMN IF NOT EXISTS published_at Nullable(DateTime64(3, 'UTC'));
