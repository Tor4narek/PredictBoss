package news

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

const MaxNewsTextBytes = 1 << 20

var ErrInvalidInput = errors.New("invalid news input")
var sourceIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var newsIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type SourceRegistry interface {
	RecordSource(context.Context, domain.CompanySource) (bool, error)
}

type CallCreator interface {
	Create(context.Context, string, json.RawMessage, string) (domain.Call, bool, error)
}

type Service struct {
	sources SourceRegistry
	calls   CallCreator
}

type input struct {
	NewsID       string  `json:"news_id"`
	SourceID     string  `json:"source_id"`
	SourceURL    string  `json:"source_url,omitempty"`
	CompanyID    string  `json:"company_id,omitempty"`
	MaterialKind string  `json:"material_kind,omitempty"`
	MediaType    string  `json:"media_type,omitempty"`
	Title        string  `json:"title,omitempty"`
	PublishedAt  *string `json:"published_at,omitempty"`
	NewsText     string  `json:"news_text"`
}

func NewService(sources SourceRegistry, callsService CallCreator) *Service {
	return &Service{sources: sources, calls: callsService}
}

// Submit validates a Serezha request, registers company source metadata when
// the article belongs to a company, then queues the asynchronous extraction.
func (s *Service) Submit(ctx context.Context, raw json.RawMessage) (domain.Call, bool, error) {
	var request input
	if err := json.Unmarshal(raw, &request); err != nil {
		return domain.Call{}, false, ErrInvalidInput
	}
	request.NewsID = strings.TrimSpace(request.NewsID)
	request.SourceID = strings.ToLower(strings.TrimSpace(request.SourceID))
	request.SourceURL = strings.TrimSpace(request.SourceURL)
	request.CompanyID = strings.ToLower(strings.TrimSpace(request.CompanyID))
	request.MaterialKind = strings.TrimSpace(request.MaterialKind)
	request.MediaType = strings.TrimSpace(request.MediaType)
	request.Title = strings.TrimSpace(request.Title)
	request.NewsText = strings.TrimSpace(request.NewsText)
	if !newsIDPattern.MatchString(request.NewsID) || !sourceIDPattern.MatchString(request.SourceID) || request.NewsText == "" || len(request.NewsText) > MaxNewsTextBytes {
		return domain.Call{}, false, ErrInvalidInput
	}
	if request.CompanyID != "" && !sourceIDPattern.MatchString(request.CompanyID) {
		return domain.Call{}, false, ErrInvalidInput
	}
	if request.MaterialKind == "" {
		request.MaterialKind = "external_news"
	}
	if request.MediaType == "" {
		request.MediaType = "text/html"
	}
	if (request.MaterialKind != "external_news" && request.MaterialKind != "alpha_analysis") ||
		(request.MediaType != "application/pdf" && request.MediaType != "text/html") {
		return domain.Call{}, false, ErrInvalidInput
	}
	if request.PublishedAt != nil {
		if _, err := time.Parse(time.RFC3339, *request.PublishedAt); err != nil {
			return domain.Call{}, false, ErrInvalidInput
		}
	}
	if request.SourceURL != "" {
		parsed, err := url.ParseRequestURI(request.SourceURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return domain.Call{}, false, ErrInvalidInput
		}
	}
	if request.CompanyID != "" {
		if request.SourceURL == "" {
			return domain.Call{}, false, ErrInvalidInput
		}
		if _, err := s.sources.RecordSource(ctx, domain.CompanySource{
			SourceID: request.SourceID, CompanyID: request.CompanyID, NewsID: request.NewsID,
			MaterialKind: request.MaterialKind, MediaType: request.MediaType, SourceURL: request.SourceURL,
			Title: request.Title, PublishedAt: request.PublishedAt,
		}); err != nil {
			return domain.Call{}, false, err
		}
	}
	normalized, err := json.Marshal(request)
	if err != nil {
		return domain.Call{}, false, err
	}
	return s.calls.Create(ctx, string(domain.AgentSerezha), normalized, "news:"+request.SourceID)
}
