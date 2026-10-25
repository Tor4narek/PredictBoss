package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/predictboss/predictor/esb/internal/application/news"
	"github.com/predictboss/predictor/esb/internal/domain"
)

// GenerateOpenAPISpec builds OpenAPI from the Go request/response contracts.
// The route paths are shared with router.go so the generated contract follows
// the routes registered by the HTTP adapter.
func GenerateOpenAPISpec() ([]byte, error) {
	schemas := make(map[string]any)
	for _, definition := range []struct {
		name   string
		typeOf reflect.Type
	}{
		{"ValeraCallBody", reflect.TypeOf(valeraCallBody{})},
		{"ValeraCallEnvelope", reflect.TypeOf(valeraCallEnvelope{})},
		{"SerezhaCallBody", reflect.TypeOf(news.CallInput{})},
		{"SaveliyCallBody", reflect.TypeOf(saveliyCallBody{})},
		{"KeshaCallBody", reflect.TypeOf(keshaCallBody{})},
		{"KonstantinCallBody", reflect.TypeOf(konstantinCallBody{})},
		{"CallLookupBody", reflect.TypeOf(getCallRequest{})},
		{"FactsQueryBody", reflect.TypeOf(getFactsRequest{})},
		{"AcceptedCall", reflect.TypeOf(acceptedCallResponse{})},
		{"Error", reflect.TypeOf(errorResponse{})},
		{"Health", reflect.TypeOf(healthResponse{})},
		{"Call", reflect.TypeOf(domain.Call{})},
		{"CompanyReport", reflect.TypeOf(domain.CompanyReport{})},
		{"ConsultantReady", reflect.TypeOf(consultantReadyResponse{})},
		{"ConsultantProcessing", reflect.TypeOf(consultantProcessingResponse{})},
		{"FactsResponse", reflect.TypeOf(factsResponse{})},
	} {
		schema, err := schemaFor(definition.typeOf)
		if err != nil {
			return nil, fmt.Errorf("generate %s schema: %w", definition.name, err)
		}
		schema["title"] = definition.name
		schemas[definition.name] = schema
	}
	if err := constrainCallSchema(schemas); err != nil {
		return nil, err
	}

	secured := []any{map[string]any{"ESBBearer": []string{}}}
	paths := map[string]any{
		healthRoute: map[string]any{
			strings.ToLower(http.MethodGet): operation("healthz", "Liveness check", "Public process liveness endpoint.", nil, nil,
				map[string]any{"200": response("Service is alive.", ref("Health"))}, nil),
		},
		readyRoute: map[string]any{
			strings.ToLower(http.MethodGet): operation("readyz", "Dependency readiness", "Checks the dependencies required by ESB.", nil, nil,
				withErrors(map[string]any{"200": response("All required dependencies are available.", ref("Health"))}, "503"), secured),
		},
		callRoute: map[string]any{
			strings.ToLower(http.MethodPost): operation("enqueueAgentCall", "Start an asynchronous agent call",
				"Select the agent with the `agent` query parameter. Every accepted call returns `call_id`; read its status with GET /call using the same agent. The JSON body shape depends on the agent.",
				[]any{
					queryParameter("agent", "Agent to invoke.", true, map[string]any{"type": "string", "enum": []string{"serezha", "saveliy", "valera", "kesha", "konstantin"}}),
					headerParameter("Idempotency-Key", "Optional request idempotency key.", false, map[string]any{"type": "string"}),
				}, requestBody("Agent-specific JSON payload.", map[string]any{"anyOf": []any{ref("ValeraCallBody"), ref("ValeraCallEnvelope"), ref("SerezhaCallBody"), ref("SaveliyCallBody"), ref("KeshaCallBody"), ref("KonstantinCallBody")}}),
				withErrors(map[string]any{
					"202": response("The asynchronous call was accepted.", ref("AcceptedCall")),
				}, "400", "401", "409", "500", "503"), secured),
			strings.ToLower(http.MethodGet): operation("readCallOrFacts", "Read a call, company report, facts, or consultant state",
				"Use `agent` to read an agent call/report or `data=facts` to query ClickHouse facts. Filters and IDs are sent in the JSON body, including for GET requests. GET for Konstantin by company returns bounded chat history and queues an analysis if its current result is missing or stale.",
				[]any{
					queryParameter("agent", "Agent whose call or company report is requested. Mutually exclusive with data=facts.", false, map[string]any{"type": "string", "enum": []string{"serezha", "saveliy", "valera", "kesha", "konstantin"}}),
					queryParameter("data", "Set to facts to query ClickHouse facts instead of an agent result.", false, map[string]any{"type": "string", "enum": []string{"facts"}}),
				}, requestBody("Call lookup, consultant history, or fact filters.", map[string]any{"anyOf": []any{ref("CallLookupBody"), ref("FactsQueryBody")}}),
				withErrors(map[string]any{
					"200": response("A call, report, fact page, or ready consultant result.", map[string]any{"oneOf": []any{ref("Call"), ref("CompanyReport"), ref("ConsultantReady"), ref("FactsResponse")}}),
					"202": response("The consultant analysis was queued or is still processing.", ref("ConsultantProcessing")),
				}, "400", "401", "404", "409", "500"), secured),
		},
	}

	document := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "Predictor ESB HTTP API",
			"version":     apiVersion,
			"description": "HTTP contract for Domain integrations. WebSocket messages are documented in asyncapi.json; MCP tools are documented in mcp-tools.md.",
		},
		"servers": []any{map[string]any{"url": "http://localhost:8081"}},
		"tags": []any{
			map[string]any{"name": "health", "description": "Service health."},
			map[string]any{"name": "calls", "description": "Asynchronous agent calls and their results."},
			map[string]any{"name": "facts", "description": "Read-only ClickHouse fact queries."},
		},
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{"ESBBearer": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "ESB_API_KEY"}},
			"schemas":         schemas,
		},
	}
	return json.MarshalIndent(document, "", "  ")
}
