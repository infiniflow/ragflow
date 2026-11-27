package chunker

import (
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"reflect"
	"strings"
	"testing"
)

func TestTokenChunkerCustomDelimiterMetadata(t *testing.T) {
	refA := &entity.MediaRef{Format: "html", Kind: entity.MediaKindKV, SHA256: strings.Repeat("a", 64)}
	refB := &entity.MediaRef{Format: "html", Kind: entity.MediaKindKV, SHA256: strings.Repeat("b", 64)}
	for _, tt := range []struct {
		name   string
		first  schema.ChunkDoc
		second schema.ChunkDoc
		want   []schema.ChunkDoc
	}{
		{
			name:  "different parents",
			first: schema.ChunkDoc{Mom: "parent-A"}, second: schema.ChunkDoc{Mom: "parent-B"},
			want: []schema.ChunkDoc{{Text: "A", Mom: "parent-A"}, {Text: "B", Mom: "parent-B"}},
		},
		{
			name:   "missing parent",
			second: schema.ChunkDoc{Mom: "parent-B"},
			want:   []schema.ChunkDoc{{Text: "A\nB", Mom: "parent-B"}},
		},
		{
			name:  "same parent",
			first: schema.ChunkDoc{Mom: "parent-A"}, second: schema.ChunkDoc{Mom: "parent-A"},
			want: []schema.ChunkDoc{{Text: "A\nB", Mom: "parent-A"}},
		},
		{
			name:  "different image IDs",
			first: schema.ChunkDoc{ImgID: "image-A"}, second: schema.ChunkDoc{ImgID: "image-B"},
			want: []schema.ChunkDoc{{Text: "A", ImgID: "image-A"}, {Text: "B", ImgID: "image-B"}},
		},
		{
			name:   "missing image ID",
			second: schema.ChunkDoc{ImgID: "image-B"},
			want:   []schema.ChunkDoc{{Text: "A\nB", ImgID: "image-B"}},
		},
		{
			name:  "different images",
			first: schema.ChunkDoc{Image: "image-A"}, second: schema.ChunkDoc{Image: "image-B"},
			want: []schema.ChunkDoc{{Text: "A", Image: "image-A"}, {Text: "B", Image: "image-B"}},
		},
		{
			name:   "missing image",
			second: schema.ChunkDoc{Image: "image-B"},
			want:   []schema.ChunkDoc{{Text: "A\nB", Image: "image-B"}},
		},
		{
			name:  "different media references",
			first: schema.ChunkDoc{MediaRef: refA}, second: schema.ChunkDoc{MediaRef: refB},
			want: []schema.ChunkDoc{{Text: "A", MediaRef: refA}, {Text: "B", MediaRef: refB}},
		},
		{
			name:   "missing media reference",
			second: schema.ChunkDoc{MediaRef: refB},
			want:   []schema.ChunkDoc{{Text: "A\nB", MediaRef: refB}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewTokenChunker(map[string]any{"delimiters": []string{"`#`"}})
			if err != nil {
				t.Fatal(err)
			}
			tt.first.Text, tt.second.Text = "A", "B"
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"name":          "doc.txt",
				"output_format": "chunks",
				"chunks":        []schema.ChunkDoc{tt.first, tt.second},
			})
			if err != nil {
				t.Fatal(err)
			}
			chunks, ok, err := schema.ChunkDocsFromAny(out["chunks"])
			if err != nil || !ok {
				t.Fatalf("chunks: %v, %v", out["chunks"], err)
			}
			for i := range chunks {
				chunks[i].DocType = ""
				chunks[i].TKNums = nil
			}
			if !reflect.DeepEqual(chunks, tt.want) {
				t.Fatalf("chunks = %#v, want %#v", chunks, tt.want)
			}
		})
	}
}

// TestTokenChunker_BareDelimiterHonored locks the #17723 fix: a bare
// (non-backtick) delimiter entry is now honored by TokenChunker. The payload
// is split on the delimiter into paragraphs (the delimiter is RETAINED
// losslessly, since a bare delimiter is a split hint, not a delete instruction),
// and those paragraphs are then merged by token_size.
// Regression guard for the "bare entries are active" contract.
func TestTokenChunker_BareDelimiterHonored(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"delimiters":       []string{"::"},
		"chunk_token_size": float64(8),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	const text = "alpha::beta::gamma::delta"
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          text,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) == 0 {
		t.Fatalf("no chunks produced")
	}
	var joined strings.Builder
	for _, ck := range chunks {
		joined.WriteString(ck["text"].(string))
	}
	// Lossless: the joined text reproduces the source exactly, with the bare
	// "::" delimiter retained inside each chunk (a delimiter is a split hint,
	// not a delete instruction).
	const want = "alpha::beta::gamma::delta"
	if joined.String() != want {
		t.Errorf("bare delimiter not honored: joined=%q want %q (chunks=%v)", joined.String(), want, chunkTexts(chunks))
	}
	for _, ck := range chunks {
		if !strings.Contains(ck["text"].(string), "::") {
			t.Errorf("bare delimiter dropped from chunk: %q", ck["text"].(string))
		}
	}
}

// TestTokenChunker_MultiByteBacktickDelimiter locks T2: a backtick-wrapped
// multi-byte (CJK) delimiter contributes its INNER content as the split
// pattern, and the longest delimiter wins over a shorter prefix of it
// (rune-descending sort). Both ensure the chunker-level delimiter handling
// is correct for non-ASCII delimiters.
func TestTokenChunker_MultiByteBacktickDelimiter(t *testing.T) {
	// Single multi-byte delimiter splits on its inner content.
	c, err := NewTokenChunker(map[string]any{
		"delimiters":       []string{"`段落`"},
		"chunk_token_size": float64(1024),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "第一部分段落第二部分",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	// Custom (backtick) delimiters are DROPPED, mirroring Python's
	// naive_merge has_custom branch: "段落" is split away, not preserved.
	want := []string{"第一部分", "第二部分"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		if got := chunks[i]["text"].(string); got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
	}

	// Longest delimiter must still win over its shorter prefix: `段落` (2 runes)
	// beats `段` (1 rune), so "A段落B" splits on "段落", not "段" — and the
	// delimiter is dropped from both chunks.
	c2, err := NewTokenChunker(map[string]any{
		"delimiters":       []string{"`段落`", "`段`"},
		"chunk_token_size": float64(1024),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out2, err := c2.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "A段落B",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks2, _ := schema.ChunkCarrierToMaps(out2["chunks"])
	if len(chunks2) != 2 || chunks2[0]["text"].(string) != "A" || chunks2[1]["text"].(string) != "B" {
		t.Errorf("longest delimiter not preferred: got %v", chunkTexts(chunks2))
	}
}
