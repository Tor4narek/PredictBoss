package httpapi

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/predictboss/predictor/esb/internal/domain"
)

// GenerateAsyncAPISpec describes the Konstantin-only WebSocket protocol using
// the same Go frame structures decoded and emitted by stream.go.
func GenerateAsyncAPISpec() ([]byte, error) {
	schemas := make(map[string]any)
	for _, definition := range []struct {
		name   string
		typeOf reflect.Type
	}{
		{"Question", reflect.TypeOf(questionFrame{})},
		{"Accepted", reflect.TypeOf(acceptedFrame{})},
		{"StreamError", reflect.TypeOf(errorFrame{})},
		{"ConsultantEvent", reflect.TypeOf(domain.ConsultantEvent{})},
	} {
		schema, err := schemaFor(definition.typeOf)
		if err != nil {
			return nil, fmt.Errorf("generate AsyncAPI %s schema: %w", definition.name, err)
		}
		schema["title"] = definition.name
		schemas[definition.name] = schema
	}
	setEnum(schemas["Question"], "kind", []string{"question"})
	setEnum(schemas["Accepted"], "kind", []string{"accepted"})
	setEnum(schemas["StreamError"], "kind", []string{"error"})
	setEnum(schemas["ConsultantEvent"], "kind", []string{"analysis_result", "assistant_answer"})

	document := map[string]any{
		"asyncapi": "2.6.0",
		"info": map[string]any{
			"title":       "Predictor ESB Konstantin WebSocket",
			"version":     apiVersion,
			"description": "The only WebSocket API in ESB. The client subscribes to a company conversation and may send question frames.",
		},
		"servers": map[string]any{
			"local": map[string]any{"url": "localhost:8081", "protocol": "ws", "description": "Send Authorization: Bearer $ESB_API_KEY during the handshake."},
		},
		"channels": map[string]any{
			streamRoute: map[string]any{
				"description": "Konstantin's company-scoped conversation. Connect using the streamUrl returned by GET /call?agent=konstantin.",
				"parameters": map[string]any{
					"companyId":      map[string]any{"description": "Company UUID.", "schema": map[string]any{"type": "string", "format": "uuid"}},
					"conversationId": map[string]any{"description": "Company-scoped conversation ID.", "schema": map[string]any{"type": "string", "format": "uuid"}},
					"requestId":      map[string]any{"description": "Optional pending analysis request to await.", "schema": map[string]any{"type": "string", "format": "uuid"}},
					"afterMessageId": map[string]any{"description": "Optional replay cursor; only later persisted messages are sent.", "schema": map[string]any{"type": "string", "format": "uuid"}},
				},
				"publish": map[string]any{
					"operationId": "askKonstantin",
					"summary":     "Send a question to Konstantin.",
					"message":     map[string]any{"oneOf": []any{map[string]any{"$ref": "#/components/messages/Question"}}},
				},
				"subscribe": map[string]any{
					"operationId": "receiveKonstantinEvents",
					"summary":     "Receive analysis, answers, acknowledgements, and errors.",
					"message": map[string]any{"oneOf": []any{
						map[string]any{"$ref": "#/components/messages/ConsultantEvent"},
						map[string]any{"$ref": "#/components/messages/Accepted"},
						map[string]any{"$ref": "#/components/messages/StreamError"},
					}},
				},
			},
		},
		"components": map[string]any{
			"messages": map[string]any{
				"Question":        map[string]any{"name": "Question", "contentType": "application/json", "payload": ref("Question")},
				"Accepted":        map[string]any{"name": "Accepted", "contentType": "application/json", "payload": ref("Accepted")},
				"StreamError":     map[string]any{"name": "StreamError", "contentType": "application/json", "payload": ref("StreamError")},
				"ConsultantEvent": map[string]any{"name": "ConsultantEvent", "contentType": "application/json", "payload": ref("ConsultantEvent")},
			},
			"schemas": schemas,
		},
	}
	return json.MarshalIndent(document, "", "  ")
}
