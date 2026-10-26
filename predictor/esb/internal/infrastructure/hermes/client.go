package hermes

import (
	"bytes"
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

// Client calls Hermes, the only runtime ESB uses to execute agent prompts.
type Client struct {
	apiKey           string
	http             *http.Client
	maxAttempts      int
	retryBaseDelay   time.Duration
	maxResponseBytes int64
}

type ClientConfig struct {
	RequestTimeout   time.Duration
	MaxAttempts      int
	RetryBaseDelay   time.Duration
	MaxResponseBytes int64
}

// NewClient creates a bounded Hermes HTTP client.
func NewClient(apiKey string, config ClientConfig) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("hermes API key is required")
	}
	if config.RequestTimeout <= 0 || config.MaxAttempts < 1 || config.MaxAttempts > 20 || config.RetryBaseDelay < 0 || config.MaxResponseBytes <= 0 {
		return nil, errors.New("hermes timeout, attempt count, retry delay, and response limit are invalid")
	}
	return &Client{apiKey: apiKey, http: &http.Client{
		Timeout:       config.RequestTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}, maxAttempts: config.MaxAttempts, retryBaseDelay: config.RetryBaseDelay,
		maxResponseBytes: config.MaxResponseBytes}, nil
}

type runRequest struct {
	Input               string           `json:"input"`
	Instructions        string           `json:"instructions"`
	ConversationHistory []domain.Message `json:"conversation_history"`
}

type runResponse struct {
	RunID string `json:"run_id"`
}

func (c *Client) Start(ctx context.Context, agent domain.Agent, input string, history []domain.Message, idempotencyKey string) (string, error) {
	endpoint, err := runtimeEndpoint(agent, "/v1/runs")
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(runRequest{Input: input, Instructions: agent.Instructions, ConversationHistory: history})
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt < c.maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
		response, requestErr := c.http.Do(request)
		if requestErr == nil {
			responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil {
				requestErr = errors.Join(readErr, closeErr)
			} else if int64(len(responseBody)) > c.maxResponseBytes {
				requestErr = errors.New("hermes response exceeds size limit")
			} else if response.StatusCode >= 200 && response.StatusCode < 300 {
				var result runResponse
				if err := json.Unmarshal(responseBody, &result); err != nil {
					return "", fmt.Errorf("decode Hermes run response: %w", err)
				}
				if result.RunID == "" {
					return "", errors.New("hermes response has no run_id")
				}
				return result.RunID, nil
			} else {
				requestErr = fmt.Errorf("hermes returned HTTP %d", response.StatusCode)
				if !retryableStatus(response.StatusCode) {
					return "", errors.Join(requestErr, closeErr)
				}
			}
		}
		lastErr = requestErr
		if attempt+1 < c.maxAttempts {
			delay := time.Duration(attempt+1) * c.retryBaseDelay
			if err := waitRetry(ctx, delay); err != nil {
				return "", err
			}
		}
	}
	return "", lastErr
}

type runStateResponse struct {
	Status string `json:"status"`
	Output string `json:"output"`
	Error  string `json:"error"`
}

func (c *Client) Status(ctx context.Context, agent domain.Agent, runID string) (state domain.RunState, resultErr error) {
	endpoint, err := runtimeEndpoint(agent, "/v1/runs/"+url.PathEscape(runID))
	if err != nil {
		return domain.RunState{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.RunState{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.http.Do(request)
	if err != nil {
		return domain.RunState{}, err
	}
	defer func() { resultErr = errors.Join(resultErr, closeResponseBody(response.Body)) }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.RunState{}, fmt.Errorf("hermes returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return domain.RunState{}, err
	}
	if int64(len(body)) > c.maxResponseBytes {
		return domain.RunState{}, errors.New("hermes response exceeds size limit")
	}
	var decoded runStateResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return domain.RunState{}, err
	}
	return domain.RunState{Status: decoded.Status, Output: decoded.Output, Error: decoded.Error}, nil
}

func (c *Client) Health(ctx context.Context, agent domain.Agent) (resultErr error) {
	endpoint, err := runtimeEndpoint(agent, "/health")
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeResponseBody(response.Body)) }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("hermes health returned HTTP %d", response.StatusCode)
	}
	return nil
}

func runtimeEndpoint(agent domain.Agent, path string) (string, error) {
	baseURL, err := url.Parse(strings.TrimSpace(agent.RuntimeURL))
	if err != nil || !baseURL.IsAbs() || baseURL.Host == "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return "", fmt.Errorf("invalid Hermes runtime URL for agent %s", agent.Type)
	}
	return strings.TrimRight(baseURL.String(), "/") + path, nil
}

func retryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func waitRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func closeResponseBody(body io.Closer) error {
	if err := body.Close(); err != nil {
		return fmt.Errorf("close Hermes response body: %w", err)
	}
	return nil
}

var _ domain.AgentRuntime = (*Client)(nil)
