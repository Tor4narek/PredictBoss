package hermes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestStartRetriesTemporaryFailuresWithSameIdempotencyKey(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Idempotency-Key"); got != "call-123" {
			t.Errorf("idempotency key = %q, want call-123", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization header = %q", got)
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"run_id":"run-123"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-key", ClientConfig{
		RequestTimeout: time.Second, MaxAttempts: 2, RetryBaseDelay: time.Millisecond, MaxResponseBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	runID, err := client.Start(context.Background(), domain.Agent{Type: domain.AgentSerezha, RuntimeURL: server.URL}, "{}", nil, "call-123")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run-123" || attempts.Load() != 2 {
		t.Fatalf("runID=%q attempts=%d, want run-123 after one retry", runID, attempts.Load())
	}
}

func TestStartDoesNotRetryPermanentFailure(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	client := testClient(t, 3)

	_, err := client.Start(context.Background(), domain.Agent{Type: domain.AgentSerezha, RuntimeURL: server.URL}, "{}", nil, "call-123")
	if err == nil || attempts.Load() != 1 {
		t.Fatalf("permanent failure err=%v attempts=%d, want error and one attempt", err, attempts.Load())
	}
}

func TestStartDoesNotFollowRedirect(t *testing.T) {
	var destinationCalls atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationCalls.Add(1)
		_, _ = w.Write([]byte(`{"run_id":"unexpected"}`))
	}))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/v1/runs", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client := testClient(t, 1)

	_, err := client.Start(context.Background(), domain.Agent{Type: domain.AgentSerezha, RuntimeURL: redirect.URL}, "{}", nil, "call-123")
	if err == nil || destinationCalls.Load() != 0 {
		t.Fatalf("redirect err=%v destination calls=%d, want blocked redirect", err, destinationCalls.Load())
	}
}

func TestStartRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"run_id": "this-response-is-too-large"})
	}))
	defer server.Close()
	client, err := NewClient("test-key", ClientConfig{
		RequestTimeout: time.Second, MaxAttempts: 1, RetryBaseDelay: 0, MaxResponseBytes: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Start(context.Background(), domain.Agent{Type: domain.AgentSerezha, RuntimeURL: server.URL}, "{}", nil, "call-123"); err == nil {
		t.Fatal("Start accepted a response that exceeded the configured limit")
	}
}

func TestStopRequestsInterruptionWithoutTreatingAcceptanceAsCompletion(t *testing.T) {
	var stopCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs/run-123/stop" {
			t.Errorf("stop request = %s %s, want POST /v1/runs/run-123/stop", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization header = %q", got)
		}
		stopCalls.Add(1)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"stopping"}`))
	}))
	defer server.Close()

	client, err := NewClient("test-key", ClientConfig{
		RequestTimeout: time.Second, StopTimeout: time.Second,
		MaxAttempts: 1, MaxResponseBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Stop(context.Background(), domain.Agent{Type: domain.AgentKonstantin, RuntimeURL: server.URL}, "run-123"); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if stopCalls.Load() != 1 {
		t.Fatalf("stop requests = %d, want 1", stopCalls.Load())
	}
}

func TestStopReportsOnlyStatusForHermesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`sensitive runtime response`))
	}))
	defer server.Close()
	client, err := NewClient("test-key", ClientConfig{
		RequestTimeout: time.Second, StopTimeout: time.Second,
		MaxAttempts: 1, MaxResponseBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Stop(context.Background(), domain.Agent{Type: domain.AgentKonstantin, RuntimeURL: server.URL}, "run-123")
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("Stop error = %v, want status only without response body", err)
	}
}

func testClient(t *testing.T, attempts int) *Client {
	t.Helper()
	client, err := NewClient("test-key", ClientConfig{
		RequestTimeout: time.Second, MaxAttempts: attempts, RetryBaseDelay: time.Millisecond, MaxResponseBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
