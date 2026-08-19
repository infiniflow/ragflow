package service

import (
	"strings"

	"ragflow/internal/entity"
)

// ApplyComponentScopedParserConfig fills dataset-scoped component params onto a
// parser_config map without reintroducing top-level flat fields. It mutates the
// provided map in place and returns it for convenience.
func ApplyComponentScopedParserConfig(
	parserConfig entity.JSONMap,
	llmID string,
) entity.JSONMap {
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}

	enableMetadata := parserConfigTruthy(parserConfig["enable_metadata"])
	metadataFields := mergeMetadataFields(parserConfig)
	hasMetadataConfig := hasMetadataConfigShape(parserConfig)

	for cpnID, raw := range parserConfig {
		params, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		cpnLower := strings.ToLower(cpnID)
		switch {
		case strings.HasPrefix(cpnLower, "extractor:") || strings.HasPrefix(cpnLower, "extractor_"):
			if value, _ := params["llm_id"].(string); strings.TrimSpace(value) == "" && strings.TrimSpace(llmID) != "" {
				params["llm_id"] = llmID
			}
			if enableMetadata && len(metadataFields) > 0 {
				params["enable_metadata"] = 1
				params["metadata"] = metadataFields
			} else if hasMetadataConfig {
				params["enable_metadata"] = 0
				params["metadata"] = []any{}
			}
		case strings.HasPrefix(cpnLower, "compiler:") || strings.HasPrefix(cpnLower, "compiler_"):
			if value, _ := params["llm_id"].(string); strings.TrimSpace(value) == "" && strings.TrimSpace(llmID) != "" {
				params["llm_id"] = llmID
			}
		}
	}

	return parserConfig
}

func mergeMetadataFields(parserConfig entity.JSONMap) []any {
	var out []any
	for _, key := range []string{"metadata", "built_in_metadata"} {
		for _, item := range anySlice(parserConfig[key]) {
			field, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name, _ := field["key"].(string)
			if strings.TrimSpace(name) == "" {
				continue
			}
			out = append(out, field)
		}
	}
	return out
}

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func hasMetadataConfigShape(parserConfig entity.JSONMap) bool {
	if parserConfig == nil {
		return false
	}
	if _, ok := parserConfig["enable_metadata"]; ok {
		return true
	}
	for _, key := range []string{"metadata", "built_in_metadata"} {
		if anySlice(parserConfig[key]) != nil {
			return true
		}
	}
	return false
}

func parserConfigTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch typed {
		case "true", "True", "TRUE", "1":
			return true
		}
	case float64:
		return typed > 0
	case int:
		return typed > 0
	case int64:
		return typed > 0
	}
	return false
}
