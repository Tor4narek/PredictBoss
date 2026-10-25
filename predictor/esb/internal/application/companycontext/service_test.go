package companycontext

import (
	"testing"

	"github.com/predictboss/predictor/esb/internal/domain"
)

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
