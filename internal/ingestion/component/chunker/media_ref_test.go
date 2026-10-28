// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunker

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	medialib "ragflow/internal/parser/media"
	parserlib "ragflow/internal/parser/parser"
)

// mediaRefHTML is a caption-bearing image item whose bytes live only in the
// source document, matching what every parser now emits.
const mediaRefHTML = `<img alt="figure" src="data:image/png;base64,` + pngBase64 + `">`

// runMediaRefChunker parses html, drives comp through the upload decorator,
// and returns the emitted chunk maps. The media source is the same html, so a
// reference that survives unit construction must materialize here.
func runMediaRefChunker(t *testing.T, comp runtime.Component, docName, html string, params map[string]any) []map[string]any {
	t.Helper()

	parsed := parserlib.NewHTMLParser().ParseWithResult(t.Context(), docName, []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}

	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	prevSource := chunkMediaSourceFactory
	chunkMediaSourceFactory = func(*gorm.DB, map[string]any) medialib.MediaSource {
		return medialib.MediaSourceFunc(func(context.Context) ([]byte, error) { return []byte(html), nil })
	}
	t.Cleanup(func() { chunkMediaSourceFactory = prevSource })

	in := map[string]any{
		"name": docName, "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	}
	for k, v := range params {
		in[k] = v
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, in)
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// uploadedMediaChunks returns the image chunks and pins the post-upload
// shape: every one carries the uploaded ID and no longer carries the
// reference or the bytes.
func uploadedMediaChunks(t *testing.T, chunks []map[string]any) []map[string]any {
	t.Helper()
	var images []map[string]any
	for _, ck := range chunks {
		if ck["doc_type_kwd"] != "image" {
			continue
		}
		images = append(images, ck)
		id, _ := ck["id"].(string)
		if id == "" {
			t.Fatalf("image chunk without id: %#v", ck)
		}
		if ck["img_id"] != testKBID+"-"+id {
			t.Errorf("img_id = %v, want the uploaded reference", ck["img_id"])
		}
		if _, ok := ck["media_ref"]; ok {
			t.Error("media_ref retained after upload")
		}
		if _, ok := ck["image"]; ok {
			t.Error("image bytes retained after upload")
		}
	}
	return images
}

// TestTokenChunkerResolvesMediaRef covers the default chunker variant: the
// reference survives unit construction and is materialized only at upload.
func TestTokenChunkerResolvesMediaRef(t *testing.T) {
	html := `<p>before</p><p>` + mediaRefHTML + `</p><p>after</p>`
	comp, err := NewTokenChunker(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	// No doc_id: the TokenChunker would try to open a PDF engine from it, and
	// the test supplies the media source directly.
	chunks := runMediaRefChunker(t, comp, "deck.html", html, nil)
	if images := uploadedMediaChunks(t, chunks); len(images) != 1 {
		t.Fatalf("image chunks = %d, want 1", len(images))
	}
}

// TestTitleChunkerCarriesMediaRef covers the group/title path, which builds
// chunks from lineRecord values instead of ChunkDoc conversions.
func TestTitleChunkerCarriesMediaRef(t *testing.T) {
	html := `<h1>Heading</h1><p>body</p>` + mediaRefHTML
	comp, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	chunks := runMediaRefChunker(t, comp, "title.html", html, nil)
	if images := uploadedMediaChunks(t, chunks); len(images) != 1 {
		t.Fatalf("image chunks = %d, want 1", len(images))
	}
}

// TestGeneralChunkerCarriesMediaRef covers the General chunker's finalize
// path, which keeps a media item alive by its reference.
func TestGeneralChunkerCarriesMediaRef(t *testing.T) {
	html := `<p>before</p>` + mediaRefHTML + `<p>after</p>`
	comp, err := NewGeneralChunker(map[string]any{"chunk_token_size": 128})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	chunks := runMediaRefChunker(t, comp, "general.html", html, map[string]any{"file_type": "html"})
	if images := uploadedMediaChunks(t, chunks); len(images) != 1 {
		t.Fatalf("image chunks = %d, want 1", len(images))
	}
}

// TestQAChunkerCarriesMediaRef covers an image item whose alt text is a QA
// pair: the QA extractor must keep the reference so the upload pass can still
// materialize the attachment.
func TestQAChunkerCarriesMediaRef(t *testing.T) {
	html := `<img alt="question&#9;answer" src="data:image/png;base64,` + pngBase64 + `">`
	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	chunks := runMediaRefChunker(t, comp, "qa.html", html, nil)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if images := uploadedMediaChunks(t, chunks); len(images) != 1 {
		t.Fatalf("image chunks = %d, want 1", len(images))
	}
}
