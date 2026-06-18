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
	"time"

	"go.uber.org/zap"

	modelModule "ragflow/internal/entity/models"
)

// KeywordExtraction extracts keywords from content using LLM.
// Corresponds to rag/prompts/generator.py:keyword_extraction().
//
// Uses ChatModel to call the LLM with a keyword extraction prompt.
// Returns comma-separated top N important keywords/phrases from the content.
func KeywordExtraction(ctx context.Context, chatModel *modelModule.ChatModel, content string, topN int) (string, error) {
	if !isUsableChatModel(chatModel) {
		return "", nil
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

// FullQuestion rewrites a multi-turn conversation into a standalone question.
// Corresponds to rag/prompts/generator.py:full_question().
func FullQuestion(ctx context.Context, chatModel *modelModule.ChatModel, messages []map[string]interface{}, language string) (string, error) {
	fallback := latestUserMessageText(messages)
	if !isUsableChatModel(chatModel) {
		return fallback, nil
	}
	if len(messages) == 0 || fallback == "" {
		return fallback, nil
	}

	promptTemplate, err := LoadPrompt("full_question_prompt")
	if err != nil {
		return fallback, fmt.Errorf("failed to load full question prompt: %w", err)
	}

	now := time.Now()
	conversation := conversationText(messages)
	renderedPrompt := RenderPrompt(promptTemplate, map[string]interface{}{
		"today":        now.Format("2006-01-02"),
		"yesterday":    now.AddDate(0, 0, -1).Format("2006-01-02"),
		"tomorrow":     now.AddDate(0, 0, 1).Format("2006-01-02"),
		"conversation": conversation,
		"language":     language,
	})

	response, err := chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, []modelModule.Message{
		{Role: "system", Content: renderedPrompt},
		{Role: "user", Content: "Output: "},
	}, chatModel.APIConfig, nil)
	if err != nil {
		return fallback, fmt.Errorf("failed to generate full question: %w", err)
	}
	if response == nil || response.Answer == nil {
		return fallback, fmt.Errorf("empty response from full question generation")
	}

	result := strings.TrimSpace(*response.Answer)
	result = thinkBlockRE.ReplaceAllString(result, "")
	result = strings.TrimSpace(result)
	if result == "" || strings.Contains(result, "**ERROR**") {
		return fallback, nil
	}
	return result, nil
}

func crossLanguagesWithChatModel(ctx context.Context, chatModel *modelModule.ChatModel, query string, languages []string) (string, error) {
	if !isUsableChatModel(chatModel) {
		return query, nil
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

func isUsableChatModel(chatModel *modelModule.ChatModel) bool {
	return chatModel != nil &&
		chatModel.ModelDriver != nil &&
		chatModel.ModelName != nil &&
		strings.TrimSpace(*chatModel.ModelName) != ""
}

func conversationText(messages []map[string]interface{}) string {
	var parts []string
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		content := messageContentText(msg["content"])
		if content == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", strings.ToUpper(role), content))
	}
	return strings.Join(parts, "\n")
}

func latestUserMessageText(messages []map[string]interface{}) string {
	for i := len(messages) - 1; i >= 0; i-- {
		role, _ := messages[i]["role"].(string)
		if role != "user" {
			continue
		}
		return messageContentText(messages[i]["content"])
	}
	return ""
}

func messageContentText(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []interface{}:
		return strings.TrimSpace(strings.Join(textBlocksFromMessageContent(typed), "\n"))
	case []map[string]interface{}:
		blocks := make([]interface{}, 0, len(typed))
		for _, block := range typed {
			blocks = append(blocks, block)
		}
		return strings.TrimSpace(strings.Join(textBlocksFromMessageContent(blocks), "\n"))
	default:
		if content == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(content))
	}
}

func textBlocksFromMessageContent(blocks []interface{}) []string {
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				texts = append(texts, text)
			}
		case map[string]interface{}:
			if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
		}
	}
	return texts
}
