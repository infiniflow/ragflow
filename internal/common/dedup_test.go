package common

import (
	"reflect"
	"testing"
)

// TestDeduplicateTrimmed verifies trim + drop-empty + dedup + order-preserving,
// and the "empty input (or all dropped) returns nil" contract.
func TestDeduplicateTrimmed(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil input returns nil", nil, nil},
		{"empty slice returns nil", []string{}, nil},
		{"single element", []string{"a"}, []string{"a"}},
		{"dedup preserves order", []string{"b", "a", "b", "c", "a"}, []string{"b", "a", "c"}},
		{"trims surrounding space", []string{" a ", "a", "  b"}, []string{"a", "b"}},
		{"drops empty string", []string{"", "a", ""}, []string{"a"}},
		{"drops whitespace-only", []string{" ", "  ", "a"}, []string{"a"}},
		// Adversarial: all entries collapse to empty -> nil, never []string{}.
		{"all empty collapses to nil", []string{"", " ", "  "}, nil},
		// Adversarial: trim makes two distinct-looking entries identical.
		{"trim collapses duplicates", []string{"x ", " x", "x"}, []string{"x"}},
		// Adversarial: unicode whitespace must be trimmed.
		{"unicode whitespace trimmed", []string{"\ta\n", "a"}, []string{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeduplicateTrimmed(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("DeduplicateTrimmed(%v) = %v, want %v", c.in, got, c.want)
			}
			if c.want == nil && got != nil {
				t.Fatalf("expected nil result for %v, got %v", c.in, got)
			}
		})
	}
}

// TestDeduplicateSorted verifies drop-empty + dedup + sort, that it does NOT
// mutate the caller's slice, and the empty->nil contract.
func TestDeduplicateSorted(t *testing.T) {
	// Sort correctness + drop empty (no trim: this variant does NOT trim).
	in := []string{"c", "", "a", "b", "a"}
	want := []string{"a", "b", "c"}
	got := DeduplicateSorted(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeduplicateSorted(%v) = %v, want %v", in, got, want)
	}
	// Adversarial: does NOT trim, so " a" and "a" stay distinct.
	inNoTrim := []string{" a", "a"}
	if got := DeduplicateSorted(inNoTrim); !reflect.DeepEqual(got, []string{" a", "a"}) {
		t.Fatalf("DeduplicateSorted(%v) = %v, want [ a a] (no trim)", inNoTrim, got)
	}
	// Does not mutate the input slice.
	if in[0] != "c" || in[1] != "" {
		t.Fatalf("DeduplicateSorted mutated input: %v", in)
	}
	// Empty contract.
	if got := DeduplicateSorted(nil); got != nil {
		t.Fatalf("DeduplicateSorted(nil) = %v, want nil", got)
	}
	if got := DeduplicateSorted([]string{}); got != nil {
		t.Fatalf("DeduplicateSorted([]string{}) = %v, want nil", got)
	}
	if got := DeduplicateSorted([]string{"", ""}); got != nil {
		t.Fatalf("DeduplicateSorted(all empty) = %v, want nil", got)
	}
}

// TestAppendUnique verifies append-into-dst, drop-empty, dedup against existing
// dst entries, and order preservation.
func TestAppendUnique(t *testing.T) {
	// Appends new, skips empties and already-present.
	dst := []string{"a", "b"}
	got := AppendUnique(dst, []string{"b", "", "c", "a"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AppendUnique(%v, ...) = %v, want %v", dst, got, want)
	}
	// Appending to a nil dst yields the new, deduped, non-empty items.
	gotNil := AppendUnique(nil, []string{"x", "x", "y"})
	if !reflect.DeepEqual(gotNil, []string{"x", "y"}) {
		t.Fatalf("AppendUnique(nil, ...) = %v, want [x y]", gotNil)
	}
	// Appending only empties/dupes leaves dst unchanged (and nil stays nil).
	gotEmpty := AppendUnique(nil, []string{"", ""})
	if gotEmpty != nil {
		t.Fatalf("AppendUnique(nil, empties) = %v, want nil", gotEmpty)
	}
	// Adversarial: AppendUnique does NOT trim, so " x" and "x" both stay.
	gotNoTrim := AppendUnique(nil, []string{" x", "x"})
	if !reflect.DeepEqual(gotNoTrim, []string{" x", "x"}) {
		t.Fatalf("AppendUnique(nil, [ x x]) = %v, want [ x x] (no trim)", gotNoTrim)
	}
}
