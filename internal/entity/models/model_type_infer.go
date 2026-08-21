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
	"regexp"
	"strings"
)

const (
	modelTypeChat      = "chat"
	modelTypeEmbedding = "embedding"
	modelTypeRerank    = "rerank"
	modelTypeASR       = "asr"
	modelTypeVision    = "vision"
	modelTypeTTS       = "tts"
	modelTypeOCR       = "ocr"
)

var modelNameTokenRE = regexp.MustCompile(`[a-z0-9]+`)

// InferMissingModelTypes returns model types for a provider model whose upstream
// list does not carry type metadata. It prefers exact all_models.json metadata,
// then strong capability hints in the name, and finally falls back to chat.
func InferMissingModelTypes(modelName string) []string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return []string{modelTypeChat}
	}
	if pm := GetProviderManager(); pm != nil {
		if model := pm.GetModelByNameOrAlias(modelName); model != nil && len(model.ModelTypes) > 0 {
			return normalizeModelTypes(model.ModelTypes)
		}
	}
	if modelTypes := inferModelTypesByName(modelName); len(modelTypes) > 0 {
		return normalizeModelTypes(modelTypes)
	}
	return []string{modelTypeChat}
}

func inferModelTypesByName(modelName string) []string {
	lowerName := strings.ToLower(modelName)
	tokens := modelNameTokens(modelName)
	hasToken := func(values ...string) bool {
		for _, value := range values {
			if _, ok := tokens[value]; ok {
				return true
			}
		}
		return false
	}

	switch {
	case hasToken("rerank", "reranker"):
		return []string{modelTypeRerank}
	case hasToken("ocr"):
		return []string{modelTypeOCR}
	case hasToken("asr", "stt", "transcribe", "transcriber", "whisper", "audio"):
		return []string{modelTypeASR}
	case hasToken("tts") || strings.Contains(lowerName, "text-to-speech"):
		return []string{modelTypeTTS}
	case hasToken("embed", "embedding", "embeddings", "bge", "e5"):
		return []string{modelTypeEmbedding}
	case strings.Contains(lowerName, "qwen-vl") ||
		strings.Contains(lowerName, "glm-4v") ||
		strings.Contains(lowerName, "minicpm-v") ||
		strings.Contains(lowerName, "gpt-4o") ||
		hasToken("vl", "vlm", "vision", "llava", "internvl", "pixtral", "image", "video"):
		return []string{modelTypeChat, modelTypeVision}
	default:
		return nil
	}
}

func modelNameTokens(modelName string) map[string]struct{} {
	if idx := strings.LastIndex(modelName, "/"); idx >= 0 && idx+1 < len(modelName) {
		modelName = modelName[idx+1:]
	}
	tokens := map[string]struct{}{}
	for _, token := range modelNameTokenRE.FindAllString(strings.ToLower(modelName), -1) {
		if token != "" {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

func normalizeModelTypes(modelTypes []string) []string {
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(modelTypes))
	for _, modelType := range modelTypes {
		modelType = strings.TrimSpace(modelType)
		if modelType == "" {
			continue
		}
		if _, ok := seen[modelType]; ok {
			continue
		}
		seen[modelType] = struct{}{}
		unique = append(unique, modelType)
	}
	if len(unique) == 0 {
		return []string{modelTypeChat}
	}

	hasChat := false
	hasVision := false
	for _, modelType := range unique {
		hasChat = hasChat || modelType == modelTypeChat
		hasVision = hasVision || modelType == modelTypeVision
	}
	for _, modelType := range unique {
		if modelType != modelTypeChat && modelType != modelTypeVision {
			return []string{modelType}
		}
	}
	if hasVision || hasChat {
		if hasVision {
			return []string{modelTypeChat, modelTypeVision}
		}
		return []string{modelTypeChat}
	}
	return []string{modelTypeChat}
}
