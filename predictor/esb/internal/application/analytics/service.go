package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

type Limits struct {
	FactsDefaultPageSize        int
	FactsMaxPageSize            int
	FactsMaxPeriod              time.Duration
	CompanyItemsDefaultPageSize int
	CompanyItemsMaxPageSize     int
	ConsultantDefaultPageSize   int
	ConsultantMaxPageSize       int
	CompanyReportDefaultWindow  time.Duration
	MaxFactsPerNews             int
	MaxFactCompanyLinks         int
	MaxMetricObservations       int
	MaxCompanyMetrics           int
}

type Service struct {
	repository domain.AnalyticsRepository
	metrics    domain.CompanyMetricSettingsRepository
	limits     Limits
}

type FactsRequest struct {
	CompanyID    string
	NewsID       string
	SourceID     string
	MaterialKind string
	Category     string
	Entity       string
	PeriodFrom   string
	PeriodTo     string
	BeforeFactID string
	Limit        int
}

func NewService(repository domain.AnalyticsRepository, metrics domain.CompanyMetricSettingsRepository, limits Limits) *Service {
	return &Service{repository: repository, metrics: metrics, limits: limits}
}

func (s *Service) SaveNewsAndFacts(ctx context.Context, news domain.News, facts []domain.Fact) ([]string, error) {
	news.ID = strings.TrimSpace(news.ID)
	news.SourceID = strings.TrimSpace(news.SourceID)
	news.SourceURL = strings.TrimSpace(news.SourceURL)
	news.Text = strings.TrimSpace(news.Text)
	if !idPattern.MatchString(news.ID) || news.SourceID == "" || news.Text == "" || len(facts) > s.limits.MaxFactsPerNews {
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

// FinalizeNews removes source text only after ClickHouse confirms the
// processing marker, source metadata, and complete fact set.
func (s *Service) FinalizeNews(ctx context.Context, news domain.News) (string, error) {
	news.ID = strings.TrimSpace(news.ID)
	news.SourceID = strings.TrimSpace(news.SourceID)
	if !idPattern.MatchString(news.ID) || news.SourceID == "" {
		return "", ErrInvalidAnalyticsInput
	}
	hash := strings.TrimSpace(news.TextHash)
	if hash == "" {
		if news.Text == "" {
			return "", ErrInvalidAnalyticsInput
		}
		digest := sha256.Sum256([]byte(news.Text))
		hash = hex.EncodeToString(digest[:])
	}
	if err := s.repository.FinalizeNews(ctx, news, hash); err != nil {
		return "", err
	}
	return hash, nil
}

func (s *Service) FactsByNews(ctx context.Context, newsID string, limit int) ([]domain.Fact, error) {
	if !idPattern.MatchString(strings.TrimSpace(newsID)) {
		return nil, ErrInvalidAnalyticsInput
	}
	return s.repository.FactsByNews(ctx, newsID, clampLimit(limit, s.limits.FactsDefaultPageSize, s.limits.FactsMaxPageSize))
}

func (s *Service) QueryFacts(ctx context.Context, request FactsRequest) (domain.FactPage, error) {
	filter := domain.FactQuery{
		CompanyID: strings.ToLower(strings.TrimSpace(request.CompanyID)), NewsID: strings.TrimSpace(request.NewsID),
		SourceID: strings.TrimSpace(request.SourceID), MaterialKind: strings.TrimSpace(request.MaterialKind),
		Category: strings.TrimSpace(request.Category), Entity: strings.TrimSpace(request.Entity),
		BeforeFactID: strings.ToLower(strings.TrimSpace(request.BeforeFactID)), Limit: request.Limit,
	}
	if filter.Limit < 0 || filter.Limit > s.limits.FactsMaxPageSize ||
		(filter.CompanyID != "" && !uuidPattern.MatchString(filter.CompanyID)) ||
		(filter.NewsID != "" && !idPattern.MatchString(filter.NewsID)) ||
		(filter.SourceID != "" && !idPattern.MatchString(filter.SourceID)) ||
		(filter.BeforeFactID != "" && !uuidPattern.MatchString(filter.BeforeFactID)) ||
		len(filter.Category) > 128 || len(filter.Entity) > 128 ||
		(filter.MaterialKind != "" && filter.MaterialKind != "external_news" && filter.MaterialKind != "alpha_analysis") {
		return domain.FactPage{}, ErrInvalidAnalyticsInput
	}
	if filter.Limit == 0 {
		filter.Limit = s.limits.FactsDefaultPageSize
	}
	periodFrom, periodTo := strings.TrimSpace(request.PeriodFrom), strings.TrimSpace(request.PeriodTo)
	if (periodFrom == "") != (periodTo == "") {
		return domain.FactPage{}, ErrInvalidAnalyticsInput
	}
	if periodFrom != "" {
		from, err := time.Parse(time.RFC3339Nano, periodFrom)
		if err != nil {
			return domain.FactPage{}, ErrInvalidAnalyticsInput
		}
		to, err := time.Parse(time.RFC3339Nano, periodTo)
		if err != nil || !from.Before(to) || to.Sub(from) > s.limits.FactsMaxPeriod {
			return domain.FactPage{}, ErrInvalidAnalyticsInput
		}
		filter.PeriodFrom, filter.PeriodTo = &from, &to
	}
	if filter.CompanyID == "" && filter.NewsID == "" && filter.SourceID == "" && filter.PeriodFrom == nil {
		return domain.FactPage{}, ErrInvalidAnalyticsInput
	}
	limit := filter.Limit
	filter.Limit++
	records, err := s.repository.QueryFacts(ctx, filter)
	if err != nil {
		return domain.FactPage{}, err
	}
	page := domain.FactPage{Facts: records}
	if len(records) > limit {
		page.HasMore = true
		page.Facts = records[:limit]
		page.NextBeforeID = page.Facts[len(page.Facts)-1].ID
	}
	if page.Facts == nil {
		page.Facts = []domain.FactRecord{}
	}
	return page, nil
}

func (s *Service) SaveFactCompanyLinks(ctx context.Context, companyID string, links []domain.FactCompanyLink) error {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	if !idPattern.MatchString(companyID) || len(links) > s.limits.MaxFactCompanyLinks {
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
	return s.repository.CompanyFacts(ctx, companyID, clampLimit(limit, s.limits.CompanyItemsDefaultPageSize, s.limits.CompanyItemsMaxPageSize))
}

func (s *Service) SaveMetricObservations(ctx context.Context, companyID string, observations []domain.MetricObservation) error {
	companyID = strings.TrimSpace(companyID)
	if !idPattern.MatchString(companyID) || len(observations) > s.limits.MaxMetricObservations {
		return ErrInvalidAnalyticsInput
	}
	for i := range observations {
		observation := &observations[i]
		observation.CompanyID = companyID
		observation.MetricID = strings.TrimSpace(observation.MetricID)
		observation.NewsID = strings.TrimSpace(observation.NewsID)
		if !uuidPattern.MatchString(observation.MetricID) || !idPattern.MatchString(observation.NewsID) {
			return fmt.Errorf("%w: metric observation at index %d", ErrInvalidAnalyticsInput, i)
		}
		if observation.Probability != nil && !probability(*observation.Probability) {
			return fmt.Errorf("%w: metric probability at index %d", ErrInvalidAnalyticsInput, i)
		}
		if observation.Value != nil && (math.IsNaN(*observation.Value) || math.IsInf(*observation.Value, 0)) {
			return fmt.Errorf("%w: metric value at index %d", ErrInvalidAnalyticsInput, i)
		}
		if observation.ID != "" && !uuidPattern.MatchString(observation.ID) {
			return fmt.Errorf("%w: observation id at index %d", ErrInvalidAnalyticsInput, i)
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

func (s *Service) CalculateMetricTotal(ctx context.Context, metricID, from, to string) (domain.MetricTotal, error) {
	metricID = strings.TrimSpace(metricID)
	if !uuidPattern.MatchString(metricID) {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	fromTime, toTime, err := metricPeriod(from, to)
	if err != nil {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	fromText, toText := fromTime.Format("2006-01-02 15:04:05"), toTime.Format("2006-01-02 15:04:05")
	companyID, err := s.metrics.CompanyIDForMetric(ctx, metricID)
	if err != nil {
		return domain.MetricTotal{}, err
	}
	var total domain.MetricTotal
	if toTime.Before(fromTime) {
		total = emptyMetricTotal(metricID, fromText, toText)
	} else {
		total, err = s.repository.CalculateMetricTotal(ctx, metricID, fromText, toText)
		if err != nil {
			return domain.MetricTotal{}, err
		}
	}
	total.CompanyID = companyID
	if err := s.repository.SaveMetricTotal(ctx, total); err != nil {
		return domain.MetricTotal{}, err
	}
	return total, nil
}

func (s *Service) CalculateAllMetricTotals(ctx context.Context, companyID, from, to string) (domain.CompanyMetricTotals, error) {
	companyID = strings.TrimSpace(companyID)
	if !uuidPattern.MatchString(companyID) {
		return domain.CompanyMetricTotals{}, ErrInvalidAnalyticsInput
	}
	fromTime, toTime, err := metricPeriod(from, to)
	if err != nil {
		return domain.CompanyMetricTotals{}, ErrInvalidAnalyticsInput
	}
	metrics, err := s.metrics.CompanyMetrics(ctx, companyID, s.limits.MaxCompanyMetrics)
	if err != nil {
		return domain.CompanyMetricTotals{}, err
	}
	result := domain.CompanyMetricTotals{CompanyID: companyID, PeriodFrom: fromTime.Format("2006-01-02 15:04:05"), PeriodTo: toTime.Format("2006-01-02 15:04:05"), Items: make([]domain.CompanyMetricTotal, 0, len(metrics))}
	for _, metric := range metrics {
		var total domain.MetricTotal
		if toTime.Before(fromTime) {
			total = emptyMetricTotal(metric.ID, result.PeriodFrom, result.PeriodTo)
		} else {
			total, err = s.repository.CalculateMetricTotal(ctx, metric.ID, result.PeriodFrom, result.PeriodTo)
			if err != nil {
				return domain.CompanyMetricTotals{}, err
			}
		}
		total.CompanyID = companyID
		if err := s.repository.SaveMetricTotal(ctx, total); err != nil {
			return domain.CompanyMetricTotals{}, err
		}
		result.Items = append(result.Items, domain.CompanyMetricTotal{MetricID: metric.ID, TypeCode: metric.TypeCode,
			Characteristic: metric.Characteristic, TotalValue: total.TotalValue, TotalProbability: total.TotalProbability,
			FactCount: total.FactCount, ProbabilityCount: total.ProbabilityCount, Links: total.Links})
	}
	return result, nil
}

func emptyMetricTotal(metricID, from, to string) domain.MetricTotal {
	return domain.MetricTotal{MetricID: metricID, PeriodFrom: from, PeriodTo: to, Links: []domain.MetricLink{}}
}

func (s *Service) CompanyReport(ctx context.Context, companyID, from, to string) (domain.CompanyReport, error) {
	if !idPattern.MatchString(strings.TrimSpace(companyID)) {
		return domain.CompanyReport{}, ErrInvalidAnalyticsInput
	}
	if from == "" && to == "" {
		end := time.Now().UTC()
		from = end.Add(-s.limits.CompanyReportDefaultWindow).Format("2006-01-02")
		to = end.Format("2006-01-02")
	}
	if _, err := dateRange(from, to); err != nil {
		return domain.CompanyReport{}, ErrInvalidAnalyticsInput
	}
	report, err := s.repository.CompanyReport(ctx, companyID, from, to)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	report.Metrics, err = s.metrics.CompanyMetrics(ctx, companyID, s.limits.MaxCompanyMetrics)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	calculated, err := s.CalculateAllMetricTotals(ctx, companyID, from, to)
	if err != nil {
		return domain.CompanyReport{}, err
	}
	report.Totals = calculated.Items
	return report, nil
}

func (s *Service) ConsultantContext(ctx context.Context, companyID string, limit int) (domain.ConsultantContext, error) {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	if !uuidPattern.MatchString(companyID) {
		return domain.ConsultantContext{}, ErrInvalidAnalyticsInput
	}
	limit = clampLimit(limit, s.limits.ConsultantDefaultPageSize, s.limits.ConsultantMaxPageSize)
	result, err := s.repository.ConsultantContext(ctx, companyID, limit)
	if err != nil {
		return domain.ConsultantContext{}, err
	}
	metrics, err := s.CalculateAllMetricTotals(ctx, companyID, "1970-01-01", time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		return domain.ConsultantContext{}, err
	}
	result.Totals = metrics.Items
	return result, nil
}

// metricPeriod accepts inclusive second-precision timestamps and date-only bounds.
// Date-only start bounds mean midnight; date-only end bounds mean the final second.
// Reversed intervals are valid and deliberately produce an empty aggregate.
func metricPeriod(from, to string) (time.Time, time.Time, error) {
	start, err := parseMetricBound(from, false)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseMetricBound(to, true)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}

func parseMetricBound(value string, endOfDate bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		if endOfDate {
			return parsed.Add(24*time.Hour - time.Second), nil
		}
		return parsed, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid metric timestamp")
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
	metrics, err := s.metrics.CompanyMetrics(ctx, companyID, s.limits.MaxCompanyMetrics)
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

func clampLimit(value, defaultValue, maxValue int) int {
	if value < 1 {
		return defaultValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
