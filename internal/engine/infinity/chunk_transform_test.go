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
	"encoding/binary"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/utility"

	infinitysdk "github.com/infiniflow/infinity-go-sdk"
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

func TestDecodeJSONFields(t *testing.T) {
	row := map[string]interface{}{
		"source_doc_ids":      `["doc-1","doc-2"]`,
		"aliases":             "alpha###beta",
		"source_chunk_hashes": `{"chunk-1":"hash-1"}`,
	}

	decodeJSONFields(row)

	if want := []interface{}{"doc-1", "doc-2"}; !reflect.DeepEqual(row["source_doc_ids"], want) {
		t.Errorf("source_doc_ids = %#v, want %#v", row["source_doc_ids"], want)
	}
	if want := []interface{}{"alpha", "beta"}; !reflect.DeepEqual(row["aliases"], want) {
		t.Errorf("aliases = %#v, want %#v", row["aliases"], want)
	}
	if want := map[string]interface{}{"chunk-1": "hash-1"}; !reflect.DeepEqual(row["source_chunk_hashes"], want) {
		t.Errorf("source_chunk_hashes = %#v, want %#v", row["source_chunk_hashes"], want)
	}
}

// TestTransformChunkFieldsDropsTenantRoutingField ensures the tenant routing
// field is not sent to Infinity, whose per-tenant table schema does not have a
// tenant_id column.
func TestTransformChunkFieldsDropsTenantRoutingField(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"tenant_id": "tenant-1",
		"kb_id":     "kb-1",
	}, nil)

	if _, ok := got["tenant_id"]; ok {
		t.Fatal("tenant_id must not be sent to Infinity")
	}
	if got["kb_id"] != "kb-1" {
		t.Fatalf("kb_id = %#v, want %q", got["kb_id"], "kb-1")
	}
}

// compiledRowStringLists are the string-list values the compile path hands over:
// Python's _JSON_LIST_FIELDS columns plus the keyword column children_kwd.
var compiledRowStringLists = map[string]interface{}{
	"source_chunk_ids":         []string{"c1", "c2"},
	"source_doc_ids":           []string{"d1"},
	"compilation_template_ids": []string{"t1"},
	"doc_ids_kwd":              []string{"d1"},
	"entity_names_kwd":         []string{"alpha", "玄德"},
	"outlinks_kwd":             []string{"entity/beta"},
	"related_kb_pages_kwd":     []string{"entity/beta"},
	"rechunked_from_chunk_ids": []string{"c9"},
	"children_kwd":             []string{"r1", "r2"},
}

// compiledRowForTransform builds a post-indexdoc compiled chunk row.
func compiledRowForTransform() map[string]interface{} {
	row := map[string]interface{}{
		"id":                  "c1",
		"doc_id":              "d1",
		"kb_id":               "kb1",
		"content_with_weight": "compiled body",
		"compile_kwd":         "tree",
		"raptor_kwd":          "root",
		"raptor_layer_int":    2,
		"md_with_weight":      "# page",
		"extra":               map[string]interface{}{"raptor_method": "gmm"},
		"q_3_vec":             []float64{0.1, 0.2, 0.3},
	}
	for k, v := range compiledRowStringLists {
		row[k] = v
	}
	return row
}

// TestInsertValuesAreEncodableConstants is the regression test for the reported
// failure
//
//	insert chunk batch 0-32 after 3 attempts: failed to insert chunks to
//	dataset: InfinityException(3058, Unsupported slice element type: string)
//
// The SDK encodes every value as a constant and rejects a Go-native []string
// (expression_parser.go parseSliceConstantValue), so the raw values must NOT
// encode while everything transformChunkFields emits must. If a future SDK
// accepts string slices the repro is gone, so the test skips instead of failing.
func TestInsertValuesAreEncodableConstants(t *testing.T) {
	for k, v := range compiledRowStringLists {
		if _, err := infinitysdk.ParseConstantValue(v); err == nil {
			t.Skipf("SDK now supports string slices; the 3058 repro is gone (raw %s=%#v is encodable)", k, v)
		}
	}

	transformed := transformChunkFields(compiledRowForTransform(), nil)
	for k, v := range transformed {
		if _, err := infinitysdk.ParseConstantValue(v); err != nil {
			t.Errorf("transformed %s = %#v cannot be encoded for the insert: %v", k, v, err)
		}
	}
	// The json columns travel as JSON arrays (Python json.dumps) and the
	// keyword column as the ### join — strings either way, never a Go slice.
	for _, k := range []string{
		"source_chunk_ids", "source_doc_ids", "compilation_template_ids", "doc_ids_kwd",
		"entity_names_kwd", "outlinks_kwd", "related_kb_pages_kwd", "rechunked_from_chunk_ids",
	} {
		if s, ok := transformed[k].(string); !ok || !strings.HasPrefix(s, "[") {
			t.Errorf("transformed %s = %#v, want a JSON array string", k, transformed[k])
		}
	}
	if want := "r1###r2"; transformed["children_kwd"] != want {
		t.Errorf("children_kwd = %#v, want %q", transformed["children_kwd"], want)
	}
	if _, ok := transformed["extra"].(string); !ok {
		t.Errorf("extra = %#v, want a JSON string", transformed["extra"])
	}
}

// TestTransformedCompiledRowNamesOnlyMappingColumns covers the failure that
// follows 3058: Infinity rejects a column the table does not have, which a
// Go-only field such as tenant_id triggered.
func TestTransformedCompiledRowNamesOnlyMappingColumns(t *testing.T) {
	columns := chunkMappingColumns(t)
	vectorColumn := regexp.MustCompile(`^q_\d+_vec$`)
	for k := range transformChunkFields(compiledRowForTransform(), nil) {
		if columns[k] || vectorColumn.MatchString(k) {
			continue
		}
		t.Errorf("transformed key %q is not a column of conf/infinity_mapping.json", k)
	}
}

// TestFieldJSONMatchesJSONMappingColumns keeps the write side's encoding set in
// step with the schema: a column conf/infinity_mapping.json declares as json must
// be encoded by fieldJSON before the insert (Infinity answers 3058 for a raw Go
// slice), and a column fieldJSON encodes must be declared json (a JSON string in
// a typed column is just as wrong).
func TestFieldJSONMatchesJSONMappingColumns(t *testing.T) {
	jsonColumns := 0
	for column, columnType := range mappingColumnTypes(t) {
		isJSON := columnType == "json"
		if isJSON {
			jsonColumns++
		}
		if encoded := fieldJSON(column); encoded != isJSON {
			t.Errorf("fieldJSON(%q) = %v, but conf/infinity_mapping.json declares type %q", column, encoded, columnType)
		}
	}
	if jsonColumns == 0 {
		t.Fatal("conf/infinity_mapping.json parsed with no json column; the mapping read is broken")
	}
}

// chunkMappingColumns reads the column set the chunk table is created from.
func chunkMappingColumns(t *testing.T) map[string]bool {
	columns := mappingColumnTypes(t)
	out := make(map[string]bool, len(columns))
	for name := range columns {
		out[name] = true
	}
	return out
}

// mappingColumnTypes reads conf/infinity_mapping.json as column -> declared type.
func mappingColumnTypes(t *testing.T) map[string]string {
	t.Helper()
	path, err := utility.FindConfFileInProject("infinity_mapping.json")
	if err != nil || path == nil {
		t.Fatalf("locate infinity_mapping.json: %v", err)
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		t.Fatalf("read %s: %v", *path, err)
	}
	var columns map[string]struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &columns); err != nil {
		t.Fatalf("parse %s: %v", *path, err)
	}
	out := make(map[string]string, len(columns))
	for name, def := range columns {
		out[name] = def.Type
	}
	return out
}

// TestTransformChunkFieldsSerializesJSONListColumns is the regression for the
// reported 3058: the compile path hands over Go-native []string for the
// json-typed provenance columns, which the Go SDK cannot encode. Python
// json-encodes them (_JSON_LIST_FIELDS).
func TestTransformChunkFieldsSerializesJSONListColumns(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"source_chunk_ids":         []string{"c1", "c2"},
		"source_doc_ids":           []interface{}{"d1"},
		"compilation_template_ids": []string{"t1"},
		"entity_names_kwd":         []string{"刘备", "玄德"},
		"outlinks_kwd":             []interface{}{"concept/三国演义"},
		"related_kb_pages_kwd":     []string{"entity/person/刘备"},
	}, nil)

	want := map[string]string{
		"source_chunk_ids":         `["c1","c2"]`,
		"source_doc_ids":           `["d1"]`,
		"compilation_template_ids": `["t1"]`,
		"entity_names_kwd":         `["刘备","玄德"]`,
		"outlinks_kwd":             `["concept/三国演义"]`,
		"related_kb_pages_kwd":     `["entity/person/刘备"]`,
	}
	for field, expected := range want {
		if got[field] != expected {
			t.Errorf("%s = %#v, want %q", field, got[field], expected)
		}
	}
}

// TestTransformChunkFieldsJSONListColumnEdgeCases: empty list -> "[]", an
// already serialized value survives, nil -> "[]".
func TestTransformChunkFieldsJSONListColumnEdgeCases(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"source_chunk_ids": []string{},
		"source_doc_ids":   `["already"]`,
		"outlinks_kwd":     nil,
	}, nil)

	if want := "[]"; got["source_chunk_ids"] != want {
		t.Errorf("empty source_chunk_ids = %#v, want %q", got["source_chunk_ids"], want)
	}
	if want := `["already"]`; got["source_doc_ids"] != want {
		t.Errorf("serialized source_doc_ids = %#v, want %q", got["source_doc_ids"], want)
	}
	if want := "[]"; got["outlinks_kwd"] != want {
		t.Errorf("nil outlinks_kwd = %#v, want %q", got["outlinks_kwd"], want)
	}
}

// TestTransformChunkFieldsJoinsGenericKeywordSlices covers the keyword branch
// beyond important_kwd: the compile path emits children_kwd / entity_names as
// plain []string, which must be joined like the JSON-decoded []interface{} form.
func TestTransformChunkFieldsJoinsGenericKeywordSlices(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"children_kwd": []string{"c1", "c2"},
		"raptor_kwd":   "root",
	}, nil)

	if want := "c1###c2"; got["children_kwd"] != want {
		t.Errorf("children_kwd = %#v, want %q", got["children_kwd"], want)
	}
	if want := "root"; got["raptor_kwd"] != want {
		t.Errorf("raptor_kwd = %#v, want %q", got["raptor_kwd"], want)
	}
}

// TestTransformChunkFieldsSerializesMapColumns pins the dict-valued varchar
// column `extra` (infinity_conn.py:572-581).
func TestTransformChunkFieldsSerializesMapColumns(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"extra": map[string]interface{}{"raptor_method": "gmm"},
	}, nil)

	if want := `{"raptor_method":"gmm"}`; got["extra"] != want {
		t.Errorf("extra = %#v, want %q", got["extra"], want)
	}
}

// TestTransformChunkFieldsSerializesObjectJSONColumn pins the object-typed json
// column (source_chunk_hashes): a map is dumped and a nil value falls back to
// the mapping default "{}".
func TestTransformChunkFieldsSerializesObjectJSONColumn(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{
		"source_chunk_hashes": map[string]interface{}{"c1": "h1"},
	}, nil)
	if want := `{"c1":"h1"}`; got["source_chunk_hashes"] != want {
		t.Errorf("source_chunk_hashes = %#v, want %q", got["source_chunk_hashes"], want)
	}

	got = transformChunkFields(map[string]interface{}{"source_chunk_hashes": nil}, nil)
	if want := "{}"; got["source_chunk_hashes"] != want {
		t.Errorf("nil source_chunk_hashes = %#v, want %q", got["source_chunk_hashes"], want)
	}
}

// TestDecodeJSONFieldsNormalizesListColumns pins the parse_json_list cases that
// do not arrive as JSON strings (infinity_conn.py:938-944): a nil value is [],
// a JSON non-list value is wrapped, an object column keeps its shape.
func TestDecodeJSONFieldsNormalizesListColumns(t *testing.T) {
	row := map[string]interface{}{
		"related_kb_pages_kwd": nil,
		"outlinks_kwd":         float64(3.5),
		"source_doc_ids":       `[]`,
		"entity_names_kwd":     []interface{}{"e1"},
		"source_chunk_hashes":  nil,
		"content_with_weight":  `{"x":1}`,
		"docnm_kwd":            "plain",
	}

	decodeJSONFields(row)

	if want := []interface{}{}; !reflect.DeepEqual(row["related_kb_pages_kwd"], want) {
		t.Errorf("nil related_kb_pages_kwd = %#v, want %#v", row["related_kb_pages_kwd"], want)
	}
	if want := []interface{}{float64(3.5)}; !reflect.DeepEqual(row["outlinks_kwd"], want) {
		t.Errorf("scalar outlinks_kwd = %#v, want %#v", row["outlinks_kwd"], want)
	}
	if want := []interface{}{}; !reflect.DeepEqual(row["source_doc_ids"], want) {
		t.Errorf("empty source_doc_ids = %#v, want %#v", row["source_doc_ids"], want)
	}
	if want := []interface{}{"e1"}; !reflect.DeepEqual(row["entity_names_kwd"], want) {
		t.Errorf("decoded entity_names_kwd = %#v, want %#v", row["entity_names_kwd"], want)
	}
	if row["source_chunk_hashes"] != nil {
		t.Errorf("object column source_chunk_hashes = %#v, want it untouched", row["source_chunk_hashes"])
	}
	if row["content_with_weight"] != `{"x":1}` {
		t.Errorf("content_with_weight should be untouched, got %#v", row["content_with_weight"])
	}
	if row["docnm_kwd"] != "plain" {
		t.Errorf("docnm_kwd should be untouched, got %#v", row["docnm_kwd"])
	}
}

// TestApplyFieldMappingsDecodesJSONList confirms the search-path post-processor
// normalizes JSON-string columns into slices, so get chunk / search / get fields
// all hand readers the same shape.
func TestApplyFieldMappingsDecodesJSONList(t *testing.T) {
	chunks := []map[string]interface{}{{
		"source_doc_ids":   `["d1","d2"]`,
		"source_chunk_ids": `["c1"]`,
		"entity_names_kwd": `["e1","e2"]`,
		"outlinks_kwd":     `["o1"]`,
		"doc_ids_kwd":      `["x1"]`,
	}}

	applyFieldMappings(chunks)

	for _, key := range []string{"source_doc_ids", "source_chunk_ids", "entity_names_kwd", "outlinks_kwd", "doc_ids_kwd"} {
		if _, ok := chunks[0][key].([]interface{}); !ok {
			t.Errorf("%s not decoded into []interface{}, got %#v", key, chunks[0][key])
		}
	}
}

// TestJSONColumnsMatchCaseInsensitively pins that the write path
// (transformChunkFields) and the read path (decodeJSONFields) agree on a json
// column name regardless of its spelling.
func TestJSONColumnsMatchCaseInsensitively(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{"Source_Doc_IDs": []string{"d1"}}, nil)
	if want := `["d1"]`; got["Source_Doc_IDs"] != want {
		t.Errorf("Source_Doc_IDs = %#v, want %q", got["Source_Doc_IDs"], want)
	}

	row := map[string]interface{}{"Source_Doc_IDs": nil}
	decodeJSONFields(row)
	if want := []interface{}{}; !reflect.DeepEqual(row["Source_Doc_IDs"], want) {
		t.Errorf("nil Source_Doc_IDs = %#v, want %#v", row["Source_Doc_IDs"], want)
	}
}

// TestTransformChunkFieldsKeepsTitleColumn pins that title_kwd survives as its
// own column. The compiled wiki/nav rows carry their label only in title_kwd and
// nav keys every cluster update on it (title_kwd = <cluster name>); folding the
// field into docnm — Python's chunk-title alias — wrote ONLY docnm, so on
// Infinity every reader of title_kwd saw "" while ES returned the field verbatim
// (nav labels degraded to the description's first line and cluster merges never
// matched, so each document opened a new root cluster).
func TestTransformChunkFieldsKeepsTitleColumn(t *testing.T) {
	got := transformChunkFields(map[string]interface{}{"title_kwd": "忠臣进谏与托孤遗志 27577543"}, nil)
	if want := "忠臣进谏与托孤遗志 27577543"; got["title_kwd"] != want {
		t.Errorf("title_kwd = %#v, want %q", got["title_kwd"], want)
	}
	// The chunk-title alias is still written when the row carries no docnm_kwd.
	if want := "忠臣进谏与托孤遗志 27577543"; got["docnm"] != want {
		t.Errorf("docnm = %#v, want %q", got["docnm"], want)
	}

	// With docnm_kwd present, the alias must not overwrite the document name,
	// and title_kwd must still keep the row's own label.
	got = transformChunkFields(map[string]interface{}{
		"docnm_kwd": "出师表.txt",
		"title_kwd": "wiki page title",
	}, nil)
	if want := "出师表.txt"; got["docnm"] != want {
		t.Errorf("docnm = %#v, want %q", got["docnm"], want)
	}
	if want := "wiki page title"; got["title_kwd"] != want {
		t.Errorf("title_kwd = %#v, want %q", got["title_kwd"], want)
	}
}

// TestStripScorePseudoColumns pins that a caller's ES-style score columns are
// dropped: Infinity rejects "_score" unless a MATCH TEXT/TENSOR or Fusion is
// present (3013), and Search re-adds the one form its query shape accepts.
func TestStripScorePseudoColumns(t *testing.T) {
	got := stripScorePseudoColumns([]string{"type_kwd", "_score", "SCORE", "similarity()", "title_kwd"})
	want := []string{"type_kwd", "title_kwd"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stripScorePseudoColumns = %#v, want %#v", got, want)
	}

	// A filter-only query keeps no score column at all.
	if got := stripScorePseudoColumns([]string{"_score"}); len(got) != 0 {
		t.Fatalf("filter-only score columns = %#v, want empty", got)
	}
}

func TestHasPartialWildcard(t *testing.T) {
	cases := []struct {
		fields []string
		want   bool
	}{
		{[]string{"id", "title_kwd"}, false},
		{[]string{"*"}, false}, // Infinity reads a bare "*" as "all columns"
		{[]string{"id", "q_*_vec"}, true},
		{[]string{"q_*_vec", "*"}, true},
	}
	for _, c := range cases {
		if got := hasPartialWildcard(c.fields); got != c.want {
			t.Errorf("hasPartialWildcard(%#v) = %v, want %v", c.fields, got, c.want)
		}
	}
}

// TestExpandSelectWildcards pins the workaround for the partial wildcard the
// compile reader asks for: Infinity parses "q_*_vec" as the expression
// "q_" * "_vec" and fails to bind it (3013), so it is expanded into the table's
// real vector column(s) and dropped when the table has none.
func TestExpandSelectWildcards(t *testing.T) {
	cols := map[string]struct {
		Type    string
		Default interface{}
	}{
		"id":         {Type: "varchar"},
		"q_1024_vec": {Type: "vector,1024,float"},
		"content":    {Type: "varchar"},
	}

	if got, want := expandSelectWildcards([]string{"id", "q_*_vec", "title_kwd"}, cols),
		[]string{"id", "q_1024_vec", "title_kwd"}; !reflect.DeepEqual(got, want) {
		t.Errorf("expandSelectWildcards = %#v, want %#v", got, want)
	}

	// A bare "*" is left untouched: Infinity supports it.
	if got, want := expandSelectWildcards([]string{"*"}, cols), []string{"*"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bare wildcard = %#v, want %#v", got, want)
	}

	// A pattern matching nothing is dropped instead of being sent verbatim.
	if got, want := expandSelectWildcards([]string{"id", "missing_*_col"}, cols), []string{"id"}; !reflect.DeepEqual(got, want) {
		t.Errorf("unmatched wildcard = %#v, want %#v", got, want)
	}

	// Resolving to an already listed column must not duplicate it.
	if got, want := expandSelectWildcards([]string{"q_1024_vec", "q_*_vec"}, cols), []string{"q_1024_vec"}; !reflect.DeepEqual(got, want) {
		t.Errorf("duplicate wildcard = %#v, want %#v", got, want)
	}

	// Columns with no vector column at all: the wildcard disappears, the rest
	// survives (a query may legitimately target a table without embeddings).
	got := expandSelectWildcards([]string{"id", "q_*_vec"}, map[string]struct {
		Type    string
		Default interface{}
	}{"id": {Type: "varchar"}})
	if want := []string{"id"}; !reflect.DeepEqual(got, want) {
		t.Errorf("table without vector column = %#v, want %#v", got, want)
	}
}

func TestWildcardMatchColumn(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"q_*_vec", "q_1024_vec", true},
		{"q_*_vec", "q_768_vec", true},
		{"q_*_vec", "content", false},
		{"q_*_vec", "q_1024_vec_x", false},
		{"*_vec", "q_1024_vec", true},
		{"q_*", "q_1024_vec", true},
		{"foo", "foo", true},
		{"foo", "bar", false},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "ac", false},
	}
	for _, c := range cases {
		if got := wildcardMatchColumn(c.pattern, c.name); got != c.want {
			t.Errorf("wildcardMatchColumn(%q, %q) = %v, want %v", c.pattern, c.name, got, c.want)
		}
	}
}

// lengthPrefixedJSON builds Infinity's raw Json/Array column encoding: one
// `[int32 little-endian length][payload]` segment per stored value.
func lengthPrefixedJSON(parts ...string) []byte {
	out := make([]byte, 0, 64)
	for _, p := range parts {
		var l [4]byte
		binary.LittleEndian.PutUint32(l[:], uint32(len(p)))
		out = append(out, l[:]...)
		out = append(out, p...)
	}
	return out
}

// TestDecodeJSONFieldsLengthPrefixedBytes pins the read path for the Json/Array
// columns Infinity returns as length-prefixed bytes instead of JSON text — the
// shape the structure graph reads compilation_template_ids from.
func TestDecodeJSONFieldsLengthPrefixedBytes(t *testing.T) {
	row := map[string]interface{}{
		// One real value after three empty writes (the observed shape).
		"compilation_template_ids": lengthPrefixedJSON("[]", "[]", "[]", `["03b36295dcb34f9e811d8073b21e7f2f"]`),
		"source_chunk_ids":         lengthPrefixedJSON("[]", "[]", "[]", `["d1962d8d1cf69ab6","1313e71f7c7c9cca"]`),
		// Every write empty: the column reads back as an empty list.
		"doc_ids_kwd": lengthPrefixedJSON("[]", "[]", "[]", "[]", "[]", "[]"),
	}
	decodeJSONFields(row)

	if want := []interface{}{"03b36295dcb34f9e811d8073b21e7f2f"}; !reflect.DeepEqual(row["compilation_template_ids"], want) {
		t.Errorf("compilation_template_ids = %#v, want %#v", row["compilation_template_ids"], want)
	}
	if want := []interface{}{"d1962d8d1cf69ab6", "1313e71f7c7c9cca"}; !reflect.DeepEqual(row["source_chunk_ids"], want) {
		t.Errorf("source_chunk_ids = %#v, want %#v", row["source_chunk_ids"], want)
	}
	if want := []interface{}{}; !reflect.DeepEqual(row["doc_ids_kwd"], want) {
		t.Errorf("doc_ids_kwd = %#v, want %#v", row["doc_ids_kwd"], want)
	}

	// A value written twice must not fabricate a second identical bucket.
	dup := map[string]interface{}{
		"compilation_template_ids": lengthPrefixedJSON(`["t1"]`, `["t1","t2"]`),
	}
	decodeJSONFields(dup)
	if want := []interface{}{"t1", "t2"}; !reflect.DeepEqual(dup["compilation_template_ids"], want) {
		t.Errorf("duplicate segments = %#v, want %#v", dup["compilation_template_ids"], want)
	}

	// A non-array segment (an object) on a list column keeps the list shape
	// (Python's parse_json_list wraps a scalar), and on a plain JSON column it
	// decodes to the object itself.
	listObj := map[string]interface{}{
		"compilation_template_ids": lengthPrefixedJSON(`{"kind":"tree"}`),
	}
	decodeJSONFields(listObj)
	wrapped, ok := listObj["compilation_template_ids"].([]interface{})
	if !ok || len(wrapped) != 1 {
		t.Fatalf("object segment on a list column = %#v, want a one-element list", listObj["compilation_template_ids"])
	}
	if m, ok := wrapped[0].(map[string]interface{}); !ok || m["kind"] != "tree" {
		t.Errorf("object segment element = %#v, want the decoded map", wrapped[0])
	}

	plainObj := map[string]interface{}{
		"source_chunk_hashes": lengthPrefixedJSON(`{"a":"b"}`),
	}
	decodeJSONFields(plainObj)
	if m, ok := plainObj["source_chunk_hashes"].(map[string]interface{}); !ok || m["a"] != "b" {
		t.Errorf("object segment on a JSON column = %#v, want the decoded map", plainObj["source_chunk_hashes"])
	}
}

func TestDecodeInfinityJSONBytesRejectsOtherShapes(t *testing.T) {
	if _, ok := decodeInfinityJSONBytes([]byte("not-length-prefixed")); ok {
		t.Error("plain text bytes must not be accepted as a Json column blob")
	}
	if _, ok := decodeInfinityJSONBytes([]byte{1, 2}); ok {
		t.Error("too-short bytes must not be accepted")
	}
	// A truncated final segment decodes the complete prefix only.
	blob := lengthPrefixedJSON(`["a"]`)
	blob = append(blob, 0, 0, 0, 9, 'x')
	value, ok := decodeInfinityJSONBytes(blob)
	if !ok {
		t.Fatal("complete leading segment must still decode")
	}
	if want := []interface{}{"a"}; !reflect.DeepEqual(value, want) {
		t.Errorf("truncated blob = %#v, want %#v", value, want)
	}
}

// TestRealignJSONColumnCells pins the redistribution of a Json/Array column that
// Infinity returns as one cell holding one segment per row: without it only row 0
// keeps the column, so the structure graph sees one entity row per read.
func TestRealignJSONColumnCells(t *testing.T) {
	columns := map[string][]interface{}{
		"id":                       {"a", "b", "c"},
		"knowledge_graph_kwd":      {"entity", "entity", "relation"},
		"compilation_template_ids": {lengthPrefixedJSON(`["t1"]`, `[]`, `["t2"]`)},
	}
	rows := []map[string]interface{}{{}, {}, {}}
	realignJSONColumnCells(rows, columns)

	wantText := []string{`["t1"]`, `[]`, `["t2"]`}
	for i, want := range wantText {
		if got := rows[i]["compilation_template_ids"]; got != want {
			t.Errorf("row %d segment = %#v, want %q", i, got, want)
		}
	}
	// Decoding each row's redistributed text yields that row's own value.
	decodeJSONFields(rows[0])
	decodeJSONFields(rows[1])
	decodeJSONFields(rows[2])
	if want := []interface{}{"t1"}; !reflect.DeepEqual(rows[0]["compilation_template_ids"], want) {
		t.Errorf("row 0 = %#v, want %#v", rows[0]["compilation_template_ids"], want)
	}
	if want := []interface{}{}; !reflect.DeepEqual(rows[1]["compilation_template_ids"], want) {
		t.Errorf("row 1 = %#v, want %#v", rows[1]["compilation_template_ids"], want)
	}
	if want := []interface{}{"t2"}; !reflect.DeepEqual(rows[2]["compilation_template_ids"], want) {
		t.Errorf("row 2 = %#v, want %#v", rows[2]["compilation_template_ids"], want)
	}

	// A blob whose segment count does not match the row count is left alone, and
	// a column that already has one cell per row is untouched.
	odd := []map[string]interface{}{{}, {}}
	realignJSONColumnCells(odd, map[string][]interface{}{
		"compilation_template_ids": {lengthPrefixedJSON(`["t1"]`, `[]`, `["t2"]`)},
	})
	if len(odd[0]) != 0 {
		t.Errorf("mismatched segment count must be left untouched, got %#v", odd[0])
	}
	aligned := []map[string]interface{}{{}, {}}
	realignJSONColumnCells(aligned, map[string][]interface{}{
		"compilation_template_ids": {`["t1"]`, `["t2"]`},
	})
	if len(aligned[0]) != 0 {
		t.Errorf("an aligned column must be left untouched, got %#v", aligned[0])
	}
	// A single row keeps the cell as it came: decodeInfinityJSONBytes handles it.
	single := []map[string]interface{}{{}}
	realignJSONColumnCells(single, map[string][]interface{}{
		"compilation_template_ids": {lengthPrefixedJSON(`["t1"]`)},
	})
	if len(single[0]) != 0 {
		t.Errorf("a single-row result must be left untouched, got %#v", single[0])
	}
	// Segments that are not JSON are not per-row values: leave the column alone
	// rather than copying the framing into every row.
	notJSON := []map[string]interface{}{{}, {}}
	realignJSONColumnCells(notJSON, map[string][]interface{}{
		"compilation_template_ids": {lengthPrefixedJSON("t1", "t2")},
	})
	if len(notJSON[0]) != 0 {
		t.Errorf("non-JSON segments must be left untouched, got %#v", notJSON[0])
	}
}
