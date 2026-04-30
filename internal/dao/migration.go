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
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"strings"

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

	// Create skill search tables
	if err := migrateSkillSearchTables(db); err != nil {
		return fmt.Errorf("failed to migrate skill search tables: %w", err)
	}

	// Create skill space tables
	if err := migrateSkillSpaceTables(db); err != nil {
		return fmt.Errorf("failed to migrate skill space tables: %w", err)
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

	// Check if 'id' column already exists using raw SQL
	var idColumnExists int64
	err := db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS 
		WHERE TABLE_NAME = 'tenant_llm' AND COLUMN_NAME = 'id'
	`).Scan(&idColumnExists).Error
	if err != nil {
		return err
	}

	if idColumnExists > 0 {
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
		var tempIdExists int64
		tx.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS 
			WHERE TABLE_NAME = 'tenant_llm' AND COLUMN_NAME = 'temp_id'`).Scan(&tempIdExists)
		if tempIdExists > 0 {
			if err := tx.Exec("ALTER TABLE tenant_llm DROP COLUMN temp_id").Error; err != nil {
				logger.Warn("Failed to drop temp_id column", zap.Error(err))
			}
		}

		// Check if there's already an 'id' column
		if idColumnExists > 0 {
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
		var idxExists int64
		tx.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS 
			WHERE TABLE_NAME = 'tenant_llm' AND INDEX_NAME = 'idx_tenant_llm_unique'`).Scan(&idxExists)
		if idxExists == 0 {
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

	// Check if unique index already exists using raw SQL
	var count int64
	db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS 
		WHERE TABLE_NAME = 'user' AND INDEX_NAME = 'idx_user_email_unique'`).Scan(&count)
	if count > 0 {
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
	if err = db.Exec(`ALTER TABLE user ADD UNIQUE INDEX idx_user_email_unique (email)`).Error; err != nil {

		// Check if error is MySQL duplicate index error (Error 1061)
		errStr := err.Error()
		if strings.Contains(errStr, "Error 1061") && strings.Contains(errStr, "Duplicate key name") {
			logger.Info("Index already exists, skipping", zap.String("error", errStr))
			return nil
		}
		return fmt.Errorf("failed to add unique index on email: %w", err)
	}

	return nil
}

// modifyColumnTypes modifies column types that need explicit ALTER statements
func modifyColumnTypes(db *gorm.DB) error {
	// Helper function to check if column exists
	columnExists := func(table, column string) bool {
		var count int64
		db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS 
			WHERE TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&count)
		return count > 0
	}

	// dialog.top_k: ensure it's INTEGER with default 1024
	if db.Migrator().HasTable("dialog") && columnExists("dialog", "top_k") {
		if err := db.Exec(`ALTER TABLE dialog MODIFY COLUMN top_k BIGINT NOT NULL DEFAULT 1024`).Error; err != nil {
			logger.Warn("Failed to modify dialog.top_k", zap.Error(err))
		}
	}

	// tenant_llm.api_key: ensure it's TEXT type
	if db.Migrator().HasTable("tenant_llm") && columnExists("tenant_llm", "api_key") {
		if err := db.Exec(`ALTER TABLE tenant_llm MODIFY COLUMN api_key LONGTEXT`).Error; err != nil {
			logger.Warn("Failed to modify tenant_llm.api_key", zap.Error(err))
		}
	}

	// api_token.dialog_id: ensure it's varchar(32)
	if db.Migrator().HasTable("api_token") && columnExists("api_token", "dialog_id") {
		if err := db.Exec(`ALTER TABLE api_token MODIFY COLUMN dialog_id VARCHAR(32)`).Error; err != nil {
			logger.Warn("Failed to modify api_token.dialog_id", zap.Error(err))
		}
	}

	// canvas_template.title and description: ensure they're LONGTEXT type (same as Python JSONField)
	// Note: Python's JSONField uses null=True with application-level default, not database DEFAULT
	if db.Migrator().HasTable("canvas_template") {
		if columnExists("canvas_template", "title") {
			if err := db.Exec(`ALTER TABLE canvas_template MODIFY COLUMN title LONGTEXT NULL`).Error; err != nil {
				logger.Warn("Failed to modify canvas_template.title", zap.Error(err))
			}
		}
		if columnExists("canvas_template", "description") {
			if err := db.Exec(`ALTER TABLE canvas_template MODIFY COLUMN description LONGTEXT NULL`).Error; err != nil {
				logger.Warn("Failed to modify canvas_template.description", zap.Error(err))
			}
		}
	}

	// system_settings.value: ensure it's LONGTEXT
	if db.Migrator().HasTable("system_settings") && columnExists("system_settings", "value") {
		if err := db.Exec(`ALTER TABLE system_settings MODIFY COLUMN value LONGTEXT NOT NULL`).Error; err != nil {
			logger.Warn("Failed to modify system_settings.value", zap.Error(err))
		}
	}

	// knowledgebase.raptor_task_finish_at: ensure it's DateTime
	if db.Migrator().HasTable("knowledgebase") && columnExists("knowledgebase", "raptor_task_finish_at") {
		if err := db.Exec(`ALTER TABLE knowledgebase MODIFY COLUMN raptor_task_finish_at DATETIME`).Error; err != nil {
			logger.Warn("Failed to modify knowledgebase.raptor_task_finish_at", zap.Error(err))
		}
	}

	// knowledgebase.mindmap_task_finish_at: ensure it's DateTime
	if db.Migrator().HasTable("knowledgebase") && columnExists("knowledgebase", "mindmap_task_finish_at") {
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

	// Helper to check if column exists
	columnExists := func(column string) bool {
		var count int64
		db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS 
			WHERE TABLE_NAME = ? AND COLUMN_NAME = ?`, tableName, column).Scan(&count)
		return count > 0
	}

	// Check if old column exists
	if !columnExists(oldName) {
		return nil
	}

	// Check if new column already exists
	if columnExists(newName) {
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

	// Check if column exists using raw SQL
	var count int64
	db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_NAME = ? AND COLUMN_NAME = ?`, tableName, columnName).Scan(&count)
	if count > 0 {
		return nil
	}

	logger.Info("Adding column",
		zap.String("table", tableName),
		zap.String("column", columnName))
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)
	return db.Exec(sql).Error
}

// migrateSkillSearchTables creates skill search related tables
func migrateSkillSearchTables(db *gorm.DB) error {
	// Create skill_search_configs table only
	if !db.Migrator().HasTable("skill_search_configs") {
		logger.Info("Creating skill_search_configs table...")
		sql := `
		CREATE TABLE IF NOT EXISTS skill_search_configs (
			id VARCHAR(32) PRIMARY KEY,
			tenant_id VARCHAR(32) NOT NULL,
			space_id VARCHAR(128) NOT NULL DEFAULT 'default',
			embd_id VARCHAR(128) NOT NULL,
			vector_similarity_weight FLOAT DEFAULT 0.3,
			similarity_threshold FLOAT DEFAULT 0.2,
			field_config JSON,
			rerank_id VARCHAR(128),
			tenant_rerank_id BIGINT,
			top_k BIGINT DEFAULT 10,
			index_version VARCHAR(32) DEFAULT '1.0.0',
			status VARCHAR(1) DEFAULT '1',
			create_time BIGINT,
			update_time DATETIME,
			INDEX idx_tenant_id (tenant_id),
			INDEX idx_space_id (space_id),
			UNIQUE INDEX idx_tenant_space_embd (tenant_id, space_id, embd_id)
		)
		`
		if err := db.Exec(sql).Error; err != nil {
			logger.Warn("Failed to create skill_search_configs table with MySQL dialect, trying generic", zap.Error(err))
			if err := db.AutoMigrate(&entity.SkillSearchConfig{}); err != nil {
				return err
			}
			// AutoMigrate doesn't create unique indexes, so create them explicitly
			logger.Info("Creating unique indexes for skill_search_configs...")
			if err := db.Exec(`ALTER TABLE skill_search_configs ADD UNIQUE INDEX idx_tenant_space_embd (tenant_id, space_id, embd_id)`).Error; err != nil {
				return fmt.Errorf("failed to create unique index idx_tenant_space_embd: %w", err)
			}
		}
	} else {
		// Add space_id for existing installations.
		if err := addColumnIfNotExists(db, "skill_search_configs", "space_id", "VARCHAR(128) NOT NULL DEFAULT 'default'"); err != nil {
			return fmt.Errorf("failed to add space_id column to skill_search_configs: %w", err)
		}

		// Drop legacy unique index (tenant_id, embd_id) to allow per-space configs.
		var legacyIndexExists int64
		db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS 
			WHERE TABLE_NAME = 'skill_search_configs' AND INDEX_NAME = 'idx_tenant_embd'`).Scan(&legacyIndexExists)
		if legacyIndexExists > 0 {
			logger.Info("Dropping legacy unique index idx_tenant_embd from skill_search_configs...")
			if err := db.Exec(`ALTER TABLE skill_search_configs DROP INDEX idx_tenant_embd`).Error; err != nil {
				return fmt.Errorf("failed to drop legacy unique index idx_tenant_embd: %w", err)
			}
		}

		// Table exists, check if unique index exists
		var indexExists int64
		db.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS 
			WHERE TABLE_NAME = 'skill_search_configs' AND INDEX_NAME = 'idx_tenant_space_embd'`).Scan(&indexExists)
		if indexExists == 0 {
			logger.Info("Adding unique index idx_tenant_space_embd to skill_search_configs...")
			if err := db.Exec(`ALTER TABLE skill_search_configs 
				ADD UNIQUE INDEX idx_tenant_space_embd (tenant_id, space_id, embd_id)`).Error; err != nil {
				return fmt.Errorf("failed to add unique index idx_tenant_space_embd: %w", err)
			}
		}
	}

	return nil
}

// migrateSkillSpaceTables creates skill space related tables
func migrateSkillSpaceTables(db *gorm.DB) error {
	if !db.Migrator().HasTable("skill_spaces") {
		logger.Info("Creating skill_spaces table...")
		sql := `
		CREATE TABLE IF NOT EXISTS skill_spaces (
			id VARCHAR(32) PRIMARY KEY,
			tenant_id VARCHAR(32) NOT NULL,
			name VARCHAR(128) NOT NULL,
			folder_id VARCHAR(32) NOT NULL,
			description TEXT,
			embd_id VARCHAR(128),
			rerank_id VARCHAR(128),
			top_k INT DEFAULT 10,
			status VARCHAR(1) DEFAULT '1',
			create_time BIGINT,
			update_time DATETIME,
			INDEX idx_tenant_id (tenant_id),
			UNIQUE INDEX idx_tenant_name_status (tenant_id, name, status)
		)
		`
		if err := db.Exec(sql).Error; err != nil {
			logger.Warn("Failed to create skill_spaces table with MySQL dialect, trying generic", zap.Error(err))
			// Try with AutoMigrate as fallback
			if err := db.AutoMigrate(&entity.SkillSpace{}); err != nil {
				return err
			}
			// AutoMigrate doesn't create unique indexes, so create them explicitly
			logger.Info("Creating unique indexes for skill_spaces...")
			if err := db.Exec(`ALTER TABLE skill_spaces ADD UNIQUE INDEX idx_tenant_name_status (tenant_id, name, status)`).Error; err != nil {
				return fmt.Errorf("failed to create unique index idx_tenant_name_status: %w", err)
			}
		}
	} else {
		// Migrate existing table: add status column first, then update index
		if err := addColumnIfNotExists(db, "skill_spaces", "status", "VARCHAR(1) NOT NULL DEFAULT '1'"); err != nil {
			return fmt.Errorf("failed to add status column to skill_spaces: %w", err)
		}
		// Migrate index after status column exists
		if err := migrateSkillSpaceIndex(db); err != nil {
			return fmt.Errorf("failed to migrate skill_space index: %w", err)
		}
	}

	return nil
}

// migrateSkillSpaceIndex migrates the unique index to include status
func migrateSkillSpaceIndex(db *gorm.DB) error {
	// Check if old index exists and drop it
	var oldIndexExists int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS 
		WHERE TABLE_NAME = 'skill_spaces' AND INDEX_NAME = 'idx_tenant_name'
	`).Scan(&oldIndexExists)
	
	if oldIndexExists > 0 {
		logger.Info("Dropping old idx_tenant_name index from skill_spaces...")
		if err := db.Exec(`DROP INDEX idx_tenant_name ON skill_spaces`).Error; err != nil {
			return fmt.Errorf("failed to drop old index idx_tenant_name: %w", err)
		}
	}
	
	// Check if new index exists
	var newIndexExists int64
	db.Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS 
		WHERE TABLE_NAME = 'skill_spaces' AND INDEX_NAME = 'idx_tenant_name_status'
	`).Scan(&newIndexExists)
	
	if newIndexExists == 0 {
		logger.Info("Creating new idx_tenant_name_status index on skill_spaces...")
		if err := db.Exec(`CREATE UNIQUE INDEX idx_tenant_name_status ON skill_spaces(tenant_id, name, status)`).Error; err != nil {
			return fmt.Errorf("failed to create unique index idx_tenant_name_status: %w", err)
		}
	}
	
	return nil
}
