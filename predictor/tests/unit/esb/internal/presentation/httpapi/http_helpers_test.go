package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRequiresOneBoundedObject(t *testing.T) {
	tests := []struct {
		name string
		body string
		max  int64
		want string
	}{
		{name: "object", body: `{"companyId":"acme"}`, max: 128, want: `{"companyId":"acme"}`},
		{name: "array is not an object", body: `[]`, max: 128},
		{name: "trailing JSON value", body: `{} {}`, max: 128},
		{name: "oversized body", body: `{"text":"too long"}`, max: 4},
		{name: "malformed JSON", body: `{"companyId":`, max: 128},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/call", strings.NewReader(test.body))
			decoded, err := decodeJSONBody(httptest.NewRecorder(), request, test.max)
			if test.want != "" {
				if err != nil || string(decoded) != test.want {
					t.Fatalf("decodeJSONBody = %q, %v; want %q", decoded, err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatal("decodeJSONBody accepted invalid input")
			}
		})
	}
}

func TestAuthorizeRejectsMissingOrIncorrectBearerToken(t *testing.T) {
	protected := authorize("expected", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, authorization := range []string{"", "Bearer wrong", "expected"} {
		request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		protected.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "unauthorized") {
			t.Errorf("authorization %q got status %d and body %q", authorization, response.Code, response.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	request.Header.Set("Authorization", "Bearer expected")
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("valid bearer token got status %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestReadinessReportsDependencyFailureWithoutLeakingError(t *testing.T) {
	handler := &readinessHandler{ready: func(_ context.Context) error { return nil }}
	request := httptest.NewRequest(http.MethodGet, readyRoute, nil)
	response := httptest.NewRecorder()
	handler.readiness(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ready"`) {
		t.Fatalf("healthy readiness response = %d %s", response.Code, response.Body.String())
	}

	handler.ready = func(_ context.Context) error { return errors.New("database connection string leaked") }
	response = httptest.NewRecorder()
	handler.readiness(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "dependency_unavailable") || strings.Contains(response.Body.String(), "connection string") {
		t.Fatalf("dependency failure leaked details or returned wrong status: %d %s", response.Code, response.Body.String())
	}
}

func TestWriteErrorUsesStableJSONContract(t *testing.T) {
	response := httptest.NewRecorder()
	writeError(response, http.StatusConflict, "profile_conflict")
	if response.Code != http.StatusConflict || response.Header().Get("Content-Type") != "application/json" || response.Body.String() != "{\"error\":\"profile_conflict\"}\n" {
		t.Fatalf("writeError response = %d %q %q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}
