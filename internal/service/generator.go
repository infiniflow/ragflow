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
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"go.uber.org/zap"
)

// KeywordExtraction extracts keywords from content using LLM.
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

// fullQuestionTmpl mirrors the Python Jinja2 template
// rag/prompts/full_question_prompt.md. The rendered output is used as the
// system message; the user message is just "Output: ".
var fullQuestionTmpl = template.Must(template.New("full_question").Parse(`## Role
A helpful assistant.

## Task & Steps
1. Generate a full user question that would follow the conversation.
2. If the user's question involves relative dates, convert them into absolute dates based on today ({{.Today}}).
   - "yesterday" = {{.Yesterday}}, "tomorrow" = {{.Tomorrow}}

## Requirements & Restrictions
- If the user's latest question is already complete, don't do anything — just return the original question.
- DON'T generate anything except a refined question.
{{- if .Language }}
- Text generated MUST be in {{.Language}}.
{{- else }}
- Text generated MUST be in the same language as the original user's question.
{{- end }}

---

## Examples

### Example 1
**Conversation:**

USER: What is the name of Donald Trump's father?
ASSISTANT: Fred Trump.
USER: And his mother?

**Output:** What's the name of Donald Trump's mother?

---

### Example 2
**Conversation:**

USER: What is the name of Donald Trump's father?
ASSISTANT: Fred Trump.
USER: And his mother?
ASSISTANT: Mary Trump.
USER: What's her full name?

**Output:** What's the full name of Donald Trump's mother Mary Trump?

---

### Example 3
**Conversation:**

USER: What's the weather today in London?
ASSISTANT: Cloudy.
USER: What's about tomorrow in Rochester?

**Output:** What's the weather in Rochester on {{.Tomorrow}}?

---

## Real Data

**Conversation:**

{{.Conversation}}
`))

var errorMarkerRE = regexp.MustCompile(`\*\*ERROR\*\*`)

// FullQuestion rewrites the latest user question in light of prior
// conversation context (pronouns, dates, follow-ups). Falls back to the
// latest user message on LLM error.
// When language is empty, the original language is preserved (matching Python).
//
// The prompt structure mirrors Python's full_question():
//   - System: fullQuestionTmpl (instructions, examples, conversation)
//   - User: "Output: "
//
// This matches rag/prompts/full_question_prompt.md rendered via Jinja2.
func FullQuestion(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	messages []map[string]interface{},
	language string,
) (string, error) {
	if chatModel == nil || chatModel.ModelDriver == nil {
		return "", fmt.Errorf("FullQuestion: nil chat model")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("FullQuestion: empty messages")
	}

	var convLines []string
	for _, m := range messages {
		role, _ := m["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		content, _ := m["content"].(string)
		convLines = append(convLines, fmt.Sprintf("%s: %s", strings.ToUpper(role), content))
	}
	conv := strings.Join(convLines, "\n")

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().Add(24 * time.Hour).Format("2006-01-02")
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")

	var buf bytes.Buffer
	if err := fullQuestionTmpl.Execute(&buf, map[string]string{
		"Today":        today,
		"Yesterday":    yesterday,
		"Tomorrow":     tomorrow,
		"Conversation": conv,
		"Language":     language,
	}); err != nil {
		return fallbackToLatestUser(messages), fmt.Errorf("FullQuestion: render template: %w", err)
	}
	system := buf.String()

	modelName := ""
	if chatModel.ModelName != nil {
		modelName = *chatModel.ModelName
	}
	msgs := []modelModule.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: "Output: "},
	}
	resp, err := chatModel.ModelDriver.ChatWithMessages(
		modelName, msgs, chatModel.APIConfig, nil,
	)
	if err != nil {
		return fallbackToLatestUser(messages), err
	}
	if resp == nil || resp.Answer == nil {
		return fallbackToLatestUser(messages), fmt.Errorf("FullQuestion: empty response")
	}
	cleaned := strings.TrimSpace(*resp.Answer)
	cleaned = thinkBlockRE.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)
	if errorMarkerRE.MatchString(cleaned) {
		return fallbackToLatestUser(messages), nil
	}
	if cleaned == "" {
		return fallbackToLatestUser(messages), nil
	}
	return cleaned, nil
}

// fallbackToLatestUser returns the last user message, or "" if none.
func fallbackToLatestUser(messages []map[string]interface{}) string {
	for i := len(messages) - 1; i >= 0; i-- {
		role, _ := messages[i]["role"].(string)
		if role == "user" {
			if c, ok := messages[i]["content"].(string); ok {
				return c
			}
			return ""
		}
	}
	return ""
}
