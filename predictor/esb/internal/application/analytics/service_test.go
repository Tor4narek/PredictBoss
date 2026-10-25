package analytics

import (
	"context"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type metricTotalRepositoryFake struct {
	domain.AnalyticsRepository
	calculated []string
	saved      []domain.MetricTotal
}

func (r *metricTotalRepositoryFake) CalculateMetricTotal(_ context.Context, metricID, from, to string) (domain.MetricTotal, error) {
	r.calculated = append(r.calculated, metricID)
	value := float64(len(r.calculated))
	return domain.MetricTotal{MetricID: metricID, PeriodFrom: from, PeriodTo: to, TotalValue: &value, Links: []domain.MetricLink{}}, nil
}

func (r *metricTotalRepositoryFake) SaveMetricTotal(_ context.Context, total domain.MetricTotal) error {
	r.saved = append(r.saved, total)
	return nil
}

type metricSettingsFake struct {
	domain.CompanyMetricSettingsRepository
	metrics   []domain.CompanyMetric
	metric    domain.CompanyMetric
	companyID string
}

func testLimits() Limits {
	return Limits{
		FactsDefaultPageSize: 100, FactsMaxPageSize: 100, FactsMaxPeriod: 90 * 24 * time.Hour,
		CompanyItemsDefaultPageSize: 100, CompanyItemsMaxPageSize: 500,
		ConsultantDefaultPageSize: 100, ConsultantMaxPageSize: 500,
		CompanyReportDefaultWindow: 30 * 24 * time.Hour,
		MaxFactsPerNews:            500, MaxFactCompanyLinks: 1000, MaxMetricObservations: 1000, MaxCompanyMetrics: 500,
	}
}

func (r *metricSettingsFake) CompanyMetrics(context.Context, string, int) ([]domain.CompanyMetric, error) {
	return r.metrics, nil
}

func (r *metricSettingsFake) MetricForID(context.Context, string) (domain.CompanyMetric, string, error) {
	return r.metric, r.companyID, nil
}

func TestCalculateMetricTotalPersistsCompanyOwnedSnapshot(t *testing.T) {
	repository := &metricTotalRepositoryFake{}
	service := NewService(repository, &metricSettingsFake{
		companyID: "company-1", metric: domain.CompanyMetric{Unit: "₽/месяц"},
	}, testLimits())

	metricID := "11111111-1111-4111-8111-111111111111"
	total, err := service.CalculateMetricTotal(context.Background(), metricID, "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatalf("CalculateMetricTotal returned error: %v", err)
	}
	if total.CompanyID != "company-1" || total.Unit != "₽/месяц" || len(repository.saved) != 1 || repository.saved[0].MetricID != metricID {
		t.Fatalf("metric total was not persisted with its PostgreSQL owner: total=%#v saved=%#v", total, repository.saved)
	}
}

func TestCalculateAllMetricTotalsPersistsEmptyAndPopulatedRows(t *testing.T) {
	repository := &metricTotalRepositoryFake{}
	settings := &metricSettingsFake{metrics: []domain.CompanyMetric{
		{ID: "11111111-1111-4111-8111-111111111111", TypeCode: "ставка", Characteristic: "лизинга", Unit: "% годовых"},
		{ID: "22222222-2222-4222-8222-222222222222", TypeCode: "цена", Characteristic: "топлива"},
	}}
	service := NewService(repository, settings, testLimits())

	totals, err := service.CalculateAllMetricTotals(context.Background(), "22222222-2222-4222-8222-222222222222", "2026-09-30", "2026-09-01")
	if err != nil {
		t.Fatalf("CalculateAllMetricTotals returned error: %v", err)
	}
	if len(totals.Items) != 2 || len(repository.saved) != 2 || len(repository.calculated) != 0 {
		t.Fatalf("reversed period did not persist both empty rows: totals=%#v saved=%#v calculated=%#v", totals, repository.saved, repository.calculated)
	}
	if totals.Items[0].Unit != "% годовых" {
		t.Fatalf("metric total did not preserve its PostgreSQL measurement unit: %#v", totals.Items[0])
	}
	for _, total := range repository.saved {
		if total.CompanyID != "22222222-2222-4222-8222-222222222222" || total.TotalValue != nil || total.FactCount != 0 {
			t.Errorf("empty ClickHouse snapshot has incorrect data: %#v", total)
		}
	}
}
