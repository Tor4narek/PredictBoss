package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
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

	mcp.AddTool(server, &mcp.Tool{Name: "save_news_and_facts", Description: "Store submitted news and company-independent facts in ClickHouse. Confidence accepts a JSON number or a numeric string; ESB normalizes numeric strings."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveNewsFactsInput) (*mcp.CallToolResult, saveNewsFactsOutput, error) {
		if strings.TrimSpace(input.NewsID) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.NewsText) == "" {
			return nil, saveNewsFactsOutput{}, fmt.Errorf("news_id, source_id and news_text are required")
		}
		facts := make([]domain.Fact, 0, len(input.Facts))
		for i, fact := range input.Facts {
			confidence, err := numericValue(fact.Confidence)
			if err != nil {
				return nil, saveNewsFactsOutput{}, fmt.Errorf("fact %d confidence must be a number: %w", i, err)
			}
			facts = append(facts, domain.Fact{Text: fact.FactText, Category: fact.Category, Entities: fact.Entities, Evidence: fact.Evidence, OccurredAt: fact.OccurredAt, Confidence: confidence})
		}
		ids, err := service.SaveNewsAndFacts(ctx, domain.News{ID: input.NewsID, SourceID: input.SourceID, SourceURL: input.SourceURL,
			CompanyID: input.CompanyID, MaterialKind: input.MaterialKind, MediaType: input.MediaType, Title: input.Title,
			PublishedAt: input.PublishedAt, Text: input.NewsText}, facts)
		if err != nil {
			return nil, saveNewsFactsOutput{}, err
		}
		return nil, saveNewsFactsOutput{NewsID: input.NewsID, Inserted: len(ids), FactIDs: ids}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_facts", Description: "Read universal facts for a news_id from ClickHouse. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input newsFactsInput) (*mcp.CallToolResult, factsOutput, error) {
		limit, err := integerValue(input.Limit)
		if err != nil {
			return nil, factsOutput{}, fmt.Errorf("limit must be an integer: %w", err)
		}
		facts, err := service.FactsByNews(ctx, input.NewsID, limit)
		if err != nil {
			return nil, factsOutput{}, err
		}
		return nil, factsOutput{Facts: facts}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_fact_company_links", Description: "Persist Savely's supported links between universal facts and a company."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyFactLinksInput) (*mcp.CallToolResult, insertedOutput, error) {
		links := make([]domain.FactCompanyLink, 0, len(input.Links))
		for i, link := range input.Links {
			confidence, err := numericValue(link.Confidence)
			if err != nil {
				return nil, insertedOutput{}, fmt.Errorf("link %d confidence must be a number: %w", i, err)
			}
			links = append(links, domain.FactCompanyLink{FactID: link.FactID, Relevance: link.Relevance, Confidence: confidence})
		}
		if err := service.SaveFactCompanyLinks(ctx, input.CompanyID, links); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(links)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "get_company_facts", Description: "Read universal facts that have already been linked to a company. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, companyFactsOutput, error) {
		limit, err := integerValue(input.Limit)
		if err != nil {
			return nil, companyFactsOutput{}, fmt.Errorf("limit must be an integer: %w", err)
		}
		facts, err := service.CompanyFacts(ctx, input.CompanyID, limit)
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

	mcp.AddTool(server, &mcp.Tool{Name: "get_company_metrics", Description: "Read the company's structured metric settings from PostgreSQL through the ESB. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, companyMetricsOutput, error) {
		limit, err := integerValue(input.Limit)
		if err != nil {
			return nil, companyMetricsOutput{}, fmt.Errorf("limit must be an integer: %w", err)
		}
		metrics, err := companyContext.CompanyMetrics(ctx, input.CompanyID, limit)
		if err != nil {
			return nil, companyMetricsOutput{}, err
		}
		return nil, companyMetricsOutput{Metrics: metrics}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "save_metric_observations", Description: "Store metric observations linked to news and facts. Values and probabilities accept JSON numbers or numeric strings; ESB normalizes numeric strings. Use null for unknown values."}, func(ctx context.Context, _ *mcp.CallToolRequest, input metricObservationsInput) (*mcp.CallToolResult, insertedOutput, error) {
		observations := make([]domain.MetricObservation, 0, len(input.Observations))
		for i, observation := range input.Observations {
			if observation.CompanyID != "" && !strings.EqualFold(strings.TrimSpace(observation.CompanyID), strings.TrimSpace(input.CompanyID)) {
				return nil, insertedOutput{}, fmt.Errorf("observation %d company_id must match the request company_id", i)
			}
			probability, err := numericValue(observation.Probability)
			if err != nil {
				return nil, insertedOutput{}, fmt.Errorf("observation %d probability must be a number: %w", i, err)
			}
			var value *float64
			if observation.Value != nil {
				parsed, err := numericValue(observation.Value)
				if err != nil {
					return nil, insertedOutput{}, fmt.Errorf("observation %d value must be a number or null: %w", i, err)
				}
				value = &parsed
			}
			observations = append(observations, domain.MetricObservation{ID: observation.ID, CompanyID: input.CompanyID, MetricID: observation.MetricID, NewsID: observation.NewsID,
				SourceID: observation.SourceID, SourceURL: observation.SourceURL, MaterialKind: observation.MaterialKind, Value: value,
				Probability: probability, PeriodFrom: observation.PeriodFrom, PeriodTo: observation.PeriodTo, SourceFactIDs: observation.SourceFactIDs})
		}
		if err := service.SaveMetricObservations(ctx, input.CompanyID, observations); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(observations)}, nil
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

	mcp.AddTool(server, &mcp.Tool{Name: "save_company_analysis", Description: "Persist a probability-based analysis. Probability accepts a JSON number or numeric string; ESB normalizes numeric strings."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyAnalysisInput) (*mcp.CallToolResult, insertedOutput, error) {
		probability, err := numericValue(input.Probability)
		if err != nil {
			return nil, insertedOutput{}, fmt.Errorf("analysis probability must be a number: %w", err)
		}
		analysis := domain.CompanyAnalysis{CompanyID: input.CompanyID, PeriodFrom: input.PeriodFrom, PeriodTo: input.PeriodTo, Probability: probability, MetricIDs: input.MetricIDs, Text: input.Text}
		if err := service.SaveCompanyAnalysis(ctx, analysis); err != nil {
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

	mcp.AddTool(server, &mcp.Tool{Name: "get_consultant_context", Description: "Read pre-aggregated linked facts, observations, and totals from ClickHouse without requiring a fixed period. Results include source IDs. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, domain.ConsultantContext, error) {
		limit, err := integerValue(input.Limit)
		if err != nil {
			return nil, domain.ConsultantContext{}, fmt.Errorf("limit must be an integer: %w", err)
		}
		result, err := service.ConsultantContext(ctx, input.CompanyID, limit)
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
	Confidence any      `json:"confidence" jsonschema:"confidence from 0 to 1; accepts a number or numeric string"`
}
type saveNewsFactsOutput struct {
	NewsID   string   `json:"news_id"`
	Inserted int      `json:"inserted"`
	FactIDs  []string `json:"fact_ids"`
}
type newsFactsInput struct {
	NewsID string `json:"news_id"`
	Limit  any    `json:"limit,omitempty" jsonschema:"integer limit; accepts an integer or numeric string"`
}
type factsOutput struct {
	Facts []domain.Fact `json:"facts"`
}
type companyFactLinksInput struct {
	CompanyID string                 `json:"company_id"`
	Links     []factCompanyLinkInput `json:"links"`
}
type factCompanyLinkInput struct {
	FactID     string `json:"fact_id"`
	Relevance  string `json:"relevance"`
	Confidence any    `json:"confidence" jsonschema:"confidence from 0 to 1; accepts a number or numeric string"`
}
type companyLimitInput struct {
	CompanyID string `json:"company_id"`
	Limit     any    `json:"limit,omitempty" jsonschema:"integer limit; accepts an integer or numeric string"`
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
	CompanyID    string                   `json:"company_id"`
	Observations []metricObservationInput `json:"observations"`
}
type metricObservationInput struct {
	ID            string   `json:"observation_id,omitempty"`
	CompanyID     string   `json:"company_id,omitempty" jsonschema:"optional company ID; when supplied it must match the request company_id"`
	MetricID      string   `json:"metric_id"`
	NewsID        string   `json:"news_id"`
	SourceID      string   `json:"source_id,omitempty"`
	SourceURL     string   `json:"source_url,omitempty"`
	MaterialKind  string   `json:"material_kind,omitempty"`
	Value         any      `json:"value" jsonschema:"numeric value or null; accepts a number or numeric string"`
	Probability   any      `json:"probability" jsonschema:"probability from 0 to 1; accepts a number or numeric string"`
	PeriodFrom    string   `json:"period_from"`
	PeriodTo      string   `json:"period_to"`
	SourceFactIDs []string `json:"source_fact_ids"`
}
type companyAnalysisInput struct {
	CompanyID   string   `json:"company_id"`
	PeriodFrom  string   `json:"period_from"`
	PeriodTo    string   `json:"period_to"`
	Probability any      `json:"probability" jsonschema:"probability from 0 to 1; accepts a number or numeric string"`
	MetricIDs   []string `json:"metric_ids"`
	Text        string   `json:"text"`
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

func numericValue(value any) (float64, error) {
	var parsed float64
	switch value := value.(type) {
	case float64:
		parsed = value
	case float32:
		parsed = float64(value)
	case int:
		parsed = float64(value)
	case int64:
		parsed = float64(value)
	case json.Number:
		var err error
		parsed, err = value.Float64()
		if err != nil {
			return 0, err
		}
	case string:
		var err error
		parsed, err = strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unsupported numeric value of type %T", value)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("numeric value must be finite")
	}
	return parsed, nil
}

func integerValue(value any) (int, error) {
	if value == nil {
		return 0, nil
	}
	parsed, err := numericValue(value)
	if err != nil {
		return 0, err
	}
	if parsed < 0 || parsed > 1_000_000 || math.Trunc(parsed) != parsed {
		return 0, fmt.Errorf("integer value is outside the supported range")
	}
	return int(parsed), nil
}
