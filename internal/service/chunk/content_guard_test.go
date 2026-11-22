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

package chunk

import (
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/service"
)

// TestExternalImageSourcesRecognizesBothMarkupShapes keeps the guard aligned
// with what a chunk can carry: Markdown images and raw <img> tags.
func TestExternalImageSourcesRecognizesBothMarkupShapes(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{"markdown remote", "see ![alt](https://cdn.example.com/a.png) here", []string{"https://cdn.example.com/a.png"}},
		{"markdown angle brackets", "![alt](<http://example.com/b.png>)", []string{"http://example.com/b.png"}},
		{"html quoted", `<p>x</p><img class="a" src="https://example.com/c.png">`, []string{"https://example.com/c.png"}},
		{"html unquoted", `<img src=https://example.com/d.png>`, []string{"https://example.com/d.png"}},
		{"data uri", "![alt](data:image/png;base64,AAAA)", []string{"data:image/png;base64,AAAA"}},
		{"local path kept", "![alt](images/fig-1.png)", nil},
		{"plain link kept", "[doc](https://example.com/page)", nil},
		{"no image", "plain text with a URL https://example.com/x", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := externalImageSources(tc.content)
			if len(got) != len(tc.want) {
				t.Fatalf("sources = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("sources = %#v, want %#v", got, tc.want)
				}
			}
		})
	}
}

// TestValidateEditedContentOnlyRejectsNewSources pins the grandfather rule: a
// chunk that already holds a remote reference stays editable, but the edit may
// not add another one.
func TestValidateEditedContentOnlyRejectsNewSources(t *testing.T) {
	previous := "text ![old](http://legacy.example.com/x.png)"
	if err := validateEditedContent(previous, "edited "+previous); err != nil {
		t.Fatalf("reusing the stored source must be allowed, got %v", err)
	}
	err := validateEditedContent(previous, previous+" ![new](https://cdn.example.com/y.png)")
	var coded interface{ Code() common.ErrorCode }
	if !errors.As(err, &coded) || coded.Code() != common.CodeArgumentError {
		t.Fatalf("new external source error = %v, want CodeArgumentError", err)
	}
	if !strings.Contains(err.Error(), "image_base64") {
		t.Fatalf("error = %v, want it to name the upload path", err)
	}
}

// TestValidateEditedContentTruncatesLongSource keeps a data URI from being
// echoed back in full.
func TestValidateEditedContentTruncatesLongSource(t *testing.T) {
	uri := "data:image/png;base64," + strings.Repeat("A", maxReportedImageSource*2)
	err := validateEditedContent("", "![x]("+uri+")")
	if err == nil {
		t.Fatal("a data URI must be rejected")
	}
	if len(err.Error()) > maxReportedImageSource+200 {
		t.Fatalf("error message is %d bytes, want the source truncated", len(err.Error()))
	}
}

// newContentGuardTestService builds the service the editing tests need, with
// the storage side stubbed out.
func newContentGuardTestService(engine *updateChunkTestEngine) *ChunkService {
	svc := &ChunkService{
		docEngine:   engine,
		kbDAO:       dao.NewKnowledgebaseDAO(),
		documentDAO: dao.NewDocumentDAO(),
		// The analyzer pool is not started in unit tests, and the edit path now
		// refuses to store text without its tokens, so the seams answer instead.
		tokenizeFunc:            func(text, _ string) (string, error) { return text, nil },
		fineGrainedTokenizeFunc: func(text, _ string) (string, error) { return text, nil },
	}
	svc.removeChunkImageFunc = func(string, string) error { return nil }
	return svc
}

// TestUpdateChunkKeepsPositionsWhenContentChanges pins the source-region
// invariant: the coordinates say where the chunk was parsed from, not what it
// currently says, so rewriting the text leaves them where the parser put them.
func TestUpdateChunkKeepsPositionsWhenContentChanges(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "old body",
		"position_int":        []interface{}{[]interface{}{1.0, 0.0, 100.0, 0.0, 20.0}},
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)

	body := "new body"
	if err := svc.UpdateChunk(chunkTestContext(t, "user-1", "tenant-1"), &service.UpdateChunkRequest{
		DatasetID:  "kb-1",
		DocumentID: "doc-a",
		ChunkID:    "chunk-1",
		Content:    &body,
	}); err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}

	if value, ok := engine.updateCalls[0].newValue["position_int"]; ok {
		t.Fatalf("position_int = %#v, want the stored coordinates left alone", value)
	}
}

// TestUpdateChunkKeepsPositionsWhenSentOrUnchanged covers the write side of the
// same rule: coordinates change only when the caller sends coordinates, and an
// edit that leaves the text alone does not write them either way.
func TestUpdateChunkKeepsPositionsWhenSentOrUnchanged(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "old body",
		"position_int":        []interface{}{[]interface{}{1.0, 0.0, 100.0, 0.0, 20.0}},
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)
	ctx := chunkTestContext(t, "user-1", "tenant-1")

	body := "new body"
	sent := []interface{}{[]interface{}{2.0, 0.0, 100.0, 30.0, 50.0}}
	if err := svc.UpdateChunk(ctx, &service.UpdateChunkRequest{
		DatasetID:  "kb-1",
		DocumentID: "doc-a",
		ChunkID:    "chunk-1",
		Content:    &body,
		Positions:  sent,
	}); err != nil {
		t.Fatalf("UpdateChunk with positions: %v", err)
	}
	got, ok := engine.updateCalls[0].newValue["position_int"].([]interface{})
	if !ok || len(got) != 1 {
		t.Fatalf("position_int = %#v, want the caller's coordinates", engine.updateCalls[0].newValue["position_int"])
	}

	available := false
	if err := svc.UpdateChunk(ctx, &service.UpdateChunkRequest{
		DatasetID:  "kb-1",
		DocumentID: "doc-a",
		ChunkID:    "chunk-1",
		Available:  &available,
	}); err != nil {
		t.Fatalf("UpdateChunk without content: %v", err)
	}
	if _, ok := engine.updateCalls[1].newValue["position_int"]; ok {
		t.Fatalf("a non-content edit must leave position_int untouched, got %#v", engine.updateCalls[1].newValue["position_int"])
	}
}

// TestUpdateChunkRejectsNewExternalImage covers the three shapes the edit API
// refuses and the fact that a refused edit never reaches the engine.
func TestUpdateChunkRejectsNewExternalImage(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "old body",
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)

	bodies := []string{
		"see ![x](https://cdn.example.com/a.png)",
		`<img src="http://example.com/b.png">`,
		"![x](data:image/png;base64,AAAA)",
	}
	for _, body := range bodies {
		content := body
		err := svc.UpdateChunk(chunkTestContext(t, "user-1", "tenant-1"), &service.UpdateChunkRequest{
			DatasetID:  "kb-1",
			DocumentID: "doc-a",
			ChunkID:    "chunk-1",
			Content:    &content,
		})
		var coded interface{ Code() common.ErrorCode }
		if !errors.As(err, &coded) || coded.Code() != common.CodeArgumentError {
			t.Fatalf("%q: error = %v, want CodeArgumentError", body, err)
		}
	}
	if len(engine.updateCalls) != 0 {
		t.Fatalf("engine writes = %d, want 0 for rejected edits", len(engine.updateCalls))
	}
}

// TestUpdateChunkImageRemoveKeepsDocType pins the fix to the removal branch: a
// table that carried a preview image is still a table afterwards.
func TestUpdateChunkImageRemoveKeepsDocType(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "| a | b |",
		"doc_type_kwd":        "table",
		"img_id":              "kb-1-chunk-1",
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)

	mode := imageUpdateModeRemove
	if err := svc.UpdateChunk(chunkTestContext(t, "user-1", "tenant-1"), &service.UpdateChunkRequest{
		DatasetID:       "kb-1",
		DocumentID:      "doc-a",
		ChunkID:         "chunk-1",
		ImageUpdateMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}

	update := engine.updateCalls[0].newValue
	if got := update["doc_type_kwd"]; got != "table" {
		t.Fatalf("doc_type_kwd = %v, want table", got)
	}
	if got := update["img_id"]; got != "" {
		t.Fatalf("img_id = %v, want it cleared", got)
	}
	if _, ok := update["available_int"]; ok {
		t.Fatalf("a table with text must stay available, got %#v", update["available_int"])
	}
}

// TestUpdateChunkImageRemoveDeactivatesEmptyBlock keeps an image-only chunk from
// surviving as an empty, still-searchable hit; an explicit `available` wins.
func TestUpdateChunkImageRemoveDeactivatesEmptyBlock(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "   ",
		"doc_type_kwd":        "image",
		"img_id":              "kb-1-chunk-1",
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)
	ctx := chunkTestContext(t, "user-1", "tenant-1")

	mode := imageUpdateModeRemove
	if err := svc.UpdateChunk(ctx, &service.UpdateChunkRequest{
		DatasetID:       "kb-1",
		DocumentID:      "doc-a",
		ChunkID:         "chunk-1",
		ImageUpdateMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}
	update := engine.updateCalls[0].newValue
	if got := update["available_int"]; got != 0 {
		t.Fatalf("available_int = %v, want 0 for an empty block", got)
	}
	if got := update["doc_type_kwd"]; got != "text" {
		t.Fatalf("doc_type_kwd = %v, want text for a degraded image block", got)
	}

	keep := true
	if err := svc.UpdateChunk(ctx, &service.UpdateChunkRequest{
		DatasetID:       "kb-1",
		DocumentID:      "doc-a",
		ChunkID:         "chunk-1",
		ImageUpdateMode: &mode,
		Available:       &keep,
	}); err != nil {
		t.Fatalf("UpdateChunk with available: %v", err)
	}
	if got, ok := engine.updateCalls[1].newValue["available_int"]; ok && got == 0 {
		t.Fatalf("an explicit available=true must win, got %v", got)
	}
}

// TestUpdateChunkKeepsDerivedPdfPositions pins that the whole PDF triplet is
// left alone: page_num_int and top_int are derived from the same matrix as
// position_int, so writing one without the others would desynchronise them.
func TestUpdateChunkKeepsDerivedPdfPositions(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "old body",
		"position_int":        []interface{}{[]interface{}{1.0, 0.0, 100.0, 0.0, 20.0}},
		"page_num_int":        []interface{}{float64(1)},
		"top_int":             []interface{}{float64(20)},
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)

	body := "new body"
	if err := svc.UpdateChunk(chunkTestContext(t, "user-1", "tenant-1"), &service.UpdateChunkRequest{
		DatasetID:  "kb-1",
		DocumentID: "doc-a",
		ChunkID:    "chunk-1",
		Content:    &body,
	}); err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}

	update := engine.updateCalls[0].newValue
	for _, field := range []string{"position_int", "page_num_int", "top_int"} {
		if value, ok := update[field]; ok {
			t.Fatalf("%s = %#v, want the stored coordinates left alone", field, value)
		}
	}
}

// TestUpdateChunkKeepsRowIndexWithoutPdfPage covers the two chunk kinds whose
// top_int is not a PDF offset: a spreadsheet block (position_int, no
// page_num_int, top_int = row index) and a QA block (top_int only). Neither is
// written on an edit, so neither can be clobbered by the other's shape.
func TestUpdateChunkKeepsRowIndexWithoutPdfPage(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "| a | b |",
		"position_int":        []interface{}{[]interface{}{1.0, 2.0, 3.0, 4.0, 5.0}},
		"top_int":             float64(7),
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)

	body := "| a | c |"
	if err := svc.UpdateChunk(chunkTestContext(t, "user-1", "tenant-1"), &service.UpdateChunkRequest{
		DatasetID:  "kb-1",
		DocumentID: "doc-a",
		ChunkID:    "chunk-1",
		Content:    &body,
	}); err != nil {
		t.Fatalf("UpdateChunk: %v", err)
	}

	update := engine.updateCalls[0].newValue
	if value, ok := update["position_int"]; ok {
		t.Fatalf("position_int = %#v, want the cell range left alone", value)
	}
	if value, ok := update["top_int"]; ok {
		t.Fatalf("top_int = %#v, want the spreadsheet row index kept", value)
	}
}

// TestUpdateChunkRejectsOversizedContent keeps a pasted page from being stored
// as one chunk.
func TestUpdateChunkRejectsOversizedContent(t *testing.T) {
	pushChunkTestDB(t, setupChunkTestDB(t))
	insertChunkTestUserTenant(t, "user-1", "tenant-1")
	insertChunkTestKB(t, "kb-1", "tenant-1")
	insertChunkTestDoc(t, "doc-a", "kb-1")

	engine := &updateChunkTestEngine{existingChunk: map[string]interface{}{
		"doc_id":              "doc-a",
		"content_with_weight": "old body",
		"available_int":       1,
	}}
	svc := newContentGuardTestService(engine)

	body := strings.Repeat("a", maxEditedChunkLength+1)
	err := svc.UpdateChunk(chunkTestContext(t, "user-1", "tenant-1"), &service.UpdateChunkRequest{
		DatasetID:  "kb-1",
		DocumentID: "doc-a",
		ChunkID:    "chunk-1",
		Content:    &body,
	})
	var coded interface{ Code() common.ErrorCode }
	if !errors.As(err, &coded) || coded.Code() != common.CodeArgumentError {
		t.Fatalf("error = %v, want CodeArgumentError", err)
	}
	if len(engine.updateCalls) != 0 {
		t.Fatalf("engine writes = %d, want 0 for a rejected edit", len(engine.updateCalls))
	}
}
