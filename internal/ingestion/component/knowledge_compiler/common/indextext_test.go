package common

import "testing"

func TestPayloadDescriptionExcludesEvidenceByDefault(t *testing.T) {
	payload := map[string]any{
		"name":        "capital",
		"type":        "fact",
		"description": "The capital is Paris.",
		"evidence": []any{
			map[string]any{"quote": "The capital is Paris.", "chunk_id": "c1", "start": 0, "end": 21},
		},
	}
	got := PayloadDescription(payload, nil)

	if want := "The capital is Paris."; !contains(got, want) {
		t.Errorf("claim text missing from index text: %q", got)
	}
	// A nested map must never render as Go map syntax into the index text.
	if contains(got, "map[") || contains(got, "chunk_id") || contains(got, "quote:") {
		t.Errorf("evidence leaked into index text: %q", got)
	}
}

func TestPayloadDescriptionHonoursCustomExclusion(t *testing.T) {
	payload := map[string]any{"a": "one", "b": "two"}
	if got := PayloadDescription(payload, map[string]bool{"b": true}); got != "one" {
		t.Errorf("custom exclusion not applied: %q", got)
	}
}

func TestPayloadDescriptionIsDeterministic(t *testing.T) {
	// Go map iteration order is random; keys must be sorted so vectors are
	// stable across runs.
	payload := map[string]any{"z": "last", "a": "first", "m": "middle"}
	want := "first middle last"
	for i := 0; i < 50; i++ {
		if got := PayloadDescription(payload, nil); got != want {
			t.Fatalf("non-deterministic index text: got %q want %q", got, want)
		}
	}
}

func TestPayloadDescriptionFlattensSlices(t *testing.T) {
	payload := map[string]any{
		"tags":    []string{"x", "", "y"},
		"mixed":   []any{"z", 42, nil},
		"skipped": []string{"nope"},
	}
	got := PayloadDescription(payload, map[string]bool{"skipped": true})
	if !contains(got, "x") || !contains(got, "y") || !contains(got, "z") {
		t.Errorf("slices not flattened: %q", got)
	}
	if contains(got, "nope") {
		t.Errorf("excluded slice key present: %q", got)
	}
}

func TestStringOfScalars(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"s", "s"},
		{float64(3), "3"},
		{float64(3.5), "3.5"},
		{int64(7), "7"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
		{map[string]any{}, ""},
	}
	for _, c := range cases {
		if got := StringOf(c.in); got != c.want {
			t.Errorf("StringOf(%#v) = %q want %q", c.in, got, c.want)
		}
	}
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
