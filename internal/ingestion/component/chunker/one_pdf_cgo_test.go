//go:build cgo

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
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/storage"
)

func newMultiPagePDFFixture(t *testing.T, pageCount int) []byte {
	t.Helper()

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	// Objects: 1=Catalog, 2=Pages, then per page: (3+2*i)=Page, (4+2*i)=Content
	totalObjs := 3 + pageCount*2
	offsets := make([]int, totalObjs)
	writeObject := func(number int, body string) {
		t.Helper()
		offsets[number] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", number, body)
	}

	kids := make([]string, 0, pageCount)
	for i := 0; i < pageCount; i++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+i*2))
	}
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount))

	for i := 0; i < pageCount; i++ {
		pageObj := 3 + i*2
		contentObj := pageObj + 1
		writeObject(pageObj, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 842] /Resources << >> /Contents %d 0 R >>", contentObj))

		streamBody := fmt.Sprintf("0.%d 0.2 0.8 rg\n60 680 220 40 re f\n", (i%9)+1)
		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)
		if _, err := w.Write([]byte(streamBody)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		writeObject(contentObj, fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", compressed.Len(), compressed.Bytes()))
	}

	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", totalObjs)
	pdf.WriteString("0000000000 65535 f \n")
	for number := 1; number < totalObjs; number++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[number])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", totalObjs, xref)
	return pdf.Bytes()
}

func TestOneChunker_PDFPreviewCropping(t *testing.T) {
	twoPagePDF := newMultiPagePDFFixture(t, 2)

	setupEnv := func(t *testing.T, pdfData []byte) (storage.Storage, func()) {
		t.Helper()
		store := storage.NewMemoryStorage()
		prevStorage, prevUploader := component.SetStorageFactoryOverride, ChunkImageUploader
		component.SetStorageFactoryOverride = func() storage.Storage { return store }
		ChunkImageUploader = component.DefaultImageUploader
		var once sync.Once
		restore := func() {
			once.Do(func() {
				component.SetStorageFactoryOverride = prevStorage
				ChunkImageUploader = prevUploader
			})
		}
		t.Cleanup(restore)
		if pdfData != nil {
			if err := store.Put(t.Context(), "one-kb", "doc.pdf", pdfData); err != nil {
				t.Fatal(err)
			}
		}
		return store, restore
	}

	t.Run("single_item_text", func(t *testing.T) {
		store, cleanup := setupEnv(t, twoPagePDF)
		t.Cleanup(cleanup)

		ctx := withIngestionGlobals(t, "one-kb", "doc-single")
		comp, err := NewOneChunker(nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
			"name": "doc.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Resume summary", "doc_type_kwd": "text", "_pdf_positions": [][]float64{{1, 50, 300, 100, 180}}},
			},
		})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		chunks := outputChunks(t, out)
		if len(chunks) != 1 {
			t.Fatalf("chunks = %d, want 1", len(chunks))
		}
		id, _ := chunks[0]["id"].(string)
		if chunks[0]["img_id"] != "one-kb-"+id || chunks[0]["image"] != nil {
			t.Fatalf("unexpected image upload result: %#v", chunks[0])
		}
		rawPNG, err := store.Get(ctx, "one-kb", id)
		if err != nil {
			t.Fatalf("store.Get preview: %v", err)
		}
		if _, err := png.DecodeConfig(bytes.NewReader(rawPNG)); err != nil {
			t.Fatalf("invalid cropped PNG: %v", err)
		}
	})

	t.Run("single_item_media", func(t *testing.T) {
		store, cleanup := setupEnv(t, twoPagePDF)
		t.Cleanup(cleanup)

		ctx := withIngestionGlobals(t, "one-kb", "doc-media")
		comp, err := NewOneChunker(nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
			"name": "doc.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Figure 1", "doc_type_kwd": "image", "_pdf_positions": [][]float64{{1, 50, 300, 100, 180}}},
			},
		})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		chunks := outputChunks(t, out)
		if len(chunks) != 1 {
			t.Fatalf("chunks = %d, want 1", len(chunks))
		}
		id, _ := chunks[0]["id"].(string)
		if chunks[0]["img_id"] != "one-kb-"+id {
			t.Fatalf("img_id = %v, want %q", chunks[0]["img_id"], "one-kb-"+id)
		}
		rawPNG, err := store.Get(ctx, "one-kb", id)
		if err != nil {
			t.Fatalf("store.Get preview: %v", err)
		}
		if _, err := png.DecodeConfig(bytes.NewReader(rawPNG)); err != nil {
			t.Fatalf("invalid cropped PNG: %v", err)
		}
	})

	t.Run("multi_item_cross_page_filters_invalid", func(t *testing.T) {
		store, cleanup := setupEnv(t, twoPagePDF)
		t.Cleanup(cleanup)

		ctx := withIngestionGlobals(t, "one-kb", "doc-multi")
		comp, err := NewOneChunker(nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
			"name": "resume.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Page 1", "_pdf_positions": [][]float64{{1, 50, 300, 100, 180}, {1, 10, 10, 5, 5}}},
				{"text": "Page 2", "_pdf_positions": [][]float64{{2, 50, 300, 100, 180}, {0, 10, 50, 10, 50}}},
			},
		})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		chunks := outputChunks(t, out)
		if len(chunks) != 1 {
			t.Fatalf("chunks = %d, want 1", len(chunks))
		}
		wantID := common.ChunkID("doc-multi", "Page 1\nPage 2")
		if chunks[0]["id"] != wantID || chunks[0]["img_id"] != "one-kb-"+wantID {
			t.Fatalf("unexpected chunk identity: id=%v img_id=%v", chunks[0]["id"], chunks[0]["img_id"])
		}
		rawPNG, err := store.Get(ctx, "one-kb", wantID)
		if err != nil {
			t.Fatalf("store.Get preview: %v", err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(rawPNG))
		if err != nil {
			t.Fatalf("invalid cropped PNG: %v", err)
		}
		if cfg.Width <= 0 || cfg.Height <= 0 {
			t.Fatalf("unexpected PNG dimensions: %dx%d", cfg.Width, cfg.Height)
		}
	})

	t.Run("multi_item_invalid_coords_only", func(t *testing.T) {
		_, cleanup := setupEnv(t, twoPagePDF)
		t.Cleanup(cleanup)

		ctx := withIngestionGlobals(t, "one-kb", "doc-invalid")
		comp, err := NewOneChunker(nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
			"name": "resume.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Only text 1", "_pdf_positions": [][]float64{{1, 10, 10, 5, 5}}},
				{"text": "Only text 2", "_pdf_positions": [][]float64{{1, 0, 0, 0, 0}}},
			},
		})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		chunks := outputChunks(t, out)
		if len(chunks) != 1 {
			t.Fatalf("chunks = %d, want 1", len(chunks))
		}
		if toString(chunks[0]["img_id"]) != "" || toString(chunks[0]["image"]) != "" {
			t.Fatalf("expected no preview image without crop regions, got %#v", chunks[0])
		}
	})

	t.Run("multi_item_inline_image_prefers_pdf_crop_and_falls_back", func(t *testing.T) {
		inlineBytes := []byte("fake-inline-png-bytes")
		inlineURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(inlineBytes)

		// 1. With valid PDF source, PDF crop overrides the sub-item's inline image.
		store, cleanup := setupEnv(t, twoPagePDF)
		ctx := withIngestionGlobals(t, "one-kb", "doc-override")
		comp, err := NewOneChunker(nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
			"name": "resume.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Part 1", "image": inlineURI, "_pdf_positions": [][]float64{{1, 50, 300, 100, 180}}},
				{"text": "Part 2", "_pdf_positions": [][]float64{{2, 50, 300, 100, 180}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		chunks := outputChunks(t, out)
		id, _ := chunks[0]["id"].(string)
		gotPNG, err := store.Get(ctx, "one-kb", id)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(gotPNG, inlineBytes) {
			t.Fatalf("expected PDF-cropped preview instead of sub-item inline image")
		}
		cleanup()

		// 2. When source PDF is unavailable, inline image fallback survives.
		store2, cleanup2 := setupEnv(t, nil)
		t.Cleanup(cleanup2)
		ctx2 := withIngestionGlobals(t, "one-kb", "doc-fallback")
		out2, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx2, nil, map[string]any{
			"name": "resume.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "missing.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Part 1", "image": inlineURI, "_pdf_positions": [][]float64{{1, 50, 300, 100, 180}}},
				{"text": "Part 2", "_pdf_positions": [][]float64{{2, 50, 300, 100, 180}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		chunks2 := outputChunks(t, out2)
		id2, _ := chunks2[0]["id"].(string)
		gotFallback, err := store2.Get(ctx2, "one-kb", id2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gotFallback, inlineBytes) {
			t.Fatalf("expected inline image fallback when PDF crop unavailable")
		}
	})

	t.Run("caps_preview_at_max_pages_and_keeps_full_positions", func(t *testing.T) {
		sevenPagePDF := newMultiPagePDFFixture(t, 7)

		storeFive, cleanupFive := setupEnv(t, sevenPagePDF)
		t.Cleanup(cleanupFive)
		comp, err := NewOneChunker(nil)
		if err != nil {
			t.Fatal(err)
		}
		ctxFive := withIngestionGlobals(t, "one-kb", "doc-five")
		outFive, err := (&imageUploadDecorator{inner: comp}).Invoke(ctxFive, nil, map[string]any{
			"name": "five.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Page 1", "_pdf_positions": [][]float64{{1, 50, 300, 100, 160}}},
				{"text": "Page 2", "_pdf_positions": [][]float64{{2, 50, 300, 100, 160}}},
				{"text": "Page 3", "_pdf_positions": [][]float64{{3, 50, 300, 100, 160}}},
				{"text": "Page 4", "positions": [][]float64{{4, 50, 300, 100, 160}}},
				{"text": "Page 5", "positions": [][]float64{{5, 50, 300, 100, 160}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		fiveID, _ := outputChunks(t, outFive)[0]["id"].(string)
		fivePNG, err := storeFive.Get(ctxFive, "one-kb", fiveID)
		if err != nil {
			t.Fatal(err)
		}
		fiveImg, err := png.Decode(bytes.NewReader(fivePNG))
		if err != nil {
			t.Fatal(err)
		}
		cleanupFive()

		storeSeven, cleanupSeven := setupEnv(t, sevenPagePDF)
		t.Cleanup(cleanupSeven)
		ctxSeven := withIngestionGlobals(t, "one-kb", "doc-seven")
		outSeven, err := (&imageUploadDecorator{inner: comp}).Invoke(ctxSeven, nil, map[string]any{
			"name": "seven.pdf", "file_type": "pdf", "bucket": "one-kb", "path": "doc.pdf",
			"output_format": "json",
			"json": []map[string]any{
				{"text": "Page 1", "_pdf_positions": [][]float64{{1, 50, 300, 100, 160}}},
				{"text": "Page 2", "_pdf_positions": [][]float64{{2, 50, 300, 100, 160}}},
				{"text": "Page 3", "_pdf_positions": [][]float64{{3, 50, 300, 100, 160}}},
				{"text": "Page 4", "positions": [][]float64{{4, 50, 300, 100, 160}}},
				{"text": "Page 5", "positions": [][]float64{{5, 50, 300, 100, 160}}},
				{"text": "Page 6", "positions": [][]float64{{6, 50, 300, 100, 160}}},
				{"text": "Page 7", "_pdf_positions": [][]float64{{7, 50, 300, 100, 160}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		sevenChunks := outputChunks(t, outSeven)
		sevenID, _ := sevenChunks[0]["id"].(string)
		sevenPNG, err := storeSeven.Get(ctxSeven, "one-kb", sevenID)
		if err != nil {
			t.Fatal(err)
		}
		sevenImg, err := png.Decode(bytes.NewReader(sevenPNG))
		if err != nil {
			t.Fatal(err)
		}
		if sevenImg.Bounds().Dy() != fiveImg.Bounds().Dy() {
			t.Fatalf("7-page preview height = %d, want 5-page capped height %d", sevenImg.Bounds().Dy(), fiveImg.Bounds().Dy())
		}
		pdfRows, _ := sevenChunks[0]["_pdf_positions"].([][]float64)
		posRows, _ := sevenChunks[0]["positions"].([][]float64)
		if len(pdfRows) != 4 || pdfRows[3][0] != 7 {
			t.Fatalf("_pdf_positions = %v, want all 4 _pdf_positions rows including page 7", pdfRows)
		}
		if len(posRows) != 3 || posRows[2][0] != 6 {
			t.Fatalf("positions = %v, want all 3 positions rows through page 6", posRows)
		}
	})
}
