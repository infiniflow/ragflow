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

package metafilter

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestBuildMetaFilterQueryNonFiniteValues(t *testing.T) {
	for _, value := range []string{"NaN", "Inf", "Infinity", "+Infinity", "-Inf", "-Infinity"} {
		for _, op := range []string{"=", "≠", ">", "<", "≥", "≤"} {
			t.Run(value+"/"+op, func(t *testing.T) {
				query, err := BuildMetaFilterQuery([]map[string]interface{}{
					{"key": "engine", "op": op, "value": value},
				}, "and", []string{"kb-1"})
				if err != nil {
					t.Fatal(err)
				}
				body, err := json.Marshal(query)
				if err != nil {
					t.Fatal(err)
				}
				want := `"` + value + `"`
				if op == "=" || op == "≠" {
					want = `"meta_fields.engine.keyword":{"case_insensitive":true,"value":"` + strings.ToLower(value) + `"}`
				}
				if !strings.Contains(string(body), want) {
					t.Errorf("query %s does not contain %s", body, want)
				}
			})
		}
	}
}
