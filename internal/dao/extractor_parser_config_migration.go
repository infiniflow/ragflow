package dao

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// This step follows rc2.dev1 because database migrations share one version marker.
const extractorParserConfigTargetVersion = "v1.0.0-rc2.dev2"
const builtinExtractorNodeID = "Extractor:AutoExtractDefault"

var builtinExtractorPipelines = map[string]struct{}{
	"general":      {},
	"naive":        {},
	"audio":        {},
	"book":         {},
	"email":        {},
	"laws":         {},
	"manual":       {},
	"one":          {},
	"paper":        {},
	"picture":      {},
	"presentation": {},
}
var extractorFlatKeys = []string{"auto_keywords", "auto_questions", "enable_metadata", "metadata", "built_in_metadata"}

type extractorConfigMigrationRow struct {
	ID           string  `gorm:"column:id"`
	ParserID     string  `gorm:"column:parser_id"`
	PipelineID   *string `gorm:"column:pipeline_id"`
	ParserConfig *string `gorm:"column:parser_config"`
}

func migrateExtractorParserConfigs(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return err
	}
	if shouldSkipMigration(version, extractorParserConfigTargetVersion) {
		return nil
	}
	found := false
	for _, target := range []struct {
		table string
		model any
	}{{"knowledgebase", &entity.Knowledgebase{}}, {"document", &entity.Document{}}} {
		if !db.WithContext(ctx).Migrator().HasTable(target.model) {
			continue
		}
		found = true
		count, err := migrateExtractorConfigRows(ctx, db, target.model)
		if err != nil {
			return fmt.Errorf("migrate %s extractor configs: %w", target.table, err)
		}
		common.Info("Migrated Python extractor configs", zap.String("table", target.table), zap.Int("rows", count))
	}
	if !found {
		return nil
	}
	return setDatabaseMigrationVersion(ctx, db, extractorParserConfigTargetVersion)
}

func migrateExtractorConfigRows(ctx context.Context, db *gorm.DB, model any) (int, error) {
	count := 0
	var rows []extractorConfigMigrationRow
	result := db.WithContext(ctx).Model(model).Select("id", "parser_id", "pipeline_id", "parser_config").Order("id").FindInBatches(&rows, parserConfigMigrationBatchSize, func(batch *gorm.DB, _ int) error {
		return batch.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				if row.ParserConfig == nil || strings.TrimSpace(*row.ParserConfig) == "" {
					continue
				}
				config, ok := parseParserConfig(row.ParserConfig)
				if !ok {
					return fmt.Errorf("row %s: invalid parser_config JSON", row.ID)
				}
				changed, err := migrateExtractorConfig(config, row.ParserID, row.PipelineID)
				if err != nil {
					return fmt.Errorf("row %s: %w", row.ID, err)
				}
				if !changed {
					continue
				}
				if err := tx.Model(model).Where("id = ?", row.ID).Update("parser_config", entity.JSONMap(config)).Error; err != nil {
					return err
				}
				count++
			}
			return nil
		})
	})
	return count, result.Error
}

// migrateExtractorConfig rewrites only Python's top-level extraction settings.
// Existing node fields win over flat Python values. A modular metadata transport
// object replaces node metadata, matching ApplyComponentScopedParserConfig.
// Unsupported inputs return an error before the config is mutated.
func migrateExtractorConfig(config map[string]any, parserID string, pipelineID *string) (bool, error) {
	present := false
	for _, key := range extractorFlatKeys {
		if _, ok := config[key]; ok {
			present = true
			break
		}
	}
	if !present {
		return false, nil
	}
	var targets []map[string]any
	var newNode map[string]any
	if pipelineID == nil || strings.TrimSpace(*pipelineID) == "" {
		_, builtin := builtinExtractorPipelines[strings.ToLower(strings.TrimSpace(parserID))]
		if !builtin {
			return false, nil
		}
		if raw, exists := config[builtinExtractorNodeID]; exists {
			node, ok := raw.(map[string]any)
			if !ok {
				return false, fmt.Errorf("%s is not an object", builtinExtractorNodeID)
			}
			targets = append(targets, node)
		} else {
			newNode = map[string]any{}
			targets = append(targets, newNode)
		}
	} else {
		for id, raw := range config {
			id = strings.ToLower(id)
			if strings.HasPrefix(id, "extractor:") || strings.HasPrefix(id, "extractor_") {
				if node, ok := raw.(map[string]any); ok {
					targets = append(targets, node)
				}
			}
		}
		if len(targets) == 0 {
			return false, nil
		}
	}
	groups, replaceMetadata, err := convertPythonExtractorGroups(config)
	if err != nil {
		return false, err
	}
	for _, node := range targets {
		for name := range groups {
			if raw, exists := node[name]; exists && !(name == "metadata" && replaceMetadata) {
				if _, ok := raw.(map[string]any); !ok {
					return false, fmt.Errorf("%s group is not an object", name)
				}
			}
		}
	}
	for _, node := range targets {
		for name, defaults := range groups {
			group, ok := node[name].(map[string]any)
			if !ok || (name == "metadata" && replaceMetadata) {
				node[name] = maps.Clone(defaults)
				continue
			}
			for key, value := range defaults {
				if _, exists := group[key]; !exists {
					group[key] = value
				}
			}
		}
	}
	if newNode != nil {
		config[builtinExtractorNodeID] = newNode
	}
	for _, key := range extractorFlatKeys {
		delete(config, key)
	}
	return true, nil
}

func convertPythonExtractorGroups(config map[string]any) (map[string]map[string]any, bool, error) {
	groups := map[string]map[string]any{}
	for _, field := range []struct{ key, group string }{{"auto_keywords", "keywords"}, {"auto_questions", "questions"}} {
		if raw, exists := config[field.key]; exists {
			value, ok := raw.(float64)
			if raw == nil {
				value, ok = 0, true
			}
			if !ok || value < 0 || value != float64(int(value)) {
				return nil, false, fmt.Errorf("%s must be a non-negative integer", field.key)
			}
			groups[field.group] = map[string]any{"top_n": int(value)}
		}
	}
	source := config
	replaceMetadata := false
	_, hasToggle := config["enable_metadata"]
	_, hasFields := config["metadata"]
	_, hasBuiltIn := config["built_in_metadata"]
	if obj, ok := config["metadata"].(map[string]any); ok {
		if len(obj) == 0 && !hasToggle && !hasBuiltIn {
			source, replaceMetadata = obj, true
		}
		for _, key := range []string{"enabled", "metadata", "built_in_metadata"} {
			if _, exists := obj[key]; exists {
				source = obj
				replaceMetadata = true
				break
			}
		}
	}
	if hasToggle || hasFields || hasBuiltIn {
		flag := source["enable_metadata"]
		if replaceMetadata {
			flag = source["enabled"]
		}
		if flag != nil {
			switch value := flag.(type) {
			case bool:
			case float64:
				if value != 0 && value != 1 {
					return nil, false, fmt.Errorf("metadata enabled must be bool or 0/1")
				}
			default:
				return nil, false, fmt.Errorf("metadata enabled must be bool or 0/1")
			}
		}
		fields, err := convertExtractorMetadataFields(source["metadata"])
		if err != nil {
			return nil, false, fmt.Errorf("metadata: %w", err)
		}
		builtIn, err := convertExtractorMetadataFields(source["built_in_metadata"])
		if err != nil {
			return nil, false, fmt.Errorf("built_in_metadata: %w", err)
		}
		groups["metadata"] = map[string]any{"enabled": common.ParserConfigBool(flag), "metadata": fields, "built_in_metadata": builtIn}
	}
	return groups, replaceMetadata, nil
}

// convertExtractorMetadataFields accepts field lists and flat JSON Schemas that
// fit MetadataFieldDef. Constraints the target cannot express are rejected.
func convertExtractorMetadataFields(raw any) ([]common.MetadataFieldDef, error) {
	if raw == nil {
		return []common.MetadataFieldDef{}, nil
	}
	if schema, ok := raw.(map[string]any); ok {
		if len(schema) == 0 {
			return []common.MetadataFieldDef{}, nil
		}
		for key := range schema {
			if key != "type" && key != "properties" && key != "additionalProperties" {
				return nil, fmt.Errorf("unsupported JSON Schema keyword %s", key)
			}
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || schema["type"] != "object" || schema["additionalProperties"] != false {
			return nil, fmt.Errorf("expected flat object schema with additionalProperties=false")
		}
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		fields := make([]any, 0, len(names))
		for _, name := range names {
			prop, ok := properties[name].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("property %s is not an object", name)
			}
			for key := range prop {
				if key != "type" && key != "description" && key != "enum" {
					return nil, fmt.Errorf("unsupported property constraint %s.%s", name, key)
				}
			}
			field := maps.Clone(prop)
			field["key"] = name
			fields = append(fields, field)
		}
		raw = fields
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected field list")
	}
	normalized := make([]any, 0, len(list))
	for _, rawField := range list {
		field, ok := rawField.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field is not an object")
		}
		key, ok := field["key"].(string)
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("field requires a non-empty key without surrounding whitespace")
		}
		for _, name := range []string{"type", "description", "descriptions"} {
			if value := field[name]; value != nil {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s.%s must be a string", key, name)
				}
			}
		}
		if value := field["enum"]; value != nil {
			values, ok := value.([]any)
			if !ok {
				return nil, fmt.Errorf("%s.enum must be a string list", key)
			}
			for _, value := range values {
				if _, ok := value.(string); !ok {
					return nil, fmt.Errorf("%s.enum must be a string list", key)
				}
			}
		}
		item := maps.Clone(field)
		if description, _ := item["description"].(string); description == "" {
			item["description"] = item["descriptions"]
		}
		normalized = append(normalized, item)
	}
	return common.MetadataFieldDefsFromRaw(normalized), nil
}
