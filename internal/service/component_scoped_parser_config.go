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
	if !enableMetadata {
		if metaMap, ok := parserConfig["metadata"].(map[string]any); ok {
			enableMetadata = parserConfigTruthy(metaMap["enabled"])
		}
	}
	if !enableMetadata {
		enableMetadata = parserConfigTruthy(parserConfig["enabled"])
	}

	metadataFields := extractFieldList(parserConfig, "fields", "metadata")
	builtInMetadataFields := extractFieldList(parserConfig, "built_in_metadata", "")

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
			delete(params, "enable_metadata")

			hasTopLevelFields := len(metadataFields) > 0 || len(builtInMetadataFields) > 0

			if hasTopLevelFields {
				params["metadata"] = map[string]any{
					"enabled":           enableMetadata,
					"metadata":          metadataFields,
					"built_in_metadata": builtInMetadataFields,
				}
			} else {
				nodeCustom := extractFieldList(params, "fields", "metadata")
				nodeBuiltIn := extractFieldList(params, "built_in_metadata", "")
				nodeEnabled := false

				if metaMap, ok := params["metadata"].(map[string]any); ok && metaMap["enabled"] != nil {
					nodeEnabled = parserConfigTruthy(metaMap["enabled"])
				} else if mcMap, ok := params["metadata_config"].(map[string]any); ok && mcMap["enabled"] != nil {
					nodeEnabled = parserConfigTruthy(mcMap["enabled"])
				} else if params["enable_metadata"] != nil {
					nodeEnabled = parserConfigTruthy(params["enable_metadata"])
				} else if enableMetadata {
					nodeEnabled = true
				}

				if len(nodeCustom) > 0 || len(nodeBuiltIn) > 0 || nodeEnabled {
					params["metadata"] = map[string]any{
						"enabled":           nodeEnabled,
						"metadata":          nodeCustom,
						"built_in_metadata": nodeBuiltIn,
					}
				} else {
					params["metadata"] = map[string]any{
						"enabled":           false,
						"metadata":          []any{},
						"built_in_metadata": []any{},
					}
				}
			}
			delete(params, "metadata_config")
		case strings.HasPrefix(cpnLower, "compiler:") || strings.HasPrefix(cpnLower, "compiler_"):
			if value, _ := params["llm_id"].(string); strings.TrimSpace(value) == "" && strings.TrimSpace(llmID) != "" {
				params["llm_id"] = llmID
			}
		}
	}

	return parserConfig
}

func mergeMetadataFields(parserConfig entity.JSONMap) []any {
	custom := extractFieldList(parserConfig, "fields", "metadata")
	builtIn := extractFieldList(parserConfig, "built_in_metadata", "")
	out := make([]any, 0, len(custom)+len(builtIn))
	out = append(out, custom...)
	out = append(out, builtIn...)
	return out
}

func extractFieldList(parserConfig entity.JSONMap, primaryKey, fallbackKey string) []any {
	if parserConfig == nil {
		return []any{}
	}
	if raw, ok := parserConfig[primaryKey]; ok && raw != nil {
		if slice := extractValidFields(raw); len(slice) > 0 || primaryKey == "fields" {
			return slice
		}
	}
	if fallbackKey != "" {
		if raw, ok := parserConfig[fallbackKey]; ok && raw != nil {
			if slice := extractValidFields(raw); len(slice) > 0 {
				return slice
			}
		}
	}
	for _, subKey := range []string{"metadata", "metadata_config"} {
		if metaMap, ok := parserConfig[subKey].(map[string]any); ok {
			if raw, ok := metaMap[primaryKey]; ok && raw != nil {
				if slice := extractValidFields(raw); len(slice) > 0 {
					return slice
				}
			}
			if fallbackKey != "" {
				if raw, ok := metaMap[fallbackKey]; ok && raw != nil {
					if slice := extractValidFields(raw); len(slice) > 0 {
						return slice
					}
				}
			}
		}
	}
	return []any{}
}

func extractValidFields(value any) []any {
	items := anySlice(value)
	if items == nil {
		return []any{}
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
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
	case []map[string]string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			m := make(map[string]any, len(item))
			for k, v := range item {
				m[k] = v
			}
			out = append(out, m)
		}
		return out
	case []MetadataConfigField:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, map[string]any{
				"key":         item.Key,
				"type":        item.Type,
				"description": item.Description,
				"enum":        item.Enum,
			})
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
	if _, ok := parserConfig["enabled"]; ok {
		return true
	}
	for _, key := range []string{"fields", "metadata", "built_in_metadata"} {
		if anySlice(parserConfig[key]) != nil {
			return true
		}
	}
	if _, ok := parserConfig["metadata"].(map[string]any); ok {
		return true
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
