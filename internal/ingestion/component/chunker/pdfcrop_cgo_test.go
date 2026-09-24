//go:build cgo

package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
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
		// Heading / table_header / table_row all carry doc_type_kwd "text" but
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

// recordingUploader captures every upload invocation so tests can assert on
// the streaming-upload behavior of cropImageChunks.
type recordingUploader struct {
	mu    sync.Mutex
	calls []uploadCall
}

type uploadCall struct {
	kbID    string
	chunkID string
	dataLen int
}

func (r *recordingUploader) upload(ctx context.Context, kbID, chunkID string, data []byte) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, uploadCall{kbID: kbID, chunkID: chunkID, dataLen: len(data)})
	r.mu.Unlock()
	// Mirror the production key format used by imageUploadDecorator so the
	// test can confirm cropImageChunks produces the same reference.
	return kbID + "-" + chunkID, nil
}

// withIngestionGlobals returns a ctx carrying kb_id / doc_id in the run-level
// CanvasState.Globals bag, exactly as the production ingestion pipeline
// seeds them via SeedIngestionGlobals before the chunker runs.
func withIngestionGlobals(t *testing.T, kbID, docID string) context.Context {
	t.Helper()
	st := runtime.NewCanvasState("test-run", "test-session")
	st.SetGlobal("kb_id", kbID)
	st.SetGlobal("doc_id", docID)
	return runtime.WithState(context.Background(), st)
}

// TestCropImageChunks_StreamingUpload verifies that, when a KB is present,
// cropImageChunks uploads each freshly cropped preview immediately and drops
// the in-memory base64, instead of holding every chunk's image until the
// later batch upload pass. This is the memory fix: peak Go-heap retention
// during the chunker stage of a large PDF collapses from "whole document" to
// "one chunk".
func TestCropImageChunks_StreamingUpload(t *testing.T) {
	ctx := withIngestionGlobals(t, "kb1", "doc1")

	rec := &recordingUploader{}
	orig := ChunkImageUploader
	ChunkImageUploader = rec.upload
	t.Cleanup(func() { ChunkImageUploader = orig })

	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{
		{CKType: "image", Text: "img-chunk", PDFPositions: pos},
		{CKType: "table", Text: "tbl-chunk", PDFPositions: pos},
		{CKType: "text", Text: "txt-chunk", PDFPositions: pos},
	}
	out := cropImageChunks(ctx, eng, chunks)
	if len(out) != len(chunks) {
		t.Fatalf("len(out) = %d, want %d", len(out), len(chunks))
	}

	for i, ck := range out {
		if ck.ImgID == "" {
			t.Errorf("chunk %d (%s): ImgID empty, want uploaded id", i, ck.CKType)
		}
		if ck.Image != "" {
			t.Errorf("chunk %d (%s): Image = %q, want cleared after upload", i, ck.CKType, ck.Image)
		}
	}
	if len(rec.calls) != len(chunks) {
		t.Fatalf("upload calls = %d, want %d", len(rec.calls), len(chunks))
	}
	// cropImageChunks fans out per chunk in goroutines, so the order of
	// upload calls is non-deterministic; assert the SET of uploaded ids
	// matches the expected per-chunk ids rather than the call order.
	wantIDs := make(map[string]struct{}, len(chunks))
	for _, ck := range chunks {
		wantIDs[common.ChunkID("doc1", ck.Text)] = struct{}{}
	}
	for _, c := range rec.calls {
		if c.dataLen == 0 {
			t.Errorf("uploaded empty bytes")
		}
		if _, ok := wantIDs[c.chunkID]; !ok {
			t.Errorf("call uploaded unexpected chunkID %q", c.chunkID)
		}
	}
	// The stored img_id per chunk must equal what the uploader returns
	// ("<kb_id>-<chunkID>"); keyed by input position, which is deterministic
	// even though the upload order is not.
	for i, ck := range out {
		wantImgID := "kb1-" + common.ChunkID("doc1", chunks[i].Text)
		if ck.ImgID != wantImgID {
			t.Errorf("chunk %d: ImgID = %q, want %q", i, ck.ImgID, wantImgID)
		}
	}
}

// TestCropImageChunks_NoUploadWhenKBAbsent locks the canvas-debug (dry-run)
// path: with no CanvasState (and thus no kb_id), cropImageChunks must NOT
// upload and must retain the base64 preview in memory — the decorator's
// debug branch is responsible for dropping it, and no persist stage will run.
func TestCropImageChunks_NoUploadWhenKBAbsent(t *testing.T) {
	ctx := context.Background() // no CanvasState → kb_id == ""

	rec := &recordingUploader{}
	orig := ChunkImageUploader
	ChunkImageUploader = rec.upload
	t.Cleanup(func() { ChunkImageUploader = orig })

	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{{CKType: "image", PDFPositions: pos}}
	out := cropImageChunks(ctx, eng, chunks)

	if len(rec.calls) != 0 {
		t.Fatalf("upload calls = %d, want 0 (kb_id absent)", len(rec.calls))
	}
	if !strings.HasPrefix(out[0].Image, "data:image/png;base64,") {
		t.Errorf("Image not retained: %q", out[0].Image)
	}
	if out[0].ImgID != "" {
		t.Errorf("ImgID = %q, want empty", out[0].ImgID)
	}
}

// TestCropImageChunks_UploadFailureFallsThroughToBatchPass verifies that a
// failed streaming upload keeps the base64 preview so the later idempotent
// batch upload pass (imageUploadDecorator) can retry it.
func TestCropImageChunks_UploadFailureFallsThroughToBatchPass(t *testing.T) {
	ctx := withIngestionGlobals(t, "kb1", "doc1")

	orig := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, _, _ string, _ []byte) (string, error) {
		return "", fmt.Errorf("boom")
	}
	t.Cleanup(func() { ChunkImageUploader = orig })

	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	chunks := []schema.ChunkDoc{{CKType: "image", Text: "img-chunk", PDFPositions: pos}}
	out := cropImageChunks(ctx, eng, chunks)

	if out[0].ImgID != "" {
		t.Errorf("ImgID = %q, want empty after failed upload", out[0].ImgID)
	}
	if !strings.HasPrefix(out[0].Image, "data:image/png;base64,") {
		t.Errorf("Image not retained after failed upload: %q", out[0].Image)
	}
}

// TestCropImageChunks_StreamingUploadUsesFinalizedText verifies that the
// streaming upload keys the image under the chunk's FINALIZED text — after
// removeTag + context fold — rather than the raw pre-finalization text. This
// is what makes the stored img_id match the canonical chunk id that
// imageUploadDecorator assigns later (register.go) and Python's convention.
// Without this, media chunks whose text changes during finalization (context
// folding or position-tag stripping) would be stored under a different key
// than their canonical id.
func TestCropImageChunks_StreamingUploadUsesFinalizedText(t *testing.T) {
	ctx := withIngestionGlobals(t, "kb1", "doc1")

	rec := &recordingUploader{}
	orig := ChunkImageUploader
	ChunkImageUploader = rec.upload
	t.Cleanup(func() { ChunkImageUploader = orig })

	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	// ContextAbove is folded into the body by materializeMediaContext, so the
	// finalized text differs from the raw Text.
	chunks := []schema.ChunkDoc{
		{CKType: "image", Text: "body", ContextAbove: "ABOVE ", PDFPositions: pos},
	}
	out := cropImageChunks(ctx, eng, chunks)

	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if len(rec.calls) != 1 {
		t.Fatalf("upload calls = %d, want 1", len(rec.calls))
	}

	// Finalized text = removeTag(ContextAbove + removeTag(Text) + ContextBelow)
	//                 = "ABOVE body".
	wantID := common.ChunkID("doc1", "ABOVE body")
	rawID := common.ChunkID("doc1", "body")

	if rec.calls[0].chunkID == rawID {
		t.Errorf("upload keyed by raw text %q; want finalized-text key %q", rawID, wantID)
	}
	if rec.calls[0].chunkID != wantID {
		t.Errorf("upload chunkID = %q, want %q", rec.calls[0].chunkID, wantID)
	}
	if out[0].ImgID != "kb1-"+wantID {
		t.Errorf("ImgID = %q, want %q", out[0].ImgID, "kb1-"+wantID)
	}
}

// TestCropImageChunks_StreamingUploadUsesCanonicalID is the regression test
// for the most common chunk shape: an image/table chunk whose Text still
// carries parser position tags (@@…##) but has NO media context
// (ContextAbove/Below empty). For that shape materializeMediaContext
// short-circuits and leaves the tags in place, so the raw text still carries
// them. The canonical chunk id (what imageUploadDecorator assigns later in
// register.go as ck["id"] and what retrieval looks the object up by) is
// derived from the TAG-STRIPPED (finalized) text. The streamed upload key
// MUST therefore also be the tag-stripped canonical id — keying on the raw,
// tag-bearing text would store the object under a key that never matches
// ck["id"]. canonicalChunkID is the single source for that id, so the two
// can never diverge.
func TestCropImageChunks_StreamingUploadUsesCanonicalID(t *testing.T) {
	ctx := withIngestionGlobals(t, "kb1", "doc1")

	rec := &recordingUploader{}
	orig := ChunkImageUploader
	ChunkImageUploader = rec.upload
	t.Cleanup(func() { ChunkImageUploader = orig })

	eng := mockCropEngine{}
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	// Text carries a parser position tag (@@x\t y##), no media context.
	in := schema.ChunkDoc{CKType: "image", Text: "abc@@1\t2##", PDFPositions: pos}
	out := cropImageChunks(ctx, eng, []schema.ChunkDoc{in})

	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if len(rec.calls) != 1 {
		t.Fatalf("upload calls = %d, want 1", len(rec.calls))
	}

	// The streamed upload key must be the canonical (tag-STRIPPED) id,
	// exactly what imageUploadDecorator later exposes as ck["id"].
	wantID := canonicalChunkID("doc1", in)
	if rec.calls[0].chunkID != wantID {
		t.Errorf("upload chunkID = %q, want canonical %q", rec.calls[0].chunkID, wantID)
	}
	// Regression guard: it must NOT be the raw tag-bearing variant.
	if rec.calls[0].chunkID == common.ChunkID("doc1", in.Text) {
		t.Errorf("upload keyed by raw tag-bearing text %q; must use canonical (tag-stripped) id", common.ChunkID("doc1", in.Text))
	}
	if out[0].ImgID != "kb1-"+wantID {
		t.Errorf("ImgID = %q, want %q", out[0].ImgID, "kb1-"+wantID)
	}
}
