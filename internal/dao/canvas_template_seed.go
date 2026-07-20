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
	"encoding/json"
	"fmt"
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
func SeedCanvasTemplates() error {
	if err := addColumnIfNotExists(DB, "canvas_template", "parser_ids", "LONGTEXT NULL"); err != nil {
		return fmt.Errorf("failed to ensure canvas_template.parser_ids column: %w", err)
	}

	var allTemplates []*entity.CanvasTemplate
	var allIDs []string
	for _, dir := range findTemplateDirs() {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			common.Warn("Failed to read template directory", zap.String("dir", dir), zap.Error(err))
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		templates, ids := loadTemplatesFromDir(dir, entries)
		allTemplates = append(allTemplates, templates...)
		allIDs = append(allIDs, ids...)
	}

	if len(allTemplates) == 0 {
		common.Warn("No template directories found, skipping canvas template seeding")
		return nil
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		for _, tmpl := range allTemplates {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"avatar", "title", "description", "canvas_type", "canvas_types", "canvas_category", "dsl",
				}),
			}).Create(tmpl).Error; err != nil {
				return fmt.Errorf("failed to save agent template %s: %w", tmpl.ID, err)
			}
		}
		if err := tx.Where("id NOT IN ?", allIDs).Delete(&entity.CanvasTemplate{}).Error; err != nil {
			return fmt.Errorf("failed to remove stale agent templates: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	common.Info("Seeded canvas templates", zap.Int("count", len(allTemplates)))
	return nil
}

func loadTemplatesFromDir(dir string, entries []os.DirEntry) ([]*entity.CanvasTemplate, []string) {
	templates := make([]*entity.CanvasTemplate, 0, len(entries))
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			common.Warn("Failed to read agent template", zap.String("file", path), zap.Error(err))
			continue
		}
		tmpl, err := parseCanvasTemplateFile(raw)
		if err != nil {
			common.Warn("Failed to parse agent template", zap.String("file", path), zap.Error(err))
			continue
		}
		templates = append(templates, tmpl)
		ids = append(ids, tmpl.ID)
	}
	return templates, ids
}

func findTemplateDirs() []string {
	var dirs []string
	if d := findAgentTemplatesDir(); d != "" {
		dirs = append(dirs, d)
	}
	if d := findIngestionTemplatesDir(); d != "" {
		dirs = append(dirs, d)
	}
	return dirs
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

func seedCanvasTemplates(db *gorm.DB, dir string, entries []os.DirEntry) (int, error) {
	templates := make([]*entity.CanvasTemplate, 0, len(entries))
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			common.Warn("Failed to read agent template", zap.String("file", path), zap.Error(err))
			continue
		}
		tmpl, err := parseCanvasTemplateFile(raw)
		if err != nil {
			common.Warn("Failed to parse agent template", zap.String("file", path), zap.Error(err))
			continue
		}
		templates = append(templates, tmpl)
		ids = append(ids, tmpl.ID)
	}
	if len(templates) == 0 {
		return 0, nil
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, tmpl := range templates {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"avatar", "title", "description", "canvas_type", "canvas_types", "canvas_category", "dsl",
				}),
			}).Create(tmpl).Error; err != nil {
				return fmt.Errorf("failed to save agent template %s: %w", tmpl.ID, err)
			}
		}
		if err := tx.Where("id NOT IN ?", ids).Delete(&entity.CanvasTemplate{}).Error; err != nil {
			return fmt.Errorf("failed to remove stale agent templates: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(templates), nil
}

func parseCanvasTemplateFile(raw []byte) (*entity.CanvasTemplate, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var data map[string]any
	if err := decoder.Decode(&data); err != nil {
		return nil, err
	}

	tmpl := &entity.CanvasTemplate{
		CanvasCategory: "agent_canvas",
	}

	if v, ok := data["id"]; ok {
		tmpl.ID = fmt.Sprint(v)
	}
	if v, ok := data["title"].(map[string]any); ok {
		tmpl.Title = entity.JSONMap(v)
	}
	if v, ok := data["description"].(map[string]any); ok {
		tmpl.Description = entity.JSONMap(v)
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
		tmpl.DSL = entity.JSONMap(v)
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
