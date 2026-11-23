package oceanbase

import (
	"encoding/json"
	"testing"
)

// TestNumberToFloatKeepsNarrowContract pins the pre-convergence contract:
// database/sql numeric kinds and json.Number are numeric, strings are not.
// common.ToFloat parses strings, so a naive delegation changes search sort
// ordering for string-valued fields.
func TestNumberToFloatKeepsNarrowContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want float64
	}{
		{"float64", 12.5, 12.5},
		{"float32", float32(12.5), 12.5},
		{"int", 12, 12},
		{"int32", int32(12), 12},
		{"int64", int64(12), 12},
		{"json.Number", json.Number("12.5"), 12.5},
	} {
		t.Run("accept_"+tc.name, func(t *testing.T) {
			got, ok := numberToFloat(tc.v)
			if !ok || got != tc.want {
				t.Fatalf("numberToFloat(%v) = %v,%v, want %v,true", tc.v, got, ok, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		v    any
	}{
		{"string", "12"},
		{"bool", true},
	} {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			if got, ok := numberToFloat(tc.v); ok {
				t.Fatalf("numberToFloat(%v) = %v,true, want rejected", tc.v, got)
			}
		})
	}
}
