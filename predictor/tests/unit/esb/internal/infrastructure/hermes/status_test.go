package hermes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestStatusDecodesRunStateAndHandlesHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want test-key bearer", r.Header.Get("Authorization"))
		}
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, err := w.Write([]byte(`{"status":"completed","output":"saved","error":""}`))
		if err != nil {
			t.Errorf("write status response: %v", err)
		}
	}))
	defer server.Close()
	client := testClient(t, 1)
	agent := domain.Agent{Type: domain.AgentKesha, RuntimeURL: server.URL}

	state, err := client.Status(context.Background(), agent, "run-1")
	if err != nil || state.Status != "completed" || state.Output != "saved" {
		t.Fatalf("Status = %#v, %v", state, err)
	}
	if _, err := client.Status(context.Background(), agent, "missing"); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("not-found status error = %v, want HTTP 404", err)
	}
}

func TestHealthUsesBoundedAuthenticatedRequest(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{name: "ready", statusCode: http.StatusNoContent},
		{name: "unavailable", statusCode: http.StatusServiceUnavailable, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health" || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("health request path/auth = %q/%q", r.URL.Path, r.Header.Get("Authorization"))
				}
				w.WriteHeader(test.statusCode)
			}))
			defer server.Close()
			client := testClient(t, 1)
			err := client.Health(context.Background(), domain.Agent{Type: domain.AgentValera, RuntimeURL: server.URL})
			if (err != nil) != test.wantErr {
				t.Fatalf("Health error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestStatusHonorsContextCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := testClient(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Status(ctx, domain.Agent{Type: domain.AgentKesha, RuntimeURL: server.URL}, "run-cancel")
		result <- err
	}()
	<-requestStarted
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Status error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Status did not stop after context cancellation")
	}
}
