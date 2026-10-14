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
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"testing"

	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

// txOptionsConnector opens the sqlmock connection under its registered DSN
// and wraps it so tests can observe the driver.TxOptions database/sql hands
// to BeginTx; go-sqlmock accepts but discards them, so it offers no
// expectation API for transaction options.
type txOptionsConnector struct {
	drv driver.Driver
	dsn string
	got *driver.TxOptions
}

func (c txOptionsConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return txOptionsConn{Conn: conn, got: c.got}, nil
}

func (c txOptionsConnector) Driver() driver.Driver { return c.drv }

type txOptionsConn struct {
	driver.Conn
	got *driver.TxOptions
}

func (c txOptionsConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	*c.got = opts
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

// QueryContext delegates so database/sql keeps using the query path instead
// of falling back to Prepare on the wrapper.
func (c txOptionsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

// newRunSQLEngine wires an Engine around a sqlmock database that records the
// transaction options in got, so tests can prove RunSQL opens its
// transaction READ ONLY.
func newRunSQLEngine(t *testing.T) (*Engine, sqlmock.Sqlmock, *driver.TxOptions) {
	t.Helper()
	db, mock, err := sqlmock.NewWithDSN(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	got := new(driver.TxOptions)
	recDB := sql.OpenDB(txOptionsConnector{drv: db.Driver(), dsn: t.Name(), got: got})
	t.Cleanup(func() { recDB.Close(); db.Close() })
	return newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, recDB), mock, got
}

// TestRunSQLExecutesInReadOnlyRolledBackTransaction checks the admin SQL
// console's read-only, always-rolled-back execution envelope.
func TestRunSQLExecutesInReadOnlyRolledBackTransaction(t *testing.T) {
	engine, mock, gotTxOptions := newRunSQLEngine(t)

	mock.ExpectBegin()
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
	if !gotTxOptions.ReadOnly {
		t.Fatal("RunSQL must open its transaction READ ONLY")
	}
}

// TestRunSQLDataModifyingCTEFailsInsideReadOnlyTransaction checks that even
// data-modifying CTEs fail under the read-only envelope.
func TestRunSQLDataModifyingCTEFailsInsideReadOnlyTransaction(t *testing.T) {
	engine, mock, gotTxOptions := newRunSQLEngine(t)

	// The CTE passes the SELECT/WITH prefix checks; only the READ ONLY
	// transaction stops it, with the server rejecting the write.
	mock.ExpectBegin()
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
	if !gotTxOptions.ReadOnly {
		t.Fatal("RunSQL must open its transaction READ ONLY")
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
