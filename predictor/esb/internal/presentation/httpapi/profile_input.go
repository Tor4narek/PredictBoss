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
	if nested, ok := body["payload"]; ok {
		if err := json.Unmarshal(nested, &body); err != nil {
			return nil, err
		}
	}
	companyID := body["companyId"]
	if len(companyID) == 0 {
		return input, nil
	}
	profileRevision := body["profileRevision"]
	company := body["company"]
	if len(profileRevision) == 0 || len(company) == 0 {
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
	_ = json.Unmarshal(raw, &value)
	return value.RequestID
}
