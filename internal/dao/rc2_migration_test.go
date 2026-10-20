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
	"encoding/json"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupRC2MigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.SystemSettings{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.TenantLLM{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to auto-migrate tables: %v", err)
	}
	return db
}

func TestRC2DataMigration_StartupOrderPreservesTenantModelMaxTokens(t *testing.T) {
	ctx := t.Context()
	db := setupRC2MigrationDB(t)

	// 1. Set DB version to rc1
	if err := setDatabaseMigrationVersion(ctx, db, conversationHistoryTargetVersion); err != nil {
		t.Fatalf("failed to set rc1 migration version: %v", err)
	}

	// 2. Insert knowledgebase with legacy flat parser config and retired keys
	insertIngestionConfig(t, db, "knowledgebase", "kb-1", "general", `{"chunk_token_num":128,"raptor":{"use_raptor":true},"auto_keywords":8}`)

	// 3. Seed legacy tenant_llm and matching target model
	seedLegacyTenantModelMaxTokens(t, db, 65536)
	targetModel := entity.TenantModel{
		ID:         "model-target",
		ModelName:  "custom-model",
		ProviderID: "provider-1",
		InstanceID: "instance-1",
		ModelType:  int(entity.ModelTypeChat),
		Extra:      `{"is_tools":true}`,
	}
	if err := db.Create(&targetModel).Error; err != nil {
		t.Fatalf("failed to seed target tenant model: %v", err)
	}

	// 4. Run the rc2 steps in official startup sequence
	if err := migrateIngestionParserConfigs(ctx, db); err != nil {
		t.Fatalf("migrateIngestionParserConfigs error = %v", err)
	}
	if err := migrateTenantModelMaxTokens(ctx, db); err != nil {
		t.Fatalf("migrateTenantModelMaxTokens error = %v", err)
	}

	// 5. Verify target tenant model was properly backfilled with 65536
	var updatedModel entity.TenantModel
	if err := db.Where("id = ?", targetModel.ID).Take(&updatedModel).Error; err != nil {
		t.Fatalf("failed to read updated tenant model: %v", err)
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(updatedModel.Extra), &extra); err != nil {
		t.Fatalf("failed to unmarshal updated extra: %v", err)
	}
	maxTokensVal, ok := extra["max_tokens"]
	if !ok {
		t.Fatalf("extra.max_tokens was not backfilled, extra: %s", updatedModel.Extra)
	}
	if maxTokensFloat, ok := maxTokensVal.(float64); !ok || int64(maxTokensFloat) != 65536 {
		t.Fatalf("extra.max_tokens = %v, want 65536", maxTokensVal)
	}

	// 6. Verify knowledgebase parser config was converted and raptor retired key removed
	var updatedKB entity.Knowledgebase
	if err := db.Where("id = ?", "kb-1").Take(&updatedKB).Error; err != nil {
		t.Fatalf("failed to read updated knowledgebase: %v", err)
	}
	kbConfig := updatedKB.ParserConfig
	if _, remains := kbConfig["raptor"]; remains {
		t.Fatal("retired raptor key was not removed")
	}
	if _, remains := kbConfig["chunk_token_num"]; remains {
		t.Fatal("flat chunk_token_num was not converted")
	}

	// 7. Verify version marker is rc2
	ver, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("failed to get database migration version: %v", err)
	}
	if ver != rc2MigrationTargetVersion {
		t.Fatalf("database version = %q, want %q", ver, rc2MigrationTargetVersion)
	}
}

func TestRC2DataMigration_RollbackOnFailure(t *testing.T) {
	ctx := t.Context()
	db := setupRC2MigrationDB(t)

	// Set DB version to rc1
	if err := setDatabaseMigrationVersion(ctx, db, conversationHistoryTargetVersion); err != nil {
		t.Fatalf("failed to set rc1 migration version: %v", err)
	}

	// Convert a knowledgebase row before the invalid document row fails migration.
	insertIngestionConfig(t, db, "knowledgebase", "kb-0", "general", `{"chunk_token_num":128}`)
	insertIngestionConfig(t, db, "document", "doc-invalid", "general", `not-valid-json`)

	// Seed tenant model that should NOT be modified due to rollback
	seedLegacyTenantModelMaxTokens(t, db, 65536)
	targetModel := entity.TenantModel{
		ID:         "model-target-rollback",
		ModelName:  "custom-model",
		ProviderID: "provider-1",
		InstanceID: "instance-1",
		ModelType:  int(entity.ModelTypeChat),
		Extra:      `{"is_tools":true}`,
	}
	if err := db.Create(&targetModel).Error; err != nil {
		t.Fatalf("failed to seed target tenant model: %v", err)
	}

	// Run rc2 migration which should fail
	err := migrateRC2Data(ctx, db)
	if err == nil {
		t.Fatal("expected migrateRC2Data to fail on invalid document parser_config JSON")
	}

	// The knowledgebase conversion ran before the document failed, so rollback must
	// restore the original flat configuration rather than partially upgrading it.
	var checkKB entity.Knowledgebase
	if err := db.Where("id = ?", "kb-0").Take(&checkKB).Error; err != nil {
		t.Fatalf("failed to read checkKB: %v", err)
	}
	if _, remains := checkKB.ParserConfig["chunk_token_num"]; !remains {
		t.Fatal("expected kb-0 parser_config to remain in its original flat form")
	}

	// Verify tenant model extra was NOT modified
	var checkModel entity.TenantModel
	if err := db.Where("id = ?", targetModel.ID).Take(&checkModel).Error; err != nil {
		t.Fatalf("failed to read checkModel: %v", err)
	}
	if checkModel.Extra != `{"is_tools":true}` {
		t.Fatalf("expected Extra to remain untouched, got: %s", checkModel.Extra)
	}

	// Verify version remains at rc1
	ver, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("failed to get database migration version: %v", err)
	}
	if ver != conversationHistoryTargetVersion {
		t.Fatalf("database version = %q, want %q", ver, conversationHistoryTargetVersion)
	}
}

func TestRC2DataMigration_RollsBackWhenMarkerWriteFails(t *testing.T) {
	ctx := t.Context()
	db := setupRC2MigrationDB(t)
	if err := setDatabaseMigrationVersion(ctx, db, conversationHistoryTargetVersion); err != nil {
		t.Fatalf("failed to set rc1 migration version: %v", err)
	}
	insertIngestionConfig(t, db, "knowledgebase", "kb-marker-failure", "general", `{"chunk_token_num":128}`)
	seedLegacyTenantModelMaxTokens(t, db, 65536)
	targetModel := entity.TenantModel{
		ID:         "model-marker-failure",
		ModelName:  "custom-model",
		ProviderID: "provider-1",
		InstanceID: "instance-1",
		ModelType:  int(entity.ModelTypeChat),
		Extra:      `{"is_tools":true}`,
	}
	if err := db.Create(&targetModel).Error; err != nil {
		t.Fatalf("failed to seed target tenant model: %v", err)
	}

	backfillObserved := false
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_rc2_migration_marker", func(tx *gorm.DB) {
		setting, ok := tx.Statement.Dest.(*entity.SystemSettings)
		if ok && setting.Name == migrationDBVersionMarker && setting.Value == rc2MigrationTargetVersion {
			var backfilled entity.TenantModel
			if err := tx.Session(&gorm.Session{NewDB: true}).Where("id = ?", targetModel.ID).Take(&backfilled).Error; err == nil {
				var extra map[string]any
				if err := json.Unmarshal([]byte(backfilled.Extra), &extra); err == nil {
					_, backfillObserved = extra["max_tokens"]
				}
			}
			tx.AddError(errors.New("injected migration marker write failure"))
		}
	}); err != nil {
		t.Fatalf("failed to register migration marker failure callback: %v", err)
	}

	if err := migrateRC2Data(ctx, db); err == nil {
		t.Fatal("expected migrateRC2Data to fail when writing the rc2 marker")
	}
	if !backfillObserved {
		t.Fatal("expected tenant model backfill to complete before injected marker failure")
	}

	var checkKB entity.Knowledgebase
	if err := db.Where("id = ?", "kb-marker-failure").Take(&checkKB).Error; err != nil {
		t.Fatalf("failed to read checkKB: %v", err)
	}
	if _, remains := checkKB.ParserConfig["chunk_token_num"]; !remains {
		t.Fatal("expected parser config conversion to roll back after marker write failure")
	}
	var checkModel entity.TenantModel
	if err := db.Where("id = ?", targetModel.ID).Take(&checkModel).Error; err != nil {
		t.Fatalf("failed to read tenant model: %v", err)
	}
	if checkModel.Extra != `{"is_tools":true}` {
		t.Fatalf("expected tenant model extra to roll back, got: %s", checkModel.Extra)
	}

	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("failed to read migration version: %v", err)
	}
	if version != conversationHistoryTargetVersion {
		t.Fatalf("database version = %q, want %q", version, conversationHistoryTargetVersion)
	}
}

func TestRC2DataMigration_Idempotency(t *testing.T) {
	ctx := t.Context()
	db := setupRC2MigrationDB(t)

	if err := setDatabaseMigrationVersion(ctx, db, rc2MigrationTargetVersion); err != nil {
		t.Fatalf("failed to set rc2 version: %v", err)
	}

	// Calling migrateRC2Data when already at rc2 should immediately return nil
	if err := migrateRC2Data(ctx, db); err != nil {
		t.Fatalf("expected migrateRC2Data to be no-op when already migrated, got: %v", err)
	}
}
