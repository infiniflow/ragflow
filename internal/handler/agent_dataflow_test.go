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

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/task"
)

// debugResultStub returns a fixed chunk list so the DataFlow entry points can
// be exercised without standing up a real ingestion pipeline.
func debugResultStub(chunks ...map[string]any) func(ctx context.Context, user *entity.User, kbID, canvasID, fileName string, fileData []byte) (*task.PipelineResult, error) {
	return func(ctx context.Context, user *entity.User, kbID, canvasID, fileName string, fileData []byte) (*task.PipelineResult, error) {
		return &task.PipelineResult{Chunks: chunks}, nil
	}
}

func dataflowCanvas(id, userID string) *entity.UserCanvas {
	cv := makeWebhookCanvas(id, userID, "Webhook", nil)
	cv.CanvasCategory = "dataflow_canvas"
	return cv
}

func decodeSuccess(t *testing.T, body []byte) (int, []map[string]any, string) {
	t.Helper()
	var resp struct {
		Code    int              `json:"code"`
		Data    []map[string]any `json:"data"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
	return resp.Code, resp.Data, resp.Message
}

// TestAgentChatCompletions_DataFlowReturnsChunks pins that the existing
// chat/completions endpoint serves the dataflow debug (dry-run) for DataFlow
// canvases: it returns the parsed chunks inline, synchronously, without a
// dedicated dataflow/debug route. kb_id is optional, so an empty one must not
// reject the request. The webhook entry point is intentionally NOT a dataflow
// debug surface — see TestWebhook_DataFlowRejected in agent_webhook_test.go.
func TestAgentChatCompletions_DataFlowReturnsChunks(t *testing.T) {
	h := &AgentHandler{
		loader:      &fakeCanvasLoader{canvas: dataflowCanvas("c1", "u-1")},
		fileService: &fakeAgentFileService{blob: []byte("file-bytes")},
	}
	h.debugRunner = debugResultStub(map[string]any{"text": "dbg-chunk"})

	// The web contract sends the file list wrapped one level:
	// `files: [[{id, name}]]` (see use-run-dataflow.ts:34). The dataflow
	// debug path unwraps both layers and downloads the referenced bytes.
	c, w := webhookCtx("POST", "/api/v1/agents/c1/chat/completions",
		`{"agent_id":"c1","files":[[{"id":"f1","name":"doc.txt"}]]}`, "application/json")

	h.AgentChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	code, data, msg := decodeSuccess(t, w.Body.Bytes())
	if code != int(common.CodeSuccess) {
		t.Errorf("code = %d, want %d", code, common.CodeSuccess)
	}
	if msg != "success" {
		t.Errorf("message = %q, want %q", msg, "success")
	}
	if len(data) != 1 || data[0]["text"] != "dbg-chunk" {
		t.Errorf("data = %#v, want one chunk with text %q", data, "dbg-chunk")
	}
}

// TestAgentChatCompletions_DataFlowWithKbID pins that an explicit kb_id on the
// chat/completions DataFlow debug request is accepted and forwarded to the
// debug runner (used by canvases whose embedding resolution needs it).
func TestAgentChatCompletions_DataFlowWithKbID(t *testing.T) {
	var seenKbID string
	h := &AgentHandler{
		loader:      &fakeCanvasLoader{canvas: dataflowCanvas("c1", "u-1")},
		fileService: &fakeAgentFileService{blob: []byte("file-bytes")},
	}
	h.debugRunner = func(_ context.Context, _ *entity.User, kbID, _, _ string, _ []byte) (*task.PipelineResult, error) {
		seenKbID = kbID
		return &task.PipelineResult{Chunks: []map[string]any{{"text": "dbg-kb"}}}, nil
	}

	c, w := webhookCtx("POST", "/api/v1/agents/c1/chat/completions",
		`{"agent_id":"c1","kb_id":"kb-9","files":[[{"id":"f1","name":"doc.txt"}]]}`, "application/json")

	h.AgentChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if seenKbID != "kb-9" {
		t.Errorf("debug runner kb_id = %q, want %q", seenKbID, "kb-9")
	}
}
