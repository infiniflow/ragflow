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

package component

import (
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/storage"
)

// TestMessage_ExportAttachment mirrors the regression behind the
// "download file type selected but no download button" report: a
// Message with output_format=md must surface outputs["attachment"]
// with {doc_id, format, file_name} and persist the file in the
// tenant-downloads bucket, exactly like Python's _convert_content.
func TestMessage_ExportAttachment(t *testing.T) {
	storageFactory := storage.GetStorageFactory()
	prevStorage := storageFactory.GetStorage()
	memStorage := storage.NewMemoryStorage()
	storageFactory.SetStorage(memStorage)
	t.Cleanup(func() { storageFactory.SetStorage(prevStorage) })

	c, err := New(componentNameMessage, map[string]any{
		"text":          "# Report\n\nHello **world**",
		"output_format": "md",
	})
	if err != nil {
		t.Fatalf("New(Message): %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	attachment, ok := out["attachment"].(map[string]any)
	if !ok {
		t.Fatalf("attachment = %#v, want a descriptor map (download button depends on it)", out["attachment"])
	}
	docID, _ := attachment["doc_id"].(string)
	if docID == "" {
		t.Fatalf("attachment doc_id empty: %#v", attachment)
	}
	if attachment["format"] != "markdown" {
		t.Errorf("format = %v, want markdown (md is normalized)", attachment["format"])
	}
	fileName, _ := attachment["file_name"].(string)
	if !strings.HasPrefix(fileName, docID[:8]+".markdown") {
		t.Errorf("file_name = %q, want <docID[:8]>.markdown", fileName)
	}

	blob, err := memStorage.Get(ctx, "tenant-1-downloads", docID)
	if err != nil {
		t.Fatalf("stored blob missing: %v", err)
	}
	if !strings.Contains(string(blob), "Hello **world**") {
		t.Errorf("stored blob = %q, want the exported markdown content", string(blob))
	}
}

// TestMessage_ExportAttachmentXlsx covers the Excel branch: markdown
// tables land in sheets inside the stored workbook.
func TestMessage_ExportAttachmentXlsx(t *testing.T) {
	storageFactory := storage.GetStorageFactory()
	prevStorage := storageFactory.GetStorage()
	memStorage := storage.NewMemoryStorage()
	storageFactory.SetStorage(memStorage)
	t.Cleanup(func() { storageFactory.SetStorage(prevStorage) })

	c, err := New(componentNameMessage, map[string]any{
		"text":          "| K | V |\n|---|---|\n| a | 1 |",
		"output_format": "xlsx",
	})
	if err != nil {
		t.Fatalf("New(Message): %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	attachment, ok := out["attachment"].(map[string]any)
	if !ok {
		t.Fatalf("attachment = %#v, want a descriptor map", out["attachment"])
	}
	docID, _ := attachment["doc_id"].(string)
	if attachment["format"] != "xlsx" {
		t.Errorf("format = %v, want xlsx", attachment["format"])
	}
	blob, err := memStorage.Get(ctx, "tenant-1-downloads", docID)
	if err != nil {
		t.Fatalf("stored blob missing: %v", err)
	}
	// XLSX workbooks are ZIP archives and start with the PK magic.
	if len(blob) < 4 || string(blob[:2]) != "PK" {
		t.Errorf("stored blob is not a ZIP/XLSX payload (first bytes: %x)", blob[:4])
	}
}

// TestMessage_ExportAttachmentHTML pins the html branch conversion
// contract: the exported file is produced from the resolved markdown
// (converted to an HTML fragment, as pypandoc does in Python), not
// from the already-escaped output_format rendering — headings and
// emphasis must survive as real HTML instead of literal text.
func TestMessage_ExportAttachmentHTML(t *testing.T) {
	storageFactory := storage.GetStorageFactory()
	prevStorage := storageFactory.GetStorage()
	memStorage := storage.NewMemoryStorage()
	storageFactory.SetStorage(memStorage)
	t.Cleanup(func() { storageFactory.SetStorage(prevStorage) })

	c, err := New(componentNameMessage, map[string]any{
		"text":          "# Report\n\nHello **world**",
		"output_format": "html",
	})
	if err != nil {
		t.Fatalf("New(Message): %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := canvas.WithState(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	attachment, ok := out["attachment"].(map[string]any)
	if !ok {
		t.Fatalf("attachment = %#v, want a descriptor map", out["attachment"])
	}
	if attachment["format"] != "html" {
		t.Errorf("format = %v, want html", attachment["format"])
	}
	docID, _ := attachment["doc_id"].(string)
	if docID == "" {
		t.Fatalf("attachment doc_id empty: %#v", attachment)
	}

	blob, err := memStorage.Get(ctx, "tenant-1-downloads", docID)
	if err != nil {
		t.Fatalf("stored blob missing: %v", err)
	}
	html := string(blob)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Errorf("exported blob is not a full HTML document")
	}
	for _, want := range []string{"<h1>Report</h1>", "<strong>world</strong>"} {
		if !strings.Contains(html, want) {
			t.Errorf("exported html = %q, want it to contain %q (markdown converted, not literal text)", html, want)
		}
	}
	for _, banned := range []string{"&lt;h1&gt;", "&lt;strong&gt;", "# Report"} {
		if strings.Contains(html, banned) {
			t.Errorf("exported html = %q, must not contain escaped/literal markdown %q", html, banned)
		}
	}
}

// TestMessage_NoExportWithoutFileFormat ensures plain/markdown text
// rendering formats do not fabricate attachments.
func TestMessage_NoExportWithoutFileFormat(t *testing.T) {
	c, err := New(componentNameMessage, map[string]any{
		"text": "plain hello",
	})
	if err != nil {
		t.Fatalf("New(Message): %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := withStateForTest(t.Context(), state)

	out, err := c.Invoke(ctx, nil, map[string]any{"stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, ok := out["attachment"]; ok {
		t.Errorf("attachment = %#v, want absent for plain rendering", out["attachment"])
	}
}
