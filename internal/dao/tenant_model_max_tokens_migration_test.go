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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupTenantModelMaxTokensMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.SystemSettings{},
		&entity.TenantLLM{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to migrate tenant model tables: %v", err)
	}
	return db
}

func seedLegacyTenantModelMaxTokens(t *testing.T, db *gorm.DB, maxTokens int64) {
	t.Helper()
	modelName := "custom-model"
	modelType := "chat"
	apiKey := `{"api_key":"secret","is_tools":true}`
	if err := db.Create(&entity.TenantModelProvider{
		ID:           "provider-1",
		ProviderName: "Ollama",
		TenantID:     "tenant-1",
	}).Error; err != nil {
		t.Fatalf("failed to seed provider: %v", err)
	}
	if err := db.Create(&entity.TenantModelInstance{
		ID:           "instance-1",
		InstanceName: "default",
		ProviderID:   "provider-1",
		APIKey:       "secret",
		Status:       "active",
	}).Error; err != nil {
		t.Fatalf("failed to seed model instance: %v", err)
	}
	if err := db.Create(&entity.TenantLLM{
		TenantID:   "tenant-1",
		LLMFactory: "Ollama",
		LLMName:    &modelName,
		ModelType:  &modelType,
		APIKey:     &apiKey,
		MaxTokens:  maxTokens,
		Status:     "1",
	}).Error; err != nil {
		t.Fatalf("failed to seed legacy tenant_llm row: %v", err)
	}
}

func TestMigrateTenantModelMaxTokensBackfillsOnlyMissingValues(t *testing.T) {
	db := setupTenantModelMaxTokensMigrationDB(t)
	seedLegacyTenantModelMaxTokens(t, db, 65536)

	for _, legacy := range []struct {
		name      string
		maxTokens int64
	}{
		{name: "configured-model", maxTokens: 32768},
		{name: "zero-model", maxTokens: 4096},
	} {
		modelType := "chat"
		apiKey := `{"api_key":"secret","is_tools":true}`
		if err := db.Create(&entity.TenantLLM{
			TenantID:   "tenant-1",
			LLMFactory: "Ollama",
			LLMName:    &legacy.name,
			ModelType:  &modelType,
			APIKey:     &apiKey,
			MaxTokens:  legacy.maxTokens,
			Status:     "1",
		}).Error; err != nil {
			t.Fatalf("failed to seed legacy model %q: %v", legacy.name, err)
		}
	}

	models := []entity.TenantModel{
		{
			ID:         "model-missing",
			ModelName:  "custom-model",
			ProviderID: "provider-1",
			InstanceID: "instance-1",
			ModelType:  int(entity.ModelTypeChat),
			Extra:      `{"is_tools":true}`,
		},
		{
			ID:         "model-configured",
			ModelName:  "configured-model",
			ProviderID: "provider-1",
			InstanceID: "instance-1",
			ModelType:  int(entity.ModelTypeChat),
			Extra:      `{"max_tokens":16384,"is_tools":true}`,
		},
		{
			ID:         "model-zero",
			ModelName:  "zero-model",
			ProviderID: "provider-1",
			InstanceID: "instance-1",
			ModelType:  int(entity.ModelTypeChat),
			Extra:      `{"max_tokens":0}`,
		},
	}
	if err := db.Create(&models).Error; err != nil {
		t.Fatalf("failed to seed tenant models: %v", err)
	}
	if err := setDatabaseMigrationVersion(t.Context(), db, conversationHistoryTargetVersion); err != nil {
		t.Fatalf("failed to set rc1 migration version: %v", err)
	}

	if err := migrateTenantModelMaxTokens(t.Context(), db); err != nil {
		t.Fatalf("migrateTenantModelMaxTokens() error = %v", err)
	}

	var missing, configured, zero entity.TenantModel
	for _, item := range []struct {
		id     string
		target *entity.TenantModel
	}{
		{id: "model-missing", target: &missing},
		{id: "model-configured", target: &configured},
		{id: "model-zero", target: &zero},
	} {
		if err := db.Where("id = ?", item.id).Take(item.target).Error; err != nil {
			t.Fatalf("failed to read tenant model %q: %v", item.id, err)
		}
	}
	if got := modelExtraMaxTokens(missing.Extra); got != 65536 {
		t.Fatalf("backfilled max_tokens = %d, want 65536 (extra: %s)", got, missing.Extra)
	}
	var missingExtra struct {
		IsTools bool `json:"is_tools"`
	}
	if err := json.Unmarshal([]byte(missing.Extra), &missingExtra); err != nil {
		t.Fatalf("failed to decode backfilled extra: %v", err)
	}
	if !missingExtra.IsTools {
		t.Fatalf("backfill lost existing extra fields: %s", missing.Extra)
	}
	if got := modelExtraMaxTokens(configured.Extra); got != 16384 {
		t.Fatalf("existing max_tokens was overwritten: got %d, want 16384", got)
	}
	if got := modelExtraMaxTokens(zero.Extra); got != 0 {
		t.Fatalf("explicit zero max_tokens was overwritten: got %d, want 0", got)
	}
	if hasMaxTokens, err := tenantModelExtraHasMaxTokens(zero.Extra); err != nil || !hasMaxTokens {
		t.Fatalf("zero-model max_tokens presence = %v, err = %v; want present", hasMaxTokens, err)
	}

	version, err := GetDatabaseMigrationVersion(t.Context(), db)
	if err != nil {
		t.Fatalf("GetDatabaseMigrationVersion() error = %v", err)
	}
	if version != tenantModelMaxTokensTargetVersion {
		t.Fatalf("database migration version = %q, want %q", version, tenantModelMaxTokensTargetVersion)
	}
}

func TestBackfillTenantModelMaxTokensSkipsCatalogDefaults(t *testing.T) {
	var providerName, modelName, modelType string
	var catalogLimit int64
	for _, factory := range loadFactoryLLMInfos() {
		for _, model := range factory.LLM {
			if err := json.Unmarshal(model.MaxTokens, &catalogLimit); err == nil && catalogLimit > 0 {
				providerName, modelName = factory.Name, model.LLMName
				modelType = factoryLLMModelTypes(model.ModelType)[0]
				break
			}
		}
		if providerName != "" {
			break
		}
	}
	if providerName == "" {
		t.Fatal("factory catalog contains no positive model limits")
	}
	for _, tc := range []struct {
		name        string
		limit       int64
		wantUpdated int
	}{
		{name: "catalog default", limit: catalogLimit, wantUpdated: 0},
		{name: "custom limit", limit: catalogLimit + 1, wantUpdated: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTenantModelMaxTokensMigrationDB(t)
			seedLegacyTenantModelMaxTokens(t, db, tc.limit)
			if err := db.Model(&entity.TenantModelProvider{}).Where("id = ?", "provider-1").Update("provider_name", providerName).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&entity.TenantLLM{}).Where("tenant_id = ?", "tenant-1").Updates(map[string]any{
				"llm_factory": providerName, "llm_name": modelName, "model_type": modelType,
			}).Error; err != nil {
				t.Fatal(err)
			}
			model := entity.TenantModel{
				ID: "catalog-model", ProviderID: "provider-1", InstanceID: "instance-1",
				ModelName: modelName, ModelType: modelTypeStringToBit[modelType], Extra: `{"is_tools":true}`,
			}
			if err := db.Create(&model).Error; err != nil {
				t.Fatal(err)
			}
			updated, err := backfillTenantModelMaxTokens(t.Context(), db)
			if err != nil || updated != tc.wantUpdated {
				t.Fatalf("backfill = %d, %v; want %d, nil", updated, err, tc.wantUpdated)
			}
			if err := db.Where("id = ?", model.ID).Take(&model).Error; err != nil {
				t.Fatal(err)
			}
			if tc.wantUpdated == 0 && model.Extra != `{"is_tools":true}` {
				t.Fatalf("catalog default changed extra: %s", model.Extra)
			}
			if tc.wantUpdated == 1 && int64(modelExtraMaxTokens(model.Extra)) != tc.limit {
				t.Fatalf("custom limit was not preserved: %s", model.Extra)
			}
		})
	}
}
