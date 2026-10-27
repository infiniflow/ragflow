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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/media"
	parserlib "ragflow/internal/parser/parser"
)

// TestTokenChunkerResolvesMediaRef covers the default chunker variant: the
// reference survives unit construction and is materialized only at upload.
func TestTokenChunkerResolvesMediaRef(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<p>before</p><p><img alt="figure" src="` + uri + `"></p><p>after</p>`

	p := parserlib.NewHTMLParser()
	p.MediaMode = parserlib.MediaRef
	parsed := p.ParseWithResult(t.Context(), "deck.html", []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}

	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	prevSource := ChunkMediaSource
	ChunkMediaSource = func(*gorm.DB, map[string]any) media.Source {
		return media.SourceFunc(func(context.Context) ([]byte, error) { return []byte(html), nil })
	}
	t.Cleanup(func() { ChunkMediaSource = prevSource })

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

// TestMediaRefShadowDoesNotChangeOutput pins the shadow contract: a resolver
// failure during verification must not alter the chunk or block the inline
// upload.
func TestMediaRefShadowDoesNotChangeOutput(t *testing.T) {
	t.Setenv(EnvMediaRefShadow, "1")
	want, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + pngBase64
	html := `<img alt="figure" src="` + uri + `">`

	p := parserlib.NewHTMLParser()
	p.MediaMode = parserlib.MediaBoth
	parsed := p.ParseWithResult(t.Context(), "shadow.html", []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}

	var uploaded []byte
	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, data []byte) (string, error) {
		uploaded = append([]byte(nil), data...)
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	prevSource := ChunkMediaSource
	ChunkMediaSource = func(*gorm.DB, map[string]any) media.Source {
		return media.SourceFunc(func(context.Context) ([]byte, error) {
			return nil, errors.New("source unavailable")
		})
	}
	t.Cleanup(func() { ChunkMediaSource = prevSource })

	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "shadow.html", "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	})
	if err != nil {
		t.Fatalf("decorator: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0]["img_id"] == "" {
		t.Fatal("shadow failure blocked the inline upload")
	}
	if !bytes.Equal(uploaded, want) {
		t.Fatalf("uploaded %d bytes, want the inline image", len(uploaded))
	}
}

// TestTitleChunkerCarriesMediaRef covers the group/title path, which builds
// chunks from lineRecord values instead of ChunkDoc conversions.
func TestTitleChunkerCarriesMediaRef(t *testing.T) {
	uri := "data:image/png;base64," + pngBase64
	html := `<h1>Heading</h1><p>body</p><img alt="figure" src="` + uri + `">`

	p := parserlib.NewHTMLParser()
	p.MediaMode = parserlib.MediaRef
	parsed := p.ParseWithResult(t.Context(), "title.html", []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}

	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	prevSource := ChunkMediaSource
	ChunkMediaSource = func(*gorm.DB, map[string]any) media.Source {
		return media.SourceFunc(func(context.Context) ([]byte, error) { return []byte(html), nil })
	}
	t.Cleanup(func() { ChunkMediaSource = prevSource })

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
		if ck["img_id"] == "" {
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
	p.MediaMode = parserlib.MediaRef
	parsed := p.ParseWithResult(t.Context(), "general.html", []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}

	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	prevSource := ChunkMediaSource
	ChunkMediaSource = func(*gorm.DB, map[string]any) media.Source {
		return media.SourceFunc(func(context.Context) ([]byte, error) { return []byte(html), nil })
	}
	t.Cleanup(func() { ChunkMediaSource = prevSource })

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
		if ck["img_id"] == "" {
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
	src := media.SourceFunc(func(context.Context) ([]byte, error) {
		t.Fatal("resolver must not run for an invalid reference")
		return nil, nil
	})
	if _, err := uploadChunkMedia(t.Context(), []map[string]any{ck}, up, testKBID, media.NewResolver(), src); err == nil {
		t.Fatal("invalid media_ref was treated as absent")
	}
}
