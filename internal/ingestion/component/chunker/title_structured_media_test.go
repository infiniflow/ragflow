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

package chunker

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

const titleTestTableHTML = "<table><tr><th>name</th><th>age</th></tr><tr><td>Alice</td><td>30</td></tr></table>"

// pngBase64Red is a second valid 1x1 PNG (red), distinct from pngBase64, used
// to pin that captionless images keep distinct chunk ids.
const pngBase64Red = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// titleFamilyCase pairs a title-family entry point with its display name.
type titleFamilyCase struct {
	name string
	comp runtime.Component
}

// titleFamilyComponents builds the three entry points behind the shared
// extractLineRecords record path. TitleChunker's group/hierarchy methods
// dispatch into the same invokeGroup/invokeHierarchy implementations, so the
// standalone components cover every title-family strategy.
func titleFamilyComponents(t *testing.T) []titleFamilyCase {
	t.Helper()
	levels := [][]string{{`^# `}}
	group, err := NewGroupTitleChunker(map[string]any{"levels": levels})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	hierarchy, err := NewHierarchyTitleChunker(map[string]any{"hierarchy": 1, "levels": levels})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	manual, err := NewManualChunker(map[string]any{"levels": levels})
	if err != nil {
		t.Fatalf("NewManualChunker: %v", err)
	}
	return []titleFamilyCase{
		{"group", group},
		{"hierarchy", hierarchy},
		{"manual", manual},
	}
}

// titleFamilyChunks invokes one title-family component over a json payload and
// returns the emitted chunks.
func titleFamilyChunks(t *testing.T, comp runtime.Component, items []map[string]any) []map[string]any {
	t.Helper()
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.pdf",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

// renderedChunkText returns the HTML rendering the chunker derives from the
// item's structured TableData, through the same map round trip the chunker
// performs.
func renderedChunkText(t *testing.T, item map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(item["table"])
	if err != nil {
		t.Fatalf("marshal table: %v", err)
	}
	var typed entity.TableData
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("unmarshal table: %v", err)
	}
	return tableutil.RenderTableHTML(&typed)
}

// TestTitleFamilyRendersStructuredTableAtomic: a parser-shaped table item
// (structured TableData, no text) must survive the title-family record path as
// one atomic chunk carrying the verbatim HTML rendering — header rows rendered
// as <th> per HeaderRows — with its positions kept and no neighbor text glued
// in.
func TestTitleFamilyRendersStructuredTableAtomic(t *testing.T) {
	item := tableItem(t, titleTestTableHTML, map[string]any{
		"_pdf_positions": [][]float64{{1, 10, 200, 10, 40}},
	})
	want := renderedChunkText(t, item)
	if !strings.Contains(want, "<th>name</th>") || !strings.Contains(want, "<td>Alice</td>") {
		t.Fatalf("fixture rendering lost header/cell markup: %q", want)
	}

	for _, tc := range titleFamilyComponents(t) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "# H1", "doc_type_kwd": "text"},
				item,
				{"text": "body paragraph", "doc_type_kwd": "text"},
			})
			var tableChunks []map[string]any
			for _, ck := range chunks {
				if ck["doc_type_kwd"] == "table" {
					tableChunks = append(tableChunks, ck)
				}
			}
			if len(tableChunks) != 1 {
				t.Fatalf("table chunks = %d, want exactly 1 (chunks: %#v)", len(tableChunks), chunks)
			}
			ck := tableChunks[0]
			if ck["text"] != want {
				t.Errorf("table chunk text = %q, want the verbatim rendering %q", ck["text"], want)
			}
			if _, ok := ck["table"]; ok {
				t.Errorf("structured TableData must not leak onto the chunk: %#v", ck["table"])
			}
			if got := ck["_pdf_positions"]; !reflect.DeepEqual(got, [][]any{{float64(1), float64(10), float64(200), float64(10), float64(40)}}) {
				t.Errorf("_pdf_positions = %#v, want the source matrix", got)
			}
			for _, other := range chunks {
				if other["doc_type_kwd"] != "table" && strings.Contains(toString(other["text"]), "<table>") {
					t.Errorf("table markup leaked into a text chunk: %#v", other)
				}
			}
		})
	}
}

// TestTitleFamilyKeepsEmptyTextImageItem: an image item legitimately carries no
// text — its payload is the image field — and must survive as a chunk instead
// of being dropped by the empty-text skip.
func TestTitleFamilyKeepsEmptyTextImageItem(t *testing.T) {
	payload := "data:image/png;base64," + pngBase64
	for _, tc := range titleFamilyComponents(t) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "", "image": payload, "doc_type_kwd": "image"},
			})
			if len(chunks) != 1 {
				t.Fatalf("chunks = %d, want the image item kept as 1 chunk", len(chunks))
			}
			ck := chunks[0]
			if ck["doc_type_kwd"] != "image" {
				t.Errorf("doc_type_kwd = %v, want image", ck["doc_type_kwd"])
			}
			if ck["image"] != payload {
				t.Errorf("image payload lost: %v", ck["image"])
			}
			if ck["text"] != "" {
				t.Errorf("text = %q, want empty", ck["text"])
			}
		})
	}
}

// TestTitleFamilyEmptyTextImageUploadsThroughDecorator: the decorated chunker
// uploads the payload at the chunk id, writes img_id, and drops the raw bytes —
// the empty-text image chunk must flow through the same decorator path as
// every other chunker's media chunk.
func TestTitleFamilyEmptyTextImageUploadsThroughDecorator(t *testing.T) {
	prev := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prev })

	comp, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	decorated := &imageUploadDecorator{inner: comp}

	payload := "data:image/png;base64," + pngBase64
	out, err := decorated.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.docx",
		"kb_id":         testKBID,
		"output_format": "json",
		"json": []map[string]any{
			{"text": "", "image": payload, "doc_type_kwd": "image"},
		},
	})
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	ck := chunks[0]
	// No doc_id in the payload (it would trigger the on-demand PDF crop path,
	// which needs a database), so the id hashes the empty document id plus the
	// captionless image's own identity (mediaChunkIdentity).
	wantChunkID := common.ChunkID("", mediaChunkIdentity(payload, ""))
	if ck["id"] != wantChunkID {
		t.Errorf("id = %v, want %q", ck["id"], wantChunkID)
	}
	if ck["img_id"] != testKBID+"-"+wantChunkID {
		t.Errorf("img_id = %v, want %q", ck["img_id"], testKBID+"-"+wantChunkID)
	}
	if _, ok := ck["image"]; ok {
		t.Errorf("image not dropped after upload: %v", ck["image"])
	}
}

// TestTitleFamilyCaptionlessImagesGetDistinctIDs: two different captionless
// images in one document must not collapse onto the same chunk id — a shared
// id would make the second upload overwrite the first image in MinIO and merge
// the two index entries.
func TestTitleFamilyCaptionlessImagesGetDistinctIDs(t *testing.T) {
	uploaded := map[string]int{}
	prev := ChunkImageUploader
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		uploaded[chunkID]++
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prev })

	comp, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	decorated := &imageUploadDecorator{inner: comp}

	out, err := decorated.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.docx",
		"kb_id":         testKBID,
		"output_format": "json",
		"json": []map[string]any{
			{"text": "", "image": "data:image/png;base64," + pngBase64, "doc_type_kwd": "image"},
			{"text": "", "image": "data:image/png;base64," + pngBase64Red, "doc_type_kwd": "image"},
		},
	})
	if err != nil {
		t.Fatalf("decorated Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %d, want 2", len(chunks))
	}
	idA, _ := chunks[0]["id"].(string)
	idB, _ := chunks[1]["id"].(string)
	if idA == "" || idB == "" || idA == idB {
		t.Fatalf("captionless image ids must be distinct, got %q and %q", idA, idB)
	}
	if idA == common.ChunkID("", "") || idB == common.ChunkID("", "") {
		t.Fatalf("id still hashes the empty text alone: %q / %q", idA, idB)
	}
	if len(uploaded) != 2 || uploaded[idA]+uploaded[idB] != 2 {
		t.Fatalf("uploads must land on two distinct keys, got %v", uploaded)
	}
}

// TestTitleFamilyReportsDecodeFailure: a payload that declares a format but
// carries no matching payload fails decode and short-circuits through the
// family-wide input-error shape instead of silently producing nothing.
func TestTitleFamilyReportsDecodeFailure(t *testing.T) {
	for _, tc := range titleFamilyComponents(t) {
		t.Run(tc.name, func(t *testing.T) {
			out, err := tc.comp.Invoke(t.Context(), nil, map[string]any{
				"name":          "doc.pdf",
				"output_format": "json", // no "json" payload → decode fails
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if msg, _ := out["_ERROR"].(string); !strings.HasPrefix(msg, "Input error:") {
				t.Fatalf("_ERROR = %v, want an input error", out["_ERROR"])
			}
			if chunks, _ := out["chunks"].([]map[string]any); len(chunks) != 0 {
				t.Fatalf("chunks = %#v, want none", chunks)
			}
		})
	}
}

// TestTitleFamilyDropsFullyEmptyItems: an item with no text, no table and no
// image has nothing retrievable and stays dropped.
func TestTitleFamilyDropsFullyEmptyItems(t *testing.T) {
	for _, tc := range titleFamilyComponents(t) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "", "doc_type_kwd": "text"},
			})
			if len(chunks) != 0 {
				t.Fatalf("chunks = %d, want 0 for a fully empty item: %#v", len(chunks), chunks)
			}
			chunks = titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "", "doc_type_kwd": "text"},
				{"text": "real body", "doc_type_kwd": "text"},
			})
			if len(chunks) != 1 || !strings.Contains(toString(chunks[0]["text"]), "real body") {
				t.Fatalf("chunks = %#v, want only the retrievable text chunk", chunks)
			}
		})
	}
}

// TestTitleFamilyPreservesSheetIdentity: a spreadsheet item routed through a
// title chunker keeps its sheet identity so its positions matrix stays in the
// spreadsheet vocabulary instead of being decoded as PDF boxes.
func TestTitleFamilyPreservesSheetIdentity(t *testing.T) {
	item := tableItem(t, "<table><tr><th>Q</th><th>A</th></tr><tr><td>q1</td><td>a1</td></tr></table>",
		map[string]any{
			"sheet":       "Orders",
			"sheet_index": 1,
			"positions":   [][]float64{{1, 1, 1, 1, 2}, {1, 2, 2, 1, 2}},
		})
	comp, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	chunks := titleFamilyChunks(t, comp, []map[string]any{item})
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	ck := chunks[0]
	if ck["sheet"] != "Orders" {
		t.Errorf("sheet = %v, want Orders", ck["sheet"])
	}
	if ck["sheet_index"] != float64(1) {
		t.Errorf("sheet_index = %#v, want 1", ck["sheet_index"])
	}
	if _, ok := ck["_pdf_positions"]; ok {
		t.Errorf("spreadsheet selections leaked into _pdf_positions: %#v", ck["_pdf_positions"])
	}
	if got := ck["positions"]; !reflect.DeepEqual(got, [][]any{
		{float64(1), float64(1), float64(1), float64(1), float64(2)},
		{float64(1), float64(2), float64(2), float64(1), float64(2)},
	}) {
		t.Errorf("positions = %#v, want the source matrix", got)
	}
}
