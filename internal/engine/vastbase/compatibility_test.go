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

	"github.com/lib/pq"
)

// These tests lock the byte-level storage contract the Python psycopg2
// connector established on the vb-dev-1 branch. Tables written by either
// implementation must be readable by the other, so the wire formats below are
// frozen: ### joined keyword text, floatvector text rendered at float32
// precision, position_int stored as a flat integer[] regrouped in fives, and
// JSON object columns stored as JSON text.

func TestKeywordRoundTripMatchesPythonFormat(t *testing.T) {
	encoded, err := encodeColumnValue("important_kwd", []interface{}{"政策", "检索", "ragflow"})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "政策###检索###ragflow" {
		t.Fatalf("python ### join = %#v", encoded)
	}
	decoded := decodeColumnValue("chunk", "important_kwd", encoded)
	if !reflect.DeepEqual(decoded, []string{"政策", "检索", "ragflow"}) {
		t.Fatalf("python ### split = %#v", decoded)
	}
	// A single-element list round-trips without separators.
	encoded, _ = encodeColumnValue("source_id", []interface{}{"doc-1"})
	decoded = decodeColumnValue("chunk", "source_id", encoded)
	if !reflect.DeepEqual(decoded, []string{"doc-1"}) {
		t.Fatalf("single element = %#v -> %#v", encoded, decoded)
	}
}

func TestVectorRoundTripMatchesPsycopg2Float32(t *testing.T) {
	// float32 precision both ways: encode narrows, decode widens back.
	encoded, err := encodeVector([]interface{}{0.123456789, 2.5, -0.75})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "[0.12345679,2.5,-0.75]" {
		t.Fatalf("float32 text = %q", encoded)
	}
	decoded := decodeColumnValue("chunk", "q_3_vec", encoded)
	numbers, ok := decoded.([]float64)
	if !ok || len(numbers) != 3 {
		t.Fatalf("float32 round trip = %#v", decoded)
	}
	// The text form is exact at float32 precision: narrowing the decoded
	// float64 back to float32 must reproduce the original components.
	for i, original := range []float32{float32(0.123456789), 2.5, -0.75} {
		if float32(numbers[i]) != original {
			t.Fatalf("component %d: %v != %v", i, float32(numbers[i]), original)
		}
	}
}

func TestPositionIntRoundTripRegroupsByFive(t *testing.T) {
	original := []interface{}{
		[]interface{}{int64(0), 12, 34, 56, 78},
		[]interface{}{int64(0), 98, 76, 54, 32},
	}
	stored, err := encodeColumnValue("position_int", original)
	if err != nil {
		t.Fatal(err)
	}
	array, ok := stored.(pq.Int64Array)
	if !ok || len(array) != 10 {
		t.Fatalf("flat storage = %#v", stored)
	}
	// lib/pq renders int64[] as {0,12,34,...} when the server returns it as
	// text into a dynamic scan; decode from that literal.
	decoded := decodeColumnValue("chunk", "position_int", "{0,12,34,56,78,0,98,76,54,32}")
	want := [][]int64{{0, 12, 34, 56, 78}, {0, 98, 76, 54, 32}}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("regroup = %#v", decoded)
	}
}

func TestIntegerArrayRoundTrip(t *testing.T) {
	stored, err := encodeColumnValue("page_num_int", []interface{}{int64(3), int64(1), int64(4)})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.(pq.Int64Array); !ok {
		t.Fatalf("int[] binding = %#v", stored)
	}
	decoded := decodeColumnValue("chunk", "page_num_int", "{3,1,4}")
	if !reflect.DeepEqual(decoded, []int64{3, 1, 4}) {
		t.Fatalf("int[] decode = %#v", decoded)
	}
}

func TestJSONColumnRoundTrip(t *testing.T) {
	object := map[string]interface{}{"_group_id": "g-7", "nested": map[string]interface{}{"k": 1.5}}
	encoded, err := encodeColumnValue("metadata", object)
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeColumnValue("chunk", "metadata", encoded.(string))
	decodedObject, ok := decoded.(map[string]interface{})
	if !ok || decodedObject["_group_id"] != "g-7" {
		t.Fatalf("json round trip = %#v", decoded)
	}
	// Unwritten metadata rows hold DEFAULT '' and must pass through so the
	// exists-filter semantics stay intact.
	if got := decodeColumnValue("chunk", "metadata", ""); got != "" {
		t.Fatalf("DEFAULT '' passthrough = %#v", got)
	}
}

func TestZeroBackfillVectorFormat(t *testing.T) {
	// Documents missing a vector column the table already has are backfilled
	// with zeros; the text form must match encodeVector's rendering.
	encoded, err := encodeVector([]interface{}{0.0, 0.0, 0.0, 0.0})
	if err != nil {
		t.Fatal(err)
	}
	if zeroVector(4) != encoded {
		t.Fatalf("zeroVector(4) = %q, encodeVector = %q", zeroVector(4), encoded)
	}
	if got := decodeColumnValue("chunk", "q_4_vec", zeroVector(4)); !reflect.DeepEqual(got, []float64{0, 0, 0, 0}) {
		t.Fatalf("zero vector decode = %#v", got)
	}
}

func TestKbIDListCollapsesToFirst(t *testing.T) {
	// The Python writer stored the first kb of the list; shared tables rely
	// on that single value for row-level scoping.
	encoded, err := encodeColumnValue("kb_id", []interface{}{"kb-1", "kb-2"})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "kb-1" {
		t.Fatalf("kb_id = %#v", encoded)
	}
	if encoded, _ := encodeColumnValue("kb_id", []interface{}{}); encoded != nil {
		t.Fatalf("empty kb list = %#v", encoded)
	}
}

func TestTextColumnsCarryEmptyStringDefault(t *testing.T) {
	// The mapping's ES-parity contract: unwritten text is '' (not NULL), so
	// exists filters can treat '' as absent and must_not as present.
	for _, column := range chunkColumns {
		if column.name == "position_int" || column.name == "page_num_int" || column.name == "top_int" {
			if column.defaultSQL != "" {
				t.Fatalf("array column %s must have no DEFAULT, got %q", column.name, column.defaultSQL)
			}
			continue
		}
		if column.defaultSQL == "" {
			t.Fatalf("column %s lost its DEFAULT clause", column.name)
		}
	}
	for _, column := range memoryColumns {
		if column.defaultSQL == "" {
			t.Fatalf("memory column %s lost its DEFAULT clause", column.name)
		}
	}
	if metadataColumns[len(metadataColumns)-1].defaultSQL != "'{}'" {
		t.Fatalf("meta_fields must DEFAULT '{}', got %q", metadataColumns[len(metadataColumns)-1].defaultSQL)
	}
}
