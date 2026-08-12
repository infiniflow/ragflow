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

package oceanbase

import (
	"reflect"
	"testing"
)

func TestGetAggregationPreservesScalarStrings(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"docnm_kwd": "report,final.pdf"},
		{"docnm_kwd": "report,final.pdf"},
		{"docnm_kwd": "guide.pdf"},
		{"docnm_kwd": ""},
		{},
	}
	want := []map[string]interface{}{
		{"key": "report,final.pdf", "count": 2},
		{"key": "guide.pdf", "count": 1},
	}
	if got := engine.GetAggregation(chunks, "docnm_kwd"); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAggregation() = %#v, want %#v", got, want)
	}
}

func TestGetAggregationCountsArrayElements(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"tag_kwd": []string{"rag", "database"}},
		{"tag_kwd": []interface{}{"rag", "", 7}},
	}
	want := []map[string]interface{}{
		{"key": "rag", "count": 2},
		{"key": "database", "count": 1},
	}
	if got := engine.GetAggregation(chunks, "tag_kwd"); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAggregation() = %#v, want %#v", got, want)
	}
}

func TestGetHighlightUsesBoundariesAndTokens(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"id": "english", "content_with_weight": "Apple cat concatenate.", "content_ltks": "apple cat concatenate"},
		{"id": "chinese", "content_with_weight": "这是数据库系统", "content_ltks": "这是 数据库 系统"},
		{"id": "missing", "content_with_weight": "nothing relevant", "content_ltks": "nothing relevant"},
	}
	want := map[string]string{
		"english": "<em>Apple</em> <em>cat</em> concatenate.",
		"chinese": "这是<em>数据库</em>系统",
	}
	if got := engine.GetHighlight(chunks, []string{"apple", "cat", "数据库"}, "content_with_weight"); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight() = %#v, want %#v", got, want)
	}
}

func TestGetHighlightUsesSkillID(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{{
		"skill_id": "skill-1",
		"content":  "OceanBase search",
	}}
	want := map[string]string{"skill-1": "<em>OceanBase</em> search"}
	if got := engine.GetHighlight(chunks, []string{"oceanbase"}, "content"); !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight() = %#v, want %#v", got, want)
	}
}
