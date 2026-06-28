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
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"regexp"

	"github.com/kaptinlin/jsonrepair"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	modelModule "ragflow/internal/entity/models"
)

// MetaFilterCondition represents a single filter condition
type MetaFilterCondition struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
	Op    string      `json:"op"`
}

// MetaFilterResult represents the result of LLM-generated filter
type MetaFilterResult struct {
	Conditions []MetaFilterCondition `json:"conditions"`
	Logic      string                `json:"logic"`
}

// compareValues compares two metadata values for relational operators
// (>, <, >=, <=). It attempts numeric comparison first so that
// lexicographic ordering like "10" < "2" or "100" < "20" no longer
// produces wrong results for years, prices, counts, etc. If either
// operand is not a valid number, it falls back to lexicographic string
// comparison so non-numeric metadata (names, tags, etc.) still works.
func compareValues(val1, val2, op string) bool {
	if f1, err1 := strconv.ParseFloat(val1, 64); err1 == nil {
		if f2, err2 := strconv.ParseFloat(val2, 64); err2 == nil {
			switch op {
			case ">":
				return f1 > f2
			case "<":
				return f1 < f2
			case ">=":
				return f1 >= f2
			case "<=":
				return f1 <= f2
			}
		}
	}
	switch op {
	case ">":
		return val1 > val2
	case "<":
		return val1 < val2
	case ">=":
		return val1 >= val2
	case "<=":
		return val1 <= val2
	}
	return false
}

// ManualValueResolver is a callback function to transform manual filter values
type ManualValueResolver func(map[string]interface{}) map[string]interface{}

// NoMatchDocIDSentinel forces retrieval to return no documents when filters match nothing.
const NoMatchDocIDSentinel = "-999"

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
		common.Warn("Failed to render meta filter template, using fallback", zap.Error(err))
		// Fallback to empty prompt
		return ""
	}
	return prompt
}

// GenMetaFilter generates filter conditions using LLM based on metadata and question.
func GenMetaFilter(ctx context.Context, chatModel *modelModule.ChatModel, metaData common.MetaData, question string, constraints map[string]string) (*MetaFilterResult, error) {
	if chatModel == nil {
		return nil, fmt.Errorf("chat model is nil")
	}

	if len(metaData) == 0 {
		return &MetaFilterResult{Conditions: []MetaFilterCondition{}, Logic: "and"}, nil
	}

	// Build metadata structure for prompt
	metaDataStructure := make(map[string][]string)
	for key, values := range metaData {
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		metaDataStructure[key] = keys
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
		common.Warn("ChatWithMessages failed for GenMetaFilter",
			zap.String("model",

				*chatModel.ModelName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to generate meta filter: %w", err)
	}

	if response == nil || response.Answer == nil {
		return nil, fmt.Errorf("empty response from meta filter generation")
	}

	// Clean up response
	responseStr := strings.TrimSpace(*response.Answer)
	responseStr = thinkBlockRE.ReplaceAllString(responseStr, "")
	responseStr = jsonFenceRE.ReplaceAllString(responseStr, "")
	responseStr = strings.TrimSpace(responseStr)

	// Parse JSON with repair — try standard parsing first, then attempt repairs
	var result MetaFilterResult
	if err := json.Unmarshal([]byte(responseStr), &result); err != nil {
		// Attempt JSON repair for common LLM output issues
		repaired, rerr := jsonrepair.Repair(responseStr)
		if rerr != nil {
			repaired = responseStr
		}
		if err2 := json.Unmarshal([]byte(repaired), &result); err2 != nil {
			common.Warn("Failed to parse meta filter response after repair",
				zap.String("raw", responseStr[:min(len(responseStr), 200)]),
				zap.Error(err))
			return &MetaFilterResult{Conditions: []MetaFilterCondition{}, Logic: "and"}, nil
		}
	}

	common.Info("GenMetaFilter result", zap.Any("conditions", result.Conditions), zap.String("logic", result.Logic))

	return &result, nil
}

// ApplyMetaFilter applies filter conditions to metadata and returns matching doc IDs.
// It converts service-layer MetaFilterCondition to common.MetaCondition, then delegates
// all conditions and their logic to common.MetaFilter which handles multi-condition
// AND/OR merging internally. This eliminates the duplicate merge logic that previously
// existed between ApplyMetaFilter and common.MetaFilter.
func ApplyMetaFilter(metaData common.MetaData, filters []MetaFilterCondition, logic string) []string {
	if len(filters) == 0 {
		return []string{}
	}

	conditions := make([]common.MetaCondition, 0, len(filters))
	for _, f := range filters {
		conditions = append(conditions, convertToMetaCondition(f))
	}

	return common.MetaFilter(metaData, &common.MetaFilterInput{
		Conditions: conditions,
		Logic:      logic,
	})
}

// convertToMetaCondition converts a MetaFilterCondition to common.MetaCondition,
// normalizing operator symbols and value types for compatibility with common.MetaFilter.
//
// Operator normalization:
//   - "==" = "="    "!=" = "≠"
//   - ">=" = "≥"    "<=" = "≤"
//   - "is" = "="    "not is" = "≠"
//     (see common.metadata_utils.operatorMapping for the full list)
//
// Value conversion:
//   - "in" / "not in": comma-separated string → []interface{} (as expected by common.MetaFilter)
//   - all other operators: passed through as-is (string)
func convertToMetaCondition(f MetaFilterCondition) common.MetaCondition {
	mc := common.MetaCondition{
		Key:      f.Key,
		Operator: common.NormalizeOperator(f.Op),
		Value:    f.Value,
	}
	switch f.Op {
	case "in", "not in":
		strVal, _ := f.Value.(string)
		parts := strings.Split(strVal, ",")
		arr := make([]interface{}, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				arr = append(arr, trimmed)
			}
		}
		mc.Value = arr
	}
	return mc
}

// applySingleCondition applies a single filter condition and returns matching doc IDs
func applySingleCondition(metaData map[string]interface{}, condition MetaFilterCondition) []string {
	key := condition.Key
	rawValue := condition.Value
	op := condition.Op

	// For most operators, value is a single string; only "in" / "not in" accept lists.
	strValue := fmt.Sprintf("%v", rawValue)

	valueMap, ok := metaData[key].(map[string]interface{})
	if !ok {
		return []string{}
	}

	var result []string

	switch op {
	case "=", "==":
		if docIDs, exists := valueMap[strValue]; exists {
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
			if strings.EqualFold(val, strValue) {
				continue
			}
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
	case "contains":
		for val, docIDs := range valueMap {
			if strings.Contains(strings.ToLower(val), strings.ToLower(strValue)) {
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
		}
	case "not contains":
		for val, docIDs := range valueMap {
			if !strings.Contains(strings.ToLower(val), strings.ToLower(strValue)) {
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
		}
	case "in":
		inValues := metaFilterValues(rawValue)
		for _, v := range inValues {
			if docIDs, exists := valueMap[v]; exists {
				switch ids := docIDs.(type) {
				case []interface{}:
					for _, id := range ids {
						if idStr, ok := id.(string); ok {
							result = append(result, idStr)
						}
					}
				case []string:
					result = append(result, ids...)
				}
			}
		}
	case "not in":
		excludeValues := make(map[string]bool)
		for _, v := range metaFilterValues(rawValue) {
			excludeValues[strings.ToLower(v)] = true
		}
		for val, docIDs := range valueMap {
			if !excludeValues[strings.ToLower(val)] {
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
		}
	case "start with":
		for val, docIDs := range valueMap {
			if strings.HasPrefix(strings.ToLower(val), strings.ToLower(strValue)) {
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
		}
	case "end with":
		for val, docIDs := range valueMap {
			if strings.HasSuffix(strings.ToLower(val), strings.ToLower(strValue)) {
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
		}
	case "empty":
		if len(valueMap) == 0 {
			return []string{}
		}
	case "not empty":
		if len(valueMap) > 0 {
			for _, docIDs := range valueMap {
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
		}
	case ">":
		for val, docIDs := range valueMap {
			if compareValues(val, strValue, ">") {
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
		}
	case "<":
		for val, docIDs := range valueMap {
			if compareValues(val, strValue, "<") {
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
		}
	case ">=":
		for val, docIDs := range valueMap {
			if compareValues(val, strValue, ">=") {
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
		}
	case "<=":
		for val, docIDs := range valueMap {
			if compareValues(val, strValue, "<=") {
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
		}
	default:
		// Default to equality check
		if docIDs, exists := valueMap[strValue]; exists {
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
	}
	return result
}

// metaFilterValues extracts string values from a MetaFilterCondition Value which can
// be a single string, a []string, or a []interface{} (used by "in" / "not in" operators).
func metaFilterValues(value interface{}) []string {
	switch v := value.(type) {
	case string:
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				result = append(result, p)
			}
		}
		return result
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					result = append(result, s)
				}
			}
		}
		return result
	case []string:
		result := make([]string, 0, len(v))
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, s)
			}
		}
		return result
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}

// MetadataConditionToDocIDs applies metadata_condition against pre-loaded
// metadata and returns a comma-separated doc ID string.
// Returns "-999" when conditions are non-empty but match nothing.
func MetadataConditionToDocIDs(metaData common.MetaData, metadataCondition map[string]interface{}) string {
	if metadataCondition == nil {
		return ""
	}
	input := common.ParseAndConvert(metadataCondition)
	if input == nil {
		return ""
	}
	filtered := common.MetaFilter(metaData, input)

	rawConditions, _ := metadataCondition["conditions"].([]interface{})
	if len(rawConditions) > 0 && len(filtered) == 0 {
		return "-999"
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, ",")
}

// ApplyMetaDataFilter applies metadata filtering rules and returns filtered doc_ids
// Supports three modes:
// - auto: generate filter conditions via LLM
// - semi_auto: generate conditions using selected metadata keys only via LLM
// - manual: directly filter based on provided conditions
//
// When kbIDs is supplied, metadata filters are pushed down to the doc metadata
// index (ES/Infinity) via FilterDocIdsByMetaPushdown instead of being evaluated
// in-memory. The in-memory meta_filter path remains the fallback.
func ApplyMetaDataFilter(
	ctx context.Context,
	metaDataFilter map[string]interface{},
	metaData common.MetaData,
	question string,
	chatModel *modelModule.ChatModel,
	baseDocIDs []string,
	kbIDs []string,
	manualValueResolver ...ManualValueResolver,
) ([]string, bool) {
	if metaDataFilter == nil {
		return baseDocIDs, false
	}

	method, _ := metaDataFilter["method"].(string)

	// Helper to run metadata filter with push-down fallback
	// runMetadataFilter executes filter conditions via push-down (ES/Infinity)
	// when possible, falling back to in-memory filtering when push-down is not
	// viable or fails.
	//
	// The nil-vs-empty-slice convention (matching Python):
	//   nil        -> push-down was not viable / errored -> fall back to in-memory
	//   []string{} -> push-down succeeded but found 0 matching docs -> definitive,
	//                 do NOT fall back to in-memory (empty result is authoritative)
	runMetadataFilter := func(conditions []MetaFilterCondition, logic string) []string {
		// Try ES/Infinity push-down first
		if len(conditions) > 0 && len(kbIDs) > 0 {
			docEngine := engine.Get()
			if docEngine != nil {
				// Convert []MetaFilterCondition to []map[string]interface{}
				condMaps := make([]map[string]interface{}, len(conditions))
				for i, c := range conditions {
					condMaps[i] = map[string]interface{}{
						"key":   c.Key,
						"op":    c.Op,
						"value": c.Value,
					}
				}
				pushdownIDs := docEngine.FilterDocIdsByMetaPushdown(ctx, kbIDs, condMaps, logic)
				// nil  = push-down not viable / errored -> fall back to in-memory
				// non-nil (including empty slice) = push-down definitive -> use as-is
				if pushdownIDs != nil {
					return pushdownIDs
				}
			}
		}
		// Fall back to in-memory filter
		return ApplyMetaFilter(metaData, conditions, logic)
	}

	switch method {
	case "auto":
		filters, err := GenMetaFilter(ctx, chatModel, metaData, question, nil)
		if err != nil {
			common.Warn("Failed to generate meta filter", zap.Error(err))
			return baseDocIDs, false
		}
		filteredIDs := runMetadataFilter(filters.Conditions, filters.Logic)
		docIDs := constrainDocIDs(baseDocIDs, filteredIDs)
		if len(docIDs) == 0 {
			return nil, true // Return nil to indicate auto filter returned empty
		}
		return docIDs, false

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
			filteredMeta := make(common.MetaData)
			for _, key := range selectedKeys {
				if val, exists := metaData[key]; exists {
					filteredMeta[key] = val
				}
			}

			if len(filteredMeta) > 0 {
				filters, err := GenMetaFilter(ctx, chatModel, filteredMeta, question, constraints)
				if err != nil {
					common.Warn("Failed to generate meta filter", zap.Error(err))
					return baseDocIDs, false
				}
				filteredIDs := runMetadataFilter(filters.Conditions, filters.Logic)
				docIDs := constrainDocIDs(baseDocIDs, filteredIDs)
				if len(docIDs) == 0 {
					return nil, true
				}
				return docIDs, false
			}
		}

	case "manual":
		manualFilters, _ := metaDataFilter["manual"].([]interface{})
		logic := "and"
		if logicVal, ok := metaDataFilter["logic"].(string); ok {
			logic = logicVal
		}
		if len(manualFilters) == 0 {
			return baseDocIDs, false
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
				if value, exists := cond["value"]; exists {
					condition.Value = value
				}
				if op, ok := cond["op"].(string); ok {
					condition.Op = op
				}
				conditions = append(conditions, condition)
			}
		}

		filteredIDs := runMetadataFilter(conditions, logic)
		docIDs := constrainDocIDs(baseDocIDs, filteredIDs)
		if len(manualFilters) > 0 && len(docIDs) == 0 {
			return []string{NoMatchDocIDSentinel}, false
		}
		return docIDs, false
	}

	return baseDocIDs, false
}

func constrainDocIDs(baseDocIDs, filteredDocIDs []string) []string {
	filteredDocIDs = common.Deduplicate(filteredDocIDs)
	if len(baseDocIDs) == 0 {
		return filteredDocIDs
	}
	if len(filteredDocIDs) == 0 {
		return []string{}
	}

	filteredSet := make(map[string]struct{}, len(filteredDocIDs))
	for _, docID := range filteredDocIDs {
		filteredSet[docID] = struct{}{}
	}
	result := make([]string, 0, min(len(baseDocIDs), len(filteredSet)))
	seen := make(map[string]struct{}, len(baseDocIDs))
	for _, docID := range baseDocIDs {
		if _, allowed := filteredSet[docID]; !allowed {
			continue
		}
		if _, exists := seen[docID]; exists {
			continue
		}
		seen[docID] = struct{}{}
		result = append(result, docID)
	}
	return result
}
