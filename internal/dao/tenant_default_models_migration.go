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
	"fmt"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const tenantDefaultModelsTargetVersion = "v1.0.0-rc3.dev5"

var tenantDefaultModelColumns = []string{
	"llm_id", "tenant_llm_id",
	"embd_id", "tenant_embd_id",
	"asr_id", "tenant_asr_id",
	"img2txt_id", "tenant_img2txt_id",
	"rerank_id", "tenant_rerank_id",
	"tts_id", "tenant_tts_id",
	"ocr_id", "tenant_ocr_id",
	"decision_id", "tenant_decision_id",
}

type tenantDefaultModelsLegacySchema struct {
	ID               string  `gorm:"column:id;primaryKey"`
	LLMID            string  `gorm:"column:llm_id"`
	TenantLLMID      *string `gorm:"column:tenant_llm_id"`
	EmbdID           string  `gorm:"column:embd_id"`
	TenantEmbdID     *string `gorm:"column:tenant_embd_id"`
	ASRID            string  `gorm:"column:asr_id"`
	TenantASRID      *string `gorm:"column:tenant_asr_id"`
	Img2TxtID        string  `gorm:"column:img2txt_id"`
	TenantImg2TxtID  *string `gorm:"column:tenant_img2txt_id"`
	RerankID         string  `gorm:"column:rerank_id"`
	TenantRerankID   *string `gorm:"column:tenant_rerank_id"`
	TTSID            *string `gorm:"column:tts_id"`
	TenantTTSID      *string `gorm:"column:tenant_tts_id"`
	OCRID            *string `gorm:"column:ocr_id"`
	TenantOCRID      *string `gorm:"column:tenant_ocr_id"`
	DecisionID       *string `gorm:"column:decision_id"`
	TenantDecisionID *string `gorm:"column:tenant_decision_id"`
}

func (tenantDefaultModelsLegacySchema) TableName() string {
	return "tenant"
}

func tenantDefaultModelColumnExists(migrator gorm.Migrator, name string) (bool, error) {
	columns, err := migrator.ColumnTypes(&tenantDefaultModelsLegacySchema{})
	if err != nil {
		return false, err
	}
	for _, column := range columns {
		if strings.EqualFold(column.Name(), name) {
			return true, nil
		}
	}
	return false, nil
}

// migrateTenantDefaultModels moves the old tenant-wide model selections to
// the tenant owner's membership, then removes the tenant-wide columns.
func migrateTenantDefaultModels(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(version, tenantDefaultModelsTargetVersion) {
		return nil
	}

	scoped := db.WithContext(ctx)
	migrator := scoped.Migrator()
	if migrator.HasTable("tenant") && migrator.HasTable(&entity.UserTenant{}) {
		if err := scoped.Transaction(func(tx *gorm.DB) error {
			for _, column := range tenantDefaultModelColumns {
				exists, err := tenantDefaultModelColumnExists(tx.Migrator(), column)
				if err != nil {
					return fmt.Errorf("inspect tenant.%s: %w", column, err)
				}
				if !exists {
					continue
				}
				if !tx.Migrator().HasColumn(&entity.UserTenant{}, column) {
					return fmt.Errorf("user_tenant.%s is missing", column)
				}
				query := fmt.Sprintf(
					"UPDATE user_tenant SET %s = (SELECT tenant.%s FROM tenant WHERE tenant.id = user_tenant.tenant_id) "+
						"WHERE (user_tenant.%s IS NULL OR user_tenant.%s = '') "+
						"AND (user_tenant.role = 'owner' OR user_tenant.user_id = user_tenant.tenant_id) "+
						"AND EXISTS (SELECT 1 FROM tenant WHERE tenant.id = user_tenant.tenant_id)",
					column, column, column, column,
				)
				if err := tx.Exec(query).Error; err != nil {
					return fmt.Errorf("copy tenant.%s to user_tenant: %w", column, err)
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("copy tenant default models: %w", err)
		}

		for _, column := range tenantDefaultModelColumns {
			exists, err := tenantDefaultModelColumnExists(migrator, column)
			if err != nil {
				return fmt.Errorf("inspect tenant.%s: %w", column, err)
			}
			if !exists {
				continue
			}
			if err := migrator.DropColumn(&tenantDefaultModelsLegacySchema{}, column); err != nil {
				return fmt.Errorf("drop tenant.%s: %w", column, err)
			}
			exists, err = tenantDefaultModelColumnExists(migrator, column)
			if err != nil {
				return fmt.Errorf("inspect tenant.%s after dropping it: %w", column, err)
			}
			if exists {
				if err := scoped.Exec("ALTER TABLE tenant DROP COLUMN " + column).Error; err != nil {
					return fmt.Errorf("drop tenant.%s: %w", column, err)
				}
			}
			exists, err = tenantDefaultModelColumnExists(migrator, column)
			if err != nil {
				return fmt.Errorf("inspect tenant.%s after dropping it: %w", column, err)
			}
			if exists {
				return fmt.Errorf("tenant.%s still exists after dropping it", column)
			}
		}
	}

	if err := setDatabaseMigrationVersion(ctx, db, tenantDefaultModelsTargetVersion); err != nil {
		return fmt.Errorf("mark tenant default model migration version: %w", err)
	}
	return nil
}
