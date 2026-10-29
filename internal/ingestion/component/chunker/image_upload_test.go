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
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, or express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package chunker

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	medialib "ragflow/internal/parser/media"
	parserlib "ragflow/internal/parser/parser"

	"gorm.io/gorm"
)

const (
	testKBID  = "kb-1"
	testDocID = "doc-1"
)

// pngBase64 is a tiny 1x1 PNG payload (raw base64, no data-URL prefix).
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestImageUploadDecorator_ParserUnpaddedImages(t *testing.T) {
	want, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"html", "markdown"} {
		t.Run(format, func(t *testing.T) {
			// The same PNG with its optional base64 padding omitted.
			uri := "data:image/png;base64," + pngBase64[:len(pngBase64)-2]
			var parsed parserlib.ParseResult
			ctx := t.Context()
			if format == "html" {
				source := []byte(`<img alt="caption" src="` + uri + `">`)
				cache := &chunkerMediaCache{}
				ctx = medialib.WithCache(ctx, cache)
				parsed = parserlib.NewHTMLParser().ParseWithResult(ctx, "image.html", source)
			} else {
				p, err := parserlib.NewMarkdownParser(parserlib.GoMarkdown)
				if err != nil {
					t.Fatal(err)
				}
				parsed = p.ParseWithResult(t.Context(), "image.md", []byte("![caption]("+uri+")"))
			}
			if parsed.Err != nil {
				t.Fatal(parsed.Err)
			}
			var uploaded []byte
			prev := ChunkImageUploader
			ChunkImageUploader = func(_ context.Context, kbID, chunkID string, data []byte) (string, error) {
				uploaded = append([]byte(nil), data...)
				return kbID + "-" + chunkID, nil
			}
			t.Cleanup(func() { ChunkImageUploader = prev })
			comp, err := NewOneChunker(nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
				"name": "image." + format, "kb_id": testKBID, "doc_id": testDocID,
				"output_format": "json", "json": parsed.JSON,
			})
			if err != nil {
				t.Fatalf("parser image failed at chunk upload: %v", err)
			}
			if !bytes.Equal(uploaded, want) {
				t.Fatalf("uploaded bytes differ from original PNG: got %d bytes, want %d", len(uploaded), len(want))
			}
			chunks, ok := out["chunks"].([]map[string]any)
			if !ok || len(chunks) != 1 {
				t.Fatalf("expected one chunk, got %#v", out)
			}
			if chunks[0]["img_id"] != testKBID+"-"+common.ChunkID(testDocID, "caption") {
				t.Errorf("unexpected image reference: %v", chunks[0]["img_id"])
			}
			if _, ok := chunks[0]["image"]; ok {
				t.Error("raw image retained after upload")
			}
		})
	}
}

// TestUploadOneImage_UploadsBytes pins the pure upload contract: it receives
// already-decoded bytes (not the raw chunk image field) plus the chunkID, and
// returns the img_id — it does NOT touch any chunk map.
func TestUploadOneImage_UploadsBytes(t *testing.T) {
	var gotBucket, gotKey string
	var gotData []byte
	up := func(_ context.Context, kbID, chunkID string, data []byte) (string, error) {
		gotBucket, gotKey, gotData = kbID, chunkID, data
		return fmt.Sprintf("%s-%s", kbID, chunkID), nil
	}
	chunkID := common.ChunkID(testDocID, "caption")
	data, _ := base64.StdEncoding.DecodeString(pngBase64)

	imgID, err := uploadOneImage(context.Background(), up, testKBID, chunkID, data)
	if err != nil {
		t.Fatalf("uploadOneImage: %v", err)
	}
	if imgID != testKBID+"-"+chunkID {
		t.Errorf("img_id = %q, want %q", imgID, testKBID+"-"+chunkID)
	}
	if gotBucket != testKBID || gotKey != chunkID {
		t.Errorf("upload target = (%q,%q), want (%q,%q)", gotBucket, gotKey, testKBID, chunkID)
	}
	if string(gotData) != string(data) {
		t.Errorf("uploaded bytes mismatch")
	}
}

// TestUploadChunkImages_WritesImgIDAndDropsImage pins the caller-side contract:
// uploadChunkMedia reads ck["id"] (already written by the decorator prior to
// calling upload), decodes the chunk's image, uploads via uploadOneImage, then
// writes ck["img_id"] and deletes the raw image field.
func TestUploadChunkImages_WritesImgIDAndDropsImage(t *testing.T) {
	var gotKey string
	up := func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		gotKey = chunkID
		return kbID + "-" + chunkID, nil
	}
	chunkID := common.ChunkID(testDocID, "caption")
	ck := map[string]any{"id": chunkID, "content_with_weight": "caption", "image": "data:image/png;base64," + pngBase64}

	if _, err := uploadChunkMedia(context.Background(), []map[string]any{ck}, up, testKBID, testDocID); err != nil {
		t.Fatalf("uploadChunkMedia: %v", err)
	}
	wantImgID := testKBID + "-" + chunkID
	if ck["img_id"] != wantImgID {
		t.Errorf("img_id = %v, want %q", ck["img_id"], wantImgID)
	}
	if ck["id"] != chunkID {
		t.Errorf("id = %v, want %q", ck["id"], chunkID)
	}
	if _, ok := ck["image"]; ok {
		t.Errorf("image field not deleted: %v", ck["image"])
	}
	if gotKey != chunkID {
		t.Errorf("upload key = %q, want %q", gotKey, chunkID)
	}
}

// TestUploadChunkImages_SkipsWhenImgIDPresent mirrors Python's
// `if d.get("img_id"): return` bypass — a chunk that already carries an
// img_id is left untouched and no upload happens.
func TestUploadChunkImages_SkipsWhenImgIDPresent(t *testing.T) {
	called := false
	up := func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		called = true
		return "should-not-be-used", nil
	}
	chunkID := common.ChunkID(testDocID, "x")
	ck := map[string]any{"id": chunkID, "content_with_weight": "x", "img_id": "preset-img", "image": "data:image/png;base64," + pngBase64}
	if _, err := uploadChunkMedia(context.Background(), []map[string]any{ck}, up, testKBID, testDocID); err != nil {
		t.Fatalf("uploadChunkMedia: %v", err)
	}
	if called {
		t.Error("uploader was called; expected skip when img_id preset")
	}
	if ck["img_id"] != "preset-img" {
		t.Errorf("img_id = %v, want preserved %q", ck["img_id"], "preset-img")
	}
	if ck["id"] != chunkID {
		t.Errorf("id = %v, want preserved %q", ck["id"], chunkID)
	}
}

// TestUploadChunkImages_NoImage: a chunk with no image gets img_id="" and no
// upload (Python: img_id="" branch). The id is preserved.
func TestUploadChunkImages_NoImage(t *testing.T) {
	called := false
	up := func(_ context.Context, _, _ string, _ []byte) (string, error) {
		called = true
		return "", nil
	}
	chunkID := common.ChunkID(testDocID, "text only")
	ck := map[string]any{"id": chunkID, "content_with_weight": "text only"}
	if _, err := uploadChunkMedia(context.Background(), []map[string]any{ck}, up, testKBID, testDocID); err != nil {
		t.Fatalf("uploadChunkMedia: %v", err)
	}
	if called {
		t.Error("uploader was called for a chunk without image")
	}
	if ck["img_id"] != "" {
		t.Errorf("img_id = %v, want empty", ck["img_id"])
	}
	if ck["id"] != chunkID {
		t.Errorf("id = %v, want preserved %q", ck["id"], chunkID)
	}
}

// TestUploadChunkImage_ErrorOnMissingID verifies that uploadChunkImage errors
// when ck["id"] is absent — the caller (decorator) must always set id first.
func TestUploadChunkImage_ErrorOnMissingID(t *testing.T) {
	up := func(_ context.Context, _, _ string, _ []byte) (string, error) {
		return "should-not-be-called", nil
	}
	ck := map[string]any{"content_with_weight": "x", "image": "data:image/png;base64," + pngBase64}
	_, err := uploadChunkMedia(context.Background(), []map[string]any{ck}, up, testKBID, testDocID)
	if err == nil {
		t.Fatal("expected error when chunk missing id")
	}
}

// TestUploadOneImage_ConcurrencyLimit verifies the process-wide semaphore caps
// concurrent uploads at the configured limit.
func TestUploadOneImage_ConcurrencyLimit(t *testing.T) {
	const limit = 3
	setImageUploadConcurrencyForTest(t, limit)

	var inflight, maxSeen int64
	release := make(chan struct{})
	up := func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		cur := atomic.AddInt64(&inflight, 1)
		for {
			m := atomic.LoadInt64(&maxSeen)
			if cur <= m || atomic.CompareAndSwapInt64(&maxSeen, m, cur) {
				break
			}
		}
		<-release
		atomic.AddInt64(&inflight, -1)
		return fmt.Sprintf("%s-%s", kbID, chunkID), nil
	}

	const n = 12
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			data, _ := base64.StdEncoding.DecodeString(pngBase64)
			chunkID := common.ChunkID(testDocID, fmt.Sprintf("c%d", i))
			_, _ = uploadOneImage(context.Background(), up, testKBID, chunkID, data)
		}(i)
	}
	waitForInflight(t, &inflight, limit)
	close(release)
	wg.Wait()

	if maxSeen > limit {
		t.Errorf("max concurrent uploads = %d, want <= %d", maxSeen, limit)
	}
	if maxSeen < limit {
		t.Errorf("max concurrent uploads = %d, expected to reach limit %d", maxSeen, limit)
	}
}

// TestImageUploadDecorator_EndToEnd drives a real OneChunker through the
// registration decorator and asserts the produced chunk was uploaded (img_id
// and id set, image dropped). Uses the ChunkImageUploader override seam so no
// real storage backend is needed.
func TestImageUploadDecorator_EndToEnd(t *testing.T) {
	var gotKB, gotKey string
	prev := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		gotKB, gotKey = kbID, chunkID
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prev })

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatalf("NewOneChunker: %v", err)
	}
	decorated := &imageUploadDecorator{inner: comp}

	inputs := map[string]any{
		"name":   "doc.pdf",
		"kb_id":  testKBID,
		"doc_id": testDocID,
		"chunks": []map[string]any{
			{"text": "a cropped figure", "image": "data:image/png;base64," + pngBase64},
		},
	}
	out, err := decorated.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok || len(chunks) == 0 {
		t.Fatalf("no chunks in output")
	}
	ck := chunks[0]
	wantChunkID := common.ChunkID(testDocID, "a cropped figure")
	if ck["img_id"] != testKBID+"-"+wantChunkID {
		t.Errorf("img_id = %v, want %q", ck["img_id"], testKBID+"-"+wantChunkID)
	}
	if ck["id"] != wantChunkID {
		t.Errorf("id = %v, want chunk id %q written back", ck["id"], wantChunkID)
	}
	if _, ok := ck["image"]; ok {
		t.Errorf("image not dropped: %v", ck["image"])
	}
	if gotKB != testKBID || gotKey != wantChunkID {
		t.Errorf("upload target = (%q,%q), want (%q,%q)", gotKB, gotKey, testKBID, wantChunkID)
	}
}

// setImageUploadConcurrencyForTest swaps the process-wide upload semaphore to
// `n` slots for the duration of the test, restoring the original after.
func setImageUploadConcurrencyForTest(t *testing.T, n int) {
	t.Helper()
	prev := imageUploadSem
	imageUploadSem = make(chan struct{}, n)
	t.Cleanup(func() { imageUploadSem = prev })
}

// waitForInflight blocks until the atomic counter reaches `target` or fails.
func waitForInflight(t *testing.T, counter *int64, target int64) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt64(counter) >= target {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d inflight uploads (got %d)", target, atomic.LoadInt64(counter))
		case <-time.After(time.Millisecond):
		}
	}
}

// TestImageUploadDecorator_DebugSkipsUpload verifies that a debug/dry-run run
// (empty kb_id) does NOT upload the chunk image to storage, while still
// dropping the raw image bytes. This keeps the debug path free of MinIO side
// effects and preserves the current memory behaviour (raw bytes discarded, not
// held until a persist stage). An empty kb_id only occurs in canvas debug
// (dry-run) mode; production ingestion always supplies a KB.
func TestImageUploadDecorator_DebugSkipsUpload(t *testing.T) {
	prev := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, _, _ string, _ []byte) (string, error) {
		t.Fatalf("ChunkImageUploader must not be called in a debug run")
		return "", nil
	}
	t.Cleanup(func() { ChunkImageUploader = prev })

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatalf("NewOneChunker: %v", err)
	}
	decorated := &imageUploadDecorator{inner: comp}

	inputs := map[string]any{
		"name":   "doc.pdf",
		"kb_id":  "", // debug mode: no KB -> no upload
		"doc_id": testDocID,
		"chunks": []map[string]any{
			{"text": "a cropped figure", "image": "data:image/png;base64," + pngBase64},
		},
	}
	out, err := decorated.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok || len(chunks) == 0 {
		t.Fatalf("expected chunks in output, got %#v", out)
	}
	ck := chunks[0]
	if _, stillHas := ck["image"]; stillHas {
		t.Errorf("raw image should be dropped in debug run, but ck[\"image\"] is still present")
	}
	if id, _ := ck["img_id"].(string); id != "" {
		t.Errorf("img_id should not be set in debug run, got %q", id)
	}
}

// stubChunker is a deterministic fake runtime.Component used to exercise the
// imageUploadDecorator without depending on a real chunker variant's slicing
// behaviour. It returns exactly the chunks it was constructed with.
type stubChunker struct {
	chunks []map[string]any
}

func (s *stubChunker) Invoke(_ context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	return map[string]any{"chunks": s.chunks}, nil
}

// TestImageUploadDecorator_DebugCapsChunks pins the canvas-debug chunk cap:
// when CanvasState.Globals carries debug_chunk_cap (>=1), the decorator
// truncates the chunker output to the leading N chunks for preview. This is
// the single choke point that every chunker variant flows through, so the cap
// applies regardless of which chunker variant produced the chunks. The test
// drives a stub chunker (deterministic output) through the decorator with a
// ctx that has the cap set, and asserts only the first N chunks survive.
func TestImageUploadDecorator_DebugCapsChunks(t *testing.T) {
	const capN = 3
	src := make([]map[string]any, 0, 6)
	for i := 0; i < 6; i++ {
		src = append(src, map[string]any{
			"text": fmt.Sprintf("chunk-%d", i),
		})
	}
	decorated := &imageUploadDecorator{inner: &stubChunker{chunks: src}}

	inputs := map[string]any{
		"name":   "doc.pdf",
		"kb_id":  "", // debug run
		"doc_id": testDocID,
	}

	st := &runtime.CanvasState{Globals: map[string]any{globals.DebugChunkCapKey: capN}}
	ctx := runtime.WithState(context.Background(), st)

	out, err := decorated.Invoke(ctx, nil, inputs)
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok {
		t.Fatalf("chunks missing or wrong type: %#v", out["chunks"])
	}
	if len(chunks) != capN {
		t.Fatalf("len(chunks) = %d, want %d (debug chunk cap not applied)", len(chunks), capN)
	}
	for i, ck := range chunks {
		want := fmt.Sprintf("chunk-%d", i)
		if got, _ := ck["text"].(string); got != want {
			t.Errorf("chunks[%d].text = %q, want %q (wrong chunk kept after truncation)", i, got, want)
		}
	}
}

// TestImageUploadDecorator_NoCapKeepsAll asserts that without debug_chunk_cap
// in globals the decorator leaves every chunk intact — the cap is opt-in via
// the global, so persist runs and chunker-less debug runs are untouched.
func TestImageUploadDecorator_NoCapKeepsAll(t *testing.T) {
	src := make([]map[string]any, 0, 5)
	for i := 0; i < 5; i++ {
		src = append(src, map[string]any{
			"text": fmt.Sprintf("chunk-%d", i),
		})
	}
	decorated := &imageUploadDecorator{inner: &stubChunker{chunks: src}}

	inputs := map[string]any{
		"name":   "doc.pdf",
		"kb_id":  "",
		"doc_id": testDocID,
	}
	// No CanvasState in ctx → DebugChunkCap returns 0 → no truncation.
	out, err := decorated.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok || len(chunks) != 5 {
		t.Fatalf("len(chunks) = %d, want 5 (no cap should keep all)", len(chunks))
	}
}

// TestImageUploadDecorator_ResolvesMediaRef pins the ref-mode path: the parser
// writes decoded bytes to the run cache, emits only a reference, and the upload
// pass materializes that reference without rescanning the source document.
func TestImageUploadDecorator_ResolvesMediaRef(t *testing.T) {
	want, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + pngBase64
	html := `<img src="` + uri + `">`

	p := parserlib.NewHTMLParser()
	cache := &chunkerMediaCache{}
	ctx := medialib.WithCache(t.Context(), cache)
	parsed := p.ParseWithResult(ctx, "image.html", []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	ref := entity.MediaRefFromAny(parsed.JSON[0].ToMap()["media_ref"])
	if ref == nil {
		t.Fatal("parser emitted no media_ref")
	}

	var uploaded []byte
	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, data []byte) (string, error) {
		uploaded = append([]byte(nil), data...)
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
		"name": "image.html", "kb_id": testKBID, "doc_id": testDocID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	wantID := common.ChunkID(testDocID, mediaChunkIdentity("", "", ref))
	if chunks[0]["id"] != wantID {
		t.Errorf("id = %v, want %q", chunks[0]["id"], wantID)
	}
	if chunks[0]["img_id"] != testKBID+"-"+wantID {
		t.Errorf("img_id = %v, want %q", chunks[0]["img_id"], testKBID+"-"+wantID)
	}
	if !bytes.Equal(uploaded, want) {
		t.Errorf("uploaded %d bytes, want the source image", len(uploaded))
	}
	if _, ok := chunks[0]["media_ref"]; ok {
		t.Error("media_ref retained after upload")
	}
	if _, ok := chunks[0]["image"]; ok {
		t.Error("image bytes retained after upload")
	}
}

// TestImageUploadDecoratorFailsOnCacheMiss pins that a valid parser reference
// must remain materializable at upload time. A cache miss is a task failure,
// including for a captionless image that would otherwise look droppable.
func TestImageUploadDecoratorFailsOnCacheMiss(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64

	p := parserlib.NewHTMLParser()
	cache := &chunkerMediaCache{}
	ctx := medialib.WithCache(t.Context(), cache)
	parsed := p.ParseWithResult(ctx, "image.html", []byte(`<img src="`+uri+`">`))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	cache.getErr = medialib.ErrMediaNotFound

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
		"name": "image.html", "kb_id": testKBID, "doc_id": testDocID,
		"output_format": "json", "json": parsed.JSON,
	})
	if !errors.Is(err, medialib.ErrMediaNotFound) {
		t.Fatalf("decorator error = %v, want ErrMediaNotFound", err)
	}
}

// TestImageUploadDecoratorFailsOnCacheReadError pins that a backend failure is
// not converted into a missing-image skip.
func TestImageUploadDecoratorFailsOnCacheReadError(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64

	p := parserlib.NewHTMLParser()
	cache := &chunkerMediaCache{}
	ctx := medialib.WithCache(t.Context(), cache)
	parsed := p.ParseWithResult(ctx, "image.html", []byte(`<img src="`+uri+`">`))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	backendErr := errors.New("kvrocks unavailable")
	cache.getErr = backendErr

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
		"name": "image.html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	}); !errors.Is(err, backendErr) {
		t.Fatalf("decorator error = %v, want backend failure", err)
	}
}

// TestUploadChunkMediaSkipsMaterializationWhenImgIDExists pins the bypass:
// an already-uploaded image must not be reconstructed, and the redundant
// inline copy must not survive into the index.
func TestUploadChunkMediaSkipsMaterializationWhenImgIDExists(t *testing.T) {
	ref := entity.MediaRef{
		Format: "html",
		Kind:   entity.MediaKindKV,
		Scope:  "run-1",
		SHA256: strings.Repeat("a", 64),
		MIME:   "image/png",
		Size:   5,
	}
	ck := map[string]any{
		"id":           "chunk-1",
		"text":         "",
		"doc_type_kwd": "image",
		"img_id":       testKBID + "-chunk-1",
		"image":        "data:image/png;base64," + pngBase64,
		"media_ref":    ref,
	}
	up := func(context.Context, string, string, []byte) (string, error) {
		t.Fatal("upload must not run when img_id exists")
		return "", nil
	}
	kept, err := uploadChunkMedia(t.Context(), []map[string]any{ck}, up, testKBID, testDocID)
	if err != nil {
		t.Fatalf("uploadChunkMedia: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept = %d, want 1", len(kept))
	}
	if _, ok := ck["image"]; ok {
		t.Error("redundant image survived the img_id bypass")
	}
	if _, ok := ck["media_ref"]; ok {
		t.Error("media_ref survived the img_id bypass")
	}
	if ck["img_id"] != testKBID+"-chunk-1" {
		t.Errorf("img_id = %v", ck["img_id"])
	}
}

// TestUploadChunkMediaRejectsInvalidMediaRef pins the malformed-reference
// path: a present ref that does not decode must fail the run, not persist a
// chunk without its image.
func TestUploadChunkMediaRejectsInvalidMediaRef(t *testing.T) {
	ck := map[string]any{
		"id":           "chunk-1",
		"text":         "",
		"doc_type_kwd": "image",
		"media_ref":    map[string]any{"format": "html"},
	}
	up := func(context.Context, string, string, []byte) (string, error) { return "", nil }
	if _, err := uploadChunkMedia(t.Context(), []map[string]any{ck}, up, testKBID, testDocID); err == nil {
		t.Fatal("invalid media_ref was treated as absent")
	}
}
