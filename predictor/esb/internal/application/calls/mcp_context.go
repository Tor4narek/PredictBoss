package calls

import (
	"encoding/json"
	"errors"
	"time"
)

const mcpCallContextLifetime = 30 * time.Minute

func addMCPCallContext(input json.RawMessage, callContext string) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil || callContext == "" {
		return "", errors.New("cannot attach MCP context to call input")
	}
	encoded, err := json.Marshal(callContext)
	if err != nil {
		return "", err
	}
	fields["call_context"] = encoded
	updated, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return string(updated), nil
}
