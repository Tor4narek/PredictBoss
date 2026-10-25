package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/predictboss/predictor/esb/internal/domain"
)

type MetricSettings struct{ pool *pgxpool.Pool }

func NewMetricSettings(pool *pgxpool.Pool) *MetricSettings {
	return &MetricSettings{pool: pool}
}

func (r *MetricSettings) UpsertCompanyMetrics(ctx context.Context, companyID string, metrics []domain.CompanyMetric) error {
	if len(metrics) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
(company_id, metric_id, name, description, metric_type, characteristic, unit, parameters)
VALUES ($1, $2, $3, $4, $5::company_metric_type, $6, $7, $8::jsonb)
ON CONFLICT (company_id, metric_id) DO UPDATE SET
name = EXCLUDED.name, description = EXCLUDED.description, metric_type = EXCLUDED.metric_type,
characteristic = EXCLUDED.characteristic, unit = EXCLUDED.unit, parameters = EXCLUDED.parameters,
updated_at = now()`, companyID, metric.ID, metric.Name, metric.Description, string(metric.Type), metric.Characteristic, metric.Unit, string(encoded))
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *MetricSettings) CompanyMetrics(ctx context.Context, companyID string, limit int) ([]domain.CompanyMetric, error) {
	rows, err := r.pool.Query(ctx, `SELECT metric_id, name, description, unit, metric_type::text,
characteristic, parameters FROM company_metric_settings WHERE company_id = $1
ORDER BY metric_id LIMIT $2`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	metrics := make([]domain.CompanyMetric, 0)
	for rows.Next() {
		var metric domain.CompanyMetric
		var metricType string
		var parameters []byte
		if err := rows.Scan(&metric.ID, &metric.Name, &metric.Description, &metric.Unit, &metricType, &metric.Characteristic, &parameters); err != nil {
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
