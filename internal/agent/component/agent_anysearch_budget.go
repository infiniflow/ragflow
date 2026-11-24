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
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

func agentToolSchemaTokens(tools []*schema.ToolInfo) (int, error) {
	var definitions []map[string]any
	for _, info := range tools {
		if info == nil {
			continue
		}
		definition := map[string]any{"name": info.Name, "description": info.Desc}
		if info.ParamsOneOf != nil {
			parameters, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return 0, err
			}
			definition["parameters"] = parameters
		}
		definitions = append(definitions, map[string]any{"type": "function", "function": definition})
	}
	encoded, err := json.Marshal(definitions)
	if err != nil {
		return 0, err
	}
	return tokenizer.NumTokensFromString(string(encoded)), nil
}

// fitAnySearchInput bounds model-facing copies; recorded tool output and sources stay intact.
func (m *roundLimitedChatModel) fitAnySearchInput(input []*schema.Message) ([]*schema.Message, error) {
	if m.contextLength <= 0 {
		return input, nil
	}
	names := make(map[string]string)
	for _, msg := range input {
		if msg != nil {
			for _, call := range msg.ToolCalls {
				names[call.ID] = call.Function.Name
			}
		}
	}
	var candidates []int
	for i, msg := range input {
		if msg == nil || msg.Role != schema.Tool {
			continue
		}
		name := msg.ToolName
		if name == "" {
			name = names[msg.ToolCallID]
		}
		if name == "anysearch_search" || name == "anysearch_discover" || name == "anysearch_extract" {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return input, nil
	}
	budget := int(float64(m.contextLength)*0.97) - m.outputTokens - m.toolSchemaTokens
	fits := func(messages []*schema.Message) bool {
		// Estimate text as in chat.FitMessages; image tokens depend on the
		// provider and image dimensions, not the length of their data URI.
		textOnly := make([]*schema.Message, len(messages))
		for i, msg := range messages {
			if msg == nil {
				continue
			}
			copied := *msg
			copied.UserInputMultiContent = nil
			for _, part := range msg.UserInputMultiContent {
				if part.Type == schema.ChatMessagePartTypeText {
					copied.UserInputMultiContent = append(copied.UserInputMultiContent, part)
				}
			}
			textOnly[i] = &copied
		}
		encoded, err := json.Marshal(textOnly)
		return err == nil && tokenizer.NumTokensFromString(string(encoded)) <= budget
	}
	if fits(input) {
		return input, nil
	}
	fitted := append([]*schema.Message(nil), input...)
	for len(candidates) > 0 {
		largest := 0
		for i := range candidates {
			if len(fitted[candidates[i]].Content) > len(fitted[candidates[largest]].Content) {
				largest = i
			}
		}
		index := candidates[largest]
		candidates = append(candidates[:largest], candidates[largest+1:]...)
		copied := *fitted[index]
		copied.Content = `{"_ERROR":"AnySearch output omitted because the selected model has insufficient remaining context. Request fewer results or split the batch."}`
		fitted[index] = &copied
		if fits(fitted) {
			return fitted, nil
		}
	}
	return nil, fmt.Errorf("agent: messages and tool definitions exceed the selected model context after omitting AnySearch output; shorten the conversation or select a larger context window")
}
