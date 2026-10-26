package companycontext

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type profileRepositoryFake struct {
	domain.ProfileRepository
	markdown string
	key      string
	saveErr  error
	saves    int
}

func (r *profileRepositoryFake) Get(context.Context, string) (string, string, error) {
	return r.markdown, r.key, nil
}

func (r *profileRepositoryFake) Save(_ context.Context, companyID, markdown string) (string, error) {
	r.saves++
	r.markdown = markdown
	r.key = "companies/" + companyID + "/profile.md"
	return r.key, r.saveErr
}

type metricSettingsRepositoryFake struct {
	domain.CompanyMetricSettingsRepository
	lockErr        error
	lockCalls      int
	companyID      string
	queryCompanyID string
	queryLimit     int
	listedMetrics  []domain.CompanyMetric
	revision       int64
	profileHash    string
	metrics        []domain.CompanyMetric
	replaceErr     error
	replaceCalls   int
	reservedHash   string
	reserveCalls   int
	syncedRevision int64
	syncedHash     string
	syncCalls      int
}

func (r *metricSettingsRepositoryFake) WithCompanyProfileLock(ctx context.Context, companyID string, work func(context.Context) error) error {
	r.lockCalls++
	if r.lockErr != nil {
		return r.lockErr
	}
	return work(ctx)
}

func (r *metricSettingsRepositoryFake) ReplaceCompanyMetricsAtRevision(_ context.Context, companyID string, revision int64, hash string, metrics []domain.CompanyMetric) error {
	r.replaceCalls++
	r.companyID, r.revision, r.profileHash, r.metrics = companyID, revision, hash, metrics
	return r.replaceErr
}

func (r *metricSettingsRepositoryFake) MarkCompanyProfileSynced(_ context.Context, companyID string, revision int64, hash string) error {
	r.syncCalls++
	r.companyID, r.syncedRevision, r.syncedHash = companyID, revision, hash
	return nil
}

func (r *metricSettingsRepositoryFake) ReserveCompanyProfileRevision(_ context.Context, _ string, _ int64, hash string) error {
	r.reserveCalls++
	r.reservedHash = hash
	return nil
}

func (r *metricSettingsRepositoryFake) CompanyMetrics(_ context.Context, companyID string, limit int) ([]domain.CompanyMetric, error) {
	r.queryCompanyID, r.queryLimit = companyID, limit
	return r.listedMetrics, nil
}

func TestNormalizeMetricsPreservesMeasurementUnit(t *testing.T) {
	metrics, err := normalizeMetrics("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", []domain.CompanyMetric{{
		TypeCode: "цена", Type: domain.MetricCurrency, Characteristic: "запчастей", Unit: "₽/шт.",
	}}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 || metrics[0].Unit != "₽/шт." {
		t.Fatalf("metric measurement unit = %#v, want ₽/шт.", metrics)
	}
}

func TestSaveCompanyProfileAndMetricsCanRetryAfterS3Failure(t *testing.T) {
	companyID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	profiles := &profileRepositoryFake{saveErr: errors.New("MinIO temporarily unavailable")}
	metrics := &metricSettingsRepositoryFake{}
	service := NewService(profiles, metrics, Limits{MaxMetricsPerRequest: 10})
	companyMetrics := []domain.CompanyMetric{{TypeCode: "цена", Type: domain.MetricCurrency, Characteristic: "сырья", Unit: "₽/тонна"}}

	if _, err := service.SaveCompanyProfileAndMetrics(context.Background(), companyID, 7, "# Company", companyMetrics); err == nil {
		t.Fatal("profile save unexpectedly succeeded while S3 was unavailable")
	}
	if metrics.replaceCalls != 1 || metrics.syncCalls != 0 || profiles.saves != 1 {
		t.Fatalf("partial save state is incorrect: replace=%d synced=%d S3=%d", metrics.replaceCalls, metrics.syncCalls, profiles.saves)
	}
	if got := metrics.metrics[0].Unit; got != "₽/тонна" {
		t.Fatalf("metric unit was not passed to PostgreSQL: %q", got)
	}

	profiles.saveErr = nil
	key, err := service.SaveCompanyProfileAndMetrics(context.Background(), companyID, 7, "# Company", companyMetrics)
	if err != nil {
		t.Fatalf("retry did not repair the partial S3 write: %v", err)
	}
	if key != "companies/"+companyID+"/profile.md" || metrics.replaceCalls != 2 || metrics.syncCalls != 1 || profiles.saves != 2 {
		t.Fatalf("retry did not complete the same revision idempotently: key=%q replace=%d synced=%d S3=%d", key, metrics.replaceCalls, metrics.syncCalls, profiles.saves)
	}
}

func TestReserveProfileRevisionUsesCanonicalRequestHash(t *testing.T) {
	metrics := &metricSettingsRepositoryFake{}
	service := NewService(&profileRepositoryFake{}, metrics, Limits{MaxMetricsPerRequest: 10})
	companyID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	for _, body := range []string{`{"company":{"name":"Acme","active":true}}`, `{"company":{"active":true,"name":"Acme"}}`} {
		if err := service.ReserveProfileRevision(context.Background(), companyID, 3, []byte(body)); err != nil {
			t.Fatalf("ReserveProfileRevision returned error: %v", err)
		}
		if len(metrics.reservedHash) != 64 {
			t.Fatalf("request hash length = %d, want SHA-256 hex", len(metrics.reservedHash))
		}
		if metrics.reserveCalls == 1 {
			metrics.profileHash = metrics.reservedHash
		} else if metrics.profileHash != metrics.reservedHash {
			t.Fatalf("JSON key order changed canonical request hash: %q != %q", metrics.profileHash, metrics.reservedHash)
		}
	}
	if err := service.ReserveProfileRevision(context.Background(), companyID, 0, []byte(`{}`)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero revision error = %v, want invalid input", err)
	}
}

func TestCompanyMetricsClampsLimitAndNormalizesCompanyID(t *testing.T) {
	settings := &metricSettingsRepositoryFake{listedMetrics: []domain.CompanyMetric{{TypeCode: "цена"}}}
	service := NewService(&profileRepositoryFake{}, settings, Limits{DefaultPageSize: 5, MaxPageSize: 10})
	_, err := service.CompanyMetrics(context.Background(), " AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA ", 100)
	if err != nil {
		t.Fatal(err)
	}
	if settings.queryCompanyID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || settings.queryLimit != 10 || settings.lockCalls != 0 {
		t.Fatalf("metric read did not normalize company and clamp limit: company=%q limit=%d lock_calls=%d", settings.queryCompanyID, settings.queryLimit, settings.lockCalls)
	}
}

func TestNormalizeMetricsRejectsDuplicateCatalogCodes(t *testing.T) {
	companyID := strings.ToLower("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA")
	_, err := normalizeMetrics(companyID, []domain.CompanyMetric{
		{TypeCode: "цена", Type: domain.MetricCurrency, Characteristic: "сырья"},
		{TypeCode: " цена ", Type: domain.MetricCurrency, Characteristic: "оборудования"},
	}, 10)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate metric type_code error = %v, want invalid input", err)
	}
}
