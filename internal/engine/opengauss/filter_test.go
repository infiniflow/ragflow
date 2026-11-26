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

package opengauss

import (
	"reflect"
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

func TestOpenGaussNestedFilterBindings(t *testing.T) {
	e := Engine{}
	condition := map[string]any{
		"available_int": 1,
		"or": []map[string]any{
			{"and": []any{map[string]any{"doc_id": "d"}, map[string]any{"kb_id": "a"}}},
			{"kb_id": "b"},
		},
	}
	sql, args, err := e.buildFilter(condition, "chunk", testColumns())
	if err != nil {
		t.Fatal(err)
	}
	want := `"available_int" = $1 AND (((("doc_id" = $2) AND ("kb_id" = $3))) OR ("kb_id" = $4))`
	if sql != want || !reflect.DeepEqual(args, []any{1, "d", "a", "b"}) {
		t.Fatalf("filter = %s, args = %v", sql, args)
	}
	condition["or"] = []any{map[string]any{"doc_id": "d"}, true}
	if _, _, err := e.buildFilter(condition, "chunk", testColumns()); err == nil {
		t.Fatal("accepted malformed group")
	}
}
