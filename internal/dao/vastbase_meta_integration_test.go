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

//go:build integration

package dao

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/server/config"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ptr[T any](v T) *T { return &v }

// vastbaseIntegrationConfig reads the target instance from the environment:
// VASTBASE_META_HOST/PORT/USER/PASSWORD/NAME/SSL_MODE (defaults match the
// bundled docker deployment). Run with:
//
//	go test -tags integration ./internal/dao/ -run TestVastbaseMeta -v
func vastbaseIntegrationConfig(t *testing.T) config.VastbaseMetaConfig {
	t.Helper()
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return fallback
	}
	port, err := strconv.Atoi(get("VASTBASE_META_PORT", "5432"))
	if err != nil {
		t.Fatalf("invalid VASTBASE_META_PORT: %v", err)
	}
	cfg := config.VastbaseMetaConfig{
		Host:         get("VASTBASE_META_HOST", "127.0.0.1"),
		Port:         port,
		User:         get("VASTBASE_META_USER", "ragflow"),
		Password:     get("VASTBASE_META_PASSWORD", "Infini_Rag@123"),
		DatabaseName: get("VASTBASE_META_NAME", "rag_flow"),
		SSLMode:      get("VASTBASE_META_SSL_MODE", "disable"),
	}
	return cfg
}

func openVastbaseIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(newVastbaseDialector(vastbaseIntegrationConfig(t)), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open vastbase: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

// TestVastbaseMetaAutoMigrateAllModels runs the full AutoMigrate model set
// against a real Vastbase: every CREATE TABLE and index statement must be
// accepted, and a second run must converge without ALTERs failing.
func TestVastbaseMetaAutoMigrateAllModels(t *testing.T) {
	db := openVastbaseIntegrationDB(t)
	ctx := context.Background()
	for _, model := range autoMigrateModels() {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			t.Fatalf("autoMigrate %T: %v", model, err)
		}
	}
	// Convergence pass: remapped types must match what was created.
	for _, model := range autoMigrateModels() {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			t.Fatalf("converge %T: %v", model, err)
		}
	}
}

// TestVastbaseMetaTenantLLMAutoIncrementRoundtrip checks the INSERT ...
// RETURNING path on the one auto-increment business model: the id must be
// read back without ON CONFLICT being involved.
func TestVastbaseMetaTenantLLMAutoIncrementRoundtrip(t *testing.T) {
	db := openVastbaseIntegrationDB(t)
	modelType := "chat"
	llmName := "integration-model"
	row := &entity.TenantLLM{
		TenantID:   "integration-tenant",
		LLMFactory: "openai-api-compatible",
		ModelType:  &modelType,
		APIKey:     ptr("integration"),
		APIBase:    ptr("https://example.invalid/v1"),
		LLMName:    &llmName,
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create tenant_llm: %v", err)
	}
	if row.ID == 0 {
		t.Fatal("auto-increment id was not read back via RETURNING")
	}
	defer db.Delete(&entity.TenantLLM{}, row.ID)

	var stored entity.TenantLLM
	if err := db.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?",
		row.TenantID, row.LLMFactory, llmName).First(&stored).Error; err != nil {
		t.Fatalf("read back tenant_llm: %v", err)
	}
	if stored.ID != row.ID {
		t.Fatalf("read-back id = %d, want %d", stored.ID, row.ID)
	}
}

// TestVastbaseMetaSystemSettingsUpsert checks the ON CONFLICT DO UPDATE path
// without RETURNING — the combination Vastbase B mode rejects when a default-
// valued field drags RETURNING in. Mirrors setDatabaseMigrationVersion's shape.
func TestVastbaseMetaSystemSettingsUpsert(t *testing.T) {
	db := openVastbaseIntegrationDB(t)
	ctx := context.Background()
	upsert := func(value string) error {
		setting := entity.SystemSettings{
			Name: "integration.marker", Source: "integration",
			DataType: "string", Value: value,
		}
		return db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"source", "data_type", "value", "update_time", "update_date"}),
		}).Create(&setting).Error
	}
	defer db.Exec("DELETE FROM system_settings WHERE name = ?", "integration.marker")

	if err := upsert("first"); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if err := upsert("second"); err != nil {
		t.Fatalf("conflicting upsert: %v", err)
	}
	var value string
	if err := db.Raw("SELECT value FROM system_settings WHERE name = ?", "integration.marker").
		Row().Scan(&value); err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if value != "second" {
		t.Fatalf("marker value = %q, want second", value)
	}
}

// TestVastbaseMetaDuplicateKeyTranslation checks the unique-index conflict
// surfaces as gorm.ErrDuplicatedKey, which the service layer's "already
// exists" mapping consumes.
func TestVastbaseMetaDuplicateKeyTranslation(t *testing.T) {
	db := openVastbaseIntegrationDB(t)
	dup := &entity.SystemSettings{
		Name: "integration.dup", Source: "integration", DataType: "string", Value: "v",
	}
	defer db.Exec("DELETE FROM system_settings WHERE name = ?", "integration.dup")
	if err := db.Create(dup).Error; err != nil {
		t.Fatalf("seed unique row: %v", err)
	}
	conflict := &entity.SystemSettings{
		Name: "integration.dup", Source: "integration", DataType: "string", Value: "v2",
	}
	err := db.Create(conflict).Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("duplicate insert error = %v, want gorm.ErrDuplicatedKey", err)
	}
	if !IsDuplicateKeyErr(err) {
		t.Fatalf("IsDuplicateKeyErr(%v) = false", err)
	}
}

// TestVastbaseMetaDuplicateObjectSQLSTATE pins the wire form the
// convergence skip in autoMigrateSafely matches: re-issuing DDL for an
// existing table, column and index must surface as *pq.Error with the
// duplicate-object SQLSTATEs (42P07/42701), not merely as text containing
// "already exists". A Vastbase that wraps or recodes these errors would
// break the typed classification and fail here.
func TestVastbaseMetaDuplicateObjectSQLSTATE(t *testing.T) {
	db := openVastbaseIntegrationDB(t)
	const marker = "integration_duplicate_marker"
	db.Exec("DROP TABLE IF EXISTS " + marker)
	defer db.Exec("DROP TABLE IF EXISTS " + marker)

	for _, ddl := range []string{
		`CREATE TABLE ` + marker + ` (id varchar(32))`,
		`ALTER TABLE ` + marker + ` ADD COLUMN value varchar(64)`,
		`CREATE INDEX idx_integration_dup_marker ON ` + marker + ` (id)`,
	} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatalf("seed DDL: %s: %v", ddl, err)
		}
		err := db.Exec(ddl).Error
		if err == nil {
			t.Fatalf("duplicate DDL accepted: %s", ddl)
		}
		var pqErr *pq.Error
		if !errors.As(err, &pqErr) {
			t.Fatalf("duplicate DDL error is not *pq.Error: %T: %v", err, err)
		}
		if pqErr.Code != "42P07" && pqErr.Code != "42701" {
			t.Fatalf("duplicate DDL SQLSTATE = %s, want 42P07 or 42701: %v", pqErr.Code, err)
		}
		if !isPostgresDuplicateObjectErr(err) {
			t.Fatalf("duplicate DDL error not classified as convergence: %v", err)
		}
	}
}

// TestVastbaseMetaBModeMySQLSyntax proves the metadata database really runs
// in B (MySQL) compatibility mode. The DAO layer's hand-written SQL stays
// verbatim MySQL — unquoted references to the user table (reserved in
// vanilla PostgreSQL), DATE_FORMAT, the REGEXP operator, DELETE ... ORDER
// BY ... LIMIT — and relies on B mode to parse it over the PostgreSQL wire
// protocol. A failure here usually means the database was not created with
// DBCOMPATIBILITY = 'B' (docker/vastbase/init/01_init.sh does that).
func TestVastbaseMetaBModeMySQLSyntax(t *testing.T) {
	db := openVastbaseIntegrationDB(t)
	ctx := context.Background()
	for _, model := range []any{&entity.User{}, &entity.UserTenant{}, &entity.UserCanvas{}, &entity.TimeRecord{}} {
		if err := autoMigrateSafely(ctx, db, model); err != nil {
			t.Fatalf("autoMigrate %T: %v", model, err)
		}
	}

	const userID = "vbmodeuser0000000000000000000000"   // 32 chars, User.id size
	const tenantID = "vbmodetenant00000000000000000000" // 32 chars
	defer func() {
		db.Exec("DELETE FROM user_tenant WHERE user_id = ?", userID)
		db.Exec("DELETE FROM user WHERE id = ?", userID)
	}()

	// Unquoted user-table JOIN plus DATE_FORMAT, via the real DAO query
	// (UserTenantDAO.GetMembersByTenantID).
	if err := db.Create(&entity.User{
		ID: userID, Nickname: "vb-mode", Email: "vb-mode@example.invalid",
		IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0",
		Status: ptr("1"), IsSuperuser: ptr(false),
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID: "vbmodeut000000000000000000000000", UserID: userID, TenantID: tenantID,
		Role: "member", InvitedBy: userID, Status: ptr("1"),
	}).Error; err != nil {
		t.Fatalf("seed user_tenant: %v", err)
	}
	members, err := NewUserTenantDAO().GetMembersByTenantID(ctx, db, tenantID)
	if err != nil {
		t.Fatalf("GetMembersByTenantID (user join + DATE_FORMAT): %v", err)
	}
	if len(members) != 1 || members[0].Nickname != "vb-mode" {
		t.Fatalf("members = %+v, want one vb-mode row", members)
	}

	// REGEXP operator (applyUserCanvasTagFilter's predicate shape).
	var matches int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM user_canvas WHERE user_canvas.tags REGEXP ?",
		"(^|,)[[:space:]]*go[[:space:]]*(,|$)",
	).Scan(&matches).Error; err != nil {
		t.Fatalf("REGEXP predicate: %v", err)
	}

	// DELETE ... ORDER BY ... LIMIT (TimeRecordDAO.DeleteOldest) — and the
	// LIMIT must actually bound the deletion, not just parse.
	for i := 0; i < 2; i++ {
		if err := db.Create(&entity.TimeRecord{Data: "vb-mode"}).Error; err != nil {
			t.Fatalf("seed time_record: %v", err)
		}
	}
	defer db.Exec("DELETE FROM time_records WHERE data = ?", "vb-mode")
	if err := NewTimeRecordDAO().DeleteOldest(ctx, db, 1); err != nil {
		t.Fatalf("DeleteOldest (DELETE ... ORDER BY ... LIMIT): %v", err)
	}
	var remaining int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM time_records WHERE data = ?", "vb-mode",
	).Scan(&remaining).Error; err != nil {
		t.Fatalf("count time_records: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining time_records = %d, want 1 (LIMIT must bound the delete)", remaining)
	}
}
