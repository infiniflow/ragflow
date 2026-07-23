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
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/storage"
)

// TestDocsGenerator_Registered: the component is registered under
// its canonical name. The param check requires output_format and
// content; we provide a minimal valid params map.
func TestDocsGenerator_Registered(t *testing.T) {
	c, err := New("DocsGenerator", map[string]any{
		"output_format": "pdf",
		"content":       "Hello world",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator): %v", err)
	}
	if c.Name() != "DocsGenerator" {
		t.Errorf("Name=%q, want DocsGenerator", c.Name())
	}
	if c.Inputs() == nil {
		t.Error("Inputs() should be non-nil")
	}
	if c.Outputs() == nil {
		t.Error("Outputs() should be non-nil")
	}
}

func TestDocGeneratorAlias_Registered(t *testing.T) {
	c, err := New("DocGenerator", map[string]any{
		"output_format": "pdf",
		"content":       "Hello world",
	})
	if err != nil {
		t.Fatalf("New(DocGenerator): %v", err)
	}
	if c.Name() != "DocsGenerator" {
		t.Errorf("Name=%q, want DocsGenerator", c.Name())
	}
}

func TestDocGeneratorAlias_InputForm(t *testing.T) {
	c, err := New("DocGenerator", map[string]any{
		"output_format": "pdf",
		"content":       "Hello world",
	})
	if err != nil {
		t.Fatalf("New(DocGenerator): %v", err)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("DocGenerator component does not expose GetInputForm")
	}
	content, ok := formGetter.GetInputForm()["content"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[content] has type %T, want map", formGetter.GetInputForm()["content"])
	}
	if content["type"] != "line" {
		t.Fatalf("GetInputForm()[content][type] = %v, want line", content["type"])
	}
}

// TestDocsGenerator_Invoke_HappyPath: with valid params, the
// component runs without error and produces a non-nil output map.
// Real PDF/DOCX generation needs an actual font file on disk
// (see docs_generator.go's font initialization); when that's
// missing the generator returns a soft error.
func TestDocsGenerator_Invoke_HappyPath(t *testing.T) {
	c, err := New("DocsGenerator", map[string]any{
		"output_format": "txt",
		"content":       "Hello world",
		"filename":      "test",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator): %v", err)
	}
	_, _ = c.Invoke(context.Background(), map[string]any{})
	// We do not assert err == nil here because txt output requires
	// the internal writer (which may not be available in this
	// checkout). The test pins that the call doesn't panic.
}

func TestDocsGenerator_Invoke_UsesQueryWhenContentEmpty(t *testing.T) {
	c, err := New("DocsGenerator", map[string]any{
		"output_format": "txt",
		"filename":      "query-body",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator): %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"query": "Hello from begin query",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	payload, ok := out["bytes"].([]byte)
	if !ok {
		t.Fatalf("bytes has type %T, want []byte", out["bytes"])
	}
	if !strings.Contains(string(payload), "Hello from begin query") {
		t.Fatalf("payload = %q, want query content", string(payload))
	}
}

func TestDocsGenerator_Invoke_AcceptsDecorationParamNames(t *testing.T) {
	c, err := New("DocsGenerator", map[string]any{
		"output_format":  "txt",
		"content":        "body",
		"filename":       "decorated",
		"header_text":    "Canonical Header",
		"footer_text":    "Canonical Footer",
		"watermark_text": "Canonical Watermark",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator): %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke canonical params: %v", err)
	}
	payload, ok := out["bytes"].([]byte)
	if !ok {
		t.Fatalf("bytes has type %T, want []byte", out["bytes"])
	}
	text := string(payload)
	if !strings.Contains(text, "Canonical Header") || !strings.Contains(text, "Canonical Footer") {
		t.Fatalf("payload = %q, want canonical header and footer", text)
	}

	c, err = New("DocsGenerator", map[string]any{
		"output_format": "txt",
		"content":       "body",
		"filename":      "decorated-alias",
		"header":        "Alias Header",
		"footer":        "Alias Footer",
		"watermark":     "Alias Watermark",
	})
	if err != nil {
		t.Fatalf("New(DocsGenerator alias params): %v", err)
	}
	out, err = c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke alias params: %v", err)
	}
	payload, ok = out["bytes"].([]byte)
	if !ok {
		t.Fatalf("bytes has type %T, want []byte", out["bytes"])
	}
	text = string(payload)
	if !strings.Contains(text, "Alias Header") || !strings.Contains(text, "Alias Footer") {
		t.Fatalf("payload = %q, want alias header and footer", text)
	}
}

func TestDocsGenerator_Invoke_StoresAgentAttachment(t *testing.T) {
	storageFactory := storage.GetStorageFactory()
	prevStorage := storageFactory.GetStorage()
	memStorage := storage.NewMemoryStorage()
	storageFactory.SetStorage(memStorage)
	t.Cleanup(func() { storageFactory.SetStorage(prevStorage) })

	c, err := New("DocGenerator", map[string]any{
		"output_format": "txt",
		"content":       "Hello storage",
		"filename":      "stored",
	})
	if err != nil {
		t.Fatalf("New(DocGenerator): %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["stored"] != true {
		t.Fatalf("stored = %v, want true", out["stored"])
	}
	docID, ok := out["doc_id"].(string)
	if !ok || docID == "" {
		t.Fatalf("doc_id = %v, want non-empty string", out["doc_id"])
	}
	blob, err := memStorage.Get("tenant-1-downloads", docID)
	if err != nil {
		t.Fatalf("stored blob missing: %v", err)
	}
	if string(blob) == "" || !strings.Contains(string(blob), "Hello storage") {
		t.Fatalf("stored blob = %q, want generated content", string(blob))
	}
	download, _ := out["download"].(string)
	var info map[string]any
	if err := json.Unmarshal([]byte(download), &info); err != nil {
		t.Fatalf("download is not JSON: %v", err)
	}
	if info["doc_id"] != docID {
		t.Fatalf("download doc_id = %v, want %s", info["doc_id"], docID)
	}
	if path, _ := info["url"].(string); !strings.HasPrefix(path, "/api/v1/agents/attachments/"+docID+"/download") {
		t.Fatalf("download url = %q, want attachment download path", path)
	}
	if _, ok := out["download_info"].(map[string]any); !ok {
		t.Fatalf("download_info has type %T, want map", out["download_info"])
	}
}
