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
	"testing"

	"ragflow/internal/engine/types"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSearchAppliesPaginationAfterMergingTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("seekdb", "legacy_doc", db)

	for _, table := range []struct {
		name string
		rows *sqlmock.Rows
	}{
		{
			name: "ragflow_tenant_1",
			rows: sqlmock.NewRows([]string{"id", "create_timestamp_flt"}).
				AddRow("chunk-1", 10.0).
				AddRow("chunk-4", 7.0),
		},
		{
			name: "ragflow_tenant_2",
			rows: sqlmock.NewRows([]string{"id", "create_timestamp_flt"}).
				AddRow("chunk-2", 9.0).
				AddRow("chunk-3", 8.0),
		},
	} {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
			WithArgs("legacy_doc", table.name).
			WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(`id`) FROM `" + table.name + "` WHERE 1=1")).
			WillReturnRows(sqlmock.NewRows([]string{"COUNT(id)"}).AddRow(2))
		mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`, `create_timestamp_flt` FROM `" + table.name + "` WHERE 1=1 ORDER BY `create_timestamp_flt` DESC LIMIT 0, 3")).
			WillReturnRows(table.rows)
	}

	orderBy := (&types.OrderByExpr{}).Desc("create_timestamp_flt")
	result, err := engine.Search(t.Context(), &types.SearchRequest{
		IndexNames:   []string{"ragflow_tenant_1", "ragflow_tenant_2"},
		Offset:       1,
		Limit:        2,
		SelectFields: []string{"id"},
		OrderBy:      orderBy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 4 {
		t.Fatalf("Search() total = %d, want 4", result.Total)
	}
	if len(result.Chunks) != 2 {
		t.Fatalf("Search() returned %d chunks, want 2: %#v", len(result.Chunks), result.Chunks)
	}
	gotIDs := []interface{}{result.Chunks[0]["id"], result.Chunks[1]["id"]}
	if want := []interface{}{"chunk-2", "chunk-3"}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("Search() page IDs = %v, want %v", gotIDs, want)
	}
	if _, exists := result.Chunks[0]["create_timestamp_flt"]; exists {
		t.Fatalf("Search() exposed an internal sort field: %#v", result.Chunks[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildQualifiedSelectFieldsUsesStructuredColumns(t *testing.T) {
	fields, aliases, err := buildQualifiedSelectFields([]string{"content", "row_id()"}, "memory", "t")
	if err != nil {
		t.Fatal(err)
	}
	wantFields := "`t`.`id`, `t`.`content_ltks` AS `content`, `t`.`id` AS `row_id`"
	if fields != wantFields {
		t.Fatalf("buildQualifiedSelectFields() = %q, want %q", fields, wantFields)
	}
	if wantAliases := []string{"id", "content", "row_id"}; !reflect.DeepEqual(aliases, wantAliases) {
		t.Fatalf("buildQualifiedSelectFields() aliases = %v, want %v", aliases, wantAliases)
	}
}
