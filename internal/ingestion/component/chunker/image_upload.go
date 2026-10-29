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

// Chunk image upload: materialize deferred media from the run cache, upload it
// to object storage, record the img_id reference, and drop the in-memory bytes.
// This bounds peak memory to a single chunk's image lifetime instead of
// carrying every image across component boundaries.
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

// uploadChunkMedia is the caller-side image pass for a chunker's output. It
// runs at the chunker stage (not in the Tokenizer) so bytes are dropped as soon
// as they are uploaded:
//   - img_id already set → keep it, skip upload (Python bypass).
//   - no image or ref    → img_id="", nothing to upload.
//   - media_ref only     → materialize one image from the run media cache.
//   - otherwise          → decode the image, upload the bytes via
//     uploadOneImage at key=ck["id"], set img_id="<kb_id>-<chunk_id>", and
//     delete the raw image field (用完即弃).
//
// Materializing inside the loop keeps peak memory at one image instead of the
// whole document. Every cache, integrity, scope, or transport failure aborts
// the invocation; an image is never silently dropped.
//
// A chunk that already carries img_id skips resolution entirely: the image is
// already stored, so a reconstructed copy would only leak into the index.
//
// The caller (imageUploadDecorator) must write ck["id"] before calling this
// function.
func uploadChunkMedia(ctx context.Context, chunks []map[string]any, up component.ImageUploader, kbID, docID string) ([]map[string]any, error) {
	for _, ck := range chunks {
		imgID, _ := ck["img_id"].(string)
		if imgID != "" {
			// The image was already uploaded. A materialized copy would not
			// be uploaded again, so it must not survive into the index.
			delete(ck, "image")
			delete(ck, "media_ref")
			continue
		}

		image, _ := ck["image"].(string)
		if image == "" {
			data, ref, err := component.ResolveMediaRef(ctx, ck)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return nil, ctxErr
				}
				return nil, fmt.Errorf("materialize media for document %s: %w", docID, err)
			}
			if ref != nil {
				chunkID, _ := ck["id"].(string)
				if chunkID == "" {
					return nil, fmt.Errorf("uploadChunkMedia: chunk missing id")
				}
				imgID, err := uploadOneImage(ctx, up, kbID, chunkID, data)
				if err != nil {
					return nil, err
				}
				ck["img_id"] = imgID
				delete(ck, "image")
				delete(ck, "media_ref")
				continue
			}
		}
		if err := uploadChunkImage(ctx, ck, up, kbID); err != nil {
			return nil, err
		}
		delete(ck, "media_ref")
	}
	return chunks, nil
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
// Markdown, docx). A bare base64 string is also accepted.
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
