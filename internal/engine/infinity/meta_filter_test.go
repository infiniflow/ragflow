//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package infinity

import (
	"reflect"
	"testing"
)

// TestSplitJSONParts pins down the quote-aware split. The previous
// implementation only tracked an `inQuote` bool toggled on `'`, so
// double-quoted strings containing commas were split incorrectly and
// single quotes inside double-quoted strings (e.g. apostrophes) were
// also mis-handled.
func TestSplitJSONParts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "single_quoted_basic",
			in:   `'a', 'b', 'c'`,
			want: []string{`'a'`, ` 'b'`, ` 'c'`},
		},
		{
			name: "double_quoted_with_commas_inside",
			in:   `"a,b", "c"`,
			want: []string{`"a,b"`, ` "c"`},
		},
		{
			name: "apostrophe_inside_double_quoted",
			// "don't", "we, can't" — the apostrophes and the comma in
			// "we, can't" must not break the double-quoted regions.
			in:   `"don't", "we, can't"`,
			want: []string{`"don't"`, ` "we, can't"`},
		},
		{
			name: "nested_brackets_inside_quotes_do_not_count",
			in:   `"{a,b}", [1, 2]`,
			want: []string{`"{a,b}"`, ` [1, 2]`},
		},
		{
			name: "comma_inside_brackets_outside_quotes_splits",
			in:   `[1, 2], [3, 4]`,
			want: []string{`[1, 2]`, ` [3, 4]`},
		},
		{
			name: "trailing_comma_drops_empty_part",
			in:   `"a", "b",`,
			want: []string{`"a"`, ` "b"`},
		},
		{
			// Regression: previously the parser toggled quote state on
			// every '"', so the comma inside the first element split
			// the array into three pieces and corrupted in/not in filters.
			name: "escaped_double_quote_does_not_split",
			in:   `"a\"b,c", "d"`,
			want: []string{`"a\"b,c"`, ` "d"`},
		},
		{
			// `\\\"` = escaped backslash + escaped quote. The backslash
			// is a literal, the quote is escaped, so no split.
			name: "escaped_backslash_then_escaped_quote",
			in:   `"a\\\"b", "c"`,
			want: []string{`"a\\\"b"`, ` "c"`},
		},
		{
			// Symmetric handling for single-quoted regions.
			name: "escaped_single_quote_does_not_split",
			in:   `'a\'b,c', 'd'`,
			want: []string{`'a\'b,c'`, ` 'd'`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitJSONParts(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitJSONParts(%q):\n got  %#v\n want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// TestParseJSONArrayDoubleQuotedWithCommas is the regression case from
// the bug report: ["a,b", "c"] must yield two elements, not three.
func TestParseJSONArrayDoubleQuotedWithCommas(t *testing.T) {
	got := parseJSONArray(`["a,b", "c"]`)
	want := []interface{}{"a,b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseJSONArray([\"a,b\", \"c\"]):\n got  %#v\n want %#v", got, want)
	}
}

// TestTranslateContainsEmptyValueErrors pins down the empty-value path
// in translateContains. The Python reference raises ValueError for
// `if not value and value != 0`; the Go port must mirror that instead
// of returning "" (which would join into malformed SQL like
// "( AND other_condition)" once BuildInfinityFilter is called).
func TestTranslateContainsEmptyValueErrors(t *testing.T) {
	tr := NewMetaFilterTranslator()
	cases := []struct {
		name string
		flt  map[string]interface{}
	}{
		{
			name: "nil_value",
			flt:  map[string]interface{}{"op": "contains", "key": "author", "value": nil},
		},
		{
			name: "empty_string",
			flt:  map[string]interface{}{"op": "contains", "key": "author", "value": ""},
		},
		{
			name: "empty_slice",
			flt:  map[string]interface{}{"op": "contains", "key": "author", "value": []string{}},
		},
		{
			name: "empty_map",
			flt:  map[string]interface{}{"op": "contains", "key": "author", "value": map[string]interface{}{}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tr.Translate(tc.flt)
			if err == nil {
				t.Fatalf("expected error for empty value, got %q", got)
			}
			if got != "" {
				t.Errorf("expected empty SQL on error, got %q", got)
			}
		})
	}
}

// TestTranslateContainsNumericZeroStaysGuarded confirms the
// "value == 0 is not empty" branch inherited from the Python
// reference: numeric 0 is a real value to search for, not an
// empty guard rail.
func TestTranslateContainsNumericZeroStaysGuarded(t *testing.T) {
	tr := NewMetaFilterTranslator()
	flt := map[string]interface{}{"op": "contains", "key": "year", "value": 0}
	got, err := tr.Translate(flt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `JSON_CONTAINS(meta_fields, '$.year', 0)`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestBuildInfinityFilterRejectsEmptyContains guards the downstream
// effect: an empty-value contains must propagate as an error out of
// BuildInfinityFilter, not silently produce "( AND ...)" SQL.
func TestBuildInfinityFilterRejectsEmptyContains(t *testing.T) {
	filters := []map[string]interface{}{
		{"op": "contains", "key": "author", "value": ""},
		{"op": "=", "key": "year", "value": 2026},
	}
	_, err := BuildInfinityFilter(filters, "and")
	if err == nil {
		t.Fatal("expected error from BuildInfinityFilter, got nil")
	}
}

// TestTotalHitsFromInfinityExtraInfo pins down the parser that decodes
// the JSON payload Infinity returns in QueryResult.ExtraInfo when the
// total_hits_count option is set. The shape isn't part of the public
// SDK contract, so we accept several spellings; getting the
// (total > cap) trigger wrong would let the push-down silently drop
// docs and the caller would never fall back to the in-memory path.
func TestTotalHitsFromInfinityExtraInfo(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		wantVal int64
		wantOk  bool
	}{
		{
			name:    "empty",
			extra:   "",
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "invalid_json",
			extra:   "not json",
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "total_hits_count_under_cap",
			extra:   `{"total_hits_count": 42}`,
			wantVal: 42,
			wantOk:  true,
		},
		{
			name:    "total_hits_count_at_cap",
			extra:   `{"total_hits_count": 10000}`,
			wantVal: 10000,
			wantOk:  true,
		},
		{
			name:    "total_hits_count_over_cap_triggers_fallback",
			extra:   `{"total_hits_count": 50000}`,
			wantVal: 50000,
			wantOk:  true,
		},
		{
			name:    "camelCase_alias",
			extra:   `{"totalHitsCount": 7}`,
			wantVal: 7,
			wantOk:  true,
		},
		{
			name:    "short_alias",
			extra:   `{"total": 9}`,
			wantVal: 9,
			wantOk:  true,
		},
		{
			name:    "no_recognized_key",
			extra:   `{"unrelated": 1}`,
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "negative_value_rejected",
			extra:   `{"total_hits_count": -1}`,
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "non_integer_value_rejected",
			extra:   `{"total_hits_count": "many"}`,
			wantVal: 0,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOk := totalHitsFromInfinityExtraInfo(tt.extra)
			if gotOk != tt.wantOk {
				t.Errorf("ok mismatch: got %v want %v (extra=%q)", gotOk, tt.wantOk, tt.extra)
			}
			if gotVal != tt.wantVal {
				t.Errorf("value mismatch: got %d want %d (extra=%q)", gotVal, tt.wantVal, tt.extra)
			}
		})
	}
}

// TestInfinityPushdownCapSemantics documents the overflow contract for
// the Infinity path: the query is built with .Limit(metaPushdownMaxSize)
// and .Option({total_hits_count: true}); when the parsed total exceeds
// the cap, FilterDocIdsByMetaPushdown must return nil so the caller
// falls back to the in-memory meta_filter rather than returning a
// truncated slice as a definitive answer.
func TestInfinityPushdownCapSemantics(t *testing.T) {
	extra := `{"total_hits_count": 12345}`
	total, ok := totalHitsFromInfinityExtraInfo(extra)
	if !ok {
		t.Fatal("expected total to be parseable from a well-formed Infinity ExtraInfo payload")
	}
	if total <= int64(metaPushdownMaxSize) {
		t.Fatalf("expected total > cap, got total=%d cap=%d", total, metaPushdownMaxSize)
	}
}
