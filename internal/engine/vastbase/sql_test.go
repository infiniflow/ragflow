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
	"regexp"
	"testing"

	"ragflow/internal/engine/tablequery"
	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

const testVastbaseKBID = "11111111-1111-1111-1111-111111111111"

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

func (c txOptionsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func newTableQueryEngine(t *testing.T) (*Engine, sqlmock.Sqlmock, *driver.TxOptions) {
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

func TestExecuteTableQueryInReadOnlyRolledBackTransaction(t *testing.T) {
	engine, mock, gotTxOptions := newTableQueryEngine(t)

	vq, err := tablequery.Validate(tablequery.Scope{
		TenantID:  "t1",
		TableName: "ragflow_t1",
		KBIDs:     []string{testVastbaseKBID},
		Fields:    tablequery.FieldsFromFieldMap(map[string]interface{}{"title": "Title"}),
	}, tablequery.Query{
		Select: []string{"title"},
		Where:  &tablequery.FilterNode{Field: "title", Op: tablequery.OpEq, Value: "hello"},
	}, true)
	if err != nil {
		t.Fatalf("tablequery.Validate: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT "doc_id", "docnm_kwd", "title" FROM "ragflow_t1" WHERE ("title" = $1) AND ("kb_id" = $2) LIMIT 1024`)).
		WithArgs("hello", testVastbaseKBID).
		WillReturnRows(sqlmock.NewRows([]string{"doc_id", "docnm_kwd", "title"}).AddRow("d1", "doc.xlsx", "hello"))
	mock.ExpectRollback()

	rows, err := engine.ExecuteTableQuery(context.Background(), vq)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["doc_id"] != "d1" || rows[0]["title"] != "hello" {
		t.Fatalf("rows = %#v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if !gotTxOptions.ReadOnly {
		t.Fatal("ExecuteTableQuery must open its transaction READ ONLY")
	}
}

func TestExecuteTableQueryRejectsNilQueryBeforeTouchingDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	if _, err := engine.ExecuteTableQuery(context.Background(), nil); err == nil {
		t.Fatal("must reject nil ValidatedQuery")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
