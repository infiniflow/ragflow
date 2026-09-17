package service

import (
	"strings"

	"ragflow/internal/entity"
)

// ApplyComponentScopedParserConfig scopes a dataset-level modular metadata
// config (parserConfig["metadata"] = {enabled, metadata, built_in_metadata})
// onto every Extractor node and strips the legacy flat metadata fields
// (enable_metadata / metadata_config / built_in_metadata / fields) both at the
// top level and on nodes. Legacy flat metadata forms are intentionally not
// supported. When a dataset-level modular metadata config is present it is
// authoritative and replaces each Extractor node's metadata; when absent, the
// node's own modular metadata is preserved. It mutates the provided map in
// place and returns it for convenience.
func ApplyComponentScopedParserConfig(
	parserConfig entity.JSONMap,
	llmID string,
) entity.JSONMap {
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}

	var datasetMeta map[string]any
	if mm, ok := parserConfig["metadata"].(map[string]any); ok {
		datasetMeta = cloneJSONMap(mm)
	}

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
			delete(params, "metadata_config")
			delete(params, "built_in_metadata")
			delete(params, "fields")
			if datasetMeta != nil {
				params["metadata"] = cloneJSONMap(datasetMeta)
				continue
			}
			if _, ok := params["metadata"].(map[string]any); !ok {
				params["metadata"] = map[string]any{
					"enabled":           false,
					"metadata":          []any{},
					"built_in_metadata": []any{},
				}
			}
		case strings.HasPrefix(cpnLower, "compiler:") || strings.HasPrefix(cpnLower, "compiler_"):
			if value, _ := params["llm_id"].(string); strings.TrimSpace(value) == "" && strings.TrimSpace(llmID) != "" {
				params["llm_id"] = llmID
			}
		}
	}

	// Strip legacy top-level flat metadata fields; keep the modular object
	// under "metadata" when present, otherwise drop the key entirely.
	delete(parserConfig, "enable_metadata")
	delete(parserConfig, "metadata_config")
	delete(parserConfig, "built_in_metadata")
	delete(parserConfig, "fields")
	if datasetMeta == nil {
		delete(parserConfig, "metadata")
	}

	return parserConfig
}

func cloneJSONMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
