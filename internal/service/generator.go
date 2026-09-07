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

package service

import (
	"context"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"regexp"
	"strings"

	"go.uber.org/zap"

	modelModule "ragflow/internal/entity/models"
)

// KeywordExtraction extracts keywords from content using LLM.
// Corresponds to rag/prompts/generator.py:keyword_extraction().
//
// Uses ChatModel to call the LLM with a keyword extraction prompt.
// Returns comma-separated top N important keywords/phrases from the content.
func KeywordExtraction(ctx context.Context, chatModel *modelModule.ChatModel, content string, topN int) (string, error) {
	if chatModel == nil {
		return "", fmt.Errorf("chat model is nil")
	}

	if content == "" {
		return "", nil
	}

	if topN <= 0 {
		topN = 3
	}

	// Load keyword prompt template from file
	keywordPromptTemplate, err := LoadPrompt("keyword_prompt")
	if err != nil {
		return "", fmt.Errorf("failed to load keyword prompt: %w", err)
	}

	// Render template with content and topn
	renderedPrompt := RenderPrompt(keywordPromptTemplate, map[string]interface{}{
		"content": content,
		"topn":    topN,
	})

	// Build messages: system prompt + user "Output:"
	messages := []modelModule.Message{
		{Role: "system", Content: renderedPrompt},
		{Role: "user", Content: "Output: "},
	}

	// Use low temperature for deterministic keyword extraction (matching Python behavior)
	modelConfig := &modelModule.ChatConfig{
		Temperature: func() *float64 { t := 0.2; return &t }(),
	}

	// Call LLM using ChatModel
	response, err := chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, modelConfig)
	if err != nil {
		return "", fmt.Errorf("failed to extract keywords: %w", err)
	}

	if response == nil || response.Answer == nil {
		return "", fmt.Errorf("empty response from keyword extraction")
	}

	common.Info("KeywordExtraction result", zap.String("response", *response.Answer))

	// Clean up response - remove thinking tags if present
	result := strings.TrimSpace(*response.Answer)
	result = thinkBlockRE.ReplaceAllString(result, "")
	result = strings.TrimSpace(result)

	if strings.Contains(result, "**ERROR**") {
		return "", fmt.Errorf("error in keyword extraction response")
	}

	return result, nil
}

// CrossLanguages translates a question into multiple languages using LLM.
// The model is fetched internally based on llmID:
//   - If llmID is empty, fetches tenant's default chat model
//   - If llmID is not empty, fetches the specified model (or image2text if type matches)
func CrossLanguages(ctx context.Context, tenantID string, llmID string, query string, languages []string) (string, error) {
	common.Debug("CrossLanguages invoked",
		zap.String("tenantID", tenantID),
		zap.String("llmID", llmID),
		zap.Strings("languages", languages))

	modelProviderSvc := NewModelProviderService()
	var chatModel *modelModule.ChatModel
	var err error

	if llmID != "" {
		modelTypes, err := modelProviderSvc.GetModelTypeByName(tenantID, llmID)
		if err != nil {
			return query, fmt.Errorf("failed to get model type: %w", err)
		}
		resolvedType := entity.ModelTypeChat
		for _, mt := range modelTypes {
			if mt == entity.ModelTypeImage2Text {
				resolvedType = entity.ModelTypeImage2Text
				break
			}
		}
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetModelConfigFromProviderInstance(tenantID, resolvedType, llmID)
		if err != nil {
			return query, fmt.Errorf("failed to get chat model: %w", err)
		}
		chatModel = modelModule.NewChatModel(driver, &modelName, apiConfig)
	} else {
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(tenantID, entity.ModelTypeChat)
		if err != nil {
			return query, fmt.Errorf("failed to get default chat model: %w", err)
		}
		chatModel = modelModule.NewChatModel(driver, &modelName, apiConfig)
	}
	if chatModel == nil {
		return query, fmt.Errorf("failed to get chat model: nil chat model")
	}

	if query == "" {
		return query, nil
	}

	if len(languages) == 0 {
		return query, nil
	}

	// Load system prompt from embedded file
	systemPrompt, err := LoadPrompt("cross_languages_sys_prompt")
	if err != nil {
		return query, fmt.Errorf("failed to load system prompt: %w", err)
	}

	// Load user prompt template from file
	userPromptTemplate, err := LoadPrompt("cross_languages_user_prompt")
	if err != nil {
		return query, fmt.Errorf("failed to load user prompt: %w", err)
	}

	// Render user prompt with query and languages
	userPrompt := RenderPrompt(userPromptTemplate, map[string]interface{}{
		"query":     query,
		"languages": languages,
	})

	// Build messages: system prompt + user prompt
	messages := []modelModule.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	// Use low temperature for deterministic translation (matching Python behavior)
	modelConfig := &modelModule.ChatConfig{
		Temperature: func() *float64 { t := 0.2; return &t }(),
	}

	// Call LLM using ChatModel
	response, err := chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, modelConfig)
	if err != nil {
		return query, fmt.Errorf("failed to translate question: %w", err)
	}

	if response == nil || response.Answer == nil {
		return query, fmt.Errorf("empty response from cross languages translation")
	}

	result := *response.Answer

	// Clean up response - remove think tags and trim
	result = thinkBlockRE.ReplaceAllString(result, "")

	if strings.Contains(result, "**ERROR**") {
		return query, nil
	}

	// Parse response
	result = regexp.MustCompile(`(?i)^output:\s*`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`\n+`).ReplaceAllString(result, "")

	parts := strings.Split(result, "===")
	var translations []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			translations = append(translations, trimmed)
		}
	}

	if len(translations) > 0 {
		return strings.Join(translations, "\n"), nil
	}

	return query, nil
}
