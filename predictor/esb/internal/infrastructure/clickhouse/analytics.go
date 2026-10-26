package clickhouse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type Analytics struct {
	client                     *Client
	companyReportFactsLimit    int
	companyReportAnalysisLimit int
}

func NewAnalytics(client *Client, companyReportFactsLimit, companyReportAnalysisLimit int) *Analytics {
	return &Analytics{client: client, companyReportFactsLimit: companyReportFactsLimit, companyReportAnalysisLimit: companyReportAnalysisLimit}
}

type newsRow struct {
	ID           string  `json:"news_id"`
	SourceID     string  `json:"source_id"`
	SourceURL    string  `json:"source_url"`
	MaterialKind string  `json:"material_kind"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`
	PublishedAt  *string `json:"published_at"`
}

type newsContentRow struct {
	NewsID      string `json:"news_id"`
	ContentHash string `json:"content_hash"`
	Text        string `json:"news_text"`
}

type newsProcessingRow struct {
	NewsID      string `json:"news_id"`
	SourceID    string `json:"source_id"`
	ContentHash string `json:"content_hash"`
	FactCount   uint32 `json:"fact_count"`
}

type newsMetadataRow struct {
	ID           string `json:"news_id"`
	SourceID     string `json:"source_id"`
	SourceURL    string `json:"source_url"`
	MaterialKind string `json:"material_kind"`
	MediaType    string `json:"media_type"`
	Title        string `json:"title"`
	PublishedAt  string `json:"published_at"`
}

type countRow struct {
	Count uint64 `json:"count"`
}

type factRow struct {
	ID         string   `json:"fact_id"`
	NewsID     string   `json:"news_id"`
	SourceID   string   `json:"source_id"`
	SourceURL  string   `json:"source_url"`
	Text       string   `json:"fact_text"`
	Category   string   `json:"category"`
	Entities   []string `json:"entities"`
	Evidence   string   `json:"evidence"`
	OccurredAt *string  `json:"occurred_at"`
	Confidence float64  `json:"confidence"`
}

func (r *Analytics) SaveNewsAndFacts(ctx context.Context, news domain.News, facts []domain.Fact) ([]string, error) {
	if news.MaterialKind == "" {
		news.MaterialKind = "external_news"
	}
	if news.MediaType == "" {
		news.MediaType = "text/html"
	}
	contentHash := hashNewsText(news.Text)
	if previous, found, err := r.newsProcessing(ctx, news.ID); err != nil {
		return nil, err
	} else if found && (previous.SourceID != news.SourceID || previous.ContentHash != contentHash) {
		return nil, domain.ErrConflict
	}
	if err := r.client.Insert(ctx, "news", []newsRow{{ID: news.ID, SourceID: news.SourceID, SourceURL: news.SourceURL, MaterialKind: news.MaterialKind, MediaType: news.MediaType, Title: news.Title, PublishedAt: clickHouseTime(news.PublishedAt)}}); err != nil {
		return nil, err
	}
	if err := r.client.Insert(ctx, "news_content", []newsContentRow{{NewsID: news.ID, ContentHash: contentHash, Text: news.Text}}); err != nil {
		return nil, err
	}
	rows := make([]factRow, 0, len(facts))
	ids := make([]string, 0, len(facts))
	uniqueIDs := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		id := stableUUID(news.ID + "\x00" + fact.Text)
		if fact.Entities == nil {
			fact.Entities = []string{}
		}
		rows = append(rows, factRow{ID: id, NewsID: news.ID, SourceID: news.SourceID, SourceURL: news.SourceURL, Text: fact.Text, Category: fact.Category, Entities: fact.Entities, Evidence: fact.Evidence, OccurredAt: clickHouseTime(fact.OccurredAt), Confidence: fact.Confidence})
		ids = append(ids, id)
		uniqueIDs[id] = struct{}{}
	}
	if err := r.client.Insert(ctx, "facts", rows); err != nil {
		return nil, err
	}
	if err := r.client.Insert(ctx, "news_processing", []newsProcessingRow{{
		NewsID: news.ID, SourceID: news.SourceID, ContentHash: contentHash, FactCount: uint32(len(uniqueIDs)),
	}}); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Analytics) NewsReferences(ctx context.Context, newsIDs []string) ([]domain.NewsReference, error) {
	if len(newsIDs) == 0 {
		return []domain.NewsReference{}, nil
	}
	query := fmt.Sprintf(`SELECT news_id AS id, source_id, source_url, material_kind
FROM (SELECT * FROM %s.news FINAL)
WHERE news_id IN (%s)
FORMAT JSONEachRow`, r.table("news"), escapedList(newsIDs))
	var rows []domain.NewsReference
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Analytics) FactReferences(ctx context.Context, factIDs []string) ([]domain.FactReference, error) {
	if len(factIDs) == 0 {
		return []domain.FactReference{}, nil
	}
	query := fmt.Sprintf(`SELECT fact_id AS id, news_id, source_id, source_url
FROM (SELECT * FROM %s.facts FINAL)
WHERE fact_id IN (%s)
FORMAT JSONEachRow`, r.table("facts"), escapedList(factIDs))
	var rows []domain.FactReference
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Analytics) CompanyFactIDs(ctx context.Context, companyID string, factIDs []string) ([]string, error) {
	if len(factIDs) == 0 {
		return []string{}, nil
	}
	query := fmt.Sprintf(`SELECT fact_id
FROM (SELECT * FROM %s.fact_company FINAL)
WHERE company_id = %s AND fact_id IN (%s)
FORMAT JSONEachRow`, r.table("fact_company"), escape(companyID), escapedList(factIDs))
	var rows []struct {
		FactID string `json:"fact_id"`
	}
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.FactID)
	}
	return ids, nil
}

func (r *Analytics) CompanyNewsIDs(ctx context.Context, companyID string, newsIDs []string) ([]string, error) {
	if len(newsIDs) == 0 {
		return []string{}, nil
	}
	query := fmt.Sprintf(`SELECT DISTINCT f.news_id
FROM (SELECT * FROM %s.fact_company FINAL) AS links
INNER JOIN (SELECT * FROM %s.facts FINAL) AS f ON links.fact_id = f.fact_id
WHERE links.company_id = %s AND f.news_id IN (%s)
FORMAT JSONEachRow`, r.table("fact_company"), r.table("facts"), escape(companyID), escapedList(newsIDs))
	var rows []struct {
		NewsID string `json:"news_id"`
	}
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.NewsID)
	}
	return ids, nil
}

func escapedList(values []string) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = escape(value)
	}
	return strings.Join(parts, ",")
}

// FinalizeNews verifies the completion marker, metadata, fact rows, and raw
// content hash before allowing the PostgreSQL call to complete. Retention is a
// separate operation, so text remains available until its configured age.
func (r *Analytics) FinalizeNews(ctx context.Context, expected domain.News, contentHash string) error {
	processed, found, err := r.newsProcessing(ctx, expected.ID)
	if err != nil {
		return err
	}
	if !found || processed.SourceID != expected.SourceID || processed.ContentHash != contentHash {
		return fmt.Errorf("news processing is not confirmed")
	}
	metadata, found, err := r.newsMetadata(ctx, expected.ID)
	if err != nil {
		return err
	}
	if !found || !metadataMatches(metadata, expected) {
		return fmt.Errorf("news metadata is not confirmed")
	}
	var factCounts []countRow
	query := fmt.Sprintf(`SELECT toUInt32(count()) AS count
FROM (
    SELECT *
    FROM %s.facts FINAL
    WHERE news_id = %s
)
FORMAT JSONEachRow`, r.table("facts"), escape(expected.ID))
	if err := r.client.queryRows(ctx, query, &factCounts); err != nil {
		return err
	}
	var factCount uint64
	if len(factCounts) > 0 {
		factCount = factCounts[0].Count
	}
	if factCount != uint64(processed.FactCount) {
		return fmt.Errorf("news facts are not confirmed")
	}
	var contentCounts []countRow
	contentQuery := fmt.Sprintf(`SELECT toUInt32(count()) AS count
FROM (
    SELECT *
    FROM %s.news_content FINAL
    WHERE news_id = %s
      AND content_hash = %s
)
FORMAT JSONEachRow`, r.table("news_content"), escape(expected.ID), escape(contentHash))
	if err := r.client.queryRows(ctx, contentQuery, &contentCounts); err != nil {
		return err
	}
	if len(contentCounts) == 0 || contentCounts[0].Count == 0 {
		var remaining []countRow
		remainingQuery := fmt.Sprintf(`SELECT toUInt32(count()) AS count
FROM (
    SELECT *
    FROM %s.news_content FINAL
    WHERE news_id = %s
)
FORMAT JSONEachRow`, r.table("news_content"), escape(expected.ID))
		if err := r.client.queryRows(ctx, remainingQuery, &remaining); err != nil {
			return err
		}
		if len(remaining) > 0 && remaining[0].Count > 0 {
			return fmt.Errorf("raw news text does not match the confirmed content hash")
		}
		return nil // A previous retention pass removed text after this marker was confirmed.
	}
	// The finalizer proves persistence and returns the hash to PostgreSQL. Raw
	// text remains available until the independently configured retention pass.
	return nil
}

func (r *Analytics) newsProcessing(ctx context.Context, newsID string) (newsProcessingRow, bool, error) {
	var rows []newsProcessingRow
	query := fmt.Sprintf(`SELECT news_id, source_id, content_hash, fact_count
FROM %s.news_processing FINAL
WHERE news_id = %s
LIMIT 1
FORMAT JSONEachRow`, r.table("news_processing"), escape(newsID))
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return newsProcessingRow{}, false, err
	}
	if len(rows) == 0 {
		return newsProcessingRow{}, false, nil
	}
	return rows[0], true, nil
}

func (r *Analytics) newsMetadata(ctx context.Context, newsID string) (newsMetadataRow, bool, error) {
	var rows []newsMetadataRow
	query := fmt.Sprintf(`SELECT
    news_id,
    source_id,
    source_url,
    material_kind,
    media_type,
    title,
    ifNull(toString(published_at), '') AS published_at
FROM %s.news FINAL
WHERE news_id = %s
LIMIT 1
FORMAT JSONEachRow`, r.table("news"), escape(newsID))
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return newsMetadataRow{}, false, err
	}
	if len(rows) == 0 {
		return newsMetadataRow{}, false, nil
	}
	return rows[0], true, nil
}

func metadataMatches(actual newsMetadataRow, expected domain.News) bool {
	materialKind := expected.MaterialKind
	if materialKind == "" {
		materialKind = "external_news"
	}
	mediaType := expected.MediaType
	if mediaType == "" {
		mediaType = "text/html"
	}
	if actual.ID != expected.ID || actual.SourceID != expected.SourceID || actual.SourceURL != expected.SourceURL ||
		actual.MaterialKind != materialKind || actual.MediaType != mediaType || actual.Title != expected.Title {
		return false
	}
	if expected.PublishedAt == nil {
		return actual.PublishedAt == ""
	}
	expectedTime, err := time.Parse(time.RFC3339Nano, *expected.PublishedAt)
	if err != nil {
		return false
	}
	actualTime, err := time.ParseInLocation("2006-01-02 15:04:05.000", actual.PublishedAt, time.UTC)
	return err == nil && actualTime.Equal(expectedTime.Truncate(time.Millisecond))
}

func hashNewsText(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func (r *Analytics) FactsByNews(ctx context.Context, newsID string, limit int) ([]domain.Fact, error) {
	query := fmt.Sprintf(`SELECT f.fact_id AS fact_id, f.news_id AS news_id, f.source_id AS source_id, f.source_url AS source_url,
ifNull(n.material_kind, 'external_news') AS material_kind, f.fact_text AS fact_text, f.category AS category,
f.entities AS entities, f.evidence AS evidence, f.occurred_at AS occurred_at, f.confidence AS confidence
FROM (SELECT * FROM %s.facts FINAL) AS f LEFT JOIN (SELECT * FROM %s.news FINAL) AS n ON f.news_id = n.news_id
WHERE f.news_id = %s ORDER BY f.inserted_at, f.fact_id LIMIT %d FORMAT JSONEachRow`, r.table("facts"), r.table("news"), escape(newsID), limit)
	var rows []domain.Fact
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Analytics) QueryFacts(ctx context.Context, filter domain.FactQuery) ([]domain.FactRecord, error) {
	companyFields := `'' AS relevance, toFloat64(0) AS match_confidence`
	if filter.CompanyID != "" {
		companyFields = "m.relevance, m.confidence AS match_confidence"
	}
	query := fmt.Sprintf(`SELECT f.fact_id AS fact_id, f.news_id AS news_id, f.source_id AS source_id,
f.source_url AS source_url, ifNull(n.material_kind, 'external_news') AS material_kind,
f.fact_text AS fact_text, f.category AS category, f.entities AS entities,
f.evidence AS evidence, f.occurred_at AS occurred_at, f.confidence AS confidence, %s
FROM (SELECT * FROM %s.facts FINAL) AS f
LEFT JOIN (SELECT * FROM %s.news FINAL) AS n ON f.news_id = n.news_id`, companyFields, r.table("facts"), r.table("news"))
	if filter.CompanyID != "" {
		query += fmt.Sprintf(`
INNER JOIN (SELECT * FROM %s.fact_company FINAL WHERE company_id = %s) AS m
    ON m.fact_id = f.fact_id`, r.table("fact_company"), escape(filter.CompanyID))
	}
	conditions := []string{"1"}
	newsConditions := make([]string, 0, 3)
	if filter.NewsID != "" {
		conditions = append(conditions, "f.news_id = "+escape(filter.NewsID))
	}
	if filter.SourceID != "" {
		newsConditions = append(newsConditions, "source_id = "+escape(filter.SourceID))
	}
	if filter.MaterialKind != "" {
		newsConditions = append(newsConditions, "material_kind = "+escape(filter.MaterialKind))
	}
	if filter.Category != "" {
		conditions = append(conditions, "f.category = "+escape(filter.Category))
	}
	if filter.Entity != "" {
		conditions = append(conditions, "arrayExists(x -> lowerUTF8(x) = lowerUTF8("+escape(filter.Entity)+"), f.entities)")
	}
	if filter.PeriodFrom != nil {
		from := escape(filter.PeriodFrom.UTC().Format(time.RFC3339Nano))
		to := escape(filter.PeriodTo.UTC().Format(time.RFC3339Nano))
		newsConditions = append(newsConditions,
			"ifNull(published_at, inserted_at) >= parseDateTime64BestEffort("+from+", 3, 'UTC')",
			"ifNull(published_at, inserted_at) < parseDateTime64BestEffort("+to+", 3, 'UTC')")
	}
	if len(newsConditions) > 0 {
		conditions = append(conditions, fmt.Sprintf(`f.news_id IN (
    SELECT news_id
    FROM %s.news FINAL
    WHERE %s
)`, r.table("news"), strings.Join(newsConditions, " AND ")))
	}
	if filter.BeforeFactID != "" {
		conditions = append(conditions, "f.fact_id < toUUID("+escape(filter.BeforeFactID)+")")
	}
	query += "\nWHERE " + strings.Join(conditions, " AND ")
	query += fmt.Sprintf("\nORDER BY f.fact_id DESC LIMIT %d FORMAT JSONEachRow", filter.Limit)
	var rows []domain.FactRecord
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Analytics) SaveFactCompanyLinks(ctx context.Context, companyID string, links []domain.FactCompanyLink) error {
	rows := make([]factCompanyRow, 0, len(links))
	for _, link := range links {
		rows = append(rows, factCompanyRow{CompanyID: companyID, FactID: link.FactID, Relevance: link.Relevance, Confidence: link.Confidence})
	}
	return r.client.Insert(ctx, "fact_company", rows)
}

type factCompanyRow struct {
	CompanyID  string  `json:"company_id"`
	FactID     string  `json:"fact_id"`
	Relevance  string  `json:"relevance"`
	Confidence float64 `json:"confidence"`
}

func (r *Analytics) CompanyFacts(ctx context.Context, companyID string, limit int) ([]domain.CompanyFact, error) {
	query := fmt.Sprintf(`SELECT
    f.fact_id AS fact_id,
    f.news_id AS news_id,
    f.source_id AS source_id,
    f.source_url AS source_url,
    ifNull(n.material_kind, 'external_news') AS material_kind,
    f.fact_text AS fact_text,
    f.category AS category,
    f.entities AS entities,
    f.evidence AS evidence,
    f.occurred_at AS occurred_at,
    f.confidence AS confidence,
    m.relevance AS relevance,
    m.confidence AS match_confidence
FROM (SELECT * FROM %s.fact_company FINAL) AS m
INNER JOIN (SELECT * FROM %s.facts FINAL) AS f ON m.fact_id = f.fact_id
LEFT JOIN (SELECT * FROM %s.news FINAL) AS n ON f.news_id = n.news_id
WHERE m.company_id = %s
ORDER BY m.inserted_at DESC
LIMIT %d
FORMAT JSONEachRow`,
		r.table("fact_company"),
		r.table("facts"),
		r.table("news"),
		escape(companyID),
		limit,
	)
	var rows []domain.CompanyFact
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

type metricObservationRow struct {
	ID            string   `json:"observation_id"`
	CompanyID     string   `json:"company_id"`
	MetricID      string   `json:"metric_id"`
	NewsID        string   `json:"news_id"`
	Value         *float64 `json:"value"`
	Probability   *float64 `json:"probability"`
	SourceFactIDs []string `json:"source_fact_ids"`
}

func (r *Analytics) SaveMetricObservations(ctx context.Context, companyID string, observations []domain.MetricObservation) error {
	rows := make([]metricObservationRow, 0, len(observations))
	for _, observation := range observations {
		id := observation.ID
		if id == "" {
			factIDs := append([]string(nil), observation.SourceFactIDs...)
			sort.Strings(factIDs)
			id = stableUUID(strings.Join([]string{companyID, observation.MetricID, observation.NewsID, strings.Join(factIDs, ",")}, "\x00"))
		}
		if observation.SourceFactIDs == nil {
			observation.SourceFactIDs = []string{}
		}
		rows = append(rows, metricObservationRow{ID: id, CompanyID: companyID, MetricID: observation.MetricID, NewsID: observation.NewsID, Value: observation.Value, Probability: observation.Probability, SourceFactIDs: observation.SourceFactIDs})
	}
	return r.client.Insert(ctx, "metric_observations", rows)
}

type companyAnalysisRow struct {
	ID          string   `json:"analysis_id"`
	CompanyID   string   `json:"company_id"`
	PeriodFrom  string   `json:"period_from"`
	PeriodTo    string   `json:"period_to"`
	Probability float64  `json:"probability"`
	MetricIDs   []string `json:"metric_ids"`
	Text        string   `json:"analysis_text"`
}

func (r *Analytics) SaveCompanyAnalysis(ctx context.Context, analysis domain.CompanyAnalysis) error {
	id := stableUUID(strings.Join([]string{analysis.CompanyID, analysis.PeriodFrom, analysis.PeriodTo}, "\x00"))
	return r.client.Insert(ctx, "company_analyses", []companyAnalysisRow{{ID: id, CompanyID: analysis.CompanyID, PeriodFrom: analysis.PeriodFrom, PeriodTo: analysis.PeriodTo, Probability: analysis.Probability, MetricIDs: analysis.MetricIDs, Text: analysis.Text}})
}

type totalRow struct {
	TotalValue       *float64 `json:"total_value"`
	TotalProbability *float64 `json:"total_probability"`
	FactCount        uint32   `json:"fact_count"`
	ProbabilityCount uint32   `json:"probability_count"`
}

type metricLinkRow struct {
	NewsID     string `json:"news_id"`
	NewsURL    string `json:"news_url"`
	NewsTime   string `json:"news_time"`
	SourceKind string `json:"source_kind"`
}

type metricTotalSnapshotRow struct {
	CompanyID        string   `json:"company_id"`
	MetricID         string   `json:"metric_id"`
	PeriodFrom       string   `json:"period_from"`
	PeriodTo         string   `json:"period_to"`
	TotalValue       *float64 `json:"total_value"`
	TotalProbability *float64 `json:"total_probability"`
	FactCount        uint32   `json:"fact_count"`
	ProbabilityCount uint32   `json:"probability_count"`
	LinksJSON        string   `json:"links_json"`
}

func (r *Analytics) CalculateMetricTotal(ctx context.Context, metricID, from, to string) (domain.MetricTotal, error) {
	observations := fmt.Sprintf("%s.metric_observations", r.table("metric_observations"))
	news := fmt.Sprintf("%s.news", r.table("news"))
	base := fmt.Sprintf(`FROM (SELECT * FROM %s FINAL) AS mo
INNER JOIN (SELECT * FROM %s FINAL) AS n ON n.news_id = mo.news_id
WHERE mo.metric_id = %s AND mo.value IS NOT NULL AND n.published_at IS NOT NULL
AND n.published_at >= parseDateTime64BestEffort(%s, 3, 'UTC')
AND n.published_at <= parseDateTime64BestEffort(%s, 3, 'UTC')`, observations, news, escape(metricID), escape(from), escape(to))
	var aggregate []totalRow
	if err := r.client.queryRows(ctx, "SELECT avgOrNull(mo.value) AS total_value, avgOrNull(mo.probability) AS total_probability, "+
		"toUInt32(count()) AS fact_count, toUInt32(count(mo.probability)) AS probability_count "+base+" FORMAT JSONEachRow", &aggregate); err != nil {
		return domain.MetricTotal{}, err
	}
	result := domain.MetricTotal{MetricID: metricID, PeriodFrom: from, PeriodTo: to, Links: []domain.MetricLink{}}
	if len(aggregate) > 0 {
		result.TotalValue = aggregate[0].TotalValue
		result.TotalProbability = aggregate[0].TotalProbability
		result.FactCount = aggregate[0].FactCount
		result.ProbabilityCount = aggregate[0].ProbabilityCount
	}
	var linkRows []metricLinkRow
	linkQuery := fmt.Sprintf(`SELECT n.news_id AS news_id, ifNull(any(n.source_url), '') AS news_url,
toString(min(n.published_at)) AS news_time,
multiIf(any(n.material_kind) = 'alpha_analysis', 'bank', any(n.material_kind) = 'external_news', 'external', '') AS source_kind %s
GROUP BY n.news_id ORDER BY min(n.published_at), n.news_id FORMAT JSONEachRow`, base)
	if err := r.client.queryRows(ctx, linkQuery, &linkRows); err != nil {
		return domain.MetricTotal{}, err
	}
	for _, link := range linkRows {
		result.Links = append(result.Links, domain.MetricLink{NewsID: link.NewsID, NewsURL: link.NewsURL, NewsTime: link.NewsTime, SourceKind: link.SourceKind})
	}
	return result, nil
}

// SaveMetricTotal stores the latest aggregate snapshot in ClickHouse. Metric
// definitions remain in PostgreSQL; this row contains only IDs and analytical
// values needed to reproduce the returned result and its source links.
func (r *Analytics) SaveMetricTotal(ctx context.Context, total domain.MetricTotal) error {
	links := total.Links
	if links == nil {
		links = []domain.MetricLink{}
	}
	encodedLinks, err := json.Marshal(links)
	if err != nil {
		return err
	}
	return r.client.Insert(ctx, "metric_totals", []metricTotalSnapshotRow{{
		CompanyID: total.CompanyID, MetricID: total.MetricID, PeriodFrom: total.PeriodFrom, PeriodTo: total.PeriodTo,
		TotalValue: total.TotalValue, TotalProbability: total.TotalProbability,
		FactCount: total.FactCount, ProbabilityCount: total.ProbabilityCount, LinksJSON: string(encodedLinks),
	}})
}

type analysisResult struct {
	CompanyID   string   `json:"company_id"`
	PeriodFrom  string   `json:"period_from"`
	PeriodTo    string   `json:"period_to"`
	Probability float64  `json:"probability"`
	MetricIDs   []string `json:"metric_ids"`
	Text        string   `json:"analysis_text"`
}

func (r *Analytics) CompanyReport(ctx context.Context, companyID, from, to string) (domain.CompanyReport, error) {
	companyFacts, err := r.CompanyFacts(ctx, companyID, r.companyReportFactsLimit)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	analysisQuery := fmt.Sprintf(`SELECT company_id, period_from, period_to, probability, metric_ids, analysis_text
FROM %s.company_analyses FINAL
WHERE company_id = %s
  AND period_from >= toDate(%s)
  AND period_to <= toDate(%s)
ORDER BY generated_at DESC
LIMIT %d
FORMAT JSONEachRow`,
		r.table("company_analyses"),
		escape(companyID),
		escape(from),
		escape(to),
		r.companyReportAnalysisLimit,
	)
	var analysisRows []analysisResult
	if err := r.client.queryRows(ctx, analysisQuery, &analysisRows); err != nil {
		return domain.CompanyReport{}, err
	}
	analyses := make([]domain.CompanyAnalysis, 0, len(analysisRows))
	for _, item := range analysisRows {
		analyses = append(analyses, domain.CompanyAnalysis{CompanyID: item.CompanyID, PeriodFrom: item.PeriodFrom, PeriodTo: item.PeriodTo, Probability: item.Probability, MetricIDs: item.MetricIDs, Text: item.Text})
	}
	return domain.CompanyReport{CompanyID: companyID, Facts: companyFacts, Analyses: analyses}, nil
}

func (r *Analytics) ConsultantContext(ctx context.Context, companyID string, limit int) (domain.ConsultantContext, error) {
	facts, err := r.CompanyFacts(ctx, companyID, limit)
	if err != nil {
		return domain.ConsultantContext{}, err
	}
	query := fmt.Sprintf(`SELECT
    mo.observation_id AS observation_id,
    mo.company_id AS company_id,
    mo.metric_id AS metric_id,
    mo.news_id AS news_id,
    if(empty(ifNull(n.source_id, '')), mo.news_id, ifNull(n.source_id, '')) AS source_id,
    ifNull(n.source_url, '') AS source_url,
    if(empty(ifNull(n.material_kind, '')), 'external_news', ifNull(n.material_kind, '')) AS material_kind,
    mo.value AS value,
    mo.probability AS probability,
    mo.source_fact_ids AS source_fact_ids
FROM (SELECT * FROM %s.metric_observations FINAL) AS mo
LEFT JOIN (SELECT * FROM %s.news FINAL) AS n ON mo.news_id = n.news_id
WHERE mo.company_id = %s
ORDER BY mo.inserted_at DESC
LIMIT %d
FORMAT JSONEachRow`,
		r.table("metric_observations"),
		r.table("news"),
		escape(companyID),
		limit,
	)
	var observations []domain.MetricObservation
	if err := r.client.queryRows(ctx, query, &observations); err != nil {
		return domain.ConsultantContext{}, err
	}
	return domain.ConsultantContext{CompanyID: companyID, Facts: facts, Observations: observations, Totals: []domain.CompanyMetricTotal{}}, nil
}

func (r *Analytics) table(_ string) string { return "`" + r.client.database + "`" }

func stableUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	data := digest[:16]
	data[6] = data[6]&0x0f | 0x50
	data[8] = data[8]&0x3f | 0x80
	value := hex.EncodeToString(data)
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}

func clickHouseTime(value *string) *string {
	if value == nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return value
	}
	formatted := parsed.UTC().Format("2006-01-02 15:04:05.000")
	return &formatted
}

var _ domain.AnalyticsRepository = (*Analytics)(nil)
