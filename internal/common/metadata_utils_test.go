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

func TestConvertConditions_OperatorMapping(t *testing.T) {
	input := map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "张三"},
			map[string]interface{}{"name": "date", "comparison_operator": ">=", "value": "2024-01-01"},
		},
	}
	result := ConvertConditions(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(result))
	}
	if result[0].Operator != "=" {
		t.Errorf("expected '=', got '%s'", result[0].Operator)
	}
	if result[1].Operator != "≥" {
		t.Errorf("expected '≥', got '%s'", result[1].Operator)
	}
}

func TestConvertConditions_Nil(t *testing.T) {
	result := ConvertConditions(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestConvertConditions_Empty(t *testing.T) {
	result := ConvertConditions(map[string]interface{}{})
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMetaFilter_Equals(t *testing.T) {
	metas := map[string]map[string][]string{
		"author": {
			"张三": {"doc1", "doc2"},
			"李四": {"doc3"},
		},
	}
	filters := []MetaCondition{{Operator: "=", Key: "author", Value: "张三"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_NumberEquals(t *testing.T) {
	metas := map[string]map[string][]string{
		"year": {
			"2024": {"doc1"},
			"2025": {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "=", Key: "year", Value: float64(2024)}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_GreaterThan(t *testing.T) {
	metas := map[string]map[string][]string{
		"score": {
			"85": {"doc1"},
			"92": {"doc2"},
			"70": {"doc3"},
		},
	}
	filters := []MetaCondition{{Operator: ">", Key: "score", Value: "80"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_GreaterThanOrEqual(t *testing.T) {
	metas := map[string]map[string][]string{
		"score": {
			"85": {"doc1"},
			"80": {"doc2"},
			"70": {"doc3"},
		},
	}
	filters := []MetaCondition{{Operator: "≥", Key: "score", Value: "80"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_DateComparison(t *testing.T) {
	metas := map[string]map[string][]string{
		"date": {
			"2024-06-01": {"doc1"},
			"2024-07-15": {"doc2"},
			"2024-05-01": {"doc3"},
		},
	}
	filters := []MetaCondition{{Operator: ">", Key: "date", Value: "2024-06-01"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestMetaFilter_Contains(t *testing.T) {
	metas := map[string]map[string][]string{
		"title": {
			"report 2024": {"doc1"},
			"summary":    {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "contains", Key: "title", Value: "report"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_NotContains(t *testing.T) {
	metas := map[string]map[string][]string{
		"title": {
			"report 2024": {"doc1"},
			"summary":    {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "not contains", Key: "title", Value: "report"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestMetaFilter_StartWith(t *testing.T) {
	metas := map[string]map[string][]string{
		"code": {
			"ABC-123": {"doc1"},
			"XYZ-456": {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "start with", Key: "code", Value: "abc"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_EndWith(t *testing.T) {
	metas := map[string]map[string][]string{
		"code": {
			"ABC-123": {"doc1"},
			"ABC-456": {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "end with", Key: "code", Value: "123"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_Empty(t *testing.T) {
	metas := map[string]map[string][]string{
		"field": {
			"":      {"doc1"},
			"value": {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "empty", Key: "field", Value: nil}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_NotEmpty(t *testing.T) {
	metas := map[string]map[string][]string{
		"field": {
			"":      {"doc1"},
			"value": {"doc2"},
		},
	}
	filters := []MetaCondition{{Operator: "not empty", Key: "field", Value: nil}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc2" {
		t.Errorf("expected [doc2], got %v", result)
	}
}

func TestMetaFilter_AndLogic(t *testing.T) {
	metas := map[string]map[string][]string{
		"author": {
			"张三": {"doc1", "doc2"},
			"李四": {"doc3"},
		},
		"year": {
			"2024": {"doc1"},
			"2025": {"doc2", "doc3"},
		},
	}
	filters := []MetaCondition{
		{Operator: "=", Key: "author", Value: "张三"},
		{Operator: "=", Key: "year", Value: "2024"},
	}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 1 || result[0] != "doc1" {
		t.Errorf("expected [doc1], got %v", result)
	}
}

func TestMetaFilter_OrLogic(t *testing.T) {
	metas := map[string]map[string][]string{
		"author": {
			"张三": {"doc1"},
			"李四": {"doc2"},
		},
	}
	filters := []MetaCondition{
		{Operator: "=", Key: "author", Value: "张三"},
		{Operator: "=", Key: "author", Value: "李四"},
	}
	result := MetaFilter(metas, filters, "or")
	if len(result) != 2 {
		t.Errorf("expected 2 docs, got %d: %v", len(result), result)
	}
}

func TestMetaFilter_KeyNotFound_And(t *testing.T) {
	metas := map[string]map[string][]string{
		"author": {"张三": {"doc1"}},
	}
	filters := []MetaCondition{{Operator: "=", Key: "nonexistent", Value: "x"}}
	result := MetaFilter(metas, filters, "and")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestMetaFilter_KeyNotFound_Or(t *testing.T) {
	metas := map[string]map[string][]string{
		"author": {"张三": {"doc1"}},
	}
	filters := []MetaCondition{{Operator: "=", Key: "nonexistent", Value: "x"}}
	result := MetaFilter(metas, filters, "or")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}
