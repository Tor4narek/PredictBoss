package mcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
)

func NewStorageHandler(service *analytics.Service, companyContext *companycontext.Service, apiKey string) http.Handler {
	server := NewStorageServer(service, companyContext)
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(w, r)
	})
}

// NewStorageServer is also used by the API documentation generator to discover
// the live tool schemas without initializing any storage connections.
func NewStorageServer(service *analytics.Service, companyContext *companycontext.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "predictor-esb", Version: "0.3.0"}, nil)
	registerCompanyProfileTools(server, companyContext)
	registerFactTools(server, service)
	registerMetricTools(server, service)
	registerReportTools(server, service)
	return server
}
