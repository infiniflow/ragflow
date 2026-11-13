package utility

import (
	"strings"
	"testing"
)

// TestMustJSON_NoTrailingNewline asserts the contract that distinguishes this
// shared helper from the legacy handler mustJSON (which used json.Encoder and
// appended a trailing newline). The shared helper uses json.Marshal, so the
// output must be compact with no trailing '\n'.
func TestMustJSON_NoTrailingNewline(t *testing.T) {
	got := MustJSON(map[string]any{"a": 1})
	want := `{"a":1}`
	if got != want {
		t.Fatalf("MustJSON = %q, want %q", got, want)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("MustJSON must not append a trailing newline, got %q", got)
	}
}

func TestMustJSON_SpecialCharsAndUnicode(t *testing.T) {
	got := MustJSON(map[string]any{"s": "a\"b\n日"})
	want := `{"s":"a\"b\n日"}`
	if got != want {
		t.Fatalf("MustJSON = %q, want %q", got, want)
	}
}

func TestMustJSON_StructAndEmpty(t *testing.T) {
	type rec struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if got, want := MustJSON(rec{Name: "x", N: 2}), `{"name":"x","n":2}`; got != want {
		t.Fatalf("MustJSON struct = %q, want %q", got, want)
	}
	if got, want := MustJSON([]int{}), `[]`; got != want {
		t.Fatalf("MustJSON empty slice = %q, want %q", got, want)
	}
	if got, want := MustJSON(map[string]any{}), `{}`; got != want {
		t.Fatalf("MustJSON empty map = %q, want %q", got, want)
	}
	if got, want := MustJSON(nil), `null`; got != want {
		t.Fatalf("MustJSON nil = %q, want %q", got, want)
	}
}

// TestMustJSON_ErrorReturnsEmpty is the adversarial test that falsifies the
// legacy wiki mustJSON, which returned "[]" on a marshal error. The shared
// helper returns "" instead, because callers pass statically-typed values and
// a marshal error is unreachable in practice.
func TestMustJSON_ErrorReturnsEmpty(t *testing.T) {
	if got := MustJSON(make(chan int)); got != "" {
		t.Fatalf("MustJSON(unmarshalable) = %q, want empty string", got)
	}
}
