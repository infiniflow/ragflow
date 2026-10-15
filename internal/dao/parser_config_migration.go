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

package dao

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	parserchunk "ragflow/internal/parser/chunk"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	legacyGeneralChunkerID  = "TokenChunker:SixApplesFall"
	currentGeneralChunkerID = "GeneralChunker:SixApplesFall"
)

var generalChunkerParamKeys = map[string]struct{}{
	"children_delimiters": {},
	"chunk_token_size":    {},
	"delimiter":           {},
	"delimiters":          {},
	"image_context_size":  {},
	"overlapped_percent":  {},
	"table_context_size":  {},
}

// migrateGeneralChunkerConfig rewrites the exact legacy component key used by
// the built-in general pipeline. Explicit values already stored under the new
// key win; unsupported TokenChunker-only fields are dropped because
// GeneralChunker has no delimiter_mode or outputs parameter.
func migrateGeneralChunkerConfig(config map[string]interface{}) bool {
	legacy, ok := config[legacyGeneralChunkerID]
	if !ok {
		return false
	}
	legacyParams, ok := legacy.(map[string]interface{})
	if !ok {
		return false
	}

	if _, exists := config[currentGeneralChunkerID]; !exists {
		params := make(map[string]interface{}, len(legacyParams))
		for key, value := range legacyParams {
			if key == "delimiter" {
				if _, exists := legacyParams["delimiters"]; !exists {
					if delimiters, ok := normalizeLegacyDelimiter(value); ok {
						params["delimiters"] = delimiters
					}
				}
				continue
			}
			if _, supported := generalChunkerParamKeys[key]; supported {
				params[key] = value
			}
		}
		config[currentGeneralChunkerID] = params
	}
	delete(config, legacyGeneralChunkerID)
	return true
}

func migrateGeneralChunkerParserConfigs(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return nil
	}

	var migratedKnowledgebases int
	if db.WithContext(ctx).Migrator().HasTable("knowledgebase") {
		count, err := migrateParserConfigRows(ctx, db, &entity.Knowledgebase{})
		if err != nil {
			return fmt.Errorf("migrate knowledgebase parser configs: %w", err)
		}
		migratedKnowledgebases = count
	}

	var migratedDocuments int
	if db.WithContext(ctx).Migrator().HasTable("document") {
		count, err := migrateParserConfigRows(ctx, db, &entity.Document{})
		if err != nil {
			return fmt.Errorf("migrate document parser configs: %w", err)
		}
		migratedDocuments = count
	}

	if migratedKnowledgebases > 0 || migratedDocuments > 0 {
		common.Info("Migrated general chunker parser configs",
			zap.Int("knowledgebases", migratedKnowledgebases),
			zap.Int("documents", migratedDocuments))
	}
	return nil
}

const parserConfigMigrationBatchSize = 256

type parserConfigMigrationRow struct {
	ID           string         `gorm:"column:id"`
	ParserConfig entity.JSONMap `gorm:"column:parser_config"`
}

func migrateParserConfigRows(ctx context.Context, db *gorm.DB, model any) (int, error) {
	var migrated int
	query := db.WithContext(ctx).
		Model(model).
		Select("id", "parser_config").
		Where("parser_id IN ? AND (pipeline_id IS NULL OR pipeline_id = '')", []string{"general", "naive"}).
		Order("id")
	var rows []parserConfigMigrationRow
	result := query.FindInBatches(&rows, parserConfigMigrationBatchSize, func(batchDB *gorm.DB, _ int) error {
		return batchDB.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				config := map[string]interface{}(row.ParserConfig)
				if !migrateGeneralChunkerConfig(config) {
					continue
				}
				if err := tx.Model(model).Where("id = ?", row.ID).
					Update("parser_config", entity.JSONMap(config)).Error; err != nil {
					return fmt.Errorf("update parser config %q: %w", row.ID, err)
				}
				migrated++
			}
			return nil
		})
	})
	if result.Error != nil {
		return migrated, result.Error
	}
	return migrated, nil
}

func normalizeLegacyDelimiter(value any) (any, bool) {
	switch value := value.(type) {
	case string:
		delimiters := parserchunk.ParseDelimiterField(value)
		out := make([]interface{}, len(delimiters))
		for i, delimiter := range delimiters {
			out[i] = delimiter
		}
		return out, true
	case []string:
		out := make([]interface{}, len(value))
		for i, delimiter := range value {
			out[i] = delimiter
		}
		return out, true
	case []interface{}:
		return value, true
	default:
		return nil, false
	}
}

// extractorParserConfigTargetVersion is the database version a completed
// extractor parser config migration brings the database to. It is stored in
// system_settings under mysql_migration.database.version, the marker shared
// with the other versioned steps, so work already performed by a previous run
// is skipped.
//
// The version must sort strictly above knowledgebaseParserConfigTargetVersion
// (v1.0.0-rc2.dev1): both steps share the one marker, and this step runs right
// after it. Marking an equal-or-lower version here would make the earlier step
// believe its work is already done on a database that never ran it.
const extractorParserConfigTargetVersion = "v1.0.0-rc2.dev2"

// builtinExtractorNodeID is the Extractor component id every Go builtin
// pipeline template carries (except qa and table, which have no Extractor).
// Migration-created rows use the same id so the row matches the pipeline the
// document actually runs with.
const builtinExtractorNodeID = "Extractor:AutoExtractDefault"

// builtinExtractorPipelines lists the builtin parser_ids whose template DSL
// carries an Extractor node. "naive" is the Python-era default parser id; it
// resolves to the "general" template at runtime, so rows storing "naive" get
// the same node as "general".
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

// flatExtractorConfigKeys are the top-level parser_config keys that
// describe extraction behavior. On the Go backend they live as modular groups
// on the Extractor node, so the migration folds them there and deletes them.
var flatExtractorConfigKeys = []string{
	"auto_keywords",
	"auto_questions",
	"topn_tags",
	"enable_metadata",
	"metadata",
	"built_in_metadata",
}

// migrateExtractorParserConfigs folds the Python 0.27.2-era extraction
// settings (top-level auto_keywords/auto_questions/topn_tags/tag_kb_ids and
// the flat metadata trio) into the Go modular Extractor groups
// (Extractor:<name>.{keywords,questions,tags,metadata}) on knowledgebase and
// document parser configs, then deletes the legacy keys.
//
// Background: the Python task executor read those flat keys directly
// (auto_keywords -> important_kwd, auto_questions -> question_kwd,
// enable_metadata+metadata/built_in_metadata -> metadata extraction,
// tag_kb_ids -> tag datasets). The Go backend only reads the component-scoped
// groups (schema.ExtractorParam; NewExtractorComponent), and the write/read
// paths drop flat keys silently, so rows upgraded from Python keep parsing
// while every extraction silently falls back to "disabled".
func migrateExtractorParserConfigs(ctx context.Context, db *gorm.DB) error {
	currentVersion, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(currentVersion, extractorParserConfigTargetVersion) {
		common.Info("Extractor parser configs already migrated, skipping",
			zap.String("current_version", currentVersion),
			zap.String("target_version", extractorParserConfigTargetVersion))
		return nil
	}
	hasKnowledgebase := db.WithContext(ctx).Migrator().HasTable("knowledgebase")
	hasDocument := db.WithContext(ctx).Migrator().HasTable("document")
	if !hasKnowledgebase && !hasDocument {
		return nil
	}

	common.Info("Running extractor parser config migration",
		zap.String("current_version", currentVersion),
		zap.String("target_version", extractorParserConfigTargetVersion))
	var migratedKnowledgebases, migratedDocuments int
	if hasKnowledgebase {
		count, err := migrateExtractorConfigRows(ctx, db, &entity.Knowledgebase{})
		if err != nil {
			return fmt.Errorf("migrate knowledgebase extractor configs: %w", err)
		}
		migratedKnowledgebases = count
	}
	if hasDocument {
		count, err := migrateExtractorConfigRows(ctx, db, &entity.Document{})
		if err != nil {
			return fmt.Errorf("migrate document extractor configs: %w", err)
		}
		migratedDocuments = count
	}
	if err := setDatabaseMigrationVersion(ctx, db, extractorParserConfigTargetVersion); err != nil {
		return fmt.Errorf("mark database migration version %s: %w", extractorParserConfigTargetVersion, err)
	}
	common.Info("Extractor parser config migration completed",
		zap.Int("knowledgebases", migratedKnowledgebases),
		zap.Int("documents", migratedDocuments),
		zap.String("version", extractorParserConfigTargetVersion))
	return nil
}

// extractorConfigMigrationRow reads one row's parser selection plus its raw
// parser_config, so a malformed payload is skipped instead of failing the
// whole migration. The parser_id decides whether a builtin template's
// Extractor node id can be materialized; pipeline_id marks canvas pipelines,
// for which no well-known node id exists.
type extractorConfigMigrationRow struct {
	ID           string  `gorm:"column:id"`
	ParserID     string  `gorm:"column:parser_id"`
	PipelineID   *string `gorm:"column:pipeline_id"`
	ParserConfig *string `gorm:"column:parser_config"`
}

func migrateExtractorConfigRows(ctx context.Context, db *gorm.DB, model any) (int, error) {
	var migrated int
	var rows []extractorConfigMigrationRow
	result := db.WithContext(ctx).Model(model).
		Select("id", "parser_id", "pipeline_id", "parser_config").Order("id").
		FindInBatches(&rows, parserConfigMigrationBatchSize, func(batchDB *gorm.DB, _ int) error {
			return batchDB.Transaction(func(tx *gorm.DB) error {
				for _, row := range rows {
					config, ok := parseParserConfig(row.ParserConfig)
					if !ok {
						if row.ParserConfig != nil && strings.TrimSpace(*row.ParserConfig) != "" {
							common.Warn("Skipping invalid extractor parser config",
								zap.String("table", tx.Statement.Table), zap.String("id", row.ID))
						}
						continue
					}
					if ids, ok := config["tag_kb_ids"].([]interface{}); ok && len(ids) > 0 {
						common.Warn("Tag dataset references require conversion to a tag source file; retaining references",
							zap.String("table", tx.Statement.Table), zap.String("id", row.ID), zap.Int("tag_datasets", len(ids)))
					}
					changed := migrateExtractorConfig(config, row.ParserID, row.PipelineID)
					if len(extractorNodeParams(config)) == 0 && collectFlatExtractorValues(config).hasValues() {
						common.Warn("Retaining extraction settings without an Extractor target",
							zap.String("table", tx.Statement.Table), zap.String("id", row.ID), zap.String("parser_id", row.ParserID))
					}
					if !changed {
						continue
					}
					if err := tx.Model(model).Where("id = ?", row.ID).
						Update("parser_config", entity.JSONMap(config)).Error; err != nil {
						return fmt.Errorf("update extractor parser config %q: %w", row.ID, err)
					}
					migrated++
				}
				return nil
			})
		})
	if result.Error != nil {
		return migrated, fmt.Errorf("read extractor parser configs: %w", result.Error)
	}
	return migrated, nil
}

// migrateExtractorConfig folds legacy extraction settings into the modular
// Extractor groups in place and reports whether the config changed.
//
// Source precedence: values already stored in the modular groups win over
// legacy keys (the migration is idempotent and never clobbers a setting the
// Go backend already wrote). The "enabled" switches the groups require are
// derived the same way the runtime does:
//   - keywords/questions/tags carry no "enabled" in old rows, so enabled is
//     derived from top_n > 0, matching NewExtractorComponent's
//     resolveExtractToggle fallback for legacy DSLs.
//   - metadata.enabled falls back to "any field declared", matching the
//     dataset metadata API's default in metadata.go.
//
// Tag dataset references are retained because a database JSON rewrite cannot
// convert their indexed content into a tag source file. Configs without an
// Extractor target retain extraction settings that cannot be applied.
func migrateExtractorConfig(config map[string]interface{}, parserID string, pipelineID *string) bool {
	if config == nil {
		return false
	}
	changed := false
	flat := collectFlatExtractorValues(config)
	flatTags, hasFlatTags := config["tags"].(map[string]interface{})
	nodes := extractorNodeParams(config)
	targets := nodes
	if len(targets) == 0 && (flat.hasValues() || len(flatTags) > 0) && builtinExtractorNodeIDFor(parserID, pipelineID) != "" {
		nodeID := builtinExtractorNodeIDFor(parserID, pipelineID)
		params := map[string]interface{}{}
		config[nodeID] = params
		targets = []map[string]interface{}{params}
		changed = true
	}

	// Pull the legacy per-node fields (early-Go extractor form) up into the
	// groups first, then the top-level flat keys (Python 0.27.2 form).
	for _, params := range nodes {
		if foldLegacyExtractorNodeFields(params) {
			changed = true
		}
	}

	if hasFlatTags && len(targets) > 0 {
		for _, params := range targets {
			if len(flatTags) > 0 && ensureExtractorGroup(params, "tags", cloneFlatMap(flatTags)) {
				changed = true
			}
		}
		delete(config, "tags")
		changed = true
	}
	if len(targets) > 0 && flat.hasValues() {
		for _, params := range targets {
			if applyFlatExtractorValues(params, flat) {
				changed = true
			}
		}
	}
	if len(targets) > 0 {
		for _, key := range flatExtractorConfigKeys {
			if _, ok := config[key]; ok {
				delete(config, key)
				changed = true
			}
		}
	} else if flat.tagsTopN != nil {
		// Retrieval can use a dataset-level tag source without an Extractor.
		if !hasFlatTags {
			flatTags = map[string]interface{}{}
			config["tags"] = flatTags
		}
		if _, exists := flatTags["top_n"]; !exists {
			flatTags["top_n"] = *flat.tagsTopN
		}
		delete(config, "topn_tags")
		changed = true
	}
	return changed
}

// builtinExtractorNodeIDFor returns the Extractor node id to materialize for
// a builtin pipeline selection, or "" when no well-known id exists (canvas
// pipelines, or builtin templates without an Extractor such as qa/table).
func builtinExtractorNodeIDFor(parserID string, pipelineID *string) string {
	if pipelineID != nil && strings.TrimSpace(*pipelineID) != "" {
		return ""
	}
	if _, ok := builtinExtractorPipelines[strings.ToLower(strings.TrimSpace(parserID))]; ok {
		return builtinExtractorNodeID
	}
	return ""
}

// extractorNodeParams returns the params maps of every Extractor node in a
// parser config, in sorted key order so multi-node rows migrate
// deterministically.
func extractorNodeParams(config map[string]interface{}) []map[string]interface{} {
	var keys []string
	for key := range config {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "extractor:") || strings.HasPrefix(lower, "extractor_") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var out []map[string]interface{}
	for _, key := range keys {
		if params, ok := config[key].(map[string]interface{}); ok {
			out = append(out, params)
		}
	}
	return out
}

// flatExtractorValues carries the Python 0.27.2 top-level extraction
// settings collected off a parser config row.
type flatExtractorValues struct {
	keywords  *int
	questions *int
	tagsTopN  *int
	enabled   any
	metadata  any
	builtIn   any
}

func (v flatExtractorValues) hasValues() bool {
	return v.keywords != nil || v.questions != nil || v.tagsTopN != nil ||
		v.enabled != nil || v.metadata != nil || v.builtIn != nil
}

func collectFlatExtractorValues(config map[string]interface{}) flatExtractorValues {
	var out flatExtractorValues
	if topN, ok := extractorTopN(config["auto_keywords"]); ok {
		out.keywords = &topN
	}
	if topN, ok := extractorTopN(config["auto_questions"]); ok {
		out.questions = &topN
	}
	if topN, ok := extractorTopN(config["topn_tags"]); ok {
		out.tagsTopN = &topN
	}
	if enabled, ok := config["enable_metadata"]; ok && enabled != nil {
		out.enabled = enabled
	}
	if metadata, ok := config["metadata"]; ok && metadata != nil {
		out.metadata = metadata
	}
	if builtIn, ok := config["built_in_metadata"]; ok && builtIn != nil {
		out.builtIn = builtIn
	}
	return out
}

// applyFlatExtractorValues writes collected flat values into one Extractor
// node's groups. Keys already present in the group win; only missing keys are
// filled. It reports whether the node changed.
func applyFlatExtractorValues(params map[string]interface{}, flat flatExtractorValues) bool {
	changed := false
	if flat.keywords != nil && ensureExtractorGroup(params, "keywords", map[string]interface{}{"top_n": *flat.keywords}) {
		changed = true
	}
	if flat.questions != nil && ensureExtractorGroup(params, "questions", map[string]interface{}{"top_n": *flat.questions}) {
		changed = true
	}
	if flat.tagsTopN != nil && ensureExtractorGroup(params, "tags", map[string]interface{}{"top_n": *flat.tagsTopN}) {
		changed = true
	}
	if metaGroup, ok := buildMigratedMetadataGroup(params["metadata"], flat.enabled, flat.metadata, flat.builtIn); ok {
		params["metadata"] = metaGroup
		changed = true
	}
	return changed
}

// ensureExtractorGroup creates the named group with the given defaults when
// the node carries no usable group yet. An existing group keeps every key it
// already declares; only missing keys are filled from defaults. It reports
// whether the node changed.
func ensureExtractorGroup(params map[string]interface{}, name string, defaults map[string]interface{}) bool {
	group, ok := params[name].(map[string]interface{})
	if !ok {
		params[name] = defaults
		return true
	}
	changed := false
	for key, value := range defaults {
		if _, exists := group[key]; !exists {
			group[key] = value
			changed = true
		}
	}
	return changed
}

// buildMigratedMetadataGroup assembles the modular metadata group from the
// Python flat trio (enable_metadata + metadata/built_in_metadata field
// lists) or a modular transport object. Existing group fields win; missing
// fields are filled from the source. It reports whether the group changed.
func buildMigratedMetadataGroup(existing, enabled, metadata, builtIn any) (map[string]interface{}, bool) {
	if enabled == nil && metadata == nil && builtIn == nil {
		return nil, false
	}
	var defaults map[string]interface{}
	if source, ok := metadata.(map[string]interface{}); ok && isModularMetadataGroup(source) {
		defaults = cloneFlatMap(source)
		if _, exists := defaults["metadata"]; !exists {
			defaults["metadata"] = []interface{}{}
		}
		if _, exists := defaults["built_in_metadata"]; !exists {
			defaults["built_in_metadata"] = legacyMetadataFieldList(builtIn)
		}
	} else {
		defaults = map[string]interface{}{
			"metadata":          legacyMetadataFieldList(metadata),
			"built_in_metadata": legacyMetadataFieldList(builtIn),
		}
	}
	group, hasGroup := existing.(map[string]interface{})
	if _, exists := defaults["enabled"]; !exists {
		fields, builtInFields := defaults["metadata"], defaults["built_in_metadata"]
		if hasGroup {
			if value, exists := group["metadata"]; exists {
				fields = value
			}
			if value, exists := group["built_in_metadata"]; exists {
				builtInFields = value
			}
		}
		defaults["enabled"] = migrateMetadataEnabled(enabled,
			len(legacyMetadataFieldList(fields))+len(legacyMetadataFieldList(builtInFields)) > 0)
	}
	if !hasGroup {
		return defaults, true
	}
	merged := cloneFlatMap(group)
	changed := false
	for key, value := range defaults {
		if _, exists := merged[key]; !exists {
			merged[key] = value
			changed = true
		}
	}
	return merged, changed
}

// migrateMetadataEnabled resolves the metadata "enabled" switch the same way
// the dataset metadata API defaults it: an explicit flag wins, otherwise any
// declared field implies enabled.
func migrateMetadataEnabled(enabled any, hasFields bool) bool {
	if enabled != nil {
		switch value := enabled.(type) {
		case bool:
			return value
		case float64:
			return value > 0
		case int:
			return value > 0
		case int64:
			return value > 0
		case string:
			lower := strings.ToLower(strings.TrimSpace(value))
			return lower == "true" || lower == "1"
		}
	}
	return hasFields
}

// legacyMetadataFieldList normalizes a Python-era metadata field list into
// the Go {key,type,description,enum} shape:
//   - []any of {name,...} items -> {key,...} (name renamed to key;
//     examples/restrict_values dropped: turn2jsonschema never reads them).
//   - a JSON-Schema object ({type:object, properties:{...}}, the shape
//     Python's turn2jsonschema emits) -> one field per property.
//
// Anything else yields an empty list.
func legacyMetadataFieldList(value any) []interface{} {
	normalizeField := func(raw map[string]interface{}) (map[string]interface{}, bool) {
		key, _ := raw["key"].(string)
		if strings.TrimSpace(key) == "" {
			key, _ = raw["name"].(string)
		}
		if strings.TrimSpace(key) == "" {
			return nil, false
		}
		field := map[string]interface{}{"key": strings.TrimSpace(key)}
		if fieldType, ok := raw["type"].(string); ok && fieldType != "" {
			field["type"] = fieldType
		}
		for _, descKey := range []string{"description", "descriptions"} {
			if desc, ok := raw[descKey].(string); ok && desc != "" {
				field["description"] = desc
				break
			}
		}
		if enum, ok := raw["enum"].([]interface{}); ok && len(enum) > 0 {
			field["enum"] = enum
		}
		return field, true
	}
	switch typed := value.(type) {
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if field, ok := normalizeField(raw); ok {
				out = append(out, field)
			}
		}
		return out
	case map[string]interface{}:
		properties, ok := typed["properties"].(map[string]interface{})
		if !ok {
			return []interface{}{}
		}
		var names []string
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		out := make([]interface{}, 0, len(names))
		for _, name := range names {
			prop, ok := properties[name].(map[string]interface{})
			if !ok {
				continue
			}
			field := map[string]interface{}{"key": name}
			if desc, ok := prop["description"].(string); ok && desc != "" {
				field["description"] = desc
			}
			if enum, ok := prop["enum"].([]interface{}); ok && len(enum) > 0 {
				field["enum"] = enum
			}
			if fieldType, ok := prop["type"].(string); ok && fieldType != "" {
				field["type"] = fieldType
			}
			out = append(out, field)
		}
		return out
	default:
		return []interface{}{}
	}
}

// foldLegacyExtractorNodeFields folds the pre-modular per-node fields (the
// early-Go extractor form NewExtractorComponent used to read: auto_keywords,
// keywords_sys_prompt, auto_questions, questions_sys_prompt, auto_tags,
// tag_file_id, enable_summary, sys_prompt, metadata_config, enable_metadata,
// flat metadata/built_in_metadata arrays, field_name) into the modular
// groups. Keys already present in a group win. It reports whether the node
// changed.
func foldLegacyExtractorNodeFields(params map[string]interface{}) bool {
	changed := false
	moveInt := func(groupName, legacyKey string) {
		if topN, ok := extractorTopN(params[legacyKey]); ok {
			if ensureExtractorGroup(params, groupName, map[string]interface{}{"top_n": topN}) {
				changed = true
			}
			delete(params, legacyKey)
			changed = true
		}
	}
	moveInt("keywords", "auto_keywords")
	moveInt("questions", "auto_questions")
	// auto_tags has no keywords-style prompt sibling; tag_file_id moves below.
	if topN, ok := extractorTopN(params["auto_tags"]); ok {
		if ensureExtractorGroup(params, "tags", map[string]interface{}{"top_n": topN}) {
			changed = true
		}
		delete(params, "auto_tags")
		changed = true
	}
	movePrompt := func(groupName, legacyKey string) {
		prompt, ok := params[legacyKey].(string)
		if !ok || strings.TrimSpace(prompt) == "" {
			return
		}
		if ensureExtractorGroup(params, groupName, map[string]interface{}{}) {
			changed = true
		}
		if group, ok := params[groupName].(map[string]interface{}); ok {
			if _, exists := group["system_prompt"]; !exists {
				group["system_prompt"] = prompt
				changed = true
			}
		}
		delete(params, legacyKey)
		changed = true
	}
	movePrompt("keywords", "keywords_sys_prompt")
	movePrompt("questions", "questions_sys_prompt")
	// enable_summary is the flat form of summary.enabled; a field_name of
	// "summary" was the even older equivalent. The node-level system_prompt /
	// sys_prompt fed Summary.SystemPrompt unconditionally, so it is preserved
	// even when the flag says disabled.
	summaryEnabled, hasSummaryFlag := params["enable_summary"]
	if !hasSummaryFlag {
		if fieldName, ok := params["field_name"].(string); ok && fieldName == "summary" {
			summaryEnabled, hasSummaryFlag = true, true
		}
	}
	sysPrompt, _ := params["system_prompt"].(string)
	if strings.TrimSpace(sysPrompt) == "" {
		sysPrompt, _ = params["sys_prompt"].(string)
	}
	if hasSummaryFlag && ensureExtractorGroup(params, "summary", map[string]interface{}{"enabled": migrateMetadataEnabled(summaryEnabled, false)}) {
		changed = true
	}
	if strings.TrimSpace(sysPrompt) != "" {
		if group, ok := params["summary"].(map[string]interface{}); ok {
			if _, exists := group["system_prompt"]; !exists {
				group["system_prompt"] = sysPrompt
				changed = true
			}
		} else {
			params["summary"] = map[string]interface{}{
				"enabled":       migrateMetadataEnabled(summaryEnabled, false),
				"system_prompt": sysPrompt,
			}
			changed = true
		}
		delete(params, "system_prompt")
		delete(params, "sys_prompt")
		changed = true
	}
	if fileID, ok := params["tag_file_id"].(string); ok && strings.TrimSpace(fileID) != "" {
		if ensureExtractorGroup(params, "tags", map[string]interface{}{}) {
			changed = true
		}
		if group, ok := params["tags"].(map[string]interface{}); ok {
			if _, exists := group["tag_file_id"]; !exists {
				group["tag_file_id"] = fileID
				changed = true
			}
		}
		delete(params, "tag_file_id")
		changed = true
	}
	for _, key := range []string{"enable_summary", "field_name", "prompt", "prompts", "system_prompt", "sys_prompt"} {
		if _, ok := params[key]; ok {
			delete(params, key)
			changed = true
		}
	}
	// metadata_config is the transitional group name; rename it to metadata.
	if metaConf, ok := params["metadata_config"].(map[string]interface{}); ok {
		if group, changedGroup := buildMigratedMetadataGroup(params["metadata"], params["enable_metadata"], metaConf, params["built_in_metadata"]); changedGroup {
			params["metadata"] = group
			changed = true
		}
		delete(params, "metadata_config")
		changed = true
	}
	// Flat node-level enable_metadata/metadata/built_in_metadata predate the
	// group object; fold both field lists and JSON Schema objects.
	meta, isMap := params["metadata"].(map[string]interface{})
	var existing any
	fields := params["metadata"]
	if isMap && isModularMetadataGroup(meta) {
		existing, fields = meta, nil
	}
	if group, changedGroup := buildMigratedMetadataGroup(existing, params["enable_metadata"], fields, params["built_in_metadata"]); changedGroup {
		params["metadata"] = group
		changed = true
	}
	for _, key := range []string{"enable_metadata", "built_in_metadata"} {
		if _, ok := params[key]; ok {
			delete(params, key)
			changed = true
		}
	}
	return changed
}

// extractorTopN reads a top_n-style value in whatever numeric shape JSON
// decoding produced. It reports false for missing, null, non-numeric, or
// negative values.
func extractorTopN(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if typed < 0 || typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case int:
		if typed < 0 {
			return 0, false
		}
		return typed, true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return int(typed), true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}

// isModularMetadataGroup reports whether a top-level "metadata" value is the
// Go dataset-level transport object ({enabled, metadata, built_in_metadata})
// rather than a Python field list or JSON-Schema dict.
func isModularMetadataGroup(value map[string]interface{}) bool {
	if len(value) == 0 {
		return true
	}
	for _, key := range []string{"enabled", "metadata", "built_in_metadata"} {
		if _, ok := value[key]; ok {
			return true
		}
	}
	return false
}

// cloneFlatMap shallow-copies a map so the migrated node owns its group
// instead of aliasing the deleted top-level value.
func cloneFlatMap(source map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
