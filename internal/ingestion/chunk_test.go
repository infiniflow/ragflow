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

package ingestion

import (
	"encoding/base64"
	"errors"
	"regexp"
	"testing"
)

const (
	testDocID  = "doc-1"
	testKBID   = "kb-1"
	testDocName = "test.pdf"
)

func TestPrepareDocs_Basic(t *testing.T) {
	sections := []map[string]any{
		{"text": "Hello World", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	c := chunks[0]
	if c.ContentWithWeight != "Hello World" {
		t.Errorf("ContentWithWeight = %q, want %q", c.ContentWithWeight, "Hello World")
	}
	if c.DocID != testDocID {
		t.Errorf("DocID = %q, want %q", c.DocID, testDocID)
	}
	if c.KBID != testKBID {
		t.Errorf("KBID = %q, want %q", c.KBID, testKBID)
	}
	if c.DocNameKwd != testDocName {
		t.Errorf("DocNameKwd = %q, want %q", c.DocNameKwd, testDocName)
	}
	if c.AvailableInt != 1 {
		t.Errorf("AvailableInt = %d, want 1", c.AvailableInt)
	}
}

func TestPrepareDocs_Multiple(t *testing.T) {
	sections := []map[string]any{
		{"text": "First", "doc_type_kwd": "text", "img_id": ""},
		{"text": "Second", "doc_type_kwd": "text", "img_id": ""},
		{"text": "Third", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// All IDs should be different (different content)
	ids := make(map[string]bool)
	for _, c := range chunks {
		if c.ContentWithWeight == "" {
			t.Error("chunk has empty ContentWithWeight")
		}
		ids[c.ID] = true
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs, got %d", len(ids))
	}
}

func TestPrepareDocs_EmptyInput(t *testing.T) {
	chunks := prepareDocs(nil, testDocID, testKBID, testDocName, 0)
	if chunks != nil {
		t.Errorf("expected nil for empty input, got %d chunks", len(chunks))
	}

	chunks = prepareDocs([]map[string]any{}, testDocID, testKBID, testDocName, 0)
	if chunks != nil {
		t.Errorf("expected nil for empty input, got %d chunks", len(chunks))
	}
}

func TestPrepareDocs_IDFormat(t *testing.T) {
	sections := []map[string]any{
		{"text": "Content for ID test", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

id := chunks[0].ID
	// xxhash64 hex digest is 16 hex chars (64 bits = 16 hex digits)
	matched, _ := regexp.MatchString(`^[0-9a-f]{1,16}$`, id)
	if !matched {
		t.Errorf("ID %q does not match hex format", id)
	}
	if len(id) == 0 {
		t.Error("ID is empty")
	}
}

func TestPrepareDocs_IDDeterministic(t *testing.T) {
	sections := []map[string]any{
		{"text": "Deterministic test content", "doc_type_kwd": "text", "img_id": ""},
	}

	chunks1 := prepareDocs(sections, testDocID, testKBID, testDocName, 0)
	chunks2 := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

if chunks1[0].ID != chunks2[0].ID {
		t.Errorf("ID not deterministic: %q vs %q", chunks1[0].ID, chunks2[0].ID)
	}

	// Different docID should produce different ID
	chunks3 := prepareDocs(sections, "doc-2", testKBID, testDocName, 0)
	if chunks1[0].ID == chunks3[0].ID {
		t.Error("different docIDs produced the same chunk ID")
	}
}

func TestPrepareDocs_Timestamps(t *testing.T) {
	sections := []map[string]any{
		{"text": "Timestamp test", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

c := chunks[0]

	// CreateTime should match the expected format
	matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`, c.CreateTime)
	if !matched {
		t.Errorf("CreateTime %q does not match format YYYY-MM-DD HH:MM:SS", c.CreateTime)
	}

	if c.CreateTimestamp <= 0 {
		t.Errorf("CreateTimestamp = %f, want > 0", c.CreateTimestamp)
	}
}

func TestPrepareDocs_Positions(t *testing.T) {
	sections := []map[string]any{
		{
			"text":      "Positioned content",
			"doc_type_kwd": "text",
			"img_id":    "",
			"positions": []float64{3, 150.5, 20, 400, 60},
		},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

c := chunks[0]
	if len(c.PageNumInt) != 1 || c.PageNumInt[0] != 3 {
		t.Errorf("PageNumInt = %v, want [3]", c.PageNumInt)
	}
	if len(c.TopInt) != 1 || c.TopInt[0] != 150 {
		t.Errorf("TopInt = %v, want [150]", c.TopInt)
	}
}

func TestPrepareDocs_NoPositions(t *testing.T) {
	sections := []map[string]any{
		{"text": "No positions", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

c := chunks[0]
	if c.PageNumInt != nil {
		t.Errorf("PageNumInt = %v, want nil", c.PageNumInt)
	}
	if c.TopInt != nil {
		t.Errorf("TopInt = %v, want nil", c.TopInt)
	}
}

func TestPrepareDocs_ImgID(t *testing.T) {
	sections := []map[string]any{
		{
			"text":      "Content with image",
			"doc_type_kwd": "text",
			"img_id":    "some_image_id_123",
		},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

c := chunks[0]
	if c.ImgID != "some_image_id_123" {
		t.Errorf("ImgID = %q, want %q", c.ImgID, "some_image_id_123")
	}
}

func TestPrepareDocs_EmptyImgID(t *testing.T) {
	sections := []map[string]any{
		{"text": "No image", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

c := chunks[0]
	if c.ImgID != "" {
		t.Errorf("ImgID = %q, want empty string", c.ImgID)
	}
}

func TestPrepareDocs_MixedContent(t *testing.T) {
	sections := []map[string]any{
		{"text": "Page 1 content", "doc_type_kwd": "text", "img_id": "",
			"positions": []float64{1, 0, 0, 100, 50}},
		{"text": "Page 2 with image", "doc_type_kwd": "text", "img_id": "img_abc",
			"positions": []float64{2, 30, 10, 200, 80}},
		{"text": "No position, no image", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	// First: has position
	if len(chunks[0].PageNumInt) != 1 || chunks[0].PageNumInt[0] != 1 {
		t.Errorf("chunk[0] PageNumInt = %v, want [1]", chunks[0].PageNumInt)
	}

	// Second: has position + img_id
	if chunks[1].ImgID != "img_abc" {
		t.Errorf("chunk[1] ImgID = %q, want %q", chunks[1].ImgID, "img_abc")
	}
	if len(chunks[1].PageNumInt) != 1 || chunks[1].PageNumInt[0] != 2 {
		t.Errorf("chunk[1] PageNumInt = %v, want [2]", chunks[1].PageNumInt)
	}

	// Third: no position, no image
	if chunks[2].PageNumInt != nil {
		t.Errorf("chunk[2] PageNumInt should be nil, got %v", chunks[2].PageNumInt)
	}
	if chunks[2].ImgID != "" {
		t.Errorf("chunk[2] ImgID should be empty, got %q", chunks[2].ImgID)
	}
}

func TestPrepareDocs_PageRank(t *testing.T) {
	sections := []map[string]any{
		{"text": "Ranked content", "doc_type_kwd": "text", "img_id": ""},
	}

	// pagerank=0 → PageRank should be 0 (unset)
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)
	if chunks[0].PageRank != 0 {
		t.Errorf("PageRank = %d, want 0", chunks[0].PageRank)
	}

	// pagerank=5 → PageRank should be 5
	chunks = prepareDocs(sections, testDocID, testKBID, testDocName, 5)
	if chunks[0].PageRank != 5 {
		t.Errorf("PageRank = %d, want 5", chunks[0].PageRank)
	}
}

func TestUploadChunkImages_WithImage(t *testing.T) {
	imgBase64 := base64.StdEncoding.EncodeToString([]byte("fake image"))
	sections := []map[string]any{
		{"text": "Has image", "doc_type_kwd": "text", "img_id": "", "image": imgBase64},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	var putBucket, putFnm string
	putFn := func(bucket, fnm string, binary []byte) error {
		putBucket = bucket
		putFnm = fnm
		if len(binary) == 0 {
			t.Error("Put called with empty binary")
		}
		return nil
	}

	if err := UploadChunkImages(sections, chunks, testKBID, putFn); err != nil {
		t.Fatalf("UploadChunkImages failed: %v", err)
	}

	expectedImgID := testKBID + "-" + chunks[0].ID
	if chunks[0].ImgID != expectedImgID {
		t.Errorf("ImgID = %q, want %q", chunks[0].ImgID, expectedImgID)
	}
	if putBucket != testKBID {
		t.Errorf("put bucket = %q, want %q", putBucket, testKBID)
	}
	if putFnm != chunks[0].ID {
		t.Errorf("put fnm = %q, want %q", putFnm, chunks[0].ID)
	}
}

func TestUploadChunkImages_PresetImgID(t *testing.T) {
	sections := []map[string]any{
		{"text": "Has preset img_id", "doc_type_kwd": "text", "img_id": "preset-id-123", "image": "should-be-ignored"},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	callCount := 0
	putFn := func(bucket, fnm string, binary []byte) error {
		callCount++
		return nil
	}

	if err := UploadChunkImages(sections, chunks, testKBID, putFn); err != nil {
		t.Fatalf("UploadChunkImages failed: %v", err)
	}

	if chunks[0].ImgID != "preset-id-123" {
		t.Errorf("ImgID = %q, want %q", chunks[0].ImgID, "preset-id-123")
	}
	if callCount != 0 {
		t.Errorf("expected 0 Put calls (preset img_id), got %d", callCount)
	}
}

func TestUploadChunkImages_NoImage(t *testing.T) {
	sections := []map[string]any{
		{"text": "No image", "doc_type_kwd": "text", "img_id": ""},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	callCount := 0
	putFn := func(bucket, fnm string, binary []byte) error {
		callCount++
		return nil
	}

	if err := UploadChunkImages(sections, chunks, testKBID, putFn); err != nil {
		t.Fatalf("UploadChunkImages failed: %v", err)
	}

	if chunks[0].ImgID != "" {
		t.Errorf("ImgID = %q, want empty string", chunks[0].ImgID)
	}
	if callCount != 0 {
		t.Errorf("expected 0 Put calls, got %d", callCount)
	}
}

func TestUploadChunkImages_Mixed(t *testing.T) {
	imgBase64 := base64.StdEncoding.EncodeToString([]byte("img1"))
	sections := []map[string]any{
		{"text": "Img1", "doc_type_kwd": "text", "img_id": "", "image": imgBase64},
		{"text": "No img", "doc_type_kwd": "text", "img_id": ""},
		{"text": "Preset", "doc_type_kwd": "text", "img_id": "preset-001"},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	var callCount int
	putFn := func(bucket, fnm string, binary []byte) error {
		callCount++
		return nil
	}

	if err := UploadChunkImages(sections, chunks, testKBID, putFn); err != nil {
		t.Fatalf("UploadChunkImages failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 Put call, got %d", callCount)
	}
	if chunks[0].ImgID != testKBID+"-"+chunks[0].ID {
		t.Errorf("chunk[0] ImgID = %q, want %q", chunks[0].ImgID, testKBID+"-"+chunks[0].ID)
	}
	if chunks[1].ImgID != "" {
		t.Errorf("chunk[1] ImgID = %q, want empty", chunks[1].ImgID)
	}
	if chunks[2].ImgID != "preset-001" {
		t.Errorf("chunk[2] ImgID = %q, want 'preset-001'", chunks[2].ImgID)
	}
}

func TestUploadChunkImages_InvalidBase64(t *testing.T) {
	sections := []map[string]any{
		{"text": "Bad image", "doc_type_kwd": "text", "img_id": "", "image": "!!!not-base64!!!"},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	putFn := func(bucket, fnm string, binary []byte) error {
		return nil
	}

	if err := UploadChunkImages(sections, chunks, testKBID, putFn); err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestUploadChunkImages_PutError(t *testing.T) {
	imgBase64 := base64.StdEncoding.EncodeToString([]byte("img"))
	sections := []map[string]any{
		{"text": "Put fails", "doc_type_kwd": "text", "img_id": "", "image": imgBase64},
	}
	chunks := prepareDocs(sections, testDocID, testKBID, testDocName, 0)

	putFn := func(bucket, fnm string, binary []byte) error {
		return errors.New("storage unavailable")
	}

	if err := UploadChunkImages(sections, chunks, testKBID, putFn); err == nil {
		t.Fatal("expected error for put failure, got nil")
	}
}

func TestChunkID_NotPanic(t *testing.T) {
	// Edge cases: empty content
	_ = chunkID("", testDocID)
	_ = chunkID("content", "")
	_ = chunkID("", "")
	// All should not panic
}

func TestChunk_String(t *testing.T) {
	c := Chunk{
		ID:                "abc123",
		DocID:             "doc-1",
		ContentWithWeight: "Short text",
		PageNumInt:        []int{1},
		TopInt:            []int{0},
	}
	s := c.String()
	if len(s) == 0 {
		t.Error("String() returned empty")
	}
}

func TestChunk_StringTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "x"
	}
	c := Chunk{
		ID:                "abc123",
		DocID:             "doc-1",
		ContentWithWeight: long,
	}
	s := c.String()
	if len(s) >= 200 {
		t.Error("String() should truncate long content")
	}
}
