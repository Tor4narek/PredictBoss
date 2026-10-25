package analytics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var ErrInvalidAnalyticsInput = errors.New("invalid analytics input")

type Service struct {
	repository domain.AnalyticsRepository
	metrics    domain.CompanyMetricSettingsRepository
}

func NewService(repository domain.AnalyticsRepository, metrics domain.CompanyMetricSettingsRepository) *Service {
	return &Service{repository: repository, metrics: metrics}
}

func (s *Service) SaveNewsAndFacts(ctx context.Context, news domain.News, facts []domain.Fact) ([]string, error) {
	news.ID = strings.TrimSpace(news.ID)
	news.SourceID = strings.TrimSpace(news.SourceID)
	news.SourceURL = strings.TrimSpace(news.SourceURL)
	news.Text = strings.TrimSpace(news.Text)
	if !idPattern.MatchString(news.ID) || news.SourceID == "" || news.Text == "" || len(facts) > 500 {
		return nil, ErrInvalidAnalyticsInput
	}
	if news.CompanyID != "" && !uuidPattern.MatchString(news.CompanyID) {
		return nil, ErrInvalidAnalyticsInput
	}
	if news.MaterialKind != "" && news.MaterialKind != "external_news" && news.MaterialKind != "alpha_analysis" {
		return nil, ErrInvalidAnalyticsInput
	}
	if news.MediaType != "" && news.MediaType != "application/pdf" && news.MediaType != "text/html" {
		return nil, ErrInvalidAnalyticsInput
	}
	if news.PublishedAt != nil {
		if _, err := time.Parse(time.RFC3339, *news.PublishedAt); err != nil {
			return nil, fmt.Errorf("%w: published_at", ErrInvalidAnalyticsInput)
		}
	}
	for i := range facts {
		facts[i].Text = strings.TrimSpace(facts[i].Text)
		if facts[i].Text == "" || !probability(facts[i].Confidence) {
			return nil, fmt.Errorf("%w: fact at index %d", ErrInvalidAnalyticsInput, i)
		}
		if facts[i].OccurredAt != nil {
			if _, err := time.Parse(time.RFC3339, *facts[i].OccurredAt); err != nil {
				return nil, fmt.Errorf("%w: occurred_at at index %d", ErrInvalidAnalyticsInput, i)
			}
		}
		if facts[i].Category == "" {
			facts[i].Category = "general"
		}
		facts[i].NewsID = news.ID
		facts[i].SourceID = news.SourceID
		facts[i].SourceURL = news.SourceURL
	}
	return s.repository.SaveNewsAndFacts(ctx, news, facts)
}

func (s *Service) FactsByNews(ctx context.Context, newsID string, limit int) ([]domain.Fact, error) {
	if !idPattern.MatchString(strings.TrimSpace(newsID)) {
		return nil, ErrInvalidAnalyticsInput
	}
	return s.repository.FactsByNews(ctx, newsID, clampLimit(limit))
}

func (s *Service) SaveFactCompanyLinks(ctx context.Context, companyID string, links []domain.FactCompanyLink) error {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	if !idPattern.MatchString(companyID) || len(links) > 1000 {
		return ErrInvalidAnalyticsInput
	}
	for i := range links {
		links[i].FactID = strings.TrimSpace(links[i].FactID)
		links[i].Relevance = strings.TrimSpace(links[i].Relevance)
		if !uuidPattern.MatchString(links[i].FactID) || links[i].Relevance == "" || !probability(links[i].Confidence) {
			return fmt.Errorf("%w: fact-company link at index %d", ErrInvalidAnalyticsInput, i)
		}
	}
	return s.repository.SaveFactCompanyLinks(ctx, companyID, links)
}

func (s *Service) CompanyFacts(ctx context.Context, companyID string, limit int) ([]domain.CompanyFact, error) {
	if !idPattern.MatchString(strings.TrimSpace(companyID)) {
		return nil, ErrInvalidAnalyticsInput
	}
	return s.repository.CompanyFacts(ctx, companyID, clampLimit(limit))
}

func (s *Service) SaveMetricObservations(ctx context.Context, companyID string, observations []domain.MetricObservation) error {
	companyID = strings.TrimSpace(companyID)
	if !idPattern.MatchString(companyID) || len(observations) > 1000 {
		return ErrInvalidAnalyticsInput
	}
	for i := range observations {
		observation := &observations[i]
		observation.CompanyID = companyID
		observation.MetricID = strings.TrimSpace(observation.MetricID)
		observation.NewsID = strings.TrimSpace(observation.NewsID)
		if !idPattern.MatchString(observation.MetricID) || !idPattern.MatchString(observation.NewsID) || !probability(observation.Probability) {
			return fmt.Errorf("%w: metric observation at index %d", ErrInvalidAnalyticsInput, i)
		}
		if observation.Value != nil && (math.IsNaN(*observation.Value) || math.IsInf(*observation.Value, 0)) {
			return fmt.Errorf("%w: metric value at index %d", ErrInvalidAnalyticsInput, i)
		}
		if _, err := dateRange(observation.PeriodFrom, observation.PeriodTo); err != nil {
			return fmt.Errorf("%w: metric period at index %d", ErrInvalidAnalyticsInput, i)
		}
		for _, factID := range observation.SourceFactIDs {
			if !uuidPattern.MatchString(factID) {
				return fmt.Errorf("%w: source fact id at index %d", ErrInvalidAnalyticsInput, i)
			}
		}
	}
	known, err := s.knownMetricIDs(ctx, companyID)
	if err != nil {
		return err
	}
	for i := range observations {
		if _, ok := known[observations[i].MetricID]; !ok {
			return fmt.Errorf("%w: unknown metric_id %q for company", ErrInvalidAnalyticsInput, observations[i].MetricID)
		}
	}
	return s.repository.SaveMetricObservations(ctx, companyID, observations)
}

func (s *Service) SaveCompanyAnalysis(ctx context.Context, analysis domain.CompanyAnalysis) error {
	analysis.CompanyID = strings.TrimSpace(analysis.CompanyID)
	analysis.Text = strings.TrimSpace(analysis.Text)
	if !idPattern.MatchString(analysis.CompanyID) || analysis.Text == "" || !probability(analysis.Probability) {
		return ErrInvalidAnalyticsInput
	}
	if _, err := dateRange(analysis.PeriodFrom, analysis.PeriodTo); err != nil {
		return ErrInvalidAnalyticsInput
	}
	for _, metricID := range analysis.MetricIDs {
		if !idPattern.MatchString(metricID) {
			return ErrInvalidAnalyticsInput
		}
	}
	known, err := s.knownMetricIDs(ctx, analysis.CompanyID)
	if err != nil {
		return err
	}
	for _, metricID := range analysis.MetricIDs {
		if _, ok := known[metricID]; !ok {
			return fmt.Errorf("%w: unknown metric_id %q for company", ErrInvalidAnalyticsInput, metricID)
		}
	}
	return s.repository.SaveCompanyAnalysis(ctx, analysis)
}

func (s *Service) CalculateMetricTotal(ctx context.Context, companyID, metricID, from, to string) (domain.MetricTotal, error) {
	if !idPattern.MatchString(strings.TrimSpace(companyID)) || !idPattern.MatchString(strings.TrimSpace(metricID)) {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	if _, err := dateRange(from, to); err != nil {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	metrics, err := s.metrics.CompanyMetrics(ctx, companyID, 500)
	if err != nil {
		return domain.MetricTotal{}, err
	}
	known := false
	for _, metric := range metrics {
		if metric.ID == metricID {
			known = true
			break
		}
	}
	if !known {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	return s.repository.CalculateMetricTotal(ctx, companyID, metricID, from, to)
}

func (s *Service) CalculateAllMetricTotals(ctx context.Context, companyID, from, to string) ([]domain.MetricTotal, error) {
	if !idPattern.MatchString(strings.TrimSpace(companyID)) {
		return nil, ErrInvalidAnalyticsInput
	}
	if _, err := dateRange(from, to); err != nil {
		return nil, ErrInvalidAnalyticsInput
	}
	metrics, err := s.metrics.CompanyMetrics(ctx, companyID, 500)
	if err != nil {
		return nil, err
	}
	totals := make([]domain.MetricTotal, 0, len(metrics))
	for _, metric := range metrics {
		total, err := s.repository.CalculateMetricTotal(ctx, companyID, metric.ID, from, to)
		if err != nil {
			return nil, err
		}
		totals = append(totals, total)
	}
	return totals, nil
}

func (s *Service) CompanyReport(ctx context.Context, companyID, from, to string) (domain.CompanyReport, error) {
	if !idPattern.MatchString(strings.TrimSpace(companyID)) {
		return domain.CompanyReport{}, ErrInvalidAnalyticsInput
	}
	if from == "" && to == "" {
		end := time.Now().UTC()
		from = end.AddDate(0, 0, -30).Format("2006-01-02")
		to = end.Format("2006-01-02")
	}
	if _, err := dateRange(from, to); err != nil {
		return domain.CompanyReport{}, ErrInvalidAnalyticsInput
	}
	report, err := s.repository.CompanyReport(ctx, companyID, from, to)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	report.Metrics, err = s.metrics.CompanyMetrics(ctx, companyID, 500)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	return report, nil
}

func (s *Service) ConsultantContext(ctx context.Context, companyID string, limit int) (domain.ConsultantContext, error) {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	if !uuidPattern.MatchString(companyID) {
		return domain.ConsultantContext{}, ErrInvalidAnalyticsInput
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return s.repository.ConsultantContext(ctx, companyID, limit)
}

func dateRange(from, to string) (time.Time, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return time.Time{}, err
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Before(start) {
		return time.Time{}, errors.New("invalid date range")
	}
	return start, nil
}

func (s *Service) knownMetricIDs(ctx context.Context, companyID string) (map[string]struct{}, error) {
	metrics, err := s.metrics.CompanyMetrics(ctx, companyID, 500)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(metrics))
	for _, metric := range metrics {
		known[metric.ID] = struct{}{}
	}
	return known, nil
}

func probability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func clampLimit(value int) int {
	if value < 1 {
		return 100
	}
	if value > 500 {
		return 500
	}
	return value
}
