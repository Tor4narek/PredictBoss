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
		{"Cancel", reflect.TypeOf(cancelFrame{})},
		{"CancelRequested", reflect.TypeOf(cancelRequestedFrame{})},
		{"CancelFailed", reflect.TypeOf(cancelFailedFrame{})},
		{"AssistantCancelled", reflect.TypeOf(assistantCancelledFrame{})},
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
	setEnum(schemas["Cancel"], "kind", []string{"cancel"})
	setEnum(schemas["CancelRequested"], "kind", []string{"cancel_requested"})
	setEnum(schemas["CancelFailed"], "kind", []string{"cancel_failed"})
	setEnum(schemas["AssistantCancelled"], "kind", []string{"assistant_cancelled"})
	setEnum(schemas["StreamError"], "kind", []string{"error"})
	setEnum(schemas["ConsultantEvent"], "kind", []string{"analysis_result", "assistant_answer"})

	document := map[string]any{
		"asyncapi": "2.6.0",
		"info": map[string]any{
			"title":       "Predictor ESB Konstantin WebSocket",
			"version":     apiVersion,
			"description": "The only WebSocket API in ESB. Questions are answered with one saved assistant_answer message containing only final user-facing text and verified evidence; internal reasoning and token deltas are never emitted. The client may cancel one request by requestId.",
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
					"afterMessageId": map[string]any{"description": "Optional replay cursor; only later persisted messages are sent.", "schema": map[string]any{"type": "string", "format": "uuid"}},
					"requestId":      map[string]any{"description": "Optional request to observe until its one-shot answer or terminal state.", "schema": map[string]any{"type": "string", "format": "uuid"}},
				},
				"publish": map[string]any{
					"operationId": "askKonstantin",
					"summary":     "Send a question to Konstantin.",
					"message": map[string]any{"oneOf": []any{
						map[string]any{"$ref": "#/components/messages/Question"},
						map[string]any{"$ref": "#/components/messages/Cancel"},
					}},
				},
				"subscribe": map[string]any{
					"operationId": "receiveKonstantinEvents",
					"summary":     "Receive one complete final answer in text, or cancellation lifecycle events. Internal reasoning and token deltas are not emitted.",
					"message": map[string]any{"oneOf": []any{
						map[string]any{"$ref": "#/components/messages/ConsultantEvent"},
						map[string]any{"$ref": "#/components/messages/Accepted"},
						map[string]any{"$ref": "#/components/messages/CancelRequested"},
						map[string]any{"$ref": "#/components/messages/CancelFailed"},
						map[string]any{"$ref": "#/components/messages/AssistantCancelled"},
						map[string]any{"$ref": "#/components/messages/StreamError"},
					}},
				},
			},
		},
		"components": map[string]any{
			"messages": map[string]any{
				"Question":           map[string]any{"name": "Question", "contentType": "application/json", "payload": ref("Question")},
				"Accepted":           map[string]any{"name": "Accepted", "contentType": "application/json", "payload": ref("Accepted")},
				"Cancel":             map[string]any{"name": "Cancel", "contentType": "application/json", "payload": ref("Cancel")},
				"CancelRequested":    map[string]any{"name": "CancelRequested", "contentType": "application/json", "payload": ref("CancelRequested")},
				"CancelFailed":       map[string]any{"name": "CancelFailed", "contentType": "application/json", "payload": ref("CancelFailed")},
				"AssistantCancelled": map[string]any{"name": "AssistantCancelled", "contentType": "application/json", "payload": ref("AssistantCancelled")},
				"StreamError":        map[string]any{"name": "StreamError", "contentType": "application/json", "payload": ref("StreamError")},
				"ConsultantEvent":    map[string]any{"name": "ConsultantEvent", "contentType": "application/json", "payload": ref("ConsultantEvent")},
			},
			"schemas": schemas,
		},
	}
	return json.MarshalIndent(document, "", "  ")
}
