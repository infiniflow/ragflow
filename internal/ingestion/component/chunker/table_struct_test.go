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

package chunker

import (
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

func TestExpandHTMLTableRowsFromTableData(t *testing.T) {
	td := &entity.TableData{
		Rows:       [][]string{{"Name", "Age"}, {"Alice", "30"}, {"Bob", "25"}},
		HeaderRows: 1,
	}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 2 {
		t.Fatalf("got %d chunks, want 2", len(out))
	}
	if out[0].Text != "- Name: Alice\n- Age: 30" {
		t.Fatalf("row0 text = %q", out[0].Text)
	}
	if out[1].Text != "- Name: Bob\n- Age: 25" {
		t.Fatalf("row1 text = %q", out[1].Text)
	}
	for _, c := range out {
		if c.TableData != nil {
			t.Fatalf("TableData must be cleared on emitted row chunk, got %+v", c.TableData)
		}
	}
}

func TestExpandHTMLTableRowsHeaderOnlySingleChunk(t *testing.T) {
	td := &entity.TableData{Rows: [][]string{{"Name", "Age"}}, HeaderRows: 1}
	out := expandHTMLTableRows(schema.ChunkDoc{DocType: "table", TableData: td})
	if len(out) != 1 {
		t.Fatalf("got %d chunks, want 1 (header-only)", len(out))
	}
	if out[0].TableData != nil {
		t.Fatalf("TableData must be cleared on the single chunk")
	}
}

func TestExpandHTMLTableRowsNonTablePassesThrough(t *testing.T) {
	item := schema.ChunkDoc{DocType: "text", Text: "just prose"}
	out := expandHTMLTableRows(item)
	if len(out) != 1 || out[0].Text != "just prose" {
		t.Fatalf("non-table not passed through: %+v", out)
	}
}
