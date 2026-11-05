package common

import (
	"reflect"
	"testing"
)

// TestFirstNonEmptyReturnsTrimmed locks the contract that FirstNonEmpty returns
// the *trimmed* first non-empty value. The wiki pipeline historically relied on
// this (its local firstNonEmpty trimmed and returned the trimmed value); the
// consolidated helper must preserve that so the four local copies can be deleted
// without a behavior change.
func TestFirstNonEmptyReturnsTrimmed(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"plain value unchanged", []string{"a", "b"}, "a"},
		{"returns first when later match", []string{"x", "y", "z"}, "x"},
		{"trims leading/trailing space of the chosen value", []string{"  picked  ", "b"}, "picked"},
		// Adversarial: the chosen candidate carries whitespace and a later
		// candidate does not. A wrong (untrimmed-return) implementation would
		// return "  picked  " here.
		{"chosen whitespace, later clean", []string{"  picked  ", "clean"}, "picked"},
		{"skips whitespace-only candidates", []string{"   ", "  ", "real"}, "real"},
		{"all empty returns empty", []string{"", " ", "  "}, ""},
		{"nil returns empty", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FirstNonEmpty(c.in...); got != c.want {
				t.Fatalf("FirstNonEmpty(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestMetaString locks the canonical MetaString behavior: returns the string
// value as-is, and "" for a missing or non-string key.
func TestMetaString(t *testing.T) {
	m := map[string]any{"k": "v", "n": 123}
	if got := MetaString(m, "k"); got != "v" {
		t.Fatalf("MetaString(k) = %q, want v", got)
	}
	if got := MetaString(m, "n"); got != "" {
		t.Fatalf("MetaString(non-string) = %q, want empty", got)
	}
	if got := MetaString(m, "missing"); got != "" {
		t.Fatalf("MetaString(missing) = %q, want empty", got)
	}
}

// TestMetaInt locks the canonical int64 MetaInt: tolerant of int64, int,
// float64, and numeric string boxings; missing/non-numeric yields (0, false).
func TestMetaInt(t *testing.T) {
	m := map[string]any{
		"i64": int64(7),
		"i":   int(8),
		"f":   float64(9.0),
		"s":   "42",
		"bad": "abc",
		"non": true,
	}
	if v, ok := MetaInt(m, "i64"); v != 7 || !ok {
		t.Fatalf("MetaInt(i64) = %d,%v want 7,true", v, ok)
	}
	if v, ok := MetaInt(m, "i"); v != 8 || !ok {
		t.Fatalf("MetaInt(i) = %d,%v want 8,true", v, ok)
	}
	if v, ok := MetaInt(m, "f"); v != 9 || !ok {
		t.Fatalf("MetaInt(f) = %d,%v want 9,true", v, ok)
	}
	// Adversarial: numeric string boxed value must still parse.
	if v, ok := MetaInt(m, "s"); v != 42 || !ok {
		t.Fatalf("MetaInt(s) = %d,%v want 42,true", v, ok)
	}
	if v, ok := MetaInt(m, "bad"); v != 0 || ok {
		t.Fatalf("MetaInt(bad) = %d,%v want 0,false", v, ok)
	}
	if v, ok := MetaInt(m, "non"); v != 0 || ok {
		t.Fatalf("MetaInt(non-numeric) = %d,%v want 0,false", v, ok)
	}
	if v, ok := MetaInt(m, "missing"); v != 0 || ok {
		t.Fatalf("MetaInt(missing) = %d,%v want 0,false", v, ok)
	}
}

// TestMetaStringSlice locks the canonical filter-empty metaStringSlice: reads
// []string and []any, keeps non-empty strings, drops empties (the []any branch
// previously filtered; the source data is produced empty-free by the dedup
// helpers so this is behavior-preserving with both legacy readers).
func TestMetaStringSlice(t *testing.T) {
	m := map[string]any{
		"strs":      []string{"a", "", "b"},
		"strsEmpty": []string{"", "", ""},
		"anys":      []any{"a", "", 5, "b"},
		"anyson":    []any{1, 2},
		"missing":   "not-a-slice",
	}
	// Both boxings must drop empties: a unified []string/[]any reader must not let
	// the source type change the result. The []string branch previously kept
	// empties (inconsistent with []any) — this guards the fix.
	if got := MetaStringSlice(m, "strs"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("MetaStringSlice(strs) = %v, want [a b]", got)
	}
	// A []string that is all-empty yields an empty (non-nil) slice, mirroring the
	// []any branch — never nil for a present typed []string key.
	if got := MetaStringSlice(m, "strsEmpty"); len(got) != 0 {
		t.Fatalf("MetaStringSlice(strsEmpty) = %v, want empty slice", got)
	}
	// []any: drops non-strings and empties.
	if got := MetaStringSlice(m, "anys"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("MetaStringSlice(anys) = %v, want [a b]", got)
	}
	// All entries dropped: the []any branch returns a non-nil empty slice
	// (matches the legacy reader); a non-slice/missing key returns nil.
	if got := MetaStringSlice(m, "anyson"); len(got) != 0 {
		t.Fatalf("MetaStringSlice(anyson) = %v, want empty slice", got)
	}
	if got := MetaStringSlice(m, "missing"); got != nil {
		t.Fatalf("MetaStringSlice(missing) = %v, want nil", got)
	}
}
