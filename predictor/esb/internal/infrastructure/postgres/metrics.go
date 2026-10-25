package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/predictboss/predictor/esb/internal/domain"
)

type MetricSettings struct{ pool *pgxpool.Pool }

func NewMetricSettings(pool *pgxpool.Pool) *MetricSettings {
	return &MetricSettings{pool: pool}
}

func (r *MetricSettings) ReserveCompanyProfileRevision(ctx context.Context, companyID string, revision int64, requestHash string) error {
	if revision < 1 || len(requestHash) != 64 {
		return domain.ErrConflict
	}
	var accepted string
	err := r.pool.QueryRow(ctx, `INSERT INTO company_profile_state
(company_id, profile_revision, profile_hash, profile_request_hash, profile_synced)
VALUES ($1::uuid, $2, '', $3, FALSE)
ON CONFLICT (company_id) DO UPDATE SET
profile_revision = EXCLUDED.profile_revision,
profile_hash = CASE WHEN company_profile_state.profile_revision < EXCLUDED.profile_revision THEN '' ELSE company_profile_state.profile_hash END,
profile_request_hash = EXCLUDED.profile_request_hash,
profile_synced = CASE WHEN company_profile_state.profile_revision < EXCLUDED.profile_revision THEN FALSE ELSE company_profile_state.profile_synced END,
updated_at = now()
WHERE company_profile_state.profile_revision < EXCLUDED.profile_revision
   OR (company_profile_state.profile_revision = EXCLUDED.profile_revision AND company_profile_state.profile_request_hash = EXCLUDED.profile_request_hash)
RETURNING company_id::text`, companyID, revision, requestHash).Scan(&accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrConflict
	}
	return err
}

func (r *MetricSettings) UpsertCompanyMetrics(ctx context.Context, companyID string, metrics []domain.CompanyMetric) error {
	return r.upsertCompanyMetrics(ctx, companyID, nil, "", metrics)
}

func (r *MetricSettings) ReplaceCompanyMetricsAtRevision(ctx context.Context, companyID string, revision int64, profileHash string, metrics []domain.CompanyMetric) error {
	if revision < 1 || len(profileHash) != 64 {
		return domain.ErrConflict
	}
	return r.upsertCompanyMetrics(ctx, companyID, &revision, profileHash, metrics)
}

func (r *MetricSettings) upsertCompanyMetrics(ctx context.Context, companyID string, revision *int64, profileHash string, metrics []domain.CompanyMetric) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if revision != nil {
		var accepted string
		err := tx.QueryRow(ctx, `INSERT INTO company_profile_state (company_id, profile_revision, profile_hash, profile_request_hash, profile_synced)
VALUES ($1::uuid, $2, $3, '', FALSE)
ON CONFLICT (company_id) DO UPDATE SET profile_revision = EXCLUDED.profile_revision,
profile_hash = EXCLUDED.profile_hash,
profile_synced = CASE WHEN company_profile_state.profile_revision < EXCLUDED.profile_revision THEN FALSE ELSE company_profile_state.profile_synced END,
updated_at = now()
WHERE company_profile_state.profile_revision < EXCLUDED.profile_revision
   OR (company_profile_state.profile_revision = EXCLUDED.profile_revision AND
       (company_profile_state.profile_hash = EXCLUDED.profile_hash OR
        (company_profile_state.profile_hash = '' AND company_profile_state.profile_synced = FALSE AND company_profile_state.profile_request_hash <> '')))
RETURNING company_id::text`, companyID, *revision, profileHash).Scan(&accepted)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrConflict
			}
			return err
		}
		typeCodes := make([]string, 0, len(metrics))
		for _, metric := range metrics {
			typeCodes = append(typeCodes, metric.TypeCode)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM company_metric_settings
WHERE company_id = $1::uuid AND NOT (type_code = ANY($2::text[]))`, companyID, typeCodes); err != nil {
			return err
		}
	}
	for _, metric := range metrics {
		parameters := metric.Parameters
		if parameters == nil {
			parameters = map[string]any{}
		}
		encoded, err := json.Marshal(parameters)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO company_metric_settings
(company_id, type_code, name, description, metric_type, characteristic, unit, parameters)
VALUES ($1::uuid, $2, $3, $4, $5::company_metric_type, $6, $7, $8::jsonb)
ON CONFLICT (company_id, type_code) DO UPDATE SET
name = EXCLUDED.name, description = EXCLUDED.description, metric_type = EXCLUDED.metric_type,
characteristic = EXCLUDED.characteristic, unit = EXCLUDED.unit, parameters = EXCLUDED.parameters,
updated_at = now()`, companyID, metric.TypeCode, metric.Name, metric.Description, string(metric.Type), metric.Characteristic, metric.Unit, string(encoded))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *MetricSettings) MarkCompanyProfileSynced(ctx context.Context, companyID string, revision int64, profileHash string) error {
	result, err := r.pool.Exec(ctx, `UPDATE company_profile_state SET profile_synced = TRUE, updated_at = now()
WHERE company_id = $1::uuid AND profile_revision = $2 AND profile_hash = $3`, companyID, revision, profileHash)
	if err == nil && result.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return err
}

func (r *MetricSettings) CompanyMetrics(ctx context.Context, companyID string, limit int) ([]domain.CompanyMetric, error) {
	rows, err := r.pool.Query(ctx, `SELECT metric_id::text, type_code, name, description, unit, metric_type::text,
characteristic, parameters FROM company_metric_settings WHERE company_id = $1::uuid
AND COALESCE((SELECT profile_synced FROM company_profile_state WHERE company_id = $1::uuid), TRUE)
ORDER BY CASE type_code
    WHEN 'ставка' THEN 1 WHEN 'депозит' THEN 2 WHEN 'курс' THEN 3
    WHEN 'логистика' THEN 4 WHEN 'цена' THEN 5 WHEN 'спрос' THEN 6
    WHEN 'запас' THEN 7 WHEN 'контрагент' THEN 8 WHEN 'зарплата' THEN 9
    ELSE 10 END, type_code, metric_id
LIMIT $2`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metrics := make([]domain.CompanyMetric, 0)
	for rows.Next() {
		var metric domain.CompanyMetric
		var metricType string
		var parameters []byte
		if err := rows.Scan(&metric.ID, &metric.TypeCode, &metric.Name, &metric.Description, &metric.Unit, &metricType, &metric.Characteristic, &parameters); err != nil {
			return nil, err
		}
		metric.Type = domain.MetricType(metricType)
		if err := json.Unmarshal(parameters, &metric.Parameters); err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, rows.Err()
}

var _ domain.CompanyMetricSettingsRepository = (*MetricSettings)(nil)
