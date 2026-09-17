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
	"math"
	"regexp"
	"strconv"
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

const (
	modelTypeSimilarityBaseline = 0.78
	modelTypeSimilarityMargin   = 0.005
)

var (
	modelNameTokenRE             = regexp.MustCompile(`[a-z0-9]+`)
	modelNameLetterNumberTokenRE = regexp.MustCompile(`^([a-z]+)([0-9]+)$`)
	modelNameNumberTokenRE       = regexp.MustCompile(`^[0-9]+$`)
)

// InferModelTypes derives RAGFlow LLM model types from the model name using
// keyword heuristics, covering all seven supported types: chat, embedding,
// rerank, asr, tts, ocr, and vision (always combined with chat).
func InferModelTypes(modelName string) []string {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return []string{modelTypeChat}
	}
	if modelTypes := inferModelTypesByName(modelName); len(modelTypes) > 0 {
		return normalizeModelTypes(modelTypes)
	}
	return []string{modelTypeChat}
}

// FillMissingModelTypes fills missing model types using only the models present
// in the provided list. Existing model types are normalized before returning.
func FillMissingModelTypes(models []ListModelResponse) []ListModelResponse {
	for i := range models {
		if len(models[i].ModelTypes) > 0 {
			models[i].ModelTypes = normalizeModelTypes(models[i].ModelTypes)
			continue
		}
		if modelTypes := inferModelTypesByName(models[i].Name); len(modelTypes) > 0 {
			models[i].ModelTypes = normalizeModelTypes(modelTypes)
		}
	}
	candidates := append([]ListModelResponse(nil), models...)
	inferred := make([][]string, len(models))
	for i := range models {
		if len(models[i].ModelTypes) == 0 {
			if modelTypes := inferModelTypesBySimilarity(models[i].Name, candidates); len(modelTypes) > 0 {
				inferred[i] = normalizeModelTypes(modelTypes)
			}
		}
	}
	for i := range models {
		if len(models[i].ModelTypes) == 0 && len(inferred[i]) > 0 {
			models[i].ModelTypes = inferred[i]
		}
		if len(models[i].ModelTypes) == 0 {
			models[i].ModelTypes = []string{modelTypeChat}
		}
	}
	return models
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
		hasToken("vl", "vlm", "vision", "llava", "internvl", "pixtral", "qvq", "image", "video"):
		return []string{modelTypeChat, modelTypeVision}
	default:
		return nil
	}
}

func modelNameTokens(modelName string) map[string]struct{} {
	tokens := modelNameComparableTokens(modelName)
	tokenSet := map[string]struct{}{}
	for _, token := range tokens {
		tokenSet[token] = struct{}{}
	}
	return tokenSet
}

func modelNameComparableTokens(modelName string) []string {
	modelName = comparableModelNameText(modelName)
	tokens := map[string]struct{}{}
	ordered := []string{}
	add := func(token string) {
		if token == "" {
			return
		}
		if _, ok := tokens[token]; ok {
			return
		}
		tokens[token] = struct{}{}
		ordered = append(ordered, token)
	}
	for _, token := range modelNameTokenRE.FindAllString(modelName, -1) {
		if parts := modelNameLetterNumberTokenRE.FindStringSubmatch(token); len(parts) == 3 {
			add(parts[1])
			add(parts[2])
			continue
		}
		add(token)
	}
	return ordered
}

func comparableModelNameText(modelName string) string {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if idx := strings.LastIndex(modelName, "/"); idx >= 0 && idx+1 < len(modelName) {
		modelName = modelName[idx+1:]
	}
	return modelName
}

func inferModelTypesBySimilarity(modelName string, models []ListModelResponse) []string {
	targetHint := inferModelTypesByName(modelName)
	target := newModelNameProfile(modelName)
	if len(target.tokenSet) == 0 {
		return nil
	}

	bestScore := 0.0
	secondScore := 0.0
	var bestTypes []string
	var secondTypes []string
	for _, model := range models {
		if len(model.ModelTypes) == 0 {
			continue
		}
		if strings.EqualFold(model.Name, modelName) {
			continue
		}
		if !compatibleModelNameCapabilities(targetHint, inferModelTypesByName(model.Name)) {
			continue
		}
		score := modelNameSimilarityScore(target, newModelNameProfile(model.Name))
		if score > bestScore {
			secondScore = bestScore
			secondTypes = bestTypes
			bestScore = score
			bestTypes = normalizeModelTypes(model.ModelTypes)
			continue
		}
		if score > secondScore {
			secondScore = score
			secondTypes = normalizeModelTypes(model.ModelTypes)
		}
	}
	if bestScore < modelTypeSimilarityBaseline {
		return nil
	}
	if secondScore > 0 && bestScore-secondScore < modelTypeSimilarityMargin && !sameModelTypes(bestTypes, secondTypes) {
		return nil
	}
	return bestTypes
}

func compatibleModelNameCapabilities(targetHint, candidateHint []string) bool {
	targetCapability := capabilityModelType(targetHint)
	candidateCapability := capabilityModelType(candidateHint)
	if targetCapability == "" {
		return candidateCapability == ""
	}
	return targetCapability == candidateCapability
}

func capabilityModelType(modelTypes []string) string {
	for _, modelType := range normalizeModelTypes(modelTypes) {
		if modelType != modelTypeChat {
			return modelType
		}
	}
	return ""
}

type modelNameProfile struct {
	tokens     []string
	tokenSet   map[string]struct{}
	brand      string
	series     string
	version    float64
	hasVersion bool
}

func newModelNameProfile(modelName string) modelNameProfile {
	rawTokens := modelNameTokenRE.FindAllString(comparableModelNameText(modelName), -1)
	profile := modelNameProfile{
		tokens:   modelNameComparableTokens(modelName),
		tokenSet: map[string]struct{}{},
	}
	for _, token := range profile.tokens {
		profile.tokenSet[token] = struct{}{}
	}
	if len(rawTokens) > 0 {
		profile.brand = rawTokens[0]
		profile.series = rawTokens[0]
	}
	for index, token := range rawTokens {
		parts := modelNameLetterNumberTokenRE.FindStringSubmatch(token)
		if len(parts) != 3 {
			continue
		}
		versionText := parts[2]
		if index+1 < len(rawTokens) && modelNameNumberTokenRE.MatchString(rawTokens[index+1]) {
			versionText += "." + rawTokens[index+1]
		}
		version, err := strconv.ParseFloat(versionText, 64)
		if err != nil {
			continue
		}
		profile.series = parts[1]
		profile.version = version
		profile.hasVersion = true
		if index > 0 {
			profile.brand = rawTokens[0]
		} else {
			profile.brand = parts[1]
		}
		break
	}
	return profile
}

func modelNameSimilarityScore(a, b modelNameProfile) float64 {
	if len(a.tokenSet) == 0 || len(b.tokenSet) == 0 || tokenSetIntersectionSize(a.tokenSet, b.tokenSet) == 0 {
		return 0
	}
	jaccard := tokenSetJaccard(a.tokenSet, b.tokenSet)
	series := modelNameSeriesScore(a, b)
	version := modelNameVersionScore(a, b)
	return 0.25*jaccard + 0.45*series + 0.30*version
}

func modelNameSeriesScore(a, b modelNameProfile) float64 {
	if a.brand == "" || b.brand == "" || a.brand != b.brand {
		return 0
	}
	if a.series != "" && a.series == b.series {
		return 1
	}
	return 0.6
}

func modelNameVersionScore(a, b modelNameProfile) float64 {
	if !a.hasVersion || !b.hasVersion || a.series == "" || a.series != b.series {
		return 0
	}
	maxVersion := math.Max(1, math.Max(a.version, b.version))
	score := 1 - math.Abs(a.version-b.version)/maxVersion
	if score < 0 {
		return 0
	}
	return score
}

func tokenSetIntersectionSize(a, b map[string]struct{}) int {
	count := 0
	for token := range a {
		if _, ok := b[token]; ok {
			count++
		}
	}
	return count
}

func tokenSetJaccard(a, b map[string]struct{}) float64 {
	intersection := tokenSetIntersectionSize(a, b)
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
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
	hasOCR := false
	for _, modelType := range unique {
		hasChat = hasChat || modelType == modelTypeChat
		hasVision = hasVision || modelType == modelTypeVision
		hasOCR = hasOCR || modelType == modelTypeOCR
	}
	if hasChat && hasOCR {
		return []string{modelTypeChat, modelTypeVision}
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

func sameModelTypes(a, b []string) bool {
	a = normalizeModelTypes(a)
	b = normalizeModelTypes(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
