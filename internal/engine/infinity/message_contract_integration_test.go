//go:build integration

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

package infinity

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"

	sdk "github.com/infiniflow/infinity-go-sdk"
)

func TestMessageSubstringLiteralAndReadErrors(t *testing.T) {
	if err := common.InitLogger("error", common.FileOutput{}, "sdk_message_contract"); err != nil {
		t.Fatal(err)
	}
	uri := os.Getenv("RAGFLOW_INFINITY_URI")
	if uri == "" {
		uri = "localhost:23817"
	}
	e, err := NewEngine(t.Context(), config.InfinityConfig{URI: uri, DBName: "sdk_source_contract"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	db, release, err := e.client.checkoutDatabase(t.Context(), "message_contract_test")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	base := fmt.Sprintf("memory_contract_%d", time.Now().UnixNano())
	name := base + "_test"
	table, err := db.CreateTable(name, sdk.TableSchema{&sdk.ColumnDefinition{Name: "id", DataType: "varchar"}, &sdk.ColumnDefinition{Name: "session_id", DataType: "varchar"}, &sdk.ColumnDefinition{Name: "valid_at", DataType: "varchar"}}, sdk.ConflictTypeError)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.DropTable(name, sdk.ConflictTypeIgnore) })
	for i, part := range []string{"middle", "a'b", `a"b`, `both'"quotes`, `slash\path`, "a_%b", "x OR 1=1"} {
		id := fmt.Sprint(i)
		if _, err := table.Insert([]map[string]interface{}{{"id": id, "session_id": "prefix" + part + "suffix", "valid_at": "2026-01-01"}}); err != nil {
			t.Fatal(err)
		}
		filter := equivalentConditionToStrRaw(map[string]interface{}{"session_id_contains": part}, nil, true)
		result, err := table.Output([]string{"id"}).Filter(filter).ToResult()
		if err != nil {
			t.Fatalf("%q: %v", part, err)
		}
		ids := result.(*sdk.QueryResult).Data["id"]
		if len(ids) != 1 || ids[0] != id {
			t.Fatalf("part=%q rows=%v filter=%s", part, ids, filter)
		}
	}

	laterName := base + "_later"
	later, err := db.CreateTable(laterName, sdk.TableSchema{&sdk.ColumnDefinition{Name: "id", DataType: "varchar"}, &sdk.ColumnDefinition{Name: "valid_at", DataType: "varchar"}}, sdk.ConflictTypeError)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.DropTable(laterName, sdk.ConflictTypeIgnore) })
	if _, err := later.Insert([]map[string]interface{}{{"id": "newest", "valid_at": "2026-02-01"}}); err != nil {
		t.Fatal(err)
	}
	for offset, expected := range []string{"newest", "2026-01-01"} {
		result, err := e.Search(t.Context(), &types.SearchRequest{IndexNames: []string{base}, KbIDs: []string{"test", "later"}, SelectFields: []string{"id", "q_*_vec"}, Limit: 1, Offset: offset, OrderBy: (&types.OrderByExpr{}).Desc("valid_at")})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Chunks) != 1 {
			t.Fatalf("offset=%d result=%+v", offset, result)
		}
		field := "id"
		if offset > 0 {
			field = "valid_at"
		}
		if result.Chunks[0][field] != expected {
			t.Fatalf("offset=%d rows=%v", offset, result.Chunks)
		}
	}
	if _, err := table.DropColumns([]string{"id"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GetChunk(t.Context(), base, "missing", []string{"test"}); err == nil || !strings.Contains(err.Error(), "failed to read message") {
		t.Fatalf("query error masked: %v", err)
	}
	if value, err := e.GetChunk(t.Context(), base, "missing", []string{"absent"}); err != nil || value != nil {
		t.Fatalf("absent table=%v,%v", value, err)
	}
}
