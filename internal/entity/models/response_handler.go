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

package models

import (
	"encoding/json"
	"fmt"
	"io"
	"ragflow/internal/common"
	"strings"

	"go.uber.org/zap"
)

// HandleNonStreamingResponse processes a complete non-streaming chat
// response using the ParserConfig's ResponseParser to extract usage.
func HandleNonStreamingResponse(
	body []byte,
	modelUsage *common.ModelUsage,
	chatConfig *ChatConfig,
	cfg *ParserConfig,
) (*ChatResponse, error) {
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract usage via the protocol-specific parser.
	var usage *TokenUsage
	if u, ok := cfg.ResponseParser(result); ok {
		usage = u
	}

	// Extract content / reasoning_content / tool_calls.
	content, reasonContent, toolCalls := extractContentAndChoices(result)

	if content == nil && len(toolCalls) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	if usage != nil {
		recordResponseUsage(modelUsage, extractRequestID(result), usage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = usage
			model, _ := result["model"].(string)
			common.Info("StreamUsage", zap.String("model", model), zap.Int("prompt", usage.PromptTokens), zap.Int("completion", usage.CompletionTokens), zap.Int("total", usage.TotalTokens))
		}
	}

	return &ChatResponse{
		Answer:        content,
		ReasonContent: reasonContent,
		ToolCalls:     toolCalls,
		Usage:         usage,
	}, nil
}

// HandleStreamingResponse processes a streaming chat response using the
// ParserConfig's StreamParser to extract usage from each event.
func HandleStreamingResponse(
	body io.Reader,
	modelUsage *common.ModelUsage,
	chatConfig *ChatConfig,
	cfg *ParserConfig,
	sender func(*string, *string) error,
) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	var streamUsage *TokenUsage
	accumulatedToolCalls := make(map[int]map[string]any)
	sawTerminal := false

	var streamModel string
	thinkSplitter := &streamThinkSplitter{}
	done, err := ParseSSEStream[map[string]any](body, func(event map[string]any) error {
		if u, ok := cfg.StreamParser(event); ok {
			streamUsage = u
		}
		if m, ok := event["model"].(string); ok {
			streamModel = m
		}

		if apiErr, ok := event["error"]; ok && apiErr != nil {
			return fmt.Errorf("upstream stream error: %v", apiErr)
		}

		choices, ok := event["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil
		}

		firstChoice, ok := choices[0].(map[string]any)
		if !ok {
			return nil
		}

		delta, ok := firstChoice["delta"].(map[string]any)
		if !ok {
			return nil
		}

		accumulateToolCallDeltas(delta, accumulatedToolCalls)

		if reasoningContent, ok := delta["reasoning_content"].(string); ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		if content, ok := delta["content"].(string); ok && content != "" {
			// Split inline <think>...</think> blocks that some
			// providers (e.g. Novita qwen3) embed in delta.content
			// so reasoning routes to the sender's second arg.
			for _, seg := range thinkSplitter.feed(content) {
				if seg.content != "" {
					c := seg.content
					if err := sender(&c, nil); err != nil {
						return err
					}
				}
				if seg.reasoning != "" {
					r := seg.reasoning
					if err := sender(nil, &r); err != nil {
						return err
					}
				}
			}
		}

		if finishReason, ok := firstChoice["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	if chatConfig != nil {
		setSortedToolCallsResult(chatConfig, accumulatedToolCalls)
	}

	if !done && !sawTerminal {
		return fmt.Errorf("stream ended before [DONE] or finish_reason")
	}

	if streamUsage != nil {
		recordResponseUsage(modelUsage, "", streamUsage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = streamUsage
			common.Info("StreamUsage", zap.String("model", streamModel), zap.Int("prompt", streamUsage.PromptTokens), zap.Int("completion", streamUsage.CompletionTokens), zap.Int("total", streamUsage.TotalTokens))
		}
	}

	// Flush any buffered think-tag tail so the final segment is emitted
	// before the [DONE] sentinel.
	if seg := thinkSplitter.flush(); seg != nil {
		if seg.content != "" {
			c := seg.content
			if err := sender(&c, nil); err != nil {
				return err
			}
		}
		if seg.reasoning != "" {
			r := seg.reasoning
			if err := sender(nil, &r); err != nil {
				return err
			}
		}
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

// streamThinkSplitter holds state across streaming content chunks so a
// <think>...</think> block that spans multiple SSE deltas is still split
// correctly between content and reasoning. Trailing bytes that could be
// the start of a tag are held back until the next chunk.
type streamThinkSplitter struct {
	buf    strings.Builder
	inside bool
}

// novitaThinkSegment is one routing decision: emit `content` via the
// sender's first arg, or emit `reasoning` via the second. Exactly one of
// the two fields is non-empty.
type novitaThinkSegment struct {
	content   string
	reasoning string
}

const (
	streamThinkOpen  = "<think>"
	streamThinkClose = "</think>"
)

func (s *streamThinkSplitter) feed(chunk string) []novitaThinkSegment {
	s.buf.WriteString(chunk)
	str := s.buf.String()
	var out []novitaThinkSegment
	for {
		var marker string
		if s.inside {
			marker = streamThinkClose
		} else {
			marker = streamThinkOpen
		}
		idx := strings.Index(str, marker)
		if idx < 0 {
			// No marker yet. Emit everything except a possible
			// partial-tag suffix at the very end.
			reserve := max(len(streamThinkOpen)-1, len(streamThinkClose)-1)
			safe := max(len(str)-reserve, 0)
			if safe < len(str) && !strings.Contains(str[safe:], "<") {
				safe = len(str)
			}
			if safe > 0 {
				if s.inside {
					out = append(out, novitaThinkSegment{reasoning: str[:safe]})
				} else {
					out = append(out, novitaThinkSegment{content: str[:safe]})
				}
				str = str[safe:]
			}
			s.buf.Reset()
			s.buf.WriteString(str)
			return out
		}
		if s.inside {
			out = append(out, novitaThinkSegment{reasoning: str[:idx]})
		} else {
			out = append(out, novitaThinkSegment{content: str[:idx]})
		}
		str = str[idx+len(marker):]
		s.inside = !s.inside
	}
}

func (s *streamThinkSplitter) flush() *novitaThinkSegment {
	if s.buf.Len() == 0 {
		return nil
	}
	remaining := s.buf.String()
	s.buf.Reset()
	if s.inside {
		return &novitaThinkSegment{reasoning: remaining}
	}
	return &novitaThinkSegment{content: remaining}
}

// extractContentAndChoices extracts content, reasoning_content, and tool_calls
// from a parsed non-streaming response.
func extractContentAndChoices(result map[string]any) (*string, *string, []map[string]any) {
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, nil, nil
	}

	firstChoice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, nil, nil
	}

	messageMap, ok := firstChoice["message"].(map[string]any)
	if !ok {
		return nil, nil, nil
	}

	var content *string
	var structuredReason *string
	if c, ok := messageMap["content"].(string); ok {
		cc := c
		content = &cc
	} else if parts, ok := messageMap["content"].([]any); ok {
		// Structured content array (e.g. Mistral magistral): each part is
		// {type: "text", text: "..."} or {type: "thinking",
		// thinking: [{type: "text", text: "..."}]}. Concatenate text
		// parts into Answer and thinking parts into ReasonContent.
		var answer, reasoning strings.Builder
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			switch part["type"] {
			case "text":
				if t, ok := part["text"].(string); ok {
					answer.WriteString(t)
				}
			case "thinking":
				if thinking, ok := part["thinking"].([]any); ok {
					for _, tp := range thinking {
						if tpm, ok := tp.(map[string]any); ok {
							if t, ok := tpm["text"].(string); ok {
								reasoning.WriteString(t)
							}
						}
					}
				}
			}
		}
		// Only treat as structured content if we extracted something;
		// otherwise fall through so callers see an empty Answer.
		if answer.Len() > 0 || reasoning.Len() > 0 {
			ans := answer.String()
			content = &ans
			rc := reasoning.String()
			structuredReason = &rc
		}
	}
	if content == nil {
		if _, ok := messageMap["tool_calls"].([]any); ok {
			// content may be nil when the response only carries tool calls.
			// Return an empty-string pointer so callers can rely on a non-nil Answer.
			empty := ""
			content = &empty
		} else if rc, ok := messageMap["reasoning_content"].(string); ok && rc != "" {
			// content may be nil when the response only carries reasoning_content.
			empty := ""
			content = &empty
		}
	}

	// A structured content array already carries its own reasoning;
	// use that instead of the top-level reasoning_content field.
	var reasonContent *string
	if structuredReason != nil {
		reasonContent = structuredReason
	} else if rc, ok := messageMap["reasoning_content"].(string); ok && rc != "" {
		reason := rc
		if reason != "" && reason[0] == '\n' {
			reason = reason[1:]
		}
		reasonContent = &reason
	} else if rc, ok := messageMap["reasoning"].(string); ok && rc != "" {
		// Some providers (e.g. Avian) report reasoning under a top-level
		// "reasoning" field instead of "reasoning_content".
		reason := rc
		reasonContent = &reason
	} else {
		// Always return a non-nil pointer so callers can rely on it.
		empty := ""
		reasonContent = &empty
	}

	// Some providers (e.g. SiliconFlow) embed thinking inline via <think> tags.
	// Extract reasoning into ReasonContent and strip it from Answer.
	if content != nil {
		if reasoning, answer := extractThinkContent(content); reasoning != nil {
			rc := *reasoning
			reasonContent = &rc
			a := *answer
			content = &a
		}
	}

	var toolCalls []map[string]any
	if tcs, ok := messageMap["tool_calls"].([]any); ok {
		for _, tc := range tcs {
			if tcMap, ok := tc.(map[string]any); ok {
				toolCalls = append(toolCalls, tcMap)
			}
		}
	}

	return content, reasonContent, toolCalls
}

// extractRequestID extracts the request ID from a parsed response.
func extractRequestID(result map[string]any) string {
	if id, ok := result["id"].(string); ok {
		return id
	}
	return ""
}
