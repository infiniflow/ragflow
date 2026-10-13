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
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// TestChunkOutputsDropsTableData pins the contract that the chunker's emitted
// result never carries the structured TableData payload: TableData is a
// parser/chunker-internal structure used only to split a table into fragments,
// and must not leak into the chunk result that downstream components
// (tokenizer, embedding, storage, API) consume. The single displayable body
// lives in Text; TableData is dropped at the one canonical emit boundary.
func TestChunkOutputsDropsTableData(t *testing.T) {
	td := &entity.TableData{Rows: [][]string{{"a", "b"}}, HeaderRows: 1}
	cases := []struct {
		name    string
		chunks  []schema.ChunkDoc
		wantLen int
	}{
		{
			name: "table + plain + table",
			chunks: []schema.ChunkDoc{
				{Text: "x", TableData: td},
				{Text: "y"},
				{Text: "z", TableData: td},
			},
			wantLen: 3,
		},
		{
			// Empty input must stay empty, not panic.
			name:    "empty",
			chunks:  []schema.ChunkDoc{},
			wantLen: 0,
		},
		{
			// All-nil TableData passes through unchanged.
			name: "all nil TableData",
			chunks: []schema.ChunkDoc{
				{Text: "p"},
				{Text: "q", DocType: "table"},
			},
			wantLen: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := chunkOutputs(tc.chunks)
			got, ok := out["chunks"].([]map[string]any)
			if !ok {
				t.Fatalf("chunks not a []map[string]any: %T", out["chunks"])
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len(chunks) = %d, want %d", len(got), tc.wantLen)
			}
			for i, c := range got {
				if _, present := c["table"]; present {
					t.Errorf("chunk %d still carries 'table' key: %#v", i, c)
				}
			}
		})
	}
}

// TestChunkOutputsPreservesText asserts that dropping TableData never discards
// the real body: Text (the display/embed body) is materialized and kept, and
// the chunk's type (doc_type_kwd) survives so downstream type-based dispatch
// (vision, enhancement) keeps working.
func TestChunkOutputsPreservesText(t *testing.T) {
	td := &entity.TableData{Rows: [][]string{{"a", "b"}}, HeaderRows: 1}
	out := chunkOutputs([]schema.ChunkDoc{
		{Text: "body-x", TableData: td, DocType: "table"},
		{Text: "body-y"},
	})
	got := out["chunks"].([]map[string]any)
	if got[0]["text"] != "body-x" || got[1]["text"] != "body-y" {
		t.Fatalf("text not preserved through chunkOutputs: %#v", got)
	}
	if _, present := got[0]["table"]; present {
		t.Fatalf("table key survived: %#v", got[0])
	}
	if got[0]["doc_type_kwd"] != "table" {
		t.Fatalf("doc_type_kwd lost while dropping TableData: %#v", got[0])
	}
}

// TestGeneralChunkerEmitsNoTableData is the end-to-end proof: a spreadsheet
// table segment fed through the GeneralChunker must come back as chunks whose
// result carries no 'table' field. Previously splitSpreadsheetTable re-attached
// the structured TableData onto every emitted piece; that leak is closed at the
// single emit boundary.
func TestGeneralChunkerEmitsNoTableData(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 20})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	var rows [][]string
	for i := 0; i < 6; i++ {
		rows = append(rows, []string{fmt.Sprintf("value-%d", i)})
	}
	item := spreadsheetSegmentItem("Sheet1", []string{"Name"}, rows, 1, 2)
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "long.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json":          []map[string]any{item},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	for i, c := range chunks {
		if _, present := c["table"]; present {
			t.Errorf("chunk %d still carries 'table' key: %#v", i, c)
		}
		text, _ := c["text"].(string)
		if !strings.Contains(text, "Name") {
			t.Errorf("chunk %d text lost header cell: %q", i, text)
		}
	}
}
