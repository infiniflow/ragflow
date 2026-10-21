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

	"ragflow/internal/ingestion/component/schema"
)

// TestChunkerOutputDropsRetiredCKType pins the output contract: ck_type is a
// retired parser key and no chunker may emit it, even when the input payload
// still carries a legacy ck_type.
func TestChunkerOutputDropsRetiredCKType(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T) []map[string]any
	}{
		{
			name: "general",
			run: func(t *testing.T) []map[string]any {
				c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 128})
				if err != nil {
					t.Fatalf("NewGeneralChunker: %v", err)
				}
				out, err := c.Invoke(t.Context(), nil, map[string]any{
					"name": "t.md", "file_type": "md", "output_format": "json",
					"json": []map[string]any{
						{"text": "# Heading", "doc_type_kwd": "text", "layout": "heading", "ck_type": "heading"},
						{"text": "body", "doc_type_kwd": "text", "ck_type": "text"},
					},
				})
				if err != nil {
					t.Fatalf("GeneralChunker.Invoke: %v", err)
				}
				return outputChunks(t, out)
			},
		},
		{
			name: "token",
			run: func(t *testing.T) []map[string]any {
				c, err := NewTokenChunker(map[string]any{"chunk_token_size": 128})
				if err != nil {
					t.Fatalf("NewTokenChunker: %v", err)
				}
				out, err := c.Invoke(t.Context(), nil, map[string]any{
					"name": "t.md", "output_format": "json",
					"json": []map[string]any{
						{"text": "# Heading", "doc_type_kwd": "text", "layout": "heading", "ck_type": "heading"},
						{"text": "body", "doc_type_kwd": "text", "ck_type": "text"},
					},
				})
				if err != nil {
					t.Fatalf("TokenChunker.Invoke: %v", err)
				}
				return outputChunks(t, out)
			},
		},
		{
			name: "one",
			run: func(t *testing.T) []map[string]any {
				c, err := NewOneChunker(nil)
				if err != nil {
					t.Fatalf("NewOneChunker: %v", err)
				}
				out, err := c.Invoke(t.Context(), nil, map[string]any{
					"name": "t.txt", "output_format": "text", "text": "hello",
				})
				if err != nil {
					t.Fatalf("OneChunker.Invoke: %v", err)
				}
				return outputChunks(t, out)
			},
		},
		{
			name: "title",
			run: func(t *testing.T) []map[string]any {
				c, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
				if err != nil {
					t.Fatalf("NewGroupTitleChunker: %v", err)
				}
				out, err := c.Invoke(t.Context(), nil, map[string]any{
					"name": "t.md",
					"chunks": []map[string]any{
						{"text": "# Heading", "doc_type_kwd": "text", "layout": "heading", "ck_type": "heading"},
						{"text": "body", "doc_type_kwd": "text", "ck_type": "text"},
					},
				})
				if err != nil {
					t.Fatalf("GroupTitleChunker.Invoke: %v", err)
				}
				return outputChunks(t, out)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunks := tc.run(t)
			if len(chunks) == 0 {
				t.Fatal("no chunks emitted")
			}
			for i, ck := range chunks {
				if kind, ok := ck["ck_type"]; ok {
					t.Errorf("chunk[%d] emits retired ck_type %v: %#v", i, kind, ck)
				}
				if layout, ok := ck["layout"].(string); ok && isStructuralLayout(layout) {
					t.Errorf("chunk[%d] leaks structural layout %q: %#v", i, layout, ck)
				}
			}
		})
	}
}

// TestChunkOutputBoundaryClearsStructuralLayout pins which layout values are
// chunker-internal. Structural block kinds are consumed before the boundary
// and must not leak; PDF model layouts pass through unchanged.
func TestChunkOutputBoundaryClearsStructuralLayout(t *testing.T) {
	out := chunkOutputs([]schema.ChunkDoc{
		{Text: "heading", Layout: "heading"},
		{Text: "paragraph", Layout: "paragraph"},
		{Text: "code", Layout: "code"},
		{Text: "figure", DocType: "image", Layout: "figure"},
		{Text: "title", Layout: "title"},
		{Text: "equation", Layout: "equation"},
	})
	chunks := outputChunks(t, out)
	for i := 0; i < 3; i++ {
		if layout, ok := chunks[i]["layout"]; ok {
			t.Errorf("chunk[%d] leaked structural layout %v: %#v", i, layout, chunks[i])
		}
	}
	for i, want := range []string{"figure", "title", "equation"} {
		if got := chunks[3+i]["layout"]; got != want {
			t.Errorf("chunk[%d] layout = %v, want %q (PDF model layout must pass through)", 3+i, got, want)
		}
	}
}

// TestTitleReadsLayoutNotRetiredCKType pins that a legacy ck_type alone no
// longer promotes a line to a heading; only layout does.
func TestTitleReadsLayoutNotRetiredCKType(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name": "t.docx",
		"chunks": []map[string]any{
			{"text": "Introduction", "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "Body.", "doc_type_kwd": "text"},
			{"text": "Conclusion", "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "Final.", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if chunks := outputChunks(t, out); len(chunks) != 1 {
		t.Fatalf("legacy ck_type must not create headings: got %d chunks, want 1", len(chunks))
	}
}

// TestMarkdownImageHeadingFoldsIntoPreviousChunk pins the base chunk
// boundary for an image-only heading: the image is a media attachment and
// folds into the preceding chunk.
func TestMarkdownImageHeadingFoldsIntoPreviousChunk(t *testing.T) {
	c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name": "t.md", "file_type": "md", "output_format": "json",
		"json": []map[string]any{
			{"text": "intro", "doc_type_kwd": "text"},
			// An image-only heading: markdown labels it as an image but keeps
			// the heading layout. Base folded it into the preceding chunk.
			{"text": "##", "doc_type_kwd": "image", "image": "data:image/png;base64,AAAA", "layout": "heading"},
			{"text": "after", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}
	if chunks := outputChunks(t, out); len(chunks) != 1 {
		t.Fatalf("image-only heading must fold into the preceding chunk: got %d chunks, want 1", len(chunks))
	}
}

// TestTitleKeepsHTMLParagraphAtomic pins the line-split gate: a paragraph
// layout keeps a multi-line block as one record, so an inner line that looks
// like a heading cannot create a spurious section.
func TestTitleKeepsHTMLParagraphAtomic(t *testing.T) {
	c, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name": "t.html",
		"chunks": []map[string]any{
			{"text": "intro\n# not a heading", "doc_type_kwd": "text", "layout": "paragraph"},
			{"text": "body", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if chunks := outputChunks(t, out); len(chunks) != 1 {
		t.Fatalf("paragraph must stay atomic: got %d chunks, want 1", len(chunks))
	}
}
