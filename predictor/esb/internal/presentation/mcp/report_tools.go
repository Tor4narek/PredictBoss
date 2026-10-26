package mcp

import (
	"context"
	"fmt"
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

	addTool(server, allowed, &mcp.Tool{Name: "get_consultant_context", Description: "Read linked facts and observations from ClickHouse and recalculate configured company metric totals through ESB. Results include source IDs; totals use news publication times across stored history. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, domain.ConsultantContext, error) {
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
		if err := authorizer.RecordMCPCallSources(ctx, callContext, consultantContextSourceIDs(result)); err != nil {
			return nil, domain.ConsultantContext{}, err
		}
		return nil, result, nil
	})
}

func consultantContextSourceIDs(value domain.ConsultantContext) []string {
	seen := make(map[string]struct{}, len(value.Facts)+len(value.Observations))
	var sourceIDs []string
	add := func(sourceID string) {
		sourceID = strings.ToLower(strings.TrimSpace(sourceID))
		if sourceID == "" {
			return
		}
		if _, exists := seen[sourceID]; exists {
			return
		}
		seen[sourceID] = struct{}{}
		sourceIDs = append(sourceIDs, sourceID)
	}
	for _, fact := range value.Facts {
		add(fact.SourceID)
	}
	for _, observation := range value.Observations {
		add(observation.SourceID)
	}
	return sourceIDs
}
