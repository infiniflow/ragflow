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

package vastbase

import (
	"context"
	"database/sql/driver"
	"fmt"
	"regexp"
	"testing"

	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestRunSQLExecutesInReadOnlyRolledBackTransaction checks the admin SQL
// console's read-only, always-rolled-back execution envelope.
func TestRunSQLExecutesInReadOnlyRolledBackTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectBegin().WithTxOptions(driver.TxOptions{ReadOnly: true})
	// No LIMIT in the input: the engine must append one.
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT "id" FROM "t1" WHERE kb_id = 'kb-1' LIMIT 1024`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("c1"))
	mock.ExpectRollback()

	rows, err := engine.RunSQL(context.Background(), "t1",
		`SELECT "id" FROM "t1" WHERE kb_id = 'kb-1'`, []string{"kb-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["id"] != "c1" {
		t.Fatalf("rows = %#v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRunSQLDataModifyingCTEFailsInsideReadOnlyTransaction checks that even
// data-modifying CTEs fail under the read-only envelope.
func TestRunSQLDataModifyingCTEFailsInsideReadOnlyTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	// The CTE passes the SELECT/WITH prefix checks; only the READ ONLY
	// transaction stops it, with the server rejecting the write.
	mock.ExpectBegin().WithTxOptions(driver.TxOptions{ReadOnly: true})
	mock.ExpectQuery("WITH d AS").
		WillReturnError(fmt.Errorf("cannot execute DELETE in a read-only transaction"))
	mock.ExpectRollback()

	if _, err := engine.RunSQL(context.Background(), "t1",
		`WITH d AS (DELETE FROM "t1" RETURNING *) SELECT * FROM d`, nil, ""); err == nil {
		t.Fatal("data-modifying CTE must fail inside the read-only transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestRunSQLRejectsMalformedInputBeforeTouchingTheDatabase checks statement
// validation happens before any connection is made.
func TestRunSQLRejectsMalformedInputBeforeTouchingTheDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	if _, err := engine.RunSQL(context.Background(), "bad;table", "SELECT 1", nil, ""); err == nil {
		t.Fatal("invalid table names must be rejected")
	}
	for _, bad := range []string{
		"DELETE FROM t1",
		"UPDATE t1 SET x = 1",
		"SELECT 1; DELETE FROM t1",
	} {
		if _, err := engine.RunSQL(context.Background(), "t1", bad, nil, ""); err == nil {
			t.Fatalf("must reject %q", bad)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
