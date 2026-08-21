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

package agentic_rag

import (
	"context"
	"errors"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

// Input carries everything Run needs to spin up a one-shot ReAct conversation
// turn. It is intentionally decoupled from the canvas runtime: the caller
// builds the model and messages directly.
type Input struct {
	// Model is the chat model backing the agent. It must support tool calling
	// (model.ToolCallingChatModel) — models.EinoChatModel does.
	Model *models.EinoChatModel
	// Messages are the conversation history plus the current user message.
	// The agent appends the system instruction itself.
	Messages []*schema.Message
	// MaxIterations caps the ReAct loop. Zero falls back to a sane default.
	MaxIterations int
	// Stream controls whether model output is streamed into the event iterator.
	Stream bool
	// Tools are the eino tools the agent may call. When empty, the default
	// smart-reasoning tool set (think, todo_write, grep_chunks, search_chunks,
	// run_javascript) is used.
	Tools []tool.BaseTool
	// OnDelta, when non-nil, receives each incremental (content, reasoning)
	// delta as it streams. When nil, deltas fall back to runtime.EmitAgentMessage.
	OnDelta func(contentDelta, thinkingDelta string)
}

// defaultMaxIterations caps the ReAct loop before the agent must answer.
const defaultMaxIterations = 50

var errNilModel = errors.New("agentic_rag: model is required")

// DefaultTools builds the default smart-reasoning tool set (the four retrieval
// and reasoning tools, plus the run_javascript sandbox). They are constructed
// directly in this package rather than looked up from the canvas tool registry.
func DefaultTools() []tool.BaseTool {
	return []tool.BaseTool{
		NewThinkTool(),
		NewTodoWriteTool(),
		NewGrepChunksTool(),
		NewSearchChunksTool(),
		NewListChunksTool(),
		NewRunJavascriptTool(),
	}
}

// Run executes a single smart-reasoning (ReAct) turn against the given model
// and returns the final assistant message content, streaming incremental
// deltas through in.OnDelta (or runtime.EmitAgentMessage when OnDelta is nil).
//
// It uses eino ADK's adk.ChatModelAgent (NOT flow/agent/react, and never
// adk/react.go directly), which provides the ReAct loop plus robustness
// (retry/failover/cancel monitoring) and recoverability (checkpoint/resume).
func Run(ctx context.Context, in Input) (string, error) {
	if in.Model == nil {
		return "", errNilModel
	}

	tools := in.Tools
	if len(tools) == 0 {
		tools = DefaultTools()
	}

	maxIter := in.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}

	common.Debug("agentic_rag: run start",
		zap.Int("max_iterations", maxIter),
		zap.Int("messages", len(in.Messages)),
		zap.Int("tools", len(tools)),
	)

	cfg := &adk.ChatModelAgentConfig{
		Name:          "smart-reasoning",
		Instruction:   Prompt(),
		Model:         in.Model,
		MaxIterations: maxIter,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
				// Execute same-round tool calls (multiple tool_calls in one model
				// response) concurrently rather than one-after-another. This is
				// eino's default (false), but we state it explicitly so the
				// parallel intent is not accidental. All six tools
				// (think/todo_write/grep_chunks/search_chunks/list_chunks/run_javascript)
				// are concurrency-safe: they keep no shared mutable state across
				// calls (run_javascript creates a fresh goja VM per invocation).
				ExecuteSequentially: false,
			},
		},
	}

	agent, err := adk.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return "", err
	}

	// EnableStreaming lives on AgentInput, not ChatModelAgentConfig.
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages:        in.Messages,
		EnableStreaming: in.Stream,
	})

	return consumeAgentEvents(ctx, iter, in.OnDelta)
}

// consumeAgentEvents drains the agent event iterator, streaming assistant
// deltas through onDelta (falling back to runtime.EmitAgentMessage when nil)
// and accumulating the final content.
func consumeAgentEvents(
	ctx context.Context,
	iter *adk.AsyncIterator[*adk.AgentEvent],
	onDelta func(contentDelta, thinkingDelta string),
) (string, error) {
	emit := func(content, reasoning string) {
		if onDelta != nil {
			onDelta(content, reasoning)
			return
		}
		runtime.EmitAgentMessage(ctx, content, reasoning)
	}

	var final string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			return final, ev.Err
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mo := ev.Output.MessageOutput
		if mo.Role != schema.Assistant {
			// Tool-result events and non-assistant messages carry the tool's
			// returned content; log it for observability, then skip.
			if mo.Message != nil {
				common.Debug("agentic_rag: tool result",
					zap.String("tool_call_id", mo.Message.ToolCallID),
					zap.Int("content_bytes", len(mo.Message.Content)),
					zap.String("content_head", truncateForLog(mo.Message.Content, 2000)),
				)
			}
			continue
		}
		// Log the tool calls the model decided to make this round.
		if mo.Message != nil && len(mo.Message.ToolCalls) > 0 {
			for i := range mo.Message.ToolCalls {
				common.Debug("agentic_rag: model tool call",
					zap.String("tool", mo.Message.ToolCalls[i].Function.Name),
					zap.String("args", truncateForLog(mo.Message.ToolCalls[i].Function.Arguments, 2000)),
				)
			}
		}
		if mo.IsStreaming {
			if mo.MessageStream == nil {
				continue
			}
			for {
				chunk, recvErr := mo.MessageStream.Recv()
				if recvErr != nil {
					break
				}
				emit(chunk.Content, chunk.ReasoningContent)
				final += chunk.Content
			}
			continue
		}
		if mo.Message != nil {
			emit(mo.Message.Content, mo.Message.ReasoningContent)
			final += mo.Message.Content
		}
	}
	return final, nil
}

// truncateForLog caps a string for log lines so a huge tool output or argument
// payload cannot blow up the log volume.
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
