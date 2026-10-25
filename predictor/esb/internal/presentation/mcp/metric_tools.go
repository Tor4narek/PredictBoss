package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/domain"
)

func registerMetricTools(server *mcp.Server, service *analytics.Service) {
	mcp.AddTool(server, &mcp.Tool{Name: "save_metric_observations", Description: "Store metric observations linked to news and facts. Values and probabilities accept JSON numbers or numeric strings; ESB normalizes numeric strings. Use null for unknown values. Period bounds are not stored on observations. ESB derives a stable observation ID from company, metric, news, and source fact IDs; supply a distinct observation_id when separate predictions share the same fact IDs."}, func(ctx context.Context, _ *mcp.CallToolRequest, input metricObservationsInput) (*mcp.CallToolResult, insertedOutput, error) {
		observations := make([]domain.MetricObservation, 0, len(input.Observations))
		for i, observation := range input.Observations {
			if observation.CompanyID != "" && !strings.EqualFold(strings.TrimSpace(observation.CompanyID), strings.TrimSpace(input.CompanyID)) {
				return nil, insertedOutput{}, fmt.Errorf("observation %d company_id must match the request company_id", i)
			}
			var probability *float64
			if observation.Probability != nil {
				parsed, err := numericValue(observation.Probability)
				if err != nil {
					return nil, insertedOutput{}, fmt.Errorf("observation %d probability must be a number or null: %w", i, err)
				}
				probability = &parsed
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
				Probability: probability, SourceFactIDs: observation.SourceFactIDs})
		}
		if err := service.SaveMetricObservations(ctx, input.CompanyID, observations); err != nil {
			return nil, insertedOutput{}, err
		}
		return nil, insertedOutput{Inserted: len(observations)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "calculate_metric_total", Description: "Recalculate one metric from ClickHouse observations for an inclusive news-time range and persist the resulting analytical snapshot in ClickHouse. Metric ownership and its returned unit are read from PostgreSQL settings."}, func(ctx context.Context, _ *mcp.CallToolRequest, input metricTotalInput) (*mcp.CallToolResult, domain.MetricTotal, error) {
		total, err := service.CalculateMetricTotal(ctx, input.MetricID, input.PeriodFrom, input.PeriodTo)
		if err != nil {
			return nil, domain.MetricTotal{}, err
		}
		return nil, total, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "calculate_all_metric_totals", Description: "Read a company's metric rows and units from PostgreSQL, recalculate each from ClickHouse observations, persist the resulting analytical snapshots in ClickHouse, and return the catalog-ordered results."}, func(ctx context.Context, _ *mcp.CallToolRequest, input periodInput) (*mcp.CallToolResult, domain.CompanyMetricTotals, error) {
		totals, err := service.CalculateAllMetricTotals(ctx, input.CompanyID, input.PeriodFrom, input.PeriodTo)
		if err != nil {
			return nil, domain.CompanyMetricTotals{}, err
		}
		return nil, totals, nil
	})

}
