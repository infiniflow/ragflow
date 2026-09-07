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

package common

import (
	"testing"
)

func TestParseAndConvert_OperatorMapping(t *testing.T) {
	input := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "Zhang San"},
			map[string]interface{}{"name": "date", "comparison_operator": ">=", "value": "2024-01-01"},
		},
	}
	result := ParseAndConvert(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Logic != "and" {
		t.Errorf("expected logic 'and', got '%s'", result.Logic)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(result.Conditions))
	}
	if result.Conditions[0].Operator != "=" {
		t.Errorf("expected '=', got '%s'", result.Conditions[0].Operator)
	}
	if result.Conditions[1].Operator != "≥" {
		t.Errorf("expected '≥', got '%s'", result.Conditions[1].Operator)
	}
}

func TestParseAndConvert_WithLogic(t *testing.T) {
	input := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "Zhang San"},
		},
		"logic": "or",
	}
	result := ParseAndConvert(input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Logic != "or" {
		t.Errorf("expected logic 'or', got '%s'", result.Logic)
	}
}

func TestParseAndConvert_NilInput(t *testing.T) {
	result := ParseAndConvert(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseAndConvert_EmptyConditions(t *testing.T) {
	result := ParseAndConvert(map[string]interface{}{})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestParseAndConvert_NoName(t *testing.T) {
	input := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"comparison_operator": "is", "value": "x"},
		},
	}
	result := ParseAndConvert(input)
	if result != nil {
		t.Errorf("expected nil for empty name, got %v", result)
	}
}

func TestConvertOperator(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"is", "="},
		{"not is", "≠"},
		{">=", "≥"},
		{"<=", "≤"},
		{"!=", "≠"},
		{"contains", "contains"},
		{"==", "="},
		{"start with", "start with"},
	}
	for _, tt := range tests {
		got := convertOperator(tt.input)
		if got != tt.expected {
			t.Errorf("convertOperator(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestMetaFilter_Equals(t *testing.T) {
	metas := MetaData{
		"author": {"Zhang San": {"doc1", "doc2"}, "Li Si": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "=", Key: "author", Value: "Zhang San"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_NumberEquals(t *testing.T) {
	metas := MetaData{
		"year": {"2024": {"doc1"}, "2025": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "=", Key: "year", Value: float64(2024)}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_GreaterThan(t *testing.T) {
	metas := MetaData{
		"score": {"85": {"doc1"}, "92": {"doc2"}, "70": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: ">", Key: "score", Value: "80"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_GreaterThanOrEqual(t *testing.T) {
	metas := MetaData{
		"score": {"85": {"doc1"}, "80": {"doc2"}, "70": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "≥", Key: "score", Value: "80"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_DateComparison(t *testing.T) {
	metas := MetaData{
		"date": {"2024-06-01": {"doc1"}, "2024-07-15": {"doc2"}, "2024-05-01": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: ">", Key: "date", Value: "2024-06-01"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestMetaFilter_DateVsNonDate(t *testing.T) {
	metas := MetaData{
		"date_field": {"xyz": {"doc1"}, "2024-06-01": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: ">", Key: "date_field", Value: "2024-01-01"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected only date-match [doc2], got %v", result)
	}
}

func TestMetaFilter_Contains(t *testing.T) {
	metas := MetaData{
		"title": {"report 2024": {"doc1"}, "summary": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "contains", Key: "title", Value: "report"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_NotContains(t *testing.T) {
	metas := MetaData{
		"title": {"report 2024": {"doc1"}, "summary": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "not contains", Key: "title", Value: "report"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestMetaFilter_StartWith(t *testing.T) {
	metas := MetaData{
		"code": {"ABC-123": {"doc1"}, "XYZ-456": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "start with", Key: "code", Value: "abc"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_EndWith(t *testing.T) {
	metas := MetaData{
		"code": {"ABC-123": {"doc1"}, "ABC-456": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "end with", Key: "code", Value: "123"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_Empty(t *testing.T) {
	metas := MetaData{
		"field": {"": {"doc1"}, "value": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "empty", Key: "field", Value: nil}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_NotEmpty(t *testing.T) {
	metas := MetaData{
		"field": {"": {"doc1"}, "value": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "not empty", Key: "field", Value: nil}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestMetaFilter_AndLogic(t *testing.T) {
	metas := MetaData{
		"author": {"Zhang San": {"doc1", "doc2"}, "Li Si": {"doc3"}},
		"year":   {"2024": {"doc1"}, "2025": {"doc2", "doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{
			{Operator: "=", Key: "author", Value: "Zhang San"},
			{Operator: "=", Key: "year", Value: "2024"},
		},
		Logic: "and",
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_OrLogic(t *testing.T) {
	metas := MetaData{
		"author": {"Zhang San": {"doc1"}, "Li Si": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{
			{Operator: "=", Key: "author", Value: "Zhang San"},
			{Operator: "=", Key: "author", Value: "Li Si"},
		},
		Logic: "or",
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_KeyNotFound_And(t *testing.T) {
	metas := MetaData{"author": {"Zhang San": {"doc1"}}}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "=", Key: "nonexistent", Value: "x"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestMetaFilter_KeyNotFound_Or(t *testing.T) {
	metas := MetaData{"author": {"Zhang San": {"doc1"}}}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "=", Key: "nonexistent", Value: "x"}},
		Logic:      "or",
	}
	result := MetaFilter(metas, input)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestMetaFilter_NilInput(t *testing.T) {
	result := MetaFilter(nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMetaFilter_EmptyInput(t *testing.T) {
	metas := MetaData{"author": {"Zhang San": {"doc1"}}}
	result := MetaFilter(metas, &MetaFilterInput{})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// --- filterSet unit tests ---

func TestFilterSet_In_Basic(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}}
	value := []interface{}{"A", "B"}
	result := filterSet(v2docs, "in", value)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestFilterSet_In_NoMatch(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}, "B": {"doc2"}}
	value := []interface{}{"X"}
	result := filterSet(v2docs, "in", value)
	if len(result) != 0 {
		t.Errorf("expected 0 docs, got %d: %v", len(result), result)
	}
}

func TestFilterSet_In_CaseSensitive(t *testing.T) {
	v2docs := MetaValueDocs{"ABC": {"doc1"}, "abc": {"doc2"}}
	value := []interface{}{"abc"}
	result := filterSet(v2docs, "in", value)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected only [doc2] (case-sensitive), got %v", result)
	}
}

func TestFilterSet_In_NonListValue(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}}
	result := filterSet(v2docs, "in", "not a list")
	if result != nil {
		t.Errorf("expected nil for non-list value, got %v", result)
	}
}

func TestFilterSet_In_EmptyList(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}}
	result := filterSet(v2docs, "in", []interface{}{})
	if len(result) != 0 {
		t.Errorf("expected 0 docs for empty list, got %d", len(result))
	}
}

func TestFilterSet_NotIn_Basic(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}}
	value := []interface{}{"A"}
	result := filterSet(v2docs, "not in", value)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestFilterSet_NotIn_CaseInsensitive(t *testing.T) {
	v2docs := MetaValueDocs{"ABC": {"doc1"}, "xyz": {"doc2"}}
	value := []interface{}{"abc"}
	result := filterSet(v2docs, "not in", value)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected only [doc2] (case-insensitive), got %v", result)
	}
}

func TestFilterSet_NotIn_ExcludeAll(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}, "B": {"doc2"}}
	value := []interface{}{"A", "B"}
	result := filterSet(v2docs, "not in", value)
	if len(result) != 0 {
		t.Errorf("expected 0 docs when all excluded, got %d", len(result))
	}
}

func TestFilterSet_NotIn_NonListValue(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}}
	result := filterSet(v2docs, "not in", "not a list")
	if result != nil {
		t.Errorf("expected nil for non-list value, got %v", result)
	}
}

func TestFilterSet_NotIn_MultipleExcludes(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}}
	value := []interface{}{"A", "B"}
	result := filterSet(v2docs, "not in", value)
	if len(result) != 1 || result[0] != "doc3" {
		t.Errorf("expected only [doc3], got %v", result)
	}
}

// --- MetaFilter integration tests for "in" / "not in" ---

func TestMetaFilter_In(t *testing.T) {
	metas := MetaData{
		"category": {"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "in", Key: "category", Value: []interface{}{"A", "B"}}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_NotIn(t *testing.T) {
	metas := MetaData{
		"category": {"A": {"doc1"}, "B": {"doc2"}, "C": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "not in", Key: "category", Value: []interface{}{"A"}}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs (B,C), got %d: %v", len(result), result)
	}
}

func TestMetaFilter_NotIn_CaseInsensitive(t *testing.T) {
	metas := MetaData{
		"code": {"ABC": {"doc1"}, "xyz": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "not in", Key: "code", Value: []interface{}{"abc"}}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected only [doc2] (case-insensitive), got %v", result)
	}
}

func TestMetaFilter_In_CaseSensitive(t *testing.T) {
	metas := MetaData{
		"code": {"ABC": {"doc1"}, "abc": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "in", Key: "code", Value: []interface{}{"abc"}}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected only [doc2] (case-sensitive), got %v", result)
	}
}

func TestFilterSet_NotIn_EmptyList(t *testing.T) {
	v2docs := MetaValueDocs{"A": {"doc1"}, "B": {"doc2"}}
	result := filterSet(v2docs, "not in", []interface{}{})
	if len(result) != 2 {
		t.Errorf("expected all 2 docs for not in empty list, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_LessThan(t *testing.T) {
	metas := MetaData{
		"score": {"85": {"doc1"}, "70": {"doc2"}, "80": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "<", Key: "score", Value: "80"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2] for <80, got %v", result)
	}
}

func TestMetaFilter_LessThanOrEqual(t *testing.T) {
	metas := MetaData{
		"score": {"85": {"doc1"}, "70": {"doc2"}, "80": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "≤", Key: "score", Value: "80"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs for ≤80, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_NotEquals(t *testing.T) {
	metas := MetaData{
		"author": {"Zhang San": {"doc1"}, "Li Si": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "≠", Key: "author", Value: "Zhang San"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2] for ≠, got %v", result)
	}
}
