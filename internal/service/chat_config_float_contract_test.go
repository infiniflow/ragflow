package service

import (
	"encoding/json"
	"math"
	"testing"
)

// TestChatConfigFloatKeepsNarrowContract pins the config override contract:
// only native numeric kinds are accepted. A string such as "NaN" or "Inf"
// must not become a non-finite float (json.Marshal rejects NaN/Inf), and
// numeric-looking strings were ignored before the convergence.
func TestChatConfigFloatKeepsNarrowContract(t *testing.T) {
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
			got, ok := chatConfigFloat(tc.v)
			if !ok || got != tc.want {
				t.Fatalf("chatConfigFloat(%v) = %v,%v, want %v,true", tc.v, got, ok, tc.want)
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
			if got, ok := chatConfigFloat(tc.v); ok {
				t.Fatalf("chatConfigFloat(%v) = %v,true, want rejected", tc.v, got)
			}
		})
	}
}
