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
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSeedSystemSettingsInsertsMissingAndKeepsExisting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.SystemSettings{}); err != nil {
		t.Fatalf("migrate system settings: %v", err)
	}

	body := `{"system_settings":[
		{"name":"enable_whitelist","source":"variable","data_type":"bool","value":"true"},
		{"name":"mail.timeout","source":"variable","data_type":"integer","value":"10"},
		{"name":"sandbox.provider_type","source":"variable","data_type":"string","value":"self_managed"},
		{"name":"","source":"variable","data_type":"string","value":""}
	]}`
	path := filepath.Join(t.TempDir(), "system_settings.json")
	if err = os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	seeds, err := loadSystemSettingsSeedFile(path)
	if err != nil {
		t.Fatalf("load seed file: %v", err)
	}
	if len(seeds) != 3 {
		t.Fatalf("got %d seeds after dropping empty-name entries, want 3", len(seeds))
	}

	ctx := t.Context()
	if err = insertMissingSystemSettings(ctx, db, seeds); err != nil {
		t.Fatalf("seed system settings: %v", err)
	}
	// Idempotent: a second run must not duplicate rows.
	if err = insertMissingSystemSettings(ctx, db, seeds); err != nil {
		t.Fatalf("re-seed system settings: %v", err)
	}

	var count int64
	if err = db.Model(&entity.SystemSettings{}).Count(&count).Error; err != nil {
		t.Fatalf("count system settings: %v", err)
	}
	if count != 3 {
		t.Fatalf("got %d rows, want 3", count)
	}

	// An operator-edited row keeps its value on reseed.
	if err = db.Model(&entity.SystemSettings{}).
		Where("name = ?", "enable_whitelist").
		Update("value", "false").Error; err != nil {
		t.Fatalf("edit seeded row: %v", err)
	}
	if err = insertMissingSystemSettings(ctx, db, seeds); err != nil {
		t.Fatalf("re-seed after edit: %v", err)
	}
	var setting entity.SystemSettings
	if err = db.Where("name = ?", "enable_whitelist").Take(&setting).Error; err != nil {
		t.Fatalf("read seeded row: %v", err)
	}
	if setting.Value != "false" {
		t.Fatalf("existing row was overwritten: got value %q, want \"false\"", setting.Value)
	}
}

func TestLoadSystemSettingsSeedFileInfersMissingDataType(t *testing.T) {
	body := `{"system_settings":[{"name":"mail.port","value":""}]}`
	path := filepath.Join(t.TempDir(), "system_settings.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	seeds, err := loadSystemSettingsSeedFile(path)
	if err != nil {
		t.Fatalf("load seed file: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("got %d seeds, want 1", len(seeds))
	}
	if seeds[0].DataType != entity.InferSystemSettingDataType("mail.port") {
		t.Fatalf("data_type not inferred: got %q", seeds[0].DataType)
	}
}
