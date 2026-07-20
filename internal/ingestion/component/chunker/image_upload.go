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

// Chunk image upload: at the moment a chunk's raw image is produced, upload it
// to object storage, record the img_id reference, and drop the in-memory bytes.
// This bounds peak memory to a single chunk's image lifetime instead
// of carrying every image across the component boundary, matching the
// pdfcrop_cgo sliding-window philosophy. Mirrors Python image2id +
// upload_to_minio (rag/utils/base64_image.py, rag/svr/task_executor.py).
package chunker

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/globals"
)

// imageUploadSem caps concurrent object-storage uploads process-wide. Default
// 10, matching Python's minio_limiter (MAX_CONCURRENT_MINIO, see
// rag/svr/task_executor_limiter.py:22). Overridable via the same env var.
var imageUploadSem = make(chan struct{}, imageUploadConcurrency())

// ChunkImageUploader is the uploader used by the chunker's image-upload pass
// (the imageUploadDecorator). It defaults to component.DefaultImageUploader;
// tests and specialized runtimes override it (e.g. with a no-op uploader).
var ChunkImageUploader component.ImageUploader = component.DefaultImageUploader

func imageUploadConcurrency() int {
	if v := os.Getenv("MAX_CONCURRENT_MINIO"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 10
}

// uploadChunkImages is the caller-side image-upload pass for a chunker's
// output. For each chunk it mirrors Python's upload_to_minio + image2id,
// running at the chunker stage (not in the Tokenizer) so the bytes are
// dropped as soon as they are produced:
//   - img_id already set  → keep it, skip upload (Python bypass).
//   - no image            → img_id="", nothing to upload.
//   - otherwise           → decode the image, upload the bytes via
//     uploadOneImage at key=ck["id"], set img_id="<kb_id>-<chunk_id>", and
//     delete the raw image field (用完即弃).
//
// The caller (imageUploadDecorator) must write ck["id"] before calling this
// function. uploadChunkImage errors when a chunk arrives without id.
func uploadChunkImages(ctx context.Context, chunks []map[string]any, up component.ImageUploader, kbID string) error {
	for _, ck := range chunks {
		if err := uploadChunkImage(ctx, ck, up, kbID); err != nil {
			return err
		}
	}
	return nil
}

// uploadChunkImage handles a single chunk map: reads ck["id"] (required),
// decodes and uploads the image, then writes img_id and removes the raw image.
func uploadChunkImage(ctx context.Context, ck map[string]any, up component.ImageUploader, kbID string) error {
	if ck == nil {
		return nil
	}
	chunkID, _ := ck["id"].(string)
	if chunkID == "" {
		return fmt.Errorf("uploadChunkImage: chunk missing id")
	}
	if imgID, _ := ck["img_id"].(string); imgID != "" {
		return nil
	}
	img, _ := ck["image"].(string)
	if img == "" {
		ck["img_id"] = ""
		return nil
	}
	data, err := decodeChunkImage(img)
	if err != nil {
		return err
	}
	imgID, err := uploadOneImage(ctx, up, kbID, chunkID, data)
	if err != nil {
		return err
	}
	ck["img_id"] = imgID
	delete(ck, "image")
	return nil
}

// uploadOneImage is the pure upload primitive: it stores already-decoded image
// bytes at (bucket=kbID, key=chunkID) and returns the img_id reference
// "<kb_id>-<chunk_id>". It does NOT read or mutate any chunk map — the caller
// is responsible for decoding the bytes, writing img_id/id, and dropping the
// raw image field. The process-wide semaphore bounds concurrent uploads.
func uploadOneImage(ctx context.Context, up component.ImageUploader, kbID, chunkID string, data []byte) (string, error) {
	select {
	case imageUploadSem <- struct{}{}:
		defer func() { <-imageUploadSem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return up(ctx, kbID, chunkID, data)
}

// decodeChunkImage strips an optional data-URL prefix and base64-decodes the
// payload. Chunker image payloads are "data:image/...;base64,<b64>" (pdfcrop,
// markdown, docx). A bare base64 string is also accepted.
func decodeChunkImage(s string) ([]byte, error) {
	if i := strings.Index(s, ";base64,"); i >= 0 {
		s = s[i+len(";base64,"):]
	} else if strings.HasPrefix(s, "data:") {
		if i := strings.IndexByte(s, ','); i >= 0 {
			s = s[i+1:]
		}
	}
	return base64.StdEncoding.DecodeString(s)
}

// resolveImageUploadContext pulls kb_id / doc_id from the run-level globals,
// mirroring how other chunkers read shared metadata (e.g. `name`).
func resolveImageUploadContext(ctx context.Context, inputs map[string]any) (kbID, docID string) {
	kbID = globals.GlobalOrInput(ctx, inputs, "kb_id", "")
	docID = globals.GlobalOrInput(ctx, inputs, "doc_id", "")
	return kbID, docID
}
