// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/migrator"
)

func TestMySQLMigrateColumnPreservesDatetimePrecision(t *testing.T) {
	for _, precision := range []int{0, 3, 6} {
		for _, tc := range []struct {
			name  string
			model interface{}
			ddl   string
		}{
			{"inferred type", &struct {
				Value *time.Time `gorm:"column:value"`
			}{}, ""},
			{"explicit type", &struct {
				Value *time.Time `gorm:"column:value;type:datetime"`
			}{}, ""},
			{"explicit precision", &struct {
				Value *time.Time `gorm:"column:value;precision:6"`
			}{}, ""},
			{"nullability", &struct {
				Value *time.Time `gorm:"column:value;not null"`
			}{}, " NOT NULL"},
			{"default", &struct {
				Value *time.Time `gorm:"column:value;default:'2000-01-01 00:00:00'"`
			}{}, " DEFAULT '2000-01-01 00:00:00'"},
			{"comment", &struct {
				Value *time.Time `gorm:"column:value;comment:clock"`
			}{}, " COMMENT 'clock'"},
		} {
			t.Run(fmt.Sprintf("precision_%d/%s", precision, tc.name), func(t *testing.T) {
				db, mock := openMySQLSpecMock(t)
				db.Dialector = migrationAwareDialector{Dialector: db.Dialector.(*mysql.Dialector)}
				db = db.Table("datetime_probe")
				stmt := &gorm.Statement{DB: db}
				if err := stmt.Parse(tc.model); err != nil {
					t.Fatal(err)
				}
				field := stmt.Schema.FieldsByDBName["value"]
				originalPrecision := field.Precision
				storedType := "datetime"
				if precision > 0 {
					storedType = fmt.Sprintf("datetime(%d)", precision)
				}
				column := migrator.ColumnType{
					SQLColumnType:    new(sql.ColumnType),
					NameValue:        sql.NullString{String: "value", Valid: true},
					DataTypeValue:    sql.NullString{String: "datetime", Valid: true},
					ColumnTypeValue:  sql.NullString{String: storedType, Valid: true},
					DecimalSizeValue: sql.NullInt64{Int64: int64(precision), Valid: true},
					NullableValue:    sql.NullBool{Bool: true, Valid: true},
					CommentValue:     sql.NullString{Valid: true},
				}
				var statements []string
				if err := db.Callback().Raw().Before("gorm:raw").Register("record_datetime_ddl", func(tx *gorm.DB) {
					statements = append(statements, tx.Statement.SQL.String())
				}); err != nil {
					t.Fatal(err)
				}
				if tc.ddl != "" {
					ddl := "ALTER TABLE `datetime_probe` MODIFY COLUMN `value` " + storedType + tc.ddl
					mock.ExpectExec(regexp.QuoteMeta(ddl)).WillReturnResult(sqlmock.NewResult(0, 0))
				}
				if err := db.Migrator().MigrateColumn(tc.model, field, column); err != nil {
					t.Fatalf("MigrateColumn: %v", err)
				}
				if tc.ddl == "" && len(statements) != 0 {
					t.Fatalf("precision-only difference issued SQL: %v", statements)
				}
				if field.Precision != originalPrecision {
					t.Fatalf("cached field precision changed from %d to %d", originalPrecision, field.Precision)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
				// The stored type override must not escape into later CREATE/ADD DDL.
				if tc.name == "inferred type" {
					if got := db.Migrator().FullDataTypeOf(field).SQL; got != "datetime(3) NULL" {
						t.Fatalf("new column type = %q, want datetime(3) NULL", got)
					}
				}
			})
		}
	}
}

func TestMySQLMigrateColumnConvertsOtherTypesAndReturnsErrors(t *testing.T) {
	db, mock := openMySQLSpecMock(t)
	db.Dialector = migrationAwareDialector{Dialector: db.Dialector.(*mysql.Dialector)}
	db = db.Table("datetime_probe")
	model := &struct {
		Value *time.Time `gorm:"column:value"`
	}{}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		t.Fatal(err)
	}
	column := migrator.ColumnType{
		SQLColumnType:    new(sql.ColumnType),
		DataTypeValue:    sql.NullString{String: "varchar", Valid: true},
		ColumnTypeValue:  sql.NullString{String: "varchar(32)", Valid: true},
		LengthValue:      sql.NullInt64{Int64: 32, Valid: true},
		DecimalSizeValue: sql.NullInt64{Valid: true},
		NullableValue:    sql.NullBool{Bool: true, Valid: true},
	}
	wantErr := errors.New("alter failed")
	mock.ExpectExec(regexp.QuoteMeta("ALTER TABLE `datetime_probe` MODIFY COLUMN `value` datetime(3) NULL")).WillReturnError(wantErr)
	if err := db.Migrator().MigrateColumn(model, stmt.Schema.FieldsByDBName["value"], column); !errors.Is(err, wantErr) {
		t.Fatalf("MigrateColumn error = %v, want %v", err, wantErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
