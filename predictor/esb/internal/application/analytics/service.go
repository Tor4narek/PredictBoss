package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
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
	MaxNewsTextBytes            int64
}

const (
	maxFactTextBytes           = 4096
	maxFactEvidenceBytes       = 2048
	maxFactCategoryBytes       = 128
	maxFactEntityBytes         = 256
	maxFactEntities            = 64
	maxLinkRelevanceBytes      = 1024
	maxAnalysisTextBytes       = 32 << 10
	maxSourceFactIDsPerRow     = 100
	maxSourceFactIDsPerRequest = 10000
)

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
	if !idPattern.MatchString(news.ID) {
		return nil, fmt.Errorf("%w: news_id must contain 1 to 128 letters, digits, dots, underscores, or hyphens", ErrInvalidAnalyticsInput)
	}
	news.SourceID = strings.ToLower(news.SourceID)
	if !uuidPattern.MatchString(news.SourceID) {
		return nil, fmt.Errorf("%w: source_id must be a UUID", ErrInvalidAnalyticsInput)
	}
	if news.Text == "" || (s.limits.MaxNewsTextBytes > 0 && int64(len(news.Text)) > s.limits.MaxNewsTextBytes) {
		return nil, fmt.Errorf("%w: news_text is empty or exceeds the configured limit", ErrInvalidAnalyticsInput)
	}
	if len(news.Title) > 512 || len(news.SourceURL) > 4096 || !validSourceURL(news.SourceURL) {
		return nil, fmt.Errorf("%w: source metadata is invalid", ErrInvalidAnalyticsInput)
	}
	if len(facts) > s.limits.MaxFactsPerNews {
		return nil, fmt.Errorf("%w: facts contains more than %d items", ErrInvalidAnalyticsInput, s.limits.MaxFactsPerNews)
	}
	if news.CompanyID != "" && !uuidPattern.MatchString(news.CompanyID) {
		return nil, fmt.Errorf("%w: company_id must be a UUID when provided", ErrInvalidAnalyticsInput)
	}
	if news.MaterialKind != "" && news.MaterialKind != "external_news" && news.MaterialKind != "alpha_analysis" {
		return nil, fmt.Errorf("%w: material_kind must be external_news or alpha_analysis", ErrInvalidAnalyticsInput)
	}
	if news.MediaType != "" && news.MediaType != "application/pdf" && news.MediaType != "text/html" {
		return nil, fmt.Errorf("%w: media_type must be text/html or application/pdf", ErrInvalidAnalyticsInput)
	}
	if news.PublishedAt != nil {
		if _, err := time.Parse(time.RFC3339, *news.PublishedAt); err != nil {
			return nil, fmt.Errorf("%w: published_at", ErrInvalidAnalyticsInput)
		}
	}
	for i := range facts {
		facts[i].Text = strings.TrimSpace(facts[i].Text)
		facts[i].Evidence = strings.TrimSpace(facts[i].Evidence)
		facts[i].Category = strings.TrimSpace(facts[i].Category)
		if facts[i].Text == "" || len(facts[i].Text) > maxFactTextBytes ||
			facts[i].Evidence == "" || len(facts[i].Evidence) > maxFactEvidenceBytes || !strings.Contains(news.Text, facts[i].Evidence) ||
			len(facts[i].Category) > maxFactCategoryBytes || len(facts[i].Entities) > maxFactEntities || !probability(facts[i].Confidence) {
			return nil, fmt.Errorf("%w: fact at index %d", ErrInvalidAnalyticsInput, i)
		}
		for _, entity := range facts[i].Entities {
			if strings.TrimSpace(entity) == "" || len(entity) > maxFactEntityBytes {
				return nil, fmt.Errorf("%w: fact entity at index %d", ErrInvalidAnalyticsInput, i)
			}
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
	news.SourceID = strings.ToLower(strings.TrimSpace(news.SourceID))
	if !idPattern.MatchString(news.ID) || !uuidPattern.MatchString(news.SourceID) {
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
	companyID = strings.ToLower(strings.TrimSpace(companyID))
	if !uuidPattern.MatchString(companyID) || len(links) > s.limits.MaxFactCompanyLinks {
		return ErrInvalidAnalyticsInput
	}
	factIDs := make([]string, 0, len(links))
	seen := make(map[string]struct{}, len(links))
	for i := range links {
		links[i].FactID = strings.ToLower(strings.TrimSpace(links[i].FactID))
		links[i].Relevance = strings.TrimSpace(links[i].Relevance)
		if !uuidPattern.MatchString(links[i].FactID) || links[i].Relevance == "" || len(links[i].Relevance) > maxLinkRelevanceBytes || !probability(links[i].Confidence) {
			return fmt.Errorf("%w: fact-company link at index %d", ErrInvalidAnalyticsInput, i)
		}
		if _, duplicate := seen[links[i].FactID]; duplicate {
			return fmt.Errorf("%w: duplicate fact-company link at index %d", ErrInvalidAnalyticsInput, i)
		}
		seen[links[i].FactID] = struct{}{}
		factIDs = append(factIDs, links[i].FactID)
	}
	references, err := s.repository.FactReferences(ctx, factIDs)
	if err != nil {
		return err
	}
	persistedIDs := stringSetFactReferences(references)
	for _, factID := range factIDs {
		if _, exists := persistedIDs[factID]; !exists {
			return fmt.Errorf("%w: one or more fact IDs are not persisted", ErrInvalidAnalyticsInput)
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
	companyID = strings.ToLower(strings.TrimSpace(companyID))
	if !uuidPattern.MatchString(companyID) || len(observations) > s.limits.MaxMetricObservations {
		return ErrInvalidAnalyticsInput
	}
	newsIDs := make([]string, 0, len(observations))
	factIDs := make([]string, 0)
	seenNews := make(map[string]struct{}, len(observations))
	seenFacts := make(map[string]struct{})
	for i := range observations {
		observation := &observations[i]
		if observation.CompanyID != "" && !strings.EqualFold(strings.TrimSpace(observation.CompanyID), companyID) {
			return fmt.Errorf("%w: observation at index %d has a mismatched company_id", ErrInvalidAnalyticsInput, i)
		}
		observation.CompanyID = companyID
		observation.MetricID = strings.ToLower(strings.TrimSpace(observation.MetricID))
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
		seenObservationFacts := make(map[string]struct{}, len(observation.SourceFactIDs))
		for factIndex, factID := range observation.SourceFactIDs {
			factID = strings.ToLower(strings.TrimSpace(factID))
			if !uuidPattern.MatchString(factID) {
				return fmt.Errorf("%w: source fact id at index %d", ErrInvalidAnalyticsInput, i)
			}
			if _, duplicate := seenObservationFacts[factID]; duplicate {
				return fmt.Errorf("%w: duplicate source fact id at index %d", ErrInvalidAnalyticsInput, i)
			}
			seenObservationFacts[factID] = struct{}{}
			observation.SourceFactIDs[factIndex] = factID
			if _, queried := seenFacts[factID]; !queried {
				if len(factIDs) >= maxSourceFactIDsPerRequest {
					return fmt.Errorf("%w: request has too many distinct source fact IDs", ErrInvalidAnalyticsInput)
				}
				seenFacts[factID] = struct{}{}
				factIDs = append(factIDs, factID)
			}
		}
		if len(observation.SourceFactIDs) > maxSourceFactIDsPerRow {
			return fmt.Errorf("%w: observation at index %d has too many source facts", ErrInvalidAnalyticsInput, i)
		}
		if observation.SourceID != "" && !uuidPattern.MatchString(observation.SourceID) {
			return fmt.Errorf("%w: observation source_id at index %d must be a UUID", ErrInvalidAnalyticsInput, i)
		}
		observation.SourceID = strings.ToLower(observation.SourceID)
		if observation.SourceURL != "" && !validSourceURL(observation.SourceURL) {
			return fmt.Errorf("%w: observation source_url at index %d is invalid", ErrInvalidAnalyticsInput, i)
		}
		if observation.MaterialKind != "" && observation.MaterialKind != "external_news" && observation.MaterialKind != "alpha_analysis" {
			return fmt.Errorf("%w: observation material_kind at index %d is invalid", ErrInvalidAnalyticsInput, i)
		}
		if _, exists := seenNews[observation.NewsID]; !exists {
			seenNews[observation.NewsID] = struct{}{}
			newsIDs = append(newsIDs, observation.NewsID)
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
	if err := s.validateObservationReferences(ctx, companyID, observations, newsIDs, factIDs); err != nil {
		return err
	}
	return s.repository.SaveMetricObservations(ctx, companyID, observations)
}

func (s *Service) validateObservationReferences(ctx context.Context, companyID string,
	observations []domain.MetricObservation, newsIDs, factIDs []string) error {
	newsReferences, err := s.repository.NewsReferences(ctx, newsIDs)
	if err != nil {
		return err
	}
	newsByID := make(map[string]domain.NewsReference, len(newsReferences))
	for _, reference := range newsReferences {
		newsByID[reference.ID] = reference
	}
	if len(newsByID) != len(newsIDs) {
		return fmt.Errorf("%w: one or more observation news IDs are not persisted", ErrInvalidAnalyticsInput)
	}

	companyNewsIDs, err := s.repository.CompanyNewsIDs(ctx, companyID, newsIDs)
	if err != nil {
		return err
	}
	companyNews := stringSet(companyNewsIDs)

	factReferences, err := s.repository.FactReferences(ctx, factIDs)
	if err != nil {
		return err
	}
	factsByID := make(map[string]domain.FactReference, len(factReferences))
	for _, reference := range factReferences {
		factsByID[reference.ID] = reference
	}
	if len(factsByID) != len(factIDs) {
		return fmt.Errorf("%w: one or more observation fact IDs are not persisted", ErrInvalidAnalyticsInput)
	}
	companyFactIDs, err := s.repository.CompanyFactIDs(ctx, companyID, factIDs)
	if err != nil {
		return err
	}
	companyFacts := stringSet(companyFactIDs)

	for i := range observations {
		observation := &observations[i]
		news, exists := newsByID[observation.NewsID]
		if !exists {
			return fmt.Errorf("%w: news_id at observation %d is not persisted", ErrInvalidAnalyticsInput, i)
		}
		if _, linked := companyNews[observation.NewsID]; !linked {
			return fmt.Errorf("%w: news_id at observation %d is not linked to the requested company", ErrInvalidAnalyticsInput, i)
		}
		if observation.SourceID != "" && observation.SourceID != news.SourceID {
			return fmt.Errorf("%w: source_id at observation %d does not match its news", ErrInvalidAnalyticsInput, i)
		}
		if observation.SourceURL != "" && observation.SourceURL != news.SourceURL {
			return fmt.Errorf("%w: source_url at observation %d does not match its news", ErrInvalidAnalyticsInput, i)
		}
		if observation.MaterialKind != "" && observation.MaterialKind != news.MaterialKind {
			return fmt.Errorf("%w: material_kind at observation %d does not match its news", ErrInvalidAnalyticsInput, i)
		}
		observation.SourceID = news.SourceID
		observation.SourceURL = news.SourceURL
		observation.MaterialKind = news.MaterialKind
		for _, factID := range observation.SourceFactIDs {
			fact, exists := factsByID[factID]
			if !exists || fact.NewsID != observation.NewsID {
				return fmt.Errorf("%w: source fact at observation %d does not belong to its news", ErrInvalidAnalyticsInput, i)
			}
			if _, linked := companyFacts[factID]; !linked {
				return fmt.Errorf("%w: source fact at observation %d is not linked to the requested company", ErrInvalidAnalyticsInput, i)
			}
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func stringSetFactReferences(references []domain.FactReference) map[string]struct{} {
	result := make(map[string]struct{}, len(references))
	for _, reference := range references {
		result[reference.ID] = struct{}{}
	}
	return result
}

func (s *Service) SaveCompanyAnalysis(ctx context.Context, analysis domain.CompanyAnalysis) error {
	analysis.CompanyID = strings.TrimSpace(analysis.CompanyID)
	analysis.Text = strings.TrimSpace(analysis.Text)
	analysis.CompanyID = strings.ToLower(analysis.CompanyID)
	if !uuidPattern.MatchString(analysis.CompanyID) || analysis.Text == "" || len(analysis.Text) > maxAnalysisTextBytes || !probability(analysis.Probability) {
		return ErrInvalidAnalyticsInput
	}
	if _, err := dateRange(analysis.PeriodFrom, analysis.PeriodTo); err != nil {
		return ErrInvalidAnalyticsInput
	}
	seenMetrics := make(map[string]struct{}, len(analysis.MetricIDs))
	for i, metricID := range analysis.MetricIDs {
		metricID = strings.ToLower(strings.TrimSpace(metricID))
		analysis.MetricIDs[i] = metricID
		if !uuidPattern.MatchString(metricID) {
			return ErrInvalidAnalyticsInput
		}
		if _, duplicate := seenMetrics[metricID]; duplicate {
			return ErrInvalidAnalyticsInput
		}
		seenMetrics[metricID] = struct{}{}
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
	companyID = strings.TrimSpace(companyID)
	metricID = strings.TrimSpace(metricID)
	if !uuidPattern.MatchString(companyID) || !uuidPattern.MatchString(metricID) {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	fromTime, toTime, err := metricPeriod(from, to)
	if err != nil {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
	}
	fromText, toText := fromTime.Format("2006-01-02 15:04:05"), toTime.Format("2006-01-02 15:04:05")
	metric, metricCompanyID, err := s.metrics.MetricForID(ctx, metricID)
	if err != nil {
		return domain.MetricTotal{}, err
	}
	if !strings.EqualFold(metricCompanyID, companyID) {
		return domain.MetricTotal{}, ErrInvalidAnalyticsInput
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
	total.CompanyID = metricCompanyID
	total.Unit = metric.Unit
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
			Characteristic: metric.Characteristic, Unit: metric.Unit,
			TotalValue: total.TotalValue, TotalProbability: total.TotalProbability,
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

func validSourceURL(raw string) bool {
	if raw == "" {
		return true
	}
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil
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
