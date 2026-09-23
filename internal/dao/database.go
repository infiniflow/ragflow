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
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"strings"
	"sync"
	"time"

	"ragflow/internal/server"

	"go.uber.org/zap"
	gormLogger "gorm.io/gorm/logger"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

var DB *gorm.DB
var modelProviderManager *models.ProviderManager
var modelProviderManagerMu sync.Mutex

// migrationAwareDialector hands out a namedIndexMigrator instead of the stock
// one.
//
// Both wrappers embed the concrete driver types, not gorm's interfaces. Embedding
// gorm.Dialector or gorm.Migrator promotes only the methods those interfaces
// declare, and gorm reaches capabilities such as ErrorTranslator,
// SavePointerDialectorInterface and BuildIndexOptionsInterface by type
// assertion on db.Dialector and db.Migrator() instead of through the
// interfaces. The storage driver exposes twenty migrator methods beyond
// gorm.Migrator, so forwarding them by hand loses one on every upgrade --
// usually surfacing as a runtime panic mid-migration.
type migrationAwareDialector struct {
	*mysql.Dialector
}

// The capabilities gorm discovers by type assertion on db.Dialector.
var (
	_ gorm.Dialector                     = migrationAwareDialector{}
	_ gorm.ErrorTranslator               = migrationAwareDialector{}
	_ gorm.SavePointerDialectorInterface = migrationAwareDialector{}
)

func newMigrationAwareDialector(dsn string) gorm.Dialector {
	base := mysql.Open(dsn)
	if dialector, ok := base.(*mysql.Dialector); ok {
		return migrationAwareDialector{Dialector: dialector}
	}
	return base
}

func (d migrationAwareDialector) Migrator(db *gorm.DB) gorm.Migrator {
	migrator, ok := d.Dialector.Migrator(db).(mysql.Migrator)
	if !ok {
		// An unsupported driver shape degrades to the stock behaviour rather
		// than panicking: a foreign migrator means we get the redundant drops
		// back, which is what shipped before this wrapper existed.
		return d.Dialector.Migrator(db)
	}
	return namedIndexMigrator{Migrator: migrator}
}

// namedIndexMigrator leaves uniqueness to the named indexes declared with
// uniqueIndex tags and created by the manual migrations.
type namedIndexMigrator struct {
	mysql.Migrator
}

var _ migrator.BuildIndexOptionsInterface = namedIndexMigrator{}

// MigrateColumnUnique drops a unique constraint only once it exists. The stock
// implementation equates "this column carries some single-column UNIQUE index"
// with "this column carries a UNIQUE constraint", derives the matching default
// name (uni_<table>_<column>) and drops it. Our named indexes are created as
// indexes and never under that name, so the DROP always targets a missing
// object and MySQL answers 1091 -- once per column, on every startup. The
// add-constraint branch is left alone: it names its own object, so it cannot
// hit the same mismatch.
// phantomUniqueDrop reports whether the stock migrator is about to drop a
// unique constraint this schema never created. See MigrateColumnUnique.
func phantomUniqueDrop(field *schema.Field, columnType gorm.ColumnType) bool {
	unique, _ := columnType.Unique()
	return unique && !field.Unique
}

func (m namedIndexMigrator) MigrateColumnUnique(dst interface{}, field *schema.Field, columnType gorm.ColumnType) error {
	if phantomUniqueDrop(field, columnType) {
		return nil
	}
	return m.Migrator.MigrateColumnUnique(dst, field, columnType)
}

// LLMFactoryConfig represents a single LLM factory configuration
type LLMFactoryConfig struct {
	Name   string      `json:"name"`
	Logo   string      `json:"logo"`
	Tags   string      `json:"tags"`
	Status string      `json:"status"`
	Rank   string      `json:"rank"`
	LLM    []LLMConfig `json:"llm"`
}

// LLMConfig represents a single LLM model configuration
type LLMConfig struct {
	LLMName   string `json:"llm_name"`
	Tags      string `json:"tags"`
	MaxTokens int64  `json:"max_tokens"`
	ModelType string `json:"model_type"`
	IsTools   bool   `json:"is_tools"`
}

// LLMFactoriesFile represents the structure of llm_factories.json
type LLMFactoriesFile struct {
	FactoryLLMInfos []LLMFactoryConfig `json:"factory_llm_infos"`
}

// InitDB initialize database connection
func InitDB(ctx context.Context, migrateDB bool) error {
	globalConfig := server.GetConfig()
	databaseConfig := globalConfig.GetMySQLConfig()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		databaseConfig.User,
		databaseConfig.Password,
		databaseConfig.Host,
		databaseConfig.Port,
		databaseConfig.DatabaseName,
		databaseConfig.Charset,
	)

	// Set log level
	var gormLogLevel gormLogger.LogLevel
	if globalConfig.GetMode() == "debug" {
		gormLogLevel = gormLogger.Info
	} else {
		gormLogLevel = gormLogger.Silent
	}

	// Connect to database
	var err error
	DB, err = gorm.Open(newMigrationAwareDialector(dsn), &gorm.Config{
		Logger: gormLogger.Default.LogMode(gormLogLevel),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
		TranslateError: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// Get general database object sql.DB
	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool
	sqlDB.SetMaxIdleConns(databaseConfig.MaxConnections)
	sqlDB.SetMaxOpenConns(databaseConfig.MaxConnections)
	sqlDB.SetConnMaxLifetime(time.Duration(databaseConfig.StaleTimeout) * time.Second)

	// Auto migrate all dataModels
	dataModels := []interface{}{
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.File{},
		&entity.File2Document{},
		&entity.TenantLLM{},
		&entity.Chat{},
		&entity.ChatChannel{},
		&entity.ChatSession{},
		&entity.ConversationMessage{},
		&entity.ConversationReference{},
		&entity.Task{},
		&entity.APIToken{},
		&entity.API4Conversation{},
		&entity.API4ConversationMessage{},
		&entity.API4ConversationReference{},
		&entity.Knowledgebase{},
		&entity.InvitationCode{},
		&entity.Document{},
		&entity.UserCanvas{},
		&entity.CanvasTemplate{},
		&entity.UserCanvasVersion{},
		&entity.LLMFactories{},
		&entity.LLM{},
		&entity.TenantLangfuse{},
		&entity.SystemSettings{},
		&entity.Connector{},
		&entity.Connector2Kb{},
		&entity.SyncLogs{},
		&entity.MCPServer{},
		&entity.Memory{},
		&entity.MemoryTask{},
		&entity.Search{},
		&entity.PipelineOperationLog{},
		&entity.EvaluationDataset{},
		&entity.EvaluationCase{},
		&entity.EvaluationRun{},
		&entity.EvaluationResult{},
		&entity.TimeRecord{},
		&entity.License{},
		&entity.SkillSearchConfig{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.TenantModelGroupMapping{},
		&entity.TenantModelProvider{},
		&entity.TenantModelGroup{},
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.FileCommit{},
		&entity.FileCommitItem{},
		&entity.KnowledgeCompileDataset{},
		&entity.WikiDocumentDirty{},
		// Knowledge-compile compilation templates and their groups. The Go
		// KnowledgeCompilerComponent resolves a compilation_template (or group)
		// from these tables at runtime, so the Go side must guarantee they exist.
		&entity.CompilationTemplate{},
		&entity.CompilationTemplateGroup{},
	}

	if migrateDB {
		// Mirror the Python flow, where tools/scripts/run_migrations.sh runs before
		// the ORM creates and converges the schema: the manual migrations have to see
		// the legacy tables as they are. Running them after AutoMigrate would let
		// AutoMigrate rewrite tenant_model.model_type from text to int before the
		// model_type_merge step can read what the Python migration wrote.
		if err = RunMigrations(ctx, DB); err != nil {
			return fmt.Errorf("failed to run manual migrations: %w", err)
		}
		if err = migrateIngestionLogRunIdentity(ctx, DB); err != nil {
			return err
		}

		common.Info("Migrating database schema...")
		for _, m := range dataModels {
			if err = autoMigrateSafely(ctx, DB, m); err != nil {
				return fmt.Errorf("failed to migrate model %T: %w", m, err)
			}
		}
		common.Info("Database schema migrated successfully")

		// Split the conversation message and reference payloads out of their
		// parent tables. It has to run after AutoMigrate, which unlike
		// RunMigrations creates the child tables this backfill writes to.
		if err = migrateConversationHistory(ctx, DB); err != nil {
			return fmt.Errorf("failed to migrate conversation history: %w", err)
		}
	} else {
		if err = migrateIngestionLogRunIdentity(ctx, DB); err != nil {
			return err
		}
		// Ensure the Go-exclusive runtime tables exist. The manual migrations are
		// performed by the standalone --migrate action, so a server-mode process
		// only converges the tables it needs itself.
		if err = autoMigrateRuntimeModels(ctx, DB); err != nil {
			return fmt.Errorf("failed to auto-migrate runtime models: %w", err)
		}
	}
	// Conversation lists filter by dialog and usually order by update time.
	for _, table := range []string{"conversation", "api_4_conversation"} {
		indexName := "idx_" + table + "_dialog_updated"
		if !DB.WithContext(ctx).Migrator().HasIndex(table, indexName) {
			if err = DB.WithContext(ctx).Exec("CREATE INDEX " + indexName + " ON " + table + " (dialog_id, update_time, id)").Error; err != nil {
				common.Warn("Failed to create conversation list index", zap.String("table", table), zap.Error(err))
			}
		}
	}
	// ingestion_task.pipeline_log_id cannot be added by AutoMigrate (see the
	// helper for why), and every ingestion_task query selects all columns, so a
	// missing column fails the whole API with Error 1054. Ensure it on both
	// startup paths rather than trusting AutoMigrate.
	if err = migrateIngestionTaskPipelineLogID(ctx, DB); err != nil {
		return err
	}
	// Seed built-in agent templates so the Go backend can serve the
	// "create agent from template" catalogue without relying on Python-side
	// initialization.
	if err = SeedCanvasTemplates(ctx, DB); err != nil {
		common.Warn("Failed to seed canvas templates", zap.Error(err))
	}
	common.Info("Database connected and migrated successfully")

	err = models.InitProviderManager("conf/models")
	if err != nil {
		common.Fatal("Failed to load model providers", zap.Error(err))
	}

	modelProviderManager = models.GetProviderManager()
	common.Info("Model providers loaded successfully")

	return nil
}

// GetDB get database instance
func GetDB() *gorm.DB {
	return DB
}

// GetModelProviderManager get database instance
func GetModelProviderManager() *models.ProviderManager {
	if modelProviderManager != nil {
		return modelProviderManager
	}

	modelProviderManagerMu.Lock()
	defer modelProviderManagerMu.Unlock()
	if modelProviderManager != nil {
		return modelProviderManager
	}
	if existing := models.GetProviderManager(); existing != nil {
		modelProviderManager = existing
		return modelProviderManager
	}
	modelConfigDir, err := findModelConfigDir()
	if err != nil {
		common.Fatal("Failed to locate model providers", zap.Error(err))
	}
	if err = models.InitProviderManager(modelConfigDir); err != nil {
		common.Fatal("Failed to load model providers", zap.Error(err))
	}
	modelProviderManager = models.GetProviderManager()
	return modelProviderManager
}

func findModelConfigDir() (string, error) {
	candidates := []string{
		"conf/models",
		filepath.Join("..", "..", "conf", "models"),
		filepath.Join("..", "..", "..", "conf", "models"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("conf/models not found")
}

// autoMigrateSafely runs AutoMigrate and ignores duplicate index errors
// This handles cases where indexes already exist (e.g., created by Python backend)
func autoMigrateSafely(ctx context.Context, db *gorm.DB, model interface{}) error {
	//err := db.Debug().AutoMigrate(model) // to print debug info
	err := db.WithContext(ctx).AutoMigrate(model)
	if err == nil {
		return nil
	}

	// Check if error is MySQL duplicate index error (Error 1061)
	errStr := err.Error()
	if strings.Contains(errStr, "Error 1061") && strings.Contains(errStr, "Duplicate key name") {
		common.Warn("Index already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1060") && strings.Contains(errStr, "Duplicate column name") {
		common.Warn("Column already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1050") && strings.Contains(errStr, "Table") {
		common.Warn("Table already exists, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1091") && strings.Contains(errStr, "Can't DROP") {
		common.Warn("Index/column already dropped, skipping", zap.String("error", errStr))
		return nil
	}

	if strings.Contains(errStr, "Error 1138") && strings.Contains(errStr, "Invalid use of NULL") {
		common.Warn("NULL value in existing rows, skipping migration change", zap.String("error", errStr))
		return nil
	}

	return err
}

// autoMigrateRuntimeModels ensures the Go-exclusive runtime tables exist. The
// manual migrations run as the standalone --migrate action, so a server-mode
// process never runs them itself.
func autoMigrateRuntimeModels(ctx context.Context, db *gorm.DB) error {
	goRuntimeModels := []interface{}{
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.MemoryTask{},
		&entity.ConversationMessage{},
		&entity.ConversationReference{},
		&entity.API4ConversationMessage{},
		&entity.API4ConversationReference{},
	}
	for _, m := range goRuntimeModels {
		if err := autoMigrateSafely(ctx, db, m); err != nil {
			tableName := fmt.Sprintf("%T", m)
			if named, ok := m.(interface{ TableName() string }); ok {
				tableName = named.TableName()
			}
			return fmt.Errorf("failed to auto-migrate runtime table %s: %w", tableName, err)
		}
	}
	return nil
}
