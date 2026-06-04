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
			map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "张三"},
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
			map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "张三"},
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
		"author": {"张三": {"doc1", "doc2"}, "李四": {"doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "=", Key: "author", Value: "张三"}},
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
		"author": {"张三": {"doc1", "doc2"}, "李四": {"doc3"}},
		"year":   {"2024": {"doc1"}, "2025": {"doc2", "doc3"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{
			{Operator: "=", Key: "author", Value: "张三"},
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
		"author": {"张三": {"doc1"}, "李四": {"doc2"}},
	}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{
			{Operator: "=", Key: "author", Value: "张三"},
			{Operator: "=", Key: "author", Value: "李四"},
		},
		Logic: "or",
	}
	result := MetaFilter(metas, input)
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_KeyNotFound_And(t *testing.T) {
	metas := MetaData{"author": {"张三": {"doc1"}}}
	input := &MetaFilterInput{
		Conditions: []MetaCondition{{Operator: "=", Key: "nonexistent", Value: "x"}},
	}
	result := MetaFilter(metas, input)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestMetaFilter_KeyNotFound_Or(t *testing.T) {
	metas := MetaData{"author": {"张三": {"doc1"}}}
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
	metas := MetaData{"author": {"张三": {"doc1"}}}
	result := MetaFilter(metas, &MetaFilterInput{})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}
