// Copyright 2025 Infiniflow, Inc. All rights reserved.
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

package component

import (
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestExtractorChunkList_AcceptsTypedChunkDocs: once the upstream chunker emits
// the typed []ChunkDoc carrier, the extractor's input reader must accept it
// (converting to the map wire form) instead of returning ok=false, which would
// silently drop every upstream chunk from the extraction context.
func TestExtractorChunkList_AcceptsTypedChunkDocs(t *testing.T) {
	got, ok := extractorChunkList([]schema.ChunkDoc{{Text: "a", ID: "1"}, {Text: "b", ID: "2"}})
	if !ok {
		t.Fatal("extractorChunkList(typed []ChunkDoc) ok=false, want true")
	}
	if len(got) != 2 || got[0]["text"] != "a" || got[1]["text"] != "b" {
		t.Fatalf("typed carrier mismatch: %+v", got)
	}

	// Legacy []map[string]any must still be accepted verbatim.
	got2, ok2 := extractorChunkList([]map[string]any{{"text": "x"}})
	if !ok2 || len(got2) != 1 || got2[0]["text"] != "x" {
		t.Fatalf("legacy []map carrier: ok=%v got=%+v", ok2, got2)
	}
}
