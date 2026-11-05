//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package dao

import (
	"context"
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const resourceUserIDMigrationTargetVersion = "v1.0.0.1"

// migrateResourceUserIDs fills owner IDs added to resource tables. Existing
// created_by values identify the resource owner; tables without created_by
// retain the OSS invariant that tenant_id identifies the owner.
func migrateResourceUserIDs(ctx context.Context, db *gorm.DB) error {
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(version, resourceUserIDMigrationTargetVersion) {
		return nil
	}

	resources := []struct {
		table          string
		model          interface{}
		ownerField     string
		ownerColumn    string
		fallbackField  string
		fallbackColumn string
	}{
		{table: "chat_channel", model: &entity.ChatChannel{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "compilation_template_group", model: &entity.CompilationTemplateGroup{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "compilation_template", model: &entity.CompilationTemplate{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "connector", model: &entity.Connector{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "dialog", model: &entity.Chat{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "file", model: &entity.File{}, ownerField: "CreatedBy", ownerColumn: "created_by", fallbackField: "TenantID", fallbackColumn: "tenant_id"},
		{table: "knowledgebase", model: &entity.Knowledgebase{}, ownerField: "CreatedBy", ownerColumn: "created_by", fallbackField: "TenantID", fallbackColumn: "tenant_id"},
		{table: "memory", model: &entity.Memory{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "search", model: &entity.Search{}, ownerField: "CreatedBy", ownerColumn: "created_by", fallbackField: "TenantID", fallbackColumn: "tenant_id"},
		{table: "mcp_server", model: &entity.MCPServer{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "skill_spaces", model: &entity.SkillSpace{}, ownerField: "TenantID", ownerColumn: "tenant_id"},
		{table: "document", model: &entity.Document{}, ownerField: "CreatedBy", ownerColumn: "created_by"},
		{table: "evaluation_datasets", model: &entity.EvaluationDataset{}, ownerField: "CreatedBy", ownerColumn: "created_by", fallbackField: "TenantID", fallbackColumn: "tenant_id"},
	}

	scoped := db.WithContext(ctx)
	migrator := scoped.Migrator()
	for _, resource := range resources {
		if !migrator.HasTable(resource.model) {
			continue
		}
		if !migrator.HasColumn(resource.model, "UserID") {
			return fmt.Errorf("%s.user_id is missing after GORM migration", resource.table)
		}
		if !migrator.HasColumn(resource.model, resource.ownerField) {
			return fmt.Errorf("%s.%s is missing", resource.table, resource.ownerColumn)
		}
		if resource.fallbackField != "" && !migrator.HasColumn(resource.model, resource.fallbackField) {
			return fmt.Errorf("%s.%s is missing", resource.table, resource.fallbackColumn)
		}

		ownerExpr := fmt.Sprintf("NULLIF(%s, '')", resource.ownerColumn)
		if resource.fallbackColumn != "" {
			ownerExpr = fmt.Sprintf("COALESCE(%s, NULLIF(%s, ''), '')", ownerExpr, resource.fallbackColumn)
		} else {
			ownerExpr = fmt.Sprintf("COALESCE(%s, '')", ownerExpr)
		}

		if err := scoped.Table(resource.table).
			Where("user_id IS NULL OR user_id = ?", "").
			UpdateColumn("user_id", gorm.Expr(ownerExpr)).Error; err != nil {
			return fmt.Errorf("backfill %s.user_id from %s: %w", resource.table, resource.ownerColumn, err)
		}
	}
	if err := setDatabaseMigrationVersion(ctx, db, resourceUserIDMigrationTargetVersion); err != nil {
		return fmt.Errorf("mark database migration version %s: %w", resourceUserIDMigrationTargetVersion, err)
	}
	return nil
}
