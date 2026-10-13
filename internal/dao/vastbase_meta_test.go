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
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/server/config"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// openVastbaseMetaForSQL builds a gorm session speaking the vastbase dialect
// without ever connecting: the postgres dialector defers sql.Open, and
// DisableAutomaticPing keeps gorm.Open from touching the pool. DryRun then
// exercises the SQL builders (ToSQL, DataTypeOf, the migrator) offline.
func openVastbaseMetaForSQL(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(newVastbaseDialector(config.VastbaseMetaConfig{
		DatabaseName: "rag_flow", Host: "vastbase", Port: 5432,
		User: "ragflow", Password: "secret",
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open vastbase dialector: %v", err)
	}
	return db
}

// TestBuildVastbaseMetaDSN pins the DSN contract: quoted values, ssl resolved
// by host when unset, explicit ssl preserved, password omitted when empty.
func TestBuildVastbaseMetaDSN(t *testing.T) {
	cfg := config.VastbaseMetaConfig{
		DatabaseName: "rag_flow", Host: "vastbase", Port: 5432,
		User: "ragflow", Password: "Infini_Rag@123",
	}
	got := buildVastbaseMetaDSN(cfg)
	for _, want := range []string{
		`host='vastbase'`, `port=5432`, `user='ragflow'`,
		`dbname='rag_flow'`, `sslmode='verify-full'`,
		`password='Infini_Rag@123'`, `connect_timeout=30`, `statement_timeout=300000`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("remote-host DSN %q missing %q", got, want)
		}
	}

	// Loopback hosts default to plaintext.
	cfg.Host = "localhost"
	if got := buildVastbaseMetaDSN(cfg); !strings.Contains(got, `sslmode='disable'`) {
		t.Errorf("loopback DSN %q should default sslmode=disable", got)
	}

	// An explicit ssl_mode always wins.
	cfg.SSLMode = "require"
	if got := buildVastbaseMetaDSN(cfg); !strings.Contains(got, `sslmode='require'`) {
		t.Errorf("explicit ssl DSN %q should keep sslmode=require", got)
	}

	// Values with quotes and backslashes stay inside the quoted keyword value.
	cfg.Host = `va'st\host`
	if got := buildVastbaseMetaDSN(cfg); !strings.Contains(got, `host='va\'st\\host'`) {
		t.Errorf("special-character DSN %q should escape the host value", got)
	}

	// Empty password: the keyword is dropped rather than sending an empty one.
	cfg.Host, cfg.Password = "vastbase", ""
	if got := buildVastbaseMetaDSN(cfg); strings.Contains(got, "password") {
		t.Errorf("empty-password DSN %q should omit the password keyword", got)
	}
}

// TestVastbaseUpsertModelsHaveNoReturningFields is the guard for the B-mode
// limitation Python worked around with returning_clause=False: Vastbase
// rejects INSERT ... ON CONFLICT ... DO UPDATE combined with RETURNING. GORM
// appends RETURNING exactly when the model's FieldsWithDefaultDBValue is
// non-empty, so every upserted model must stay free of such fields — a new
// field with a database-side default (including a blank one) would silently
// break every vastbase upsert.
func TestVastbaseUpsertModelsHaveNoReturningFields(t *testing.T) {
	db := openVastbaseMetaForSQL(t)
	for _, model := range []interface{}{
		&entity.SystemSettings{},  // migration version marker upsert
		&entity.TenantLangfuse{},  // SaveByTenantID upsert
		&entity.CanvasTemplate{}, // template seeding upsert
	} {
		stmt := &gorm.Statement{DB: db, Dest: model}
		if err := stmt.Parse(model); err != nil {
			t.Fatalf("parse %T: %v", model, err)
		}
		for _, field := range stmt.Schema.FieldsWithDefaultDBValue {
			t.Errorf("%s.%s has a database-side default, which makes the postgres dialect append RETURNING to the ON CONFLICT upsert", stmt.Schema.Table, field.DBName)
		}
	}
}

// TestVastbaseInsertSQLShape checks the generated upsert against PostgreSQL
// grammar: double-quoted identifiers (never MySQL backticks), and no RETURNING
// appended to the ON CONFLICT upsert — the combination Vastbase B mode
// rejects.
func TestVastbaseInsertSQLShape(t *testing.T) {
	db := openVastbaseMetaForSQL(t)

	setting := &entity.SystemSettings{Name: "marker", Value: "v0.27.2", DataType: "string", Source: "migration"}
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(setting)
	})
	if strings.Contains(sql, "`") {
		t.Errorf("upsert SQL %q should not quote identifiers with backticks", sql)
	}
	if !strings.Contains(sql, `"system_settings"`) {
		t.Errorf("upsert SQL %q should quote identifiers the PostgreSQL way", sql)
	}
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("upsert SQL %q combines ON CONFLICT with RETURNING, which Vastbase B mode rejects", sql)
	}
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Errorf("upsert SQL %q lost the ON CONFLICT clause", sql)
	}
}

// TestVastbaseWithoutReturningEscapeHatch pins the config escape hatch: with
// disable_returning set, no statement carries RETURNING — mirroring peewee's
// returning_clause=False for Vastbase builds that reject it outright.
func TestVastbaseWithoutReturningEscapeHatch(t *testing.T) {
	db, err := gorm.Open(newVastbaseDialector(config.VastbaseMetaConfig{
		DatabaseName: "rag_flow", Host: "vastbase", Port: 5432,
		User: "ragflow", Password: "secret", DisableReturning: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open vastbase dialector: %v", err)
	}
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Create(&entity.SystemSettings{Name: "marker", Value: "v", DataType: "string", Source: "migration"})
	})
	if strings.Contains(sql, "RETURNING") {
		t.Errorf("plain insert SQL %q should omit RETURNING under disable_returning", sql)
	}
}
