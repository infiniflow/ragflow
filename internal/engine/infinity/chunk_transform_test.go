// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package infinity

import (
	"reflect"
	"testing"
)

// TestTransformChunkFields_IngestionShape is the T0 unit-tier read-back
// baseline for the Infinity engine (issue #17371). Unlike Elasticsearch, which
// stores the suffixed field names verbatim, Infinity reverse-maps the
// ingestion-emitted names to its own base names inside transformChunkFields.
// This test calls that transform directly (the engine requires a live Infinity
// instance, so it cannot be exercised via httptest) and asserts the mapping
// stays stable. It guards T2: when the suffixing is moved out of ingestion into
// the engine write boundary, transformChunkFields must keep producing the same
// base-name document.
//
// The input mirrors the shape produced by indexdoc.ProcessChunksForPipeline
// after T1 (kb_id is a plain string).
func TestTransformChunkFields_IngestionShape(t *testing.T) {
	chunk := map[string]interface{}{
		"doc_id":               "doc-1",
		"id":                   "chunk-1",
		"kb_id":                "kb-1",
		"docnm_kwd":            "sample.md",
		"content_with_weight":  "hello world",
		"create_timestamp_flt": float64(123.0),
		"question_kwd":         []interface{}{"q1", "q2"},
		"important_kwd":        []interface{}{"k1"},
		"page_num_int":         int(1),
		"position_int":         int(2),
		"mom_id":               "parent-1",
		"available_int":        int(0),
	}

	got := transformChunkFields(chunk, nil)

	want := map[string]interface{}{
		"doc_id":               "doc-1",
		"id":                   "chunk-1",
		"kb_id":                "kb-1",
		"docnm":                "sample.md",
		"content":              "hello world",
		"create_timestamp_flt": float64(123.0),
		"questions":            "q1\nq2",
		"important_keywords":   "k1",
		"page_num_int":         int(1),
		"position_int":         int(2),
		"mom_id":               "parent-1",
		"available_int":        int(0),
	}

	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("transformed doc missing field %q", k)
			continue
		}
		if !reflect.DeepEqual(gv, wv) {
			t.Errorf("field %q = %#v, want %#v", k, gv, wv)
		}
	}

	// The suffixed ingestion-only names must not leak into the transformed doc.
	for _, leaked := range []string{"docnm_kwd", "content_with_weight", "question_kwd", "important_kwd"} {
		if _, ok := got[leaked]; ok {
			t.Errorf("transform leaked ingestion-only field %q into Infinity doc", leaked)
		}
	}
}

// TestTransformChunkFieldsHexEncodesNativeIntSlices pins the Go-native slice
// shapes the ingestion pipeline produces: AddPositions emits []int / [][]int
// (internal/ingestion/task/indexdoc/position.go), and the Infinity columns are
// VARCHAR holding the hex form. Left unconverted, Infinity rejects the insert
// with "Not support to convert Tensor(int64,5) to Varchar" (3049).
func TestTransformChunkFieldsHexEncodesNativeIntSlices(t *testing.T) {
	chunk := map[string]interface{}{
		"position_int": [][]int{{1, 10, 20, 30, 40}},
		"page_num_int": []int{1},
		"top_int":      []int{30},
	}

	got := transformChunkFields(chunk, nil)

	// Python: "_".join(f"{num:08x}" for num in flattened positions).
	if want := "00000001_0000000a_00000014_0000001e_00000028"; got["position_int"] != want {
		t.Errorf("position_int = %v, want %v", got["position_int"], want)
	}
	if want := "00000001"; got["page_num_int"] != want {
		t.Errorf("page_num_int = %v, want %v", got["page_num_int"], want)
	}
	if want := "0000001e"; got["top_int"] != want {
		t.Errorf("top_int = %v, want %v", got["top_int"], want)
	}
}

// TestTransformChunkFieldsHexEncodesJSONPositionShape keeps the decoded-JSON
// shape working: numbers arrive as float64 inside []interface{} rows.
func TestTransformChunkFieldsHexEncodesJSONPositionShape(t *testing.T) {
	chunk := map[string]interface{}{
		"position_int": []interface{}{[]interface{}{float64(1), float64(10), float64(20), float64(30), float64(40)}},
		"page_num_int": []interface{}{float64(1)},
	}

	got := transformChunkFields(chunk, nil)

	if want := "00000001_0000000a_00000014_0000001e_00000028"; got["position_int"] != want {
		t.Errorf("position_int = %v, want %v", got["position_int"], want)
	}
	if want := "00000001"; got["page_num_int"] != want {
		t.Errorf("page_num_int = %v, want %v", got["page_num_int"], want)
	}
}

// TestTransformChunkFieldsJoinsNativeKeywordSlice pins the other Go-native slice
// the ingestion hands over: important_kwd is a plain []string (SplitKeywords /
// the extractor / the tokenizer), and Python joins it with "," while counting
// the empty entries (infinity_conn.py:528-535).
func TestTransformChunkFieldsJoinsNativeKeywordSlice(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"important_kwd": []string{"alpha", "", "beta"},
	}, nil)

	if want := "alpha,beta"; got["important_keywords"] != want {
		t.Errorf("important_keywords = %v, want %v", got["important_keywords"], want)
	}
	if got["important_kwd_empty_count"] != 1 {
		t.Errorf("important_kwd_empty_count = %v, want 1", got["important_kwd_empty_count"])
	}
}
