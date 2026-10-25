package httpapi

import (
	"context"
	"net/http"
	"time"

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
	calls            *calls.Service
	analytics        *analytics.Service
	companyContext   *companycontext.Service
	news             *news.Service
	facts            *factsHandler
	maxJSONBodyBytes int64
}

type factsHandler struct {
	analytics        *analytics.Service
	maxJSONBodyBytes int64
}
type consultantHandler struct {
	calls         *calls.Service
	maxFrameBytes int64
	readTimeout   time.Duration
	writeTimeout  time.Duration
	outgoingQueue int
	replayLimit   int
}
type readinessHandler struct{ ready ReadyCheck }

type Config struct {
	MaxJSONBodyBytes int64
	MaxFrameBytes    int64
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	OutgoingQueue    int
	ReplayLimit      int
}

func NewHandler(apiKey string, callsService *calls.Service, analyticsService *analytics.Service, companyContextService *companycontext.Service, newsService *news.Service, ready ReadyCheck, config Config) http.Handler {
	mux := http.NewServeMux()
	callAPI := &callHandler{
		calls: callsService, analytics: analyticsService, companyContext: companyContextService,
		news: newsService, facts: &factsHandler{analytics: analyticsService, maxJSONBodyBytes: config.MaxJSONBodyBytes},
		maxJSONBodyBytes: config.MaxJSONBodyBytes,
	}
	consultantAPI := &consultantHandler{calls: callsService, maxFrameBytes: config.MaxFrameBytes,
		readTimeout: config.ReadTimeout, writeTimeout: config.WriteTimeout,
		outgoingQueue: config.OutgoingQueue, replayLimit: config.ReplayLimit}
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
