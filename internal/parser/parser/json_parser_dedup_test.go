package parser

import (
	"context"
	"testing"
)

// Adversarial fixtures for the single-pass JSON parse. The standard-library
// pre-probe and the custom unmarshalJSON both reject trailing data and both
// accept big numbers verbatim; removing the pre-probe must not change output.
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

func BenchmarkJSONParserObjectSinglePass(b *testing.B) {
	data := []byte(`{"id": 12345678901234567890, "note": "a < b && c > d", "price": 1.50, "tags": ["a","b","c"]}`)
	p := NewJSONParser()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p.ParseWithResult(context.Background(), "a.json", data)
	}
}
