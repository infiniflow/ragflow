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

package component

import (
	"encoding/json"
	"sort"
	"testing"
)

// TestToFloat64OKKeepsNarrowContract pins the pre-convergence contract of the
// helper used by lessScalar: only int, int64 and float64 are numeric; every
// other type (notably strings) must fall back to the caller's lexical path.
// common.ToFloat accepts strings, so a naive delegation widens the contract.
func TestToFloat64OKKeepsNarrowContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want float64
	}{
		{"int", 7, 7},
		{"int64", int64(7), 7},
		{"float64", 7.5, 7.5},
	} {
		t.Run("accept_"+tc.name, func(t *testing.T) {
			got, ok := toFloat64OK(tc.v)
			if !ok || got != tc.want {
				t.Fatalf("toFloat64OK(%v) = %v,%v, want %v,true", tc.v, got, ok, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		v    any
	}{
		{"numeric string", "10"},
		{"NaN string", "NaN"},
		{"json number", json.Number("10")},
		{"bool", true},
		{"int32", int32(10)},
		{"float32", float32(10)},
	} {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			if got, ok := toFloat64OK(tc.v); ok {
				t.Fatalf("toFloat64OK(%v) = %v,true, want rejected", tc.v, got)
			}
		})
	}
}

func TestLessScalarKeepsLexicographicOrderForNumericStrings(t *testing.T) {
	// Lexicographically "2" > "10"; numeric comparison would invert this and
	// change the sorted output of ["2","10"].
	if lessScalar("2", "10") {
		t.Fatal(`lessScalar("2","10") = true, want false (lexical order)`)
	}
	// A numeric-looking string must not become comparable with a genuine
	// number: base used the fmt.Sprintf fallback, where "9" > "10".
	if lessScalar("9", 10) {
		t.Fatal(`lessScalar("9",10) = true, want false (string fallback)`)
	}
	if !lessScalar(2, 10) {
		t.Fatal("lessScalar(2,10) = false, want true (genuine numeric order)")
	}
}

// TestLessScalarMixedListIsTransitive guards the comparison-cycle regression:
// when a numeric string is accepted, "9" becomes comparable with 10 and the
// three mixed operands form b < a < c < b, which sort.SliceStable resolves by
// comparison order rather than by data.
func TestLessScalarMixedListIsTransitive(t *testing.T) {
	items := []any{"2x", 10, "9"}
	sort.SliceStable(items, func(i, j int) bool { return lessScalar(items[i], items[j]) })
	want := []any{10, "2x", "9"}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("mixed sort = %v, want %v", items, want)
		}
	}
}
