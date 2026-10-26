package mcp

import (
	"github.com/predictboss/predictor/esb/internal/domain"
)

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
	OccurredAt *string  `json:"occurred_at" jsonschema:"RFC3339 timestamp with an explicit timezone, or null. ESB normalizes empty or non-RFC3339 values to null."`
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
	Probability   any      `json:"probability,omitempty" jsonschema:"optional probability from 0 to 1; accepts a number, numeric string, or null"`
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
	MetricID   string `json:"metric_id"`
	PeriodFrom string `json:"period_from" jsonschema:"inclusive timestamp YYYY-MM-DD HH:mm:ss; YYYY-MM-DD is accepted as start of day"`
	PeriodTo   string `json:"period_to" jsonschema:"inclusive timestamp YYYY-MM-DD HH:mm:ss; YYYY-MM-DD is accepted as end of day"`
}
type periodInput struct {
	CompanyID  string `json:"company_id"`
	PeriodFrom string `json:"period_from" jsonschema:"inclusive timestamp YYYY-MM-DD HH:mm:ss; YYYY-MM-DD is accepted as start of day"`
	PeriodTo   string `json:"period_to" jsonschema:"inclusive timestamp YYYY-MM-DD HH:mm:ss; YYYY-MM-DD is accepted as end of day"`
}
type insertedOutput struct {
	Inserted int `json:"inserted"`
}
