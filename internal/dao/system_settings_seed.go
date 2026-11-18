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
	"os"
	"path/filepath"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// systemSettingSeed mirrors one entry of conf/system_settings.json.
type systemSettingSeed struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	DataType string `json:"data_type"`
	Value    string `json:"value"`
}

type systemSettingsSeedFile struct {
	SystemSettings []systemSettingSeed `json:"system_settings"`
}

// SeedSystemSettings seeds the system_settings table from
// conf/system_settings.json. Rows whose name already exists are left
// untouched so operator edits and enterprise settings survive restarts;
// only missing entries are inserted.
func SeedSystemSettings(ctx context.Context, db *gorm.DB) error {
	path := findSystemSettingsSeedFile()
	if path == "" {
		common.Warn("conf/system_settings.json not found, skipping system settings seeding")
		return nil
	}
	seeds, err := loadSystemSettingsSeedFile(path)
	if err != nil {
		common.Warn("Failed to load system settings seed file", zap.String("file", path), zap.Error(err))
		return nil
	}
	return insertMissingSystemSettings(ctx, db, seeds)
}

func findSystemSettingsSeedFile() string {
	candidates := []string{
		"conf/system_settings.json",
		filepath.Join("..", "conf", "system_settings.json"),
		filepath.Join("..", "..", "conf", "system_settings.json"),
		filepath.Join("..", "..", "..", "conf", "system_settings.json"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func loadSystemSettingsSeedFile(path string) ([]entity.SystemSettings, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var file systemSettingsSeedFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	settings := make([]entity.SystemSettings, 0, len(file.SystemSettings))
	for _, seed := range file.SystemSettings {
		if seed.Name == "" {
			continue
		}
		if seed.DataType == "" {
			seed.DataType = entity.InferSystemSettingDataType(seed.Name)
		}
		settings = append(settings, entity.SystemSettings{
			Name:     seed.Name,
			Source:   seed.Source,
			DataType: seed.DataType,
			Value:    seed.Value,
		})
	}
	return settings, nil
}

func insertMissingSystemSettings(ctx context.Context, db *gorm.DB, seeds []entity.SystemSettings) error {
	if len(seeds) == 0 {
		return nil
	}
	var existingNames []string
	if err := db.WithContext(ctx).Model(&entity.SystemSettings{}).Pluck("name", &existingNames).Error; err != nil {
		return fmt.Errorf("failed to list existing system settings: %w", err)
	}
	existing := make(map[string]struct{}, len(existingNames))
	for _, name := range existingNames {
		existing[name] = struct{}{}
	}
	toInsert := make([]entity.SystemSettings, 0, len(seeds))
	for _, seed := range seeds {
		if _, ok := existing[seed.Name]; ok {
			continue
		}
		toInsert = append(toInsert, seed)
	}
	if len(toInsert) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).Create(&toInsert).Error; err != nil {
		return fmt.Errorf("failed to insert system settings: %w", err)
	}
	common.Info("Seeded system settings", zap.Int("count", len(toInsert)))
	return nil
}
