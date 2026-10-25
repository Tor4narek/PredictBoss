package httpapi

import (
	"context"
	"net/http"

	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/calls"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/application/news"
)

const (
	healthRoute = "/healthz"
	readyRoute  = "/readyz"
	callRoute   = "/call"
	streamRoute = "/stream"
)

type ReadyCheck func(context.Context) error

type callHandler struct {
	calls          *calls.Service
	analytics      *analytics.Service
	companyContext *companycontext.Service
	news           *news.Service
	facts          *factsHandler
}

type factsHandler struct{ analytics *analytics.Service }
type consultantHandler struct{ calls *calls.Service }
type readinessHandler struct{ ready ReadyCheck }

func NewHandler(apiKey string, callsService *calls.Service, analyticsService *analytics.Service, companyContextService *companycontext.Service, newsService *news.Service, ready ReadyCheck) http.Handler {
	mux := http.NewServeMux()
	callAPI := &callHandler{
		calls: callsService, analytics: analyticsService, companyContext: companyContextService,
		news: newsService, facts: &factsHandler{analytics: analyticsService},
	}
	consultantAPI := &consultantHandler{calls: callsService}
	readinessAPI := &readinessHandler{ready: ready}

	mux.HandleFunc("GET "+healthRoute, health)
	mux.Handle("GET "+readyRoute, authorize(apiKey, http.HandlerFunc(readinessAPI.readiness)))
	mux.Handle("POST "+callRoute, authorize(apiKey, http.HandlerFunc(callAPI.postCall)))
	mux.Handle("GET "+callRoute, authorize(apiKey, http.HandlerFunc(callAPI.getCall)))
	mux.Handle("GET "+streamRoute, authorize(apiKey, http.HandlerFunc(consultantAPI.stream)))
	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
