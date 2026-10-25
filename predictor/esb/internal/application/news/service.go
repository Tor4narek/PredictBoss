package news

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/predictboss/predictor/esb/internal/domain"
)

const MaxNewsTextBytes = 1 << 20

var ErrInvalidInput = errors.New("invalid news input")
var sourceIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type MetadataLookup interface {
	NewsLink(context.Context, string) (domain.CompanySource, error)
}

type SourceRegistry interface {
	RecordSource(context.Context, domain.CompanySource) (bool, error)
}

type CallCreator interface {
	Create(context.Context, string, json.RawMessage, string) (domain.Call, bool, error)
}

type Service struct {
	metadata MetadataLookup
	sources  SourceRegistry
	calls    CallCreator
}

func NewService(metadata MetadataLookup, sources SourceRegistry, callsService CallCreator) *Service {
	return &Service{metadata: metadata, sources: sources, calls: callsService}
}

func (s *Service) Ingest(ctx context.Context, sourceID, text string) (domain.Call, bool, error) {
	sourceID = strings.TrimSpace(sourceID)
	text = strings.TrimSpace(text)
	if !sourceIDPattern.MatchString(sourceID) || text == "" || len(text) > MaxNewsTextBytes {
		return domain.Call{}, false, ErrInvalidInput
	}
	sourceID = strings.ToLower(sourceID)
	metadata, err := s.metadata.NewsLink(ctx, sourceID)
	if err != nil {
		return domain.Call{}, false, err
	}
	if metadata.SourceID != sourceID || !sourceIDPattern.MatchString(metadata.CompanyID) || metadata.NewsID != sourceID ||
		(metadata.MaterialKind != "external_news" && metadata.MaterialKind != "alpha_analysis") ||
		(metadata.MediaType != "application/pdf" && metadata.MediaType != "text/html") || strings.TrimSpace(metadata.SourceURL) == "" {
		return domain.Call{}, false, ErrInvalidInput
	}
	metadata.SourceID = sourceID
	metadata.NewsID = sourceID
	metadata.CompanyID = strings.ToLower(metadata.CompanyID)
	_, err = s.sources.RecordSource(ctx, metadata)
	if err != nil {
		return domain.Call{}, false, err
	}
	input, err := json.Marshal(struct {
		NewsID       string  `json:"news_id"`
		SourceID     string  `json:"source_id"`
		SourceURL    string  `json:"source_url"`
		CompanyID    string  `json:"company_id"`
		MaterialKind string  `json:"material_kind"`
		MediaType    string  `json:"media_type"`
		Title        string  `json:"title"`
		PublishedAt  *string `json:"published_at,omitempty"`
		NewsText     string  `json:"news_text"`
	}{metadata.NewsID, metadata.SourceID, metadata.SourceURL, metadata.CompanyID, metadata.MaterialKind, metadata.MediaType, metadata.Title, metadata.PublishedAt, text})
	if err != nil {
		return domain.Call{}, false, err
	}
	return s.calls.Create(ctx, string(domain.AgentSerezha), input, "news:"+sourceID)
}
