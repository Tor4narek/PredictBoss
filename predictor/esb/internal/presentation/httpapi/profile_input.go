package httpapi

import (
	"encoding/json"
	"errors"
	"strings"
)

func normalizeValeraInput(input json.RawMessage) (json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(input, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, errors.New("valera request must be a JSON object")
	}
	if nested, ok := body["payload"]; ok {
		if err := json.Unmarshal(nested, &body); err != nil {
			return nil, err
		}
		if body == nil {
			return nil, errors.New("valera payload must be a JSON object")
		}
	}
	companyID := body["companyId"]
	profileRevision := body["profileRevision"]
	company := body["company"]
	if len(companyID) == 0 || len(profileRevision) == 0 || len(company) == 0 {
		return nil, errors.New("companyId, profileRevision and company are required")
	}
	var revision int64
	if err := json.Unmarshal(profileRevision, &revision); err != nil || revision < 1 {
		return nil, errors.New("profileRevision must be positive")
	}
	var id string
	if err := json.Unmarshal(companyID, &id); err != nil {
		return nil, err
	}
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return nil, errors.New("companyId must not be empty")
	}
	var companyObject map[string]json.RawMessage
	if err := json.Unmarshal(company, &companyObject); err != nil || companyObject == nil {
		return nil, errors.New("company must be a JSON object")
	}
	return json.Marshal(struct {
		CompanyID       string          `json:"company_id"`
		ProfileRevision int64           `json:"profile_revision"`
		CompanyData     json.RawMessage `json:"company_data"`
	}{id, revision, company})
}

func callRequestID(raw json.RawMessage) string {
	var value struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value.RequestID
}
