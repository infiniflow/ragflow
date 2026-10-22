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

package chunker

import (
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

// Linked-content provenance (#21032) rides the item `extra` slot from the
// Parser post-step to the indexed chunk. Every chunker family has to carry it:
// the General/Token paths re-emit ChunkDoc.Extra, and the title family joins
// records into groups, which used to drop the item keys entirely.
func mustTokenChunker(t *testing.T, chunkTokens int) runtime.Component {
	t.Helper()
	c, err := NewTokenChunker(map[string]any{"chunk_token_size": chunkTokens})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	return c
}

func TestChunkersCarryItemExtra(t *testing.T) {
	linked := map[string]any{
		"source_url":    "https://example.com/linked",
		"parent_doc_id": "doc-1",
	}
	items := func(extra map[string]any) []map[string]any {
		return []map[string]any{
			{"text": "# Heading", "doc_type_kwd": "text"},
			// A second dynamic key proves the whole item Extra travels, not just
			// the `extra` slot the feature happens to use.
			{"text": "linked body", "doc_type_kwd": "text", "extra": extra, "slide_number": 2},
		}
	}

	cases := []struct {
		name  string
		build func(t *testing.T) runtime.Component
	}{
		{
			name: "general",
			build: func(t *testing.T) runtime.Component {
				c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 128})
				if err != nil {
					t.Fatalf("NewGeneralChunker: %v", err)
				}
				return c
			},
		},
		{
			name: "token",
			build: func(t *testing.T) runtime.Component {
				c, err := NewTokenChunker(map[string]any{"chunk_token_size": 128})
				if err != nil {
					t.Fatalf("NewTokenChunker: %v", err)
				}
				return c
			},
		},
		{
			name: "title",
			build: func(t *testing.T) runtime.Component {
				c, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
				if err != nil {
					t.Fatalf("NewGroupTitleChunker: %v", err)
				}
				return c
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.build(t).Invoke(t.Context(), nil, map[string]any{
				"name": "t.md", "file_type": "md", "output_format": "json",
				"json": items(linked),
			})
			if err != nil {
				t.Fatalf("%s Invoke: %v", tc.name, err)
			}
			chunks := outputChunks(t, out)
			var labelled int
			for _, ck := range chunks {
				if !strings.Contains(toString(ck["text"]), "linked body") {
					continue
				}
				// The nested slot has to be nested already in the chunker's own
				// payload: `extra` is the declared column the engine write path
				// keeps, and the families reach it by different routes (General
				// and Token through ChunkDoc's wire map, the title family through
				// the group merge). Asserting the key here is what makes "these
				// shapes agree" a stated contract rather than an accident of
				// Extra's decode whitelist.
				if _, ok := ck["extra"]; !ok {
					t.Fatalf("%s: chunk payload carries no nested extra key: %#v", tc.name, ck)
				}
				// The title path emits the raw `extra` object; the General/Token
				// paths decode it. Assert through JSON so both shapes are covered
				// the way the engine write path sees them.
				encoded, err := json.Marshal(ck)
				if err != nil {
					t.Fatalf("%s: marshal chunk: %v", tc.name, err)
				}
				var decoded struct {
					Extra       map[string]any `json:"extra"`
					SlideNumber int            `json:"slide_number"`
				}
				if err := json.Unmarshal(encoded, &decoded); err != nil {
					t.Fatalf("%s: unmarshal chunk: %v", tc.name, err)
				}
				if decoded.Extra["source_url"] != "https://example.com/linked" || decoded.Extra["parent_doc_id"] != "doc-1" {
					t.Fatalf("%s: chunk extra = %#v, want the provenance pair (%s)", tc.name, decoded.Extra, encoded)
				}
				if decoded.SlideNumber != 2 {
					t.Fatalf("%s: slide_number = %d, want the item key carried alongside provenance (%s)", tc.name, decoded.SlideNumber, encoded)
				}
				labelled++
			}
			if labelled == 0 {
				t.Fatalf("%s: no chunk carries the linked body", tc.name)
			}
		})
	}
}

// This pins a KNOWN LIMITATION, not a desired end state. The Token chunker's
// final merge is deliberately global (Python _merge_text_chunks_by_token_size
// merges adjacent text chunks across JSON items), so a linked page can be folded
// into the parent document's under-cap tail chunk. `extra` merges first-wins per
// key, and the parent's chunk carries none, so the linked page's provenance
// lands on the merged chunk — which then reports parent body text as if it came
// from the fetched URL. Provenance is per item, not per chunk. If a merge barrier
// across differing provenance is added, this test must flip to asserting both
// sources rather than being deleted.
func TestTokenMergeAcrossSourcesAttributesTheWholeChunkToTheLinkedPage(t *testing.T) {
	out, err := mustTokenChunker(t, 512).Invoke(t.Context(), nil, map[string]any{
		"name": "t.md", "file_type": "md", "output_format": "json",
		"json": []map[string]any{
			{"text": "parent tail", "doc_type_kwd": "text"},
			{
				"text": "linked body", "doc_type_kwd": "text",
				"extra": map[string]any{"source_url": "https://example.com/linked", "parent_doc_id": "doc-1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	chunks := outputChunks(t, out)
	if len(chunks) != 1 {
		t.Fatalf("len = %d, want the two items merged into one chunk", len(chunks))
	}
	encoded, err := json.Marshal(chunks[0])
	if err != nil {
		t.Fatalf("marshal chunk: %v", err)
	}
	var decoded struct {
		Text  string         `json:"text"`
		Extra map[string]any `json:"extra"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal chunk: %v", err)
	}
	if !strings.Contains(decoded.Text, "parent tail") || !strings.Contains(decoded.Text, "linked body") {
		t.Fatalf("text = %q, want both bodies merged", decoded.Text)
	}
	if decoded.Extra["source_url"] != "https://example.com/linked" {
		t.Fatalf("chunk extra = %#v, want the linked page's URL on the merged chunk: %s", decoded.Extra, encoded)
	}
}
