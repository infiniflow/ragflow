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

package component

import (
	"context"
	"fmt"
)

// ImageUploader stores a chunk's raw image bytes and returns the img_id
// reference to persist in the index. It is the write-side counterpart to
// FetchBinary: injected into the chunker so the chunker can upload cropped /
// extracted images on the spot and drop the bytes, instead of carrying them
// across the component boundary.
type ImageUploader func(ctx context.Context, kbID, chunkID string, data []byte) (imgID string, err error)

// DefaultImageUploader stores image bytes at (bucket=kbID, key=chunkID) and
// returns img_id "<kb_id>-<chunk_id>". Mirrors Python image2id's storage_put +
// f"{bucket}-{objname}" (rag/utils/base64_image.py:80-82), minus re-encoding:
// the bytes are stored in whatever format the chunker produced.
func DefaultImageUploader(_ context.Context, kbID, chunkID string, data []byte) (string, error) {
	stg := resolveStorage()
	if stg == nil {
		return "", fmt.Errorf("no storage backend registered")
	}
	if err := stg.Put(kbID, chunkID, data); err != nil {
		return "", fmt.Errorf("store chunk image (%q,%q): %w", kbID, chunkID, err)
	}
	return kbID + "-" + chunkID, nil
}
