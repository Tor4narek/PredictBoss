package domainapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return nil, errors.New("DOMAIN_API_BASE_URL must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("DOMAIN_API_TOKEN is required")
	}
	return &Client{baseURL: parsed.String(), token: token, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (c *Client) NewsLink(ctx context.Context, sourceID string) (domain.CompanySource, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/news-links/"+url.PathEscape(sourceID), nil)
	if err != nil {
		return domain.CompanySource{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.http.Do(request)
	if err != nil {
		return domain.CompanySource{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return domain.CompanySource{}, domain.ErrNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.CompanySource{}, fmt.Errorf("Domain metadata lookup returned HTTP %d", response.StatusCode)
	}
	var result struct {
		SourceID     string `json:"sourceId"`
		CompanyID    string `json:"companyId"`
		CanonicalURL string `json:"canonicalUrl"`
		MaterialKind string `json:"materialKind"`
		MediaType    string `json:"mediaType"`
		Title        string `json:"title"`
		PublishedAt  string `json:"publishedAt"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return domain.CompanySource{}, err
	}
	metadata := domain.CompanySource{SourceID: strings.ToLower(result.SourceID), CompanyID: strings.ToLower(result.CompanyID), NewsID: strings.ToLower(result.SourceID),
		MaterialKind: result.MaterialKind, MediaType: result.MediaType, SourceURL: result.CanonicalURL, Title: result.Title}
	if result.PublishedAt != "" {
		if _, err := time.Parse(time.RFC3339, result.PublishedAt); err != nil {
			return domain.CompanySource{}, fmt.Errorf("Domain publishedAt must be ISO 8601: %w", err)
		}
		metadata.PublishedAt = &result.PublishedAt
	}
	return metadata, nil
}
