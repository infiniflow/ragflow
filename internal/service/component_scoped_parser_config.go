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

	rawMetaSlice := anySlice(parserConfig["metadata"])
	if len(rawMetaSlice) == 0 {
		if ext, ok := parserConfig["ext"].(map[string]any); ok {
			rawMetaSlice = anySlice(ext["metadata"])
		}
	}
	rawBuiltInSlice := anySlice(parserConfig["built_in_metadata"])
	if len(rawBuiltInSlice) == 0 {
		if ext, ok := parserConfig["ext"].(map[string]any); ok {
			rawBuiltInSlice = anySlice(ext["built_in_metadata"])
		}
	}
	enableMetadataVal := parserConfig["enable_metadata"]
	if enableMetadataVal == nil {
		if ext, ok := parserConfig["ext"].(map[string]any); ok {
			enableMetadataVal = ext["enable_metadata"]
		}
	}
	enableMetadata := ParserConfigTruthy(enableMetadataVal)
	if !enableMetadata && enableMetadataVal == nil {
		enableMetadata = len(rawMetaSlice) > 0 || len(rawBuiltInSlice) > 0
	}
	metadataFields := mergeMetadataFields(parserConfig)

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

			nodeLegacyEnabledVal, hasLegacyNodeEnabled := params["enable_metadata"]
			delete(params, "enable_metadata")

			customMeta := validMetadataFields(rawMetaSlice)
			builtInMeta := validMetadataFields(rawBuiltInSlice)
			if metaObj, ok := params["metadata"].(map[string]any); ok {
				if rawMetaSlice == nil {
					customMeta = validMetadataFields(anySlice(metaObj["metadata"]))
				}
				if rawBuiltInSlice == nil {
					builtInMeta = validMetadataFields(anySlice(metaObj["built_in_metadata"]))
				}
			}

			if metaObj, ok := params["metadata"].(map[string]any); ok {
				if enabledVal, hasEnabled := metaObj["enabled"]; hasEnabled {
					params["metadata"] = map[string]any{
						"enabled":           ParserConfigTruthy(enabledVal),
						"metadata":          customMeta,
						"built_in_metadata": builtInMeta,
					}
					continue
				}
			}

			nodeEnabled := enableMetadata
			if hasLegacyNodeEnabled {
				nodeEnabled = ParserConfigTruthy(nodeLegacyEnabledVal)
			}
			if nodeEnabled && len(metadataFields) > 0 {
				params["metadata"] = map[string]any{
					"enabled":           true,
					"metadata":          customMeta,
					"built_in_metadata": builtInMeta,
				}
			} else {
				params["metadata"] = map[string]any{
					"enabled":           false,
					"metadata":          customMeta,
					"built_in_metadata": builtInMeta,
				}
			}
		case strings.HasPrefix(cpnLower, "compiler:") || strings.HasPrefix(cpnLower, "compiler_"):
			if value, _ := params["llm_id"].(string); strings.TrimSpace(value) == "" && strings.TrimSpace(llmID) != "" {
				params["llm_id"] = llmID
			}
		}
	}

	return parserConfig
}

func validMetadataFields(items []any) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		field, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := field["key"].(string)
		if strings.TrimSpace(name) != "" {
			out = append(out, field)
		}
	}
	return out
}

func mergeMetadataFields(parserConfig entity.JSONMap) []any {
	var out []any
	for _, key := range []string{"metadata", "built_in_metadata"} {
		out = append(out, validMetadataFields(anySlice(parserConfig[key]))...)
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

// ParserConfigTruthy evaluates whether a parser config flag value represents true.
func ParserConfigTruthy(value any) bool {
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
