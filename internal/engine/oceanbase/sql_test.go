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
	"regexp"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"github.com/DATA-DOG/go-sqlmock"
)

func init() {
	common.InitLogger("info", common.FileOutput{}, "oceanbase_test")
}

func TestUpdateMetadataReplacesCompleteLegacyJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("oceanbase", "legacy_doc", db)

	query := "REPLACE INTO `ragflow_doc_meta_tenant-1` (id, kb_id, meta_fields) VALUES (?, ?, ?)"
	mock.ExpectExec(regexp.QuoteMeta(query)).
		WithArgs("doc-1", "kb-1", `{"author":"Alice"}`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err = engine.UpdateMetadata(t.Context(), "doc-1", "kb-1", map[string]interface{}{"author": "Alice"}, "tenant-1"); err != nil {
		t.Fatal(err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataWritesRejectInvalidTenantIdentifier(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("oceanbase", "legacy_doc", db)

	err = engine.UpdateMetadata(t.Context(), "doc-1", "kb-1",
		map[string]interface{}{"author": "Alice"}, "tenant`injected")
	if err == nil || !strings.Contains(err.Error(), "invalid SQL identifier") {
		t.Fatalf("UpdateMetadata() error = %v, want invalid identifier error", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMetadataKeysPreservesInvalidJSONRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("oceanbase", "legacy_doc", db)

	query := "SELECT meta_fields FROM `ragflow_doc_meta_tenant-1` WHERE id = ? AND kb_id = ? LIMIT 1"
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("doc-1", "kb-1").
		WillReturnRows(sqlmock.NewRows([]string{"meta_fields"}).AddRow("{"))

	err = engine.DeleteMetadataKeys(t.Context(), "doc-1", "kb-1", []string{"author"}, "tenant-1")
	if err == nil || !strings.Contains(err.Error(), "decode metadata") {
		t.Fatalf("DeleteMetadataKeys() error = %v, want JSON decode error", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSkillFilterSearchCountsPhysicalPrimaryKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("seekdb", "legacy_doc", db)
	engine.flags = featureFlags{}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(`skill_id`) FROM `skill_tenant-1_default` WHERE 1=1")).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(skill_id)"}).AddRow(0))

	chunks, total, err := engine.searchTableWithSQL(t.Context(), "skill_tenant-1_default", "skill", map[string]interface{}{}, &types.SearchRequest{Limit: 10}, searchPlan{}, chunkTableSchema{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(chunks) != 0 {
		t.Fatalf("empty skill search total=%d chunks=%#v", total, chunks)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMemorySourceIDUsesScalarSQL(t *testing.T) {
	where, args, err := buildFilter(map[string]interface{}{"source_id": "source-1"}, "memory", chunkTableSchema{})
	if err != nil {
		t.Fatal(err)
	}
	if where != "`source_id` = ?" {
		t.Fatalf("memory source_id filter = %q", where)
	}
	if len(args) != 1 || args[0] != "source-1" {
		t.Fatalf("memory source_id args = %#v", args)
	}
}

func TestLegacyChunkFilterSQL(t *testing.T) {
	const compile = "JSON_EXTRACT(`extra`, '$.compile_kwd')"
	const children = "JSON_EXTRACT(`extra`, '$.children_kwd')"
	const tokens = "JSON_EXTRACT(`extra`, '$.token_num')"
	schema := chunkTableSchema{columns: map[string]bool{"id": true, "extra": true}, collation: "utf8mb4_unicode_ci"}
	for _, tt := range []struct {
		name, key string
		value     interface{}
		predicate string
		args      []interface{}
	}{
		{"keyword", "compile_kwd", "tree's", "(CASE WHEN JSON_TYPE(" + compile + ") <> 'NULL' THEN JSON_UNQUOTE(" + compile + ") END) COLLATE utf8mb4_unicode_ci = ?", []interface{}{"tree's"}},
		{"exists", "exists", "compile_kwd", "CASE WHEN JSON_TYPE(" + compile + ") <> 'NULL' THEN " + compile + " END IS NOT NULL", nil},
		{"absent", "must_not", map[string]interface{}{"exists": "compile_kwd"}, "CASE WHEN JSON_TYPE(" + compile + ") <> 'NULL' THEN " + compile + " END IS NULL", nil},
		{"array", "children_kwd", "child%", "(JSON_TYPE(" + children + ") = 'ARRAY' AND JSON_CONTAINS(" + children + ", ?))", []interface{}{`"child%"`}},
		{"number", "token_num", 0, "CASE WHEN JSON_TYPE(" + tokens + ") IN ('INTEGER', 'DOUBLE', 'DECIMAL') THEN CAST(JSON_UNQUOTE(" + tokens + ") AS DOUBLE) END = ?", []interface{}{0}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			where, args, err := buildFilter(map[string]interface{}{tt.key: tt.value}, "chunk", schema)
			if err != nil || where != tt.predicate || !slices.Equal(args, tt.args) {
				t.Fatalf("buildFilter() = %q, %#v, %v; want %q, %#v", where, args, err, tt.predicate, tt.args)
			}
		})
	}
}

func TestKnowledgeGraphFilterSupportsCurrentAndLegacyRows(t *testing.T) {
	where, args, err := buildFilter(map[string]interface{}{"knowledge_graph_kwd": []string{"entity", "relation"}}, "chunk", chunkTableSchema{})
	if err != nil {
		t.Fatal(err)
	}
	want := "(`type_kwd` IN (?, ?) OR `knowledge_graph_kwd` IN (?, ?))"
	if where != want {
		t.Fatalf("graph filter = %q, want %q", where, want)
	}
	if !reflect.DeepEqual(args, []interface{}{"entity", "relation", "entity", "relation"}) {
		t.Fatalf("graph filter args = %#v", args)
	}
}

func TestChunkSourceIDUsesArraySQL(t *testing.T) {
	where, args, err := buildFilter(map[string]interface{}{"source_id": "source-1"}, "chunk", chunkTableSchema{})
	if err != nil {
		t.Fatal(err)
	}
	if where != "ARRAY_CONTAINS(`source_id`, ?)" {
		t.Fatalf("chunk source_id filter = %q", where)
	}
	if len(args) != 1 || args[0] != "source-1" {
		t.Fatalf("chunk source_id args = %#v", args)
	}
}
