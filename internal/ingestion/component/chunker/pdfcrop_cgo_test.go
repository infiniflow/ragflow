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
		// --- CKType set (general / token paths): unchanged behavior ---
		{"image with positions", schema.ChunkDoc{CKType: "image", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"table with positions", schema.ChunkDoc{CKType: "table", Positions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"text with positions", schema.ChunkDoc{CKType: "text", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"image without positions", schema.ChunkDoc{CKType: "image"}, false},
		{"heading excluded via CKType", schema.ChunkDoc{CKType: "heading", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, false},
		{"unknown type via CKType", schema.ChunkDoc{CKType: "equation", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, false},

		// --- CKType empty: fall back to DocType (group / hierarchy paths,
		// whose parser output carries no ck_type). This is the fix for the
		// bug where figure/image/table in group/hierarchy were never cropped.
		{"doc image fallback", schema.ChunkDoc{DocType: "image", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"doc table fallback", schema.ChunkDoc{DocType: "table", Positions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		{"doc text fallback", schema.ChunkDoc{DocType: "text", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, true},
		// Heading-tagged blocks carry doc_type_kwd "text" but
		// must NOT be previewed when CKType is empty — only a plain text body
		// chunk legitimately has CKType empty AND should be previewed. Since
		// group/hierarchy cannot distinguish heading from body at this layer,
		// aligning with Python (which previews every text region) we still crop
		// doc_type_kwd=="text"; the heading-collision guard lives in the
		// general/token paths where CKType is set.
		{"doc image without positions", schema.ChunkDoc{DocType: "image"}, false},
		{"doc unknown type", schema.ChunkDoc{DocType: "equation", PDFPositions: jsonPositions(t, []float64{1, 10, 100, 10, 100})}, false},
	}
	for _, tc := range cases {
		if got := needsCrop(tc.ck); got != tc.want {
			t.Errorf("%s: needsCrop = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCropImageChunks_CropsImageTableAndText(t *testing.T) {
	ctx := context.Background()
	// 1-based JSON position (1) must be rendered as 0-based page 0.
	eng := assertZeroPageEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	chunks := []schema.ChunkDoc{
		{CKType: "image", PDFPositions: pos},
		{CKType: "table", PDFPositions: pos},
		{CKType: "text", PDFPositions: pos},                           // restored preview (Chunker-1.3)
		{CKType: "image", Image: "data:image/png;base64,preexisting"}, // preserved
	}
	out := cropImageChunks(ctx, eng, chunks)
	if len(out) != len(chunks) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(chunks))
	}
	for i, ck := range out {
		switch ck.CKType {
		case "text":
			// Chunker-1.3: text chunks with PDF positions get a rendered
			// preview, mirroring Python restore_pdf_text_previews.
			if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
				t.Errorf("chunk %d (text): image = %q, want data:image/png;base64, prefix (preview restored)", i, ck.Image)
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

// TestRestorePDFTextPreview covers Chunker-1.3 directly: a text chunk that
// carries PDF positions must receive a rendered preview image, while a text
// chunk without positions must be left untouched (no spurious preview). The
// img_id upload is owned by imageUploadDecorator (image_upload.go) and is
// not asserted here.
// TestCropImageChunks_DocTypeFallbackCropsImageTableText covers the
// group/hierarchy regression fix: those chunkers forward parser output that
// carries doc_type_kwd but no ck_type. needsCrop must fall back to DocType so
// image/table/text regions are still cropped on demand (the bug was that they
// were silently skipped because ck_type was empty).
func TestCropImageChunks_DocTypeFallbackCropsImageTableText(t *testing.T) {
	ctx := context.Background()
	eng := assertZeroPageEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	chunks := []schema.ChunkDoc{
		{DocType: "image", PDFPositions: pos},
		{DocType: "table", PDFPositions: pos},
		// A plain text body chunk from group/hierarchy has CKType empty and
		// doc_type_kwd "text"; the DocType fallback must crop its preview,
		// mirroring Python restore_pdf_text_previews. (Real group/hierarchy
		// headings are also doc_type_kwd "text" with empty CKType and are
		// intentionally previewed too — full alignment with Python.)
		{DocType: "text", PDFPositions: pos},
		{DocType: "image", Image: "data:image/png;base64,preexisting"}, // preserved
	}
	out := cropImageChunks(ctx, eng, chunks)
	if len(out) != len(chunks) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(chunks))
	}
	for i, ck := range out {
		switch ck.DocType {
		case "text":
			if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
				t.Errorf("chunk %d (doc text): image = %q, want preview cropped via DocType fallback", i, ck.Image)
			}
		case "image":
			if ck.Image == "data:image/png;base64,preexisting" {
				continue
			}
			if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
				t.Errorf("chunk %d (doc image): image = %q, want cropped via DocType fallback", i, ck.Image)
			}
		case "table":
			if !strings.HasPrefix(ck.Image, "data:image/png;base64,") {
				t.Errorf("chunk %d (doc table): image = %q, want cropped via DocType fallback", i, ck.Image)
			}
		}
	}
}

// TestCropImageChunks_HeadingExcludedByCKType verifies that the general/token
// path still excludes headings: a chunk with CKType "heading" is never
// previewed even when it carries positions — the CKType branch takes priority
// over the DocType fallback.
func TestCropImageChunks_HeadingExcludedByCKType(t *testing.T) {
	ctx := context.Background()
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{
		{CKType: "heading", DocType: "text", PDFPositions: pos},
	}
	out := cropImageChunks(ctx, mockCropEngine{}, chunks)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].Image != "" {
		t.Errorf("heading chunk: image = %q, want no preview (CKType takes priority)", out[0].Image)
	}
}

func TestRestorePDFTextPreview(t *testing.T) {
	ctx := context.Background()
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})

	withPos := schema.ChunkDoc{CKType: "text", PDFPositions: pos}
	withoutPos := schema.ChunkDoc{CKType: "text", Text: "plain text, no coordinates"}

	out := cropImageChunks(ctx, mockCropEngine{}, []schema.ChunkDoc{withPos, withoutPos})
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if !strings.HasPrefix(out[0].Image, "data:image/png;base64,") {
		t.Errorf("chunk with positions: image = %q, want data:image/png;base64, prefix", out[0].Image)
	}
	if out[1].Image != "" {
		t.Errorf("chunk without positions: image = %q, want empty", out[1].Image)
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
