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

package schema

import (
	"encoding/json"
	"testing"

	"ragflow/internal/entity"
)

func TestChunkDocUnmarshalTableField(t *testing.T) {
	src := `{
		"doc_type_kwd": "table",
		"table": {"rows": [["Name", "Age"], ["Alice", "30"]], "header_rows": 1, "caption": "People"},
		"text": "legacy-html-should-be-ignored"
	}`
	var cd ChunkDoc
	if err := json.Unmarshal([]byte(src), &cd); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if cd.TableData == nil {
		t.Fatalf("TableData == nil, want populated")
	}
	if cd.TableData.HeaderRows != 1 {
		t.Fatalf("HeaderRows = %d, want 1", cd.TableData.HeaderRows)
	}
	if cd.TableData.Caption != "People" {
		t.Fatalf("Caption = %q, want %q", cd.TableData.Caption, "People")
	}
	if len(cd.TableData.Rows) != 2 || cd.TableData.Rows[1][0] != "Alice" {
		t.Fatalf("Rows = %v, unexpected", cd.TableData.Rows)
	}
	// The "table" key must NOT also land in Extra (no double storage).
	if _, ok := cd.Extra["table"]; ok {
		t.Fatalf("table key leaked into Extra: %v", cd.Extra)
	}
}

func TestChunkDocMarshalRoundTripTable(t *testing.T) {
	cd := ChunkDoc{
		DocType: "table",
		TableData: &entity.TableData{
			Rows:       [][]string{{"Name", "Age"}, {"Alice", "30"}},
			HeaderRows: 1,
		},
	}
	b, err := json.Marshal(cd)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var back ChunkDoc
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if back.TableData == nil || len(back.TableData.Rows) != 2 {
		t.Fatalf("round-trip lost TableData: %v", back.TableData)
	}
}

func TestChunkDocNoTableFieldUnaffected(t *testing.T) {
	src := `{"doc_type_kwd": "text", "text": "hello", "custom": 42}`
	var cd ChunkDoc
	if err := json.Unmarshal([]byte(src), &cd); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if cd.TableData != nil {
		t.Fatalf("TableData = %v, want nil", cd.TableData)
	}
	if _, ok := cd.Extra["custom"]; !ok {
		t.Fatalf("Extra lost unrelated key: %v", cd.Extra)
	}
}
