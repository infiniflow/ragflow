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
	"fmt"
	"ragflow/internal/logger"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RunMigrations runs all manual database migrations
// These are migrations that cannot be handled by AutoMigrate alone
func RunMigrations(db *gorm.DB) error {
	// Check if tenant_llm table has composite primary key and migrate to ID primary key
	if err := migrateTenantLLMPrimaryKey(db); err != nil {
		return fmt.Errorf("failed to migrate tenant_llm primary key: %w", err)
	}

	// Rename columns (correct typos)
	if err := renameColumnIfExists(db, "task", "process_duation", "process_duration"); err != nil {
		return fmt.Errorf("failed to rename task.process_duation: %w", err)
	}
	if err := renameColumnIfExists(db, "document", "process_duation", "process_duration"); err != nil {
		return fmt.Errorf("failed to rename document.process_duation: %w", err)
	}

	// Add unique index on user.email
	if err := migrateAddUniqueEmail(db); err != nil {
		return fmt.Errorf("failed to add unique index on user.email: %w", err)
	}

	// Modify column types that AutoMigrate may not handle correctly
	if err := modifyColumnTypes(db); err != nil {
		return fmt.Errorf("failed to modify column types: %w", err)
	}

	logger.Info("All manual migrations completed successfully")
	return nil
}

// migrateTenantLLMPrimaryKey migrates tenant_llm from composite primary key to ID primary key
// This corresponds to Python's update_tenant_llm_to_id_primary_key function
func migrateTenantLLMPrimaryKey(db *gorm.DB) error {
	// Check if tenant_llm table exists
	if !db.Migrator().HasTable("tenant_llm") {
		return nil
	}

	// Check if 'id' column already exists
	if db.Migrator().HasColumn("tenant_llm", "id") {
		// Check if id is already a primary key with auto_increment
		var count int64
		err := db.Raw(`
			SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS 
			WHERE TABLE_NAME = 'tenant_llm' 
			AND COLUMN_NAME = 'id' 
			AND EXTRA LIKE '%auto_increment%'
		`).Scan(&count).Error
		if err != nil {
			return err
		}
		if count > 0 {
			// Already migrated
			return nil
		}
	}

	logger.Info("Migrating tenant_llm to use ID primary key...")

	// Start transaction
	return db.Transaction(func(tx *gorm.DB) error {
		// Check for temp_id column and drop it if exists
		if tx.Migrator().HasColumn("tenant_llm", "temp_id") {
			if err := tx.Exec("ALTER TABLE tenant_llm DROP COLUMN temp_id").Error; err != nil {
				logger.Warn("Failed to drop temp_id column", zap.Error(err))
			}
		}

		// Check if there's already an 'id' column
		if tx.Migrator().HasColumn("tenant_llm", "id") {
			// Modify existing id column to be auto_increment primary key
			if err := tx.Exec(`
				ALTER TABLE tenant_llm 
				MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY
			`).Error; err != nil {
				return fmt.Errorf("failed to modify id column: %w", err)
			}
		} else {
			// Add id column as auto_increment primary key
			if err := tx.Exec(`
				ALTER TABLE tenant_llm 
				ADD COLUMN id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY FIRST
			`).Error; err != nil {
				return fmt.Errorf("failed to add id column: %w", err)
			}
		}

		// Add unique index on (tenant_id, llm_factory, llm_name)
		if !tx.Migrator().HasIndex("tenant_llm", "idx_tenant_llm_unique") {
			if err := tx.Exec(`
				ALTER TABLE tenant_llm 
				ADD UNIQUE INDEX idx_tenant_llm_unique (tenant_id, llm_factory, llm_name)
			`).Error; err != nil {
				logger.Warn("Failed to add unique index idx_tenant_llm_unique", zap.Error(err))
			}
		}

		logger.Info("tenant_llm primary key migration completed")
		return nil
	})
}

// migrateAddUniqueEmail adds unique index on user.email
func migrateAddUniqueEmail(db *gorm.DB) error {
	if !db.Migrator().HasTable("user") {
		return nil
	}

	// Check if unique index already exists
	if db.Migrator().HasIndex("user", "idx_user_email_unique") {
		return nil
	}

	// Check if there's a duplicate email issue first
	var duplicateCount int64
	err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT email FROM user GROUP BY email HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&duplicateCount).Error
	if err != nil {
		return err
	}

	if duplicateCount > 0 {
		logger.Warn("Found duplicate emails in user table, cannot add unique index", zap.Int64("count", duplicateCount))
		return nil
	}

	logger.Info("Adding unique index on user.email...")
	if err := db.Exec(`ALTER TABLE user ADD UNIQUE INDEX idx_user_email_unique (email)`).Error; err != nil {
		return fmt.Errorf("failed to add unique index on email: %w", err)
	}

	return nil
}

// modifyColumnTypes modifies column types that need explicit ALTER statements
func modifyColumnTypes(db *gorm.DB) error {
	// dialog.top_k: ensure it's INTEGER with default 1024
	if db.Migrator().HasTable("dialog") && db.Migrator().HasColumn("dialog", "top_k") {
		if err := db.Exec(`ALTER TABLE dialog MODIFY COLUMN top_k BIGINT NOT NULL DEFAULT 1024`).Error; err != nil {
			logger.Warn("Failed to modify dialog.top_k", zap.Error(err))
		}
	}

	// tenant_llm.api_key: ensure it's TEXT type
	if db.Migrator().HasTable("tenant_llm") && db.Migrator().HasColumn("tenant_llm", "api_key") {
		if err := db.Exec(`ALTER TABLE tenant_llm MODIFY COLUMN api_key LONGTEXT`).Error; err != nil {
			logger.Warn("Failed to modify tenant_llm.api_key", zap.Error(err))
		}
	}

	// api_token.dialog_id: ensure it's varchar(32)
	if db.Migrator().HasTable("api_token") && db.Migrator().HasColumn("api_token", "dialog_id") {
		if err := db.Exec(`ALTER TABLE api_token MODIFY COLUMN dialog_id VARCHAR(32)`).Error; err != nil {
			logger.Warn("Failed to modify api_token.dialog_id", zap.Error(err))
		}
	}

	// canvas_template.title and description: ensure they're JSON type
	if db.Migrator().HasTable("canvas_template") {
		if db.Migrator().HasColumn("canvas_template", "title") {
			if err := db.Exec(`ALTER TABLE canvas_template MODIFY COLUMN title JSON DEFAULT '{}'`).Error; err != nil {
				logger.Warn("Failed to modify canvas_template.title", zap.Error(err))
			}
		}
		if db.Migrator().HasColumn("canvas_template", "description") {
			if err := db.Exec(`ALTER TABLE canvas_template MODIFY COLUMN description JSON DEFAULT '{}'`).Error; err != nil {
				logger.Warn("Failed to modify canvas_template.description", zap.Error(err))
			}
		}
	}

	// system_settings.value: ensure it's LONGTEXT
	if db.Migrator().HasTable("system_settings") && db.Migrator().HasColumn("system_settings", "value") {
		if err := db.Exec(`ALTER TABLE system_settings MODIFY COLUMN value LONGTEXT NOT NULL`).Error; err != nil {
			logger.Warn("Failed to modify system_settings.value", zap.Error(err))
		}
	}

	// knowledgebase.raptor_task_finish_at: ensure it's DateTime
	if db.Migrator().HasTable("knowledgebase") && db.Migrator().HasColumn("knowledgebase", "raptor_task_finish_at") {
		if err := db.Exec(`ALTER TABLE knowledgebase MODIFY COLUMN raptor_task_finish_at DATETIME`).Error; err != nil {
			logger.Warn("Failed to modify knowledgebase.raptor_task_finish_at", zap.Error(err))
		}
	}

	// knowledgebase.mindmap_task_finish_at: ensure it's DateTime
	if db.Migrator().HasTable("knowledgebase") && db.Migrator().HasColumn("knowledgebase", "mindmap_task_finish_at") {
		if err := db.Exec(`ALTER TABLE knowledgebase MODIFY COLUMN mindmap_task_finish_at DATETIME`).Error; err != nil {
			logger.Warn("Failed to modify knowledgebase.mindmap_task_finish_at", zap.Error(err))
		}
	}

	return nil
}

// renameColumnIfExists renames a column if it exists and the new column doesn't exist
func renameColumnIfExists(db *gorm.DB, tableName, oldName, newName string) error {
	if !db.Migrator().HasTable(tableName) {
		return nil
	}

	// Check if old column exists
	if !db.Migrator().HasColumn(tableName, oldName) {
		return nil
	}

	// Check if new column already exists
	if db.Migrator().HasColumn(tableName, newName) {
		// Both exist, drop the old one
		logger.Warn("Both old and new columns exist, dropping old one",
			zap.String("table", tableName),
			zap.String("oldColumn", oldName),
			zap.String("newColumn", newName))
		return db.Migrator().DropColumn(tableName, oldName)
	}

	logger.Info("Renaming column",
		zap.String("table", tableName),
		zap.String("oldColumn", oldName),
		zap.String("newColumn", newName))
	return db.Migrator().RenameColumn(tableName, oldName, newName)
}

// addColumnIfNotExists adds a column if it doesn't exist
func addColumnIfNotExists(db *gorm.DB, tableName, columnName, columnDef string) error {
	if !db.Migrator().HasTable(tableName) {
		return nil
	}

	if db.Migrator().HasColumn(tableName, columnName) {
		return nil
	}

	logger.Info("Adding column",
		zap.String("table", tableName),
		zap.String("column", columnName))
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)
	return db.Exec(sql).Error
}
