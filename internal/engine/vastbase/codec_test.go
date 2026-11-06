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

package vastbase

import (
	"encoding/json"
	"testing"

	"github.com/lib/pq"
)

// TestNormalizeChunkEncodesPythonStorageFormats checks the physical
// encodings the Python connector's storage formats map to.
func TestNormalizeChunkEncodesPythonStorageFormats(t *testing.T) {
	document := map[string]interface{}{
		"id":            "chunk-1",
		"kb_id":         []interface{}{"kb-1", "kb-2"},
		"important_kwd": []interface{}{"alpha", "beta"},
		"source_id":     []interface{}{"doc-1"},
		"docnm_kwd":     "plain text title",
		"metadata":      map[string]interface{}{"_group_id": "g1"},
		"position_int":  []interface{}{[]interface{}{int64(1), 2, 3, 4, 5}, []interface{}{int64(6), 7, 8, 9, 10}},
		"page_num_int":  []interface{}{int64(1), int64(2)},
		"q_3_vec":       []interface{}{0.1, 0.2, 0.3},
		"custom_field":  "kept as dynamic column",
	}
	got, err := normalizeChunk(document)
	if err != nil {
		t.Fatal(err)
	}
	if got["kb_id"] != "kb-1" {
		t.Fatalf("kb_id list must collapse to first element, got %#v", got["kb_id"])
	}
	if got["important_kwd"] != "alpha###beta" || got["source_id"] != "doc-1" {
		t.Fatalf("keyword lists must ###-join, got %#v / %#v", got["important_kwd"], got["source_id"])
	}
	if got["docnm_kwd"] != "plain text title" {
		t.Fatalf("docnm_kwd stores plain text, got %#v", got["docnm_kwd"])
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(got["metadata"].(string)), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["_group_id"] != "g1" {
		t.Fatalf("metadata map must JSON-encode, got %#v", got["metadata"])
	}
	position, ok := got["position_int"].(pq.Int64Array)
	if !ok || len(position) != 10 || position[0] != 1 || position[9] != 10 {
		t.Fatalf("position_int must flatten to int64[], got %#v", got["position_int"])
	}
	pages, ok := got["page_num_int"].(pq.Int64Array)
	if !ok || len(pages) != 2 {
		t.Fatalf("page_num_int must bind as int64[], got %#v", got["page_num_int"])
	}
	if got["q_3_vec"] != "[0.1,0.2,0.3]" {
		t.Fatalf("vector text encoding changed: %#v", got["q_3_vec"])
	}
	if got["custom_field"] != "kept as dynamic column" {
		t.Fatalf("unknown fields must pass through for dynamic columns, got %#v", got)
	}
}

// TestNormalizeChunkKeepsZeroAndFalsyValues checks that zero-valued fields
// survive normalization instead of being dropped as absent.
func TestNormalizeChunkKeepsZeroAndFalsyValues(t *testing.T) {
	got, err := normalizeChunk(map[string]interface{}{
		"available_int": 0, "weight_flt": 0.0, "removed_kwd": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["available_int"] != 0 || got["weight_flt"] != 0.0 || got["removed_kwd"] != "" {
		t.Fatalf("zero values must survive encode (DEFAULT '' only fills gaps): %#v", got)
	}
}

// TestNormalizeMemoryAliasesAndDefaults checks memory field aliasing and
// default filling.
func TestNormalizeMemoryAliasesAndDefaults(t *testing.T) {
	got, err := normalizeMemory(map[string]interface{}{
		"id": "memory-1_1", "message_id": "1",
		"message_type": "raw", "status": false,
		"content":       "hello world",
		"content_embed": []interface{}{0.25, 0.5},
	}, "memory-1")
	if err != nil {
		t.Fatal(err)
	}
	if got["message_type_kwd"] != "raw" {
		t.Fatalf("message_type must map to message_type_kwd: %#v", got)
	}
	if got["status_int"] != 0 {
		t.Fatalf("status bool must encode as status_int 0/1, got %#v", got["status_int"])
	}
	if got["content_ltks"] != "hello world" {
		t.Fatalf("content must map to content_ltks: %#v", got)
	}
	if got["q_2_vec"] != "[0.25,0.5]" {
		t.Fatalf("content_embed must land in the dimension column: %#v", got["q_2_vec"])
	}
	if got["tokenized_content_ltks"] == "" {
		t.Fatal("tokenized_content_ltks companion must be derived from content")
	}
	if got["memory_id"] != "memory-1" {
		t.Fatalf("memory_id must default to the dataset id, got %#v", got["memory_id"])
	}
	if got["zone_id"] != 0 {
		t.Fatalf("zone_id must default to 0, got %#v", got["zone_id"])
	}
	if _, present := got["forget_at_flt"]; present {
		t.Fatal("memory tables carry no forget_at_flt column")
	}
}

// TestNormalizeMemoryStatusDefaultsToOne checks the memory status default.
func TestNormalizeMemoryStatusDefaultsToOne(t *testing.T) {
	got, err := normalizeMemory(map[string]interface{}{"id": "m1", "memory_id": "m"}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if got["status_int"] != 1 {
		t.Fatalf("status must default to 1 (valid), got %#v", got["status_int"])
	}
}

// TestNormalizeSkillDerivesTokenFields checks that skill normalization
// derives the tokenized companion fields.
func TestNormalizeSkillDerivesTokenFields(t *testing.T) {
	got, err := normalizeSkill(map[string]interface{}{
		"skill_id": "s-1", "name": "pdf reader", "content": "opens files",
		"q_2_vec": []interface{}{1, 0},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got["skill_id"] != "s-1" {
		t.Fatalf("skill_id must be kept, got %#v", got["skill_id"])
	}
	if got["name_tks"] == "" || got["content_tks"] == "" {
		t.Fatalf("token companion fields must be derived: %#v", got)
	}
	if _, present := got["tags_tks"]; !present {
		t.Fatalf("absent tags must still carry an (empty) token field: %#v", got)
	}
	if got["q_2_vec"] != "[1,0]" {
		t.Fatalf("skill vector encoding changed: %#v", got["q_2_vec"])
	}
}

// TestEncodeUpdateValueRoutesByKind checks UPDATE value encoding per row
// kind and column.
func TestEncodeUpdateValueRoutesByKind(t *testing.T) {
	vector, err := encodeUpdateValue("chunk", "q_2_vec", []interface{}{0.5, -0.5})
	if err != nil || vector != "[0.5,-0.5]" {
		t.Fatalf("vector update must encode text, got %#v err %v", vector, err)
	}
	status, err := encodeUpdateValue("memory", "status_int", true)
	if err != nil || status != 1 {
		t.Fatalf("memory status update must encode bool as 1, got %#v err %v", status, err)
	}
	joined, err := encodeUpdateValue("chunk", "important_kwd", []interface{}{"a", "b"})
	if err != nil || joined != "a###b" {
		t.Fatalf("chunk update must keep ### joining, got %#v err %v", joined, err)
	}
	raw, err := encodeUpdateValue("memory", "content_ltks", "raw text")
	if err != nil || raw != "raw text" {
		t.Fatalf("memory text update must pass through, got %#v err %v", raw, err)
	}
}

// TestVectorEncodingIsFloat32Precision checks that vector text round-trips
// through float32 precision, matching the Python connector.
func TestVectorEncodingIsFloat32Precision(t *testing.T) {
	// Python wrote np.float32 components; values beyond float32 precision
	// must round to the float32 rendering so shared tables stay byte-stable.
	encoded, err := encodeVector([]interface{}{1.0 / 3.0, 1e-40, 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "[0.33333334,1e-40,0.1]" {
		t.Fatalf("float32 rendering changed: %q", encoded)
	}
	if _, err := encodeVector("not a vector"); err == nil {
		t.Fatal("non-numeric vector must error")
	}
}

// TestZeroVectorAndDimensionInference checks the zero-vector placeholder
// text and dimension inference from documents.
func TestZeroVectorAndDimensionInference(t *testing.T) {
	if zeroVector(1) != "[0]" || zeroVector(3) != "[0,0,0]" {
		t.Fatalf("zeroVector format changed: %q %q", zeroVector(1), zeroVector(3))
	}
	if d := vectorDimension(map[string]interface{}{"q_1024_vec": []interface{}{0.1}}); d != 1024 {
		t.Fatalf("column-name dimension inference = %d", d)
	}
	if d := vectorDimension(map[string]interface{}{"content_embed": []interface{}{1, 2, 3, 4}}); d != 4 {
		t.Fatalf("content_embed dimension inference = %d", d)
	}
	if d := vectorDimension(map[string]interface{}{"id": "x"}); d != 0 {
		t.Fatalf("absent vector must infer 0, got %d", d)
	}
}

// TestFieldKeywordClassification checks which columns count as
// keyword-joined fields.
func TestFieldKeywordClassification(t *testing.T) {
	for field, want := range map[string]bool{
		"source_id": true, "important_kwd": true, "entity_type_kwd": true,
		"docnm_kwd": false, "knowledge_graph_kwd": false, "title_tks": false,
		"content_with_weight": false, "q_3_vec": false,
	} {
		if got := fieldKeyword(field); got != want {
			t.Fatalf("fieldKeyword(%q) = %v, want %v", field, got, want)
		}
	}
}
