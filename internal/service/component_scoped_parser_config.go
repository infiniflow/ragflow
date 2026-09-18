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

// tableColumnSettingKeys are the root-level entries a table-parser run reads and
// writes back: the mode and the per-column roles someone configured, plus the
// schema the run publishes (the discovered table_column_names, and the field_map
// the SQL retrieval prompt builds from them).
//
// They are the flat alternative to the component-scoped shape a parser dialog
// writes, so CleanComponentParams drops them from every key it does not
// recognize as a component id. A parser_config rebuilt from the pipeline DSL
// therefore has to move them back in, which is what this normalization does.
var tableColumnSettingKeys = []string{
	"table_column_mode",
	"table_column_names",
	"table_column_roles",
	"field_map",
}

// NormalizeTableColumnSettings re-attaches the root-level table column settings
// that rebuilding a parser_config from the pipeline DSL drops.
//
// A value the request itself supplied wins over the stored one, so an update can
// still change or clear a setting; a key the request does not mention keeps its
// stored value, so editing any other parser_config section — topn, the embedding
// model, the canvas parameters — cannot erase the schema the last table run
// published.
func NormalizeTableColumnSettings(
	rebuilt entity.JSONMap,
	incoming map[string]interface{},
	existing entity.JSONMap,
) entity.JSONMap {
	if rebuilt == nil {
		rebuilt = entity.JSONMap{}
	}
	for _, key := range tableColumnSettingKeys {
		if value, ok := incoming[key]; ok {
			rebuilt[key] = value
			continue
		}
		if value, ok := existing[key]; ok {
			rebuilt[key] = value
		}
	}
	return rebuilt
}

func cloneJSONMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
