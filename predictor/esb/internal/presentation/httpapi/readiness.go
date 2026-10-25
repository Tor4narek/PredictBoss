package httpapi

import (
	"net/http"
)

func (h *readinessHandler) readiness(w http.ResponseWriter, r *http.Request) {
	if h.ready != nil {
		if err := h.ready(r.Context()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "dependency_unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ready"})
}

func authorize(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}
