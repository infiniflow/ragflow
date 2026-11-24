package parser

import (
	"context"
	"testing"
)

// Adversarial fixtures for the single-pass JSON parse. Both decoders accept
// the same number syntax (and preserve digits via the second decode); the
// separate out-of-range test below covers the float64-range difference that
// the old probe's float64 conversion introduced. Trailing data and malformed
// objects must produce identical fallback text.
func TestJSONParserSinglePassEquivalence(t *testing.T) {
	p := NewJSONParser()
	cases := []struct {
		name, data string
		want       []string
	}{
		{"object-bignum", `{"id": 12345678901234567890, "price": 1.50}`, []string{`{"id":12345678901234567890,"price":1.50}`}},
		{"object-html", `{"note": "a < b"}`, []string{`{"note":"a < b"}`}},
		// trailing garbage after an object -> falls back to raw text (single line is not JSONL)
		{"trailing-data", `{"a":1} garbage`, []string{`{"a":1} garbage`}},
		// malformed object -> raw text
		{"malformed", `{"a":`, []string{`{"a":`}},
		// JSONL stays multi-item
		{"jsonl", "{\"q\":\"a\"}\n{\"q\":\"b\"}\n", []string{`{"q":"a"}`, `{"q":"b"}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertJSONTexts(t, p.ParseWithResult(context.Background(), "a.json", []byte(tc.data)), tc.want...)
		})
	}
}

// The pre-probe decoded numbers to float64, so a number outside the float64
// range made it reject the whole document as a single object and fall back to
// raw text. The number-preserving decoder accepts such numbers, so restoring
// byte-identical output requires the single-object path to reject them too.
// Array parsing never had the float64 probe, so arrays must keep accepting
// them (adversarial control case).
func TestJSONParserOutOfFloat64Range(t *testing.T) {
	p := NewJSONParser()
	cases := []struct {
		name, data string
		want       []string
	}{
		// 1e400 overflows float64 -> old probe failed -> raw text preserved.
		{"object-overflow", `{"n": 1e400}`, []string{`{"n": 1e400}`}},
		// Nested numbers were probed too -> raw text preserved.
		{"object-nested-overflow", `{"a": {"b": 1e400}}`, []string{`{"a": {"b": 1e400}}`}},
		// Adversarial: an overflowing value followed by a duplicate in-range
		// key. The old probe rejected 1e400 BEFORE decoding; the number-
		// preserving decoder overwrites it with the later "1", so a post-decode
		// range check misses it. Must still fall back to raw text.
		{"object-overflow-shadowed-by-dup-key", `{"n": 1e400, "n": 1}`, []string{`{"n": 1e400, "n": 1}`}},
		{"object-nested-overflow-shadowed", `{"a": {"n": 1e400, "n": 1}}`, []string{`{"a": {"n": 1e400, "n": 1}}`}},
		// Control: duplicate keys all in range -> old probe succeeded -> the
		// number-preserving decoder emits the last value, normalized.
		{"object-dup-key-in-range", `{"n": 1, "n": 2}`, []string{`{"n":2}`}},
		// 1e300 is within float64 range -> probe succeeded -> normalized item.
		{"object-within-range", `{"n": 1e300}`, []string{`{"n":1e300}`}},
		// Arrays never used the float64 probe -> still normalized.
		{"array-overflow", `[{"n": 1e400}]`, []string{`{"n":1e400}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertJSONTexts(t, p.ParseWithResult(context.Background(), "a.json", []byte(tc.data)), tc.want...)
		})
	}
}

func BenchmarkJSONParserObjectSinglePass(b *testing.B) {
	data := []byte(`{"id": 12345678901234567890, "note": "a < b && c > d", "price": 1.50, "tags": ["a","b","c"]}`)
	p := NewJSONParser()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.ParseWithResult(context.Background(), "a.json", data)
	}
}

// The byte scanner must skip string contents: a number-like substring inside
// a JSON string is not a numeric literal and must not trigger the fallback.
func TestJSONParserNumberLikeStringNotRejected(t *testing.T) {
	p := NewJSONParser()
	cases := []struct {
		name, data, want string
	}{
		{"digit-string", `{"note": "1e400"}`, `{"note":"1e400"}`},
		{"escaped-quote", `{"note": "he said \"x=1e400\""}`, `{"note":"he said \"x=1e400\""}`},
		{"negative-in-string", `{"note": "value -5 here"}`, `{"note":"value -5 here"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertJSONTexts(t, p.ParseWithResult(context.Background(), "a.json", []byte(tc.data)), tc.want)
		})
	}
}

// Boundary: float64 max ~= 1.8e308. 1e308 is in range (normalized), 1e309
// overflows (raw-text fallback) — pins the byte scanner's ParseFloat cutoff.
func TestJSONParserFloat64RangeBoundary(t *testing.T) {
	p := NewJSONParser()
	cases := []struct {
		name, data, want string
	}{
		{"at-boundary-1e308", `{"n": 1e308}`, `{"n":1e308}`},
		{"over-boundary-1e309", `{"n": 1e309}`, `{"n": 1e309}`},
		{"neg-over-boundary", `{"n": -1e309}`, `{"n": -1e309}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertJSONTexts(t, p.ParseWithResult(context.Background(), "a.json", []byte(tc.data)), tc.want)
		})
	}
}
