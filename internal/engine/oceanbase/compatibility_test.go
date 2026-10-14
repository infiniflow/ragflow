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
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
)

func TestLegacyChunkEncodingContract(t *testing.T) {
	document := map[string]interface{}{
		"id": "chunk-1", "kb_id": []string{"kb-1"}, "doc_id": "doc-1",
		"available_int": 0, "removed_kwd": nil, "chunk_order_int": 7, "mom_id": "parent-1",
		"metadata":      map[string]interface{}{"_group_id": "group-1", "_title": "renamed"},
		"important_kwd": []string{" alpha\t", "beta\n"},
		"q_3_vec":       []float64{0.1, 0.2, 0.3},
		"custom_field":  "preserved",
	}
	got, err := normalizeChunk(document)
	if err != nil {
		t.Fatal(err)
	}
	if got["kb_id"] != "kb-1" || got["available_int"] != 0 || got["removed_kwd"] != "N" || got["_order_id"] != 7 || got["mom_id"] != "parent-1" {
		t.Fatalf("legacy scalar/default encoding changed: %#v", got)
	}
	if got["group_id"] != "group-1" || got["docnm_kwd"] != "renamed" {
		t.Fatalf("metadata denormalization changed: %#v", got)
	}
	if got["q_3_vec"] != "[0.1,0.2,0.3]" {
		t.Fatalf("vector encoding = %q", got["q_3_vec"])
	}
	var extra map[string]interface{}
	if err := json.Unmarshal([]byte(got["extra"].(string)), &extra); err != nil {
		t.Fatal(err)
	}
	if extra["custom_field"] != "preserved" {
		t.Fatalf("unknown field was not preserved in extra: %#v", extra)
	}
}

func TestLegacyMemoryAliasesAndVectorEncoding(t *testing.T) {
	got, err := normalizeMemory(map[string]interface{}{
		"id": "memory-1_1", "message_id": "1", "memory_id": "memory-1",
		"message_type": "raw", "status": false, "content": "hello world",
		"content_embed": []float64{0.25, 0.5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["message_type_kwd"] != "raw" || got["status_int"] != 0 || got["content_ltks"] != "hello world" {
		t.Fatalf("memory aliases changed: %#v", got)
	}
	if got["q_2_vec"] != "[0.25,0.5]" {
		t.Fatalf("memory vector encoding = %q", got["q_2_vec"])
	}
	decoded := decodeLogicalRow(map[string]interface{}{
		"message_type_kwd": "raw", "status_int": int64(0), "content_ltks": "hello",
		"q_2_vec": "[0.25,0.5]",
	}, "memory")
	if decoded["message_type"] != "raw" || decoded["status"] != false || !reflect.DeepEqual(decoded["content_embed"], []interface{}{0.25, 0.5}) {
		t.Fatalf("memory read aliases changed: %#v", decoded)
	}
}

func TestSkillRowsKeepStringStatusAndSkillRowID(t *testing.T) {
	decoded := decodeLogicalRow(map[string]interface{}{
		"skill_id": "skill-1",
		"status":   "draft",
	}, "skill")
	if decoded["status"] != "draft" {
		t.Fatalf("skill status = %#v, want draft", decoded["status"])
	}

	expression, alias, err := selectExpression("row_id()", "skill")
	if err != nil {
		t.Fatal(err)
	}
	if expression != "`skill_id` AS `row_id`" || alias != "row_id" {
		t.Fatalf("skill row ID projection = (%q, %q)", expression, alias)
	}
}

func TestDBMSHybridBodyMatchesPythonSemantics(t *testing.T) {
	plan := searchPlan{
		text:   &types.MatchTextExpr{MatchingText: "hello", TopN: 10, ExtraOptions: map[string]interface{}{"minimum_should_match": 0.3}},
		dense:  &types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.1, 0.2}, EmbeddingDataType: "float", TopN: 8, ExtraOptions: map[string]interface{}{"similarity": 0.42}},
		fusion: &types.FusionExpr{Method: "weighted_sum", FusionParams: map[string]interface{}{"weights": "0.25,0.75"}},
	}
	body, ok := buildDBMSBody("chunk", map[string]interface{}{"kb_id": []string{"kb-1"}, "available_int": 0}, &types.SearchRequest{
		Offset: 2, Limit: 5, RankFeature: map[string]float64{"pagerank_fea": 0.1},
	}, plan)
	if !ok {
		t.Fatal("hybrid body unexpectedly required SQL fallback")
	}
	root, ok := body["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("hybrid body query leg = %#v", body["query"])
	}
	query, ok := root["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("hybrid body bool leg = %#v", root)
	}
	mustClauses, ok := query["must"].([]interface{})
	if !ok || len(mustClauses) == 0 {
		t.Fatalf("hybrid body must leg = %#v", query["must"])
	}
	firstClause, ok := mustClauses[0].(map[string]interface{})
	if !ok {
		t.Fatalf("hybrid body must clause = %#v", mustClauses[0])
	}
	must, ok := firstClause["query_string"].(map[string]interface{})
	if !ok {
		t.Fatalf("hybrid body query_string leg = %#v", firstClause)
	}
	if must["minimum_should_match"] != "30%" || query["boost"] != 0.25 {
		t.Fatalf("hybrid text leg = %#v", query)
	}
	knn, ok := body["knn"].(map[string]interface{})
	if !ok {
		t.Fatalf("hybrid knn leg = %#v", body["knn"])
	}
	if knn["k"] != 8 || knn["num_candidates"] != 16 || knn["similarity"] != 0.42 {
		t.Fatalf("hybrid vector leg = %#v", knn)
	}
}

func TestDBMSHybridBodyMinimumShouldMatchHalfUp(t *testing.T) {
	plan := searchPlan{
		text:   &types.MatchTextExpr{MatchingText: "hello", TopN: 10, ExtraOptions: map[string]interface{}{"minimum_should_match": 0.285}},
		dense:  &types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.1, 0.2}, EmbeddingDataType: "float", TopN: 8, ExtraOptions: map[string]interface{}{"similarity": 0.42}},
		fusion: &types.FusionExpr{Method: "weighted_sum", FusionParams: map[string]interface{}{"weights": "0.25,0.75"}},
	}
	body, ok := buildDBMSBody("chunk", map[string]interface{}{"kb_id": []string{"kb-1"}, "available_int": 0}, &types.SearchRequest{
		Offset: 2, Limit: 5, RankFeature: map[string]float64{"pagerank_fea": 0.1},
	}, plan)
	if !ok {
		t.Fatal("hybrid body unexpectedly required SQL fallback")
	}
	root := body["query"].(map[string]interface{})
	query := root["bool"].(map[string]interface{})
	must := query["must"].([]interface{})[0].(map[string]interface{})["query_string"].(map[string]interface{})
	if got := must["minimum_should_match"]; got != "29%" {
		t.Fatalf("minimum_should_match for 0.285 = %q, want 29%%", got)
	}
	knn, ok := body["knn"].(map[string]interface{})
	if !ok {
		t.Fatalf("hybrid knn leg = %#v", body["knn"])
	}
	if knn["k"] != 8 || knn["num_candidates"] != 16 || knn["similarity"] != 0.42 {
		t.Fatalf("hybrid vector leg = %#v", knn)
	}
}

func TestMetadataJSONPushdownOperators(t *testing.T) {
	predicate, args, err := buildMetaPushdownPredicate([]map[string]interface{}{
		{"key": "author", "op": "contains", "value": "Alice"},
		{"key": "year", "op": "≥", "value": "2024"},
		{"key": "tags", "op": "in", "value": "rag, database"},
	}, "and")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"JSON_UNQUOTE", "DECIMAL(65,20)", "JSON_CONTAINS", " AND "} {
		if !strings.Contains(predicate, fragment) {
			t.Errorf("metadata predicate %q is missing %q", predicate, fragment)
		}
	}
	wantArgs := []interface{}{"$.author", "Alice", "$.year", int64(2024), "$.tags", `"rag"`, "$.tags", `"database"`}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("metadata args = %#v, want %#v", args, wantArgs)
	}
	if _, _, err := buildMetaPushdownPredicate([]map[string]interface{}{{"key": "bad-key", "op": "=", "value": "x"}}, "and"); err == nil {
		t.Fatal("invalid JSON metadata key must reject push-down")
	}
}

func TestMetadataJSONPushdownRejectsUnsafeNegativeOperators(t *testing.T) {
	for _, operator := range []string{"≠", "not in"} {
		_, _, err := buildMetaPushdownPredicate([]map[string]interface{}{
			{"key": "tags", "op": operator, "value": []string{"a"}},
		}, "and")
		if err == nil {
			t.Errorf("operator %q must reject metadata push-down", operator)
		}
	}

	for _, operator := range []string{"=", "in"} {
		if _, _, err := buildMetaPushdownPredicate([]map[string]interface{}{
			{"key": "tags", "op": operator, "value": []string{"a"}},
		}, "and"); err != nil {
			t.Errorf("operator %q unexpectedly rejected metadata push-down: %v", operator, err)
		}
	}
}

func TestHybridFallbackOnlyForUnavailablePackage(t *testing.T) {
	if !isHybridUnavailableError(assertError("ERROR 1305: FUNCTION DBMS_HYBRID_SEARCH.SEARCH does not exist")) {
		t.Fatal("missing DBMS package must trigger SQL fallback")
	}
	if isHybridUnavailableError(assertError("DBMS_HYBRID_SEARCH.SEARCH syntax error in query_string")) {
		t.Fatal("query errors must be returned instead of silently falling back")
	}
	if isHybridUnavailableError(assertError("feature not supported")) {
		t.Fatal("unrelated unsupported errors must not trigger SQL fallback")
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("OceanBase_CE 4.3.5.1", "4.3.5.1") != 0 || compareVersions("4.4.1.0", "4.3.5.1") <= 0 || compareVersions("4.3.4.0", "4.3.5.1") >= 0 {
		t.Fatal("OceanBase version comparison changed")
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func columnNames(columns []columnDefinition) []string {
	result := make([]string, len(columns))
	for i, column := range columns {
		result[i] = column.name
	}
	return result
}

func columnType(columns []columnDefinition, name string) string {
	for _, column := range columns {
		if column.name == name {
			return column.typeSQL
		}
	}
	return ""
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate compatibility test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	path := filepath.Join(append([]string{repoRoot}, parts...)...)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
