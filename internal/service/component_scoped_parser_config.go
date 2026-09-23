package service

import (
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// ApplyComponentScopedParserConfig scopes the incoming modular metadata config
// (parserConfig["metadata"] = {enabled, metadata, built_in_metadata}) onto every
// Extractor node, then drops the top-level modular key so the Extractor node is the single
// place that config is stored. The top-level modular key is input-only — callers may still
// send it, but it is never persisted again.
//
// A document-level metadata map (plain key/values under the same "metadata" key) is a
// different thing: it is neither scoped onto a node nor stripped — see
// common.IsModularMetadataConfig.
//
// It also strips the legacy flat metadata fields (enable_metadata / metadata_config /
// built_in_metadata / fields) both at the top level and on nodes; those forms are not
// supported as input at all.
//
// When the incoming modular config is present it is authoritative and replaces each
// Extractor node's metadata; when absent, the node's own modular metadata is preserved.
// It mutates the provided map in place and returns it for convenience.
func ApplyComponentScopedParserConfig(
	parserConfig entity.JSONMap,
	llmID string,
) entity.JSONMap {
	if parserConfig == nil {
		parserConfig = entity.JSONMap{}
	}

	var datasetMeta map[string]any
	if mm, ok := parserConfig["metadata"].(map[string]any); ok && common.IsModularMetadataConfig(mm) {
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

	// Drop the legacy flat fields and the input-only modular object. The modular config
	// now lives on the Extractor node(s) it was scoped onto above, so the top level never
	// carries it; a document-level metadata map stays untouched.
	delete(parserConfig, "enable_metadata")
	delete(parserConfig, "metadata_config")
	delete(parserConfig, "built_in_metadata")
	delete(parserConfig, "fields")
	if datasetMeta != nil {
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
