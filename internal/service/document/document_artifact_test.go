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

package document

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

func TestDocPreviewTextRendersTableItems(t *testing.T) {
	table := &entity.TableData{
		Grid:       [][]entity.TableCell{{{Text: "Owner"}, {Text: "RET"}}},
		HeaderRows: 1,
	}
	res := parser.ParseResult{JSON: parser.MapsToItems([]map[string]any{
		{"text": "Before", "doc_type_kwd": "text"},
		{"doc_type_kwd": "table", "table": table},
	})}

	text, err := docPreviewText(res)
	if err != nil {
		t.Fatalf("docPreviewText: %v", err)
	}
	for _, want := range []string{"Before", "Owner", "RET"} {
		if !strings.Contains(text, want) {
			t.Fatalf("preview %q missing %q", text, want)
		}
	}
}
