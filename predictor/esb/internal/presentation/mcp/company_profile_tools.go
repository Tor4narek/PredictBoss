package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func registerCompanyProfileTools(server *mcp.Server, allowed map[string]struct{}, companyContext *companycontext.Service,
	agent domain.AgentType, authorizer domain.MCPCompanyCallAuthorizer) {
	addTool(server, allowed, &mcp.Tool{Name: "get_company_profile", Description: "Read the company's Markdown profile from S3 through the ESB. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyProfileInput) (*mcp.CallToolResult, companyProfileOutput, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, companyProfileOutput{}, err
		}
		markdown, key, err := companyContext.CompanyProfile(ctx, callContext.CompanyID)
		if err != nil {
			return nil, companyProfileOutput{}, err
		}
		return nil, companyProfileOutput{CompanyID: callContext.CompanyID, Key: key, Markdown: markdown}, nil
	})

	addTool(server, allowed, &mcp.Tool{Name: "save_company_profile_and_metrics", Description: "Idempotently save the company Markdown profile to S3 and upsert its structured metric settings in PostgreSQL through the ESB. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveCompanyContextInput) (*mcp.CallToolResult, companyContextOutput, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, companyContextOutput{}, err
		}
		key, err := companyContext.SaveCompanyProfileAndMetrics(ctx, callContext.CompanyID, input.ProfileRevision, input.Markdown, input.Metrics)
		if err != nil {
			return nil, companyContextOutput{}, err
		}
		return nil, companyContextOutput{CompanyID: callContext.CompanyID, ProfileRevision: input.ProfileRevision, ProfileKey: key, MetricsSaved: len(input.Metrics)}, nil
	})

	addTool(server, allowed, &mcp.Tool{Name: "save_company_metrics", Description: "Idempotently upsert structured metric settings for a company in PostgreSQL through the ESB. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyMetricsInput) (*mcp.CallToolResult, insertedOutput, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, insertedOutput{}, err
		}
		if err := companyContext.SaveCompanyMetrics(ctx, callContext.CompanyID, input.Metrics); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(input.Metrics)}, nil
	})

	addTool(server, allowed, &mcp.Tool{Name: "get_company_metrics", Description: "Read the company's structured metric settings from PostgreSQL through the ESB. `call_context` must be copied unchanged from this active call; `company_id` must match its signed company scope. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, companyMetricsOutput, error) {
		callContext, err := authorizeMCPCompany(ctx, authorizer, agent, input.CallContext, input.CompanyID)
		if err != nil {
			return nil, companyMetricsOutput{}, err
		}
		limit, err := integerValue(input.Limit)
		if err != nil {
			return nil, companyMetricsOutput{}, fmt.Errorf("limit must be an integer: %w", err)
		}
		metrics, err := companyContext.CompanyMetrics(ctx, callContext.CompanyID, limit)
		if err != nil {
			return nil, companyMetricsOutput{}, err
		}
		return nil, companyMetricsOutput{Metrics: metrics}, nil
	})

}
