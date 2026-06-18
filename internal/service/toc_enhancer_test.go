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
)

func TestGetFloat(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want float64
	}{
		{"present", map[string]interface{}{"score": 3.5}, "score", 3.5},
		{"missing", map[string]interface{}{}, "score", 0},
		{"wrong type", map[string]interface{}{"score": "3.5"}, "score", 0},
		{"nil value", map[string]interface{}{"score": nil}, "score", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFloat(tt.m, tt.key); got != tt.want {
				t.Errorf("getFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetStringDef(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		def  string
		want string
	}{
		{"present", map[string]interface{}{"name": "foo"}, "name", "default", "foo"},
		{"missing", map[string]interface{}{}, "name", "default", "default"},
		{"wrong type", map[string]interface{}{"name": 42}, "name", "default", "default"},
		{"empty string", map[string]interface{}{"name": ""}, "name", "default", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getStringDef(tt.m, tt.key, tt.def); got != tt.want {
				t.Errorf("getStringDef() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSlice(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want []interface{}
	}{
		{"present", map[string]interface{}{"items": []interface{}{1, 2, 3}}, "items", []interface{}{1, 2, 3}},
		{"missing", map[string]interface{}{}, "items", nil},
		{"wrong type", map[string]interface{}{"items": "not a slice"}, "items", nil},
		{"nil value", map[string]interface{}{"items": nil}, "items", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSlice(tt.m, tt.key)
			if len(got) != len(tt.want) {
				t.Errorf("getSlice() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getSlice()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestToFloat64Slice(t *testing.T) {
	tests := []struct {
		name   string
		v      interface{}
		want   []float64
		wantOk bool
	}{
		{"valid", []interface{}{1.0, 2.5, 3.0}, []float64{1.0, 2.5, 3.0}, true},
		{"empty", []interface{}{}, []float64{}, true},
		{"wrong type", "not a slice", nil, false},
		{"non-float element", []interface{}{1.0, "x"}, nil, false},
		{"nil", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toFloat64Slice(tt.v)
			if (got != nil) != tt.wantOk {
				t.Errorf("toFloat64Slice() ok = %v, want %v", got != nil, tt.wantOk)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("toFloat64Slice() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("toFloat64Slice()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAsMap(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want map[string]interface{}
	}{
		{"valid map", map[string]interface{}{"a": 1}, map[string]interface{}{"a": 1}},
		{"nil", nil, nil},
		{"wrong type", "not a map", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asMap(tt.v)
			if len(got) != len(tt.want) {
				t.Errorf("asMap() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("asMap()[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestBoostSimilarity(t *testing.T) {
	cm := map[string]interface{}{"similarity": 1.0}
	boostSimilarity(cm, 0.5)
	if cm["similarity"] != 1.5 {
		t.Errorf("expected 1.5, got %v", cm["similarity"])
	}

	boostSimilarity(cm, 0.0)
	if cm["similarity"] != 1.5 {
		t.Errorf("expected 1.5 (unchanged), got %v", cm["similarity"])
	}
}

func TestTopDocFromChunks_Empty(t *testing.T) {
	docID, docMap := topDocFromChunks(nil)
	if docID != "" || docMap != nil {
		t.Errorf("expected empty, got docID=%q, map=%v", docID, docMap)
	}
}

func TestTopDocFromChunks_SingleDoc(t *testing.T) {
	chunks := []map[string]interface{}{
		map[string]interface{}{"doc_id": "doc1", "kb_id": "kb1", "similarity": 0.8},
		map[string]interface{}{"doc_id": "doc1", "kb_id": "kb1", "similarity": 0.6},
	}
	docID, docMap := topDocFromChunks(chunks)
	if docID != "doc1" {
		t.Errorf("expected doc1, got %q", docID)
	}
	if docMap["doc1"] != "kb1" {
		t.Errorf("expected kb1, got %q", docMap["doc1"])
	}
}

func TestTopDocFromChunks_MultiDoc(t *testing.T) {
	chunks := []map[string]interface{}{
		map[string]interface{}{"doc_id": "doc1", "kb_id": "kb1", "similarity": 0.3},
		map[string]interface{}{"doc_id": "doc2", "kb_id": "kb2", "similarity": 0.9},
		map[string]interface{}{"doc_id": "doc1", "kb_id": "kb1", "similarity": 0.4},
	}
	docID, docMap := topDocFromChunks(chunks)
	if docID != "doc2" {
		t.Errorf("expected doc2 (accumulated 0.9 > doc1's 0.7), got %q", docID)
	}
	if docMap["doc1"] != "kb1" {
		t.Errorf("expected kb1, got %q", docMap["doc1"])
	}
	if docMap["doc2"] != "kb2" {
		t.Errorf("expected kb2, got %q", docMap["doc2"])
	}
}

func TestTopDocFromChunks_NoDocID(t *testing.T) {
	chunks := []map[string]interface{}{
		map[string]interface{}{"similarity": 0.8},
	}
	docID, docMap := topDocFromChunks(chunks)
	if docID != "" || docMap != nil {
		t.Errorf("expected empty, got docID=%q, map=%v", docID, docMap)
	}
}

func TestParseTOCEntries_Empty(t *testing.T) {
	got := parseTOCEntries(nil)
	if len(got) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got))
	}
}

func TestParseTOCEntries_SingleEntry(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_with_weight": `{"level": 1, "title": "Intro", "ids": ["c1"]}`},
	}
	got := parseTOCEntries(chunks)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Level != 1 || got[0].Title != "Intro" || len(got[0].IDs) != 1 || got[0].IDs[0] != "c1" {
		t.Errorf("unexpected entry: %+v", got[0])
	}
}

func TestParseTOCEntries_MultiEntry(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_with_weight": `[
			{"level": 1, "title": "Intro", "ids": ["c1"]},
			{"level": 2, "title": "Details", "ids": ["c2", "c3"]}
		]`},
	}
	got := parseTOCEntries(chunks)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Title != "Intro" || got[1].Title != "Details" {
		t.Errorf("unexpected titles: %q, %q", got[0].Title, got[1].Title)
	}
	if len(got[1].IDs) != 2 {
		t.Errorf("expected 2 IDs for Details, got %v", got[1].IDs)
	}
}

func TestParseTOCEntries_InvalidJSON(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_with_weight": `not json`},
	}
	got := parseTOCEntries(chunks)
	if len(got) != 0 {
		t.Errorf("expected 0 entries for invalid JSON, got %d", len(got))
	}
}

func TestParseTOCEntries_MissingField(t *testing.T) {
	chunks := []map[string]interface{}{
		{"other_field": "value"},
	}
	got := parseTOCEntries(chunks)
	if len(got) != 0 {
		t.Errorf("expected 0 entries for missing content_with_weight, got %d", len(got))
	}
}

func TestSortAndTrimChunks_Nil(t *testing.T) {
	got := sortAndTrimChunks(nil, 3)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestSortAndTrimChunks_Trim(t *testing.T) {
	chunks := []map[string]interface{}{
		map[string]interface{}{"similarity": 0.3},
		map[string]interface{}{"similarity": 0.9},
		map[string]interface{}{"similarity": 0.6},
	}
	got := sortAndTrimChunks(chunks, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if got[0]["similarity"] != 0.9 {
		t.Errorf("expected first similarity 0.9, got %v", got[0])
	}
	if got[1]["similarity"] != 0.6 {
		t.Errorf("expected second similarity 0.6, got %v", got[1])
	}
}

func TestSortAndTrimChunks_AllKept(t *testing.T) {
	chunks := []map[string]interface{}{
		map[string]interface{}{"similarity": 0.3},
		map[string]interface{}{"similarity": 0.9},
	}
	got := sortAndTrimChunks(chunks, 5)
	if len(got) != 2 {
		t.Errorf("expected all 2 chunks kept, got %d", len(got))
	}
}

func TestIndexName(t *testing.T) {
	if got := indexName("tenant1"); got != "ragflow_tenant1" {
		t.Errorf("expected 'ragflow_tenant1', got %q", got)
	}
}
