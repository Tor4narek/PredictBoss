package analytics

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type behaviorRepository struct {
	domain.AnalyticsRepository

	news             domain.News
	facts            []domain.Fact
	saveNewsErr      error
	finalizedNews    domain.News
	finalizedHash    string
	finalizeErr      error
	query            domain.FactQuery
	queryFacts       []domain.FactRecord
	queryErr         error
	observations     []domain.MetricObservation
	companyID        string
	saveObservation  error
	saveAnalysis     domain.CompanyAnalysis
	saveAnalysisErr  error
	calculatedMetric []string
}

func (r *behaviorRepository) SaveNewsAndFacts(_ context.Context, news domain.News, facts []domain.Fact) ([]string, error) {
	r.news, r.facts = news, facts
	return []string{"fact-1"}, r.saveNewsErr
}

func (r *behaviorRepository) FinalizeNews(_ context.Context, news domain.News, hash string) error {
	r.finalizedNews, r.finalizedHash = news, hash
	return r.finalizeErr
}

func (r *behaviorRepository) QueryFacts(_ context.Context, query domain.FactQuery) ([]domain.FactRecord, error) {
	r.query = query
	return r.queryFacts, r.queryErr
}

func (r *behaviorRepository) SaveMetricObservations(_ context.Context, companyID string, observations []domain.MetricObservation) error {
	r.companyID, r.observations = companyID, observations
	return r.saveObservation
}

func (r *behaviorRepository) SaveCompanyAnalysis(_ context.Context, analysis domain.CompanyAnalysis) error {
	r.saveAnalysis = analysis
	return r.saveAnalysisErr
}

func (r *behaviorRepository) CalculateMetricTotal(_ context.Context, metricID, _, _ string) (domain.MetricTotal, error) {
	r.calculatedMetric = append(r.calculatedMetric, metricID)
	return domain.MetricTotal{MetricID: metricID, Links: []domain.MetricLink{}}, nil
}

func TestSaveNewsAndFactsNormalizesBeforePersistence(t *testing.T) {
	repository := &behaviorRepository{}
	service := NewService(repository, &metricSettingsFake{}, testLimits())
	publishedAt := "2026-10-25T09:00:00Z"
	ids, err := service.SaveNewsAndFacts(context.Background(), domain.News{
		ID: " news-1 ", SourceID: " source-1 ", SourceURL: " https://example.test/news ",
		CompanyID: "11111111-1111-4111-8111-111111111111", Text: " Article ", PublishedAt: &publishedAt,
	}, []domain.Fact{{Text: " Evidence-backed fact ", Confidence: 0.8}})
	if err != nil {
		t.Fatalf("SaveNewsAndFacts returned error: %v", err)
	}
	if len(ids) != 1 || repository.news.ID != "news-1" || repository.news.Text != "Article" || repository.news.SourceID != "source-1" {
		t.Fatalf("normalized news or returned fact IDs are wrong: news=%#v ids=%#v", repository.news, ids)
	}
	if len(repository.facts) != 1 || repository.facts[0].NewsID != "news-1" || repository.facts[0].SourceID != "source-1" || repository.facts[0].Category != "general" {
		t.Fatalf("fact was not bound to normalized news metadata: %#v", repository.facts)
	}
}

func TestSaveNewsAndFactsRejectsInvalidInputsBeforePersistence(t *testing.T) {
	for _, test := range []struct {
		name string
		news domain.News
		fact domain.Fact
	}{
		{name: "missing text", news: domain.News{ID: "news-1", SourceID: "source-1"}},
		{name: "invalid published time", news: domain.News{ID: "news-1", SourceID: "source-1", Text: "body", PublishedAt: stringPointer("yesterday")}},
		{name: "invalid confidence", news: domain.News{ID: "news-1", SourceID: "source-1", Text: "body"}, fact: domain.Fact{Text: "fact", Confidence: 1.1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &behaviorRepository{}
			service := NewService(repository, &metricSettingsFake{}, testLimits())
			_, err := service.SaveNewsAndFacts(context.Background(), test.news, []domain.Fact{test.fact})
			if !errors.Is(err, ErrInvalidAnalyticsInput) {
				t.Fatalf("SaveNewsAndFacts error = %v, want invalid analytics input", err)
			}
			if repository.news.ID != "" {
				t.Fatalf("invalid input reached persistence: %#v", repository.news)
			}
		})
	}
}

func TestFinalizeNewsHashesTextAndPropagatesPersistenceFailure(t *testing.T) {
	storageErr := errors.New("ClickHouse unavailable")
	repository := &behaviorRepository{finalizeErr: storageErr}
	service := NewService(repository, &metricSettingsFake{}, testLimits())
	_, err := service.FinalizeNews(context.Background(), domain.News{ID: "news-1", SourceID: "source-1", Text: "source text"})
	if !errors.Is(err, storageErr) || repository.finalizedHash == "" || repository.finalizedNews.Text != "source text" {
		t.Fatalf("FinalizeNews did not pass a text hash and preserve the adapter error: hash=%q news=%#v err=%v", repository.finalizedHash, repository.finalizedNews, err)
	}
}

func TestQueryFactsAppliesCursorPaginationAndRejectsUnboundedQuery(t *testing.T) {
	repository := &behaviorRepository{queryFacts: []domain.FactRecord{
		{Fact: domain.Fact{ID: "00000000-0000-4000-8000-000000000001"}},
		{Fact: domain.Fact{ID: "00000000-0000-4000-8000-000000000002"}},
		{Fact: domain.Fact{ID: "00000000-0000-4000-8000-000000000003"}},
	}}
	service := NewService(repository, &metricSettingsFake{}, testLimits())
	page, err := service.QueryFacts(context.Background(), FactsRequest{
		CompanyID: " 11111111-1111-4111-8111-111111111111 ", SourceID: " source-7 ",
		PeriodFrom: "2026-10-01T00:00:00Z", PeriodTo: "2026-10-02T00:00:00Z", Limit: 2,
	})
	if err != nil {
		t.Fatalf("QueryFacts returned error: %v", err)
	}
	if !page.HasMore || len(page.Facts) != 2 || page.NextBeforeID != page.Facts[1].ID || repository.query.Limit != 3 {
		t.Fatalf("unexpected paged response or repository limit: page=%#v filter=%#v", page, repository.query)
	}
	if repository.query.CompanyID != "11111111-1111-4111-8111-111111111111" || repository.query.SourceID != "source-7" || repository.query.PeriodFrom == nil {
		t.Fatalf("filters were not normalized and parsed: %#v", repository.query)
	}
	if _, err := service.QueryFacts(context.Background(), FactsRequest{}); !errors.Is(err, ErrInvalidAnalyticsInput) {
		t.Fatalf("unbounded facts query error = %v, want invalid analytics input", err)
	}
}

func TestSaveMetricObservationsRequiresCompanyOwnedMetric(t *testing.T) {
	companyID := "11111111-1111-4111-8111-111111111111"
	metricID := "22222222-2222-4222-8222-222222222222"
	value, probability := 0.0, 0.7
	settings := &metricSettingsFake{metrics: []domain.CompanyMetric{{ID: metricID}}}
	repository := &behaviorRepository{}
	service := NewService(repository, settings, testLimits())
	observations := []domain.MetricObservation{{MetricID: metricID, NewsID: "news-7", Value: &value, Probability: &probability}}
	if err := service.SaveMetricObservations(context.Background(), companyID, observations); err != nil {
		t.Fatalf("SaveMetricObservations returned error: %v", err)
	}
	if repository.companyID != companyID || len(repository.observations) != 1 || repository.observations[0].CompanyID != companyID {
		t.Fatalf("observation was not scoped to its company: company=%q values=%#v", repository.companyID, repository.observations)
	}
	observations[0].MetricID = "33333333-3333-4333-8333-333333333333"
	if err := service.SaveMetricObservations(context.Background(), companyID, observations); !errors.Is(err, ErrInvalidAnalyticsInput) {
		t.Fatalf("unknown company metric error = %v, want invalid analytics input", err)
	}
	observations[0].MetricID = metricID
	invalid := math.NaN()
	observations[0].Value = &invalid
	if err := service.SaveMetricObservations(context.Background(), companyID, observations); !errors.Is(err, ErrInvalidAnalyticsInput) {
		t.Fatalf("non-finite observation error = %v, want invalid analytics input", err)
	}
}

func TestSaveCompanyAnalysisRejectsUnknownMetric(t *testing.T) {
	settings := &metricSettingsFake{metrics: []domain.CompanyMetric{}}
	repository := &behaviorRepository{}
	service := NewService(repository, settings, testLimits())
	err := service.SaveCompanyAnalysis(context.Background(), domain.CompanyAnalysis{
		CompanyID: "11111111-1111-4111-8111-111111111111", PeriodFrom: "2026-10-01", PeriodTo: "2026-10-02",
		Probability: 0.5, MetricIDs: []string{"22222222-2222-4222-8222-222222222222"}, Text: "summary",
	})
	if !errors.Is(err, ErrInvalidAnalyticsInput) || repository.saveAnalysis.Text != "" {
		t.Fatalf("analysis with an unknown metric reached persistence: error=%v saved=%#v", err, repository.saveAnalysis)
	}
}

func stringPointer(value string) *string { return &value }
