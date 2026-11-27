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

package oceanbase

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"regexp"
	"testing"

	"ragflow/internal/engine/tablequery"

	"github.com/DATA-DOG/go-sqlmock"
)

const testOBKBID = "11111111-1111-1111-1111-111111111111"

type obTxOptionsConnector struct {
	drv driver.Driver
	dsn string
	got *driver.TxOptions
}

func (c obTxOptionsConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return obTxOptionsConn{Conn: conn, got: c.got}, nil
}

func (c obTxOptionsConnector) Driver() driver.Driver { return c.drv }

type obTxOptionsConn struct {
	driver.Conn
	got *driver.TxOptions
}

func (c obTxOptionsConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	*c.got = opts
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func (c obTxOptionsConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func TestExecuteTableQuery_RunsParameterizedInReadOnlyTransaction(t *testing.T) {
	db, mock, err := sqlmock.NewWithDSN(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	gotTx := new(driver.TxOptions)
	recDB := sql.OpenDB(obTxOptionsConnector{drv: db.Driver(), dsn: t.Name(), got: gotTx})
	t.Cleanup(func() { recDB.Close(); db.Close() })

	eng := &Engine{db: recDB, engineType: "oceanbase"}
	vq, err := tablequery.Validate(tablequery.Scope{
		TenantID:  "t1",
		TableName: "ragflow_t1",
		KBIDs:     []string{testOBKBID},
		Fields:    tablequery.FieldsFromFieldMap(map[string]interface{}{"title": "Title"}),
	}, tablequery.Query{
		Select: []string{"title"},
		Where:  &tablequery.FilterNode{Field: "title", Op: tablequery.OpEq, Value: "report"},
	}, true)
	if err != nil {
		t.Fatalf("tablequery.Validate: %v", err)
	}

	wantSQL := "SELECT `doc_id`, `docnm_kwd`, JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.title')) AS `title` " +
		"FROM `ragflow_t1` " +
		"WHERE (JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, '$.title')) = ?) AND (`kb_id` = ?) " +
		"LIMIT 1024"
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(wantSQL)).
		WithArgs("report", testOBKBID).
		WillReturnRows(sqlmock.NewRows([]string{"doc_id", "docnm_kwd", "title"}).AddRow("d1", "doc.xlsx", "report"))
	mock.ExpectRollback()

	rows, err := eng.ExecuteTableQuery(context.Background(), vq)
	if err != nil {
		t.Fatalf("ExecuteTableQuery: %v", err)
	}
	if len(rows) != 1 || rows[0]["doc_id"] != "d1" || rows[0]["title"] != "report" {
		t.Fatalf("rows = %#v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	if !gotTx.ReadOnly {
		t.Fatal("OceanBase ExecuteTableQuery must open its transaction READ ONLY")
	}
}

func TestExecuteTableQuery_RejectsNilQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	eng := &Engine{db: db, engineType: "oceanbase"}

	if _, err := eng.ExecuteTableQuery(context.Background(), nil); err == nil {
		t.Fatal("expected error for nil ValidatedQuery")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
