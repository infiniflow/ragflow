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
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/glebarez/sqlite"
	"github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// tenantModelIDWidth is the width every tenant model-id column converges on.
// It is the same constant migrateColumnWidths widens to on purpose: a spec that
// declared a different width would narrow the column on every startup and let
// the widening migration restore it right after.
const tenantModelIDWidth = int(common.GroupModelRefWidth)

func TestKnowledgeCompileQueuesHaveNoPrimaryKey(t *testing.T) {
	for _, model := range []interface{}{&entity.KnowledgeCompileBacklog{}, &entity.KnowledgeCompileInflight{}} {
		parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse %T schema: %v", model, err)
		}
		if len(parsed.PrimaryFields) != 0 || parsed.FieldsByName["ID"] != nil {
			t.Fatalf("%s must have no primary key or ID", parsed.Table)
		}
		indexes := parsed.ParseIndexes()
		if len(indexes) != 1 {
			t.Fatalf("%s indexes = %v, want one queue index", parsed.Table, indexes)
		}
		for _, index := range indexes {
			if index.Class == "UNIQUE" || len(index.Fields) != 2 ||
				index.Fields[0].DBName != "dataset_id" || index.Fields[1].DBName != "queue_position" {
				t.Fatalf("unexpected queue index: %+v", index)
			}
		}
	}
}

// setupOrphanColumnTestDB builds the narrow shape discardOrphanedColumnValues
// needs: a table holding values left over from an int column widened to hold
// hex model ids.
func setupOrphanColumnTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.Exec("CREATE TABLE legacy_dialog (id TEXT PRIMARY KEY, tenant_llm_id TEXT)").Error; err != nil {
		t.Fatalf("failed to create legacy_dialog: %v", err)
	}
	return db
}

// TestTenantModelIDColumnSpecsMirrorsBackfillFields guards the shared inventory:
// the type repair and the backfill must cover exactly the same columns, and the
// order must be stable so consecutive startups log identically.
func TestTenantModelIDColumnSpecsMirrorsBackfillFields(t *testing.T) {
	want := 0
	for _, fields := range tenantModelIDFields {
		want += len(fields)
	}

	specs := tenantModelIDColumnSpecs()
	if len(specs) != want {
		t.Fatalf("spec count = %d, want %d", len(specs), want)
	}

	got := map[string]columnSpec{}
	for _, spec := range specs {
		got[spec.table+"."+spec.column] = spec
	}

	for table, fields := range tenantModelIDFields {
		for _, field := range fields {
			spec, ok := got[table+"."+field.tenantIDColumn]
			if !ok {
				t.Fatalf("missing spec for %s.%s", table, field.tenantIDColumn)
			}
			wantType := fmt.Sprintf("varchar(%d)", tenantModelIDWidth)
			if spec.columnType != wantType || spec.definition != wantType {
				t.Errorf("%s.%s types = %q/%q, want %q/%q",
					table, field.tenantIDColumn, spec.columnType, spec.definition, wantType, wantType)
			}
			if !spec.nullable {
				t.Errorf("%s.%s nullable = false, want true", table, field.tenantIDColumn)
			}
			if want := []int{tenantModelIDLength, tenantModelIDWidth}; !slices.Equal(spec.keepLengths, want) {
				t.Errorf("%s.%s keepLengths = %v, want %v", table, field.tenantIDColumn, spec.keepLengths, want)
			}
			if strings.Contains(spec.column, "`") {
				t.Errorf("%s.%s column carries a backtick: %q", table, field.tenantIDColumn, spec.column)
			}
		}
	}

	again := tenantModelIDColumnSpecs()
	for i := range specs {
		if !sameColumnSpec(specs[i], again[i]) {
			t.Fatalf("spec order is not deterministic at index %d: %v then %v", i, specs[i], again[i])
		}
	}
}

// sameColumnSpec compares two specs field by field. columnSpec carries a slice,
// so it is no longer comparable with ==.
func sameColumnSpec(a, b columnSpec) bool {
	return a.table == b.table &&
		a.column == b.column &&
		a.columnType == b.columnType &&
		a.nullable == b.nullable &&
		a.definition == b.definition &&
		slices.Equal(a.keepLengths, b.keepLengths)
}

// TestIsDuplicateColumnErrorClassification pins the convergence criteria for
// "the column is already there": MySQL's Error 1060 pairing on the MySQL wire
// and the typed duplicate-object SQLSTATEs on the Vastbase connection. A bare
// "already exists" text match is intentionally out — unrelated failures
// phrase themselves that way.
func TestIsDuplicateColumnErrorClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"mysql duplicate column", errors.New("Error 1060: Duplicate column name 'pipeline_log_id'"), true},
		{"mysql other error", errors.New("Error 1054: Unknown column 'x' in 'where clause'"), false},
		{"pq duplicate column", &pq.Error{Code: "42701", Message: `column "x" of relation "t" already exists`}, true},
		{"pq duplicate relation", &pq.Error{Code: "42P07", Message: `relation "uk_tenant_llm" already exists`}, true},
		{"pq other sqlstate", &pq.Error{Code: "42602", Message: "invalid name syntax"}, false},
		{"wrapped pq duplicate", fmt.Errorf("alter table: %w", &pq.Error{Code: "42701", Message: `column "x" already exists`}), true},
		{"plain already-exists text is not convergence", errors.New("the snapshot already exists"), false},
	}
	for _, tc := range cases {
		if got := isDuplicateColumnError(tc.err); got != tc.want {
			t.Errorf("%s: isDuplicateColumnError = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestModifyColumnTypesSkipsVastbase pins the vastbase bail-out. The specs
// carry MySQL type names that the B-mode catalog never reports back after a
// MODIFY (int comes back as integer), so on the Vastbase connection the step
// would re-issue the ALTER on every startup while AutoMigrate flips the
// column back — the loop measured on a live instance. The step must return
// without touching the database at all. The dialector is the vastbase
// wrapper: the guard targets that type, so a plain postgres dialector stays
// on the aligning path.
func TestModifyColumnTypesSkipsVastbase(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer mockDB.Close()
	db, err := gorm.Open(vastbaseDialector{Dialector: &postgres.Dialector{Config: &postgres.Config{Conn: mockDB}}}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := modifyColumnTypes(context.Background(), db); err != nil {
		t.Fatalf("modifyColumnTypes on vastbase: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("modifyColumnTypes must not query the vastbase catalog: %v", err)
	}
}

// TestDiscardOrphanedColumnValuesResetsOnlyOrphans covers the values a widened
// int column carries into its new life as a hex-id column. "3" is what matters:
// it looks like a model id to every reader, so leaving it behind turns a visible
// migration failure into silent bad lookups. Both live reference shapes — the
// 32-character id and the 35-character gp- reference — have to survive, because
// the sweep also runs when a legacy varchar(32) column is widened.
func TestDiscardOrphanedColumnValuesResetsOnlyOrphans(t *testing.T) {
	db := setupOrphanColumnTestDB(t)
	ctx := context.Background()

	validID := strings.Repeat("a", tenantModelIDLength)
	groupRef := "gp-" + strings.Repeat("c", tenantModelIDWidth-3)
	if len(groupRef) != tenantModelIDWidth {
		t.Fatalf("group ref length = %d, want %d", len(groupRef), tenantModelIDWidth)
	}
	orphan := "3"
	blank := ""
	tooShort := strings.Repeat("b", tenantModelIDLength-1)
	rows := []struct {
		id  string
		val *string
	}{
		{"d1", &orphan},   // leftover int key
		{"d2", nil},       // already unset
		{"d3", &validID},  // real model id
		{"d4", &blank},    // empty string
		{"d5", &tooShort}, // too short
		{"d6", &groupRef}, // gp- group reference
	}
	for _, row := range rows {
		if err := db.Exec("INSERT INTO legacy_dialog (id, tenant_llm_id) VALUES (?, ?)", row.id, row.val).Error; err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	discardOrphanedColumnValues(ctx, db, columnSpec{
		table: "legacy_dialog", column: "tenant_llm_id",
		keepLengths: []int{tenantModelIDLength, tenantModelIDWidth},
	})

	var cleared int64
	if err := db.Raw("SELECT COUNT(1) FROM legacy_dialog WHERE tenant_llm_id IS NULL").Scan(&cleared).Error; err != nil {
		t.Fatalf("count cleared: %v", err)
	}
	if cleared != 4 {
		t.Errorf("cleared rows = %d, want 4", cleared)
	}

	for _, want := range []struct {
		id  string
		val string
	}{
		{"d3", validID},
		{"d6", groupRef},
	} {
		var survivor string
		if err := db.Raw("SELECT tenant_llm_id FROM legacy_dialog WHERE id = ?", want.id).Scan(&survivor).Error; err != nil {
			t.Fatalf("read %s: %v", want.id, err)
		}
		if survivor != want.val {
			t.Errorf("live reference %s was rewritten to %q", want.id, survivor)
		}
	}
}

// TestDiscardOrphanedColumnValuesIsNoopWithoutKeepLength pins the opt-in: a
// spec that declares no width is a plain type change and must not touch data.
func TestDiscardOrphanedColumnValuesIsNoopWithoutKeepLength(t *testing.T) {
	db := setupOrphanColumnTestDB(t)
	ctx := context.Background()

	if err := db.Exec("INSERT INTO legacy_dialog (id, tenant_llm_id) VALUES ('d1', '3')").Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	discardOrphanedColumnValues(ctx, db, columnSpec{table: "legacy_dialog", column: "tenant_llm_id"})

	var survivor string
	if err := db.Raw("SELECT tenant_llm_id FROM legacy_dialog WHERE id = 'd1'").Scan(&survivor).Error; err != nil {
		t.Fatalf("read d1: %v", err)
	}
	if survivor != "3" {
		t.Errorf("tenant_llm_id = %q, want untouched 3", survivor)
	}
}
