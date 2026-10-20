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
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

// titleImageContextItems mirrors a docx parser payload: a heading, a text
// paragraph, one captionless embedded image, and a trailing paragraph.
func titleImageContextItems() []map[string]any {
	payload := "data:image/png;base64," + pngBase64
	return []map[string]any{
		{"text": "# Report", "doc_type_kwd": "text"},
		{"text": "alpha before text", "doc_type_kwd": "text"},
		{"text": "", "image": payload, "doc_type_kwd": "image"},
		{"text": "beta after text", "doc_type_kwd": "text"},
	}
}

func titleImageContextComponents(t *testing.T, extra map[string]any) []titleFamilyCase {
	t.Helper()
	merge := func(base map[string]any) map[string]any {
		out := map[string]any{"levels": [][]string{{`^# `}}}
		for k, v := range extra {
			out[k] = v
		}
		for k, v := range base {
			out[k] = v
		}
		return out
	}
	group, err := NewGroupTitleChunker(merge(nil))
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	hierarchy, err := NewHierarchyTitleChunker(merge(map[string]any{"hierarchy": 1}))
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	manual, err := NewManualChunker(merge(nil))
	if err != nil {
		t.Fatalf("NewManualChunker: %v", err)
	}
	return []titleFamilyCase{
		{"group", group},
		{"hierarchy", hierarchy},
		{"manual", manual},
	}
}

func invokeTitleFamily(t *testing.T, comp runtime.Component, items []map[string]any) []map[string]any {
	t.Helper()
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.docx",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

func findImageChunk(t *testing.T, chunks []map[string]any) map[string]any {
	t.Helper()
	for _, ck := range chunks {
		if ck["doc_type_kwd"] == "image" {
			return ck
		}
	}
	t.Fatalf("no image chunk emitted (chunks: %#v)", chunks)
	return nil
}

// TestTitleFamilyAttachesImageContext is the parity test for the reported bug:
// with image/table context configured, a captionless image sliced by the title
// family must carry the same surrounding context General produces. Without it
// the chunk body is empty and its preview never surfaces at retrieval time.
func TestTitleFamilyAttachesImageContext(t *testing.T) {
	payload := "data:image/png;base64," + pngBase64
	for _, tc := range titleImageContextComponents(t, map[string]any{
		"image_context_size": 20,
		"table_context_size": 20,
	}) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := invokeTitleFamily(t, tc.comp, titleImageContextItems())
			ck := findImageChunk(t, chunks)
			text, _ := ck["text"].(string)
			if !strings.Contains(text, "alpha before text") || !strings.Contains(text, "beta after text") {
				t.Errorf("image chunk text = %q, want surrounding context", text)
			}
			if ck["image"] != payload {
				t.Errorf("image payload lost: %v", ck["image"])
			}
			for _, key := range []string{"context_above", "context_below"} {
				if _, exists := ck[key]; exists {
					t.Errorf("image chunk must not carry %s after materialization: %+v", key, ck)
				}
			}
		})
	}
}

// TestTitleFamilyDefaultKeepsBareImageChunk pins the default (no context
// configured): the image chunk stays atomic with its payload and empty text,
// exactly as before — the fix must not change default pipelines.
func TestTitleFamilyDefaultKeepsBareImageChunk(t *testing.T) {
	payload := "data:image/png;base64," + pngBase64
	for _, tc := range titleImageContextComponents(t, nil) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := invokeTitleFamily(t, tc.comp, titleImageContextItems())
			ck := findImageChunk(t, chunks)
			if got, _ := ck["text"].(string); got != "" {
				t.Errorf("default image chunk text = %q, want empty (no context configured)", got)
			}
			if ck["image"] != payload {
				t.Errorf("image payload lost: %v", ck["image"])
			}
		})
	}
}

// TestTitleFamilyAttachesTableContext mirrors the image case for tables: a
// table sliced by the title family must carry its surrounding context when a
// table budget is configured.
func TestTitleFamilyAttachesTableContext(t *testing.T) {
	items := []map[string]any{
		{"text": "# Report", "doc_type_kwd": "text"},
		{"text": "alpha before text", "doc_type_kwd": "text"},
		{"text": "<table><tr><td>A</td></tr></table>", "doc_type_kwd": "table"},
		{"text": "beta after text", "doc_type_kwd": "text"},
	}
	for _, tc := range titleImageContextComponents(t, map[string]any{
		"table_context_size": 20,
	}) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := invokeTitleFamily(t, tc.comp, items)
			var table map[string]any
			for _, ck := range chunks {
				if ck["doc_type_kwd"] == "table" {
					table = ck
					break
				}
			}
			if table == nil {
				t.Fatalf("no table chunk emitted (chunks: %#v)", chunks)
			}
			text, _ := table["text"].(string)
			if !strings.Contains(text, "alpha before text") || !strings.Contains(text, "beta after text") {
				t.Errorf("table chunk text = %q, want surrounding context", text)
			}
		})
	}
}

// TestTitleFamilyPreservesImgID checks the typed round-trip used to attach
// context keeps an upstream image reference intact: an image item that already
// carries img_id (no inline bytes) must still carry it afterwards.
func TestTitleFamilyPreservesImgID(t *testing.T) {
	items := []map[string]any{
		{"text": "# Report", "doc_type_kwd": "text"},
		{"text": "alpha before text", "doc_type_kwd": "text"},
		{"text": "", "img_id": "kb-abc", "doc_type_kwd": "image"},
		{"text": "beta after text", "doc_type_kwd": "text"},
	}
	for _, tc := range titleImageContextComponents(t, map[string]any{
		"image_context_size": 20,
	}) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := invokeTitleFamily(t, tc.comp, items)
			ck := findImageChunk(t, chunks)
			if ck["img_id"] != "kb-abc" {
				t.Errorf("img_id = %v, want kb-abc", ck["img_id"])
			}
			text, _ := ck["text"].(string)
			if !strings.Contains(text, "alpha before text") {
				t.Errorf("image chunk text = %q, want context", text)
			}
		})
	}
}

// TestTitleChunkerDispatcherAttachesImageContext covers the wrapper component
// (TitleChunker routes to group/hierarchy) rather than only the standalone
// variants.
func TestTitleChunkerDispatcherAttachesImageContext(t *testing.T) {
	for _, method := range []string{"group", "hierarchy"} {
		t.Run(method, func(t *testing.T) {
			params := map[string]any{
				"method":             method,
				"levels":             [][]string{{`^# `}},
				"image_context_size": 20,
			}
			if method == "hierarchy" {
				params["hierarchy"] = 1
			}
			comp, err := NewTitleChunker(params)
			if err != nil {
				t.Fatalf("NewTitleChunker: %v", err)
			}
			chunks := invokeTitleFamily(t, comp, titleImageContextItems())
			ck := findImageChunk(t, chunks)
			text, _ := ck["text"].(string)
			if !strings.Contains(text, "alpha before text") {
				t.Errorf("image chunk text = %q, want context", text)
			}
		})
	}
}

// TestTitleFamilyContextLeavesTextChunksUnchanged pins the wire contract: a
// text-only chunk is identical whether context is configured or not, so the
// enabled path only changes media chunks.
func TestTitleFamilyContextLeavesTextChunksUnchanged(t *testing.T) {
	items := []map[string]any{
		{"text": "# Report", "doc_type_kwd": "text"},
		{"text": "body paragraph", "doc_type_kwd": "text"},
	}
	plain, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	withContext, err := NewGroupTitleChunker(map[string]any{
		"levels":             [][]string{{`^# `}},
		"image_context_size": 20,
		"table_context_size": 20,
	})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	want := invokeTitleFamily(t, plain, items)
	got := invokeTitleFamily(t, withContext, items)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("text-only chunks changed with context enabled:\n got %#v\nwant %#v", got, want)
	}
}

// TestContextSizeRejectsNegativeFractions pins the fix for values such as
// -0.5, which int() truncates to 0 and would otherwise pass the non-negative
// validation while silently disabling the setting.
func TestContextSizeRejectsNegativeFractions(t *testing.T) {
	if _, err := NewGroupTitleChunker(map[string]any{
		"levels":             [][]string{{`^# `}},
		"image_context_size": -0.5,
	}); err == nil {
		t.Error("TitleChunker accepted image_context_size=-0.5")
	}
	if _, err := NewTokenChunker(map[string]any{
		"delimiter_mode":     "delimiter",
		"chunk_token_size":   512,
		"table_context_size": -0.5,
	}); err == nil {
		t.Error("TokenChunker accepted table_context_size=-0.5")
	}
}
