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
	"regexp"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// KeywordExtraction extracts keywords from content using LLM.
// Corresponds to rag/prompts/generator.py:keyword_extraction().
//
// Uses ChatToModelByApiKey via ModelCredentials to call the LLM with a keyword extraction prompt.
// Returns comma-separated top N important keywords/phrases from the content.
func KeywordExtraction(ctx context.Context, creds *entity.ModelCredentials, content string, topN int) (string, error) {
	if creds == nil {
		return "", fmt.Errorf("model credentials is nil")
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

	// Call LLM using ChatWithMessagesToModelByApiKey
	modelProviderSvc := NewModelProviderService()
	responsePtr, code, err := modelProviderSvc.ChatWithMessagesToModelByApiKey(creds.ProviderName, creds.ModelName, creds.APIKey, messages)
	if err != nil {
		return "", fmt.Errorf("failed to extract keywords: code=%d, err=%w", int(code), err)
	}

	response := *responsePtr
	common.Info("KeywordExtraction result", zap.String("response", response))

	// Clean up response - remove thinking tags if present
	response = strings.TrimSpace(response)
	response = thinkBlockRE.ReplaceAllString(response, "")
	response = strings.TrimSpace(response)

	if strings.Contains(response, "**ERROR**") {
		return "", fmt.Errorf("error in keyword extraction response")
	}

	return response, nil
}

// CrossLanguages translates a question into multiple languages using LLM.
func CrossLanguages(ctx context.Context, creds *entity.ModelCredentials, query string, languages []string) (string, error) {
	if creds == nil {
		return "", fmt.Errorf("model credentials is nil")
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

	// Call LLM using ChatWithMessagesToModelByApiKey
	modelProviderSvc := NewModelProviderService()
	responsePtr, code, err := modelProviderSvc.ChatWithMessagesToModelByApiKey(creds.ProviderName, creds.ModelName, creds.APIKey, messages)
	if err != nil {
		return query, fmt.Errorf("failed to translate question: code=%d, err=%w", int(code), err)
	}

	response := *responsePtr

	// Clean up response - remove think tags and trim
	response = strings.TrimSpace(response)
	response = thinkBlockRE.ReplaceAllString(response, "")
	response = strings.TrimSpace(response)

	if strings.Contains(response, "**ERROR**") {
		return query, nil
	}

	// Parse response
	response = strings.TrimPrefix(response, "Output:")
	response = strings.TrimPrefix(response, "output:")
	response = regexp.MustCompile(`(?i)^output:\s*`).ReplaceAllString(response, "")
	response = regexp.MustCompile(`\n+`).ReplaceAllString(response, "")
	response = strings.TrimSpace(response)

	parts := strings.Split(response, "===")
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
