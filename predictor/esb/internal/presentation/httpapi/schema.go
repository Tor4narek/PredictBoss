package httpapi

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

const apiVersion = "0.3.0"

var schemaOverrides = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeOf((*any)(nil)).Elem(): {},
	reflect.TypeOf(json.RawMessage{}): {
		Type:                 "object",
		AdditionalProperties: &jsonschema.Schema{},
	},
}

func schemaFor(t reflect.Type) (map[string]any, error) {
	schema, err := jsonschema.ForType(t, &jsonschema.ForOptions{TypeSchemas: schemaOverrides})
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	delete(result, "$schema") // OpenAPI defines its own JSON Schema dialect.
	return result, nil
}

func constrainCallSchema(schemas map[string]any) error {
	for _, name := range []string{"Call", "AcceptedCall"} {
		schema, ok := schemas[name].(map[string]any)
		if !ok {
			return fmt.Errorf("schema %s is not an object", name)
		}
		setEnum(schema, "agent", []string{"serezha", "saveliy", "valera", "kesha", "konstantin"})
	}
	return nil
}

func setEnum(schemaValue any, property string, values []string) {
	schema, ok := schemaValue.(map[string]any)
	if !ok {
		return
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	field, ok := properties[property].(map[string]any)
	if !ok {
		return
	}
	field["enum"] = values
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func response(description string, schema any) map[string]any {
	return map[string]any{"description": description, "content": map[string]any{"application/json": map[string]any{"schema": schema}}}
}

func requestBody(description string, schema any) map[string]any {
	return map[string]any{"required": true, "description": description, "content": map[string]any{"application/json": map[string]any{"schema": schema}}}
}

func queryParameter(name, description string, required bool, schema any) map[string]any {
	return map[string]any{"name": name, "in": "query", "required": required, "description": description, "schema": schema}
}

func headerParameter(name, description string, required bool, schema any) map[string]any {
	return map[string]any{"name": name, "in": "header", "required": required, "description": description, "schema": schema}
}

func operation(id, summary, description string, parameters []any, body any, responses map[string]any, security []any) map[string]any {
	result := map[string]any{"operationId": id, "summary": summary, "description": description, "responses": responses}
	if id == "healthz" || id == "readyz" {
		result["tags"] = []string{"health"}
	} else {
		result["tags"] = []string{"calls"}
	}
	if parameters != nil {
		result["parameters"] = parameters
	}
	if body != nil {
		result["requestBody"] = body
	}
	if security != nil {
		result["security"] = security
	}
	return result
}

func withErrors(base map[string]any, codes ...string) map[string]any {
	for _, code := range codes {
		message := "Request failed."
		switch code {
		case "400":
			message = "Invalid request or JSON body."
		case "401":
			message = "Bearer token is missing or invalid."
		case "404":
			message = "Requested call or conversation was not found."
		case "409":
			message = "The request conflicts with current profile or source state."
		case "500":
			message = "ESB could not complete the request."
		case "503":
			message = "A required dependency is unavailable."
		}
		base[code] = response(message, ref("Error"))
	}
	return base
}
