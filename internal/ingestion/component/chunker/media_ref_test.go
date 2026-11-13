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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	medialib "ragflow/internal/parser/media"
	parserlib "ragflow/internal/parser/parser"
)

type chunkerMediaCache struct {
	data   []byte
	getErr error
	gets   int
}

func (c *chunkerMediaCache) Put(_ context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	c.data = append([]byte(nil), data...)
	sum := sha256.Sum256(data)
	return entity.MediaRef{
		Format: format,
		Kind:   entity.MediaKindKV,
		Scope:  "run-1",
		SHA256: hex.EncodeToString(sum[:]),
		MIME:   mime,
		Size:   int64(len(data)),
	}, nil
}

func (c *chunkerMediaCache) Get(context.Context, entity.MediaRef) ([]byte, error) {
	c.gets++
	if c.getErr != nil {
		return nil, c.getErr
	}
	return append([]byte(nil), c.data...), nil
}

func TestUploadChunkMedia_ReadsRepeatedReferenceFromCache(t *testing.T) {
	data := []byte("hello")
	cache := &chunkerMediaCache{data: data}
	ctx := medialib.WithCache(t.Context(), cache)
	ref, err := cache.Put(ctx, "html", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	var uploaded [][]byte
	uploader := func(_ context.Context, kbID, chunkID string, payload []byte) (string, error) {
		uploaded = append(uploaded, append([]byte(nil), payload...))
		return kbID + "-" + chunkID, nil
	}
	chunks := []map[string]any{
		{"id": "chunk-1", "text": "first", "doc_type_kwd": "image", "media_ref": ref},
		{"id": "chunk-2", "text": "second", "doc_type_kwd": "image", "media_ref": ref},
	}
	kept, err := uploadChunkMedia(ctx, chunks, uploader, testKBID, testDocID)
	if err != nil {
		t.Fatalf("uploadChunkMedia: %v", err)
	}
	if len(kept) != 2 || cache.gets != 2 {
		t.Fatalf("kept/cache reads = %d/%d, want 2/2", len(kept), cache.gets)
	}
	for i, payload := range uploaded {
		if string(payload) != string(data) {
			t.Errorf("upload %d = %q, want %q", i, payload, data)
		}
	}
	for _, ck := range kept {
		if _, ok := ck["media_ref"]; ok {
			t.Errorf("media_ref survived upload: %#v", ck)
		}
	}
}

func TestUploadChunkMedia_CacheMissFailsEvenWithCaption(t *testing.T) {
	cache := &chunkerMediaCache{getErr: medialib.ErrMediaNotFound}
	ctx := medialib.WithCache(t.Context(), cache)
	ref := entity.MediaRef{
		Format: "html",
		Kind:   entity.MediaKindKV,
		Scope:  "run-1",
		SHA256: strings.Repeat("a", 64),
		MIME:   "image/png",
		Size:   5,
	}
	chunks := []map[string]any{{
		"id": "chunk-1", "text": "caption", "doc_type_kwd": "image", "media_ref": ref,
	}}
	_, err := uploadChunkMedia(ctx, chunks, func(context.Context, string, string, []byte) (string, error) {
		t.Fatal("upload ran after a cache miss")
		return "", nil
	}, testKBID, testDocID)
	if !errors.Is(err, medialib.ErrMediaNotFound) {
		t.Fatalf("uploadChunkMedia error = %v, want ErrMediaNotFound", err)
	}
}

// mediaRefHTML is a caption-bearing image whose decoded bytes are written
// to the run media cache by the HTML parser.
const mediaRefHTML = `<img alt="figure" src="data:image/png;base64,` + pngBase64 + `">`

// runMediaRefChunker parses html, drives comp through the upload decorator,
// and returns the emitted chunk maps. The parser writes images into the same
// run cache the upload decorator later reads.
func runMediaRefChunker(t *testing.T, comp runtime.Component, docName, html string, params map[string]any) []map[string]any {
	t.Helper()

	cache := &chunkerMediaCache{}
	ctx := medialib.WithCache(t.Context(), cache)
	parsed := parserlib.NewHTMLParser().ParseWithResult(ctx, docName, []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}

	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	in := map[string]any{
		"name": docName, "kb_id": testKBID,
		"output_format": "json", "json": parsed.JSON,
	}
	for k, v := range params {
		in[k] = v
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, in)
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

// TestTokenChunkerKeepsTextlessMediaRef pins that a standalone image with no
// alt text is not dropped as an empty text chunk before materialization.
func TestTokenChunkerKeepsTextlessMediaRef(t *testing.T) {
	html := `<img src="data:image/png;base64,` + pngBase64 + `">`
	comp, err := NewTokenChunker(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := runMediaRefChunker(t, comp, "no-alt.html", html, nil)
	if images := uploadedMediaChunks(t, chunks); len(images) != 1 {
		t.Fatalf("image chunks = %d, want 1", len(images))
	}
}

// TestImageUploadDecoratorRejectsUnusableMediaRef pins the error contract at
// the component boundary: a present reference that cannot be materialized
// must fail the run, even when the item has no text and would otherwise be
// filtered as empty.
func TestImageUploadDecoratorRejectsUnusableMediaRef(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []map[string]any
	}{
		{
			name:  "textless partial reference",
			items: []map[string]any{{"doc_type_kwd": "image", "media_ref": map[string]any{"format": "html"}}},
		},
		{
			name:  "partial reference with text",
			items: []map[string]any{{"text": "caption", "media_ref": map[string]any{"format": "html"}}},
		},
		{
			name:  "unknown reference fields",
			items: []map[string]any{{"text": "caption", "media_ref": map[string]any{"unknown": "value"}}},
		},
		{
			name: "malformed reference in a merged item list",
			items: []map[string]any{
				{"text": "kept"},
				{"doc_type_kwd": "image", "media_ref": map[string]any{"format": "html"}},
			},
		},
		{
			name: "short digest",
			items: []map[string]any{{
				"text": "caption",
				"media_ref": map[string]any{
					"format": "html",
					"kind":   "kvrocks",
					"scope":  "run-1",
					"sha256": strings.Repeat("a", 63),
				},
			}},
		},
		{
			name: "non-hex digest",
			items: []map[string]any{{
				"text": "caption",
				"media_ref": map[string]any{
					"format": "html",
					"kind":   "kvrocks",
					"scope":  "run-1",
					"sha256": strings.Repeat("z", 64),
				},
			}},
		},
		{
			name: "uppercase digest",
			items: []map[string]any{{
				"text": "caption",
				"media_ref": map[string]any{
					"format": "html",
					"kind":   "kvrocks",
					"scope":  "run-1",
					"sha256": strings.ToUpper(strings.Repeat("a", 64)),
				},
			}},
		},
	} {
		for _, chunker := range []struct {
			name string
			new  func() (runtime.Component, error)
		}{
			{"one", func() (runtime.Component, error) { return NewOneChunker(nil) }},
			{"token", func() (runtime.Component, error) { return NewTokenChunker(map[string]any{}) }},
		} {
			t.Run(tc.name+"/"+chunker.name, func(t *testing.T) {
				comp, err := chunker.new()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
					"name": "bad.html", "kb_id": testKBID,
					"output_format": "json", "json": tc.items,
				}); err == nil {
					t.Fatal("unusable media_ref was accepted")
				}
			})
		}
	}
}

// TestImageUploadDecoratorAcceptsEmptyMediaRef pins the other side: an empty
// object keeps meaning "no reference", so a text item survives and a textless
// item is still filtered as empty.
func TestImageUploadDecoratorAcceptsEmptyMediaRef(t *testing.T) {
	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (&imageUploadDecorator{inner: comp}).Invoke(t.Context(), nil, map[string]any{
		"name": "empty-ref.html", "kb_id": testKBID, "doc_id": testDocID,
		"output_format": "json",
		"json": []any{
			map[string]any{"text": "kept", "media_ref": map[string]any{}},
			map[string]any{"doc_type_kwd": "image", "media_ref": map[string]any{}},
		},
	})
	if err != nil {
		t.Fatalf("empty media_ref failed: %v", err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want the text item only", len(chunks))
	}
	if chunks[0]["text"] != "kept" {
		t.Fatalf("text = %v, want kept", chunks[0]["text"])
	}
	if _, ok := chunks[0]["media_ref"]; ok {
		t.Fatal("empty media_ref reappeared on the wire")
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

// TestOneChunkerRejectsInvalidMediaRefRegardlessOfOrder pins the merge
// invariant: only the first media attachment is kept, so every present
// reference must be validated before the merge. Otherwise a valid first
// reference hides a malformed later one and the run succeeds.
func TestOneChunkerRejectsInvalidMediaRefRegardlessOfOrder(t *testing.T) {
	html := `<img alt="figure" src="data:image/png;base64,` + pngBase64 + `">`
	cache := &chunkerMediaCache{}
	ctx := medialib.WithCache(t.Context(), cache)
	parsed := parserlib.NewHTMLParser().ParseWithResult(ctx, "order.html", []byte(html))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	var validRef *entity.MediaRef
	for _, item := range parsed.JSON {
		if m := item.ToMap(); m["doc_type_kwd"] == "image" {
			validRef = entity.MediaRefFromAny(m["media_ref"])
		}
	}
	if validRef == nil {
		t.Fatal("fixture emitted no valid media_ref")
	}

	valid := map[string]any{"text": "figure", "media_ref": validRef}
	invalid := map[string]any{"doc_type_kwd": "image", "media_ref": map[string]any{"format": "html"}}

	prevUploader := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prevUploader })

	for _, tc := range []struct {
		name  string
		items []map[string]any
	}{
		{"invalid first", []map[string]any{invalid, valid}},
		{"invalid last", []map[string]any{valid, invalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := NewOneChunker(nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := (&imageUploadDecorator{inner: comp}).Invoke(ctx, nil, map[string]any{
				"name": "order.html", "kb_id": testKBID,
				"output_format": "json", "json": tc.items,
			}); err == nil {
				t.Fatal("invalid media_ref was accepted")
			}
		})
	}
}
