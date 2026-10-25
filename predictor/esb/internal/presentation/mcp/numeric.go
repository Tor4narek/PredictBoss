package mcp

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func numericValue(value any) (float64, error) {
	var parsed float64
	switch value := value.(type) {
	case float64:
		parsed = value
	case float32:
		parsed = float64(value)
	case int:
		parsed = float64(value)
	case int64:
		parsed = float64(value)
	case json.Number:
		var err error
		parsed, err = value.Float64()
		if err != nil {
			return 0, err
		}
	case string:
		var err error
		parsed, err = strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("unsupported numeric value of type %T", value)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("numeric value must be finite")
	}
	return parsed, nil
}

func integerValue(value any) (int, error) {
	if value == nil {
		return 0, nil
	}
	parsed, err := numericValue(value)
	if err != nil {
		return 0, err
	}
	if parsed < 0 || parsed > 1_000_000 || math.Trunc(parsed) != parsed {
		return 0, fmt.Errorf("integer value is outside the supported range")
	}
	return int(parsed), nil
}
