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
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"github.com/DATA-DOG/go-sqlmock"
)

func init() {
	_ = common.InitLogger("info", common.FileOutput{}, "oceanbase_test")
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

	chunks, total, err := engine.searchTableWithSQL(t.Context(), "skill_tenant-1_default", "skill", map[string]interface{}{}, &types.SearchRequest{Limit: 10}, searchPlan{})
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
	where, args, err := buildFilter(map[string]interface{}{"source_id": "source-1"}, "memory")
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

func TestChunkSourceIDUsesArraySQL(t *testing.T) {
	where, args, err := buildFilter(map[string]interface{}{"source_id": "source-1"}, "chunk")
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
