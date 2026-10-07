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
	"reflect"
	"testing"
)

func TestDecodeColumnValueBranches(t *testing.T) {
	vector := decodeColumnValue("chunk", "q_3_vec", "[0.1,0.2,0.3]")
	if !reflect.DeepEqual(vector, []float64{0.1, 0.2, 0.3}) {
		t.Fatalf("vector text decode = %#v", vector)
	}
	keywords := decodeColumnValue("chunk", "important_kwd", "alpha###beta###")
	if !reflect.DeepEqual(keywords, []string{"alpha", "beta"}) {
		t.Fatalf("keyword split must drop empty parts: %#v", keywords)
	}
	positions := decodeColumnValue("chunk", "position_int", "{1,2,3,4,5,6,7,8,9,10}")
	want := [][]int64{{1, 2, 3, 4, 5}, {6, 7, 8, 9, 10}}
	if !reflect.DeepEqual(positions, want) {
		t.Fatalf("position_int regroup = %#v", positions)
	}
	pages := decodeColumnValue("chunk", "page_num_int", "{1,2}")
	if !reflect.DeepEqual(pages, []int64{1, 2}) {
		t.Fatalf("integer array decode = %#v", pages)
	}
	metadata := decodeColumnValue("chunk", "metadata", `{"_group_id":"g1"}`)
	if metadata.(map[string]interface{})["_group_id"] != "g1" {
		t.Fatalf("json text decode = %#v", metadata)
	}
	if got := decodeColumnValue("chunk", "metadata", ""); got != "" {
		t.Fatalf("non-json text must pass through, got %#v", got)
	}
	feas := decodeColumnValue("chunk", "tag_feas", `{"tag_a":0.5}`)
	if !reflect.DeepEqual(feas, map[string]interface{}{"tag_a": 0.5}) {
		t.Fatalf("*_feas json decode = %#v", feas)
	}
	if got := decodeColumnValue("chunk", "title_tks", 42); got != 42 {
		t.Fatalf("plain values must pass through, got %#v", got)
	}
}

func TestDecodeLogicalRowMemoryRenames(t *testing.T) {
	decoded := decodeLogicalRow(map[string]interface{}{
		"id": "m1_1", "message_type_kwd": "raw", "status_int": int64(1),
		"content_ltks": "hi", "q_2_vec": "[0.5,0.25]", "forget_at": "",
	}, "memory")
	if decoded["message_type"] != "raw" || decoded["content"] != "hi" {
		t.Fatalf("memory column renaming = %#v", decoded)
	}
	// Memory keyword-named columns are scalars: no ### split on read.
	scalar := decodeLogicalRow(map[string]interface{}{"source_id": "sess-1"}, "memory")
	if scalar["source_id"] != "sess-1" {
		t.Fatalf("memory source_id must stay scalar: %#v", scalar["source_id"])
	}
	if decoded["status"] != true {
		t.Fatalf("status_int must decode to bool, got %#v", decoded["status"])
	}
	if !reflect.DeepEqual(decoded["content_embed"], []float64{0.5, 0.25}) {
		t.Fatalf("vector column must decode to content_embed: %#v", decoded["content_embed"])
	}
	chunk := decodeLogicalRow(map[string]interface{}{"important_kwd": "a###b"}, "chunk")
	if !reflect.DeepEqual(chunk["important_kwd"], []string{"a", "b"}) {
		t.Fatalf("chunk keyword decode = %#v", chunk)
	}
}

// A memory table can carry several q_N_vec columns after an embedding-model
// change; backfillVectorColumns zero-fills the stale ones and GetChunk's
// SELECT * reads them all. Whichever way the map iterates, content_embed must
// be the real embedding, never the zero placeholder.
func TestDecodeLogicalRowMemorySkipsZeroVectorPlaceholders(t *testing.T) {
	row := map[string]interface{}{
		"id":      "m1_1",
		"q_2_vec": "[0.5,0.25]",
		"q_4_vec": "[0,0,0,0]",
	}
	for i := 0; i < 50; i++ { // exercise both map iteration orders
		decoded := decodeLogicalRow(row, "memory")
		if !reflect.DeepEqual(decoded["content_embed"], []float64{0.5, 0.25}) {
			t.Fatalf("content_embed = %#v", decoded["content_embed"])
		}
	}

	// All-zero row: a placeholder is retained when no real vector exists
	// (its dimension depends on iteration order; only zero-ness is stable).
	decoded := decodeLogicalRow(map[string]interface{}{
		"q_2_vec": "[0,0]", "q_4_vec": "[0,0,0,0]",
	}, "memory")
	placeholder, _ := decoded["content_embed"].([]float64)
	if !isZeroVector(placeholder) || len(placeholder) == 0 {
		t.Fatalf("all-zero content_embed = %#v", decoded["content_embed"])
	}
}

func TestRegroupPositionsHandlesRemainders(t *testing.T) {
	// The 5-tuple regroup is a lossy contract locked with the Python writer:
	// a partial trailing group is preserved as-is, never padded.
	got := regroupPositions([]int64{1, 2, 3, 4, 5, 6})
	want := [][]int64{{1, 2, 3, 4, 5}, {6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("regroup = %#v", got)
	}
	if got := regroupPositions(nil); len(got) != 0 {
		t.Fatalf("empty regroup = %#v", got)
	}
}

func TestParseFloatVectorText(t *testing.T) {
	if got := parseFloatVectorText("[0.1,0.2]"); !reflect.DeepEqual(got, []float64{0.1, 0.2}) {
		t.Fatalf("parse = %#v", got)
	}
	if got := parseFloatVectorText("[]"); got != nil {
		t.Fatalf("empty vector must decode nil, got %#v", got)
	}
	if got := parseFloatVectorText("not a vector"); got != nil {
		t.Fatalf("malformed vector must decode nil, got %#v", got)
	}
	if got := parseFloatVectorText("[ 0.5 , 1 ]"); !reflect.DeepEqual(got, []float64{0.5, 1}) {
		t.Fatalf("whitespace tolerance = %#v", got)
	}
}

func TestParsePgArray(t *testing.T) {
	if got := parsePgArray("{a,b,c}"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("simple = %#v", got)
	}
	if got := parsePgArray(`{a,"b,c",d}`); !reflect.DeepEqual(got, []string{"a", "b,c", "d"}) {
		t.Fatalf("quoted comma = %#v", got)
	}
	if got := parsePgArray(`{"say ""hi"""}`); !reflect.DeepEqual(got, []string{`say "hi"`}) {
		t.Fatalf("escaped quote = %#v", got)
	}
	if got := parsePgArray("{}"); len(got) != 0 {
		t.Fatalf("empty = %#v", got)
	}
	if got := parsePgArray("not-an-array"); got != nil {
		t.Fatalf("malformed = %#v", got)
	}
}

func TestParsePgIntArray(t *testing.T) {
	if got := parsePgIntArray("{1,2,3}"); !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("parse = %#v", got)
	}
	if got := parsePgIntArray("{1, x, 3}"); !reflect.DeepEqual(got, []int64{1, 3}) {
		t.Fatalf("non-numeric entries must be skipped: %#v", got)
	}
}

func TestDecodeLogicalRowToleratesNilColumns(t *testing.T) {
	// NULL columns are dropped by the queryRows scan layer before decoding
	// (the ES _source contract: never-written fields come back absent); any
	// nil that still reaches decodeLogicalRow must pass through untouched.
	decoded := decodeLogicalRow(map[string]interface{}{
		"id": "c1", "title_tks": nil, "content_ltks": "kept",
	}, "chunk")
	if decoded["title_tks"] != nil {
		t.Fatalf("nil must pass through: %#v", decoded)
	}
	if decoded["content_ltks"] != "kept" {
		t.Fatalf("kept column = %#v", decoded)
	}
}
