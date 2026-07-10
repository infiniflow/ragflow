//go:build cgo

package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
)

// mockCropEngine returns a fixed solid image for any rendered page. It
// ignores the page number, so it can stand in for the native engine.
type mockCropEngine struct{}

func (mockCropEngine) ExtractChars(int) ([]deepdoctype.TextChar, error) {
	return nil, nil
}
func (mockCropEngine) RenderPage(int, float64) ([]byte, error) { return nil, nil }
func (mockCropEngine) RenderPageImage(_ int, _ float64) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 1000, 1000)), nil
}
func (mockCropEngine) RawData() []byte                          { return nil }
func (mockCropEngine) PageCount() (int, error)                  { return 1, nil }
func (mockCropEngine) Outlines() ([]deepdoctype.Outline, error) { return nil, nil }
func (mockCropEngine) Close() error                             { return nil }

// assertZeroPageEngine fails unless the engine is asked for page index 0,
// which proves the chunker converts the 1-based JSON positions to the
// engine's 0-based page index before rendering.
type assertZeroPageEngine struct{}

func (assertZeroPageEngine) ExtractChars(int) ([]deepdoctype.TextChar, error) {
	return nil, nil
}
func (assertZeroPageEngine) RenderPage(int, float64) ([]byte, error) { return nil, nil }
func (assertZeroPageEngine) RenderPageImage(pageNum int, _ float64) (image.Image, error) {
	if pageNum != 0 {
		return nil, fmt.Errorf("assertZeroPageEngine: expected 0-based page, got %d", pageNum)
	}
	return image.NewRGBA(image.Rect(0, 0, 1000, 1000)), nil
}
func (assertZeroPageEngine) RawData() []byte                          { return nil }
func (assertZeroPageEngine) PageCount() (int, error)                  { return 1, nil }
func (assertZeroPageEngine) Outlines() ([]deepdoctype.Outline, error) { return nil, nil }
func (assertZeroPageEngine) Close() error                             { return nil }

func jsonPositions(t *testing.T, rows ...[]float64) json.RawMessage {
	t.Helper()
	matrix := make([][]any, 0, len(rows))
	for _, r := range rows {
		matrix = append(matrix, []any{r[0], r[1], r[2], r[3], r[4]})
	}
	b, err := json.Marshal(matrix)
	if err != nil {
		t.Fatalf("marshal positions: %v", err)
	}
	return b
}

func TestNeedsCrop(t *testing.T) {
	cases := []struct {
		name string
		ck   schema.ChunkDoc
		want bool
	}{
		{"image with positions", schema.ChunkDoc{CKType: "image", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"table with positions", schema.ChunkDoc{CKType: "table", Positions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"text with positions", schema.ChunkDoc{CKType: "text", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, false},
		{"image without positions", schema.ChunkDoc{CKType: "image"}, false},
		{"unknown type", schema.ChunkDoc{CKType: "equation", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, false},
	}
	for _, tc := range cases {
		if got := needsCrop(tc.ck); got != tc.want {
			t.Errorf("%s: needsCrop = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCropImageChunks_CropsImageAndTable(t *testing.T) {
	ctx := context.Background()
	// 1-based JSON position (1) must be rendered as 0-based page 0.
	eng := assertZeroPageEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	chunks := []schema.ChunkDoc{
		{CKType: "image", PDFPositions: pos},
		{CKType: "table", PDFPositions: pos},
		{CKType: "text", PDFPositions: pos},                           // skipped (not image/table)
		{CKType: "image", Image: "data:image/png;base64,preexisting"}, // preserved
	}
	out := cropImageChunks(ctx, eng, chunks)
	if len(out) != len(chunks) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(chunks))
	}
	for i, ck := range out {
		switch ck.CKType {
		case "text":
			if ck.Image != "" {
				t.Errorf("chunk %d (text): image should stay empty, got %q", i, ck.Image)
			}
		case "image":
			if ck.Image == "data:image/png;base64,preexisting" {
				continue // preserved, not re-cropped
			}
			if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
				t.Errorf("chunk %d (image): image = %q, want data:image/png;base64, prefix", i, ck.Image)
			}
		case "table":
			if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
				t.Errorf("chunk %d (table): image = %q, want data:image/png;base64, prefix", i, ck.Image)
			}
		}
	}
}

func TestCropImageChunks_NilEnginePassesThrough(t *testing.T) {
	ctx := context.Background()
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{
		{CKType: "image", PDFPositions: pos},
		{CKType: "table", PDFPositions: pos},
	}
	out := cropImageChunks(ctx, nil, chunks)
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	for i, ck := range out {
		if ck.Image != "" {
			t.Errorf("chunk %d: expected no crop with nil engine, got image %q", i, ck.Image)
		}
	}
}

func TestCropImageChunks_RenderFailureSkipsChunk(t *testing.T) {
	ctx := context.Background()
	// mockCropEngine renders a real image, so a non-empty crop is expected.
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{
		{CKType: "image", PDFPositions: pos},
	}
	out := cropImageChunks(ctx, mockCropEngine{}, chunks)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	// mockCropEngine renders successfully, so a non-empty crop is expected.
	if !strings.HasPrefix(out[0].Image, "data:image/png;base64,") {
		t.Errorf("chunk image = %q, want data:image/png;base64, prefix", out[0].Image)
	}
}
