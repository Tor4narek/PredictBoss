package companycontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/predictboss/predictor/esb/internal/domain"
)

var companyIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var ErrInvalidInput = errors.New("invalid company context input")

type Service struct {
	profiles domain.ProfileRepository
	metrics  domain.CompanyMetricSettingsRepository
	limits   Limits
}

type Limits struct {
	DefaultPageSize      int
	MaxPageSize          int
	MaxMetricsPerRequest int
}

func NewService(profiles domain.ProfileRepository, metrics domain.CompanyMetricSettingsRepository, limits Limits) *Service {
	return &Service{profiles: profiles, metrics: metrics, limits: limits}
}

func (s *Service) CompanyProfile(ctx context.Context, companyID string) (string, string, error) {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	if !companyIDPattern.MatchString(companyID) {
		return "", "", ErrInvalidInput
	}
	return s.profiles.Get(ctx, companyID)
}

func (s *Service) CompanyMetrics(ctx context.Context, companyID string, limit int) ([]domain.CompanyMetric, error) {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	if !companyIDPattern.MatchString(companyID) {
		return nil, ErrInvalidInput
	}
	if limit < 1 {
		limit = s.limits.DefaultPageSize
	}
	if limit > s.limits.MaxPageSize {
		limit = s.limits.MaxPageSize
	}
	return s.metrics.CompanyMetrics(ctx, companyID, limit)
}

func (s *Service) SaveCompanyMetrics(ctx context.Context, companyID string, metrics []domain.CompanyMetric) error {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	metrics, err := normalizeMetrics(companyID, metrics, s.limits.MaxMetricsPerRequest)
	if err != nil {
		return err
	}
	return s.metrics.UpsertCompanyMetrics(ctx, companyID, metrics)
}

func (s *Service) ReserveProfileRevision(ctx context.Context, companyID string, revision int64, request json.RawMessage) error {
	companyID = strings.ToLower(strings.TrimSpace(companyID))
	if !companyIDPattern.MatchString(companyID) || revision < 1 || len(request) == 0 || !json.Valid(request) {
		return ErrInvalidInput
	}
	decoder := json.NewDecoder(strings.NewReader(string(request)))
	decoder.UseNumber()
	var canonical any
	if err := decoder.Decode(&canonical); err != nil {
		return ErrInvalidInput
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	return s.metrics.ReserveCompanyProfileRevision(ctx, companyID, revision, hex.EncodeToString(digest[:]))
}

// SaveCompanyProfileAndMetrics commits the authoritative revision and metric settings
// before writing the stable S3 object. Retrying the same revision repairs a partial S3
// failure without allowing an older revision to overwrite a newer profile.
func (s *Service) SaveCompanyProfileAndMetrics(ctx context.Context, companyID string, profileRevision int64, markdown string, metrics []domain.CompanyMetric) (string, error) {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	markdown = strings.TrimSpace(markdown)
	if !companyIDPattern.MatchString(companyID) || markdown == "" || profileRevision < 1 {
		return "", ErrInvalidInput
	}
	var key string
	err := s.metrics.WithCompanyProfileLock(ctx, companyID, func(lockCtx context.Context) error {
		var err error
		key, err = s.saveCompanyProfileAndMetrics(lockCtx, companyID, profileRevision, markdown, metrics)
		return err
	})
	return key, err
}

func (s *Service) saveCompanyProfileAndMetrics(ctx context.Context, companyID string, profileRevision int64, markdown string, metrics []domain.CompanyMetric) (string, error) {
	metrics, err := normalizeMetrics(companyID, metrics, s.limits.MaxMetricsPerRequest)
	if err != nil {
		return "", err
	}
	orderedMetrics := append([]domain.CompanyMetric(nil), metrics...)
	sort.Slice(orderedMetrics, func(i, j int) bool {
		leftOrder, leftKnown := domain.MetricCatalogOrder(orderedMetrics[i].TypeCode)
		rightOrder, rightKnown := domain.MetricCatalogOrder(orderedMetrics[j].TypeCode)
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if orderedMetrics[i].TypeCode != orderedMetrics[j].TypeCode {
			return orderedMetrics[i].TypeCode < orderedMetrics[j].TypeCode
		}
		return orderedMetrics[i].ID < orderedMetrics[j].ID
	})
	payload, err := json.Marshal(struct {
		Markdown string                 `json:"markdown"`
		Metrics  []domain.CompanyMetric `json:"metrics"`
	}{markdown, orderedMetrics})
	if err != nil {
		return "", fmt.Errorf("hash company profile update: %w", err)
	}
	digest := sha256.Sum256(payload)
	profileHash := hex.EncodeToString(digest[:])
	if err := s.metrics.ReplaceCompanyMetricsAtRevision(ctx, companyID, profileRevision, profileHash, metrics); err != nil {
		return "", fmt.Errorf("save company metric settings: %w", err)
	}
	key, err := s.profiles.Save(ctx, companyID, markdown)
	if err != nil {
		return "", fmt.Errorf("profile revision and metric settings saved but S3 profile failed: %w", err)
	}
	if err := s.metrics.MarkCompanyProfileSynced(ctx, companyID, profileRevision, profileHash); err != nil {
		return "", fmt.Errorf("S3 profile saved at %s but revision confirmation failed: %w", key, err)
	}
	return key, nil
}

func normalizeMetrics(companyID string, metrics []domain.CompanyMetric, maxMetrics int) ([]domain.CompanyMetric, error) {
	if !companyIDPattern.MatchString(companyID) || len(metrics) > maxMetrics {
		return nil, ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(metrics))
	for i := range metrics {
		metric := &metrics[i]
		metric.TypeCode = strings.TrimSpace(metric.TypeCode)
		metric.ID = "" // PostgreSQL owns metric row IDs; callers supply the catalog code.
		metric.Name = strings.TrimSpace(metric.Name)
		metric.Description = strings.TrimSpace(metric.Description)
		metric.Unit = strings.TrimSpace(metric.Unit)
		metric.Characteristic = strings.TrimSpace(metric.Characteristic)
		if metric.TypeCode == "" || len(metric.TypeCode) > 128 || metric.Characteristic == "" || !metric.Type.Valid() {
			return nil, fmt.Errorf("%w: metric at index %d", ErrInvalidInput, i)
		}
		metric.Name = metric.TypeCode
		if _, duplicate := seen[metric.TypeCode]; duplicate {
			return nil, fmt.Errorf("%w: duplicate type_code %q", ErrInvalidInput, metric.TypeCode)
		}
		seen[metric.TypeCode] = struct{}{}
	}
	return metrics, nil
}
