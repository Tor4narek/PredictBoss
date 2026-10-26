package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewStoreRejectsInvalidEndpointURL(t *testing.T) {
	for _, endpoint := range []string{"ftp://storage.example", "https://user:secret@storage.example", "http:///missing-host", "https://storage.example?token=unsafe"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := NewStore(context.Background(), Config{
				Bucket: "company-profiles", Region: "us-east-1", Endpoint: endpoint, MaxProfileBytes: 1024,
			})
			if err == nil {
				t.Fatalf("NewStore accepted invalid endpoint %q", endpoint)
			}
		})
	}
}

func TestProfileStoreReadsAndIdempotentlyOverwritesMarkdownObject(t *testing.T) {
	var putBody string
	var putCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/company-profiles/companies/acme/profile.md" {
			t.Errorf("S3 object path = %q", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			if _, err := w.Write([]byte("# Acme\n")); err != nil {
				t.Errorf("write profile response: %v", err)
			}
		case http.MethodPut:
			putCount++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read profile request: %v", err)
			}
			putBody = string(body)
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "text/markdown") {
				t.Errorf("profile Content-Type = %q", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected S3 method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	store := newTestStore(t, server.URL, 128)

	markdown, key, err := store.Get(context.Background(), "acme")
	if err != nil || markdown != "# Acme\n" || key != "companies/acme/profile.md" {
		t.Fatalf("Get = (%q, %q, %v)", markdown, key, err)
	}
	for range 2 {
		if _, err := store.Save(context.Background(), "acme", "  # Updated Acme  "); err != nil {
			t.Fatalf("Save returned error: %v", err)
		}
	}
	if putBody != "# Updated Acme" || putCount != 2 {
		t.Fatalf("repeated Save body/count = %q/%d, want trimmed Markdown and two writes", putBody, putCount)
	}
}

func TestProfileStoreRejectsOversizedAndEmptyProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("12345")); err != nil {
			t.Errorf("write profile body: %v", err)
		}
	}))
	defer server.Close()
	store := newTestStore(t, server.URL, 4)
	if _, _, err := store.Get(context.Background(), "acme"); err == nil {
		t.Fatal("Get accepted a profile above the configured byte limit")
	}
	if _, err := store.Save(context.Background(), "acme", " \n "); err == nil {
		t.Fatal("Save accepted an empty profile")
	}
	if _, err := store.Save(context.Background(), "acme", "12345"); err == nil {
		t.Fatal("Save accepted a profile above the configured byte limit")
	}
	if _, _, err := store.Get(context.Background(), "../other-company"); err == nil {
		t.Fatal("Get accepted a path traversal company ID")
	}
}

func newTestStore(t *testing.T, endpoint string, maxBytes int64) *Store {
	t.Helper()
	store, err := NewStore(context.Background(), Config{
		Bucket: "company-profiles", Region: "us-east-1", Endpoint: endpoint,
		AccessKey: "test-access", SecretKey: "test-secret", PathStyle: true,
		MaxProfileBytes: maxBytes,
	})
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	return store
}
