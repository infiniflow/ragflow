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

	"go.uber.org/zap"
)

// HandleNonStreamingResponse processes a complete non-streaming chat
// response using the ParserConfig's ResponseParser to extract usage.
func HandleNonStreamingResponse(body []byte, modelUsage *common.ModelUsage, chatConfig *ChatConfig, cfg *ParserConfig) (*ChatResponse, error) {
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for upstream error.
	if apiErr, ok := result["error"]; ok && apiErr != nil {
		return nil, fmt.Errorf("upstream error: %v", apiErr)
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
func HandleStreamingResponse(body io.Reader, modelUsage *common.ModelUsage, chatConfig *ChatConfig, cfg *ParserConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	var streamUsage *TokenUsage
	accumulatedToolCalls := make(map[int]map[string]any)
	sawTerminal := false

	var streamModel string
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

		// Some providers emit a terminal event that carries only a root-level
		// finish_reason with no choices. Check it before the choices guard so
		// the stream terminates cleanly instead of being rejected as "ended
		// before [DONE] or finish_reason".
		if finishReason, ok := event["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
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

		// Extract reasoning via the protocol hook so each provider can
		// name its reasoning field differently (reasoning_content,
		// reasoning, ...) without the shared handler knowing which.
		extractReasoning := cfg.ExtractStreamReasoning
		if extractReasoning == nil {
			extractReasoning = extractDefaultStreamReasoning
		}
		if reasoning := extractReasoning(delta); reasoning != "" {
			if err := sender(nil, &reasoning); err != nil {
				return err
			}
		}

		if content, ok := delta["content"].(string); ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
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

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
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
	if c, ok := messageMap["content"].(string); ok {
		cc := c
		content = &cc
	} else if _, ok := messageMap["tool_calls"].([]any); ok {
		// content may be nil when the response only carries tool calls.
		// Return an empty-string pointer so callers can rely on a non-nil Answer.
		empty := ""
		content = &empty
	} else if rc, ok := messageMap["reasoning_content"].(string); ok && rc != "" {
		// content may be nil when the response only carries reasoning_content.
		empty := ""
		content = &empty
	}

	var reasonContent *string
	if rc, ok := messageMap["reasoning_content"].(string); ok && rc != "" {
		reason := rc
		if reason[0] == '\n' {
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
