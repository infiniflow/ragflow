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
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"go.uber.org/zap"
)

func applyPromptTransforms(ctx context.Context, chat *entity.Chat, messages []map[string]interface{}, chatModel *modelModule.ChatModel) []map[string]interface{} {
	if chat == nil || chat.PromptConfig == nil {
		return messages
	}
	latestIdx := latestUserMessageIndex(messages)
	if latestIdx < 0 {
		return messages
	}

	transformed := copyChatMessages(messages)
	question := messageContentText(transformed[latestIdx]["content"])
	if question == "" {
		return transformed
	}

	if promptConfigBool(chat.PromptConfig, "refine_multiturn") && userMessageCount(transformed) > 1 {
		refined, err := FullQuestion(ctx, chatModel, transformed, promptConfigString(chat.PromptConfig, "language"))
		if err != nil {
			common.Warn("Failed to refine multi-turn chat question", zap.Error(err))
		} else if refined != "" {
			question = refined
		}
	}

	if languages := promptConfigStringSlice(chat.PromptConfig, "cross_languages"); len(languages) > 0 {
		translated, err := crossLanguagesWithChatModel(ctx, chatModel, question, languages)
		if err != nil {
			common.Warn("Failed to translate chat question", zap.Error(err))
		} else if translated != "" {
			question = translated
		}
	}

	if promptConfigBool(chat.PromptConfig, "keyword") {
		keywords, err := KeywordExtraction(ctx, chatModel, question, 3)
		if err != nil {
			common.Warn("Failed to extract chat question keywords", zap.Error(err))
		} else if keywords != "" {
			question = strings.TrimSpace(question + "," + keywords)
		}
	}

	transformed[latestIdx]["content"] = replaceMessageContentText(transformed[latestIdx]["content"], question)
	return transformed
}

func latestUserMessageIndex(messages []map[string]interface{}) int {
	for i := len(messages) - 1; i >= 0; i-- {
		role, _ := messages[i]["role"].(string)
		if role == "user" {
			return i
		}
	}
	return -1
}

func userMessageCount(messages []map[string]interface{}) int {
	count := 0
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "user" {
			count++
		}
	}
	return count
}

func promptConfigBool(promptConfig entity.JSONMap, key string) bool {
	value, ok := promptConfig[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	default:
		return false
	}
}

func promptConfigString(promptConfig entity.JSONMap, key string) string {
	value, _ := promptConfig[key].(string)
	return value
}

func promptConfigStringSlice(promptConfig entity.JSONMap, key string) []string {
	switch typed := promptConfig[key].(type) {
	case []string:
		return nonEmptyStrings(typed)
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
		return nonEmptyStrings(values)
	case string:
		return nonEmptyStrings(strings.Split(typed, ","))
	default:
		return nil
	}
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func replaceMessageContentText(content interface{}, text string) interface{} {
	switch typed := content.(type) {
	case []interface{}:
		copied := make([]interface{}, len(typed))
		replaced := false
		for i, block := range typed {
			if blockMap, ok := block.(map[string]interface{}); ok {
				copiedMap := make(map[string]interface{}, len(blockMap))
				for key, value := range blockMap {
					copiedMap[key] = value
				}
				if !replaced {
					if blockText, ok := copiedMap["text"].(string); ok && strings.TrimSpace(blockText) != "" {
						copiedMap["text"] = text
						replaced = true
					}
				}
				copied[i] = copiedMap
				continue
			}
			copied[i] = block
		}
		if replaced {
			return copied
		}
		return append([]interface{}{map[string]interface{}{"type": "text", "text": text}}, copied...)
	case []map[string]interface{}:
		blocks := make([]interface{}, 0, len(typed))
		for _, block := range typed {
			blocks = append(blocks, block)
		}
		return replaceMessageContentText(blocks, text)
	default:
		return text
	}
}

func copyChatMessages(messages []map[string]interface{}) []map[string]interface{} {
	copied := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		copied[i] = make(map[string]interface{}, len(msg))
		for key, value := range msg {
			copied[i][key] = value
		}
	}
	return copied
}
