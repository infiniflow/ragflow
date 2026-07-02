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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// GenerateRelatedQuestions generates related search questions for chat/searchbot endpoints.
func GenerateRelatedQuestions(tenantID, question, searchID string, searchSvc *SearchService, tenantSvc *TenantService, modelProviderSvc *ModelProviderService) ([]string, error) {
	if modelProviderSvc == nil {
		return nil, fmt.Errorf("model provider service not configured")
	}
	searchConfig := relatedQuestionsSearchConfig(searchID, searchSvc)
	modelID := relatedQuestionsModelID(tenantID, searchConfig, tenantSvc)
	prompt, err := LoadPrompt("related_question")
	if err != nil {
		return nil, err
	}
	messages := []modelModule.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "\nKeywords: " + question + "\nRelated search terms:\n    "},
	}
	response, err := modelProviderSvc.Chat(tenantID, modelID, messages, relatedQuestionsConfig(searchConfig))
	if err != nil {
		return nil, err
	}
	if response != nil && response.Answer != nil {
		return parseRelatedQuestions(*response.Answer), nil
	}
	return []string{}, nil
}

func relatedQuestionsSearchConfig(searchID string, searchSvc *SearchService) map[string]interface{} {
	if searchID == "" || searchSvc == nil {
		return map[string]interface{}{}
	}
	if detail, err := searchSvc.GetDetail(searchID); err == nil && detail != nil {
		return relatedQuestionsSearchConfigFromDetail(detail)
	}
	return map[string]interface{}{}
}

func relatedQuestionsSearchConfigFromDetail(detail map[string]interface{}) map[string]interface{} {
	if sc, ok := detail["search_config"].(map[string]interface{}); ok && sc != nil {
		return sc
	}
	if sc, ok := detail["search_config"].(entity.JSONMap); ok && sc != nil {
		return map[string]interface{}(sc)
	}
	return map[string]interface{}{}
}

func relatedQuestionsModelID(tenantID string, searchConfig map[string]interface{}, tenantSvc *TenantService) string {
	modelID, _ := searchConfig["chat_id"].(string)
	if modelID != "" || tenantSvc == nil {
		return modelID
	}
	defaultModel, err := tenantSvc.GetDefaultModelName(tenantID, entity.ModelTypeChat)
	if err == nil {
		modelID = defaultModel
	}
	return modelID
}

func relatedQuestionsConfig(searchConfig map[string]interface{}) *modelModule.ChatConfig {
	var genConf map[string]interface{}
	switch v := searchConfig["llm_setting"].(type) {
	case map[string]interface{}:
		genConf = v
	case entity.JSONMap:
		genConf = map[string]interface{}(v)
	}
	if genConf == nil {
		return &modelModule.ChatConfig{Temperature: float64Ptr(0.9)}
	}
	cfg := &modelModule.ChatConfig{}
	for key, value := range genConf {
		if key == "parameter" {
			continue
		}
		switch key {
		case "stream":
			if v, ok := value.(bool); ok {
				cfg.Stream = &v
			}
		case "thinking":
			if v, ok := value.(bool); ok {
				cfg.Thinking = &v
			}
		case "max_tokens":
			if v, ok := intFromRelatedQuestionConfig(value); ok {
				cfg.MaxTokens = &v
			}
		case "temperature":
			if v, ok := floatFromRelatedQuestionConfig(value); ok {
				cfg.Temperature = &v
			}
		case "top_p":
			if v, ok := floatFromRelatedQuestionConfig(value); ok && v > 0 {
				cfg.TopP = &v
			}
		case "do_sample":
			if v, ok := value.(bool); ok {
				cfg.DoSample = &v
			}
		case "stop":
			if stops := stringSliceFromRelatedQuestionConfig(value); len(stops) > 0 {
				cfg.Stop = &stops
			}
		case "model_class":
			if v, ok := value.(string); ok {
				cfg.ModelClass = &v
			}
		case "effort":
			if v, ok := value.(string); ok {
				cfg.Effort = &v
			}
		case "verbosity":
			if v, ok := value.(string); ok {
				cfg.Verbosity = &v
			}
		}
	}
	return cfg
}

func intFromRelatedQuestionConfig(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
	}
	return 0, false
}

func floatFromRelatedQuestionConfig(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

func stringSliceFromRelatedQuestionConfig(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

var relatedQuestionLineRe = regexp.MustCompile(`^\d+\.\s`)

func parseRelatedQuestions(text string) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		if relatedQuestionLineRe.MatchString(line) {
			result = append(result, relatedQuestionLineRe.ReplaceAllString(line, ""))
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func float64Ptr(v float64) *float64 { return &v }
