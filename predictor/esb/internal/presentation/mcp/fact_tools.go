package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func registerFactTools(server *mcp.Server, allowed map[string]struct{}, service *analytics.Service) {
	addTool(server, allowed, &mcp.Tool{Name: "save_news_and_facts", Description: "Store submitted news and company-independent facts in ClickHouse. Copy news_id, source_id, and news_text exactly from the current call. Copy optional metadata only when supplied; never invent company_id, source_url, or timestamps. An empty facts array is valid. occurred_at must be RFC3339 with an explicit timezone or null. ESB normalizes empty or non-RFC3339 values to null because the event time is unknown or ambiguous. Confidence accepts a JSON number or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input saveNewsFactsInput) (*mcp.CallToolResult, saveNewsFactsOutput, error) {
		if strings.TrimSpace(input.NewsID) == "" || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.NewsText) == "" {
			return nil, saveNewsFactsOutput{}, fmt.Errorf("news_id, source_id and news_text are required")
		}
		facts := make([]domain.Fact, 0, len(input.Facts))
		for i, fact := range input.Facts {
			confidence, err := numericValue(fact.Confidence)
			if err != nil {
				return nil, saveNewsFactsOutput{}, fmt.Errorf("fact %d confidence must be a number: %w", i, err)
			}
			occurredAt := normalizeOccurredAt(fact.OccurredAt)
			facts = append(facts, domain.Fact{Text: fact.FactText, Category: fact.Category, Entities: fact.Entities, Evidence: fact.Evidence, OccurredAt: occurredAt, Confidence: confidence})
		}
		ids, err := service.SaveNewsAndFacts(ctx, domain.News{ID: input.NewsID, SourceID: input.SourceID, SourceURL: input.SourceURL,
			CompanyID: input.CompanyID, MaterialKind: input.MaterialKind, MediaType: input.MediaType, Title: input.Title,
			PublishedAt: input.PublishedAt, Text: input.NewsText}, facts)
		if err != nil {
			return nil, saveNewsFactsOutput{}, err
		}
		return nil, saveNewsFactsOutput{NewsID: input.NewsID, Inserted: len(ids), FactIDs: ids}, nil
	})

	addTool(server, allowed, &mcp.Tool{Name: "get_facts", Description: "Read universal facts for a news_id from ClickHouse. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input newsFactsInput) (*mcp.CallToolResult, factsOutput, error) {
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

	addTool(server, allowed, &mcp.Tool{Name: "save_fact_company_links", Description: "Persist Savely's supported links between universal facts and a company."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyFactLinksInput) (*mcp.CallToolResult, insertedOutput, error) {
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

	addTool(server, allowed, &mcp.Tool{Name: "get_company_facts", Description: "Read universal facts that have already been linked to a company. Limit accepts an integer or numeric string."}, func(ctx context.Context, _ *mcp.CallToolRequest, input companyLimitInput) (*mcp.CallToolResult, companyFactsOutput, error) {
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

}

func normalizeOccurredAt(value *string) *string {
	if value == nil {
		return nil
	}

	timestamp := strings.TrimSpace(*value)
	if timestamp == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		return nil
	}
	return &timestamp
}
