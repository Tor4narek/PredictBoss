package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientQueryAndInsertUseBoundedAuthenticatedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "predictor" || password != "test-password" {
			t.Errorf("ClickHouse basic auth = (%q, %q, %t)", user, password, ok)
		}
		query := r.URL.Query().Get("query")
		if strings.HasPrefix(query, "INSERT INTO") {
			if !strings.Contains(query, "`analytics`.`facts`") {
				t.Errorf("insert query = %q", query)
			}
			var row map[string]any
			if err := json.NewDecoder(r.Body).Decode(&row); err != nil || row["fact_id"] != "fact-1" {
				t.Errorf("insert body row = %#v, error = %v", row, err)
			}
			return
		}
		if query != "SELECT 1" {
			t.Errorf("query = %q, want SELECT 1", query)
		}
		_, err := w.Write([]byte("1\n"))
		if err != nil {
			t.Errorf("write query result: %v", err)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "analytics", "predictor", "test-password", ClientConfig{RequestTimeout: time.Second, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.Query(context.Background(), "SELECT 1")
	if err != nil || string(body) != "1\n" {
		t.Fatalf("Query = %q, %v", body, err)
	}
	if err := client.Insert(context.Background(), "facts", []map[string]any{{"fact_id": "fact-1"}}); err != nil {
		t.Fatalf("Insert returned error: %v", err)
	}
	if err := client.Insert(context.Background(), "system.users", []map[string]any{}); err == nil {
		t.Fatal("Insert accepted a table outside the analytics allowlist")
	}
}

func TestQueryRowsParsesJSONEachRowAndRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("query"), "large") {
			_, err := w.Write([]byte(strings.Repeat("x", 60)))
			if err != nil {
				t.Errorf("write oversized response: %v", err)
			}
			return
		}
		_, err := w.Write([]byte("{\"id\":\"one\"}\n{\"id\":\"two\"}\n"))
		if err != nil {
			t.Errorf("write JSONEachRow response: %v", err)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "analytics", "", "", ClientConfig{RequestTimeout: time.Second, MaxResponseBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := client.queryRows(context.Background(), "SELECT ids", &rows); err != nil {
		t.Fatalf("queryRows returned error: %v", err)
	}
	if len(rows) != 2 || rows[0].ID != "one" || rows[1].ID != "two" {
		t.Fatalf("queryRows parsed %#v", rows)
	}
	if err := client.queryRows(context.Background(), "SELECT large", &rows); err == nil {
		t.Fatal("queryRows accepted a response above the configured byte limit")
	}
}

func TestNewClientRejectsUnsafeURLAndInvalidLimits(t *testing.T) {
	for _, test := range []struct {
		name string
		url  string
		cfg  ClientConfig
	}{
		{name: "relative URL", url: "/clickhouse", cfg: ClientConfig{RequestTimeout: time.Second, MaxResponseBytes: 1024}},
		{name: "URL user info", url: "http://user:pass@localhost:8123", cfg: ClientConfig{RequestTimeout: time.Second, MaxResponseBytes: 1024}},
		{name: "URL query", url: "http://localhost:8123?query=SELECT", cfg: ClientConfig{RequestTimeout: time.Second, MaxResponseBytes: 1024}},
		{name: "zero timeout", url: "http://localhost:8123", cfg: ClientConfig{MaxResponseBytes: 1024}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewClient(test.url, "analytics", "", "", test.cfg); err == nil {
				t.Fatal("NewClient accepted invalid connection settings")
			}
		})
	}
}

func TestQueryHonorsRequestCancellation(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "analytics", "", "", ClientConfig{RequestTimeout: time.Second, MaxResponseBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, queryErr := client.Query(ctx, "SELECT 1")
		result <- queryErr
	}()
	<-requestStarted
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled query error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ClickHouse query did not return after cancellation")
	}
}
