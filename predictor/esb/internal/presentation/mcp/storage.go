package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func NewStorageHandler(service *analytics.Service, companyContext *companycontext.Service, apiKey string) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "predictor-esb", Version: "0.3.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{Name: "get_company_profile", Description: "Read the company's Markdown profile from S3 through the ESB."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyProfileInput) (*mcp.CallToolResult, companyProfileOutput, error) {
		markdown, key, err := companyContext.CompanyProfile(ctx, input.CompanyID)
		if err != nil {
			return nil, companyProfileOutput{}, err
		}
		return nil, companyProfileOutput{CompanyID: strings.TrimSpace(input.CompanyID), Key: key, Markdown: markdown}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_company_profile_and_metrics", Description: "Idempotently save the company Markdown profile to S3 and upsert its structured metric settings in PostgreSQL through the ESB."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveCompanyContextInput) (*mcp.CallToolResult, companyContextOutput, error) {
		key, err := companyContext.SaveCompanyProfileAndMetrics(ctx, input.CompanyID, input.ProfileRevision, input.Markdown, input.Metrics)
		if err != nil {
			return nil, companyContextOutput{}, err
		}
		return nil, companyContextOutput{CompanyID: strings.TrimSpace(input.CompanyID), ProfileRevision: input.ProfileRevision, ProfileKey: key, MetricsSaved: len(input.Metrics)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_news_and_facts", Description: "Store the submitted news and the company-independent facts extracted from it in ClickHouse."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveNewsFactsInput) (*mcp.CallToolResult, saveNewsFactsOutput, error) {
		if strings.TrimSpace(input.NewsID) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.NewsText) == "" {
			return nil, saveNewsFactsOutput{}, fmt.Errorf("news_id, source_id and news_text are required")
		}
		facts := make([]domain.Fact, 0, len(input.Facts))
		for _, fact := range input.Facts {
			facts = append(facts, domain.Fact{Text: fact.FactText, Category: fact.Category, Entities: fact.Entities, Evidence: fact.Evidence, OccurredAt: fact.OccurredAt, Confidence: fact.Confidence})
		}
		ids, err := service.SaveNewsAndFacts(ctx, domain.News{ID: input.NewsID, SourceID: input.SourceID, SourceURL: input.SourceURL,
			CompanyID: input.CompanyID, MaterialKind: input.MaterialKind, MediaType: input.MediaType, Title: input.Title,
			PublishedAt: input.PublishedAt, Text: input.NewsText}, facts)
		if err != nil {
			return nil, saveNewsFactsOutput{}, err
		}
		return nil, saveNewsFactsOutput{NewsID: input.NewsID, Inserted: len(ids), FactIDs: ids}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_facts", Description: "Read universal facts for a news_id from ClickHouse."}, func(ctx context.Context, _ *mcp.CallToolRequest, input newsFactsInput) (*mcp.CallToolResult, factsOutput, error) {
		facts, err := service.FactsByNews(ctx, input.NewsID, input.Limit)
		if err != nil {
			return nil, factsOutput{}, err
		}
		return nil, factsOutput{Facts: facts}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_fact_company_links", Description: "Persist Savely's supported links between universal facts and a company."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyFactLinksInput) (*mcp.CallToolResult, insertedOutput, error) {
		if err := service.SaveFactCompanyLinks(ctx, input.CompanyID, input.Links); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(input.Links)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_company_facts", Description: "Read universal facts that have already been linked to a company."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, companyFactsOutput, error) {
		facts, err := service.CompanyFacts(ctx, input.CompanyID, input.Limit)
		if err != nil {
			return nil, companyFactsOutput{}, err
		}
		return nil, companyFactsOutput{Facts: facts}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_company_metrics", Description: "Idempotently upsert structured metric settings for a company in PostgreSQL through the ESB."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyMetricsInput) (*mcp.CallToolResult, insertedOutput, error) {
		if err := companyContext.SaveCompanyMetrics(ctx, input.CompanyID, input.Metrics); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(input.Metrics)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_company_metrics", Description: "Read the company's structured metric settings from PostgreSQL through the ESB."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, companyMetricsOutput, error) {
		metrics, err := companyContext.CompanyMetrics(ctx, input.CompanyID, input.Limit)
		if err != nil {
			return nil, companyMetricsOutput{}, err
		}
		return nil, companyMetricsOutput{Metrics: metrics}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_metric_observations", Description: "Store company metric observations linked to source news and supporting facts."}, func(ctx context.Context, _ *mcp.CallToolRequest, input metricObservationsInput) (*mcp.CallToolResult, insertedOutput, error) {
		if err := service.SaveMetricObservations(ctx, input.CompanyID, input.Observations); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(input.Observations)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "calculate_metric_total", Description: "Calculate and persist one probability-weighted metric total for a company and date range."}, func(ctx context.Context, _ *mcp.CallToolRequest, input metricTotalInput) (*mcp.CallToolResult, domain.MetricTotal, error) {
		total, err := service.CalculateMetricTotal(ctx, input.CompanyID, input.MetricID, input.PeriodFrom, input.PeriodTo)
		if err != nil {
			return nil, domain.MetricTotal{}, err
		}
		return nil, total, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "calculate_all_metric_totals", Description: "Calculate and persist totals for every metric configured for a company in PostgreSQL during a date range."}, func(ctx context.Context, _ *mcp.CallToolRequest, input periodInput) (*mcp.CallToolResult, totalsOutput, error) {
		totals, err := service.CalculateAllMetricTotals(ctx, input.CompanyID, input.PeriodFrom, input.PeriodTo)
		if err != nil {
			return nil, totalsOutput{}, err
		}
		return nil, totalsOutput{Totals: totals}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_company_analysis", Description: "Persist the probability-based analysis text for a company and date range."}, func(ctx context.Context, _ *mcp.CallToolRequest, input domain.CompanyAnalysis) (*mcp.CallToolResult, insertedOutput, error) {
		if err := service.SaveCompanyAnalysis(ctx, input); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: 1}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_company_report", Description: "Read a company report containing linked facts, configured metrics, totals and analyses."}, func(ctx context.Context, _ *mcp.CallToolRequest, input periodInput) (*mcp.CallToolResult, domain.CompanyReport, error) {
		report, err := service.CompanyReport(ctx, input.CompanyID, input.PeriodFrom, input.PeriodTo)
		if err != nil {
			return nil, domain.CompanyReport{}, err
		}
		return nil, report, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_consultant_context", Description: "Read the company's pre-aggregated linked facts, metric observations and calculated totals from ClickHouse without requiring a fixed date period. Results include source IDs for citations."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, domain.ConsultantContext, error) {
		result, err := service.ConsultantContext(ctx, input.CompanyID, input.Limit)
		if err != nil {
			return nil, domain.ConsultantContext{}, err
		}
		return nil, result, nil
	})

	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r)
	})
}

type saveNewsFactsInput struct {
	NewsID       string      `json:"news_id" jsonschema:"stable news identifier"`
	SourceID     string      `json:"source_id" jsonschema:"identifier of the news source"`
	SourceURL    string      `json:"source_url,omitempty"`
	CompanyID    string      `json:"company_id,omitempty"`
	MaterialKind string      `json:"material_kind,omitempty"`
	MediaType    string      `json:"media_type,omitempty"`
	Title        string      `json:"title,omitempty"`
	PublishedAt  *string     `json:"published_at,omitempty"`
	NewsText     string      `json:"news_text" jsonschema:"full cleaned news text"`
	Facts        []factInput `json:"facts"`
}
type factInput struct {
	FactText   string   `json:"fact_text"`
	Category   string   `json:"category"`
	Entities   []string `json:"entities"`
	Evidence   string   `json:"evidence"`
	OccurredAt *string  `json:"occurred_at"`
	Confidence float64  `json:"confidence"`
}
type saveNewsFactsOutput struct {
	NewsID   string   `json:"news_id"`
	Inserted int      `json:"inserted"`
	FactIDs  []string `json:"fact_ids"`
}
type newsFactsInput struct {
	NewsID string `json:"news_id"`
	Limit  int    `json:"limit,omitempty"`
}
type factsOutput struct {
	Facts []domain.Fact `json:"facts"`
}
type companyFactLinksInput struct {
	CompanyID string                   `json:"company_id"`
	Links     []domain.FactCompanyLink `json:"links"`
}
type companyLimitInput struct {
	CompanyID string `json:"company_id"`
	Limit     int    `json:"limit,omitempty"`
}
type companyFactsOutput struct {
	Facts []domain.CompanyFact `json:"facts"`
}
type companyMetricsInput struct {
	CompanyID string                 `json:"company_id"`
	Metrics   []domain.CompanyMetric `json:"metrics"`
}
type companyProfileInput struct {
	CompanyID string `json:"company_id"`
}
type saveCompanyContextInput struct {
	CompanyID       string                 `json:"company_id"`
	ProfileRevision int64                  `json:"profile_revision"`
	Markdown        string                 `json:"markdown"`
	Metrics         []domain.CompanyMetric `json:"metrics"`
}
type companyProfileOutput struct {
	CompanyID string `json:"company_id"`
	Key       string `json:"key"`
	Markdown  string `json:"markdown"`
}
type companyContextOutput struct {
	CompanyID       string `json:"company_id"`
	ProfileRevision int64  `json:"profile_revision"`
	ProfileKey      string `json:"profile_key"`
	MetricsSaved    int    `json:"metrics_saved"`
}
type companyMetricsOutput struct {
	Metrics []domain.CompanyMetric `json:"metrics"`
}
type metricObservationsInput struct {
	CompanyID    string                     `json:"company_id"`
	Observations []domain.MetricObservation `json:"observations"`
}
type metricTotalInput struct {
	CompanyID  string `json:"company_id"`
	MetricID   string `json:"metric_id"`
	PeriodFrom string `json:"period_from" jsonschema:"inclusive date YYYY-MM-DD"`
	PeriodTo   string `json:"period_to" jsonschema:"inclusive date YYYY-MM-DD"`
}
type periodInput struct {
	CompanyID  string `json:"company_id"`
	PeriodFrom string `json:"period_from" jsonschema:"inclusive date YYYY-MM-DD"`
	PeriodTo   string `json:"period_to" jsonschema:"inclusive date YYYY-MM-DD"`
}
type totalsOutput struct {
	Totals []domain.MetricTotal `json:"totals"`
}
type insertedOutput struct {
	Inserted int `json:"inserted"`
}
