package mcp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func registerReportTools(server *mcp.Server, allowed map[string]struct{}, service *analytics.Service,
	agent domain.AgentType, authorizer domain.MCPCompanyCallAuthorizer) {
	addTool(server, allowed, &mcp.Tool{Name: "save_company_analysis", Description: "Persist a probability-based analysis. Copy `call_context` unchanged from this active call; `company_id` must match its signed company scope. Probability accepts a JSON number or numeric string; ESB normalizes numeric strings."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyAnalysisInput) (*mcp.CallToolResult, insertedOutput, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, insertedOutput{}, err
		}
		probability, err := numericValue(input.Probability)
		if err != nil {
			return nil, insertedOutput{}, fmt.Errorf("analysis probability must be a number: %w", err)
		}
		analysis := domain.CompanyAnalysis{CompanyID: callContext.CompanyID, PeriodFrom: input.PeriodFrom, PeriodTo: input.PeriodTo, Probability: probability, MetricIDs: input.MetricIDs, Text: input.Text}
		if err := service.SaveCompanyAnalysis(ctx, analysis); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: 1}, nil
	})

	addTool(server, allowed, &mcp.Tool{Name: "get_company_report", Description: "Read a company report containing linked facts, configured metrics, totals and analyses. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope."}, func(ctx context.Context, _ *mcp.CallToolRequest, input periodInput) (*mcp.CallToolResult, domain.CompanyReport, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, domain.CompanyReport{}, err
		}
		report, err := service.CompanyReport(ctx, callContext.CompanyID, input.PeriodFrom, input.PeriodTo)
		if err != nil {
			return nil, domain.CompanyReport{}, err
		}
		return nil, report, nil
	})

	addTool(server, allowed, &mcp.Tool{Name: "get_consultant_context", Description: "Read linked facts and observations from ClickHouse and recalculate configured company metric totals through ESB. The ESB records publication metadata returned here for this signed call and adds verified sources to the final response. Totals use news publication times across stored history. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, domain.ConsultantContext, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, domain.ConsultantContext{}, err
		}
		limit, err := integerValue(input.Limit)
		if err != nil {
			return nil, domain.ConsultantContext{}, fmt.Errorf("limit must be an integer: %w", err)
		}
		result, err := service.ConsultantContext(ctx, callContext.CompanyID, limit)
		if err != nil {
			return nil, domain.ConsultantContext{}, err
		}
		if err := authorizer.RecordMCPCallSources(ctx, callContext, consultantContextSources(result)); err != nil {
			return nil, domain.ConsultantContext{}, err
		}
		return nil, result, nil
	})
}

var consultantSourceIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func consultantContextSources(value domain.ConsultantContext) []domain.ConsultantSource {
	byNewsID := make(map[string]domain.ConsultantSource, len(value.Facts)+len(value.Observations))
	newsBySourceID := make(map[string]string, len(value.Facts)+len(value.Observations))
	conflictedNews := make(map[string]struct{})
	conflictedSourceIDs := make(map[string]struct{})
	add := func(source domain.ConsultantSource) {
		source.SourceID = strings.ToLower(strings.TrimSpace(source.SourceID))
		source.NewsID = strings.TrimSpace(source.NewsID)
		source.SourceURL = strings.TrimSpace(source.SourceURL)
		source.MaterialKind = strings.TrimSpace(source.MaterialKind)
		source.Title = strings.TrimSpace(source.Title)
		if source.PublishedAt != nil {
			publishedAt := strings.TrimSpace(*source.PublishedAt)
			if publishedAt == "" {
				source.PublishedAt = nil
			} else {
				source.PublishedAt = &publishedAt
			}
		}
		if !validConsultantSource(source) {
			return
		}
		if previousNewsID, exists := newsBySourceID[source.SourceID]; exists && previousNewsID != source.NewsID {
			conflictedSourceIDs[source.SourceID] = struct{}{}
			delete(byNewsID, previousNewsID)
			delete(byNewsID, source.NewsID)
			conflictedNews[previousNewsID] = struct{}{}
			conflictedNews[source.NewsID] = struct{}{}
			return
		}
		newsBySourceID[source.SourceID] = source.NewsID
		if _, conflicted := conflictedSourceIDs[source.SourceID]; conflicted {
			return
		}
		if _, conflicted := conflictedNews[source.NewsID]; conflicted {
			return
		}
		if previous, exists := byNewsID[source.NewsID]; exists {
			if sameConsultantSource(previous, source) {
				return
			}
			delete(byNewsID, source.NewsID)
			conflictedNews[source.NewsID] = struct{}{}
			return
		}
		byNewsID[source.NewsID] = source
	}
	for _, fact := range value.Facts {
		add(domain.ConsultantSource{SourceID: fact.SourceID, NewsID: fact.NewsID, SourceURL: fact.SourceURL,
			MaterialKind: fact.MaterialKind, Title: fact.Title, PublishedAt: fact.PublishedAt})
	}
	for _, observation := range value.Observations {
		add(domain.ConsultantSource{SourceID: observation.SourceID, NewsID: observation.NewsID,
			SourceURL: observation.SourceURL, MaterialKind: observation.MaterialKind,
			Title: observation.Title, PublishedAt: observation.PublishedAt})
	}
	sources := make([]domain.ConsultantSource, 0, len(byNewsID))
	for _, source := range byNewsID {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(left, right int) bool {
		if sources[left].NewsID == sources[right].NewsID {
			return sources[left].SourceID < sources[right].SourceID
		}
		return sources[left].NewsID < sources[right].NewsID
	})
	return sources
}

func validConsultantSource(source domain.ConsultantSource) bool {
	if !consultantSourceIDPattern.MatchString(source.SourceID) || source.NewsID == "" || len(source.NewsID) > 512 {
		return false
	}
	if source.MaterialKind != "external_news" && source.MaterialKind != "alpha_analysis" {
		return false
	}
	if source.SourceURL == "" {
		return true
	}
	parsed, err := url.Parse(source.SourceURL)
	return err == nil && parsed.IsAbs() && parsed.Host != "" && parsed.User == nil &&
		(parsed.Scheme == "http" || parsed.Scheme == "https")
}

func sameConsultantSource(left, right domain.ConsultantSource) bool {
	if left.SourceID != right.SourceID || left.NewsID != right.NewsID || left.SourceURL != right.SourceURL ||
		left.MaterialKind != right.MaterialKind || left.Title != right.Title {
		return false
	}
	if left.PublishedAt == nil || right.PublishedAt == nil {
		return left.PublishedAt == nil && right.PublishedAt == nil
	}
	return *left.PublishedAt == *right.PublishedAt
}
