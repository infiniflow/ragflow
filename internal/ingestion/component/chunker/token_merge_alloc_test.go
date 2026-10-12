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

package chunker

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	sourceposition "ragflow/internal/parser/position"
)

// benchMergeUnitsJSON measures text merging with twenty preview boxes per
// source item and the default 512-token budget, including coordinate copying.
func benchMergeUnitsJSON(b *testing.B, n int) {
	pos := make([]byte, 0, 512)
	pos = append(pos, '[')
	for i := 0; i < 20; i++ {
		if i > 0 {
			pos = append(pos, ',')
		}
		pos = append(pos, fmt.Sprintf("[%d,0,%d,0,%d]", i, i*10, i*5)...)
	}
	pos = append(pos, ']')
	units := make([]schema.ChunkDoc, n)
	for i := 0; i < n; i++ {
		units[i] = schema.ChunkDoc{
			Text:   strings.Repeat("word ", 5),
			CKType: "text",
			TKNums: intPtr(5), Location: sourceposition.FromParserMap(map[string]any{"_pdf_positions": append(json.RawMessage(nil), pos...)}),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mergeByTokenSizeFromJSON([][]schema.ChunkDoc{units}, 512, 0)
	}
}

func BenchmarkMergeUnitsJSON_1k(b *testing.B)  { benchMergeUnitsJSON(b, 1000) }
func BenchmarkMergeUnitsJSON_3k(b *testing.B)  { benchMergeUnitsJSON(b, 3000) }
func BenchmarkMergeUnitsJSON_10k(b *testing.B) { benchMergeUnitsJSON(b, 10000) }
