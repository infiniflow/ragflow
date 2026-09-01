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
	"encoding/json"
	"net/http"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/task"
)

func dataflowCanvas(id, userID string) *entity.UserCanvas {
	cv := makeWebhookCanvas(id, userID, "Webhook", nil)
	cv.CanvasCategory = "dataflow_canvas"
	return cv
}

func decodeSuccess(t *testing.T, body []byte) (int, string, []map[string]any, string) {
	t.Helper()
	var resp struct {
		Code int `json:"code"`
		Data struct {
			MessageID string           `json:"message_id"`
			Chunks    []map[string]any `json:"chunks"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
	return resp.Code, resp.Data.MessageID, resp.Data.Chunks, resp.Message
}

// TestAgentChatCompletions_DataFlowReturnsChunks pins that the existing
// chat/completions endpoint serves the pipeline debug (dry-run) for DataFlow
// canvases: it returns the parsed chunks inline, synchronously, without a
// dedicated debug route. The webhook entry point is intentionally NOT a
// debug surface — see TestWebhook_DataFlowRejected in agent_webhook_test.go.
func TestAgentChatCompletions_DataFlowReturnsChunks(t *testing.T) {
	h := &AgentHandler{
		loader:      &fakeCanvasLoader{canvas: dataflowCanvas("c1", "u-1")},
		fileService: &fakeAgentFileService{blob: []byte("file-bytes")},
	}
	h.WithNewExecutor(func(_ *task.TaskContext, _ string, _ int) (debugExecutor, error) {
		return &fakeDebugExecutor{result: &task.PipelineResult{Chunks: []map[string]any{{"text": "dbg-chunk"}}}}, nil
	})
	// runCanvasPipelineDebug flushes the debug log via redisStore; inject a no-op
	// store so the (skipped in this test) log write does not nil-panic.
	h.WithRedisStore(&capturedStore{})

	// The web contract sends the file list wrapped one level:
	// `files: [[{id, name}]]` (see use-run-dataflow.ts:34). The pipeline
	// debug path unwraps both layers and downloads the referenced bytes.
	c, w := webhookCtx("POST", "/api/v1/agents/c1/chat/completions",
		`{"agent_id":"c1","files":[[{"id":"f1","name":"doc.txt"}]]}`, "application/json")

	h.AgentChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	code, msgID, data, msg := decodeSuccess(t, w.Body.Bytes())
	if code != int(common.CodeSuccess) {
		t.Errorf("code = %d, want %d", code, common.CodeSuccess)
	}
	if msg != "success" {
		t.Errorf("message = %q, want %q", msg, "success")
	}
	if msgID == "" {
		t.Errorf("message_id empty; front-end needs it to poll the debug log")
	}
	if len(data) != 1 || data[0]["text"] != "dbg-chunk" {
		t.Errorf("data.chunks = %#v, want one chunk with text %q", data, "dbg-chunk")
	}
}
