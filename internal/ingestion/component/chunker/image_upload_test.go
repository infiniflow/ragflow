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
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
)

const (
	testKBID  = "kb-1"
	testDocID = "doc-1"
)

// pngBase64 is a tiny 1x1 PNG payload (raw base64, no data-URL prefix).
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

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
// uploadChunkImages reads ck["id"] (already written by the decorator prior to
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

	if err := uploadChunkImages(context.Background(), []map[string]any{ck}, up, testKBID); err != nil {
		t.Fatalf("uploadChunkImages: %v", err)
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
	if err := uploadChunkImages(context.Background(), []map[string]any{ck}, up, testKBID); err != nil {
		t.Fatalf("uploadChunkImages: %v", err)
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
	if err := uploadChunkImages(context.Background(), []map[string]any{ck}, up, testKBID); err != nil {
		t.Fatalf("uploadChunkImages: %v", err)
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
	err := uploadChunkImages(context.Background(), []map[string]any{ck}, up, testKBID)
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
			{"content_with_weight": "a cropped figure", "image": "data:image/png;base64," + pngBase64},
		},
	}
	out, err := decorated.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
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
