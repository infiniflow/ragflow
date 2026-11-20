package service

import (
	"encoding/json"
	"math"
	"testing"
)

// TestFloatFromRelatedQuestionConfigKeepsNarrowContract pins the pre-convergence
// contract: native numeric kinds plus json.Number are accepted, strings are not.
// Rejecting non-finite results keeps NaN out of the LLM request body.
func TestFloatFromRelatedQuestionConfigKeepsNarrowContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want float64
	}{
		{"float64", 0.7, 0.7},
		{"float32", float32(0.5), 0.5},
		{"int", 1, 1},
		{"int64", int64(1), 1},
		{"json.Number", json.Number("0.7"), 0.7},
	} {
		t.Run("accept_"+tc.name, func(t *testing.T) {
			got, ok := floatFromRelatedQuestionConfig(tc.v)
			if !ok || got != tc.want {
				t.Fatalf("floatFromRelatedQuestionConfig(%v) = %v,%v, want %v,true", tc.v, got, ok, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		v    any
	}{
		{"numeric string", "0.7"},
		{"NaN string", "NaN"},
		{"Inf string", "Inf"},
		{"int32", int32(1)},
		{"NaN float", math.NaN()},
		{"+Inf float", math.Inf(1)},
	} {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			if got, ok := floatFromRelatedQuestionConfig(tc.v); ok {
				t.Fatalf("floatFromRelatedQuestionConfig(%v) = %v,true, want rejected", tc.v, got)
			}
		})
	}
}
