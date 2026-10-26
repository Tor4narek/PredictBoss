package mcp

import "testing"

func TestNormalizeOccurredAt(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		want    string
		wantNil bool
	}{
		{name: "nil", wantNil: true},
		{name: "empty", input: stringPointer(" "), wantNil: true},
		{name: "date only", input: stringPointer("2026-10-26"), wantNil: true},
		{name: "ambiguous time", input: stringPointer("2026-10-26 09:15:00"), wantNil: true},
		{name: "unparseable", input: stringPointer("26 October"), wantNil: true},
		{name: "RFC3339", input: stringPointer("2026-10-26T09:15:00+03:00"), want: "2026-10-26T09:15:00+03:00"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeOccurredAt(test.input)
			if test.wantNil {
				if got != nil {
					t.Fatalf("normalizeOccurredAt = %q, want nil", *got)
				}
				return
			}
			if got == nil || *got != test.want {
				t.Fatalf("normalizeOccurredAt = %v, want %q", got, test.want)
			}
		})
	}
}

func stringPointer(value string) *string {
	return &value
}
