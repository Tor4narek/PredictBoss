package clickhouse

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type cleanupIDRow struct {
	ID string `json:"id"`
}

type cleanupCompanyFactRow struct {
	CompanyID string `json:"company_id"`
	FactID    string `json:"fact_id"`
}

type cleanupObservationRow struct {
	MetricID      string `json:"metric_id"`
	ObservationID string `json:"observation_id"`
}

type cleanupMetricTotalRow struct {
	CompanyID  string `json:"company_id"`
	MetricID   string `json:"metric_id"`
	PeriodFrom string `json:"period_from_ms"`
	PeriodTo   string `json:"period_to_ms"`
}

type cleanupAnalysisRow struct {
	CompanyID  string `json:"company_id"`
	AnalysisID string `json:"analysis_id"`
}

// Prune applies each ClickHouse category's own retention policy. Every pass is
// bounded to one configured batch per category. Mutations wait for ClickHouse
// application and then read FINAL state to verify that no candidate remains.
func (r *Analytics) Prune(ctx context.Context, policy domain.AnalyticsRetention, now time.Time, terminalNewsIDs []string, batchSize int) (map[string]int, error) {
	if batchSize <= 0 {
		return nil, fmt.Errorf("cleanup batch size must be positive")
	}
	counts := make(map[string]int)
	steps := []struct {
		name   string
		policy domain.RetentionPolicy
		prune  func(time.Time) (int, error)
	}{
		{"processed_news_text", policy.ProcessedNewsText, func(cutoff time.Time) (int, error) { return r.pruneProcessedNewsText(ctx, cutoff, batchSize) }},
		{"unprocessed_news_text", policy.UnprocessedNewsText, func(cutoff time.Time) (int, error) {
			return r.pruneUnprocessedNewsText(ctx, cutoff, terminalNewsIDs, batchSize)
		}},
		{"fact_company", policy.FactCompany, func(cutoff time.Time) (int, error) { return r.pruneFactCompany(ctx, cutoff, batchSize) }},
		{"metric_observations", policy.MetricObservations, func(cutoff time.Time) (int, error) { return r.pruneMetricObservations(ctx, cutoff, batchSize) }},
		{"metric_totals", policy.MetricTotals, func(cutoff time.Time) (int, error) { return r.pruneMetricTotals(ctx, cutoff, batchSize) }},
		{"company_analyses", policy.CompanyAnalyses, func(cutoff time.Time) (int, error) { return r.pruneCompanyAnalyses(ctx, cutoff, batchSize) }},
		{"facts", policy.Facts, func(cutoff time.Time) (int, error) { return r.pruneFacts(ctx, cutoff, batchSize) }},
		{"news_metadata", policy.NewsMetadata, func(cutoff time.Time) (int, error) { return r.pruneNewsMetadata(ctx, cutoff, batchSize) }},
		{"news_marker", policy.NewsMarker, func(cutoff time.Time) (int, error) { return r.pruneNewsMarker(ctx, cutoff, batchSize) }},
	}
	for _, step := range steps {
		if step.policy.Never {
			continue
		}
		cutoff := now
		if !step.policy.Immediate {
			if step.policy.Age <= 0 {
				return counts, fmt.Errorf("%s retention must be positive, immediate, or never", step.name)
			}
			cutoff = now.Add(-step.policy.Age)
		}
		count, err := step.prune(cutoff)
		if err != nil {
			return counts, fmt.Errorf("cleanup %s: %w", step.name, err)
		}
		counts[step.name] = count
	}
	return counts, nil
}

func (r *Analytics) pruneProcessedNewsText(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	query := fmt.Sprintf(`SELECT c.news_id AS id
FROM (SELECT news_id, content_hash FROM %s.news_content FINAL) AS c
INNER JOIN (
    SELECT news_id, source_id, content_hash, fact_count, processed_at
    FROM %s.news_processing FINAL
) AS p USING (news_id)
INNER JOIN (
    SELECT news_id, source_id
    FROM %s.news FINAL
) AS n USING (news_id)
LEFT JOIN (
    SELECT news_id, count() AS fact_count
    FROM %s.facts FINAL
    GROUP BY news_id
) AS f USING (news_id)
WHERE c.content_hash = p.content_hash
  AND p.source_id = n.source_id
  AND ifNull(f.fact_count, 0) = p.fact_count
  AND p.processed_at <= %s
ORDER BY p.processed_at, c.news_id
LIMIT %d
FORMAT JSONEachRow`,
		r.table("news_content"),
		r.table("news_processing"),
		r.table("news"),
		r.table("facts"),
		chTime(cutoff),
		limit,
	)
	ids, err := r.selectCleanupIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.deleteStringKeys(ctx, "news_content", "news_id", ids)
}

func (r *Analytics) pruneUnprocessedNewsText(ctx context.Context, cutoff time.Time, terminalNewsIDs []string, limit int) (int, error) {
	if len(terminalNewsIDs) == 0 {
		return 0, nil
	}
	idList := sqlStringList(terminalNewsIDs)
	query := fmt.Sprintf(`SELECT c.news_id AS id
FROM (SELECT * FROM %s.news_content FINAL) AS c
LEFT JOIN (
    SELECT news_id
    FROM %s.news_processing FINAL
) AS p USING (news_id)
WHERE c.inserted_at <= %s
  AND p.news_id = ''
  AND c.news_id IN (%s)
ORDER BY c.inserted_at, c.news_id
LIMIT %d
FORMAT JSONEachRow`,
		r.table("news_content"),
		r.table("news_processing"),
		chTime(cutoff),
		idList,
		limit,
	)
	ids, err := r.selectCleanupIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.deleteStringKeys(ctx, "news_content", "news_id", ids)
}

func (r *Analytics) pruneNewsMetadata(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	query := fmt.Sprintf(`SELECT n.news_id AS id
FROM (SELECT * FROM %s.news FINAL) AS n
WHERE n.inserted_at <= %s
  AND n.news_id NOT IN (SELECT news_id FROM %s.news_content FINAL)
  AND n.news_id NOT IN (SELECT news_id FROM %s.facts FINAL)
  AND n.news_id NOT IN (SELECT news_id FROM %s.metric_observations FINAL)
ORDER BY n.inserted_at, n.news_id
LIMIT %d
FORMAT JSONEachRow`,
		r.table("news"),
		chTime(cutoff),
		r.table("news_content"),
		r.table("facts"),
		r.table("metric_observations"),
		limit,
	)
	ids, err := r.selectCleanupIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.deleteStringKeys(ctx, "news", "news_id", ids)
}

func (r *Analytics) pruneNewsMarker(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	query := fmt.Sprintf(`SELECT p.news_id AS id
FROM (SELECT * FROM %s.news_processing FINAL) AS p
WHERE p.processed_at <= %s
  AND p.news_id NOT IN (SELECT news_id FROM %s.news_content FINAL)
  AND p.news_id NOT IN (SELECT news_id FROM %s.facts FINAL)
  AND p.news_id NOT IN (SELECT news_id FROM %s.metric_observations FINAL)
ORDER BY p.processed_at, p.news_id
LIMIT %d
FORMAT JSONEachRow`,
		r.table("news_processing"),
		chTime(cutoff),
		r.table("news_content"),
		r.table("facts"),
		r.table("metric_observations"),
		limit,
	)
	ids, err := r.selectCleanupIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.deleteStringKeys(ctx, "news_processing", "news_id", ids)
}

func (r *Analytics) pruneFacts(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	query := fmt.Sprintf(`SELECT toString(f.fact_id) AS id FROM (SELECT * FROM %s.facts FINAL) f
WHERE f.inserted_at <= %s
 AND f.fact_id NOT IN (SELECT fact_id FROM %s.fact_company FINAL)
 AND f.fact_id NOT IN (SELECT arrayJoin(source_fact_ids) FROM %s.metric_observations FINAL)
ORDER BY f.inserted_at, f.fact_id LIMIT %d FORMAT JSONEachRow`,
		r.table("facts"), chTime(cutoff), r.table("fact_company"), r.table("metric_observations"), limit)
	ids, err := r.selectCleanupIDs(ctx, query)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return r.deleteStringKeys(ctx, "facts", "fact_id", ids)
}

func (r *Analytics) pruneFactCompany(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	var rows []cleanupCompanyFactRow
	query := fmt.Sprintf(`SELECT company_id, toString(fact_id) AS fact_id
FROM (SELECT * FROM %s.fact_company FINAL)
WHERE inserted_at <= %s
ORDER BY inserted_at, company_id, fact_id
LIMIT %d
FORMAT JSONEachRow`, r.table("fact_company"), chTime(cutoff), limit)
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, "("+escape(row.CompanyID)+", "+escape(row.FactID)+")")
	}
	return r.deleteTuples(ctx, "fact_company", "(company_id, toString(fact_id))", keys)
}

func (r *Analytics) pruneMetricObservations(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	var rows []cleanupObservationRow
	query := fmt.Sprintf(`SELECT metric_id, toString(observation_id) AS observation_id
FROM (SELECT * FROM %s.metric_observations FINAL)
WHERE inserted_at <= %s
ORDER BY inserted_at, metric_id, observation_id
LIMIT %d
FORMAT JSONEachRow`, r.table("metric_observations"), chTime(cutoff), limit)
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, "("+escape(row.MetricID)+", "+escape(row.ObservationID)+")")
	}
	return r.deleteTuples(ctx, "metric_observations", "(metric_id, toString(observation_id))", keys)
}

func (r *Analytics) pruneMetricTotals(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	var rows []cleanupMetricTotalRow
	query := fmt.Sprintf(`SELECT company_id, metric_id,
 toUnixTimestamp64Milli(period_from) AS period_from_ms, toUnixTimestamp64Milli(period_to) AS period_to_ms
FROM (SELECT * FROM %s.metric_totals FINAL)
WHERE generated_at <= %s
ORDER BY generated_at, company_id, metric_id, period_from, period_to
LIMIT %d
FORMAT JSONEachRow`, r.table("metric_totals"), chTime(cutoff), limit)
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		periodFrom, err := strconv.ParseInt(row.PeriodFrom, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse metric total start timestamp: %w", err)
		}
		periodTo, err := strconv.ParseInt(row.PeriodTo, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse metric total end timestamp: %w", err)
		}
		keys = append(keys, "("+escape(row.CompanyID)+", "+escape(row.MetricID)+", "+strconv.FormatInt(periodFrom, 10)+", "+strconv.FormatInt(periodTo, 10)+")")
	}
	return r.deleteTuples(ctx, "metric_totals", "(company_id, metric_id, toUnixTimestamp64Milli(period_from), toUnixTimestamp64Milli(period_to))", keys)
}

func (r *Analytics) pruneCompanyAnalyses(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	var rows []cleanupAnalysisRow
	query := fmt.Sprintf(`SELECT company_id, toString(analysis_id) AS analysis_id
FROM (SELECT * FROM %s.company_analyses FINAL)
WHERE generated_at <= %s
ORDER BY generated_at, company_id, analysis_id
LIMIT %d
FORMAT JSONEachRow`, r.table("company_analyses"), chTime(cutoff), limit)
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return 0, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, "("+escape(row.CompanyID)+", "+escape(row.AnalysisID)+")")
	}
	return r.deleteTuples(ctx, "company_analyses", "(company_id, toString(analysis_id))", keys)
}

func (r *Analytics) selectCleanupIDs(ctx context.Context, query string) ([]string, error) {
	var rows []cleanupIDRow
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

func (r *Analytics) deleteStringKeys(ctx context.Context, table, column string, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, escape(id))
	}
	return r.deleteAndVerify(ctx, table, column+" IN ("+strings.Join(values, ",")+")", len(ids))
}

func (r *Analytics) deleteTuples(ctx context.Context, table, columns string, tuples []string) (int, error) {
	if len(tuples) == 0 {
		return 0, nil
	}
	return r.deleteAndVerify(ctx, table, columns+" IN ("+strings.Join(tuples, ",")+")", len(tuples))
}

func (r *Analytics) deleteAndVerify(ctx context.Context, table, where string, count int) (int, error) {
	query := fmt.Sprintf(`ALTER TABLE %s.%s
DELETE WHERE %s
SETTINGS mutations_sync = 2`, r.table(table), table, where)
	if _, err := r.client.Query(ctx, query); err != nil {
		return 0, err
	}
	var remaining []countRow
	verify := fmt.Sprintf(`SELECT count() AS count
FROM (SELECT * FROM %s.%s FINAL)
WHERE %s
FORMAT JSONEachRow`, r.table(table), table, where)
	if err := r.client.queryRows(ctx, verify, &remaining); err != nil {
		return 0, err
	}
	if len(remaining) != 0 && remaining[0].Count != 0 {
		return 0, fmt.Errorf("ClickHouse mutation for %s is not fully applied", table)
	}
	return count, nil
}

func chTime(value time.Time) string {
	return "parseDateTime64BestEffort(" + escape(value.UTC().Format("2006-01-02 15:04:05.000")) + ", 3, 'UTC')"
}

func sqlStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, escape(value))
	}
	return strings.Join(quoted, ",")
}

var _ domain.AnalyticsCleanupRepository = (*Analytics)(nil)
