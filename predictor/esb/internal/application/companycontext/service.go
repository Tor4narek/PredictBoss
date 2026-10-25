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
	"sync"

	"github.com/predictboss/predictor/esb/internal/domain"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
var companyIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var ErrInvalidInput = errors.New("invalid company context input")

type Service struct {
	profiles     domain.ProfileRepository
	metrics      domain.CompanyMetricSettingsRepository
	profileLocks sync.Map
}

func NewService(profiles domain.ProfileRepository, metrics domain.CompanyMetricSettingsRepository) *Service {
	return &Service{profiles: profiles, metrics: metrics}
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
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return s.metrics.CompanyMetrics(ctx, companyID, limit)
}

func (s *Service) SaveCompanyMetrics(ctx context.Context, companyID string, metrics []domain.CompanyMetric) error {
	companyID = strings.TrimSpace(companyID)
	companyID = strings.ToLower(companyID)
	metrics, err := normalizeMetrics(companyID, metrics)
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
	lock, _ := s.profileLocks.LoadOrStore(companyID, &sync.Mutex{})
	companyLock := lock.(*sync.Mutex)
	companyLock.Lock()
	defer companyLock.Unlock()
	metrics, err := normalizeMetrics(companyID, metrics)
	if err != nil {
		return "", err
	}
	orderedMetrics := append([]domain.CompanyMetric(nil), metrics...)
	sort.Slice(orderedMetrics, func(i, j int) bool { return orderedMetrics[i].ID < orderedMetrics[j].ID })
	payload, err := json.Marshal(struct {
		Markdown string                 `json:"markdown"`
		Metrics  []domain.CompanyMetric `json:"metrics"`
	}{markdown, orderedMetrics})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	if err := s.metrics.ReplaceCompanyMetricsAtRevision(ctx, companyID, profileRevision, hex.EncodeToString(digest[:]), metrics); err != nil {
		return "", err
	}
	key, err := s.profiles.Save(ctx, companyID, markdown)
	if err != nil {
		return "", fmt.Errorf("profile revision and metric settings saved but S3 profile failed: %w", err)
	}
	if err := s.metrics.MarkCompanyProfileSynced(ctx, companyID, profileRevision, hex.EncodeToString(digest[:])); err != nil {
		return "", fmt.Errorf("S3 profile saved at %s but revision confirmation failed: %w", key, err)
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
