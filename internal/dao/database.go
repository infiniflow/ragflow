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
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"strings"
	"time"

	"ragflow/internal/server"
	"ragflow/internal/server/config"

	"go.uber.org/zap"

	// The vastbase metadata connection speaks lib/pq (DriverName "postgres"),
	// matching the document engine's client; the import registers the
	// database/sql driver regardless of which engines the binary links in,
	// and autoMigrateSafely classifies its error type.
	"github.com/lib/pq"
	gormLogger "gorm.io/gorm/logger"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

var DB *gorm.DB

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

// newVastbaseDialector builds the metadata-database dialector. lib/pq is the
// driver (DriverName "postgres"), matching the document engine's connection to
// the same instance; WithoutReturning is the peewee returning_clause=False
// escape hatch, off by default because plain RETURNING is how GORM reads back
// auto-increment ids.
//
// The metadata database must run in B (MySQL) compatibility mode — created
// that way by docker/vastbase/init/01_init.sh. B mode parses MySQL syntax
// over the PostgreSQL wire protocol, so every SQL statement the codebase
// issues (DAO queries, migrations, entity-tag DDL types like longtext) stays
// verbatim MySQL; no dialect branching is needed anywhere.
func newVastbaseDialector(cfg config.VastbaseMetaConfig) gorm.Dialector {
	return postgres.New(postgres.Config{
		DriverName:       "postgres",
		DSN:              buildVastbaseMetaDSN(cfg),
		WithoutReturning: cfg.DisableReturning,
	})
}

// isLocalMetaDBHost reports whether the metadata host is loopback or a
// unix-socket path — the only addresses allowed to default to plaintext.
func isLocalMetaDBHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return strings.HasPrefix(host, "/")
}

// quoteMetaDSNValue wraps a lib/pq keyword-DSN value in single quotes so
// hosts/users/passwords with special characters cannot break the DSN.
func quoteMetaDSNValue(v string) string {
	r := strings.ReplaceAll(v, `\`, `\\`)
	r = strings.ReplaceAll(r, `'`, `\'`)
	return "'" + r + "'"
}

// buildVastbaseMetaDSN builds the lib/pq keyword DSN for the Vastbase metadata
// database, mirroring the document engine's DSN contract: quoted values, an
// empty ssl_mode resolved by host (disable on loopback, verify-full on every
// other TCP host) and the same connect/statement timeouts. lib/pq forwards
// unrecognized keywords as startup parameters, which is how statement_timeout
// reaches the server.
func buildVastbaseMetaDSN(cfg config.VastbaseMetaConfig) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		if isLocalMetaDBHost(cfg.Host) {
			sslMode = "disable"
		} else {
			sslMode = "verify-full"
		}
	}
	parts := []string{
		fmt.Sprintf("host=%s", quoteMetaDSNValue(cfg.Host)),
		fmt.Sprintf("port=%d", cfg.Port),
		fmt.Sprintf("user=%s", quoteMetaDSNValue(cfg.User)),
		fmt.Sprintf("dbname=%s", quoteMetaDSNValue(cfg.DatabaseName)),
		fmt.Sprintf("sslmode=%s", quoteMetaDSNValue(sslMode)),
	}
	if cfg.Password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", quoteMetaDSNValue(cfg.Password)))
	}
	parts = append(parts, "connect_timeout=30", "statement_timeout=300000")
	return strings.Join(parts, " ")
}

// autoMigrateModels lists every entity AutoMigrate converges, as one source
// for InitDB and the dialect shape tests.
func autoMigrateModels() []interface{} {
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
		&entity.TenantModelProvider{},
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
	dataModels = append(dataModels, permissionDataModelsEE()...)
	return dataModels
}

// InitDB initialize database connection
func InitDB(ctx context.Context, migrateDB bool) error {
	globalConfig := server.GetConfig()

	// The metadata database dialect: mysql/oceanbase share the MySQL wire
	// path, vastbase speaks PostgreSQL.
	var dialector gorm.Dialector
	var maxConnections, staleTimeout int
	switch globalConfig.DatabaseType() {
	case "vastbase":
		vastbaseConfig := globalConfig.GetVastbaseMetaConfig()
		dialector = newVastbaseDialector(vastbaseConfig)
		maxConnections = vastbaseConfig.MaxConnections
		staleTimeout = vastbaseConfig.StaleTimeout
	default:
		databaseConfig := globalConfig.GetMySQLConfig()
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
			databaseConfig.User,
			databaseConfig.Password,
			databaseConfig.Host,
			databaseConfig.Port,
			databaseConfig.DatabaseName,
			databaseConfig.Charset,
		)
		dialector = newMigrationAwareDialector(dsn)
		maxConnections = databaseConfig.MaxConnections
		staleTimeout = databaseConfig.StaleTimeout
	}

	// Set log level
	var gormLogLevel gormLogger.LogLevel
	if globalConfig.GetMode() == "debug" {
		gormLogLevel = gormLogger.Info
	} else {
		gormLogLevel = gormLogger.Silent
	}

	// Connect to database
	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
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
	sqlDB.SetMaxIdleConns(maxConnections)
	sqlDB.SetMaxOpenConns(maxConnections)
	sqlDB.SetConnMaxLifetime(time.Duration(staleTimeout) * time.Second)

	// Auto migrate all dataModels
	dataModels := autoMigrateModels()

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
		if err = migrateUserTenantDuplicates(ctx, DB); err != nil {
			return fmt.Errorf("failed to deduplicate user_tenant memberships: %w", err)
		}

		common.Info("Migrating database schema...")
		for _, m := range dataModels {
			if err = autoMigrateSafely(ctx, DB, m); err != nil {
				return fmt.Errorf("failed to migrate model %T: %w", m, err)
			}
		}

		// Split the conversation message and reference payloads out of their
		// parent tables. It has to run after AutoMigrate, which unlike
		// RunMigrations creates the child tables this backfill writes to.
		if err = migrateConversationHistory(ctx, DB); err != nil {
			return fmt.Errorf("failed to migrate conversation history: %w", err)
		}
		// Run ingestion conversion after the conversation split. It invokes the
		// dataset cleanup last because both use the same version marker.
		if err = migrateIngestionParserConfigs(ctx, DB); err != nil {
			return fmt.Errorf("failed to migrate ingestion parser configs: %w", err)
		}

		if err = migrateTenantModelMaxTokens(ctx, DB); err != nil {
			return fmt.Errorf("failed to backfill tenant model max_tokens: %w", err)
		}
		if err = migratePermissions(ctx, DB); err != nil {
			return fmt.Errorf("failed to migrate permissions: %w", err)
		}
		if err = migrateTenantModelProviderUserUniqueIndex(ctx, DB); err != nil {
			return fmt.Errorf("failed to migrate tenant model provider unique index: %w", err)
		}
		common.Info("Database schema migrated successfully")
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
	if err = initializeSystemSettingsEE(ctx, DB); err != nil {
		return fmt.Errorf("failed to initialize enterprise system settings: %w", err)
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

	// PostgreSQL/Vastbase: the same converging conditions surface as lib/pq
	// errors carrying SQLSTATE 42P07 (relation), 42701 (column) and 42710
	// (constraint/object). Matching the typed SQLSTATE rather than an
	// "already exists" text fragment keeps unrelated failures that merely
	// phrase themselves that way from being read as convergence. Verifying
	// that the named object belongs to the model being migrated is not
	// feasible: a re-created index reports the index name, not its table. A
	// blanket "does not exist" match is deliberately absent: it would also
	// hide a bad type name, which is a schema bug, not convergence.
	if isPostgresDuplicateObjectErr(err) {
		common.Warn("Schema object already exists, skipping", zap.String("error", errStr))
		return nil
	}

	return err
}

// isPostgresDuplicateObjectErr reports whether err is one of the
// PostgreSQL-wire SQLSTATEs that mean "the schema object AutoMigrate tried
// to create already exists": 42P07 (relation), 42701 (column) and 42710
// (constraint/object). The metadata connection speaks lib/pq, and gorm's
// postgres error translator maps only 23505/23503/42703, so these driver
// errors reach this check intact.
func isPostgresDuplicateObjectErr(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	switch pqErr.Code {
	case "42P07", "42701", "42710":
		return true
	}
	return false
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
