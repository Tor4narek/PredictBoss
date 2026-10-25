package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func registerReportTools(server *mcp.Server, service *analytics.Service) {
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

	mcp.AddTool(server, &mcp.Tool{Name: "get_consultant_context", Description: "Read linked facts and observations from ClickHouse and recalculate configured company metric totals through ESB. Results include source IDs; totals use news publication times across stored history. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, domain.ConsultantContext, error) {
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
}
