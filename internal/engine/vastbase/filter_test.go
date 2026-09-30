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
	"strings"
	"testing"
)

func testColumns() map[string]columnMeta {
	columns := make(map[string]columnMeta)
	for _, column := range chunkColumns {
		dataType := column.typeSQL
		columns[column.name] = columnMeta{name: column.name, dataType: dataType}
	}
	return columns
}

func TestBuildFilterBasicsAndPlaceholderOrdering(t *testing.T) {
	columns := testColumns()
	sqlText, args, err := buildFilter(map[string]interface{}{
		"doc_id":        "doc-1",
		"kb_id":         "kb-9",
		"available_int": 0,
	}, "chunk", columns)
	if err != nil {
		t.Fatal(err)
	}
	// available_int=0 is falsy in the Python semantics and must drop.
	if sqlText != `"doc_id" = $1 AND "kb_id" = $2` {
		t.Fatalf("filter = %q", sqlText)
	}
	if !reflect.DeepEqual(args, []interface{}{"doc-1", "kb-9"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildFilterListBecomesIN(t *testing.T) {
	sqlText, args, err := buildFilter(map[string]interface{}{
		"doc_id": []interface{}{"d1", "d2", "d3"},
	}, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `"doc_id" IN ($1, $2, $3)` {
		t.Fatalf("filter = %q", sqlText)
	}
	if !reflect.DeepEqual(args, []interface{}{"d1", "d2", "d3"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildFilterUnknownColumnMatchesNothing(t *testing.T) {
	// ES dynamic mapping parity: a field the table never stored exists on no
	// row, so the condition collapses to 1=0 instead of erroring.
	sqlText, args, err := buildFilter(map[string]interface{}{"never_stored": "x"}, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != "1=0" || len(args) != 0 {
		t.Fatalf("filter = %q args = %#v", sqlText, args)
	}
	// A nil columns map means "unknown schema" (callers that cannot probe);
	// every column is then allowed through.
	sqlText, _, err = buildFilter(map[string]interface{}{"never_stored": "x"}, "chunk", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `"never_stored" = $1` {
		t.Fatalf("permissive filter = %q", sqlText)
	}
}

func TestBuildFilterKeywordConditionsAreDropped(t *testing.T) {
	// Locked Python gap: keyword-column conditions (source_id, *_kwd except
	// docnm_kwd/knowledge_graph_kwd) are silently dropped, not translated.
	sqlText, args, err := buildFilter(map[string]interface{}{
		"source_id":     "doc-1",
		"important_kwd": "alpha",
		"docnm_kwd":     "title",
	}, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `"docnm_kwd" = $1` {
		t.Fatalf("filter = %q", sqlText)
	}
	if !reflect.DeepEqual(args, []interface{}{"title"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildFilterExistsParity(t *testing.T) {
	columns := testColumns()
	sqlText, _, err := buildFilter(map[string]interface{}{
		"exists": "title_tks",
	}, "chunk", columns)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `("title_tks" IS NOT NULL AND "title_tks" <> '')` {
		t.Fatalf("text exists = %q", sqlText)
	}
	sqlText, _, err = buildFilter(map[string]interface{}{
		"exists": "pagerank_fea",
	}, "chunk", columns)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `"pagerank_fea" IS NOT NULL` {
		t.Fatalf("scalar exists = %q", sqlText)
	}
	// exists on a column the table lacks matches nothing.
	sqlText, _, _ = buildFilter(map[string]interface{}{"exists": "ghost_field"}, "chunk", columns)
	if sqlText != "1=0" {
		t.Fatalf("unknown exists = %q", sqlText)
	}
}

func TestBuildFilterMustNotExistsMirror(t *testing.T) {
	columns := testColumns()
	sqlText, _, err := buildFilter(map[string]interface{}{
		"must_not": map[string]interface{}{"exists": "forget_at"},
	}, "chunk", columns)
	if err != nil {
		t.Fatal(err)
	}
	// forget_at is absent from chunk tables: the clause holds on every row
	// and is dropped entirely.
	if sqlText != "1=1" {
		t.Fatalf("unknown must_not = %q", sqlText)
	}
	memory := make(map[string]columnMeta)
	for _, column := range memoryColumns {
		memory[column.name] = columnMeta{name: column.name, dataType: column.typeSQL}
	}
	sqlText, _, err = buildFilter(map[string]interface{}{
		"must_not": map[string]interface{}{"exists": "forget_at"},
	}, "memory", memory)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `("forget_at" IS NULL OR "forget_at" = '')` {
		t.Fatalf("text must_not = %q", sqlText)
	}
	sqlText, _, err = buildFilter(map[string]interface{}{
		"must_not": map[string]interface{}{"exists": "status_int"},
	}, "memory", memory)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `"status_int" IS NULL` {
		t.Fatalf("scalar must_not = %q", sqlText)
	}
}

func TestBuildFilterMemoryFieldMapping(t *testing.T) {
	memory := make(map[string]columnMeta)
	for _, column := range memoryColumns {
		memory[column.name] = columnMeta{name: column.name, dataType: column.typeSQL}
	}
	sqlText, args, err := buildFilter(map[string]interface{}{
		"message_type": "raw",
		"id":           "m1_1",
	}, "memory", memory)
	if err != nil {
		t.Fatal(err)
	}
	// message_type maps to message_type_kwd, a keyword column: the condition
	// is dropped exactly like the Python connector drops it.
	if sqlText != `"id" = $1` {
		t.Fatalf("memory filter = %q", sqlText)
	}
	if !reflect.DeepEqual(args, []interface{}{"m1_1"}) {
		t.Fatalf("args = %#v", args)
	}
}

func TestBuildFilterIDKeyRejected(t *testing.T) {
	if _, _, err := buildFilter(map[string]interface{}{"_id": "x"}, "chunk", testColumns()); err == nil {
		t.Fatal("_id must be rejected in favor of id")
	}
}

func TestBuildFilterEmptyAndFalsy(t *testing.T) {
	sqlText, args, err := buildFilter(map[string]interface{}{}, "chunk", testColumns())
	if err != nil || sqlText != "1=1" || len(args) != 0 {
		t.Fatalf("empty filter = %q %#v err %v", sqlText, args, err)
	}
	sqlText, _, err = buildFilter(map[string]interface{}{"doc_id": "", "removed_kwd": nil, "tag_kwd": []interface{}{}}, "chunk", testColumns())
	if err != nil || sqlText != "1=1" {
		t.Fatalf("falsy filter = %q err %v", sqlText, err)
	}
}

func TestShiftPlaceholders(t *testing.T) {
	if got := shiftPlaceholders(`"a" = $1 AND "b" IN ($2, $3)`, 3); got != `"a" = $4 AND "b" IN ($5, $6)` {
		t.Fatalf("shift = %q", got)
	}
	if got := shiftPlaceholders("$1", 0); got != "$1" {
		t.Fatalf("zero offset must be a no-op, got %q", got)
	}
}

func TestBuildFilterMetadataConditions(t *testing.T) {
	sqlText, args, err := buildFilter(map[string]interface{}{
		"metadata_filtering_conditions": map[string]interface{}{
			"logical_operator": "and",
			"conditions": []interface{}{
				map[string]interface{}{"name": "year", "comparison_operator": ">", "value": 2023},
				map[string]interface{}{"name": "author", "comparison_operator": "is", "value": "Ada"},
				map[string]interface{}{"name": "title", "comparison_operator": "start with", "value": "RAG"},
			},
		},
	}, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	want := `(CASE WHEN "metadata" LIKE '{%' THEN "metadata"::jsonb ELSE '{}'::jsonb END) ->> $1`
	if !strings.Contains(sqlText, want) {
		t.Fatalf("json extraction missing: %q", sqlText)
	}
	// Guarded numeric cast keeps non-numeric rows as NULL instead of failing.
	if !strings.Contains(sqlText, `THEN ((CASE WHEN "metadata" LIKE '{%' THEN "metadata"::jsonb ELSE '{}'::jsonb END) ->> $1)::numeric ELSE NULL END`) {
		t.Fatalf("guarded cast missing: %q", sqlText)
	}
	if !strings.Contains(sqlText, `LIKE $6`) || !strings.Contains(sqlText, `> $2`) || !strings.Contains(sqlText, `= $4`) {
		t.Fatalf("operators/args misplaced: %q", sqlText)
	}
	if len(args) != 6 {
		t.Fatalf("args = %#v", args)
	}
	if args[0] != "$.year" || args[2] != "$.author" {
		t.Fatalf("json paths = %#v", args)
	}
}

func TestBuildFilterMetadataContainsUsesJSONBContainment(t *testing.T) {
	sqlText, args, err := buildFilter(map[string]interface{}{
		"metadata_filtering_conditions": map[string]interface{}{
			"logical_operator": "or",
			"conditions": []interface{}{
				map[string]interface{}{"name": "tags", "comparison_operator": "contains", "value": "hot"},
				map[string]interface{}{"name": "tags", "comparison_operator": "not contains", "value": "cold"},
			},
		},
	}, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, `-> $1 @> $2::jsonb`) || !strings.Contains(sqlText, `NOT (CASE WHEN`) || !strings.Contains(sqlText, ` OR `) {
		t.Fatalf("contains filter = %q", sqlText)
	}
	if args[1] != `"hot"` {
		t.Fatalf("contains value must be JSON-encoded for @>, got %#v", args[1])
	}
}

func TestBuildFilterMetadataEmptyChecks(t *testing.T) {
	sqlText, _, err := buildFilter(map[string]interface{}{
		"metadata_filtering_conditions": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"name": "note", "comparison_operator": "empty"},
			},
		},
	}, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, `IS NULL OR`) || !strings.Contains(sqlText, `= '{}'`) {
		t.Fatalf("empty filter = %q", sqlText)
	}
}
