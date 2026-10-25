package clickhouse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type Analytics struct{ client *Client }

func NewAnalytics(client *Client) *Analytics { return &Analytics{client: client} }

type newsRow struct {
	ID        string `json:"news_id"`
	SourceID  string `json:"source_id"`
	SourceURL string `json:"source_url"`
	Text      string `json:"news_text"`
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
	if err := r.client.Insert(ctx, "news", []newsRow{{ID: news.ID, SourceID: news.SourceID, SourceURL: news.SourceURL, Text: news.Text}}); err != nil {
		return nil, err
	}
	rows := make([]factRow, 0, len(facts))
	ids := make([]string, 0, len(facts))
	for _, fact := range facts {
		id := stableUUID(news.ID + "\x00" + fact.Text)
		if fact.Entities == nil {
			fact.Entities = []string{}
		}
		rows = append(rows, factRow{ID: id, NewsID: news.ID, SourceID: news.SourceID, SourceURL: news.SourceURL, Text: fact.Text, Category: fact.Category, Entities: fact.Entities, Evidence: fact.Evidence, OccurredAt: fact.OccurredAt, Confidence: fact.Confidence})
		ids = append(ids, id)
	}
	if err := r.client.Insert(ctx, "facts", rows); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Analytics) FactsByNews(ctx context.Context, newsID string, limit int) ([]domain.Fact, error) {
	query := fmt.Sprintf(`SELECT fact_id, news_id, source_id, source_url, fact_text, category, entities, evidence,
occurred_at, confidence FROM %s.facts FINAL WHERE news_id = %s ORDER BY inserted_at, fact_id LIMIT %d FORMAT JSONEachRow`, r.table("facts"), escape(newsID), limit)
	var rows []domain.Fact
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
	query := fmt.Sprintf(`SELECT f.fact_id, f.news_id, f.source_id, f.source_url, f.fact_text, f.category, f.entities,
f.evidence, f.occurred_at, f.confidence, m.relevance, m.confidence AS match_confidence
FROM %s.fact_company AS m FINAL INNER JOIN %s.facts AS f FINAL ON m.fact_id = f.fact_id
WHERE m.company_id = %s ORDER BY m.inserted_at DESC LIMIT %d FORMAT JSONEachRow`, r.table("fact_company"), r.table("facts"), escape(companyID), limit)
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
	Probability   float64  `json:"probability"`
	PeriodFrom    string   `json:"period_from"`
	PeriodTo      string   `json:"period_to"`
	SourceFactIDs []string `json:"source_fact_ids"`
}

func (r *Analytics) SaveMetricObservations(ctx context.Context, companyID string, observations []domain.MetricObservation) error {
	rows := make([]metricObservationRow, 0, len(observations))
	for _, observation := range observations {
		id := stableUUID(strings.Join([]string{companyID, observation.MetricID, observation.NewsID, observation.PeriodFrom, observation.PeriodTo}, "\x00"))
		if observation.SourceFactIDs == nil {
			observation.SourceFactIDs = []string{}
		}
		rows = append(rows, metricObservationRow{ID: id, CompanyID: companyID, MetricID: observation.MetricID, NewsID: observation.NewsID, Value: observation.Value, Probability: observation.Probability, PeriodFrom: observation.PeriodFrom, PeriodTo: observation.PeriodTo, SourceFactIDs: observation.SourceFactIDs})
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
	CompanyID        string  `json:"company_id"`
	MetricID         string  `json:"metric_id"`
	PeriodFrom       string  `json:"period_from"`
	PeriodTo         string  `json:"period_to"`
	Total            float64 `json:"total"`
	ObservationCount uint32  `json:"observation_count"`
}

func (r *Analytics) CalculateMetricTotal(ctx context.Context, companyID, metricID, from, to string) (domain.MetricTotal, error) {
	query := fmt.Sprintf(`SELECT %s AS company_id, %s AS metric_id, toString(toDate(%s)) AS period_from,
toString(toDate(%s)) AS period_to, sum(ifNull(value, 0) * probability) AS total,
countIf(isNotNull(value)) AS observation_count FROM %s.metric_observations
WHERE company_id = %s AND metric_id = %s AND period_to >= toDate(%s) AND period_from <= toDate(%s)
GROUP BY company_id, metric_id FORMAT JSONEachRow`, escape(companyID), escape(metricID), escape(from), escape(to), r.table("metric_observations"), escape(companyID), escape(metricID), escape(from), escape(to))
	var rows []totalRow
	if err := r.client.queryRows(ctx, query, &rows); err != nil {
		return domain.MetricTotal{}, err
	}
	total := totalRow{CompanyID: companyID, MetricID: metricID, PeriodFrom: from, PeriodTo: to}
	if len(rows) != 0 {
		total = rows[0]
	}
	return r.saveTotal(ctx, total)
}

func (r *Analytics) saveTotal(ctx context.Context, row totalRow) (domain.MetricTotal, error) {
	if err := r.client.Insert(ctx, "metric_totals", []totalRow{row}); err != nil {
		return domain.MetricTotal{}, err
	}
	return domain.MetricTotal{CompanyID: row.CompanyID, MetricID: row.MetricID, PeriodFrom: row.PeriodFrom, PeriodTo: row.PeriodTo, Total: row.Total, ObservationCount: row.ObservationCount, CalculatedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
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
	totalsQuery := fmt.Sprintf(`SELECT company_id, metric_id, period_from, period_to, total, observation_count,
calculated_at FROM %s.metric_totals FINAL WHERE company_id = %s AND period_from >= toDate(%s) AND period_to <= toDate(%s)
ORDER BY calculated_at DESC LIMIT 500 FORMAT JSONEachRow`, r.table("metric_totals"), escape(companyID), escape(from), escape(to))
	var totals []domain.MetricTotal
	if err := r.client.queryRows(ctx, totalsQuery, &totals); err != nil {
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
	return domain.CompanyReport{CompanyID: companyID, Facts: companyFacts, Totals: totals, Analyses: analyses}, nil
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

var _ domain.AnalyticsRepository = (*Analytics)(nil)
