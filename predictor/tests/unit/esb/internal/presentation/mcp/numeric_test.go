package mcp

import (
	"encoding/json"
	"math"
	"testing"
)

func TestNumericValueAcceptsSupportedScalarsAndRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []any{float64(1.25), float32(2.5), int(3), int64(4), json.Number("5.5"), " 6.25 "} {
		if _, err := numericValue(value); err != nil {
			t.Errorf("numericValue(%v) returned error: %v", value, err)
		}
	}
	for _, value := range []any{math.NaN(), math.Inf(1), "not-a-number", true, []int{1}} {
		if _, err := numericValue(value); err == nil {
			t.Errorf("numericValue(%v) accepted an unsupported or non-finite number", value)
		}
	}
}

func TestIntegerValueEnforcesIntegralRangeAndNilDefault(t *testing.T) {
	if got, err := integerValue(nil); err != nil || got != 0 {
		t.Fatalf("integerValue(nil) = %d, %v, want (0, nil)", got, err)
	}
	for _, value := range []any{float64(0), int(15), json.Number("1000000"), "42"} {
		if _, err := integerValue(value); err != nil {
			t.Errorf("integerValue(%v) returned error: %v", value, err)
		}
	}
	for _, value := range []any{float64(-1), float64(1.1), float64(1_000_001), "invalid"} {
		if _, err := integerValue(value); err == nil {
			t.Errorf("integerValue(%v) accepted out-of-range or fractional value", value)
		}
	}
}
