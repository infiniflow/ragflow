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

package tree

import (
	"fmt"
	"log"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

const treeLLMLogGreen = "\033[32m"
const treeLLMLogReset = "\033[0m"

// logTreeLLMRequest and logTreeLLMResponse are intentionally verbose while
// Tree debugging is enabled: the summary/claim input is otherwise only visible
// inside the provider request, making it impossible to tell whether a bad
// product came from missing input or from the model's response.
func logTreeLLMRequest(stage string, req common.ChatRequest, attempt int) {
	log.Printf("%s[tree][llm][%s] request attempt=%d llm_id=%q json_mode=%t max_tokens=%s\n--- system prompt ---\n%s\n--- user prompt ---\n%s\n%s",
		treeLLMLogGreen,
		stage,
		attempt,
		req.LLMID,
		req.JSONMode,
		formatTreeLLMMaxTokens(req.MaxTokens),
		req.SystemPrompt,
		req.UserPrompt,
		treeLLMLogReset,
	)
}

func logTreeLLMResponse(stage string, attempt int, resp *common.ChatResponse, err error) {
	if err != nil {
		log.Printf("%s[tree][llm][%s] response attempt=%d error=%v%s", treeLLMLogGreen, stage, attempt, err, treeLLMLogReset)
		return
	}
	if resp == nil {
		log.Printf("%s[tree][llm][%s] response attempt=%d <nil response>%s", treeLLMLogGreen, stage, attempt, treeLLMLogReset)
		return
	}
	log.Printf("%s[tree][llm][%s] response attempt=%d\n--- content ---\n%s\n%s", treeLLMLogGreen, stage, attempt, resp.Content, treeLLMLogReset)
}

func formatTreeLLMMaxTokens(maxTokens *int) string {
	if maxTokens == nil {
		return "<default>"
	}
	return fmt.Sprintf("%d", *maxTokens)
}
