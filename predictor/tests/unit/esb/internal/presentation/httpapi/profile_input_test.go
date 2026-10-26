package httpapi

import (
	"encoding/json"
	"testing"
)

func TestNormalizeValeraInputFlattensPayloadAndNormalizesCompanyID(t *testing.T) {
	input := json.RawMessage(`{"payload":{"companyId":"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA","profileRevision":2,"company":{"name":"Acme"}}}`)
	got, err := normalizeValeraInput(input)
	if err != nil {
		t.Fatalf("normalizeValeraInput returned error: %v", err)
	}
	var decoded struct {
		CompanyID       string          `json:"company_id"`
		ProfileRevision int64           `json:"profile_revision"`
		CompanyData     json.RawMessage `json:"company_data"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized input is invalid JSON: %v", err)
	}
	if decoded.CompanyID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || decoded.ProfileRevision != 2 || string(decoded.CompanyData) != `{"name":"Acme"}` {
		t.Fatalf("normalized Valera input = %#v", decoded)
	}
}

func TestNormalizeValeraInputRejectsIncompleteOrInvalidRevision(t *testing.T) {
	for _, input := range []string{
		`{"companyId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`,
		`{"companyId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","profileRevision":0,"company":{}}`,
		`{"companyId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","profileRevision":1.5,"company":{}}`,
		`{"payload":null}`,
		`null`,
		`[]`,
		`{"companyId":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","profileRevision":1,"company":null}`,
		`not-json`,
	} {
		if _, err := normalizeValeraInput(json.RawMessage(input)); err == nil {
			t.Errorf("normalizeValeraInput accepted %s", input)
		}
	}
}

func TestCallRequestIDHandlesValidAndMalformedPayload(t *testing.T) {
	if got := callRequestID(json.RawMessage(`{"request_id":"req-1"}`)); got != "req-1" {
		t.Fatalf("callRequestID = %q, want req-1", got)
	}
	if got := callRequestID(json.RawMessage(`not-json`)); got != "" {
		t.Fatalf("malformed call request ID = %q, want empty", got)
	}
}
