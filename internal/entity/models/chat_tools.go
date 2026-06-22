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
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"ragflow/internal/tokenizer"
)

const (
	defaultMaxRetries = 3
	defaultMaxRounds  = 5
)

// ChatWithTools runs the non-streaming tool-calling loop.
func (cm *ChatModel) ChatWithTools(ctx context.Context, system string, history []Message, chatCfg *ChatConfig) (string, int, error) {
	tc := cm.ToolConfig
	if tc == nil {
		return "", 0, fmt.Errorf("ChatWithTools called without bound tools")
	}

	var toolsList interface{}
	if err := json.Unmarshal([]byte(tc.Tools), &toolsList); err != nil {
		return "", 0, fmt.Errorf("failed to parse tools JSON: %w", err)
	}

	maxRounds := tc.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	maxRetries := tc.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	if system != "" && len(history) > 0 && history[0].Role != "system" {
		history = append([]Message{{Role: "system", Content: system}}, history...)
	}

	baseHistory := make([]Message, len(history))
	copy(baseHistory, history)

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return "", 0, ctx.Err()
		default:
		}

		h := make([]Message, len(baseHistory))
		copy(h, baseHistory)

		answer, tokens, err := runToolLoop(ctx, cm, h, toolsList, chatCfg, maxRounds)
		if err == nil {
			return answer, tokens, nil
		}
	}
	return "", 0, fmt.Errorf("ChatWithTools failed after %d retries", maxRetries)
}

func runToolLoop(ctx context.Context, cm *ChatModel, history []Message, toolsList interface{}, chatCfg *ChatConfig, maxRounds int) (string, int, error) {
	var totalTokens int

	for round := 0; round <= maxRounds; round++ {
		select {
		case <-ctx.Done():
			return "", totalTokens, ctx.Err()
		default:
		}
		cfg := *chatCfg
		cfg.Tools = toolsList
		tcChoice := "auto"
		cfg.ToolChoice = &tcChoice

		resp, err := cm.ModelDriver.ChatWithMessages(*cm.ModelName, history, cm.APIConfig, &cfg)
		if err != nil {
			return "", totalTokens, fmt.Errorf("round %d: %w", round, err)
		}
		if resp == nil {
			return "", totalTokens, fmt.Errorf("round %d: nil response", round)
		}

		if len(resp.ToolCalls) == 0 {
			answer := ""
			if resp.Answer != nil {
				answer = *resp.Answer
			}
			if resp.ReasonContent != nil && *resp.ReasonContent != "" {
				answer = "<think>" + *resp.ReasonContent + "</think>" + answer
			}
			totalTokens += tokenizer.NumTokensFromString(answer)
			return answer, totalTokens, nil
		}

		history = appendToolResults(history, resp.ToolCalls, cm.ToolConfig.ToolCallSession)
	}

	// Exceeded max rounds
	history = append(history, Message{
		Role:    "user",
		Content: fmt.Sprintf("Exceed max rounds: %d", maxRounds),
	})
	cfg := *chatCfg
	resp, err := cm.ModelDriver.ChatWithMessages(*cm.ModelName, history, cm.APIConfig, &cfg)
	if err != nil {
		return "", totalTokens, fmt.Errorf("final call: %w", err)
	}
	if resp == nil || resp.Answer == nil {
		return "", totalTokens, fmt.Errorf("final call: no answer")
	}
	totalTokens += tokenizer.NumTokensFromString(*resp.Answer)
	return *resp.Answer, totalTokens, nil
}

// ChatStreamlyWithTools runs the streaming tool-calling loop.
func (cm *ChatModel) ChatStreamlyWithTools(ctx context.Context, system string, history []Message, chatCfg *ChatConfig, sender func(*string, *string) error) (int, error) {
	tc := cm.ToolConfig
	if tc == nil {
		return 0, fmt.Errorf("ChatStreamlyWithTools called without bound tools")
	}

	var toolsList interface{}
	if err := json.Unmarshal([]byte(tc.Tools), &toolsList); err != nil {
		return 0, fmt.Errorf("failed to parse tools JSON: %w", err)
	}

	maxRounds := tc.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	maxRetries := tc.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	if system != "" && len(history) > 0 && history[0].Role != "system" {
		history = append([]Message{{Role: "system", Content: system}}, history...)
	}

	baseHistory := make([]Message, len(history))
	copy(baseHistory, history)

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		h := make([]Message, len(baseHistory))
		copy(h, baseHistory)

		totalTokens, err := runStreamToolLoop(ctx, cm, h, toolsList, chatCfg, maxRounds, sender)
		if err == nil {
			return totalTokens, nil
		}
	}
	return 0, fmt.Errorf("ChatStreamlyWithTools failed after %d retries", maxRetries)
}

func runStreamToolLoop(ctx context.Context, cm *ChatModel, history []Message, toolsList interface{}, chatCfg *ChatConfig, maxRounds int, sender func(*string, *string) error) (int, error) {
	var totalTokens int

	for round := 0; round <= maxRounds; round++ {
		select {
		case <-ctx.Done():
			return totalTokens, ctx.Err()
		default:
		}
		cfg := *chatCfg
		cfg.Tools = toolsList
		tcChoice := "auto"
		cfg.ToolChoice = &tcChoice
		cfg.Stream = boolPtr(true)
		var tcs []map[string]interface{}
		cfg.ToolCallsResult = &tcs

		reasoningStarted := false
		var answer string
		var pendingThinkClose bool

		err := cm.ModelDriver.ChatStreamlyWithSender(*cm.ModelName, history, cm.APIConfig, &cfg, func(delta *string, reason *string) error {
			if reason != nil && *reason != "" {
				if !reasoningStarted {
					reasoningStarted = true
					thinkOpen := "<think>"
					if e := sender(&thinkOpen, nil); e != nil {
						return e
					}
				}
				pendingThinkClose = true
				return sender(reason, nil)
			}
			// Reasoning ended, close the think block if open
			if pendingThinkClose {
				pendingThinkClose = false
				thinkClose := "</think>"
				if e := sender(&thinkClose, nil); e != nil {
					return e
				}
			}
			if delta != nil && *delta != "" {
				if *delta == "[DONE]" {
					return nil
				}
				totalTokens += tokenizer.NumTokensFromString(*delta)
				answer += *delta
				if e := sender(delta, nil); e != nil {
					return e
				}
			}
			return nil
		})
		// Close any unclosed think block after stream completes
		if pendingThinkClose {
			pendingThinkClose = false
			thinkClose := "</think>"
			if e := sender(&thinkClose, nil); e != nil {
				return totalTokens, e
			}
		}
		if err != nil {
			return totalTokens, fmt.Errorf("round %d: %w", round, err)
		}

		var toolCalls []map[string]interface{}
		if cfg.ToolCallsResult != nil {
			toolCalls = *cfg.ToolCallsResult
		}

		if answer != "" && len(toolCalls) == 0 {
			return totalTokens, nil
		}
		if len(toolCalls) == 0 {
			return totalTokens, fmt.Errorf("round %d: no content and no tool_calls", round)
		}

		history = appendToolResults(history, toolCalls, cm.ToolConfig.ToolCallSession)
	}

	// Exceeded max rounds
	history = append(history, Message{
		Role:    "user",
		Content: fmt.Sprintf("Exceed max rounds: %d", maxRounds),
	})
	cfg := *chatCfg
	cfg.Stream = boolPtr(true)
	return totalTokens, cm.ModelDriver.ChatStreamlyWithSender(*cm.ModelName, history, cm.APIConfig, &cfg, sender)
}

// appendToolResults executes tool calls concurrently, appends the assistant
// message with tool_calls and individual tool result messages to history.
func appendToolResults(history []Message, toolCalls []map[string]interface{}, session ToolCallSession) []Message {
	if session == nil {
		history = append(history, Message{
			Role:      "assistant",
			Content:   nil,
			ToolCalls: toolCalls,
		})
		for _, tc := range toolCalls {
			tcID, _ := tc["id"].(string)
			history = append(history, Message{
				Role:       "tool",
				Content:    "Error: no tool session configured",
				ToolCallID: tcID,
			})
		}
		return history
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	type toolResult struct {
		index   int
		tcID    string
		content string
	}
	results := make([]toolResult, len(toolCalls))

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, tcMap map[string]interface{}) {
			defer wg.Done()
			var result toolResult
			result.index = idx
			fn, ok := tcMap["function"].(map[string]interface{})
			if !ok {
				mu.Lock()
				results[idx] = result
				mu.Unlock()
				return
			}
			name, _ := fn["name"].(string)
			argsStr, _ := fn["arguments"].(string)
			result.tcID, _ = tcMap["id"].(string)

			var args map[string]interface{}
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args = map[string]interface{}{"raw_arguments": argsStr}
			}

			res, err := session.ToolCall(name, args)
			if err != nil {
				result.content = fmt.Sprintf("Error: %s", err.Error())
			} else {
				result.content = res
			}
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, tc)
	}
	wg.Wait()

	history = append(history, Message{
		Role:      "assistant",
		Content:   nil,
		ToolCalls: toolCalls,
	})

	for _, r := range results {
		history = append(history, Message{
			Role:       "tool",
			Content:    r.content,
			ToolCallID: r.tcID,
		})
	}

	return history
}

func boolPtr(b bool) *bool {
	return &b
}
