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
	"encoding/base64"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/storage"
)

// TestUploadCodeExecArtifacts pins the artifact upload path: sandbox
// artifacts (name/content_b64/mime_type/size) are stored under a
// fresh object name and surfaced with a downloadable URL, image
// markdown, and an attachment-count content section. Undecodable or
// un-uploadable entries are skipped, matching the Python skip-on-failure
// semantics.
func TestUploadCodeExecArtifacts(t *testing.T) {
	t.Parallel()

	st := storage.NewMemoryStorage()
	artifacts := []any{
		map[string]any{
			"name":        "chart.png",
			"mime_type":   "image/png",
			"size":        float64(9),
			"content_b64": base64.StdEncoding.EncodeToString([]byte("fake-png")),
		},
		map[string]any{
			"name":        "data.csv",
			"mime_type":   "text/csv",
			"size":        float64(5),
			"content_b64": base64.StdEncoding.EncodeToString([]byte("a,b\n1,2")),
		},
		// Undecodable payload must be skipped, not fail the batch.
		map[string]any{"name": "broken.txt", "content_b64": "!!not-base64!!"},
		// Missing payload must be skipped.
		map[string]any{"name": "empty.txt", "content_b64": ""},
	}

	uploaded, markdown, attachmentContent := uploadCodeExecArtifacts(context.Background(), artifacts, "sess-1", st)
	if len(uploaded) != 2 {
		t.Fatalf("uploaded len = %d, want 2", len(uploaded))
	}
	if len(markdown) != 2 {
		t.Fatalf("markdown len = %d, want 2", len(markdown))
	}

	url := uploaded[0]["url"].(string)
	if !strings.HasPrefix(url, "/api/v1/documents/artifact/") {
		t.Errorf("url = %q, want /api/v1/documents/artifact/ prefix", url)
	}
	if !strings.HasSuffix(url, ".png?session_id=sess-1") {
		t.Errorf("url = %q, want .png?session_id=sess-1 suffix", url)
	}
	if uploaded[0]["name"] != "chart.png" || uploaded[0]["mime_type"] != "image/png" {
		t.Errorf("uploaded[0] = %v, want chart.png image/png", uploaded[0])
	}

	if markdown[0] != "!["+"chart.png]("+url+")" {
		t.Errorf("markdown[0] = %q, want image markdown with url", markdown[0])
	}
	if !strings.HasPrefix(markdown[1], "[Download data.csv](") {
		t.Errorf("markdown[1] = %q, want download link", markdown[1])
	}

	if !strings.Contains(attachmentContent, "attachment_count: 2") {
		t.Errorf("attachmentContent = %q, want attachment_count: 2", attachmentContent)
	}
	if !strings.Contains(attachmentContent, "attachment1 (image): chart.png") {
		t.Errorf("attachmentContent = %q, want image section title", attachmentContent)
	}

	// The stored object must actually exist under the bucket with the
	// uuid-based name embedded in the URL.
	storageName := strings.TrimSuffix(strings.TrimPrefix(url, "/api/v1/documents/artifact/"), "?session_id=sess-1")
	if !st.ObjExist(context.Background(), "sandbox-artifacts", storageName) {
		t.Errorf("object %q not found in bucket sandbox-artifacts", storageName)
	}

	// Entries that already carry a hosted url (published by the
	// CodeExec tool) are reused verbatim — no re-upload happens, the
	// markdown points at the tool's canonical URL.
	published, pubMarkdown, pubContent := uploadCodeExecArtifacts(context.Background(), []any{
		map[string]any{
			"name":      "already_hosted.png",
			"url":       "/api/v1/documents/artifact/abc.png",
			"mime_type": "image/png",
			"size":      float64(7),
		},
	}, "sess-1", st)
	if len(published) != 1 {
		t.Fatalf("published len = %d, want 1", len(published))
	}
	if got := published[0]["url"].(string); got != "/api/v1/documents/artifact/abc.png" {
		t.Errorf("published url = %q, want tool URL reused verbatim", got)
	}
	if len(pubMarkdown) != 1 || pubMarkdown[0] != "!["+"already_hosted.png](/api/v1/documents/artifact/abc.png)" {
		t.Errorf("pubMarkdown = %#v, want image markdown from tool URL", pubMarkdown)
	}
	if !strings.Contains(pubContent, "attachment_count: 1") {
		t.Errorf("pubContent = %q, want attachment_count: 1", pubContent)
	}
}

// fakeSandboxClient adapts a function literal to the CodeExec
// SandboxClient interface.
type fakeSandboxClient func(ctx context.Context, req agenttool.SandboxRequest) (*agenttool.SandboxResponse, error)

func (f fakeSandboxClient) ExecuteCode(ctx context.Context, req agenttool.SandboxRequest) (*agenttool.SandboxResponse, error) {
	return f(ctx, req)
}

// TestCodeExecComponentInvokeAttachesArtifacts verifies the canvas
// CodeExec node turns sandbox-returned artifacts into message
// attachments: decoded["_ARTIFACTS"] carries upload URLs,
// decoded["attachments"] is populated, and content is appended with
// the attachment sections.
func TestCodeExecComponentInvokeAttachesArtifacts(t *testing.T) {
	prevClient := agenttool.GetSandboxClient()
	agenttool.SetSandboxClient(fakeSandboxClient(func(_ context.Context, _ agenttool.SandboxRequest) (*agenttool.SandboxResponse, error) {
		return &agenttool.SandboxResponse{
			ExitCode: 0,
			Metadata: map[string]any{
				"artifacts": []any{
					map[string]any{
						"name":        "simple_plot.png",
						"mime_type":   "image/png",
						"size":        float64(10),
						"content_b64": base64.StdEncoding.EncodeToString([]byte("png-bytes")),
					},
				},
			},
		}, nil
	}))
	t.Cleanup(func() { agenttool.SetSandboxClient(prevClient) })

	factory := storage.GetStorageFactory()
	prevStorage := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(prevStorage) })

	comp, err := newCodeExecComponent(nil)
	if err != nil {
		t.Fatalf("newCodeExecComponent: %v", err)
	}

	ctx := runtime.WithState(context.Background(), runtime.NewCanvasState("run-1", "sess-test"))
	decoded, err := comp.Invoke(ctx, nil, map[string]any{
		"lang":   "python",
		"script": "def main():\n    return {\"ok\": True}\n",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	artifacts, ok := decoded["_ARTIFACTS"].([]map[string]any)
	if !ok || len(artifacts) != 1 {
		t.Fatalf("_ARTIFACTS = %#v, want 1 uploaded artifact", decoded["_ARTIFACTS"])
	}
	entry := artifacts[0]
	url, _ := entry["url"].(string)
	// The CodeExec tool hosts the blob and publishes the canonical
	// /api/v1/documents/artifact/<name> URL (no session_id suffix);
	// the component must consume it as-is.
	if !strings.HasPrefix(url, "/api/v1/documents/artifact/") || !strings.HasSuffix(url, ".png") || strings.Contains(url, "?") {
		t.Errorf("artifact url = %q, want hosted /api/v1/documents/artifact/<name>.png URL", url)
	}

	attachments, ok := decoded["attachments"].([]string)
	if !ok || len(attachments) != 1 {
		t.Fatalf("attachments = %#v, want 1 markdown entry", decoded["attachments"])
	}
	if attachments[0] != "!["+"simple_plot.png]("+url+")" {
		t.Errorf("attachments[0] = %q, want image markdown", attachments[0])
	}

	content, _ := decoded["content"].(string)
	if !strings.Contains(content, "attachment_count: 1") {
		t.Errorf("content = %q, want attachment_count section appended", content)
	}
}
