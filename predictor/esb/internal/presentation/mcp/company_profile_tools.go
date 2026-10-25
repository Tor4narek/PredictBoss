package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
)

func registerCompanyProfileTools(server *mcp.Server, companyContext *companycontext.Service) {
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

}
