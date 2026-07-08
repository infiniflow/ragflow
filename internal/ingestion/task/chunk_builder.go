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

package task

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cespare/xxhash/v2"
)

// PAGERANK_FLD is the field key for pagerank feature value on a chunk,
// matching Python common.constants.PAGERANK_FLD = "pagerank_fea".
const PAGERANK_FLD = "pagerank_fea"

// Chunk represents a single document chunk ready for embedding and indexing.
// Mirrors the Python dict format produced by ChunkService._prepare_docs_and_upload().
type Chunk struct {
	ID                string  `json:"id"`                   // xxhash64(content_with_weight + doc_id) hex
	DocID             string  `json:"doc_id"`               // document ID
	KBID              string  `json:"kb_id"`                // knowledge base ID
	DocNameKwd        string  `json:"docnm_kwd"`            // document file name
	ContentWithWeight string  `json:"content_with_weight"`  // main text content
	CreateTime        string  `json:"create_time"`          // "2006-01-02 15:04:05"
	CreateTimestamp   float64 `json:"create_timestamp_flt"` // Unix timestamp
	ImgID             string  `json:"img_id"`               // image ID (from MinIO or inline)
	PageNumInt        []int   `json:"page_num_int"`         // page numbers
	TopInt            []int   `json:"top_int"`              // top positions
	AvailableInt      int     `json:"available_int"`        // availability flag, default 1
	PageRank          int     `json:"pagerank_fea"`         // pagerank feature value, 0 = unset
}

// prepareDocs converts ParseWithDeepDoc output ([]map[string]any) to a list of
// Chunk structs. Matches Python ChunkService._prepare_docs_and_upload().
//
// Input sections come from ParseWithDeepDoc; each map may contain:
//   - "text"         (string)    — the section text → ContentWithWeight
//   - "doc_type_kwd" (string)    — document type hint (unused directly in chunk)
//   - "img_id"       (string)    — inline image ID (empty string if none)
//   - "positions"    ([]float64) — [pageNum, top, left, width, height]
//
// This is a pure function with no I/O side effects. Image upload to MinIO
// is handled separately by the caller.
func PrepareDocs(sections []map[string]any, docID, kbID, docName string, pagerank int) []Chunk {
	if len(sections) == 0 {
		return nil
	}

	now := time.Now()
	createTime := now.Format("2006-01-02 15:04:05")
	createTimestamp := float64(now.Unix())

	chunks := make([]Chunk, 0, len(sections))
	for _, sec := range sections {
		text, _ := sec["text"].(string)

		c := Chunk{
			DocID:             docID,
			KBID:              kbID,
			DocNameKwd:        docName,
			ContentWithWeight: text,
			CreateTime:        createTime,
			CreateTimestamp:   createTimestamp,
			AvailableInt:      1,
			PageRank:          pagerank,
		}

		// Extract img_id if present
		if imgID, ok := sec["img_id"].(string); ok {
			c.ImgID = imgID
		}

		// Extract positions: [pageNum, top, left, width, height]
		if positions, ok := sec["positions"].([]float64); ok && len(positions) >= 2 {
			c.PageNumInt = append(c.PageNumInt, int(positions[0]))
			c.TopInt = append(c.TopInt, int(positions[1]))
		}

		// Generate deterministic chunk ID: xxhash64(content_with_weight + doc_id) → hex
		c.ID = ChunkID(text, docID)
		chunks = append(chunks, c)
	}

	return chunks
}

// ChunkID generates a deterministic chunk identifier matching Python's:
//
//	xxhash.xxh64((content_with_weight + str(doc_id)).encode("utf-8", "surrogatepass")).hexdigest()
func ChunkID(contentWithWeight, docID string) string {
	return fmt.Sprintf("%016x", xxhash.Sum64String(contentWithWeight+docID))
}

// String returns a human-readable summary of the chunk for logging.
func (c *Chunk) String() string {
	preview := c.ContentWithWeight
	if utf8.RuneCountInString(preview) > 80 {
		preview = string([]rune(preview)[:80]) + "..."
	}
	return fmt.Sprintf("Chunk{id=%s doc=%s page=%v top=%v text=%q}",
		c.ID, c.DocID, c.PageNumInt, c.TopInt, preview)
}

// UploadChunkImages uploads base64-encoded images from sections to storage
// and sets each chunk's ImgID to the resulting storage reference.
//
// Matches Python ChunkService._prepare_docs_and_upload() image upload logic:
//   - If section has "img_id" pre-set → use it directly (skip upload)
//   - If section has "image" (base64) → decode → upload via putFn → set ImgID
//   - Otherwise → ImgID stays empty
//
// putFn is typically storage.Put(bucket, fnm, data) where bucket=kbID, fnm=chunkID.
// sections and chunks must correspond 1:1 by index.
func UploadChunkImages(sections []map[string]any, chunks []Chunk, kbID string, putFn func(bucket, fnm string, binary []byte) error) error {
	for i := range chunks {
		sec := sections[i]

		// Already has img_id from parser — use as-is
		if imgID, ok := sec["img_id"].(string); ok && imgID != "" {
			chunks[i].ImgID = imgID
			continue
		}

		// No image data — skip
		imgRaw, ok := sec["image"].(string)
		if !ok || imgRaw == "" {
			continue
		}

		imgBytes, err := decodeChunkImagePayload(imgRaw)
		if err != nil {
			return fmt.Errorf("decode image for chunk %s: %w", chunks[i].ID, err)
		}

		// Upload to storage (bucket=kbID, fnm=chunkID)
		if err := putFn(kbID, chunks[i].ID, imgBytes); err != nil {
			return fmt.Errorf("upload image for chunk %s: %w", chunks[i].ID, err)
		}

		// Set ImgID to storage reference (matching Python f"{bucket}-{objname}")
		chunks[i].ImgID = fmt.Sprintf("%s-%s", kbID, chunks[i].ID)
	}
	return nil
}

func decodeChunkImagePayload(raw string) ([]byte, error) {
	if idx := strings.Index(raw, ","); strings.HasPrefix(raw, "data:image/") && idx >= 0 {
		raw = raw[idx+1:]
	}
	return base64.StdEncoding.DecodeString(raw)
}
