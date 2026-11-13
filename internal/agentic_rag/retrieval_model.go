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
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/entity/models"
)

// retrievalModel requires results from each configured source before allowing
// an answer. It derives progress from this turn's real tool call/result pairs,
// so prior conversation turns and incomplete calls cannot satisfy retrieval.
type retrievalModel struct {
	model.ToolCallingChatModel
	requiredTools []string
}

func modelForTools(ctx context.Context, chatModel *models.EinoChatModel, tools []tool.BaseTool, datasetIDs []string) (model.ToolCallingChatModel, error) {
	names := make(map[string]bool, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("agentic_rag: retrieval tool info: %w", err)
		}
		if info != nil {
			names[info.Name] = true
		}
	}
	var required []string
	if len(datasetIDs) > 0 {
		for _, name := range []string{"search_bm25_chunks", "search_chunks", "grep_chunks", "search_semantic_chunks"} {
			if names[name] {
				required = append(required, name)
				break
			}
		}
		if len(required) == 0 {
			return nil, fmt.Errorf("agentic_rag: datasets configured but no corpus search tool is enabled")
		}
	}
	if names[webSearchToolName] {
		required = append(required, webSearchToolName)
	}
	if len(required) == 0 {
		return chatModel, nil
	}
	return &retrievalModel{ToolCallingChatModel: chatModel, requiredTools: required}, nil
}

func (m *retrievalModel) optionsFor(messages []*schema.Message, opts []model.Option) []model.Option {
	start := len(messages)
	for start > 0 {
		start--
		if messages[start] != nil && (messages[start].Role == schema.User || messages[start].Role == "") {
			break
		}
	}
	calls := make(map[string]string)
	completed := make(map[string]bool)
	for _, message := range messages[start:] {
		if message == nil {
			continue
		}
		if message.Role == schema.Assistant {
			for _, call := range message.ToolCalls {
				calls[call.ID] = call.Function.Name
			}
		}
		if message.Role == schema.Tool && calls[message.ToolCallID] != "" {
			completed[calls[message.ToolCallID]] = true
		}
	}
	choice := model.WithToolChoice(schema.ToolChoiceAllowed)
	for _, name := range m.requiredTools {
		if !completed[name] {
			choice = model.WithToolChoice(schema.ToolChoiceForced, name)
			break
		}
	}
	options := make([]model.Option, 0, len(opts)+1)
	options = append(options, opts...)
	return append(options, choice)
}

func (m *retrievalModel) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.ToolCallingChatModel.Generate(ctx, messages, m.optionsFor(messages, opts)...)
}

func (m *retrievalModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.ToolCallingChatModel.Stream(ctx, messages, m.optionsFor(messages, opts)...)
}

func (m *retrievalModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := m.ToolCallingChatModel.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &retrievalModel{ToolCallingChatModel: bound, requiredTools: m.requiredTools}, nil
}
