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

package service

import (
	"sort"
	"testing"

	"ragflow/internal/common"
)

func TestApplyMetaFilter_Equals(t *testing.T) {
	metas := common.MetaData{
		"author": {"Zhang San": {"doc1", "doc2"}, "Li Si": {"doc3"}},
	}
	filters := []MetaFilterCondition{{Key: "author", Value: "Zhang San", Op: "="}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_NotEquals(t *testing.T) {
	metas := common.MetaData{
		"author": {"Zhang San": {"doc1"}, "Li Si": {"doc2"}},
	}
	filters := []MetaFilterCondition{{Key: "author", Value: "Zhang San", Op: "!="}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestApplyMetaFilter_Contains(t *testing.T) {
	metas := common.MetaData{
		"title": {"hello world": {"doc1"}, "goodbye": {"doc2"}},
	}
	filters := []MetaFilterCondition{{Key: "title", Value: "hello", Op: "contains"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestApplyMetaFilter_NotContains(t *testing.T) {
	metas := common.MetaData{
		"title": {"hello world": {"doc1"}, "goodbye": {"doc2"}},
	}
	filters := []MetaFilterCondition{{Key: "title", Value: "hello", Op: "not contains"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestApplyMetaFilter_In(t *testing.T) {
	metas := common.MetaData{
		"category": {"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}},
	}
	filters := []MetaFilterCondition{{Key: "category", Value: "A,B", Op: "in"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_NotIn(t *testing.T) {
	metas := common.MetaData{
		"category": {"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}},
	}
	filters := []MetaFilterCondition{{Key: "category", Value: "A", Op: "not in"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs (B,C), got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_StartWith(t *testing.T) {
	metas := common.MetaData{
		"code": {"ABC-123": {"doc1"}, "XYZ-456": {"doc2"}},
	}
	filters := []MetaFilterCondition{{Key: "code", Value: "abc", Op: "start with"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestApplyMetaFilter_EndWith(t *testing.T) {
	metas := common.MetaData{
		"code": {"ABC-123": {"doc1"}, "ABC-456": {"doc2"}},
	}
	filters := []MetaFilterCondition{{Key: "code", Value: "123", Op: "end with"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestApplyMetaFilter_AndLogic(t *testing.T) {
	metas := common.MetaData{
		"author": {"Zhang San": {"doc1", "doc2"}, "Li Si": {"doc3"}},
		"year":   {"2024": {"doc1"}, "2025": {"doc2", "doc3"}},
	}
	filters := []MetaFilterCondition{
		{Key: "author", Value: "Zhang San", Op: "="},
		{Key: "year", Value: "2024", Op: "="},
	}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestApplyMetaFilter_OrLogic(t *testing.T) {
	metas := common.MetaData{
		"author": {"Zhang San": {"doc1"}, "Li Si": {"doc2"}},
	}
	filters := []MetaFilterCondition{
		{Key: "author", Value: "Zhang San", Op: "="},
		{Key: "author", Value: "Li Si", Op: "="},
	}
	result := ApplyMetaFilter(metas, filters, "or")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_EmptyFilters(t *testing.T) {
	result := ApplyMetaFilter(nil, nil, "and")
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestApplyMetaFilter_KeyNotFound(t *testing.T) {
	metas := common.MetaData{"author": {"Zhang San": {"doc1"}}}
	filters := []MetaFilterCondition{{Key: "nonexistent", Value: "x", Op: "="}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 0 {
		t.Errorf("expected 0, got %v", result)
	}
}

func TestApplyMetaFilter_EqualsAlias(t *testing.T) {
	metas := common.MetaData{"author": {"Zhang San": {"doc1"}}}
	filters := []MetaFilterCondition{{Key: "author", Value: "Zhang San", Op: "=="}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 {
		t.Errorf("expected 1 doc for ==, got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_NotEqualsAlias(t *testing.T) {
	metas := common.MetaData{"author": {"Zhang San": {"doc1"}, "Li Si": {"doc2"}}}
	filters := []MetaFilterCondition{{Key: "author", Value: "Zhang San", Op: "≠"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2] for ≠, got %v", result)
	}
}

func TestApplyMetaFilter_GreaterThan(t *testing.T) {
	metas := common.MetaData{"score": {"85": {"doc1"}, "70": {"doc2"}}}
	filters := []MetaFilterCondition{{Key: "score", Value: "80", Op: ">"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1] for >80, got %v", result)
	}
}

func TestApplyMetaFilter_LessThan(t *testing.T) {
	metas := common.MetaData{"score": {"85": {"doc1"}, "70": {"doc2"}}}
	filters := []MetaFilterCondition{{Key: "score", Value: "80", Op: "<"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2] for <80, got %v", result)
	}
}

func TestApplyMetaFilter_GreaterThanOrEqual(t *testing.T) {
	metas := common.MetaData{"score": {"85": {"doc1"}, "80": {"doc2"}, "70": {"doc3"}}}
	filters := []MetaFilterCondition{{Key: "score", Value: "80", Op: "≥"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs for ≥80, got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_LessThanOrEqual(t *testing.T) {
	metas := common.MetaData{"score": {"85": {"doc1"}, "80": {"doc2"}, "70": {"doc3"}}}
	filters := []MetaFilterCondition{{Key: "score", Value: "80", Op: "≤"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs for ≤80, got %d: %v", len(result), result)
	}
}

func TestApplyMetaFilter_Empty(t *testing.T) {
	metas := common.MetaData{"status": {"": {"doc1"}, "active": {"doc2"}}}
	filters := []MetaFilterCondition{{Key: "status", Value: "", Op: "empty"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1] for empty, got %v", result)
	}
}

func TestApplyMetaFilter_NotEmpty(t *testing.T) {
	metas := common.MetaData{"status": {"": {"doc1"}, "active": {"doc2"}}}
	filters := []MetaFilterCondition{{Key: "status", Value: "", Op: "not empty"}}
	result := ApplyMetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2] for not empty, got %v", result)
	}
}

// --- convertToMetaCondition unit tests ---

func TestConvertToMetaCondition_OperatorNormalization(t *testing.T) {
	tests := []struct {
		op       string
		expected string
	}{
		{"=", "="},
		{"==", "="},
		{"!=", "≠"},
		{"≠", "≠"},
		{">=", "≥"},
		{"≥", "≥"},
		{"<=", "≤"},
		{"≤", "≤"},
		{"contains", "contains"},
		{"not contains", "not contains"},
		{"in", "in"},
		{"not in", "not in"},
		{"start with", "start with"},
		{"end with", "end with"},
		{"empty", "empty"},
		{"not empty", "not empty"},
		{">", ">"},
		{"<", "<"},
	}
	for _, tt := range tests {
		f := MetaFilterCondition{Key: "field", Value: "x", Op: tt.op}
		mc := convertToMetaCondition(f)
		if mc.Operator != tt.expected {
			t.Errorf("Op=%q: expected Operator=%q, got %q", tt.op, tt.expected, mc.Operator)
		}
		if mc.Key != "field" {
			t.Errorf("Op=%q: Key changed to %q", tt.op, mc.Key)
		}
	}
}

func TestConvertToMetaCondition_InValue(t *testing.T) {
	f := MetaFilterCondition{Key: "category", Value: "A,B,C", Op: "in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %d: %v", len(vals), vals)
	}
	if vals[0] != "A" || vals[1] != "B" || vals[2] != "C" {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestConvertToMetaCondition_NotInValueTrim(t *testing.T) {
	f := MetaFilterCondition{Key: "category", Value: " A , B ", Op: "not in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 2 || vals[0] != "A" || vals[1] != "B" {
		t.Errorf("expected [A B] after trim, got %v", vals)
	}
}

func TestConvertToMetaCondition_StringValuePassthrough(t *testing.T) {
	f := MetaFilterCondition{Key: "author", Value: "Zhang San", Op: "="}
	mc := convertToMetaCondition(f)
	v, ok := mc.Value.(string)
	if !ok {
		t.Fatalf("expected string, got %T", mc.Value)
	}
	if v != "Zhang San" {
		t.Errorf("expected 'Zhang San', got %q", v)
	}
}

func TestConvertToMetaCondition_InEmptyParts(t *testing.T) {
	f := MetaFilterCondition{Key: "cat", Value: "A,,B", Op: "in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 2 {
		t.Errorf("expected 2 values after filtering empty parts, got %d: %v", len(vals), vals)
	}
	if vals[0] != "A" || vals[1] != "B" {
		t.Errorf("expected [A B], got %v", vals)
	}
}

func TestConvertToMetaCondition_InOnlyWhitespaceParts(t *testing.T) {
	f := MetaFilterCondition{Key: "cat", Value: "A, ,  ,B", Op: "in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 2 {
		t.Errorf("expected 2 values after filtering whitespace, got %d: %v", len(vals), vals)
	}
	if vals[0] != "A" || vals[1] != "B" {
		t.Errorf("expected [A B], got %v", vals)
	}
}

func TestConvertToMetaCondition_InAllEmptyParts(t *testing.T) {
	f := MetaFilterCondition{Key: "cat", Value: ",,,", Op: "in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 0 {
		t.Errorf("expected 0 values for all-empty input, got %d: %v", len(vals), vals)
	}
}

func TestConvertToMetaCondition_NotInEmptyParts(t *testing.T) {
	f := MetaFilterCondition{Key: "cat", Value: "A,,B", Op: "not in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 2 {
		t.Errorf("expected 2 values after filtering empty parts, got %d: %v", len(vals), vals)
	}
}

func TestConvertToMetaCondition_InMixedSpaces(t *testing.T) {
	f := MetaFilterCondition{Key: "cat", Value: " A , B , C ", Op: "in"}
	mc := convertToMetaCondition(f)
	vals, ok := mc.Value.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", mc.Value)
	}
	if len(vals) != 3 {
		t.Errorf("expected 3 values, got %d: %v", len(vals), vals)
	}
	if vals[0] != "A" || vals[1] != "B" || vals[2] != "C" {
		t.Errorf("expected [A B C], got %v", vals)
	}
}

// buildValueMap constructs the valueMap shape that applySingleCondition
// expects: metaData[key] is a map[string]interface{} where each value is
// the list of doc IDs that carry that metadata value.
func buildValueMap(pairs map[string][]string) map[string]interface{} {
	m := make(map[string]interface{}, len(pairs))
	for k, ids := range pairs {
		m[k] = ids
	}
	return m
}

func asSortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

// TestApplySingleConditionRelationalNumericOrdering pins down the fix for
// lexicographic ordering in relational operators. Before the fix, "10" < "2"
// evaluated true, so a filter like `year > 2` against values {2, 10, 20, 100}
// would have produced {10, 100, 20} (lexicographic) instead of {10, 20, 100}
// (numeric). The same bug applied to <, >=, <=.
func TestApplySingleConditionRelationalNumericOrdering(t *testing.T) {
	// valueMap[year] = { value -> [docID,...] }
	metaData := map[string]interface{}{
		"year": buildValueMap(map[string][]string{
			"2":   {"d-2"},
			"10":  {"d-10"},
			"20":  {"d-20"},
			"100": {"d-100"},
		}),
	}

	tests := []struct {
		name string
		op   string
		val  string
		want []string
	}{
		{name: "gt_returns_strictly_greater", op: ">", val: "2", want: []string{"d-10", "d-20", "d-100"}},
		{name: "gt_lexicographic_trap", op: ">", val: "20", want: []string{"d-100"}}, // "100" > "20" numerically, not lex
		{name: "lt_returns_strictly_lesser", op: "<", val: "20", want: []string{"d-2", "d-10"}},
		{name: "lt_lexicographic_trap", op: "<", val: "10", want: []string{"d-2"}}, // "2" < "10" numerically, not lex
		{name: "gte_includes_equal", op: ">=", val: "10", want: []string{"d-10", "d-20", "d-100"}},
		{name: "lte_includes_equal", op: "<=", val: "10", want: []string{"d-2", "d-10"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond := MetaFilterCondition{Key: "year", Op: tt.op, Value: tt.val}
			got := asSortedIDs(applySingleCondition(metaData, cond))
			want := asSortedIDs(tt.want)
			if len(got) != len(want) {
				t.Fatalf("len = %d (%v), want %d (%v)", len(got), got, len(want), want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("[%d] = %q, want %q (full got=%v want=%v)", i, got[i], want[i], got, want)
				}
			}
		})
	}
}

// TestApplySingleConditionRelationalFallsBackToString ensures non-numeric
// metadata (names, tags, etc.) still works via lexicographic comparison when
// at least one side isn't a valid number.
func TestApplySingleConditionRelationalFallsBackToString(t *testing.T) {
	metaData := map[string]interface{}{
		"tag": buildValueMap(map[string][]string{
			"apple":  {"d-a"},
			"banana": {"d-b"},
			"cherry": {"d-c"},
		}),
	}

	tests := []struct {
		op   string
		val  string
		want []string
	}{
		{op: ">", val: "banana", want: []string{"d-c"}},        // "cherry" > "banana"
		{op: "<", val: "cherry", want: []string{"d-a", "d-b"}}, // apple, banana
		{op: ">=", val: "banana", want: []string{"d-b", "d-c"}},
		{op: "<=", val: "banana", want: []string{"d-a", "d-b"}},
	}
	for _, tt := range tests {
		t.Run(tt.op+tt.val, func(t *testing.T) {
			cond := MetaFilterCondition{Key: "tag", Op: tt.op, Value: tt.val}
			got := asSortedIDs(applySingleCondition(metaData, cond))
			want := asSortedIDs(tt.want)
			if len(got) != len(want) {
				t.Fatalf("op=%q val=%q: got %v, want %v", tt.op, tt.val, got, want)
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("op=%q val=%q [%d] = %q, want %q", tt.op, tt.val, i, got[i], want[i])
				}
			}
		})
	}
}

// TestCompareValuesDirectly exercises the helper in isolation for
// completeness — in particular, the numeric-only path and the mixed
// numeric/string fallback path.
func TestCompareValuesDirectly(t *testing.T) {
	cases := []struct {
		v1, v2, op string
		want       bool
	}{
		// Numeric ordering — the bug we are fixing
		{"10", "2", ">", true},
		{"2", "10", ">", false},
		{"100", "20", "<", false},
		{"20", "100", "<", true},
		{"10", "10", ">=", true},
		{"10", "10", "<=", true},
		// String fallback
		{"banana", "apple", ">", true},
		{"apple", "banana", ">", false},
		// Mixed: one side is a number, the other isn't → fallback
		{"10", "banana", ">", false},
		{"banana", "10", ">", true},
		// Unknown op
		{"1", "2", "==", false},
	}
	for _, tt := range cases {
		got := compareValues(tt.v1, tt.v2, tt.op)
		if got != tt.want {
			t.Errorf("compareValues(%q, %q, %q) = %v, want %v", tt.v1, tt.v2, tt.op, got, tt.want)
		}
	}
}
