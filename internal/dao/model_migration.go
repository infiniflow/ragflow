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
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// This file ports tools/scripts/mysql_migration.py. It backfills the tenant
// model tables (tenant_model_provider, tenant_model_instance, tenant_model)
// from the legacy tenant_llm table, rewrites stored model ids from
// "<model>@<provider>" to "<model>@default@<provider>", and populates the
// tenant_*_id columns. Every step is idempotent, so re-running a stage is safe,
// and the two steps are gated by the database version in system_settings,
// mirroring the two --database-version calls in tools/scripts/run_migrations.sh.

const modelMigrationInstanceName = "default"

// modelTypeStringToBit mirrors MODEL_TYPE_TO_INT in the Python migration and
// entity.ModelType's bit layout.
var modelTypeStringToBit = map[string]int{
	"chat":        1,
	"embedding":   2,
	"asr":         4,
	"speech2text": 4,
	"vision":      8,
	"image2text":  8,
	"rerank":      16,
	"tts":         32,
	"ocr":         64,
}

// ensureTenantModelTables creates the tenant model tables the base step backfills
// when they do not exist yet and the legacy tenant_llm table does. It mirrors the
// CREATE TABLE IF NOT EXISTS that the Python stages run before inserting.
//
// Existing tables are never altered: tenant_model.model_type must keep the text
// shape the Python stage left behind so mergeTenantModelTypes can still read the
// legacy model names instead of the integer AutoMigrate would converge it to.
func ensureTenantModelTables(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("tenant_llm") {
		return nil
	}
	for _, model := range []any{&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}} {
		if scoped.Migrator().HasTable(model) {
			continue
		}
		if err := scoped.Migrator().CreateTable(model); err != nil {
			return err
		}
	}
	return nil
}

// ensureMigrationVersionTable creates system_settings when it is missing. The
// model data migration records its progress there and runs before AutoMigrate
// (see InitDB), so on a database that predates the table the version marker could
// not be written. Every later startup would then replay the migration against
// tenant_model rows that already hold data.
func ensureMigrationVersionTable(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	if scoped.Migrator().HasTable("system_settings") {
		return nil
	}
	return scoped.Migrator().CreateTable(&entity.SystemSettings{})
}

// migrateModelData runs the model related data migrations in the two versioned
// steps used by tools/scripts/run_migrations.sh. Each step records its own
// version in migrationDBVersionMarker, so work already performed by the Python
// script, or by a previous Go run, is skipped.
//
//   - The base step (modelMigrationBaseVersion) rebuilds
//     tenant_model_provider/tenant_model_instance/tenant_model from tenant_llm
//     and normalizes the stored model ids. It is only needed on a database older
//     than v0.26.0.
//   - The follow-up step (modelMigrationTargetVersion) seeds the factory
//     declared models, merges the tenant_model rows into the integer model_type
//     representation and populates the tenant_*_id columns. It is needed on
//     every database older than v0.27.1, including one already at v0.26.0.
//
// The step runs before AutoMigrate (see InitDB), so it creates the tables it
// depends on itself: system_settings for the version marker, and, on a database
// older than v0.26.0, the tenant model tables it backfills from tenant_llm.
func migrateModelData(ctx context.Context, db *gorm.DB) error {
	if err := ensureMigrationVersionTable(ctx, db); err != nil {
		return fmt.Errorf("ensure migration version table: %w", err)
	}
	if err := ensureTenantModelTables(ctx, db); err != nil {
		return fmt.Errorf("ensure tenant model tables: %w", err)
	}

	currentVersion, err := getDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}

	if shouldSkipMigration(currentVersion, modelMigrationBaseVersion) {
		common.Info("Tenant model tables already migrated, skipping base step",
			zap.String("current_version", currentVersion),
			zap.String("step_version", modelMigrationBaseVersion))
	} else {
		common.Info("Running base tenant model table migration",
			zap.String("current_version", currentVersion),
			zap.String("step_version", modelMigrationBaseVersion))

		if err := migrateTenantModelProviders(ctx, db); err != nil {
			return fmt.Errorf("migrate tenant_model_provider: %w", err)
		}
		if err := migrateTenantModelInstances(ctx, db); err != nil {
			return fmt.Errorf("migrate tenant_model_instance: %w", err)
		}
		if err := migrateTenantModels(ctx, db); err != nil {
			return fmt.Errorf("migrate tenant_model: %w", err)
		}
		if err := normalizeStoredModelIDs(ctx, db); err != nil {
			return fmt.Errorf("normalize model ids: %w", err)
		}
		if err := setDatabaseMigrationVersion(ctx, db, modelMigrationBaseVersion); err != nil {
			return fmt.Errorf("mark database migration version %s: %w", modelMigrationBaseVersion, err)
		}
		currentVersion = modelMigrationBaseVersion
	}

	if shouldSkipMigration(currentVersion, modelMigrationTargetVersion) {
		common.Info("Tenant model data already migrated, skipping",
			zap.String("current_version", currentVersion),
			zap.String("target_version", modelMigrationTargetVersion))
		return nil
	}
	common.Info("Running tenant model data migration",
		zap.String("current_version", currentVersion),
		zap.String("target_version", modelMigrationTargetVersion))

	if err := seedTenantModels(ctx, db); err != nil {
		return fmt.Errorf("seed tenant_model: %w", err)
	}
	if err := mergeTenantModelTypes(ctx, db); err != nil {
		return fmt.Errorf("merge tenant_model model types: %w", err)
	}
	if err := populateTenantModelIDColumns(ctx, db); err != nil {
		return fmt.Errorf("populate tenant model id columns: %w", err)
	}

	if err := setDatabaseMigrationVersion(ctx, db, modelMigrationTargetVersion); err != nil {
		return fmt.Errorf("mark database migration version: %w", err)
	}
	common.Info("Tenant model data migration completed", zap.String("version", modelMigrationTargetVersion))
	return nil
}

// modelMigrationNewID returns a 32 character hex id, matching uuid.uuid1().hex.
func modelMigrationNewID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

// modelStatusIsActive mirrors the status mapping used by the Python stages:
// "active" if status in ("1", "active", "enable") else "inactive".
func modelStatusIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "1", "active", "enable":
		return true
	default:
		return false
	}
}

func modelStatusValue(status string) string {
	if modelStatusIsActive(status) {
		return "active"
	}
	return "inactive"
}

func isAPIKeyFieldSubset(parsed map[string]any) bool {
	for key := range parsed {
		if key != "api_key" && key != "is_tools" {
			return false
		}
	}
	return true
}

// stripIsToolsFromAPIKey mirrors MigrationStage._strip_is_tools_from_api_key.
// It is the canonical form used to match an api_key against stored instances.
func stripIsToolsFromAPIKey(apiKey string) string {
	if !strings.HasPrefix(apiKey, "{") {
		return apiKey
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(apiKey), &parsed); err != nil {
		return apiKey
	}
	if isAPIKeyFieldSubset(parsed) {
		if inner, ok := parsed["api_key"].(string); ok {
			return inner
		}
		return apiKey
	}
	if _, ok := parsed["is_tools"]; !ok {
		return apiKey
	}
	delete(parsed, "is_tools")
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return apiKey
	}
	return string(encoded)
}

// extractExtraFromAPIKey mirrors TenantModelStage._extract_extra_from_api_key.
func extractExtraFromAPIKey(apiKey string) string {
	if !strings.HasPrefix(apiKey, "{") {
		return "{}"
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(apiKey), &parsed); err != nil {
		return "{}"
	}
	if isTools, ok := parsed["is_tools"].(bool); ok && isTools {
		return `{"is_tools":true}`
	}
	return "{}"
}

// tenantLLMSourceRecord is a projected row of the legacy tenant_llm table.
type tenantLLMSourceRecord struct {
	TenantID   string         `gorm:"column:tenant_id"`
	Factory    string         `gorm:"column:llm_factory"`
	ModelName  sql.NullString `gorm:"column:model_name"`
	ModelType  sql.NullString `gorm:"column:model_type"`
	APIKey     sql.NullString `gorm:"column:api_key"`
	Status     sql.NullString `gorm:"column:status"`
	ProviderID string         `gorm:"column:provider_id"`
}

// migrateTenantModelProviders ports TenantModelProviderStage: one provider row
// per distinct (tenant_id, llm_factory) found in tenant_llm.
func migrateTenantModelProviders(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("tenant_llm") || !scoped.Migrator().HasTable("tenant_model_provider") {
		return nil
	}

	var rows []struct {
		TenantID string `gorm:"column:tenant_id"`
		Factory  string `gorm:"column:llm_factory"`
	}
	if err := scoped.Raw(`
		SELECT DISTINCT t1.tenant_id AS tenant_id, t1.llm_factory AS llm_factory
		FROM tenant_llm t1
		WHERE NOT EXISTS (
			SELECT 1 FROM tenant_model_provider t2
			WHERE t2.tenant_id = t1.tenant_id AND t2.provider_name = t1.llm_factory
		)`).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	if err := scoped.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if err := tx.Exec(
				"INSERT INTO tenant_model_provider (id, provider_name, tenant_id, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, ?)",
				modelMigrationNewID(), row.Factory, row.TenantID, nowMs, now, nowMs, now,
			).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	common.Info("Migrated tenant_model_provider records from tenant_llm", zap.Int("count", len(rows)))
	return nil
}

// migrateTenantModelInstances ports TenantModelInstanceStage: one instance row
// per (tenant_id, llm_factory) taking the first api_key value after dedup.
func migrateTenantModelInstances(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	for _, table := range []string{"tenant_llm", "tenant_model_provider", "tenant_model_instance"} {
		if !scoped.Migrator().HasTable(table) {
			return nil
		}
	}

	existing, err := buildTenantModelInstanceCanonicalSet(ctx, scoped)
	if err != nil {
		return err
	}

	var rows []tenantLLMSourceRecord
	if err := scoped.Raw(`
		SELECT tl.tenant_id AS tenant_id, tl.llm_factory AS llm_factory, tl.api_key AS api_key,
		       MAX(tl.status) AS status, tmp.id AS provider_id
		FROM tenant_llm tl
		INNER JOIN tenant_model_provider tmp
			ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory
		WHERE NOT EXISTS (
			SELECT 1 FROM tenant_model_instance tmi
			WHERE tmi.provider_id = tmp.id AND tmi.api_key = tl.api_key
		)
		GROUP BY tl.tenant_id, tl.llm_factory, tl.api_key, tmp.id`).Scan(&rows).Error; err != nil {
		return err
	}

	rows = filterExistingTenantModelInstances(rows, existing)
	rows = dedupTenantModelInstances(rows)
	if len(rows) == 0 {
		return nil
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	if err := scoped.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if err := tx.Exec(
				"INSERT INTO tenant_model_instance (id, instance_name, provider_id, api_key, status, extra, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				modelMigrationNewID(), modelMigrationInstanceName, row.ProviderID, row.APIKey.String,
				modelStatusValue(row.Status.String), "{}", nowMs, now, nowMs, now,
			).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	common.Info("Migrated tenant_model_instance records from tenant_llm", zap.Int("count", len(rows)))
	return nil
}

// buildTenantModelInstanceCanonicalSet collects the canonical api_key already
// stored for each provider so repeated runs do not insert duplicate instances.
func buildTenantModelInstanceCanonicalSet(ctx context.Context, db *gorm.DB) (map[string]bool, error) {
	var rows []struct {
		ProviderID string         `gorm:"column:provider_id"`
		APIKey     sql.NullString `gorm:"column:api_key"`
	}
	if err := db.Raw("SELECT provider_id, api_key FROM tenant_model_instance").Scan(&rows).Error; err != nil {
		return nil, err
	}
	existing := make(map[string]bool, len(rows))
	for _, row := range rows {
		existing[row.ProviderID+"\x00"+stripIsToolsFromAPIKey(row.APIKey.String)] = true
	}
	return existing, nil
}

func filterExistingTenantModelInstances(rows []tenantLLMSourceRecord, existing map[string]bool) []tenantLLMSourceRecord {
	if len(existing) == 0 {
		return rows
	}
	filtered := make([]tenantLLMSourceRecord, 0, len(rows))
	for _, row := range rows {
		if existing[row.ProviderID+"\x00"+stripIsToolsFromAPIKey(row.APIKey.String)] {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

// dedupTenantModelInstances keeps a single instance per provider, dropping rows
// whose canonical api_key repeats within the same (tenant, factory, provider).
func dedupTenantModelInstances(rows []tenantLLMSourceRecord) []tenantLLMSourceRecord {
	groups := make(map[string][]tenantLLMSourceRecord)
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		key := row.TenantID + "\x00" + row.Factory + "\x00" + row.ProviderID
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], row)
	}

	result := make([]tenantLLMSourceRecord, 0, len(rows))
	for _, key := range order {
		group := groups[key]
		seen := make(map[string]bool, len(group))
		for _, row := range group {
			canonical := stripIsToolsFromAPIKey(row.APIKey.String)
			if seen[canonical] {
				continue
			}
			seen[canonical] = true
			result = append(result, row)
		}
	}
	return result
}

// migrateTenantModels ports TenantModelStage combined with ModelTypeMergeStage.
// tenant_model.model_type is an integer column, so the merged representation is
// written directly: only groups whose merged status is "active" are kept and
// model_type is the OR of the contributing model bits.
func migrateTenantModels(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	for _, table := range []string{"tenant_llm", "tenant_model_provider", "tenant_model_instance", "tenant_model"} {
		if !scoped.Migrator().HasTable(table) {
			return nil
		}
	}

	instanceLookup, err := buildTenantModelInstanceLookup(ctx, scoped)
	if err != nil {
		return err
	}
	if len(instanceLookup) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		SELECT tl.llm_name AS model_name, tmp.id AS provider_id, tl.model_type AS model_type,
		       tl.status AS status, tl.api_key AS api_key
		FROM tenant_llm tl
		INNER JOIN tenant_model_provider tmp
			ON tmp.tenant_id = tl.tenant_id AND tmp.provider_name = tl.llm_factory
		WHERE %s
			AND NOT EXISTS (
				SELECT 1 FROM tenant_model tm
				WHERE tm.provider_id = tmp.id AND tm.model_name = tl.llm_name
			)`, buildTenantModelStatusCondition())

	var rows []tenantLLMSourceRecord
	if err := scoped.Raw(query).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	type modelGroup struct {
		providerID string
		instanceID string
		modelName  string
		modelType  int
		status     string
		extra      string
	}

	groups := make(map[string]*modelGroup)
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		instanceID := instanceLookup[row.ProviderID+"\x00"+stripIsToolsFromAPIKey(row.APIKey.String)]
		if instanceID == "" {
			continue
		}
		key := row.ProviderID + "\x00" + instanceID + "\x00" + row.ModelName.String
		group, ok := groups[key]
		if !ok {
			group = &modelGroup{
				providerID: row.ProviderID,
				instanceID: instanceID,
				modelName:  row.ModelName.String,
				status:     modelStatusValue(row.Status.String),
				extra:      extractExtraFromAPIKey(row.APIKey.String),
			}
			groups[key] = group
			order = append(order, key)
		}
		group.modelType |= modelTypeStringToBit[strings.ToLower(strings.TrimSpace(row.ModelType.String))]
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	inserted := 0
	if err := scoped.Transaction(func(tx *gorm.DB) error {
		for _, key := range order {
			group := groups[key]
			if group.status != "active" {
				continue
			}
			if err := tx.Exec(
				"INSERT INTO tenant_model (id, model_name, provider_id, instance_id, model_type, status, extra, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				modelMigrationNewID(), group.modelName, group.providerID, group.instanceID,
				group.modelType, "active", group.extra, nowMs, now, nowMs, now,
			).Error; err != nil {
				return err
			}
			inserted++
		}
		return nil
	}); err != nil {
		return err
	}

	if inserted > 0 {
		common.Info("Migrated tenant_model records from tenant_llm", zap.Int("count", inserted))
	}
	return nil
}

// buildTenantModelInstanceLookup maps provider_id + canonical api_key to the
// instance id, mirroring TenantModelStage._get_instance_lookup.
func buildTenantModelInstanceLookup(ctx context.Context, db *gorm.DB) (map[string]string, error) {
	var rows []struct {
		ID         string         `gorm:"column:id"`
		ProviderID string         `gorm:"column:provider_id"`
		APIKey     sql.NullString `gorm:"column:api_key"`
	}
	if err := db.Raw("SELECT id, provider_id, api_key FROM tenant_model_instance").Scan(&rows).Error; err != nil {
		return nil, err
	}
	lookup := make(map[string]string, len(rows))
	for _, row := range rows {
		lookup[row.ProviderID+"\x00"+stripIsToolsFromAPIKey(row.APIKey.String)] = row.ID
	}
	return lookup, nil
}

// buildTenantModelStatusCondition mirrors TenantModelStage._build_status_condition.
func buildTenantModelStatusCondition() string {
	emptyFactories := emptyLLMFactories()
	if len(emptyFactories) == 0 {
		return "tl.status = '0'"
	}
	sorted := append([]string(nil), emptyFactories...)
	sort.Strings(sorted)
	quoted := make([]string, 0, len(sorted))
	for _, name := range sorted {
		quoted = append(quoted, "'"+strings.ReplaceAll(name, "'", "''")+"'")
	}
	return fmt.Sprintf("(tl.status = '0' OR (tl.status = '1' AND tl.llm_factory IN (%s)))", strings.Join(quoted, ", "))
}

// emptyLLMFactories returns the factory names whose "llm" list is empty in
// conf/llm_factories.json, matching TenantModelStage._get_empty_llm_factories.
func emptyLLMFactories() []string {
	var empty []string
	for _, factory := range loadFactoryLLMInfos() {
		if len(factory.LLM) == 0 {
			empty = append(empty, factory.Name)
		}
	}
	return empty
}

// factoryLLMInfo is one entry of "factory_llm_infos" in conf/llm_factories.json.
type factoryLLMInfo struct {
	Name string       `json:"name"`
	LLM  []factoryLLM `json:"llm"`
}

// factoryLLM is one model declared by a factory. ModelType is either a string or
// an array of strings, so it is decoded lazily.
type factoryLLM struct {
	LLMName   string          `json:"llm_name"`
	ModelType json.RawMessage `json:"model_type"`
	IsTools   bool            `json:"is_tools"`
	MaxTokens json.RawMessage `json:"max_tokens"`
}

// loadFactoryLLMInfos reads and parses conf/llm_factories.json, returning nil when
// the file is missing or unreadable.
func loadFactoryLLMInfos() []factoryLLMInfo {
	path := locateLLMFactoriesFile()
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		common.Warn("Failed to read llm_factories.json for model migration", zap.String("path", path), zap.Error(err))
		return nil
	}
	var payload struct {
		FactoryLLMInfos []factoryLLMInfo `json:"factory_llm_infos"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		common.Warn("Failed to parse llm_factories.json for model migration", zap.String("path", path), zap.Error(err))
		return nil
	}
	return payload.FactoryLLMInfos
}

// locateLLMFactoriesFile finds conf/llm_factories.json by walking up from the
// working directory and falling back to the executable's directory.
func locateLLMFactoriesFile() string {
	candidates := []string{filepath.Join("conf", "llm_factories.json")}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for depth := 0; depth < 5; depth++ {
			candidates = append(candidates, filepath.Join(dir, "conf", "llm_factories.json"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "conf", "llm_factories.json"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// Tenant model seeding ports TenantModelSeedingStage.

// tenantModelSeedRow tracks an existing tenant_model row so factory-declared
// model types can be merged into it without a second table rewrite.
type tenantModelSeedRow struct {
	id        string
	modelType int
}

// seedTenantModels fills tenant_model with the models declared by
// conf/llm_factories.json for every provider/instance created by the previous
// steps. model_type is stored as the merged integer, so the declared factory
// bits are OR-ed into the matching (provider_id, instance_id, model_name) row
// and a fresh row is inserted when none exists.
func seedTenantModels(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	for _, table := range []string{"tenant_model_provider", "tenant_model_instance", "tenant_model"} {
		if !scoped.Migrator().HasTable(table) {
			return nil
		}
	}

	factories := loadFactoryLLMInfos()
	if len(factories) == 0 {
		return nil
	}

	existingExtras, err := loadTenantModelExtraMap(scoped)
	if err != nil {
		return err
	}
	existingStatuses, err := loadTenantModelStatusMap(scoped)
	if err != nil {
		return err
	}
	existing, err := loadTenantModelRowMap(scoped)
	if err != nil {
		return err
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	inserted, updated := 0, 0
	err = scoped.Transaction(func(tx *gorm.DB) error {
		for _, factory := range factories {
			if len(factory.LLM) == 0 {
				continue
			}
			providerName, extraInclude, extraExclude := factoryProviderFilter(factory.Name)
			providerIDs, err := loadProviderIDsByName(scoped, providerName)
			if err != nil {
				return err
			}
			for _, providerID := range providerIDs {
				instanceIDs, err := loadInstanceIDsByProvider(scoped, providerID, extraInclude, extraExclude)
				if err != nil {
					return err
				}
				for _, instanceID := range instanceIDs {
					for _, llm := range factory.LLM {
						if llm.LLMName == "" {
							continue
						}
						bits := factoryModelTypeBits(llm)
						if bits == 0 {
							continue
						}
						key := providerID + "\x00" + instanceID + "\x00" + llm.LLMName
						if row, ok := existing[key]; ok {
							if row.modelType|bits == row.modelType {
								continue
							}
							merged := row.modelType | bits
							if err := tx.Exec(
								"UPDATE tenant_model SET model_type = ?, update_time = ?, update_date = ? WHERE id = ?",
								merged, nowMs, now, row.id,
							).Error; err != nil {
								return err
							}
							existing[key] = tenantModelSeedRow{id: row.id, modelType: merged}
							updated++
							continue
						}
						extra := existingExtras[llm.LLMName]
						if extra == "" {
							extra = buildFactoryModelExtra(llm)
						}
						status := existingStatuses[llm.LLMName]
						if status == "" {
							status = "active"
						}
						id := modelMigrationNewID()
						if err := tx.Exec(
							"INSERT INTO tenant_model (id, model_name, provider_id, instance_id, model_type, status, extra, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
							id, llm.LLMName, providerID, instanceID, bits, status, extra, nowMs, now, nowMs, now,
						).Error; err != nil {
							return err
						}
						existing[key] = tenantModelSeedRow{id: id, modelType: bits}
						inserted++
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if inserted > 0 || updated > 0 {
		common.Info("Seeded tenant_model from llm_factories.json", zap.Int("inserted", inserted), zap.Int("merged", updated))
	}
	return nil
}

// factoryProviderFilter mirrors the siliconflow special case in
// TenantModelSeedingStage.execute: the intl factory targets SILICONFLOW instances
// whose extra contains "region": "intl", while the global factory excludes them.
func factoryProviderFilter(factoryName string) (providerName, extraInclude, extraExclude string) {
	switch factoryName {
	case "siliconflow_intl":
		return "SILICONFLOW", `%"region": "intl"%`, ""
	case "SILICONFLOW":
		return "SILICONFLOW", "", `%"region": "intl"%`
	default:
		return factoryName, "", ""
	}
}

// factoryModelTypeBits merges every model_type declared for a factory model.
func factoryModelTypeBits(llm factoryLLM) int {
	bits := 0
	for _, name := range factoryLLMModelTypes(llm.ModelType) {
		bits |= modelTypeStringToBit[strings.ToLower(strings.TrimSpace(name))]
	}
	return bits
}

// factoryLLMModelTypes decodes a model_type that is either a string or an array
// of strings, defaulting to "chat" like the Python stage.
func factoryLLMModelTypes(raw json.RawMessage) []string {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && single != "" {
		return []string{single}
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil && len(list) > 0 {
		return list
	}
	return []string{"chat"}
}

// buildFactoryModelExtra builds the extra JSON stored for a newly seeded model,
// mirroring TenantModelSeedingStage._build_extra.
func buildFactoryModelExtra(llm factoryLLM) string {
	extra := make(map[string]any)
	if llm.IsTools {
		extra["is_tools"] = true
	}
	if len(llm.MaxTokens) > 0 && string(llm.MaxTokens) != "null" {
		var tokens any
		if err := json.Unmarshal(llm.MaxTokens, &tokens); err == nil {
			extra["max_tokens"] = tokens
		}
	}
	if len(extra) == 0 {
		return "{}"
	}
	encoded, err := json.Marshal(extra)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func loadTenantModelRowMap(db *gorm.DB) (map[string]tenantModelSeedRow, error) {
	var rows []struct {
		ID         string         `gorm:"column:id"`
		ProviderID string         `gorm:"column:provider_id"`
		InstanceID string         `gorm:"column:instance_id"`
		ModelName  string         `gorm:"column:model_name"`
		ModelType  sql.NullString `gorm:"column:model_type"`
	}
	if err := db.Raw("SELECT id, provider_id, instance_id, model_name, model_type FROM tenant_model").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]tenantModelSeedRow, len(rows))
	for _, row := range rows {
		result[row.ProviderID+"\x00"+row.InstanceID+"\x00"+row.ModelName] = tenantModelSeedRow{
			id:        row.ID,
			modelType: mergeTenantModelTypeBits(row.ModelType.String),
		}
	}
	return result, nil
}

func loadTenantModelExtraMap(db *gorm.DB) (map[string]string, error) {
	var rows []struct {
		ModelName string `gorm:"column:model_name"`
		Extra     string `gorm:"column:extra"`
	}
	if err := db.Raw("SELECT model_name, extra FROM tenant_model WHERE extra IS NOT NULL AND extra != '{}'").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.ModelName != "" && row.Extra != "" && row.Extra != "{}" {
			result[row.ModelName] = row.Extra
		}
	}
	return result, nil
}

func loadTenantModelStatusMap(db *gorm.DB) (map[string]string, error) {
	var rows []struct {
		ModelName string `gorm:"column:model_name"`
		Status    string `gorm:"column:status"`
	}
	if err := db.Raw("SELECT model_name, status FROM tenant_model WHERE status IS NOT NULL AND status != 'unsupported'").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.ModelName != "" && row.Status != "" {
			result[row.ModelName] = row.Status
		}
	}
	return result, nil
}

func loadProviderIDsByName(db *gorm.DB, providerName string) ([]string, error) {
	var rows []struct {
		ID string `gorm:"column:id"`
	}
	if err := db.Raw("SELECT id FROM tenant_model_provider WHERE provider_name = ?", providerName).Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

func loadInstanceIDsByProvider(db *gorm.DB, providerID, extraInclude, extraExclude string) ([]string, error) {
	var (
		rows []struct {
			ID string `gorm:"column:id"`
		}
		query string
		args  []any
	)
	switch {
	case extraInclude != "" && extraExclude != "":
		query = "SELECT id FROM tenant_model_instance WHERE provider_id = ? AND extra LIKE ? AND extra NOT LIKE ?"
		args = []any{providerID, extraInclude, extraExclude}
	case extraInclude != "":
		query = "SELECT id FROM tenant_model_instance WHERE provider_id = ? AND extra LIKE ?"
		args = []any{providerID, extraInclude}
	case extraExclude != "":
		query = "SELECT id FROM tenant_model_instance WHERE provider_id = ? AND extra NOT LIKE ?"
		args = []any{providerID, extraExclude}
	default:
		query = "SELECT id FROM tenant_model_instance WHERE provider_id = ?"
		args = []any{providerID}
	}
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

// Merge tenant model types ports ModelTypeMergeStage.

// tenantModelMergeSource is one tenant_model row read by mergeTenantModelTypes.
// model_type is read as text because the stage only runs on tables that have not
// been converted to the integer representation yet.
type tenantModelMergeSource struct {
	ID         string         `gorm:"column:id"`
	ModelName  sql.NullString `gorm:"column:model_name"`
	ProviderID string         `gorm:"column:provider_id"`
	InstanceID string         `gorm:"column:instance_id"`
	ModelType  sql.NullString `gorm:"column:model_type"`
	Status     sql.NullString `gorm:"column:status"`
	Extra      sql.NullString `gorm:"column:extra"`
	CreateTime sql.NullInt64  `gorm:"column:create_time"`
	CreateDate sql.NullTime   `gorm:"column:create_date"`
	UpdateTime sql.NullInt64  `gorm:"column:update_time"`
	UpdateDate sql.NullTime   `gorm:"column:update_date"`
}

// mergedTenantModelRecord is one merged tenant_model row written back by
// mergeTenantModelTypes.
type mergedTenantModelRecord struct {
	modelName  sql.NullString
	providerID string
	instanceID string
	modelType  int
	status     string
	extra      sql.NullString
	createTime sql.NullInt64
	createDate sql.NullTime
	updateTime sql.NullInt64
	updateDate sql.NullTime
}

// mergeTenantModelTypes ports ModelTypeMergeStage: it collapses the tenant_model
// rows sharing (provider_id, instance_id, model_name) into a single row whose
// model_type is the integer OR of the contributing model bits, dropping the bits
// that only ever appeared on "unsupported" rows. Groups whose merged status is
// not "active" are dropped.
//
// The stage only rewrites a table whose model_type is still stored as text. When
// the column is already an integer type the merge has been applied and the stage
// is a no-op, mirroring ModelTypeMergeStage.check.
func mergeTenantModelTypes(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable("tenant_model") {
		return nil
	}

	isInteger, err := tenantModelModelTypeIsInteger(scoped)
	if err != nil {
		return err
	}
	if isInteger {
		common.Info("tenant_model.model_type is already an integer, merge stage skipped")
		return nil
	}

	var rows []tenantModelMergeSource
	if err := scoped.Raw(
		"SELECT id, model_name, provider_id, instance_id, model_type, status, extra, create_time, create_date, update_time, update_date FROM tenant_model ORDER BY id",
	).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	type mergeGroup struct {
		first       tenantModelMergeSource
		supported   int
		unsupported int
		status      string
	}

	groups := make(map[string]*mergeGroup, len(rows))
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		key := row.ProviderID + "\x00" + row.InstanceID + "\x00" + row.ModelName.String
		group, ok := groups[key]
		if !ok {
			group = &mergeGroup{first: row}
			groups[key] = group
			order = append(order, key)
		}
		bits := mergeTenantModelTypeBits(row.ModelType.String)
		if row.Status.String == "unsupported" {
			group.unsupported |= bits
			continue
		}
		group.supported |= bits
		if group.status == "" {
			group.status = row.Status.String
		}
	}

	nowMs := time.Now().UnixMilli()
	now := time.Now()
	merged := make([]mergedTenantModelRecord, 0, len(order))
	for _, key := range order {
		group := groups[key]
		status := group.status
		if status == "" {
			status = "active"
		}
		if status != "active" {
			continue
		}
		record := mergedTenantModelRecord{
			modelName:  group.first.ModelName,
			providerID: group.first.ProviderID,
			instanceID: group.first.InstanceID,
			modelType:  group.supported &^ group.unsupported,
			status:     status,
			extra:      group.first.Extra,
			createTime: group.first.CreateTime,
			createDate: group.first.CreateDate,
			updateTime: group.first.UpdateTime,
			updateDate: group.first.UpdateDate,
		}
		if !record.createTime.Valid {
			record.createTime = sql.NullInt64{Int64: nowMs, Valid: true}
		}
		if !record.updateTime.Valid {
			record.updateTime = sql.NullInt64{Int64: nowMs, Valid: true}
		}
		if !record.createDate.Valid {
			record.createDate = sql.NullTime{Time: now, Valid: true}
		}
		if !record.updateDate.Valid {
			record.updateDate = sql.NullTime{Time: now, Valid: true}
		}
		merged = append(merged, record)
	}

	// The whole table is replaced even when every group was dropped, mirroring the
	// Python stage that always swaps in the merged table. Leaving the source rows
	// behind would keep text model_type values in place for the integer typed reads
	// that follow.
	if err := scoped.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM tenant_model").Error; err != nil {
			return err
		}
		for _, record := range merged {
			if err := tx.Exec(
				"INSERT INTO tenant_model (id, model_name, provider_id, instance_id, model_type, status, extra, create_time, create_date, update_time, update_date) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				modelMigrationNewID(), record.modelName, record.providerID, record.instanceID,
				record.modelType, record.status, record.extra,
				record.createTime, record.createDate, record.updateTime, record.updateDate,
			).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	common.Info("Merged tenant_model model types", zap.Int("rows", len(merged)))
	return nil
}

// mergeTenantModelTypeBits maps a stored model_type to its bit. Legacy rows hold
// the LLMType name, while rows written by the seeding stage already hold the
// merged integer, so numeric text is accepted as well.
func mergeTenantModelTypeBits(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if bit, ok := modelTypeStringToBit[strings.ToLower(trimmed)]; ok {
		return bit
	}
	if value, err := strconv.Atoi(trimmed); err == nil {
		return value
	}
	return 0
}

// tenantModelModelTypeIsInteger reports whether tenant_model.model_type is stored
// as an integer column, in which case the merge has already been applied.
func tenantModelModelTypeIsInteger(db *gorm.DB) (bool, error) {
	columnTypes, err := db.Migrator().ColumnTypes("tenant_model")
	if err != nil {
		return false, err
	}
	for _, columnType := range columnTypes {
		if columnType.Name() != "model_type" {
			continue
		}
		return strings.Contains(strings.ToLower(columnType.DatabaseTypeName()), "int"), nil
	}
	return false, nil
}

// Model id normalization ports ModelIdConfigStage.

var modelIDStringColumns = map[string][]string{
	"tenant":        {"llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id", "ocr_id"},
	"knowledgebase": {"embd_id"},
	"dialog":        {"llm_id", "rerank_id"},
	"memory":        {"embd_id", "llm_id"},
}

var modelIDJSONColumns = map[string][]string{
	"knowledgebase":          {"parser_config"},
	"document":               {"parser_config"},
	"search":                 {"search_config"},
	"user_canvas":            {"dsl"},
	"canvas_template":        {"dsl"},
	"user_canvas_version":    {"dsl"},
	"api_4_conversation":     {"dsl"},
	"pipeline_operation_log": {"dsl"},
	"connector":              {"config"},
	"evaluation_runs":        {"config_snapshot"},
}

var modelIDFieldSet = map[string]bool{
	"llm_id":          true,
	"embd_id":         true,
	"embedding_model": true,
	"rerank_id":       true,
	"asr_id":          true,
	"img2txt_id":      true,
	"tts_id":          true,
	"ocr_id":          true,
}

var searchConfigModelIDFieldSet = map[string]bool{"chat_id": true}

// normalizeModelID rewrites "<model>@<provider>" into "<model>@default@<provider>".
// Values that are not exactly two non-empty parts are returned unchanged.
func normalizeModelID(value string) (string, bool) {
	parts := strings.Split(value, "@")
	if len(parts) != 2 {
		return value, false
	}
	modelName, providerName := parts[0], parts[1]
	if modelName == "" || providerName == "" {
		return value, false
	}
	return modelName + "@default@" + providerName, true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// normalizeModelIDConfig recursively normalizes model id fields inside a decoded
// JSON document, mirroring ModelIdConfigStage.normalize_config.
func normalizeModelIDConfig(value any, path []string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			keyPath := append(append([]string{}, path...), key)
			shouldNormalize := modelIDFieldSet[key] || (searchConfigModelIDFieldSet[key] && containsString(path, "search_config"))
			if shouldNormalize {
				if str, ok := item.(string); ok {
					normalizedValue, itemChanged := normalizeModelID(str)
					normalized[key] = normalizedValue
					changed = changed || itemChanged
					continue
				}
				normalized[key] = item
				continue
			}
			normalizedItem, itemChanged := normalizeModelIDConfig(item, keyPath)
			normalized[key] = normalizedItem
			changed = changed || itemChanged
		}
		return normalized, changed
	case []any:
		changed := false
		normalized := make([]any, len(typed))
		for index, item := range typed {
			indexPath := append(append([]string{}, path...), strconv.Itoa(index))
			normalizedItem, itemChanged := normalizeModelIDConfig(item, indexPath)
			normalized[index] = normalizedItem
			changed = changed || itemChanged
		}
		return normalized, changed
	default:
		return value, false
	}
}

// normalizeStoredModelIDs rewrites stored model ids across the tables listed by
// ModelIdConfigStage.
func normalizeStoredModelIDs(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	for _, table := range sortedTableNames(modelIDStringColumns) {
		if !scoped.Migrator().HasTable(table) {
			continue
		}
		for _, column := range modelIDStringColumns[table] {
			if !scoped.Migrator().HasColumn(table, column) {
				continue
			}
			if err := normalizeModelIDStringColumn(ctx, scoped, table, column); err != nil {
				return err
			}
		}
	}
	for _, table := range sortedTableNames(modelIDJSONColumns) {
		if !scoped.Migrator().HasTable(table) {
			continue
		}
		for _, column := range modelIDJSONColumns[table] {
			if !scoped.Migrator().HasColumn(table, column) {
				continue
			}
			if err := normalizeModelIDJSONColumn(ctx, scoped, table, column); err != nil {
				return err
			}
		}
	}
	return nil
}

// sortedTableNames returns a stable ordering so the migration is deterministic.
func sortedTableNames(columns map[string][]string) []string {
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeModelIDStringColumn(ctx context.Context, db *gorm.DB, table, column string) error {
	var rows []struct {
		ID    string         `gorm:"column:id"`
		Value sql.NullString `gorm:"column:value"`
	}
	query := fmt.Sprintf("SELECT id AS id, `%s` AS value FROM `%s` WHERE `%s` IS NOT NULL AND `%s` LIKE '%%@%%'", column, table, column, column)
	if err := db.Raw(query).Scan(&rows).Error; err != nil {
		return err
	}

	updates := make(map[string]string)
	for _, row := range rows {
		if !row.Value.Valid || row.Value.String == "" {
			continue
		}
		if normalized, changed := normalizeModelID(row.Value.String); changed {
			updates[row.ID] = normalized
		}
	}
	return applyModelIDUpdates(ctx, db, table, column, updates)
}

func normalizeModelIDJSONColumn(ctx context.Context, db *gorm.DB, table, column string) error {
	var rows []struct {
		ID    string         `gorm:"column:id"`
		Value sql.NullString `gorm:"column:value"`
	}
	query := fmt.Sprintf("SELECT id AS id, `%s` AS value FROM `%s` WHERE `%s` IS NOT NULL AND `%s` != ''", column, table, column, column)
	if err := db.Raw(query).Scan(&rows).Error; err != nil {
		return err
	}

	updates := make(map[string]string)
	for _, row := range rows {
		var decoded any
		if err := json.Unmarshal([]byte(row.Value.String), &decoded); err != nil {
			continue
		}
		normalized, changed := normalizeModelIDConfig(decoded, []string{column})
		if !changed {
			continue
		}
		encoded, err := json.Marshal(normalized)
		if err != nil {
			continue
		}
		updates[row.ID] = string(encoded)
	}
	return applyModelIDUpdates(ctx, db, table, column, updates)
}

func applyModelIDUpdates(ctx context.Context, db *gorm.DB, table, column string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}
	statement := fmt.Sprintf("UPDATE `%s` SET `%s` = ? WHERE id = ?", table, column)
	return db.Transaction(func(tx *gorm.DB) error {
		for id, value := range updates {
			if err := tx.Exec(statement, value, id).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Tenant model id population ports TenantModelIdMigrationStage.

type tenantModelIDField struct {
	tenantIDColumn string
	legacyColumn   string
	modelType      string
}

var tenantModelIDFields = map[string][]tenantModelIDField{
	"tenant": {
		{"tenant_llm_id", "llm_id", "chat"},
		{"tenant_embd_id", "embd_id", "embedding"},
		{"tenant_asr_id", "asr_id", "speech2text"},
		{"tenant_img2txt_id", "img2txt_id", "image2text"},
		{"tenant_rerank_id", "rerank_id", "rerank"},
		{"tenant_tts_id", "tts_id", "tts"},
		{"tenant_ocr_id", "ocr_id", "ocr"},
	},
	"knowledgebase": {
		{"tenant_embd_id", "embd_id", "embedding"},
	},
	"dialog": {
		{"tenant_llm_id", "llm_id", "chat"},
		{"tenant_rerank_id", "rerank_id", "rerank"},
	},
	"memory": {
		{"tenant_embd_id", "embd_id", "embedding"},
		{"tenant_llm_id", "llm_id", "chat"},
	},
}

type tenantModelLookupKey struct {
	tenantID  string
	modelName string
	provider  string
	modelType string
}

// populateTenantModelIDColumns ports TenantModelIdMigrationStage: it ensures the
// tenant_*_id columns exist and fills them from the legacy *_id columns by
// resolving the referenced tenant_model row.
func populateTenantModelIDColumns(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	for _, table := range []string{"tenant_model", "tenant_model_provider", "tenant_model_instance"} {
		if !scoped.Migrator().HasTable(table) {
			return nil
		}
	}

	for _, table := range sortedTenantModelIDTables() {
		if !scoped.Migrator().HasTable(table) {
			continue
		}
		for _, field := range tenantModelIDFields[table] {
			if !scoped.Migrator().HasColumn(table, field.tenantIDColumn) {
				if err := ensureTenantModelIDColumn(ctx, scoped, table, field.tenantIDColumn); err != nil {
					return err
				}
			}
		}
	}

	lookup, err := buildTenantModelLookup(ctx, scoped)
	if err != nil {
		return err
	}
	if len(lookup) == 0 {
		return nil
	}

	total := 0
	for _, table := range sortedTenantModelIDTables() {
		if !scoped.Migrator().HasTable(table) {
			continue
		}
		for _, field := range tenantModelIDFields[table] {
			if !scoped.Migrator().HasColumn(table, field.tenantIDColumn) || !scoped.Migrator().HasColumn(table, field.legacyColumn) {
				continue
			}
			updated, err := populateTenantModelIDColumn(ctx, scoped, table, field, lookup)
			if err != nil {
				return err
			}
			total += updated
		}
	}

	if total > 0 {
		common.Info("Populated tenant model id columns", zap.Int("rows", total))
	}
	return nil
}

func sortedTenantModelIDTables() []string {
	names := make([]string, 0, len(tenantModelIDFields))
	for name := range tenantModelIDFields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ensureTenantModelIDColumn(ctx context.Context, db *gorm.DB, table, column string) error {
	common.Info("Adding tenant model id column", zap.String("table", table), zap.String("column", column))
	statement := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` VARCHAR(32) NULL", table, column)
	return db.Exec(statement).Error
}

// buildTenantModelLookup maps (tenant_id, model_name, provider, model_type) to
// tenant_model.id for every active model, expanding the model_type bitmask.
func buildTenantModelLookup(ctx context.Context, db *gorm.DB) (map[tenantModelLookupKey]string, error) {
	var rows []struct {
		ID           string `gorm:"column:id"`
		ModelName    string `gorm:"column:model_name"`
		ModelType    int    `gorm:"column:model_type"`
		TenantID     string `gorm:"column:tenant_id"`
		ProviderName string `gorm:"column:provider_name"`
	}
	if err := db.Raw(`
		SELECT tm.id AS id, tm.model_name AS model_name, tm.model_type AS model_type,
		       tmp.tenant_id AS tenant_id, tmp.provider_name AS provider_name
		FROM tenant_model tm
		INNER JOIN tenant_model_provider tmp ON tm.provider_id = tmp.id
		WHERE tm.status = 'active'`).Scan(&rows).Error; err != nil {
		return nil, err
	}

	lookup := make(map[tenantModelLookupKey]string)
	for _, row := range rows {
		for typeName, typeBit := range modelTypeStringToBit {
			if row.ModelType&typeBit != 0 {
				lookup[tenantModelLookupKey{row.TenantID, row.ModelName, row.ProviderName, typeName}] = row.ID
			}
		}
	}
	return lookup, nil
}

func populateTenantModelIDColumn(ctx context.Context, db *gorm.DB, table string, field tenantModelIDField, lookup map[tenantModelLookupKey]string) (int, error) {
	tenantIDExpr := "tenant_id"
	if table == "tenant" {
		// The tenant table's primary key is the tenant id itself.
		tenantIDExpr = "id"
	}
	query := fmt.Sprintf(
		"SELECT id AS id, %s AS tenant_id, `%s` AS model_name FROM `%s` WHERE (`%s` IS NULL OR `%s` = '' OR LENGTH(`%s`) <> 32) AND `%s` IS NOT NULL AND `%s` != ''",
		tenantIDExpr, field.legacyColumn, table,
		field.tenantIDColumn, field.tenantIDColumn, field.tenantIDColumn,
		field.legacyColumn, field.legacyColumn,
	)

	var rows []struct {
		ID        string `gorm:"column:id"`
		TenantID  string `gorm:"column:tenant_id"`
		ModelName string `gorm:"column:model_name"`
	}
	if err := db.Raw(query).Scan(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	updateStatement := fmt.Sprintf("UPDATE `%s` SET `%s` = ? WHERE id = ?", table, field.tenantIDColumn)
	updated := 0
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			tenantID := row.TenantID
			if table == "tenant" {
				tenantID = row.ID
			}
			resolved := resolveTenantModelID(lookup, tenantID, row.ModelName, field.modelType)
			if err := tx.Exec(updateStatement, resolved, row.ID).Error; err != nil {
				return err
			}
			updated++
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return updated, nil
}

// resolveTenantModelID ports TenantModelIdMigrationStage._resolve_model_id.
func resolveTenantModelID(lookup map[tenantModelLookupKey]string, tenantID, modelName, modelType string) string {
	pureName, _, providerName := splitModelName(modelName)
	if pureName == "" || providerName == "" {
		return ""
	}
	if id, ok := lookup[tenantModelLookupKey{tenantID, pureName, providerName, modelType}]; ok {
		return id
	}
	for key, id := range lookup {
		if key.tenantID == tenantID && key.modelName == pureName && key.modelType == modelType {
			return id
		}
	}
	return ""
}

// splitModelName ports TenantModelIdMigrationStage._split_model_name:
// "{model}@{factory}" or "{model}@{instance}@{factory}".
func splitModelName(modelName string) (string, string, string) {
	if modelName == "" {
		return "", "", ""
	}
	parts := strings.Split(modelName, "@")
	switch len(parts) {
	case 1:
		return parts[0], modelMigrationInstanceName, ""
	case 2:
		return parts[0], modelMigrationInstanceName, parts[1]
	default:
		return parts[0], parts[1], parts[2]
	}
}
