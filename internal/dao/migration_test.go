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
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const tenantModelIDWidth = 32

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
			if spec.columnType != "varchar(32)" || spec.definition != "varchar(32)" {
				t.Errorf("%s.%s types = %q/%q, want varchar(32)/varchar(32)",
					table, field.tenantIDColumn, spec.columnType, spec.definition)
			}
			if !spec.nullable {
				t.Errorf("%s.%s nullable = false, want true", table, field.tenantIDColumn)
			}
			if spec.keepLength != tenantModelIDWidth {
				t.Errorf("%s.%s keepLength = %d, want %d", table, field.tenantIDColumn, spec.keepLength, tenantModelIDWidth)
			}
			if strings.Contains(spec.column, "`") {
				t.Errorf("%s.%s column carries a backtick: %q", table, field.tenantIDColumn, spec.column)
			}
		}
	}

	again := tenantModelIDColumnSpecs()
	for i := range specs {
		if specs[i] != again[i] {
			t.Fatalf("spec order is not deterministic at index %d: %v then %v", i, specs[i], again[i])
		}
	}
}

// TestDiscardOrphanedColumnValuesResetsOnlyOrphans covers the values a widened
// int column carries into its new life as a hex-id column. "3" is what matters:
// it looks like a model id to every reader, so leaving it behind turns a visible
// migration failure into silent bad lookups.
func TestDiscardOrphanedColumnValuesResetsOnlyOrphans(t *testing.T) {
	db := setupOrphanColumnTestDB(t)
	ctx := context.Background()

	validID := strings.Repeat("a", tenantModelIDWidth)
	orphan := "3"
	blank := ""
	tooShort := strings.Repeat("b", tenantModelIDWidth-1)
	rows := []struct {
		id  string
		val *string
	}{
		{"d1", &orphan},   // leftover int key
		{"d2", nil},       // already unset
		{"d3", &validID},  // real model id
		{"d4", &blank},    // empty string
		{"d5", &tooShort}, // too short
	}
	for _, row := range rows {
		if err := db.Exec("INSERT INTO legacy_dialog (id, tenant_llm_id) VALUES (?, ?)", row.id, row.val).Error; err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	discardOrphanedColumnValues(ctx, db, columnSpec{
		table: "legacy_dialog", column: "tenant_llm_id", keepLength: tenantModelIDWidth,
	})

	var cleared int64
	if err := db.Raw("SELECT COUNT(1) FROM legacy_dialog WHERE tenant_llm_id IS NULL").Scan(&cleared).Error; err != nil {
		t.Fatalf("count cleared: %v", err)
	}
	if cleared != 4 {
		t.Errorf("cleared rows = %d, want 4", cleared)
	}

	var survivor string
	if err := db.Raw("SELECT tenant_llm_id FROM legacy_dialog WHERE id = 'd3'").Scan(&survivor).Error; err != nil {
		t.Fatalf("read d3: %v", err)
	}
	if survivor != validID {
		t.Errorf("valid model id was rewritten to %q", survivor)
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
