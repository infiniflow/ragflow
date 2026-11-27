// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package dao

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type mysqlDatetimeFixture struct {
	ID    int64      `gorm:"column:id;primaryKey;autoIncrement:false"`
	Value *time.Time `gorm:"column:value;index"`
	Added *time.Time `gorm:"column:added"`
}

func TestMySQLDatetimePrecision_Integration(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: this test needs a throwaway MySQL database")
	}
	db, err := gorm.Open(newMigrationAwareDialector(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	var modifies int
	if err := db.Callback().Raw().Before("gorm:raw").Register("count_datetime_modifications", func(tx *gorm.DB) {
		sql := strings.ToUpper(tx.Statement.SQL.String())
		if strings.HasPrefix(sql, "ALTER TABLE") && strings.Contains(sql, "MODIFY COLUMN") {
			modifies++
		}
	}); err != nil {
		t.Fatal(err)
	}
	createTable := func(t *testing.T, table, ddl string) *gorm.DB {
		t.Helper()
		if db.Migrator().HasTable(table) {
			t.Fatalf("%s already exists; use a throwaway database", table)
		}
		if ddl != "" {
			if err := db.Exec(ddl).Error; err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			if err := db.Migrator().DropTable(table); err != nil {
				t.Errorf("drop %s: %v", table, err)
			}
		})
		return db.Table(table).Session(&gorm.Session{})
	}
	assertType := func(t *testing.T, table, column, want string) {
		t.Helper()
		got, _, exists, err := columnDefinition(t.Context(), db, table, column)
		if err != nil || !exists || got != want {
			t.Fatalf("%s.%s type = %q, exists = %v, error = %v; want %s", table, column, got, exists, err, want)
		}
	}

	for _, precision := range []int{0, 3, 6} {
		t.Run(fmt.Sprintf("existing_%d", precision), func(t *testing.T) {
			table := fmt.Sprintf("mysql_datetime_precision_%d", precision)
			storedType := "datetime"
			if precision > 0 {
				storedType = fmt.Sprintf("datetime(%d)", precision)
			}
			scoped := createTable(t, table, fmt.Sprintf("CREATE TABLE %s (id bigint PRIMARY KEY, value %s NULL)", table, storedType))
			sample := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.Local)
			if err := db.Exec("INSERT INTO "+table+" (id, value) VALUES (?, ?)", 1, sample).Error; err != nil {
				t.Fatal(err)
			}
			var before mysqlDatetimeFixture
			if err := scoped.Select("value").Take(&before, "id = ?", 1).Error; err != nil {
				t.Fatal(err)
			}
			beforeModifies := modifies
			for pass := 0; pass < 2; pass++ {
				if err := scoped.AutoMigrate(&mysqlDatetimeFixture{}); err != nil {
					t.Fatal(err)
				}
			}
			if modifies != beforeModifies {
				t.Fatalf("precision-only migration issued %d MODIFY statements", modifies-beforeModifies)
			}
			assertType(t, table, "value", storedType)
			assertType(t, table, "added", "datetime(3)")
			if !scoped.Migrator().HasIndex(&mysqlDatetimeFixture{}, "Value") {
				t.Fatal("missing index on the preserved datetime column")
			}
			var after mysqlDatetimeFixture
			if err := scoped.Take(&after, "id = ?", 1).Error; err != nil {
				t.Fatal(err)
			}
			if before.Value == nil || after.Value == nil || !before.Value.Equal(*after.Value) {
				t.Fatalf("stored time changed: before %v, after %v", before.Value, after.Value)
			}

			// A necessary ALTER must retain precision while applying other changes.
			changed := &struct {
				ID    int64      `gorm:"column:id;primaryKey;autoIncrement:false"`
				Value *time.Time `gorm:"column:value;not null;comment:clock;index"`
				Added *time.Time `gorm:"column:added"`
			}{}
			if err := scoped.AutoMigrate(changed); err != nil {
				t.Fatal(err)
			}
			assertType(t, table, "value", storedType)
			_, nullable, _, err := columnDefinition(t.Context(), db, table, "value")
			if err != nil || nullable {
				t.Fatalf("NOT NULL change was lost: nullable %v, error %v", nullable, err)
			}
			beforeModifies = modifies
			if err := scoped.AutoMigrate(changed); err != nil {
				t.Fatal(err)
			}
			if modifies != beforeModifies {
				t.Fatal("converged datetime definition was altered again")
			}
		})
	}

	t.Run("new_table", func(t *testing.T) {
		const table = "mysql_datetime_fresh"
		scoped := createTable(t, table, "")
		if err := scoped.AutoMigrate(&mysqlDatetimeFixture{}); err != nil {
			t.Fatal(err)
		}
		assertType(t, table, "value", "datetime(3)")
	})

	t.Run("manual_spec", func(t *testing.T) {
		const table = "mysql_datetime_manual"
		createTable(t, table, "CREATE TABLE "+table+" (value datetime(6) NOT NULL)")
		spec := columnSpec{table: table, column: "value", columnType: "datetime", nullable: true, definition: "datetime NULL", strict: true}
		if err := applyColumnSpec(t.Context(), db, spec); err != nil {
			t.Fatal(err)
		}
		assertType(t, table, "value", "datetime(6)")
		_, nullable, _, err := columnDefinition(t.Context(), db, table, "value")
		if err != nil || !nullable {
			t.Fatalf("nullable change was lost: nullable %v, error %v", nullable, err)
		}
		beforeModifies := modifies
		if err := applyColumnSpec(t.Context(), db, spec); err != nil {
			t.Fatal(err)
		}
		if modifies != beforeModifies {
			t.Fatal("converged manual datetime specification was altered again")
		}
	})
}
