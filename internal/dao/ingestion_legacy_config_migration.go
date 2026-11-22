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

package dao

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// ingestionLegacyConfigTargetVersion is the version a completed legacy
// ingestion-config normalization brings the database to. It sorts above the
// resource-owner backfill (v1.0.0.1), so this step runs after every earlier
// version-gated migration instead of moving the shared marker past them.
const ingestionLegacyConfigTargetVersion = "v1.0.0.2"

// migrateIngestionLegacyConfigs normalizes ingestion component params that were
// stored by Python or by early Go builds. It covers both the top-level flat
// keys (including rows a previous cleanup-only marker skipped) and the legacy
// keys inside Extractor / TitleChunker / TokenChunker nodes. Custom canvas DSLs
// are normalized in user_canvas and user_canvas_version because those rows, not
// the parser_config snapshot, are the source of truth for custom pipelines.
// parser_config rows fail closed; a canvas row that cannot be parsed or
// normalized is logged and skipped instead.
func migrateIngestionLegacyConfigs(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return err
	}
	if shouldSkipMigration(version, ingestionLegacyConfigTargetVersion) {
		return nil
	}

	found := false
	for _, target := range []struct {
		table string
		model any
	}{
		{"knowledgebase", &entity.Knowledgebase{}},
		{"document", &entity.Document{}},
	} {
		if !db.WithContext(ctx).Migrator().HasTable(target.model) {
			continue
		}
		found = true
		count, err := migrateIngestionParserConfigRows(ctx, db, target.model, target.table, true)
		if err != nil {
			return fmt.Errorf("migrate %s legacy ingestion config: %w", target.table, err)
		}
		common.Info("Normalized legacy ingestion parser configs",
			zap.String("table", target.table),
			zap.String("version", ingestionLegacyConfigTargetVersion),
			zap.Int("rows", count))
	}
	for _, target := range []struct {
		table string
		model any
	}{
		{"user_canvas", &entity.UserCanvas{}},
		{"user_canvas_version", &entity.UserCanvasVersion{}},
	} {
		if !db.WithContext(ctx).Migrator().HasTable(target.model) {
			continue
		}
		found = true
		count, err := migrateCanvasDSLRows(ctx, db, target.model, target.table)
		if err != nil {
			return fmt.Errorf("migrate %s legacy ingestion config: %w", target.table, err)
		}
		common.Info("Normalized legacy ingestion canvas DSLs",
			zap.String("table", target.table),
			zap.String("version", ingestionLegacyConfigTargetVersion),
			zap.Int("rows", count))
	}
	if !found {
		return nil
	}
	return setDatabaseMigrationVersion(ctx, db, ingestionLegacyConfigTargetVersion)
}

type canvasDSLMigrationRow struct {
	ID  string  `gorm:"column:id"`
	DSL *string `gorm:"column:dsl"`
}

// migrateCanvasDSLRows normalizes one canvas table. A row whose DSL is not
// valid JSON or that the normalizer cannot convert is logged and skipped:
// canvas DSLs are user-authored and are not schema-validated on save, so one
// bad row must not block the upgrade for every tenant. The row keeps its
// legacy shape, which the frontend fallback still reads and re-saves in the
// modular format.
func migrateCanvasDSLRows(ctx context.Context, db *gorm.DB, model any, table string) (int, error) {
	count := 0
	var rows []canvasDSLMigrationRow
	result := db.WithContext(ctx).Model(model).
		Select("id", "dsl").Order("id").
		FindInBatches(&rows, parserConfigMigrationBatchSize, func(batch *gorm.DB, _ int) error {
			return batch.Transaction(func(tx *gorm.DB) error {
				for _, row := range rows {
					if row.DSL == nil || strings.TrimSpace(*row.DSL) == "" {
						continue
					}
					var dsl map[string]any
					if err := json.Unmarshal([]byte(*row.DSL), &dsl); err != nil {
						common.Warn("Skipping canvas DSL that is not valid JSON",
							zap.String("table", table),
							zap.String("row_id", row.ID),
							zap.Error(err))
						continue
					}
					if dsl == nil {
						continue
					}
					changed, err := normalizeCanvasDSL(dsl, ingestionConfigRow{table: table, rowID: row.ID})
					if err != nil {
						common.Warn("Skipping canvas DSL the migration cannot normalize",
							zap.String("table", table),
							zap.String("row_id", row.ID),
							zap.Error(err))
						continue
					}
					if !changed {
						continue
					}
					if err := tx.Model(model).Where("id = ?", row.ID).
						Update("dsl", entity.JSONMap(dsl)).Error; err != nil {
						return fmt.Errorf("row %s: update dsl: %w", row.ID, err)
					}
					count++
				}
				return nil
			})
		})
	return count, result.Error
}

// normalizeLegacyIngestionNodes walks component-scoped parser_config keys and
// rewrites the component-specific legacy shapes. Unrelated nodes are untouched.
func normalizeLegacyIngestionNodes(config map[string]any, row ingestionConfigRow) (bool, error) {
	changed := false
	for id, raw := range config {
		params, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		nodeChanged, err := normalizeLegacyComponentParams(id, "", params, row)
		if err != nil {
			return false, fmt.Errorf("%s: %w", id, err)
		}
		changed = changed || nodeChanged
	}
	return changed, nil
}

func normalizeLegacyComponentParams(id, componentName string, params map[string]any, row ingestionConfigRow) (bool, error) {
	if params == nil {
		return false, nil
	}
	kind, _, hasKind := strings.Cut(id, ":")
	if !hasKind || !isLegacyComponentKind(kind) {
		kind = strings.TrimSpace(componentName)
	}
	switch kind {
	case common.ComponentNameExtractor:
		fieldName, _ := params["field_name"].(string)
		if fieldName != "" && extractorSysPromptGroup(fieldName) == "" {
			row.retire("field_name", fmt.Sprintf("component %s: field_name %q has no Go consumer", id, fieldName))
			if raw, exists := params["sys_prompt"]; exists && raw != nil {
				row.retire("sys_prompt", fmt.Sprintf("component %s: field_name %q has no prompt target", id, fieldName))
			}
		}
		return normalizeLegacyExtractorNode(params)
	case common.ComponentNameTitleChunker,
		common.ComponentNameGroupTitleChunker,
		common.ComponentNameHierarchyTitleChunker,
		common.ComponentNameManualChunker:
		return normalizeLegacyTitleChunkerNode(params)
	case common.ComponentNameTokenChunker:
		return normalizeLegacyTokenChunkerNode(params)
	default:
		return false, nil
	}
}

func isLegacyComponentKind(kind string) bool {
	switch kind {
	case common.ComponentNameExtractor,
		common.ComponentNameTitleChunker,
		common.ComponentNameGroupTitleChunker,
		common.ComponentNameHierarchyTitleChunker,
		common.ComponentNameManualChunker,
		common.ComponentNameTokenChunker:
		return true
	default:
		return false
	}
}

func normalizeLegacyExtractorNode(params map[string]any) (bool, error) {
	changed, err := normalizeExtractorGroupAliases(params)
	if err != nil {
		return false, err
	}
	if !hasLegacyExtractorNodeFlatKey(params) {
		return changed, nil
	}
	fieldName, _ := params["field_name"].(string)
	groups, _, err := convertPythonExtractorGroups(params, extractorSysPromptGroup(fieldName))
	if err != nil {
		return false, err
	}
	applied, err := applyExtractorGroups(params, groups, false)
	if err != nil {
		return false, err
	}
	return changed || applied, nil
}

// normalizeExtractorGroupAliases rewrites the pre-modular in-group key
// "sys_prompt" to the current "system_prompt" without touching current groups.
func normalizeExtractorGroupAliases(params map[string]any) (bool, error) {
	changed := false
	for _, name := range []string{"keywords", "questions", "summary"} {
		group, ok := params[name].(map[string]any)
		if !ok {
			continue
		}
		raw, exists := group["sys_prompt"]
		if !exists {
			continue
		}
		value, err := extractorString(name+".sys_prompt", raw)
		if err != nil {
			return false, err
		}
		if _, exists := group["system_prompt"]; !exists {
			group["system_prompt"] = value
		}
		delete(group, "sys_prompt")
		changed = true
	}
	return changed, nil
}

func normalizeLegacyTitleChunkerNode(params map[string]any) (bool, error) {
	raw, exists := params["rules"]
	if !exists {
		return false, nil
	}
	if raw == nil {
		delete(params, "rules")
		return true, nil
	}
	levels, err := titleRulesToLevels(raw)
	if err != nil {
		return false, fmt.Errorf("rules: %w", err)
	}
	if !validTitleLevels(params["levels"]) {
		params["levels"] = levels
	}
	delete(params, "rules")
	return true, nil
}

func validTitleLevels(raw any) bool {
	switch levels := raw.(type) {
	case [][]string:
		return len(levels) > 0
	case []any:
		if len(levels) == 0 {
			return false
		}
		for _, group := range levels {
			if _, ok := group.([]any); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func titleRulesToLevels(raw any) ([]any, error) {
	rules, ok := anyObjectSlice(raw)
	if !ok {
		return nil, fmt.Errorf("must be an array of objects")
	}
	levels := make([]any, 0, len(rules))
	for index, rule := range rules {
		rawLevels, exists := rule["levels"]
		if !exists {
			return nil, fmt.Errorf("rule %d is missing levels", index)
		}
		levelItems, ok := anyObjectSlice(rawLevels)
		if !ok {
			return nil, fmt.Errorf("rule %d levels must be an array of objects", index)
		}
		level := make([]any, 0, len(levelItems))
		for levelIndex, item := range levelItems {
			rawExpression, exists := item["expression"]
			if !exists {
				continue
			}
			expression, ok := rawExpression.(string)
			if !ok {
				return nil, fmt.Errorf("rule %d level %d expression must be a string", index, levelIndex)
			}
			if strings.TrimSpace(expression) == "" {
				continue
			}
			level = append(level, expression)
		}
		if len(level) > 0 {
			levels = append(levels, level)
		}
	}
	return levels, nil
}

func anyObjectSlice(raw any) ([]map[string]any, bool) {
	switch list := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			obj, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, obj)
		}
		return out, true
	case []map[string]any:
		return list, true
	default:
		return nil, false
	}
}

func normalizeLegacyTokenChunkerNode(params map[string]any) (bool, error) {
	raw, exists := params["delimiter_mode"]
	if !exists || raw == nil {
		return false, nil
	}
	mode, ok := raw.(string)
	if !ok {
		return false, fmt.Errorf("delimiter_mode must be a string")
	}
	if mode != "token_size" {
		return false, nil
	}
	params["delimiter_mode"] = "delimiter"
	return true, nil
}

// normalizeCanvasDSL rewrites both copies of a canvas component's params:
// dsl.components[*].obj.params (what the runtime reads) and
// dsl.graph.nodes[*].data.form (what the operator form loads).
func normalizeCanvasDSL(dsl map[string]any, row ingestionConfigRow) (bool, error) {
	changed := false
	if inner, ok := dsl["dsl"].(map[string]any); ok {
		innerChanged, err := normalizeCanvasDSL(inner, row)
		if err != nil {
			return false, err
		}
		changed = changed || innerChanged
	}
	if components, ok := dsl["components"].(map[string]any); ok {
		for id, raw := range components {
			component, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			obj, ok := component["obj"].(map[string]any)
			if !ok {
				continue
			}
			componentName, _ := obj["component_name"].(string)
			params, ok := obj["params"].(map[string]any)
			if !ok {
				continue
			}
			nodeChanged, err := normalizeLegacyComponentParams(id, componentName, params, row)
			if err != nil {
				return false, fmt.Errorf("component %s: %w", id, err)
			}
			changed = changed || nodeChanged
		}
	}
	graph, _ := dsl["graph"].(map[string]any)
	nodes, _ := anyObjectSlice(graph["nodes"])
	for _, node := range nodes {
		id, _ := node["id"].(string)
		data, ok := node["data"].(map[string]any)
		if !ok {
			continue
		}
		form, ok := data["form"].(map[string]any)
		if !ok {
			continue
		}
		label, _ := data["label"].(string)
		nodeChanged, err := normalizeLegacyComponentParams(id, label, form, row)
		if err != nil {
			return false, fmt.Errorf("node %s: %w", id, err)
		}
		changed = changed || nodeChanged
	}
	return changed, nil
}
