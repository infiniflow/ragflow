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
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestOneChunkerStripsCoordTagsJSON asserts the single-chunk ("one") JSON path
// strips @@...## coordinate tags from the merged output, matching the Python
// flow chunker's tag-free "one" behaviour (token_chunker.py delimiter_mode one).
// Regression: emitOneFromItems previously joined item texts verbatim, leaking
// coordinate tags into the single emitted chunk.
func TestOneChunkerStripsCoordTagsJSON(t *testing.T) {
	items := []schema.ChunkDoc{
		{Text: "Sentence one@@1\t2\t3\t4##", CKType: "text"},
		{Text: "Sentence two@@1\t2\t3\t4##", CKType: "text"},
	}
	out := emitOneFromItems(items, nil)
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	if strings.Contains(text, "@@") || strings.Contains(text, "##") {
		t.Errorf("coord tag leaked into one-chunk JSON output: %q", text)
	}
	if want := "Sentence one\nSentence two"; text != want {
		t.Errorf("one-chunk JSON text = %q, want %q", text, want)
	}
}

// TestOneChunkerStripsCoordTagsText asserts the single-chunk text/markdown/html
// path also strips coordinate tags before emitting.
func TestOneChunkerStripsCoordTagsText(t *testing.T) {
	out := emitOne("Hello world@@1\t2\t3\t4##", "text")
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	text, _ := chunks[0]["text"].(string)
	if strings.Contains(text, "@@") || strings.Contains(text, "##") {
		t.Errorf("coord tag leaked into one-chunk text output: %q", text)
	}
	if want := "Hello world"; text != want {
		t.Errorf("one-chunk text = %q, want %q", text, want)
	}
}
