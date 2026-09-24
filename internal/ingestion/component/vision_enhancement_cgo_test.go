//go:build cgo

package component

import (
	"context"
	"image"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// mockVisionEngine returns a fixed image for any rendered page so the
// on-demand cropper can exercise CropSectionPositions without a real PDF.
type mockVisionEngine struct {
	closed      *bool
	pageWidth   float64
	pageHeight  float64
	renderCalls *int
}

func (m mockVisionEngine) ExtractChars(int) ([]deepdoctype.TextChar, error) {
	return nil, nil
}
func (m mockVisionEngine) RenderPage(int, float64) ([]byte, error) { return nil, nil }
func (m mockVisionEngine) RenderPageImage(int, float64) (image.Image, error) {
	if m.renderCalls != nil {
		*m.renderCalls++
	}
	return image.NewRGBA(image.Rect(0, 0, 1000, 1000)), nil
}
func (m mockVisionEngine) PageSize(int) (float64, float64, error) {
	width, height := m.pageWidth, m.pageHeight
	if width == 0 {
		width = 333
	}
	if height == 0 {
		height = 333
	}
	return width, height, nil
}
func (m mockVisionEngine) RawData() []byte                          { return nil }
func (m mockVisionEngine) PageCount() (int, error)                  { return 1, nil }
func (m mockVisionEngine) Outlines() ([]deepdoctype.Outline, error) { return nil, nil }
func (m mockVisionEngine) Close() error {
	if m.closed != nil {
		*m.closed = true
	}
	return nil
}

func cgoPositions() map[string]any {
	// 1-based page 1 region; PositionsFromMatrix shifts to 0-based.
	return map[string]any{
		"_pdf_positions": [][]any{{1, 10.0, 100.0, 10.0, 100.0}},
	}
}

// TestVisionCropImage_OnDemandFromPositions verifies the core fix: a parsed
// item that carries PDF positions but no inlined image is cropped on demand
// by re-acquiring the source PDF from storage and rendering the region.
func TestVisionCropImage_OnDemandFromPositions(t *testing.T) {
	closed := false
	oldFetcher := visionSourceFetcher
	oldOpener := visionEngineOpener
	defer func() {
		visionSourceFetcher = oldFetcher
		visionEngineOpener = oldOpener
	}()
	visionSourceFetcher = func(ctx context.Context, bucket, path string) ([]byte, error) {
		return []byte("%PDF-fake-engine-bytes"), nil
	}
	visionEngineOpener = func(data []byte) (deepdoctype.PDFEngine, error) {
		return mockVisionEngine{closed: &closed}, nil
	}

	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{"bucket": "b", "path": "p"})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	defer cropper.Close()

	img, err := cropper.Crop(context.Background(), cgoPositions())
	if err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if img == nil || img.Raster == nil {
		t.Fatal("Crop returned no raster; expected an on-demand cropped image")
	}
	if img.Raster.Bounds().Empty() {
		t.Fatal("Crop returned an empty raster")
	}
	if closed {
		t.Fatal("engine closed before use completed")
	}
}

func TestVisionCropImage_RejectsOversizedPageBeforeRendering(t *testing.T) {
	renderCalls := 0
	oldFetcher := visionSourceFetcher
	oldOpener := visionEngineOpener
	defer func() {
		visionSourceFetcher = oldFetcher
		visionEngineOpener = oldOpener
	}()
	visionSourceFetcher = func(context.Context, string, string) ([]byte, error) {
		return []byte("%PDF-fake-engine-bytes"), nil
	}
	visionEngineOpener = func([]byte) (deepdoctype.PDFEngine, error) {
		return mockVisionEngine{pageWidth: 5000, pageHeight: 1000, renderCalls: &renderCalls}, nil
	}

	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{"bucket": "b", "path": "p"})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	defer cropper.Close()

	img, err := cropper.Crop(context.Background(), cgoPositions())
	if err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if img != nil {
		t.Fatalf("Crop = %#v, want nil for an oversized page", img)
	}
	if renderCalls != 0 {
		t.Fatalf("render calls = %d, want page rejected before raster allocation", renderCalls)
	}
}

func TestVisionCropImage_RejectsAggregatePagePixelsBeforeRendering(t *testing.T) {
	renderCalls := 0
	oldFetcher := visionSourceFetcher
	oldOpener := visionEngineOpener
	defer func() {
		visionSourceFetcher = oldFetcher
		visionEngineOpener = oldOpener
	}()
	visionSourceFetcher = func(context.Context, string, string) ([]byte, error) {
		return []byte("%PDF-fake-engine-bytes"), nil
	}
	visionEngineOpener = func([]byte) (deepdoctype.PDFEngine, error) {
		return mockVisionEngine{pageWidth: 2000, pageHeight: 2000, renderCalls: &renderCalls}, nil
	}

	item := map[string]any{
		"_pdf_positions": [][]any{{[]any{1, 2}, 10.0, 100.0, 10.0, 100.0}},
	}
	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{"bucket": "b", "path": "p"})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	defer cropper.Close()

	img, err := cropper.Crop(context.Background(), item)
	if err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if img != nil {
		t.Fatalf("Crop = %#v, want nil when combined page pixels exceed the OCR budget", img)
	}
	if renderCalls != 0 {
		t.Fatalf("render calls = %d, want pages rejected before raster allocation", renderCalls)
	}
}

// TestVisionCropImage_PassthroughInline proves that when the item already
// carries an inlined image (docx/markdown, or any pre-inlined source), the
// cropper returns it directly without touching storage.
func TestVisionCropImage_PassthroughInline(t *testing.T) {
	fetchCalled := false
	oldFetcher := visionSourceFetcher
	defer func() { visionSourceFetcher = oldFetcher }()
	visionSourceFetcher = func(ctx context.Context, bucket, path string) ([]byte, error) {
		fetchCalled = true
		return nil, nil
	}

	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	defer cropper.Close()

	const inline = "data:image/png;base64,preinlined"
	img, err := cropper.Crop(context.Background(), map[string]any{"image": inline})
	if err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if img == nil || img.VLMData != inline {
		t.Fatalf("Crop = %#v, want VLM payload %q", img, inline)
	}
	if fetchCalled {
		t.Fatal("storage fetch must not be called for an inlined image")
	}
}

// TestVisionCropImage_NoImageNoPositions proves an item with neither an image
// nor positions yields no crop (no spurious storage access, no error).
func TestVisionCropImage_NoImageNoPositions(t *testing.T) {
	fetchCalled := false
	oldFetcher := visionSourceFetcher
	defer func() { visionSourceFetcher = oldFetcher }()
	visionSourceFetcher = func(ctx context.Context, bucket, path string) ([]byte, error) {
		fetchCalled = true
		return nil, nil
	}

	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	defer cropper.Close()

	img, err := cropper.Crop(context.Background(), map[string]any{"doc_type_kwd": "image"})
	if err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if img != nil {
		t.Fatalf("Crop = %#v, want nil (no image, no positions)", img)
	}
	if fetchCalled {
		t.Fatal("storage fetch must not be called without positions")
	}
}

// TestVisionCropImage_NonPDFBytesIsNoOp proves the %PDF- guard: if storage
// returns non-PDF bytes (e.g. a docx/markdown mislabeled, or a lookup miss),
// the cropper does not attempt to open an engine and returns no crop.
func TestVisionCropImage_NonPDFBytesIsNoOp(t *testing.T) {
	oldFetcher := visionSourceFetcher
	oldOpener := visionEngineOpener
	defer func() {
		visionSourceFetcher = oldFetcher
		visionEngineOpener = oldOpener
	}()
	visionSourceFetcher = func(ctx context.Context, bucket, path string) ([]byte, error) {
		return []byte("this is not a pdf"), nil
	}
	openerCalled := false
	visionEngineOpener = func(data []byte) (deepdoctype.PDFEngine, error) {
		openerCalled = true
		return mockVisionEngine{}, nil
	}

	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{"bucket": "b", "path": "p"})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	defer cropper.Close()

	img, err := cropper.Crop(context.Background(), cgoPositions())
	if err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if img != nil {
		t.Fatalf("Crop = %#v, want nil for non-PDF bytes", img)
	}
	if openerCalled {
		t.Fatal("engine opener must not run on non-PDF bytes")
	}
}

// TestVisionCropImage_EngineClosedAfterUse proves the re-acquired engine is
// released (no native handle leak) once cropping is done.
func TestVisionCropImage_EngineClosedAfterUse(t *testing.T) {
	closed := false
	oldFetcher := visionSourceFetcher
	oldOpener := visionEngineOpener
	defer func() {
		visionSourceFetcher = oldFetcher
		visionEngineOpener = oldOpener
	}()
	visionSourceFetcher = func(ctx context.Context, bucket, path string) ([]byte, error) {
		return []byte("%PDF-fake"), nil
	}
	visionEngineOpener = func(data []byte) (deepdoctype.PDFEngine, error) {
		return mockVisionEngine{closed: &closed}, nil
	}

	cropper, err := newVisionImageCropper(context.Background(), nil, map[string]any{"bucket": "b", "path": "p"})
	if err != nil {
		t.Fatalf("newVisionImageCropper: %v", err)
	}
	if _, err := cropper.Crop(context.Background(), cgoPositions()); err != nil {
		t.Fatalf("Crop: %v", err)
	}
	if err := cropper.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed {
		t.Fatal("engine should be closed after use")
	}
}
