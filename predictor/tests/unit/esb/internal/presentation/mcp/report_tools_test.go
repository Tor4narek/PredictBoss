package mcp

import (
	"reflect"
	"testing"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestConsultantContextSourceIDsDeduplicatesAndNormalizes(t *testing.T) {
	context := domain.ConsultantContext{
		Facts: []domain.CompanyFact{
			{Fact: domain.Fact{SourceID: " 22222222-2222-4222-8222-222222222222 "}},
			{Fact: domain.Fact{SourceID: "22222222-2222-4222-8222-222222222222"}},
			{Fact: domain.Fact{SourceID: ""}},
		},
		Observations: []domain.MetricObservation{
			{SourceID: "33333333-3333-4333-8333-333333333333"},
			{SourceID: "22222222-2222-4222-8222-222222222222"},
		},
	}
	want := []string{"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	if got := consultantContextSourceIDs(context); !reflect.DeepEqual(got, want) {
		t.Fatalf("consultantContextSourceIDs = %#v, want %#v", got, want)
	}
}
