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

	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	medialib "ragflow/internal/parser/media"
	parserlib "ragflow/internal/parser/parser"
	"ragflow/internal/storage"
)

// TestTokenChunkerResolvesMediaRef covers the default chunker variant: the
// reference survives unit construction and is materialized only at upload.
func TestTokenChunkerResolvesMediaRef(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<p>before</p><p><img alt="figure" src="` + uri + `"></p><p>after</p>`

	p := parserlib.NewHTMLParser()
	parsed := p.ParseWithResult(t.Context(), "deck.html", []byte(html))
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

	comp, err := NewTokenChunker(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	// No doc_id: the TokenChunker would try to open a PDF engine from it, and
	// this test supplies the media source directly.
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "deck.html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	images := 0
	for _, ck := range chunks {
		if ck["doc_type_kwd"] != "image" {
			continue
		}
		images++
		id, _ := ck["id"].(string)
		if id == "" {
			t.Fatalf("image chunk without id: %#v", ck)
		}
		if ck["img_id"] != testKBID+"-"+id {
			t.Errorf("img_id = %v, want uploaded reference", ck["img_id"])
		}
		if _, ok := ck["media_ref"]; ok {
			t.Error("media_ref retained after upload")
		}
		if _, ok := ck["image"]; ok {
			t.Error("image bytes retained after upload")
		}
	}
	if images != 1 {
		t.Fatalf("image chunks = %d, want 1", images)
	}
}

// TestTitleChunkerCarriesMediaRef covers the group/title path, which builds
// chunks from lineRecord values instead of ChunkDoc conversions.
func TestTitleChunkerCarriesMediaRef(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<h1>Heading</h1><p>body</p><img alt="figure" src="` + uri + `">`

	p := parserlib.NewHTMLParser()
	parsed := p.ParseWithResult(t.Context(), "title.html", []byte(html))
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

	comp, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "title.html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	images := 0
	for _, ck := range chunks {
		if ck["doc_type_kwd"] != "image" {
			continue
		}
		images++
		if imgID, ok := ck["img_id"].(string); !ok || imgID == "" {
			t.Errorf("image chunk not uploaded: %#v", ck)
		}
		if _, ok := ck["media_ref"]; ok {
			t.Error("media_ref retained after upload")
		}
	}
	if images != 1 {
		t.Fatalf("image chunks = %d, want 1", images)
	}
}

// TestGeneralChunkerCarriesMediaRef covers the General chunker's finalize
// path, which keeps a media item alive by its reference.
func TestGeneralChunkerCarriesMediaRef(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<p>before</p><img alt="figure" src="` + uri + `"><p>after</p>`

	p := parserlib.NewHTMLParser()
	parsed := p.ParseWithResult(t.Context(), "general.html", []byte(html))
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

	comp, err := NewGeneralChunker(map[string]any{"chunk_token_size": 128})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "general.html", "file_type": "html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	images := 0
	for _, ck := range chunks {
		if ck["doc_type_kwd"] != "image" {
			continue
		}
		images++
		if imgID, ok := ck["img_id"].(string); !ok || imgID == "" {
			t.Errorf("image chunk not uploaded: %#v", ck)
		}
		if _, ok := ck["media_ref"]; ok {
			t.Error("media_ref retained after upload")
		}
	}
	if images != 1 {
		t.Fatalf("image chunks = %d, want 1", images)
	}
}

// TestOneChunkerKeepsImgIDForTextlessItem covers the single-item guard: an
// item kept alive by ImgID must carry that ID into the output chunk.
func TestOneChunkerKeepsImgIDForTextlessItem(t *testing.T) {
	out := emitOneFromItems([]schema.ChunkDoc{{DocType: "image", ImgID: "kb-1-chunk-1"}}, nil)
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0]["img_id"] != "kb-1-chunk-1" {
		t.Fatalf("img_id = %v, want kb-1-chunk-1", chunks[0]["img_id"])
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
	src := medialib.MediaSourceFunc(func(context.Context) ([]byte, error) {
		t.Fatal("resolver must not run for an invalid reference")
		return nil, nil
	})
	if _, err := uploadChunkMedia(t.Context(), []map[string]any{ck}, up, testKBID, testDocID, medialib.NewMediaResolver(), src); err == nil {
		t.Fatal("invalid media_ref was treated as absent")
	}
}

// TestChunkMediaSourceResolvesStorage covers both source locations the
// chunker accepts: the forwarded bucket/path pair and doc_id-driven lookup.
func TestChunkMediaSourceResolvesStorage(t *testing.T) {
	ctx := t.Context()
	payload := []byte("<p>source</p>")
	mem := storage.NewMemoryStorage()
	if err := mem.Put(ctx, "kb-1", "docs/a.html", payload); err != nil {
		t.Fatal(err)
	}
	prevStorage := component.SetStorageFactoryOverride
	component.SetStorageFactoryOverride = func() storage.Storage { return mem }
	t.Cleanup(func() { component.SetStorageFactoryOverride = prevStorage })

	src := chunkMediaSourceFactory(nil, map[string]any{"bucket": "kb-1", "path": "docs/a.html"})
	data, err := src.Bytes(ctx)
	if err != nil || string(data) != string(payload) {
		t.Fatalf("bucket/path source = %q, %v", data, err)
	}

	prevResolve := component.ResolveDocumentStorageOverride
	component.ResolveDocumentStorageOverride = func(string) (*component.DocumentStorageRef, error) {
		return &component.DocumentStorageRef{Bucket: "kb-1", Path: "docs/a.html"}, nil
	}
	t.Cleanup(func() { component.ResolveDocumentStorageOverride = prevResolve })

	src = chunkMediaSourceFactory(&gorm.DB{}, map[string]any{"doc_id": "doc-1"})
	data, err = src.Bytes(ctx)
	if err != nil || string(data) != string(payload) {
		t.Fatalf("doc_id source = %q, %v", data, err)
	}
}

// TestImageUploadDecorator_ResolvesSelfClosingImage covers the upload path
// for the common <img ... /> form.
func TestImageUploadDecorator_ResolvesSelfClosingImage(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<img alt="figure" src="` + uri + `" />`

	p := parserlib.NewHTMLParser()
	parsed := p.ParseWithResult(t.Context(), "self-closing.html", []byte(html))
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

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "self-closing.html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	images := 0
	for _, ck := range chunks {
		if ck["doc_type_kwd"] != "image" {
			continue
		}
		images++
		if imgID, ok := ck["img_id"].(string); !ok || imgID == "" {
			t.Fatalf("self-closing image not uploaded: %#v", ck)
		}
	}
	if images != 1 {
		t.Fatalf("image chunks = %d, want 1", images)
	}
}

// TestQAChunkerCarriesMediaRef covers an image item whose alt text is a QA
// pair: the QA extractor must keep the reference so the upload pass can still
// materialize the attachment.
func TestQAChunkerCarriesMediaRef(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<img alt="question&#9;answer" src="` + uri + `">`

	parsed := parserlib.NewHTMLParser().ParseWithResult(t.Context(), "qa.html", []byte(html))
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

	comp, err := NewQAChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "qa.html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	chunk := chunks[0]
	if chunk["doc_type_kwd"] != "image" {
		t.Fatalf("doc_type = %v, want image", chunk["doc_type_kwd"])
	}
	if imgID, ok := chunk["img_id"].(string); !ok || imgID == "" {
		t.Fatalf("QA image not uploaded: %#v", chunk)
	}
	if _, ok := chunk["media_ref"]; ok {
		t.Fatal("media_ref retained after upload")
	}
}
