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

package elasticsearch

import (
	"reflect"
	"testing"
)

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

func TestParseJSONArrayDoubleQuotedWithCommas(t *testing.T) {
	got := parseJSONArray(`["a,b", "c"]`)
	want := []interface{}{"a,b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseJSONArray([\"a,b\", \"c\"]):\n got  %#v\n want %#v", got, want)
	}
}

// TestTotalHitsFromESResponse pins down the helper that decides whether
// the ES push-down response was truncated at metaPushdownMaxSize.
// Returning a non-nil (total, ok=true) value with total > cap is what
// drives the caller to fall back to the in-memory meta_filter; getting
// this wrong (false negative) would let the push-down silently drop docs.
func TestTotalHitsFromESResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    map[string]interface{}
		wantVal int64
		wantOk  bool
	}{
		{
			name:    "missing_hits",
			resp:    map[string]interface{}{},
			wantVal: 0,
			wantOk:  false,
		},
		{
			name:    "missing_total",
			resp:    map[string]interface{}{"hits": map[string]interface{}{}},
			wantVal: 0,
			wantOk:  false,
		},
		{
			name: "exact_total_under_cap",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": float64(42)},
				},
			},
			wantVal: 42,
			wantOk:  true,
		},
		{
			name: "exact_total_at_cap",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": float64(metaPushdownMaxSize)},
				},
			},
			wantVal: int64(metaPushdownMaxSize),
			wantOk:  true,
		},
		{
			name: "overflow_total_over_cap",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": float64(metaPushdownMaxSize + 1)},
				},
			},
			wantVal: int64(metaPushdownMaxSize + 1),
			wantOk:  true,
		},
		{
			name: "wrong_type",
			resp: map[string]interface{}{
				"hits": map[string]interface{}{
					"total": map[string]interface{}{"value": "10000"},
				},
			},
			wantVal: 0,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOk := totalHitsFromESResponse(tt.resp)
			if gotOk != tt.wantOk {
				t.Errorf("ok mismatch: got %v want %v", gotOk, tt.wantOk)
			}
			if gotVal != tt.wantVal {
				t.Errorf("value mismatch: got %d want %d", gotVal, tt.wantVal)
			}
		})
	}
}

// TestPushdownCapSemantics documents the contract for the overflow path:
// when total > cap, the slice we'd build from ExtractDocIDs is necessarily
// a strict subset of the truth. FilterDocIdsByMetaPushdown must return
// nil in that case so the caller falls back to the in-memory meta_filter.
//
// This test exercises the response-shape contract directly rather than
// spinning up a real ES client; the integration is covered by the
// service-level tests in internal/service/metadata_filter_test.go.
func TestPushdownCapSemantics(t *testing.T) {
	// Build a response that LOOKS like a successful push-down but has
	// total > cap. totalHitsFromESResponse must report ok=true and the
	// value must exceed the cap, which is the trigger condition for
	// returning nil in FilterDocIdsByMetaPushdown.
	resp := map[string]interface{}{
		"hits": map[string]interface{}{
			"total": map[string]interface{}{"value": float64(metaPushdownMaxSize + 1)},
			"hits": []interface{}{
				map[string]interface{}{"_id": "doc-1"},
			},
		},
	}
	total, ok := totalHitsFromESResponse(resp)
	if !ok {
		t.Fatal("expected total to be parseable from a well-formed ES response")
	}
	if total <= int64(metaPushdownMaxSize) {
		t.Fatalf("expected total > cap, got total=%d cap=%d", total, metaPushdownMaxSize)
	}
}
