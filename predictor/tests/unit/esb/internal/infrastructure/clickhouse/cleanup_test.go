package clickhouse

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestPruneRetriesAfterMutationFailureAndVerifiesFinalState(t *testing.T) {
	var mu sync.Mutex
	present := true
	mutationAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := strings.Join(strings.Fields(r.URL.Query().Get("query")), " ")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(query, "SELECT c.news_id AS id"):
			if present {
				_, _ = fmt.Fprintln(w, `{"id":"news-1"}`)
			}
		case strings.Contains(query, "ALTER TABLE `default`.news_content DELETE"):
			mutationAttempts++
			if mutationAttempts == 1 {
				http.Error(w, "simulated mutation failure", http.StatusInternalServerError)
				return
			}
			present = false
		case strings.Contains(query, "SELECT count() AS count"):
			count := 0
			if present {
				count = 1
			}
			_, _ = fmt.Fprintf(w, `{"count":%d}`+"\n", count)
		default:
			http.Error(w, "unexpected ClickHouse query", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "default", "predictor", "test-password", ClientConfig{
		RequestTimeout: time.Second, MaxResponseBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewAnalytics(client, 10, 10)
	never := domain.RetentionPolicy{Never: true}
	policy := domain.AnalyticsRetention{
		ProcessedNewsText: domain.RetentionPolicy{Age: time.Hour}, UnprocessedNewsText: never,
		NewsMetadata: never, NewsMarker: never, Facts: never, FactCompany: never,
		MetricObservations: never, MetricTotals: never, CompanyAnalyses: never,
	}

	if _, err := repository.Prune(context.Background(), policy, time.Now().UTC(), nil, 10); err == nil {
		t.Fatal("expected the injected ClickHouse mutation failure")
	}
	mu.Lock()
	if !present || mutationAttempts != 1 {
		mu.Unlock()
		t.Fatalf("failed mutation changed state: present=%t attempts=%d", present, mutationAttempts)
	}
	mu.Unlock()

	counts, err := repository.Prune(context.Background(), policy, time.Now().UTC(), nil, 10)
	if err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if counts["processed_news_text"] != 1 {
		t.Fatalf("cleanup counts = %#v", counts)
	}
	mu.Lock()
	defer mu.Unlock()
	if present || mutationAttempts != 2 {
		t.Fatalf("successful retry was not verified: present=%t attempts=%d", present, mutationAttempts)
	}
}
