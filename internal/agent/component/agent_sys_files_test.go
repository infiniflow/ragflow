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
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

// The Agent component must attach sys.files uploads to the current user
// message, mirroring Python's LLM._prepare_prompt_variables that the Python
// Agent inherits. Regression test for「智能体附件上传图片后视觉模型收不到图片」:
// buildAgentInputMessages used to send text-only messages, so vision models
// answered "未上传图片".
func TestBuildAgentInputMessagesAttachesSysFileImages(t *testing.T) {
	state := runtime.NewCanvasState("run-agent-files", "task-agent-files")
	state.Sys["files"] = []any{
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUg",
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUg", // duplicate must be deduped
		"uploaded text excerpt",
	}
	ctx := runtime.WithState(t.Context(), state)

	messages := buildAgentInputMessages(ctx, AgentParam{UserPrompt: "描述图片内容"})
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	msg := messages[0]
	if msg.Role != schema.User {
		t.Fatalf("role = %q, want user", msg.Role)
	}
	if msg.Content != "" {
		t.Fatalf("Content = %q, want empty when multi-content parts are set", msg.Content)
	}
	parts := msg.UserInputMultiContent
	if len(parts) != 2 {
		t.Fatalf("part count = %d, want 2 (text + deduped image)", len(parts))
	}
	if parts[0].Type != schema.ChatMessagePartTypeText {
		t.Fatalf("parts[0].Type = %q, want text", parts[0].Type)
	}
	wantText := "描述图片内容\n\nuploaded text excerpt"
	if parts[0].Text != wantText {
		t.Fatalf("parts[0].Text = %q, want %q", parts[0].Text, wantText)
	}
	if parts[1].Type != schema.ChatMessagePartTypeImageURL {
		t.Fatalf("parts[1].Type = %q, want image_url", parts[1].Type)
	}
	if parts[1].Image == nil || parts[1].Image.URL == nil ||
		*parts[1].Image.URL != "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg" {
		t.Fatalf("parts[1].Image.URL = %v, want the data URI", parts[1].Image)
	}
}

func TestBuildAgentInputMessagesTextOnlySysFiles(t *testing.T) {
	state := runtime.NewCanvasState("run-agent-text-files", "task-agent-text-files")
	state.Sys["files"] = []any{"file body one", "file body two"}
	ctx := runtime.WithState(t.Context(), state)

	messages := buildAgentInputMessages(ctx, AgentParam{UserPrompt: "总结这些文件"})
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	want := "总结这些文件\n\nfile body one\n\nfile body two"
	if messages[0].Content != want {
		t.Fatalf("Content = %q, want %q", messages[0].Content, want)
	}
	if len(messages[0].UserInputMultiContent) != 0 {
		t.Fatalf("UserInputMultiContent should stay empty without images, got %#v", messages[0].UserInputMultiContent)
	}
}

func TestBuildAgentInputMessagesWithoutSysFilesUnchanged(t *testing.T) {
	state := runtime.NewCanvasState("run-agent-no-files", "task-agent-no-files")
	ctx := runtime.WithState(t.Context(), state)

	messages := buildAgentInputMessages(ctx, AgentParam{UserPrompt: "你好"})
	if len(messages) != 1 || messages[0].Content != "你好" {
		t.Fatalf("messages = %#v, want a single plain user message", messages)
	}
}
