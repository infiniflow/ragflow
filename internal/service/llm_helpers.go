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
	"regexp"
	"strings"

	"ragflow/internal/entity"
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

	// Build system prompt (matching Python KEYWORD_PROMPT_TEMPLATE)
	systemPrompt := "## Role\n" +
		"You are a text analyzer.\n\n" +
		"## Task\n" +
		"Extract the most important keywords/phrases of a given piece of text content.\n\n" +
		"## Requirements\n" +
		"- Summarize the text content, and give the top " + fmt.Sprintf("%d", topN) + " important keywords/phrases.\n" +
		"- The keywords MUST be in the same language as the given piece of text content.\n" +
		"- The keywords are delimited by ENGLISH COMMA.\n" +
		"- Output keywords ONLY.\n\n" +
		"---\n\n" +
		"## Text Content\n" +
		content

	// Call LLM using ChatToModelByApiKey
	modelProviderSvc := NewModelProviderService()
	responsePtr, code, err := modelProviderSvc.ChatToModelByApiKey(creds.ProviderName, creds.ModelName, creds.APIKey, systemPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to extract keywords: code=%d, err=%w", int(code), err)
	}

	response := *responsePtr

	// Clean up response - remove thinking tags if present (matching Python L208)
	response = strings.TrimSpace(response)
	response = strings.ReplaceAll(response, "<think>", "")
	response = strings.ReplaceAll(response, "[/think]", "")
	response = strings.TrimSpace(response)

	if strings.Contains(response, "**ERROR**") {
		return "", fmt.Errorf("error in keyword extraction response")
	}

	return response, nil
}

// CrossLanguages translates a question into multiple languages using LLM.
// Corresponds to rag/prompts/generator.py:cross_languages().
//
// Uses ChatToModelByApiKey via ModelCredentials to call the LLM with a translation prompt.
// Returns translations separated by "===" for each target language.
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

	languagesStr := strings.Join(languages, ", ")

	// Build system prompt (matching Python CROSS_LANGUAGES_SYS_PROMPT_TEMPLATE:
	// Role + Behavior Rules + Example with FIXED values, no query/languages substitution)
	systemPrompt := "## Role\n" +
		"A streamlined multilingual translator.\n\n" +
		"## Behavior Rules\n" +
		"1. Accept batch translation requests in the following format:\n" +
		"   **Input:** `[text]`\n" +
		"   **Target Languages:** comma-separated list\n\n" +
		"2. Maintain:\n" +
		"   - Original formatting (tables, lists, spacing)\n" +
		"   - Technical terminology accuracy\n" +
		"   - Cultural context appropriateness\n\n" +
		"3. Output translations in the following format:\n\n" +
		"[Translation in language1]\n" +
		"###\n" +
		"[Translation in language2]\n\n" +
		"---\n\n" +
		"## Example\n\n" +
		"**Input:**\n" +
		"Hello World! Let's discuss AI safety.\n" +
		"===\n" +
		"Chinese, French, Japanese\n\n" +
		"**Output:**\n" +
		"你好世界！让我们讨论人工智能安全问题。\n" +
		"###\n" +
		"Bonjour le monde ! Parlons de la sécurité de l'IA。\n" +
		"###\n" +
		"こんにちは世界！AIの安全性について話し合いましょう。"

	// Build user prompt (matching Python CROSS_LANGUAGES_USER_PROMPT_TEMPLATE with query/languages substituted)
	userPrompt := "**Input:**\n" + query + "\n===\n" + languagesStr + "\n\n**Output:**"

	// Call LLM using ChatToModelByApiKey
	modelProviderSvc := NewModelProviderService()
	fullPrompt := systemPrompt + "\n\n" + userPrompt
	responsePtr, code, err := modelProviderSvc.ChatToModelByApiKey(creds.ProviderName, creds.ModelName, creds.APIKey, fullPrompt)
	if err != nil {
		return query, fmt.Errorf("failed to translate question: code=%d, err=%w", int(code), err)
	}

	response := *responsePtr

	// Clean up response - remove think tags and trim (matching Python L285)
	response = strings.TrimSpace(response)
	response = strings.ReplaceAll(response, "<think>", "")
	response = strings.ReplaceAll(response, "[/think]", "")
	response = strings.TrimSpace(response)

	if strings.Contains(response, "**ERROR**") {
		return query, nil
	}

	// Parse response (matching Python L288: re.sub(r"(^Output:|\n+)", "", ans, flags=re.DOTALL).split("===").join("\n"))
	response = strings.TrimPrefix(response, "Output:")
	response = strings.TrimPrefix(response, "output:")
	response = regexp.MustCompile(`(?i)^output:\s*`).ReplaceAllString(response, "")
	// Python uses \n+ (with re.DOTALL) which replaces ALL newlines with "" (empty string)
	// This collapses "Trans1\n###\nTrans2" into "Trans1###Trans2"
	response = regexp.MustCompile(`\n+`).ReplaceAllString(response, "")
	response = strings.TrimSpace(response)

	parts := strings.Split(response, "===")
	var translations []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		// NOTE: Python does NOT remove trailing ### - it stays in the output
		// So we should NOT use TrimSuffix here
		if trimmed != "" {
			translations = append(translations, trimmed)
		}
	}

	// Python joins with "\n", but due to the \n+ regex collapse BEFORE split,
	// "Trans1\n###\nTrans2" becomes "Trans1###Trans2", then split by "===" gives ["Trans1###Trans2"]
	// Then join by "\n" gives "Trans1###Trans2" (the ### stays inline)
	if len(translations) > 0 {
		return strings.Join(translations, "\n"), nil
	}

	return query, nil
}
