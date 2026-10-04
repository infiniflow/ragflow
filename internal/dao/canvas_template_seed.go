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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SeedCanvasTemplates seeds the canvas_template table from the built-in
// agent/templates/*.json and internal/ingestion/pipeline/template/*.json files.
func SeedCanvasTemplates(ctx context.Context, db *gorm.DB) error {
	if err := addColumnIfNotExists(ctx, db, "canvas_template", "parser_ids", "LONGTEXT NULL"); err != nil {
		return fmt.Errorf("failed to ensure canvas_template.parser_ids column: %w", err)
	}

	var allTemplates []*entity.CanvasTemplate
	var allIDs []string
	complete := true
	for _, dir := range findTemplateDirs() {
		if dir == "" {
			complete = false
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			complete = false
			common.Warn("Failed to read template directory", zap.String("dir", dir), zap.Error(err))
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		templates, ids, err := loadTemplatesFromDir(dir, entries)
		if err != nil || len(templates) == 0 {
			complete = false
			common.Warn("Incomplete canvas template resources", zap.String("dir", dir), zap.Int("loaded", len(templates)), zap.Error(err))
		}
		allTemplates = append(allTemplates, templates...)
		allIDs = append(allIDs, ids...)
	}

	if len(allTemplates) == 0 {
		common.Warn("No template directories found, skipping canvas template seeding")
		return nil
	}

	count, err := seedCanvasTemplates(ctx, db, allTemplates, allIDs, complete)
	if err != nil {
		return err
	}
	common.Info("Seeded canvas templates", zap.Int("count", count))
	return nil
}

// loadTemplatesFromDir returns valid catalog entries and reports any incomplete
// resources, allowing callers to update healthy entries without pruning.
func loadTemplatesFromDir(dir string, entries []os.DirEntry) ([]*entity.CanvasTemplate, []string, error) {
	templates := make([]*entity.CanvasTemplate, 0, len(entries))
	ids := make([]string, 0, len(entries))
	var loadErrors []error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("read canvas template %s: %w", path, err))
			continue
		}
		tmpl, err := parseCanvasTemplateFile(raw)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("parse canvas template %s: %w", path, err))
			continue
		}
		if tmpl == nil {
			continue // standalone canvas DSL, not a catalog template
		}
		templates = append(templates, tmpl)
		ids = append(ids, tmpl.ID)
	}
	return templates, ids, errors.Join(loadErrors...)
}

// findTemplateDirs retains missing sources so incomplete deployments cannot
// authorize removal of catalog rows from an unavailable source.
func findTemplateDirs() []string {
	return []string{findAgentTemplatesDir(), findIngestionTemplatesDir()}
}

func findIngestionTemplatesDir() string {
	candidates := []string{
		"internal/ingestion/pipeline/template",
		filepath.Join("..", "internal", "ingestion", "pipeline", "template"),
		filepath.Join("..", "..", "internal", "ingestion", "pipeline", "template"),
		filepath.Join("..", "..", "..", "internal", "ingestion", "pipeline", "template"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

// seedCanvasTemplates only prunes stale rows after every built-in source loaded.
func seedCanvasTemplates(ctx context.Context, db *gorm.DB, templates []*entity.CanvasTemplate, ids []string, prune bool) (int, error) {
	if len(templates) == 0 {
		return 0, nil
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, tmpl := range templates {
			if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"avatar", "title", "description", "canvas_type", "canvas_types", "canvas_category", "dsl",
				}),
			}).Create(tmpl).Error; err != nil {
				return fmt.Errorf("failed to save agent template %s: %w", tmpl.ID, err)
			}
		}
		if prune {
			if err := tx.Where("id NOT IN ?", ids).Delete(&entity.CanvasTemplate{}).Error; err != nil {
				return fmt.Errorf("failed to remove stale canvas templates: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(templates), nil
}

// parseCanvasTemplateFile validates a catalog identity. Standalone canvas DSLs
// return nil without error because they are component fixtures, not catalog rows.
func parseCanvasTemplateFile(raw []byte) (*entity.CanvasTemplate, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("canvas template must contain exactly one JSON object")
	}

	tmpl := &entity.CanvasTemplate{
		CanvasCategory: "agent_canvas",
	}

	id, present := data["id"]
	if !present {
		// Component fixtures such as compiler.json share this directory, but a
		// standalone DSL has no catalog identity or nested dsl envelope.
		_, wrapped := data["dsl"]
		if _, standalone := data["components"].(map[string]any); standalone && !wrapped {
			return nil, nil
		}
		return nil, fmt.Errorf("canvas template is missing id")
	}
	switch value := id.(type) {
	case string:
		tmpl.ID = value
	case json.Number:
		tmpl.ID = value.String()
	default:
		return nil, fmt.Errorf("canvas template id must be a string or number")
	}
	if strings.TrimSpace(tmpl.ID) == "" {
		return nil, fmt.Errorf("canvas template id must not be empty")
	}
	if v, ok := data["title"].(map[string]any); ok {
		tmpl.Title = v
	}
	if v, ok := data["description"].(map[string]any); ok {
		tmpl.Description = v
	}
	if v, ok := data["avatar"].(string); ok && v != "" {
		tmpl.Avatar = &v
	}
	if v, ok := data["canvas_category"].(string); ok && v != "" {
		tmpl.CanvasCategory = v
	}

	canvasTypes := collectCanvasTypes(data["canvas_type"], data["canvas_types"])
	if len(canvasTypes) > 0 {
		tmpl.CanvasTypes = canvasTypes
		if first, ok := canvasTypes[0].(string); ok {
			tmpl.CanvasType = &first
		}
	}

	if v, ok := data["dsl"].(map[string]any); ok {
		tmpl.DSL = v
	}

	return tmpl, nil
}

func collectCanvasTypes(rawType, rawTypes any) entity.JSONSlice {
	seen := make(map[string]struct{})
	var result entity.JSONSlice

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		result = append(result, s)
	}

	if s, ok := rawType.(string); ok {
		add(s)
	}

	switch v := rawTypes.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				add(s)
			}
		}
	case []string:
		for _, s := range v {
			add(s)
		}
	case string:
		add(v)
	}

	return result
}

func findAgentTemplatesDir() string {
	candidates := []string{
		"internal/agent/templates",
		"agent/templates",
		filepath.Join("..", "agent", "templates"),
		filepath.Join("..", "..", "agent", "templates"),
		filepath.Join("..", "..", "..", "agent", "templates"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}
