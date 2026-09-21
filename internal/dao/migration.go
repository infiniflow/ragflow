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
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RunMigrations runs all manual database migrations
// These are migrations that cannot be handled by AutoMigrate alone
func RunMigrations(ctx context.Context, db *gorm.DB) error {
	// Check if tenant_llm table has composite primary key and migrate to ID primary key
	if err := migrateTenantLLMPrimaryKey(ctx, db); err != nil {
		return fmt.Errorf("failed to migrate tenant_llm primary key: %w", err)
	}

	// Rename columns (correct typos)
	if err := renameColumnIfExists(ctx, db, "task", "process_duation", "process_duration"); err != nil {
		return fmt.Errorf("failed to rename task.process_duation: %w", err)
	}
	if err := renameColumnIfExists(ctx, db, "document", "process_duation", "process_duration"); err != nil {
		return fmt.Errorf("failed to rename document.process_duation: %w", err)
	}

	// Add unique index on user.email
	if err := migrateAddUniqueEmail(ctx, db); err != nil {
		return fmt.Errorf("failed to add unique index on user.email: %w", err)
	}

	// Add unique index on ingestion_task.document_id
	if err := migrateIngestionTaskDocumentIDUnique(ctx, db); err != nil {
		return fmt.Errorf("failed to add unique index on ingestion_task.document_id: %w", err)
	}

	// Modify column types that AutoMigrate may not handle correctly
	if err := modifyColumnTypes(ctx, db); err != nil {
		return fmt.Errorf("failed to modify column types: %w", err)
	}

	// Add case-insensitive unique constraint on knowledgebase (tenant_id, name)
	if err := migrateKnowledgebaseNameUnique(ctx, db); err != nil {
		return fmt.Errorf("failed to add unique index on knowledgebase (tenant_id, name): %w", err)
	}

	// Add unique constraint on user_canvas (user_id, canvas_category, title)
	if err := migrateUserCanvasTitleUnique(ctx, db); err != nil {
		return fmt.Errorf("failed to add unique index on user_canvas (user_id, canvas_category, title): %w", err)
	}

	if err := migrateGeneralChunkerParserConfigs(ctx, db); err != nil {
		return fmt.Errorf("failed to migrate general chunker parser configs: %w", err)
	}

	// Backfill the tenant model tables from the legacy tenant_llm table.
	if err := migrateModelData(ctx, db); err != nil {
		return fmt.Errorf("failed to migrate tenant model data: %w", err)
	}

	common.Info("All manual migrations completed successfully")
	return nil
}

// migrateTenantLLMPrimaryKey migrates tenant_llm from its legacy composite
// primary key (tenant_id, llm_factory, llm_name) to a surrogate auto-increment
// "id" column.
//
// MySQL allows a single PRIMARY KEY per table, so the composite key has to be
// dropped before the new key column can be promoted. The legacy uniqueness
// guarantee is preserved by a unique index on the old key columns.
func migrateTenantLLMPrimaryKey(ctx context.Context, db *gorm.DB) error {
	if !db.WithContext(ctx).Migrator().HasTable("tenant_llm") {
		return nil
	}

	// Idempotency: an auto_increment "id" means the table is already migrated.
	idIsAutoIncrement, err := isAutoIncrementColumn(ctx, db, "tenant_llm", "id")
	if err != nil {
		return err
	}
	if idIsAutoIncrement {
		return nil
	}

	common.Info("Migrating tenant_llm to use ID primary key...")

	// MySQL commits DDL implicitly, so this transaction groups the statements
	// rather than making them atomic. Every step is therefore written to be
	// re-runnable after an interrupted attempt.
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A leftover temp_id can only come from an interrupted run.
		if err := dropColumnIfExists(ctx, tx, "tenant_llm", "temp_id"); err != nil {
			return err
		}

		idExists, err := columnExists(ctx, tx, "tenant_llm", "id")
		if err != nil {
			return err
		}

		if idExists {
			alreadyKey, err := dropLegacyPrimaryKey(ctx, tx, "tenant_llm", "id")
			if err != nil {
				return err
			}
			// Re-declaring the primary key on a column that already holds it is
			// rejected as a second primary key definition.
			definition := "BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY"
			if alreadyKey {
				definition = "BIGINT NOT NULL AUTO_INCREMENT"
			}
			if err = tx.Exec("ALTER TABLE tenant_llm MODIFY COLUMN id " + definition).Error; err != nil {
				return fmt.Errorf("failed to make tenant_llm.id auto_increment: %w", err)
			}
		} else {
			if err = tx.Exec(`ALTER TABLE tenant_llm ADD COLUMN temp_id BIGINT NULL`).Error; err != nil {
				return fmt.Errorf("failed to add temp_id column: %w", err)
			}
			// Number the rows explicitly instead of relying on assignment order,
			// so the resulting primary key is stable across runs.
			if err = tx.Exec(`SET @ragflow_tenant_llm_row = 0`).Error; err != nil {
				return fmt.Errorf("failed to initialize the row counter: %w", err)
			}
			if err = tx.Exec(`UPDATE tenant_llm
				SET temp_id = (@ragflow_tenant_llm_row := @ragflow_tenant_llm_row + 1)
				ORDER BY tenant_id, llm_factory, llm_name`).Error; err != nil {
				return fmt.Errorf("failed to number tenant_llm rows: %w", err)
			}
			if _, err = dropLegacyPrimaryKey(ctx, tx, "tenant_llm", "temp_id"); err != nil {
				return err
			}
			if err = tx.Exec(`ALTER TABLE tenant_llm
				MODIFY COLUMN temp_id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY`).Error; err != nil {
				return fmt.Errorf("failed to make temp_id the primary key: %w", err)
			}
		}

		// Preserve the uniqueness contract of the legacy composite primary key.
		legacyKey := []string{"tenant_id", "llm_factory", "llm_name"}
		hasUnique, err := hasUniqueIndex(ctx, tx, "tenant_llm", legacyKey)
		if err != nil {
			return err
		}
		if !hasUnique {
			if err = addUniqueIndex(ctx, tx, "tenant_llm", "uk_tenant_llm", legacyKey); err != nil {
				return err
			}
		}

		if !idExists {
			if err = tx.Exec(`ALTER TABLE tenant_llm RENAME COLUMN temp_id TO id`).Error; err != nil {
				return fmt.Errorf("failed to rename temp_id to id: %w", err)
			}
		}

		common.Info("tenant_llm primary key migration completed")
		return nil
	})
}

// migrateAddUniqueEmail enforces uniqueness on user.email.
//
// Rows that already share an address are resolved before the index is created:
// the superuser - or the oldest account when the address has none - keeps it,
// and the remaining rows are renamed to "<email>_DUPLICATE_<id prefix>".
// Renaming is what makes the index creatable on installations that accumulated
// duplicates before the constraint existed.
func migrateAddUniqueEmail(ctx context.Context, db *gorm.DB) error {
	if !db.WithContext(ctx).Migrator().HasTable("user") {
		return nil
	}

	hasUnique, err := hasUniqueIndex(ctx, db, "user", []string{"email"})
	if err != nil {
		return err
	}
	if hasUnique {
		return nil
	}

	if err = renameDuplicateEmails(ctx, db); err != nil {
		return err
	}

	// Reaching this point means any existing index on the address is not unique,
	// and a same-named non-unique index would block the unique one.
	if err = dropIndexIfExists(ctx, db, "user", "idx_user_email"); err != nil {
		return err
	}

	common.Info("Adding unique index on user.email...")
	return addUniqueIndex(ctx, db, "user", "idx_user_email", []string{"email"})
}

func migrateIngestionTaskDocumentIDUnique(ctx context.Context, db *gorm.DB) error {
	if !db.WithContext(ctx).Migrator().HasTable("ingestion_task") {
		return nil
	}

	const indexName = "idx_ingestion_task_document_id"
	columns := []string{"document_id"}

	hasUnique, err := hasUniqueIndex(ctx, db, "ingestion_task", columns)
	if err != nil {
		return err
	}
	if hasUnique {
		return nil
	}

	var duplicateCount int64
	if err = db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT document_id FROM ingestion_task GROUP BY document_id HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&duplicateCount).Error; err != nil {
		return err
	}
	if duplicateCount > 0 {
		common.Warn("Found duplicate document_id values in ingestion_task, cannot add unique index", zap.Int64("count", duplicateCount))
		return nil
	}

	// A non-unique index under the same name would block the unique one.
	if err = dropIndexIfExists(ctx, db, "ingestion_task", indexName); err != nil {
		return err
	}

	return addUniqueIndex(ctx, db, "ingestion_task", indexName, columns)
}

// migrateIngestionTaskPipelineLogID adds ingestion_task.pipeline_log_id when
// the column is missing.
//
// It cannot be left to AutoMigrate. The model declares document_id's unique key
// as a named unique index (uniqueIndex:idx_ingestion_task_document_id), while
// the column is UNIQUE at the database level. GORM's MigrateColumnUnique reads
// that pairing as a stray unique constraint: it issues
// `ALTER TABLE ingestion_task DROP FOREIGN KEY uni_ingestion_task_document_id`
// for the default constraint name, which does not exist, so MySQL fails with
// 1091. AutoMigrate aborts on the first error, and because autoMigrateSafely
// treats 1091 as a benign "already dropped" case, the failure is swallowed and
// the migration reports success -- before ever reaching the missing column.
// A column added to this table would therefore never be created, and every
// ingestion_task query (the DAO selects all columns) would fail with
// Error 1054.
func migrateIngestionTaskPipelineLogID(ctx context.Context, db *gorm.DB) error {
	// Use the model, not the bare table name: HasColumn resolves the field
	// against the statement schema and dereferences it, which a plain string
	// does not provide (the SQLite migrator tolerates it, the MySQL one panics).
	migrator := db.WithContext(ctx).Migrator()
	if !migrator.HasTable(&entity.IngestionTask{}) {
		return nil
	}
	if migrator.HasColumn(&entity.IngestionTask{}, "pipeline_log_id") {
		return nil
	}
	if err := db.WithContext(ctx).Exec(
		"ALTER TABLE ingestion_task ADD COLUMN pipeline_log_id varchar(32) NULL",
	).Error; err != nil {
		if strings.Contains(err.Error(), "Error 1060") && strings.Contains(err.Error(), "Duplicate column name") {
			return nil
		}
		return fmt.Errorf("failed to add ingestion_task.pipeline_log_id: %w", err)
	}
	common.Info("Added ingestion_task.pipeline_log_id")
	return nil
}

// migrateIngestionLogRunIdentity adds the columns and indexes that make each
// ingestion event belong to one immutable pipeline-operation-log run. It is
// deliberately explicit: the runtime startup path auto-migrates ingestion
// tables and cannot be trusted to converge an existing schema safely.
func migrateIngestionLogRunIdentity(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	if migrator.HasTable(&entity.PipelineOperationLog{}) {
		if !migrator.HasColumn(&entity.PipelineOperationLog{}, "run_count") {
			if err := db.WithContext(ctx).Exec(
				"ALTER TABLE pipeline_operation_log ADD COLUMN run_count int NULL",
			).Error; err != nil && !isDuplicateColumnError(err) {
				return fmt.Errorf("add pipeline_operation_log.run_count: %w", err)
			}
		}
		if err := createIngestionLogIndexIfMissing(migrator, &entity.PipelineOperationLog{}, "idx_pipeline_operation_log_document_run", "add pipeline_operation_log document/run index"); err != nil {
			return err
		}
	}

	if migrator.HasTable(&entity.IngestionTaskLog{}) {
		if !migrator.HasColumn(&entity.IngestionTaskLog{}, "pipeline_log_id") {
			if err := db.WithContext(ctx).Exec(
				"ALTER TABLE ingestion_task_log ADD COLUMN pipeline_log_id varchar(32) NULL",
			).Error; err != nil && !isDuplicateColumnError(err) {
				return fmt.Errorf("add ingestion_task_log.pipeline_log_id: %w", err)
			}
		}
		if !migrator.HasColumn(&entity.IngestionTaskLog{}, "event_type") {
			if err := db.WithContext(ctx).Exec(
				"ALTER TABLE ingestion_task_log ADD COLUMN event_type tinyint NOT NULL DEFAULT 4",
			).Error; err != nil && !isDuplicateColumnError(err) {
				return fmt.Errorf("add ingestion_task_log.event_type: %w", err)
			}
		}
		if err := createIngestionLogIndexIfMissing(migrator, &entity.IngestionTaskLog{}, "idx_ingestion_task_log_pipeline_id", "add ingestion_task_log pipeline index"); err != nil {
			return err
		}
	}
	return nil
}

type ingestionLogSchemaMigrator interface {
	HasIndex(any, string) bool
	CreateIndex(any, string) error
}

func createIngestionLogIndexIfMissing(migrator ingestionLogSchemaMigrator, model any, name, operation string) error {
	if migrator.HasIndex(model, name) {
		return nil
	}
	if err := migrator.CreateIndex(model, name); err != nil && !migrator.HasIndex(model, name) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "Error 1060") && strings.Contains(text, "Duplicate column name")
}

// migrateKnowledgebaseNameUnique adds a case-insensitive unique constraint on
// (tenant_id, name) for valid knowledge bases. A VIRTUAL generated column
// (name_ci) computes LOWER(name) only for status='1' rows and is NULL otherwise,
// so soft-deleted rows never block name reuse. The unique index backstops the
// check-then-write path in CreateDataset/UpdateDataset against concurrent
// duplicate inserts, and the resulting duplicate-key error is mapped back to the
// "already exists" domain error at the service layer.
func migrateKnowledgebaseNameUnique(ctx context.Context, db *gorm.DB) error {
	if !db.WithContext(ctx).Migrator().HasTable("knowledgebase") {
		return nil
	}

	// Add the generated column if it does not exist yet.
	var colExists int64
	if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'knowledgebase' AND COLUMN_NAME = 'name_ci'`).Scan(&colExists).Error; err != nil {
		return err
	}
	if colExists == 0 {
		common.Info("Adding generated column name_ci to knowledgebase...")
		if err := db.WithContext(ctx).Exec(`ALTER TABLE knowledgebase
			ADD COLUMN name_ci VARCHAR(128) GENERATED ALWAYS AS (
				CASE WHEN status = '1' THEN LOWER(name) ELSE NULL END
			) VIRTUAL`).Error; err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "Error 1060") && strings.Contains(errStr, "Duplicate column name") {
				common.Info("Column name_ci already exists, skipping", zap.String("error", errStr))
			} else {
				return fmt.Errorf("failed to add generated column name_ci: %w", err)
			}
		}
	}

	const indexName = "idx_kb_tenant_name_ci"

	// Check whether the unique index already exists.
	var idxExists int64
	if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'knowledgebase' AND INDEX_NAME = ?`, indexName).Scan(&idxExists).Error; err != nil {
		return err
	}
	if idxExists > 0 {
		return nil
	}

	// Check for duplicate valid names before adding the index.
	var duplicateCount int64
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT tenant_id, name_ci FROM knowledgebase
			WHERE name_ci IS NOT NULL
			GROUP BY tenant_id, name_ci HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&duplicateCount).Error; err != nil {
		return err
	}
	if duplicateCount > 0 {
		return fmt.Errorf("found %d duplicate (tenant_id, name) pairs among valid knowledge bases; resolve these before the unique index can be created", duplicateCount)
	}

	common.Info("Adding unique index on knowledgebase (tenant_id, name_ci)...")
	if err := db.WithContext(ctx).Exec("ALTER TABLE knowledgebase ADD UNIQUE INDEX " + indexName + " (tenant_id, name_ci)").Error; err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "Error 1061") && strings.Contains(errStr, "Duplicate key name") {
			common.Info("Index already exists, skipping", zap.String("error", errStr))
			return nil
		}
		return fmt.Errorf("failed to add unique index on knowledgebase (tenant_id, name_ci): %w", err)
	}

	return nil
}

func migrateUserCanvasTitleUnique(ctx context.Context, db *gorm.DB) error {
	if !db.WithContext(ctx).Migrator().HasTable("user_canvas") {
		return nil
	}

	const indexName = "idx_user_canvas_user_category_title"

	var idxExists int64
	if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user_canvas' AND INDEX_NAME = ?`, indexName).Scan(&idxExists).Error; err != nil {
		return err
	}
	if idxExists > 0 {
		return nil
	}

	var duplicateCount int64
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM (
			SELECT user_id, canvas_category, title FROM user_canvas
			WHERE title IS NOT NULL
			GROUP BY user_id, canvas_category, title HAVING COUNT(*) > 1
		) AS duplicates
	`).Scan(&duplicateCount).Error; err != nil {
		return err
	}
	if duplicateCount > 0 {
		return fmt.Errorf("found %d duplicate (user_id, canvas_category, title) groups in user_canvas; resolve these before the unique index can be created", duplicateCount)
	}

	common.Info("Adding unique index on user_canvas (user_id, canvas_category, title)...")
	if err := db.WithContext(ctx).Exec("ALTER TABLE user_canvas ADD UNIQUE INDEX " + indexName + " (user_id, canvas_category, title)").Error; err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "Error 1061") && strings.Contains(errStr, "Duplicate key name") {
			common.Info("Index already exists, skipping", zap.String("error", errStr))
			return nil
		}
		return fmt.Errorf("failed to add unique index on user_canvas (user_id, canvas_category, title): %w", err)
	}

	return nil
}

// modifyColumnTypes aligns columns whose stored type must match the Python
// models in api/db/db_models.py. The target types are also declared on the Go
// entities, so AutoMigrate converges on the same definition and the two paths
// cannot fight over the column.
func modifyColumnTypes(ctx context.Context, db *gorm.DB) error {
	specs := []columnSpec{
		// dialog.top_k mirrors Python's IntegerField(default=1024).
		{
			table: "dialog", column: "top_k",
			columnType: "int", nullable: false,
			definition: "int NOT NULL DEFAULT 1024",
		},
		// tenant_llm.api_key mirrors Python's TextField(null=True).
		{
			table: "tenant_llm", column: "api_key",
			columnType: "text", nullable: true,
			definition: "text",
		},
		// api_token.dialog_id mirrors Python's CharField(max_length=32, null=True).
		{
			table: "api_token", column: "dialog_id",
			columnType: "varchar(32)", nullable: true,
			definition: "varchar(32)",
		},
		// canvas_template.title/description mirror Python's JSONField, which is a
		// nullable LONGTEXT.
		{
			table: "canvas_template", column: "title",
			columnType: "longtext", nullable: true,
			definition: "longtext NULL",
		},
		{
			table: "canvas_template", column: "description",
			columnType: "longtext", nullable: true,
			definition: "longtext NULL",
		},
		// system_settings.value mirrors Python's EmptyStringTextField(null=False).
		{
			table: "system_settings", column: "value",
			columnType: "text", nullable: false,
			definition: "text NOT NULL",
		},
		// knowledgebase task timestamps mirror Python's DateTimeField.
		{
			table: "knowledgebase", column: "raptor_task_finish_at",
			columnType: "datetime", nullable: true,
			definition: "datetime",
		},
		{
			table: "knowledgebase", column: "mindmap_task_finish_at",
			columnType: "datetime", nullable: true,
			definition: "datetime",
		},
	}

	for _, spec := range specs {
		if err := applyColumnSpec(ctx, db, spec); err != nil {
			return err
		}
	}

	return nil
}

// renameColumnIfExists renames a column if it exists and the new column doesn't exist
func renameColumnIfExists(ctx context.Context, db *gorm.DB, tableName, oldName, newName string) error {
	if !db.WithContext(ctx).Migrator().HasTable(tableName) {
		return nil
	}

	oldExists, err := columnExists(ctx, db, tableName, oldName)
	if err != nil {
		return err
	}
	if !oldExists {
		return nil
	}

	newExists, err := columnExists(ctx, db, tableName, newName)
	if err != nil {
		return err
	}
	if newExists {
		// Both exist, drop the old one
		common.Warn("Both old and new columns exist, dropping old one",
			zap.String("table", tableName),
			zap.String("oldColumn", oldName),
			zap.String("newColumn", newName))
		return db.WithContext(ctx).Migrator().DropColumn(tableName, oldName)
	}

	common.Info("Renaming column",
		zap.String("table", tableName),
		zap.String("oldColumn", oldName),
		zap.String("newColumn", newName))
	return db.WithContext(ctx).Migrator().RenameColumn(tableName, oldName, newName)
}

// addColumnIfNotExists adds a column if it doesn't exist
func addColumnIfNotExists(ctx context.Context, db *gorm.DB, tableName, columnName, columnDef string) error {
	if !db.WithContext(ctx).Migrator().HasTable(tableName) {
		return nil
	}

	exists, err := columnExists(ctx, db, tableName, columnName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	common.Info("Adding column",
		zap.String("table", tableName),
		zap.String("column", columnName))
	sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)
	return db.WithContext(ctx).Exec(sql).Error
}

// columnExists reports whether the current database already has the column.
// TABLE_SCHEMA is pinned to DATABASE() so that a MySQL server hosting more than
// one RAGFlow database never matches another schema's tables.
//
// It replaces gorm's Migrator.HasColumn, which panics when the table is passed
// as a string: HasColumn only fills Statement.Table for strings and then
// dereferences the unparsed Statement.Schema.
func columnExists(ctx context.Context, db *gorm.DB, table, column string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
	`, table, column).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// columnDefinition returns the stored COLUMN_TYPE and whether the column accepts
// NULL. ok is false when the column does not exist.
func columnDefinition(ctx context.Context, db *gorm.DB, table, column string) (columnType string, nullable, ok bool, err error) {
	var row struct {
		ColumnType string
		IsNullable string
	}
	if err = db.WithContext(ctx).Raw(`
		SELECT COLUMN_TYPE AS column_type, IS_NULLABLE AS is_nullable
		FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
	`, table, column).Scan(&row).Error; err != nil {
		return "", false, false, err
	}
	return row.ColumnType, strings.EqualFold(row.IsNullable, "YES"), row.ColumnType != "", nil
}

// isAutoIncrementColumn reports whether the column is declared AUTO_INCREMENT.
func isAutoIncrementColumn(ctx context.Context, db *gorm.DB, table, column string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
		  AND EXTRA LIKE '%auto_increment%'
	`, table, column).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// primaryKeyColumns lists the current PRIMARY KEY columns in key order.
func primaryKeyColumns(ctx context.Context, db *gorm.DB, table string) ([]string, error) {
	var columns []string
	if err := db.WithContext(ctx).Raw(`
		SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
		ORDER BY ORDINAL_POSITION
	`, table).Scan(&columns).Error; err != nil {
		return nil, err
	}
	return columns, nil
}

// dropLegacyPrimaryKey removes the table's PRIMARY KEY unless it is already the
// single-column key being migrated to. MySQL rejects a second PRIMARY KEY
// definition, so the legacy key must be dropped first. It reports whether
// keepColumn is the primary key once it returns.
func dropLegacyPrimaryKey(ctx context.Context, db *gorm.DB, table, keepColumn string) (bool, error) {
	columns, err := primaryKeyColumns(ctx, db, table)
	if err != nil {
		return false, err
	}
	if len(columns) == 1 && columns[0] == keepColumn {
		return true, nil
	}
	if len(columns) == 0 {
		return false, nil
	}
	common.Info("Dropping legacy primary key", zap.String("table", table), zap.Strings("columns", columns))
	if err = db.WithContext(ctx).Exec("ALTER TABLE " + table + " DROP PRIMARY KEY").Error; err != nil {
		return false, fmt.Errorf("failed to drop the legacy primary key on %s: %w", table, err)
	}
	return false, nil
}

// hasUniqueIndex reports whether a unique index spans exactly the given columns.
// Matching on the column set rather than on the index name keeps the migrations
// idempotent when an equivalent index already exists under another name.
func hasUniqueIndex(ctx context.Context, db *gorm.DB, table string, columns []string) (bool, error) {
	// The column names come from this file's call sites, never from input.
	quoted := "'" + strings.Join(columns, "', '") + "'"
	var count int64
	// An index only qualifies when its whole column set is the requested one: a
	// broader unique index such as (email, tenant_id) does not make email unique
	// on its own. Prefix indexes are excluded because they only constrain a
	// leading substring.
	if err := db.WithContext(ctx).Raw(fmt.Sprintf(`
		SELECT COUNT(*) FROM (
			SELECT INDEX_NAME FROM INFORMATION_SCHEMA.STATISTICS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
			  AND NON_UNIQUE = 0 AND INDEX_NAME <> 'PRIMARY'
			  AND SUB_PART IS NULL
			GROUP BY INDEX_NAME
			HAVING COUNT(*) = %d
			   AND COUNT(CASE WHEN COLUMN_NAME IN (%s) THEN 1 END) = %d
		) AS equivalent
	`, len(columns), quoted, len(columns)), table).Scan(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// addUniqueIndex creates a unique index over the given columns.
func addUniqueIndex(ctx context.Context, db *gorm.DB, table, indexName string, columns []string) error {
	sql := fmt.Sprintf("ALTER TABLE %s ADD UNIQUE INDEX %s (%s)", table, indexName, strings.Join(columns, ", "))
	if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
		if isDuplicateIndexErr(err) {
			common.Info("Index already exists, skipping", zap.String("index", indexName))
			return nil
		}
		return fmt.Errorf("failed to add unique index %s on %s: %w", indexName, table, err)
	}
	return nil
}

// dropIndexIfExists removes a named index when it is present.
func dropIndexIfExists(ctx context.Context, db *gorm.DB, table, indexName string) error {
	var count int64
	if err := db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?
	`, table, indexName).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	if err := db.WithContext(ctx).Exec(fmt.Sprintf("ALTER TABLE %s DROP INDEX %s", table, indexName)).Error; err != nil {
		return fmt.Errorf("failed to drop index %s on %s: %w", indexName, table, err)
	}
	return nil
}

// dropColumnIfExists removes a column when it is present.
func dropColumnIfExists(ctx context.Context, db *gorm.DB, table, column string) error {
	exists, err := columnExists(ctx, db, table, column)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	common.Info("Dropping column", zap.String("table", table), zap.String("column", column))
	if err = db.WithContext(ctx).Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)).Error; err != nil {
		return fmt.Errorf("failed to drop column %s on %s: %w", column, table, err)
	}
	return nil
}

// isDuplicateIndexErr reports whether the error is MySQL error 1061 (duplicate
// key name), which means the index already exists.
func isDuplicateIndexErr(err error) bool {
	return strings.Contains(err.Error(), "Error 1061") || strings.Contains(err.Error(), "Duplicate key name")
}

// renameDuplicateEmails renames every user row that shares an address with
// another row, leaving the address on the superuser - or on the oldest row when
// the address has no superuser. The unique index is only creatable once no
// duplicates remain.
func renameDuplicateEmails(ctx context.Context, db *gorm.DB) error {
	var duplicates []string
	if err := db.WithContext(ctx).Raw(`
		SELECT email FROM user GROUP BY email HAVING COUNT(*) > 1
	`).Scan(&duplicates).Error; err != nil {
		return err
	}
	if len(duplicates) == 0 {
		return nil
	}

	common.Warn("Renaming duplicate user emails before adding the unique index", zap.Int("addresses", len(duplicates)))
	for _, email := range duplicates {
		var ids []string
		if err := db.WithContext(ctx).Raw(`
			SELECT id FROM user WHERE email = ?
			ORDER BY is_superuser DESC, create_time ASC
		`, email).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) < 2 {
			continue
		}
		for _, id := range ids[1:] {
			if err := db.WithContext(ctx).Exec(
				`UPDATE user SET email = ? WHERE id = ?`,
				duplicateEmailAddress(email, id), id,
			).Error; err != nil {
				return fmt.Errorf("failed to rename duplicate email %q: %w", email, err)
			}
		}
	}
	return nil
}

// duplicateEmailAddress builds the placeholder address for a shadowed duplicate
// row, clamped to the column width so the rename cannot fail on length.
func duplicateEmailAddress(email, id string) string {
	const (
		maxEmailLength = 255
		duplicateTag   = "_DUPLICATE_"
		idPrefixLength = 8
	)
	if len(id) > idPrefixLength {
		id = id[:idPrefixLength]
	}
	suffix := duplicateTag + id
	runes := []rune(email)
	if keep := maxEmailLength - len(suffix); len(runes) > keep {
		runes = runes[:keep]
	}
	return string(runes) + suffix
}

// integerDisplayWidth matches the deprecated integer display width emitted by
// MySQL 5.7 ("int(11)"), which MySQL 8 accepts but no longer reports.
var integerDisplayWidth = regexp.MustCompile(`^(tinyint|smallint|mediumint|int|bigint)\(\d+\)$`)

// normalizeColumnType strips integer display widths so a stored type compares
// equal to its canonical name.
func normalizeColumnType(columnType string) string {
	lowered := strings.ToLower(strings.TrimSpace(columnType))
	if match := integerDisplayWidth.FindStringSubmatch(lowered); match != nil {
		return match[1]
	}
	return lowered
}

// columnSpec is the definition a column is expected to have.
type columnSpec struct {
	table      string
	column     string
	columnType string // compared against INFORMATION_SCHEMA.COLUMNS.COLUMN_TYPE
	nullable   bool
	definition string // used verbatim when an ALTER is required
}

// applyColumnSpec aligns a column with its expected definition, issuing DDL only
// when the stored definition differs so that a steady-state startup runs no
// ALTER at all.
func applyColumnSpec(ctx context.Context, db *gorm.DB, spec columnSpec) error {
	if !db.WithContext(ctx).Migrator().HasTable(spec.table) {
		return nil
	}

	currentType, nullable, exists, err := columnDefinition(ctx, db, spec.table, spec.column)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if normalizeColumnType(currentType) == normalizeColumnType(spec.columnType) && nullable == spec.nullable {
		return nil
	}

	common.Info("Modifying column type",
		zap.String("table", spec.table),
		zap.String("column", spec.column),
		zap.String("from", currentType),
		zap.String("to", spec.columnType))
	if err = db.WithContext(ctx).Exec(
		fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", spec.table, spec.column, spec.definition),
	).Error; err != nil {
		common.Warn("Failed to modify column",
			zap.String("table", spec.table),
			zap.String("column", spec.column),
			zap.Error(err))
	}
	return nil
}
