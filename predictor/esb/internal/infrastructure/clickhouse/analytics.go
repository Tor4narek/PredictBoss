package clickhouse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type Analytics struct{ client *Client }

func NewAnalytics(client *Client) *Analytics { return &Analytics{client: client} }

type newsRow struct {
	ID           string  `json:"news_id"`
	SourceID     string  `json:"source_id"`
	SourceURL    string  `json:"source_url"`
	CompanyID    string  `json:"company_id"`
	MaterialKind string  `json:"material_kind"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`
	PublishedAt  *string `json:"published_at"`
	Text         string  `json:"news_text"`
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
	if err := r.client.Insert(ctx, "news", []newsRow{{ID: news.ID, SourceID: news.SourceID, SourceURL: news.SourceURL, CompanyID: news.CompanyID, MaterialKind: news.MaterialKind, MediaType: news.MediaType, Title: news.Title, PublishedAt: clickHouseTime(news.PublishedAt), Text: news.Text}}); err != nil {
		return nil, err
	}
	rows := make([]factRow, 0, len(facts))
	ids := make([]string, 0, len(facts))
	for _, fact := range facts {
		id := stableUUID(news.ID + "\x00" + fact.Text)
		if fact.Entities == nil {
			fact.Entities = []string{}
		}
		rows = append(rows, factRow{ID: id, NewsID: news.ID, SourceID: news.SourceID, SourceURL: news.SourceURL, Text: fact.Text, Category: fact.Category, Entities: fact.Entities, Evidence: fact.Evidence, OccurredAt: clickHouseTime(fact.OccurredAt), Confidence: fact.Confidence})
		ids = append(ids, id)
	}
	if err := r.client.Insert(ctx, "facts", rows); err != nil {
		return nil, err
	}
	return ids, nil
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
		conditions = append(conditions, fmt.Sprintf("f.news_id IN (SELECT news_id FROM %s.news FINAL WHERE %s)", r.table("news"), strings.Join(newsConditions, " AND ")))
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
	query := fmt.Sprintf(`SELECT f.fact_id AS fact_id, f.news_id AS news_id, f.source_id AS source_id, f.source_url AS source_url,
ifNull(n.material_kind, 'external_news') AS material_kind, f.fact_text AS fact_text, f.category AS category,
f.entities AS entities, f.evidence AS evidence, f.occurred_at AS occurred_at, f.confidence AS confidence,
m.relevance AS relevance, m.confidence AS match_confidence
FROM (SELECT * FROM %s.fact_company FINAL) AS m INNER JOIN (SELECT * FROM %s.facts FINAL) AS f ON m.fact_id = f.fact_id
LEFT JOIN (SELECT * FROM %s.news FINAL) AS n ON f.news_id = n.news_id
WHERE m.company_id = %s ORDER BY m.inserted_at DESC LIMIT %d FORMAT JSONEachRow`, r.table("fact_company"), r.table("facts"), r.table("news"), escape(companyID), limit)
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

type analysisResult struct {
	CompanyID   string   `json:"company_id"`
	PeriodFrom  string   `json:"period_from"`
	PeriodTo    string   `json:"period_to"`
	Probability float64  `json:"probability"`
	MetricIDs   []string `json:"metric_ids"`
	Text        string   `json:"analysis_text"`
}

func (r *Analytics) CompanyReport(ctx context.Context, companyID, from, to string) (domain.CompanyReport, error) {
	companyFacts, err := r.CompanyFacts(ctx, companyID, 200)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	analysisQuery := fmt.Sprintf(`SELECT company_id, period_from, period_to, probability, metric_ids, analysis_text
FROM %s.company_analyses FINAL WHERE company_id = %s AND period_from >= toDate(%s) AND period_to <= toDate(%s)
ORDER BY generated_at DESC LIMIT 100 FORMAT JSONEachRow`, r.table("company_analyses"), escape(companyID), escape(from), escape(to))
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
	query := fmt.Sprintf(`SELECT mo.observation_id AS observation_id, mo.company_id AS company_id,
mo.metric_id AS metric_id, mo.news_id AS news_id,
if(empty(ifNull(n.source_id, '')), mo.news_id, ifNull(n.source_id, '')) AS source_id, ifNull(n.source_url, '') AS source_url,
if(empty(ifNull(n.material_kind, '')), 'external_news', ifNull(n.material_kind, '')) AS material_kind,
mo.value AS value, mo.probability AS probability,
mo.source_fact_ids AS source_fact_ids FROM (SELECT * FROM %s.metric_observations FINAL) AS mo
LEFT JOIN (SELECT * FROM %s.news FINAL) AS n ON mo.news_id = n.news_id
WHERE mo.company_id = %s ORDER BY mo.inserted_at DESC LIMIT %d FORMAT JSONEachRow`, r.table("metric_observations"), r.table("news"), escape(companyID), limit)
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
