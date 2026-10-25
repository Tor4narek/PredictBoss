package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

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

func NewClient(apiKey string, config ClientConfig) *Client {
	return &Client{apiKey: apiKey, http: &http.Client{Timeout: config.RequestTimeout}, maxAttempts: config.MaxAttempts,
		retryBaseDelay: config.RetryBaseDelay, maxResponseBytes: config.MaxResponseBytes}
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
	body, err := json.Marshal(runRequest{Input: input, Instructions: agent.Instructions, ConversationHistory: history})
	if err != nil {
		return "", err
	}
	var lastErr error
	for attempt := 0; attempt < c.maxAttempts; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(agent.RuntimeURL, "/")+"/v1/runs", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", idempotencyKey)
		response, requestErr := c.http.Do(request)
		if requestErr == nil {
			responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
			response.Body.Close()
			if readErr == nil && int64(len(responseBody)) > c.maxResponseBytes {
				requestErr = fmt.Errorf("Hermes response exceeds size limit")
			} else if readErr == nil && response.StatusCode >= 200 && response.StatusCode < 300 {
				var result runResponse
				if err := json.Unmarshal(responseBody, &result); err == nil && result.RunID != "" {
					return result.RunID, nil
				}
				requestErr = fmt.Errorf("Hermes response has no run_id")
			} else if readErr != nil {
				requestErr = readErr
			} else {
				requestErr = fmt.Errorf("Hermes returned HTTP %d", response.StatusCode)
			}
		}
		lastErr = requestErr
		if attempt+1 < c.maxAttempts {
			delay := time.Duration(attempt+1) * c.retryBaseDelay
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
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

func (c *Client) Status(ctx context.Context, agent domain.Agent, runID string) (domain.RunState, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(agent.RuntimeURL, "/")+"/v1/runs/"+runID, nil)
	if err != nil {
		return domain.RunState{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.http.Do(request)
	if err != nil {
		return domain.RunState{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.RunState{}, fmt.Errorf("Hermes returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxResponseBytes+1))
	if err != nil {
		return domain.RunState{}, err
	}
	if int64(len(body)) > c.maxResponseBytes {
		return domain.RunState{}, fmt.Errorf("Hermes response exceeds size limit")
	}
	var state runStateResponse
	if err := json.Unmarshal(body, &state); err != nil {
		return domain.RunState{}, err
	}
	return domain.RunState{Status: state.Status, Output: state.Output, Error: state.Error}, nil
}

func (c *Client) Health(ctx context.Context, agent domain.Agent) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(agent.RuntimeURL, "/")+"/health", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Hermes health returned HTTP %d", response.StatusCode)
	}
	return nil
}

var _ domain.AgentRuntime = (*Client)(nil)
