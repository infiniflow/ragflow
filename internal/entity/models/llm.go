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

// Package models — EinoChatModel thin wrapper (Phase 2 P0, plan §2.11.6 D1).
//
// Bridges the existing RAGFlow provider-specific *ChatModel (OpenAI, Anthropic,
// Gemini, …) to eino's model.BaseChatModel / model.ToolCallingChatModel
// interface so the ReAct agent (internal/agent/component/agent.go) can
// consume it directly. The wrapper does NOT reimplement provider logic — it
// translates eino's []schema.Message + model.Option into the existing
// ChatModel + APIConfig + ChatConfig call shape, and converts the
// *ChatResponse back into a *schema.Message.
//
// Why a separate file: the plan forbids editing existing files in this
// package (types.go, dummy.go, openai.go, …). Adding llm.go keeps the bridge
// self-contained and easy to remove if/when providers get first-class eino
// adapters.
package models

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// EinoChatModel adapts a RAGFlow *ChatModel to eino's chat model interfaces.
// It is safe for concurrent use: all per-request state lives in the
// receiver's fields which are only mutated through WithTools (which returns
// a new instance, never mutating in place — see eino's
// components/model/interface.go:84-99 for the rationale).
type EinoChatModel struct {
	inner   *ChatModel
	chatCfg *ChatConfig
	tools   []*schema.ToolInfo
}

// NewEinoChatModel wraps an existing RAGFlow *ChatModel so it can be passed
// to eino constructs (ReAct agent, Workflow, etc.). The chatConfig argument
// carries temperature / max_tokens / etc. — pass nil for provider defaults.
//
// Driver is taken from cm.ModelDriver, model name from cm.ModelName, and
// API key / region from cm.APIConfig. These are fixed for the lifetime of
// the wrapper; per-request variations belong in WithTools / a new instance.
func NewEinoChatModel(cm *ChatModel, chatConfig *ChatConfig) *EinoChatModel {
	return &EinoChatModel{
		inner:   cm,
		chatCfg: chatConfig,
	}
}

// name returns the underlying model name (best-effort; nil-safe).
func (m *EinoChatModel) name() string {
	if m == nil || m.inner == nil || m.inner.ModelName == nil {
		return ""
	}
	return *m.inner.ModelName
}

// toInternalMessages converts eino's []schema.Message into the existing
// RAGFlow []Message type. System / user / assistant roles are preserved;
// tool-role messages are mapped to "tool" (the existing model layer already
// speaks that string — see types.go:9).
func toInternalMessages(msgs []*schema.Message) []Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]Message, 0, len(msgs))
	for _, mm := range msgs {
		if mm == nil {
			continue
		}
		role := string(mm.Role)
		if role == "" {
			role = "user"
		}
		out = append(out, Message{Role: role, Content: mm.Content})
	}
	return out
}

// fromInternalResponse converts a *ChatResponse to *schema.Message. The
// existing ChatResponse only carries answer text (+ optional reasoning), so
// the resulting Message has Role=Assistant and Content=answer.
func fromInternalResponse(resp *ChatResponse) *schema.Message {
	if resp == nil {
		return &schema.Message{Role: schema.Assistant, Content: ""}
	}
	content := ""
	if resp.Answer != nil {
		content = *resp.Answer
	}
	return &schema.Message{Role: schema.Assistant, Content: content}
}

// Generate blocks until the model returns a complete response. Mirrors
// eino's model.BaseChatModel.Generate.
func (m *EinoChatModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m == nil || m.inner == nil || m.inner.ModelDriver == nil {
		return nil, fmt.Errorf("models: EinoChatModel: nil inner ModelDriver")
	}
	internal := toInternalMessages(msgs)
	if m.inner.ModelName == nil {
		return nil, fmt.Errorf("models: EinoChatModel: nil model name")
	}
	// ChatWithMessages does not take a context.Context today — Phase 0 kept
	// the signature stable. We log a guard so a future context-aware
	// signature can be slotted in without changing call sites.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Reset stale per-call usage before the call so that a response
	// without a usage block doesn't leak the previous call's data.
	// Mirrors Python's LLMBundle._reset_last_usage().
	m.inner.LastUsage = nil
	resp, err := m.inner.ModelDriver.ChatWithMessages(*m.inner.ModelName, internal, m.inner.APIConfig, m.chatCfg)
	if err != nil {
		return nil, fmt.Errorf("models: EinoChatModel.Generate(%s): %w", *m.inner.ModelName, err)
	}
	// Record the per-call token usage so the canvas-level aggregator (and
	// Langfuse) can compute the run total. Mirrors Python's
	// LLMBundle._report_usage() / self.mdl.last_usage pattern.
	if resp != nil && resp.Usage != nil {
		m.inner.LastUsage = &ChatUsage{
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, TotalTokens: resp.Usage.TotalTokens,
		}
		recordUsageFromResponse(ctx, m.inner)
	}
	return fromInternalResponse(resp), nil
}

// Stream returns a schema.StreamReader that yields message chunks
// incrementally. Uses the existing ChatStreamlyWithSender pathway; the
// sender callback pushes the streamed delta into the StreamReader.
func (m *EinoChatModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m == nil || m.inner == nil || m.inner.ModelDriver == nil {
		return nil, fmt.Errorf("models: EinoChatModel: nil inner ModelDriver")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.inner.ModelName == nil {
		return nil, fmt.Errorf("models: EinoChatModel: nil model name")
	}
	internal := toInternalMessages(msgs)

	sr, sw := schema.Pipe[*schema.Message](1)
	var sendMu sync.Mutex
	sender := func(content *string, _ *string) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		if content == nil {
			return nil
		}
		// Copy the string — the underlying buffer may be reused.
		chunk := *content
		if closed := sw.Send(&schema.Message{Role: schema.Assistant, Content: chunk}, nil); closed {
			return fmt.Errorf("models: stream closed before send completed")
		}
		return nil
	}
	go func() {
		defer sw.Close()
		if err := m.inner.ModelDriver.ChatStreamlyWithSender(*m.inner.ModelName, internal, m.inner.APIConfig, m.chatCfg, sender); err != nil {
			_ = sw.Send(nil, err)
		}
	}()
	return sr, nil
}

// WithTools returns a NEW EinoChatModel instance with the given tools
// attached. The receiver is never mutated — this satisfies eino's
// ToolCallingChatModel contract and is safe under concurrent use.
//
// P0 caveat: the existing RAGFlow provider drivers do not natively consume
// eino's *schema.ToolInfo; the tools are stored on the wrapper for
// future use (Phase 2.5 will plumb them into the driver call). For now
// returning them in the streamed / generated content is a no-op on the
// wire — agents that depend on tool calling will surface this gap during
// Phase 3 ReAct integration.
func (m *EinoChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if m == nil {
		return nil, fmt.Errorf("models: EinoChatModel.WithTools: nil receiver")
	}
	cp := *m
	cp.tools = append([]*schema.ToolInfo(nil), tools...)
	return &cp, nil
}

// Tools returns the tools currently bound to the wrapper (used by
// introspection; not part of any eino interface).
func (m *EinoChatModel) Tools() []*schema.ToolInfo {
	if m == nil {
		return nil
	}
	return append([]*schema.ToolInfo(nil), m.tools...)
}

// Inner exposes the wrapped *ChatModel for callers that need direct
// access (e.g. to read token usage from the response after a custom
// Generate call). Not part of any eino interface.
func (m *EinoChatModel) Inner() *ChatModel {
	if m == nil {
		return nil
	}
	return m.inner
}

// Name returns the wrapped model name (used by tools / debugging).
func (m *EinoChatModel) Name() string {
	return m.name()
}
