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

package parser

import (
	"context"
	"reflect"
	"testing"
)

// TestJSONParser_KeepsValuesVerbatim verifies that re-serialised JSON items
// keep integers beyond float64 precision exactly and do not escape <, > and &
// into <, > and &.
func TestJSONParser_KeepsValuesVerbatim(t *testing.T) {
	const obj = `{"id": 12345678901234567891, "note": "a < b && c > d"}`
	const want = `{"id":12345678901234567891,"note":"a < b && c > d"}`
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"object", obj, []string{want}},
		{"array", "[" + obj + ", 9007199254740993]", []string{want, "9007199254740993"}},
		{"jsonl", obj + "\n" + obj + "\n", []string{want, want}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := NewJSONParser().ParseWithResult(context.Background(), "data.json", []byte(tc.input))
			var got []string
			for _, item := range res.JSON {
				got = append(got, item["text"].(string))
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("texts = %q, want %q", got, tc.want)
			}
		})
	}
}
