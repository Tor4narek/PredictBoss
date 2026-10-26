package mcp

import (
	"reflect"
	"testing"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestConsultantContextSourcesDeduplicatesAndPreservesMetadata(t *testing.T) {
	publishedAt := "2026-10-25 09:00:00.000"
	context := domain.ConsultantContext{
		Facts: []domain.CompanyFact{
			{Fact: domain.Fact{SourceID: " 22222222-2222-4222-8222-222222222222 ", NewsID: "news-1",
				SourceURL: " https://example.test/news/1 ", MaterialKind: "external_news", Title: "Headline", PublishedAt: &publishedAt}},
			{Fact: domain.Fact{SourceID: "33333333-3333-4333-8333-333333333333", NewsID: "news-2",
				MaterialKind: "alpha_analysis"}},
			{Fact: domain.Fact{SourceID: "", NewsID: "news-without-source", MaterialKind: "external_news"}},
		},
		Observations: []domain.MetricObservation{
			{SourceID: "22222222-2222-4222-8222-222222222222", NewsID: "news-1",
				SourceURL: "https://example.test/news/1", MaterialKind: "external_news", Title: "Headline", PublishedAt: &publishedAt},
			{SourceID: "not-a-uuid", NewsID: "invalid", MaterialKind: "external_news"},
		},
	}
	want := []domain.ConsultantSource{
		{SourceID: "22222222-2222-4222-8222-222222222222", NewsID: "news-1",
			SourceURL: "https://example.test/news/1", MaterialKind: "external_news", Title: "Headline", PublishedAt: &publishedAt},
		{SourceID: "33333333-3333-4333-8333-333333333333", NewsID: "news-2", MaterialKind: "alpha_analysis"},
	}
	if got := consultantContextSources(context); !reflect.DeepEqual(got, want) {
		t.Fatalf("consultantContextSources = %#v, want %#v", got, want)
	}
}

func TestConsultantContextSourcesDropsConflictingPublicationMetadata(t *testing.T) {
	context := domain.ConsultantContext{Facts: []domain.CompanyFact{
		{Fact: domain.Fact{SourceID: "22222222-2222-4222-8222-222222222222", NewsID: "news-1",
			MaterialKind: "external_news", SourceURL: "https://example.test/first"}},
		{Fact: domain.Fact{SourceID: "22222222-2222-4222-8222-222222222222", NewsID: "news-2",
			MaterialKind: "external_news", SourceURL: "https://example.test/second"}},
		{Fact: domain.Fact{SourceID: "33333333-3333-4333-8333-333333333333", NewsID: "news-3",
			MaterialKind: "external_news", SourceURL: "javascript:alert(1)"}},
	}}
	if got := consultantContextSources(context); len(got) != 0 {
		t.Fatalf("consultantContextSources returned ambiguous or unsafe publication metadata: %#v", got)
	}
}
