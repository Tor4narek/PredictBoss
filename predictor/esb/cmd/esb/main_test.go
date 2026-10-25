package main

import (
	"encoding/json"
	"testing"
)

func TestMaxHistoryLimitDefaultsButPreservesZero(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int
		bad  bool
	}{
		{name: "omitted uses default", json: `{}`, want: defaultHistoryLimit},
		{name: "explicit zero disables history", json: `{"max_history_messages":0}`, want: 0},
		{name: "positive limit is preserved", json: `{"max_history_messages":12}`, want: 12},
		{name: "negative limit is invalid", json: `{"max_history_messages":-1}`, bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var definition agentFile
			if err := json.Unmarshal([]byte(tt.json), &definition); err != nil {
				t.Fatal(err)
			}
			got, err := maxHistoryLimit(definition)
			if tt.bad {
				if err == nil {
					t.Fatal("expected a negative limit to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("history limit = %d, want %d", got, tt.want)
			}
		})
	}
}
