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
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/logger"
)

// MetaFilterCondition represents a single filter condition
type MetaFilterCondition struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Op    string `json:"op"`
}

// MetaFilterResult represents the result of LLM-generated filter
type MetaFilterResult struct {
	Conditions []MetaFilterCondition `json:"conditions"`
	Logic      string                `json:"logic"`
}

// ManualValueResolver is a callback function to transform manual filter values
type ManualValueResolver func(map[string]interface{}) map[string]interface{}

// metaFilterTemplateCache caches the template content
var metaFilterTemplateCache string

// getMetaFilterTemplate loads and caches the meta_filter.md template
func getMetaFilterTemplate() (string, error) {
	if metaFilterTemplateCache != "" {
		return metaFilterTemplateCache, nil
	}

	// Try to find meta_filter.md relative to the rag module
	// Look for it in rag/prompts/ directory
	possiblePaths := []string{
		"rag/prompts/meta_filter.md",
		"../rag/prompts/meta_filter.md",
		"../../rag/prompts/meta_filter.md",
	}

	var templateContent string
	for _, path := range possiblePaths {
		content, err := os.ReadFile(path)
		if err == nil {
			templateContent = string(content)
			break
		}
	}

	if templateContent == "" {
		// Fallback: return error
		return "", fmt.Errorf("could not find meta_filter.md template")
	}

	metaFilterTemplateCache = templateContent
	return templateContent, nil
}

// renderMetaFilterTemplate renders the Jinja2-like template from meta_filter.md
func renderMetaFilterTemplate(currentDate, metadataKeys, question, constraints string) (string, error) {
	templateContent, err := getMetaFilterTemplate()
	if err != nil {
		return "", err
	}

	// Replace variables
	result := strings.ReplaceAll(templateContent, "{{ current_date }}", currentDate)
	result = strings.ReplaceAll(result, "{{ metadata_keys }}", metadataKeys)
	result = strings.ReplaceAll(result, "{{ user_question }}", question)

	// Handle {% if constraints %}...{% endif %}
	constraintRegex := regexp.MustCompile(`(?s)\{%\s*if\s+constraints\s*%\}(.+?)\{%\s*endif\s*%\}`)
	if constraints != "" {
		// Replace with the content inside the if block
		result = constraintRegex.ReplaceAllString(result, "$1")
	} else {
		// Remove the entire if block
		result = constraintRegex.ReplaceAllString(result, "")
	}

	// Clean up any extra newlines from removed blocks
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")

	return strings.TrimSpace(result), nil
}

// genMetaFilterPrompt builds the prompt for LLM-based metadata filter generation
func genMetaFilterPrompt(metaDataJSON, question, constraintsJSON, currentDate string) string {
	prompt, err := renderMetaFilterTemplate(currentDate, metaDataJSON, question, constraintsJSON)
	if err != nil {
		logger.Warn("Failed to render meta filter template, using fallback", zap.Error(err))
		// Fallback to empty prompt
		return ""
	}
	return prompt
}

// GenMetaFilter generates filter conditions using LLM based on metadata and question.
func GenMetaFilter(ctx context.Context, chatModel *modelModule.ChatModel, metaData map[string]interface{}, question string, constraints map[string]string) (*MetaFilterResult, error) {
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}

	if len(metaData) == 0 {
		return &MetaFilterResult{Conditions: []MetaFilterCondition{}, Logic: "and"}, nil
	}

	// Build metadata structure for prompt
	metaDataStructure := make(map[string][]string)
	for key, values := range metaData {
		if valueMap, ok := values.(map[string]interface{}); ok {
			keys := make([]string, 0, len(valueMap))
			for k := range valueMap {
				keys = append(keys, k)
			}
			metaDataStructure[key] = keys
		}
	}

	metaDataJSON, _ := json.Marshal(metaDataStructure)
	constraintsJSON := ""
	if constraints != nil {
		constraintsBytes, _ := json.Marshal(constraints)
		constraintsJSON = string(constraintsBytes)
	}

	// Build the prompt
	currentDate := time.Now().Format("2006-01-02")
	systemPrompt := genMetaFilterPrompt(string(metaDataJSON), question, constraintsJSON, currentDate)

	// Build user message
	userMessage := "Generate filters:"

	// Build messages: system prompt + user message
	messages := []modelModule.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	// Call LLM using ChatModel
	response, err := chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, nil)
	if err != nil {
		logger.Warn("ChatWithMessages failed for GenMetaFilter",
			zap.String("model", *chatModel.ModelName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to generate meta filter: %w", err)
	}

	if response == nil || response.Answer == nil {
		return nil, fmt.Errorf("empty response from meta filter generation")
	}

	// Clean up response
	responseStr := strings.TrimSpace(*response.Answer)
	responseStr = thinkBlockRE.ReplaceAllString(responseStr, "")
	responseStr = strings.TrimSpace(responseStr)

	// Remove markdown code blocks if present
	responseStr = strings.TrimPrefix(responseStr, "```json")
	responseStr = strings.TrimPrefix(responseStr, "```")
	responseStr = strings.TrimSuffix(responseStr, "```")
	responseStr = strings.TrimSpace(responseStr)

	// Parse JSON
	var result MetaFilterResult
	if err := json.Unmarshal([]byte(responseStr), &result); err != nil {
		logger.Warn("Failed to parse meta filter response, returning empty conditions", zap.Error(err))
		return &MetaFilterResult{Conditions: []MetaFilterCondition{}, Logic: "and"}, nil
	}

	logger.Info("GenMetaFilter result", zap.Any("conditions", result.Conditions), zap.String("logic", result.Logic))

	return &result, nil
}

// ApplyMetaFilter applies filter conditions to metadata and returns matching doc IDs
func ApplyMetaFilter(metaData map[string]interface{}, filters []MetaFilterCondition, logic string) []string {
	if len(filters) == 0 {
		return []string{}
	}

	docIDSet := make(map[string]bool)

	for i, condition := range filters {
		matchingIDs := applySingleCondition(metaData, condition)
		if i == 0 {
			for _, id := range matchingIDs {
				docIDSet[id] = true
			}
		} else {
			if logic == "or" {
				// Union
				for _, id := range matchingIDs {
					docIDSet[id] = true
				}
			} else {
				// AND - intersection
				newSet := make(map[string]bool)
				for _, id := range matchingIDs {
					if docIDSet[id] {
						newSet[id] = true
					}
				}
				docIDSet = newSet
			}
		}
	}

	// Convert to list
	result := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		result = append(result, id)
	}
	return result
}

// applySingleCondition applies a single filter condition and returns matching doc IDs
func applySingleCondition(metaData map[string]interface{}, condition MetaFilterCondition) []string {
	key := condition.Key
	value := condition.Value
	op := condition.Op

	valueMap, ok := metaData[key].(map[string]interface{})
	if !ok {
		return []string{}
	}

	var result []string

	switch op {
	case "=", "==":
		if docIDs, exists := valueMap[value]; exists {
			switch v := docIDs.(type) {
			case []interface{}:
				for _, id := range v {
					if idStr, ok := id.(string); ok {
						result = append(result, idStr)
					}
				}
			case []string:
				result = append(result, v...)
			}
		}
	case "!=", "≠":
		for val, docIDs := range valueMap {
			if val != value {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "contains":
		for val, docIDs := range valueMap {
			if strings.Contains(strings.ToLower(val), strings.ToLower(value)) {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "not contains":
		for val, docIDs := range valueMap {
			if !strings.Contains(strings.ToLower(val), strings.ToLower(value)) {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "in":
		values := strings.Split(value, ",")
		for _, v := range values {
			v = strings.TrimSpace(v)
			if docIDs, exists := valueMap[v]; exists {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "not in":
		excludeValues := make(map[string]bool)
		for _, v := range strings.Split(value, ",") {
			excludeValues[strings.TrimSpace(strings.ToLower(v))] = true
		}
		for val, docIDs := range valueMap {
			if !excludeValues[strings.ToLower(val)] {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "start with":
		for val, docIDs := range valueMap {
			if strings.HasPrefix(strings.ToLower(val), strings.ToLower(value)) {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "end with":
		for val, docIDs := range valueMap {
			if strings.HasSuffix(strings.ToLower(val), strings.ToLower(value)) {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "empty":
		if len(valueMap) == 0 {
			return []string{}
		}
	case "not empty":
		if len(valueMap) > 0 {
			for _, docIDs := range valueMap {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case ">":
		for val, docIDs := range valueMap {
			if val > value {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "<":
		for val, docIDs := range valueMap {
			if val < value {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case ">=":
		for val, docIDs := range valueMap {
			if val >= value {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	case "<=":
		for val, docIDs := range valueMap {
			if val <= value {
				if ids, ok := docIDs.([]interface{}); ok {
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				}
			}
		}
	default:
		// Default to equality check
		if docIDs, exists := valueMap[value]; exists {
			if ids, ok := docIDs.([]interface{}); ok {
				for _, id := range ids {
					if idStr, ok := id.(string); ok {
						result = append(result, idStr)
					}
				}
			}
		}
	}

	return result
}

// ApplyMetaDataFilter applies metadata filtering rules and returns filtered doc_ids
// Supports three modes:
// - auto: generate filter conditions via LLM
// - semi_auto: generate conditions using selected metadata keys only via LLM
// - manual: directly filter based on provided conditions
func ApplyMetaDataFilter(
	ctx context.Context,
	metaDataFilter map[string]interface{},
	metaData map[string]interface{},
	question string,
	chatModel *modelModule.ChatModel,
	baseDocIDs []string,
	manualValueResolver ...ManualValueResolver,
) ([]string, bool) {
	if metaDataFilter == nil {
		return baseDocIDs, false
	}

	docIDs := make([]string, len(baseDocIDs))
	copy(docIDs, baseDocIDs)

	method, _ := metaDataFilter["method"].(string)

	switch method {
	case "auto":
		filters, err := GenMetaFilter(ctx, chatModel, metaData, question, nil)
		if err != nil {
			logger.Warn("Failed to generate meta filter", zap.Error(err))
			return docIDs, false
		}
		filteredIDs := ApplyMetaFilter(metaData, filters.Conditions, filters.Logic)
		docIDs = append(docIDs, filteredIDs...)
		if len(docIDs) == 0 {
			return nil, true // Return nil to indicate auto filter returned empty
		}

	case "semi_auto":
		selectedKeys := []string{}
		constraints := make(map[string]string)

		if semiAuto, ok := metaDataFilter["semi_auto"].([]interface{}); ok {
			for _, item := range semiAuto {
				switch v := item.(type) {
				case string:
					selectedKeys = append(selectedKeys, v)
				case map[string]interface{}:
					if key, ok := v["key"].(string); ok {
						selectedKeys = append(selectedKeys, key)
						if op, ok := v["op"].(string); ok {
							constraints[key] = op
						}
					}
				}
			}
		}

		if len(selectedKeys) > 0 {
			// Filter metadata to only selected keys
			filteredMeta := make(map[string]interface{})
			for _, key := range selectedKeys {
				if val, exists := metaData[key]; exists {
					filteredMeta[key] = val
				}
			}

			if len(filteredMeta) > 0 {
				filters, err := GenMetaFilter(ctx, chatModel, filteredMeta, question, constraints)
				if err != nil {
					logger.Warn("Failed to generate meta filter", zap.Error(err))
					return docIDs, false
				}
				filteredIDs := ApplyMetaFilter(metaData, filters.Conditions, filters.Logic)
				docIDs = append(docIDs, filteredIDs...)
				if len(docIDs) == 0 {
					return nil, true
				}
			}
		}

	case "manual":
		manualFilters, _ := metaDataFilter["manual"].([]interface{})
		logic := "and"
		if logicVal, ok := metaDataFilter["logic"].(string); ok {
			logic = logicVal
		}

		// Apply manual_value_resolver callback if provided
		if len(manualValueResolver) > 0 && manualValueResolver[0] != nil {
			resolver := manualValueResolver[0]
			resolvedFilters := make([]interface{}, 0, len(manualFilters))
			for _, item := range manualFilters {
				if cond, ok := item.(map[string]interface{}); ok {
					resolvedFilters = append(resolvedFilters, resolver(cond))
				}
			}
			manualFilters = resolvedFilters
		}

		conditions := make([]MetaFilterCondition, 0, len(manualFilters))
		for _, item := range manualFilters {
			if cond, ok := item.(map[string]interface{}); ok {
				condition := MetaFilterCondition{}
				if key, ok := cond["key"].(string); ok {
					condition.Key = key
				}
				if value, ok := cond["value"].(string); ok {
					condition.Value = value
				}
				if op, ok := cond["op"].(string); ok {
					condition.Op = op
				}
				conditions = append(conditions, condition)
			}
		}

		filteredIDs := ApplyMetaFilter(metaData, conditions, logic)
		docIDs = append(docIDs, filteredIDs...)
		if len(manualFilters) > 0 && len(docIDs) == 0 {
			return []string{"-999"}, false
		}
	}

	return docIDs, false
}
