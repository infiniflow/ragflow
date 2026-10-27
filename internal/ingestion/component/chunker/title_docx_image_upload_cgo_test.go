//go:build cgo

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

package chunker

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/parser/parser"
)

// docxParserImageFixture is the checked-in DOCX carrying a table and one
// embedded image; it exercises the real office_oxide IR path rather than a
// hand-written item map.
const docxParserImageFixture = "docx_table_image.docx"

// TestTitleChunkerUploadsDOCXParserImage feeds a real DOCX parse result
// through the registered title-chunker entry point — the production wiring,
// including the image-upload decorator every registered chunker is wrapped in
// — and asserts every image the parser emitted reaches object storage with its
// bytes intact.
//
// The DOCX image-budget fix (#21230) is about the chunk editor showing an
// image chunk with no picture to preview, which is a delivery symptom: a
// payload the parser drops never becomes stored bytes, and payloadless image
// items still flow through the chunker. The parser-tier tests pin the budget
// at its source; this pins the other half of the chain, that a DOCX-shaped
// parser result (image item with empty text, raw base64 payload and table
// metadata) survives chunking and uploads.
func TestTitleChunkerUploadsDOCXParserImage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "parser", "parser", "testdata", docxParserImageFixture))
	if err != nil {
		t.Fatal(err)
	}
	res := parser.NewDOCXParser().ParseWithResult(t.Context(), docxParserImageFixture, data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	items := parser.ItemsToMaps(res.JSON)

	// Source of truth for the assertion: the image bytes the parser emitted.
	var want [][]byte
	var sourceImages int
	for _, item := range items {
		if item["doc_type_kwd"] != "image" {
			continue
		}
		sourceImages++
		encoded, _ := item["image"].(string)
		if encoded == "" {
			t.Fatalf("fixture image item %d has no payload: %v", sourceImages, item)
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("decode fixture image payload: %v", err)
		}
		want = append(want, decoded)
	}
	if sourceImages == 0 {
		t.Fatalf("fixture produced no image items, the test would assert nothing")
	}

	var uploaded [][]byte
	original := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, img []byte) (string, error) {
		uploaded = append(uploaded, append([]byte(nil), img...))
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = original })

	factory, _, _, ok := runtime.DefaultRegistry.Lookup(ComponentNameGroupTitleChunker)
	if !ok || factory == nil {
		t.Fatalf("%s is not registered", ComponentNameGroupTitleChunker)
	}
	comp, err := factory(ComponentNameGroupTitleChunker, map[string]any{"levels": [][]string{{`^intro$`}}})
	if err != nil {
		t.Fatalf("construct %s: %v", ComponentNameGroupTitleChunker, err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name": docxParserImageFixture, "kb_id": "kb-1",
		"output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected chunks carrier: %#v", out["chunks"])
	}

	var imageChunks int
	for _, ck := range chunks {
		if ck["doc_type_kwd"] != "image" {
			continue
		}
		imageChunks++
		if imgID, _ := ck["img_id"].(string); imgID == "" {
			t.Fatalf("image chunk %d has no img_id: %v", imageChunks, ck)
		}
		if _, stillRaw := ck["image"]; stillRaw {
			t.Errorf("image chunk %d kept its raw payload after upload: %v", imageChunks, ck)
		}
	}
	if imageChunks != sourceImages {
		t.Fatalf("image chunks = %d, want %d (one per parser image item)", imageChunks, sourceImages)
	}
	if len(uploaded) != len(want) {
		t.Fatalf("uploaded images = %d, want %d", len(uploaded), len(want))
	}
	for i := range want {
		if string(uploaded[i]) != string(want[i]) {
			t.Errorf("uploaded image %d = %d bytes, want the parser's %d bytes", i, len(uploaded[i]), len(want[i]))
		}
	}
}
