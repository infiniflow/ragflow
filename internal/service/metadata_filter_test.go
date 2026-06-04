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
