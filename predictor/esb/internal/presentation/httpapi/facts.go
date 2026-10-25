package httpapi

import (
	"errors"
	"log"
	"net/http"

	"github.com/predictboss/predictor/esb/internal/application/analytics"
)

func (h *factsHandler) getFacts(w http.ResponseWriter, r *http.Request) {
	var request getFactsRequest
	if err := decodeJSONInto(w, r, &request, h.maxJSONBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json_body")
		return
	}
	if request.CompanyID == "" {
		request.CompanyID = request.CompanyIDSnake
	}
	if request.NewsID == "" {
		request.NewsID = request.NewsIDSnake
	}
	if request.SourceID == "" {
		request.SourceID = request.SourceIDSnake
	}
	if request.MaterialKind == "" {
		request.MaterialKind = request.MaterialKindAlt
	}
	if request.PeriodFrom == "" {
		request.PeriodFrom = request.PeriodFromSnake
	}
	if request.PeriodTo == "" {
		request.PeriodTo = request.PeriodToSnake
	}
	if request.BeforeFactID == "" {
		request.BeforeFactID = request.BeforeFactSnake
	}
	page, err := h.analytics.QueryFacts(r.Context(), analytics.FactsRequest{
		CompanyID: request.CompanyID, NewsID: request.NewsID, SourceID: request.SourceID,
		MaterialKind: request.MaterialKind, Category: request.Category, Entity: request.Entity,
		PeriodFrom: request.PeriodFrom, PeriodTo: request.PeriodTo,
		BeforeFactID: request.BeforeFactID, Limit: request.Limit,
	})
	if errors.Is(err, analytics.ErrInvalidAnalyticsInput) {
		writeError(w, http.StatusBadRequest, "invalid_facts_query")
		return
	}
	if err != nil {
		log.Printf("read facts from ClickHouse: %v", err)
		writeError(w, http.StatusInternalServerError, "facts_lookup_failed")
		return
	}
	writeJSON(w, http.StatusOK, factsResponse{
		Data: "facts", Facts: page.Facts, HasMore: page.HasMore,
		NextBeforeFactID: page.NextBeforeID,
	})
}
