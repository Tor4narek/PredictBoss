package companycontext

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/predictboss/predictor/esb/internal/domain"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

var ErrInvalidInput = errors.New("invalid company context input")

type Service struct {
	profiles domain.ProfileRepository
	metrics  domain.CompanyMetricSettingsRepository
}

func NewService(profiles domain.ProfileRepository, metrics domain.CompanyMetricSettingsRepository) *Service {
	return &Service{profiles: profiles, metrics: metrics}
}

func (s *Service) CompanyProfile(ctx context.Context, companyID string) (string, string, error) {
	companyID = strings.TrimSpace(companyID)
	if !idPattern.MatchString(companyID) {
		return "", "", ErrInvalidInput
	}
	return s.profiles.Get(ctx, companyID)
}

func (s *Service) CompanyMetrics(ctx context.Context, companyID string, limit int) ([]domain.CompanyMetric, error) {
	companyID = strings.TrimSpace(companyID)
	if !idPattern.MatchString(companyID) {
		return nil, ErrInvalidInput
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return s.metrics.CompanyMetrics(ctx, companyID, limit)
}

func (s *Service) SaveCompanyMetrics(ctx context.Context, companyID string, metrics []domain.CompanyMetric) error {
	companyID = strings.TrimSpace(companyID)
	metrics, err := normalizeMetrics(companyID, metrics)
	if err != nil {
		return err
	}
	return s.metrics.UpsertCompanyMetrics(ctx, companyID, metrics)
}

// SaveCompanyProfileAndMetrics writes the profile first and settings in one PostgreSQL
// transaction. Both writes are repeatable: S3 overwrites the stable profile key and
// PostgreSQL upserts by (company_id, metric_id), so retrying after a partial failure is safe.
func (s *Service) SaveCompanyProfileAndMetrics(ctx context.Context, companyID, markdown string, metrics []domain.CompanyMetric) (string, error) {
	companyID = strings.TrimSpace(companyID)
	markdown = strings.TrimSpace(markdown)
	if !idPattern.MatchString(companyID) || markdown == "" {
		return "", ErrInvalidInput
	}
	metrics, err := normalizeMetrics(companyID, metrics)
	if err != nil {
		return "", err
	}
	key, err := s.profiles.Save(ctx, companyID, markdown)
	if err != nil {
		return "", err
	}
	if err := s.metrics.UpsertCompanyMetrics(ctx, companyID, metrics); err != nil {
		return "", fmt.Errorf("profile saved at %s but metric settings failed: %w", key, err)
	}
	return key, nil
}

func normalizeMetrics(companyID string, metrics []domain.CompanyMetric) ([]domain.CompanyMetric, error) {
	if !idPattern.MatchString(companyID) || len(metrics) > 500 {
		return nil, ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(metrics))
	for i := range metrics {
		metric := &metrics[i]
		metric.ID = strings.TrimSpace(metric.ID)
		metric.Name = strings.TrimSpace(metric.Name)
		metric.Description = strings.TrimSpace(metric.Description)
		metric.Unit = strings.TrimSpace(metric.Unit)
		metric.Characteristic = strings.TrimSpace(metric.Characteristic)
		if !idPattern.MatchString(metric.ID) || metric.Name == "" || metric.Characteristic == "" || !metric.Type.Valid() {
			return nil, fmt.Errorf("%w: metric at index %d", ErrInvalidInput, i)
		}
		if _, duplicate := seen[metric.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate metric_id %q", ErrInvalidInput, metric.ID)
		}
		seen[metric.ID] = struct{}{}
	}
	return metrics, nil
}
