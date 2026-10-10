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
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const tenantModelMaxTokensTargetVersion = "v1.0.0-rc2"

type tenantModelMaxTokensTarget struct {
	ID         string `gorm:"column:id"`
	ProviderID string `gorm:"column:provider_id"`
	InstanceID string `gorm:"column:instance_id"`
	ModelName  string `gorm:"column:model_name"`
	ModelType  int    `gorm:"column:model_type"`
	Extra      string `gorm:"column:extra"`
}

type tenantLLMMaxTokensSource struct {
	ModelName    sql.NullString `gorm:"column:model_name"`
	ModelType    sql.NullString `gorm:"column:model_type"`
	APIKey       sql.NullString `gorm:"column:api_key"`
	MaxTokens    sql.NullInt64  `gorm:"column:max_tokens"`
	ProviderID   string         `gorm:"column:provider_id"`
	ProviderName string         `gorm:"column:provider_name"`
}

type tenantModelMaxTokensUpdate struct {
	extra     string
	maxTokens int64
}

// migrateTenantModelMaxTokens copies the legacy per-model context window into
// tenant_model.extra for databases that already completed the tenant-model
// migration. Existing extra.max_tokens values are never overwritten.
// Values identical to the factory catalog are not stored as tenant overrides.
//
// It runs after the rc1 migrations because all database migrations share one
// monotonically increasing version marker.
func migrateTenantModelMaxTokens(ctx context.Context, db *gorm.DB) error {
	currentVersion, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(currentVersion, tenantModelMaxTokensTargetVersion) {
		common.Info("Tenant model max_tokens already migrated, skipping",
			zap.String("current_version", currentVersion),
			zap.String("target_version", tenantModelMaxTokensTargetVersion))
		return nil
	}

	updated, err := backfillTenantModelMaxTokens(ctx, db)
	if err != nil {
		return err
	}
	if err := setDatabaseMigrationVersion(ctx, db, tenantModelMaxTokensTargetVersion); err != nil {
		return fmt.Errorf("mark database migration version %s: %w", tenantModelMaxTokensTargetVersion, err)
	}
	common.Info("Tenant model max_tokens migration completed",
		zap.String("version", tenantModelMaxTokensTargetVersion),
		zap.Int("models_updated", updated))
	return nil
}

func backfillTenantModelMaxTokens(ctx context.Context, db *gorm.DB) (int, error) {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("tenant_llm") {
		common.Info("Legacy tenant_llm table not found; no max_tokens to backfill")
		return 0, nil
	}
	if !scoped.Migrator().HasColumn(&entity.TenantLLM{}, "MaxTokens") {
		return 0, fmt.Errorf("legacy tenant_llm.max_tokens column is missing")
	}
	for _, table := range []string{"tenant_model_provider", "tenant_model_instance", "tenant_model"} {
		if !scoped.Migrator().HasTable(table) {
			return 0, fmt.Errorf("required table %s is missing", table)
		}
	}

	instanceLookup, err := buildTenantModelInstanceLookup(ctx, scoped)
	if err != nil {
		return 0, fmt.Errorf("build tenant model instance lookup: %w", err)
	}
	var sourceRows []tenantLLMMaxTokensSource
	if err := scoped.Raw(`
		SELECT tl.llm_name AS model_name, tl.model_type AS model_type,
		       tl.api_key AS api_key, tl.max_tokens AS max_tokens,
	       tmp.id AS provider_id, tmp.provider_name AS provider_name
		FROM tenant_llm tl
		INNER JOIN tenant_model_provider tmp
			ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory
		ORDER BY tl.tenant_id, tl.llm_factory, tl.llm_name, tl.model_type`).Scan(&sourceRows).Error; err != nil {
		return 0, fmt.Errorf("read legacy tenant_llm model limits: %w", err)
	}
	if len(sourceRows) == 0 {
		return 0, nil
	}

	var targets []tenantModelMaxTokensTarget
	if err := scoped.Raw(`
		SELECT id, provider_id, instance_id, model_name, model_type, extra
		FROM tenant_model`).Scan(&targets).Error; err != nil {
		return 0, fmt.Errorf("read tenant model limits: %w", err)
	}
	targetsByKey := make(map[string][]tenantModelMaxTokensTarget, len(targets))
	for _, target := range targets {
		key := tenantModelMaxTokensKey(target.ProviderID, target.InstanceID, target.ModelName)
		targetsByKey[key] = append(targetsByKey[key], target)
	}

	updates := make(map[string]tenantModelMaxTokensUpdate)
	catalogLimits := factoryModelMaxTokensLookup(loadFactoryLLMInfos())
	for _, row := range sourceRows {
		if !row.ModelName.Valid || !row.MaxTokens.Valid || row.MaxTokens.Int64 <= 0 {
			continue
		}
		catalogKey := factoryModelMaxTokensKey(row.ProviderName, row.ModelName.String, row.ModelType.String)
		if limit, exists := catalogLimits[catalogKey]; exists && limit == row.MaxTokens.Int64 {
			continue
		}
		instanceID := instanceLookup[row.ProviderID+"\x00"+stripIsToolsFromAPIKey(row.APIKey.String)]
		if instanceID == "" {
			continue
		}
		key := tenantModelMaxTokensKey(row.ProviderID, instanceID, row.ModelName.String)
		candidates := targetsByKey[key]
		if len(candidates) == 0 {
			continue
		}
		modelType := modelTypeStringToBit[strings.ToLower(strings.TrimSpace(row.ModelType.String))]
		if modelType == 0 && len(candidates) != 1 {
			continue
		}
		for _, target := range candidates {
			if modelType != 0 && target.ModelType != 0 && target.ModelType&modelType == 0 {
				continue
			}
			hasMaxTokens, err := tenantModelExtraHasMaxTokens(target.Extra)
			if err != nil {
				return 0, fmt.Errorf("decode tenant_model.extra for model %q: %w", target.ID, err)
			}
			if hasMaxTokens {
				continue
			}
			current := updates[target.ID]
			if row.MaxTokens.Int64 > current.maxTokens {
				updates[target.ID] = tenantModelMaxTokensUpdate{
					extra:     target.Extra,
					maxTokens: row.MaxTokens.Int64,
				}
			}
		}
	}

	if len(updates) == 0 {
		return 0, nil
	}
	ids := make([]string, 0, len(updates))
	for id := range updates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	updated := 0
	if err := scoped.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			update := updates[id]
			extra, err := setModelExtraMaxTokens(update.extra, update.maxTokens)
			if err != nil {
				return fmt.Errorf("encode tenant_model.extra for model %q: %w", id, err)
			}
			result := tx.Model(&entity.TenantModel{}).Where("id = ?", id).Update("extra", extra)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("tenant model %q disappeared during max_tokens backfill", id)
			}
			updated++
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("update tenant_model.extra: %w", err)
	}
	return updated, nil
}

func tenantModelMaxTokensKey(providerID, instanceID, modelName string) string {
	return providerID + "\x00" + instanceID + "\x00" + modelName
}

func factoryModelMaxTokensKey(providerName, modelName, modelType string) string {
	return strings.ToLower(providerName) + "\x00" + modelName + "\x00" + strings.ToLower(strings.TrimSpace(modelType))
}

func factoryModelMaxTokensLookup(factories []factoryLLMInfo) map[string]int64 {
	limits := make(map[string]int64)
	for _, factory := range factories {
		for _, model := range factory.LLM {
			var maxTokens int64
			if err := json.Unmarshal(model.MaxTokens, &maxTokens); err != nil || maxTokens <= 0 {
				continue
			}
			for _, modelType := range factoryLLMModelTypes(model.ModelType) {
				limits[factoryModelMaxTokensKey(factory.Name, model.LLMName, modelType)] = maxTokens
			}
		}
	}
	return limits
}

func tenantModelExtraHasMaxTokens(extra string) (bool, error) {
	if strings.TrimSpace(extra) == "" {
		return false, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(extra), &values); err != nil {
		return false, err
	}
	_, exists := values["max_tokens"]
	return exists, nil
}

func setModelExtraMaxTokens(raw string, maxTokens int64) (string, error) {
	var extra map[string]json.RawMessage
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &extra); err != nil {
			return "", err
		}
	}
	if extra == nil {
		extra = make(map[string]json.RawMessage)
	}
	value, err := json.Marshal(maxTokens)
	if err != nil {
		return "", err
	}
	extra["max_tokens"] = value
	encoded, err := json.Marshal(extra)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
