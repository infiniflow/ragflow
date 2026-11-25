package service

import (
	"encoding/json"
	"math"
	"testing"
)

// TestFloatFromSearchConfigKeepsNarrowContract pins the pre-convergence
// contract: only native numeric kinds are accepted. common.ToFloat also
// parses strings (including "NaN" and "Inf"), which would let a persisted
// llm_setting value reach the LLM request body and break json.Marshal.
func TestFloatFromSearchConfigKeepsNarrowContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want float64
	}{
		{"float64", 0.7, 0.7},
		{"float32", float32(0.5), 0.5},
		{"int", 1, 1},
		{"int64", int64(1), 1},
	} {
		t.Run("accept_"+tc.name, func(t *testing.T) {
			got, ok := floatFromSearchConfig(tc.v)
			if !ok || got != tc.want {
				t.Fatalf("floatFromSearchConfig(%v) = %v,%v, want %v,true", tc.v, got, ok, tc.want)
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
		{"json number", json.Number("0.7")},
		{"int32", int32(1)},
		{"NaN float", math.NaN()},
		{"+Inf float", math.Inf(1)},
		{"-Inf float", math.Inf(-1)},
	} {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			if got, ok := floatFromSearchConfig(tc.v); ok {
				t.Fatalf("floatFromSearchConfig(%v) = %v,true, want rejected", tc.v, got)
			}
		})
	}
}

// TestGenerationFloatFallsBackOnInvalidPersistedValue guards the end-to-end
// path from a saved llm_setting to the LLM request body: a persisted string
// such as "NaN" must fall back instead of producing a non-finite float.
func TestGenerationFloatFallsBackOnInvalidPersistedValue(t *testing.T) {
	for _, invalid := range []any{"NaN", "Inf", "0.7", json.Number("0.7"), int32(1)} {
		got := generationFloat(map[string]interface{}{"temperature": invalid}, "temperature", 0.7)
		if got == nil || *got != 0.7 {
			t.Fatalf("generationFloat(temperature=%v) = %v, want fallback 0.7", invalid, got)
		}
	}
}

// TestGenerationFloatHonorsEnabledFlag covers the coupled {key}/{key}_enabled
// pair: a disabled flag substitutes the fallback even when a valid value is
// persisted, and an enabled flag keeps a valid persisted value.
func TestGenerationFloatHonorsEnabledFlag(t *testing.T) {
	if got := generationFloat(map[string]interface{}{
		"temperature":         0.9,
		"temperature_enabled": false,
	}, "temperature", 0.7); got == nil || *got != 0.7 {
		t.Fatalf("disabled flag = %v, want fallback 0.7", got)
	}
	if got := generationFloat(map[string]interface{}{
		"temperature":         0.9,
		"temperature_enabled": true,
	}, "temperature", 0.7); got == nil || *got != 0.9 {
		t.Fatalf("enabled flag = %v, want persisted 0.9", got)
	}
}
