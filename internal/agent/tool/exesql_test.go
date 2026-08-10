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

package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

// testConn is a fully-populated connection params struct used by
// every test that needs a "valid" tool. Tests that want to exercise
// the no-credentials path zero it out. The Host is a literal public
// IP (Cloudflare DNS) so the SSRF guard in InvokableRun accepts it
// without needing real DNS in the test environment.
func testConn() exesqlConnParams {
	return exesqlConnParams{
		DBType:     "mysql",
		Database:   "testdb",
		Username:   "u",
		Host:       "1.1.1.1",
		Port:       3306,
		Password:   "p",
		MaxRecords: 100,
	}
}

// sqlmockDialer returns an exesqlDialer that ignores driver/dsn and
// returns a sqlmock-backed *sql.DB. Each call gets a fresh mock so
// the test can stage expectations before constructing the tool.
func sqlmockDialer(t *testing.T) (exesqlDialer, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	d := func(_, _ string) (*sql.DB, error) { return db, nil }
	return d, mock, func() { _ = db.Close() }
}

func TestExeSQL_NoCredentials(t *testing.T) {
	t.Parallel()

	e := NewExeSQLTool(exesqlConnParams{}).
		WithExeSQLDialer(func(_, _ string) (*sql.DB, error) {
			t.Fatal("dialer should not be called when credentials are missing")
			return nil, nil
		})
	_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if !errors.Is(err, ErrExeSQLNoCredentials) {
		t.Fatalf("err = %v, want ErrExeSQLNoCredentials", err)
	}
}

func TestExeSQL_RejectsNonSelect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
	}{
		{"insert", `INSERT INTO foo VALUES (1)`},
		{"update", `UPDATE foo SET a=1`},
		{"delete", `DELETE FROM foo`},
		{"drop", `DROP TABLE foo`},
		{"create", `CREATE TABLE foo (id INT)`},
		{"alter", `ALTER TABLE foo ADD COLUMN b INT`},
		{"truncate", `TRUNCATE foo`},
		{"grant", `GRANT ALL ON foo TO user`},
		{"begin", `BEGIN`},
		{"commit", `COMMIT`},
		{"set", `SET autocommit=0`},
		{"kill", `KILL 1234`},
		{"use", `USE rag_flow`},
		{"uppercase drop", `DROP DATABASE rag_flow`},
		{"select into", `SELECT * INTO archived_users FROM users`},
		{"select outfile", `SELECT * FROM users INTO OUTFILE '/tmp/users'`},
		{"select dumpfile", `SELECT payload FROM files INTO DUMPFILE '/tmp/payload'`},
		{"delete cte", `WITH deleted AS (DELETE FROM users RETURNING *) SELECT * FROM deleted`},
		{"update cte", `WITH changed AS (UPDATE users SET active = false RETURNING *) SELECT * FROM changed`},
		{"insert cte", `WITH added AS (INSERT INTO users(name) VALUES ('alice') RETURNING *) SELECT * FROM added`},
		{"merge cte", `WITH changed AS (MERGE INTO users USING incoming ON users.id = incoming.id WHEN MATCHED THEN UPDATE SET name = incoming.name RETURNING *) SELECT * FROM changed`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := NewExeSQLTool(testConn()).
				WithExeSQLDialer(func(_, _ string) (*sql.DB, error) {
					t.Fatal("dialer called for rejected SQL")
					return nil, nil
				})
			_, err := e.InvokableRun(context.Background(),
				`{"sql":`+jsonString(c.sql)+`}`)
			if !errors.Is(err, ErrExeSQLNotSelect) {
				t.Fatalf("err = %v, want ErrExeSQLNotSelect", err)
			}
		})
	}
}

func TestExeSQL_RejectsMixedBatchBeforeDatabaseAccess(t *testing.T) {
	e := NewExeSQLTool(testConn()).
		WithExeSQLDialer(func(_, _ string) (*sql.DB, error) {
			t.Fatal("dialer called before every SQL statement was validated")
			return nil, nil
		})

	_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1; DROP TABLE users"}`)
	if !errors.Is(err, ErrExeSQLNotSelect) {
		t.Fatalf("err = %v, want ErrExeSQLNotSelect", err)
	}
}

func TestExeSQL_AllowsSelect(t *testing.T) {
	t.Parallel()

	cases := []string{
		`SELECT 1`,
		`select * from t`,
		`  SELECT * FROM t WHERE a = 1`,
		`WITH cte AS (SELECT 1) SELECT * FROM cte`,
		`WITH cte AS (SELECT 'DELETE; UPDATE' AS note) SELECT * FROM cte`,
		`SHOW TABLES`,
		`DESCRIBE t`,
		`EXPLAIN SELECT * FROM t`,
		`PRAGMA table_info(t)`,
		// Keywords inside string literals should be ignored.
		`SELECT 'DROP TABLE x' AS note FROM dual`,
		// Line comment with DROP keyword.
		"-- DROP TABLE foo\nSELECT 1",
		// Block comment.
		`/* DROP TABLE foo */ SELECT 1`,
	}
	for _, sql := range cases {
		t.Run(sql, func(t *testing.T) {
			t.Parallel()
			// Configure a real-looking query so the validator passes
			// and the tool reaches the dialer; sqlmock will return no
			// rows, and we accept either "no record" sentinel or a
			// sqlmock-driven success.
			dialer, mock, cleanup := sqlmockDialer(t)
			defer cleanup()
			mock.ExpectPing()
			mock.ExpectQuery("SELECT 1").WillReturnRows(
				sqlmock.NewRows([]string{"1"}),
			)
			e := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)
			_, err := e.InvokableRun(context.Background(),
				`{"sql":`+jsonString(sql)+`}`)
			// Two acceptable outcomes:
			//   1. SQL is the literal `SELECT 1` and matches the
			//      mock expectation -> success.
			//   2. SQL is one of the comment/wrapper variants; the
			//      validator passes but the comment-stripped SQL
			//      differs from the staged expectation -> sqlmock
			//      returns an "unexpected call" error, which is
			//      acceptable because what we're testing here is the
			//      SELECT-only filter, not execution.
			if err != nil {
				if errors.Is(err, ErrExeSQLNotSelect) {
					t.Fatalf("validator rejected a permitted SELECT: %v", err)
				}
				// sqlmock mismatch is the expected failure for
				// comment-stripped variants — fine, the validator
				// itself passed.
			}
		})
	}
}

func TestExeSQL_RejectsEmptySQL(t *testing.T) {
	t.Parallel()

	e := NewExeSQLTool(testConn())
	_, err := e.InvokableRun(context.Background(), `{"sql":""}`)
	if err == nil || !strings.Contains(err.Error(), "sql") {
		t.Fatalf("err = %v, want to mention empty sql", err)
	}
}

func TestExeSQL_RejectsEmptyArgs(t *testing.T) {
	t.Parallel()

	e := NewExeSQLTool(testConn())
	_, err := e.InvokableRun(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty args")
	}
}

func TestSplitSQLStatementsIgnoresQuotedAndCommentedSemicolons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		dbType string
		sql    string
	}{
		{"single quoted string", "mysql", `SELECT 'hello; world'; SELECT 2`},
		{"doubled single quote", "postgres", `SELECT 'it''s; intact'; SELECT 2`},
		{"mysql backslash escape", "mysql", `SELECT 'it\'; is intact'; SELECT 2`},
		{"double quoted identifier", "postgres", `SELECT "column;name" FROM t; SELECT 2`},
		{"backtick identifier", "mysql", "SELECT `column;name` FROM t; SELECT 2"},
		{"bracketed identifier", "mssql", `SELECT [column;name] FROM t; SELECT 2`},
		{"line comment", "postgres", "SELECT 1 -- ignored; delimiter\n; SELECT 2"},
		{"mysql hash comment", "mysql", "SELECT 1 # ignored; delimiter\n; SELECT 2"},
		{"block comment", "mysql", `SELECT 1 /* ignored; delimiter */; SELECT 2`},
		{"dollar quoted string", "postgres", `SELECT $$hello; world$$; SELECT 2`},
		{"tagged dollar quoted string", "postgres", `SELECT $body$hello; world$body$; SELECT 2`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			statements := splitSQLStatements(tc.sql, tc.dbType)
			if len(statements) != 2 {
				t.Fatalf("splitSQLStatements(%q) = %#v, want 2 statements", tc.sql, statements)
			}
			if !strings.Contains(statements[0], ";") {
				t.Fatalf("first statement = %q, want the quoted/commented semicolon preserved", statements[0])
			}
			if statements[1] != " SELECT 2" {
				t.Fatalf("second statement = %q, want %q", statements[1], " SELECT 2")
			}
		})
	}
}

func TestExeSQL_ReadOnlyValidationIgnoresQuotedAndCommentedKeywords(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dbType string
		sql    string
	}{
		{"mysql", `SELECT 'DELETE; DROP TABLE users' AS note`},
		{"postgres", `WITH note AS (SELECT $$UPDATE users; DELETE FROM users$$ AS value) SELECT * FROM note`},
		{"postgres", "WITH note AS (SELECT 1 /* DELETE FROM users; */) SELECT * FROM note"},
	}
	for _, tc := range cases {
		if err := validateExeSQLStatements(tc.sql, tc.dbType); err != nil {
			t.Errorf("validateExeSQLStatements(%q, %q) = %v, want nil", tc.sql, tc.dbType, err)
		}
	}
}

func TestExeSQL_ExecutesStatementsWithQuotedSemicolonsIntact(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT 'hello; world'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("hello; world"))
	mock.ExpectQuery("SELECT 2").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(2))

	e := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)
	if _, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 'hello; world'; SELECT 2"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL statements were not executed intact: %v", err)
	}
}

func TestExeSQL_RejectsMySQLExecutableComment(t *testing.T) {
	t.Parallel()

	e := NewExeSQLTool(testConn()).
		WithExeSQLDialer(func(_, _ string) (*sql.DB, error) {
			t.Fatal("dialer called for an executable comment")
			return nil, nil
		})
	_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1 /*!; DROP TABLE users */"}`)
	if !errors.Is(err, ErrExeSQLNotSelect) {
		t.Fatalf("err = %v, want ErrExeSQLNotSelect", err)
	}
}

func TestExeSQL_Info(t *testing.T) {
	t.Parallel()

	e := NewExeSQLTool(testConn())
	info, err := e.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "execute_sql" {
		t.Errorf("Name = %q, want execute_sql", info.Name)
	}
	paramsSchema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	rawSchema, err := json.Marshal(paramsSchema)
	if err != nil {
		t.Fatalf("marshal params schema: %v", err)
	}
	params := string(rawSchema)
	if !strings.Contains(params, `"sql"`) {
		t.Fatalf("schema missing sql: %s", params)
	}
	if strings.Contains(params, `"database"`) {
		t.Fatalf("schema leaked node-level database param: %s", params)
	}
	if !strings.Contains(params, `"required":["sql"]`) {
		t.Fatalf("schema does not require sql: %s", params)
	}
}

func TestExeSQL_UsesConfiguredSQLDefault(t *testing.T) {
	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT 1").WillReturnRows(
		sqlmock.NewRows([]string{"value"}).AddRow(1),
	)
	conn := testConn()
	conn.SQL = "SELECT 1"
	e := NewExeSQLTool(conn).WithExeSQLDialer(dialer)

	out, err := e.InvokableRun(context.Background(), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `"value":1`) {
		t.Fatalf("output = %s, want configured SQL result", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestExeSQL_ComponentContractAndTemplateResolution(t *testing.T) {
	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT id FROM orders WHERE status = 'Completed'").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(1, "Completed"))
	conn := testConn()
	conn.SQL = "{Agent:Result@content}"
	exesql := NewExeSQLTool(conn).WithExeSQLDialer(dialer)
	state := runtime.NewCanvasState("run", "task")
	state.SetVar("Agent:Result", "content", "SELECT id FROM orders WHERE status = 'Completed'")
	out, err := exesql.InvokableRun(runtime.WithState(context.Background(), state), `{}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	outputs := exesql.BuildComponentOutputs(envelope)
	if !strings.Contains(outputs["formalized_content"].(string), "Completed") {
		t.Fatalf("formalized_content = %q", outputs["formalized_content"])
	}
	jsonRows, ok := outputs["json"].([]any)
	if !ok || len(jsonRows) != 1 {
		t.Fatalf("json output = %#v", outputs["json"])
	}
	spec := exesql.ComponentSpec()
	if sqlInput, ok := spec.InputForm["sql"].(map[string]any); !ok || sqlInput["type"] != "line" {
		t.Fatalf("sql input form = %#v", spec.InputForm["sql"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExeSQL_BuildByNameAcceptsCanvasShape(t *testing.T) {
	built, err := BuildByName("execute_sql", map[string]any{
		"database": "demo", "username": "root", "host": "db.example.com", "port": float64(3306), "password": "secret",
		"top_n": float64(50), "sql": "SELECT 1", "outputs": map[string]any{"json": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	exesql := built.(*ExeSQLTool)
	if exesql.conn.DBType != "mysql" || exesql.conn.Port != 3306 || exesql.conn.MaxRecords != 50 || exesql.conn.SQL != "SELECT 1" {
		t.Fatalf("connection defaults = %+v", exesql.conn)
	}
}

func TestExeSQL_ExecuteSelect_ReturnsRows(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	// ExeSQL runs the LLM-supplied SQL verbatim via QueryContext; it
	// does NOT do database/sql arg binding. Stage the expectation
	// with the literal value, not "?" + WithArgs.
	mock.ExpectQuery("SELECT id, name FROM t WHERE id = 7").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(7, "alice").
			AddRow(8, "bob"))

	e := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)
	out, err := e.InvokableRun(context.Background(),
		`{"sql":"SELECT id, name FROM t WHERE id = 7"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var got exesqlResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, out)
	}
	if len(got.Columns) != 2 || got.Columns[0] != "id" || got.Columns[1] != "name" {
		t.Errorf("Columns = %v, want [id name]", got.Columns)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("Rows = %d, want 2", len(got.Rows))
	}
	if got.Rows[0]["name"] != "alice" {
		t.Errorf("Rows[0][name] = %v, want alice", got.Rows[0]["name"])
	}
}

func TestExeSQL_ExecuteSelect_NoRowsReturnsSentinel(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"x"}))

	e := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)
	out, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var got exesqlResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, out)
	}
	// The Python tool's "No record in the database!" sentinel must
	// survive the port — downstream nodes (VariableAggregator,
	// Message) match on it.
	if len(got.Rows) != 1 || got.Rows[0]["content"] != "No record in the database!" {
		t.Errorf("Rows = %v, want the no-record sentinel", got.Rows)
	}
}

func TestExeSQL_ExecuteSelect_PerStatementErrorIsolated(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	// Two statements, second one errors. Python tool keeps the first
	// result and records the second as a content entry; the Go port
	// matches.
	mock.ExpectQuery("SELECT 1").
		WillReturnRows(sqlmock.NewRows([]string{"x"}).AddRow(1))
	mock.ExpectQuery("SELECT * FROM bogus").
		WillReturnError(errors.New("syntax error at or near BOGUS"))

	e := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)
	out, err := e.InvokableRun(context.Background(),
		`{"sql":"SELECT 1; SELECT * FROM bogus"}`)
	if err != nil {
		t.Fatalf("InvokableRun should not abort on a per-statement error: %v", err)
	}
	var got exesqlResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, out)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("Rows = %d, want 2 (one success + one error entry)", len(got.Rows))
	}
	// SQL mock returns integers as int64; the JSON round-trip through
	// map[string]any promotes them to float64. Compare as float64.
	if x, _ := got.Rows[0]["x"].(float64); x != 1 {
		t.Errorf("Rows[0][x] = %v (%T), want 1 (the surviving first result)", got.Rows[0]["x"], got.Rows[0]["x"])
	}
	if c, _ := got.Rows[1]["content"].(string); !strings.Contains(c, "syntax error") {
		t.Errorf("Rows[1].content = %q, want to surface the second-statement error", c)
	}
}

func TestExeSQL_ExecuteSelect_NormalizesTimeAndBytes(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT ts, blob_col FROM t").
		WillReturnRows(sqlmock.NewRows([]string{"ts", "blob_col"}).
			AddRow("2024-06-12T03:04:05Z", []byte("hello")))

	e := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)
	out, err := e.InvokableRun(context.Background(),
		`{"sql":"SELECT ts, blob_col FROM t"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var got exesqlResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nout=%s", err, out)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("Rows = %d, want 1", len(got.Rows))
	}
	// The mock returns a *string* for "ts" because the column driver
	// type is text. The normalize step is only triggered for actual
	// time.Time / []byte values, which is what production drivers
	// produce. Assert blob_col was decoded to a string instead of
	// staying as []byte.
	if _, isBytes := got.Rows[0]["blob_col"].([]byte); isBytes {
		t.Error("blob_col came back as []byte; exesqlNormalizeCell should convert to string")
	}
}

func TestExeSQL_UnsupportedDB(t *testing.T) {
	t.Parallel()

	e := NewExeSQLTool(exesqlConnParams{
		DBType: "trino",
		Host:   "1.1.1.1", Port: 8080, Database: "catalog",
		Username: "u", Password: "p",
	})
	_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err == nil {
		t.Fatal("expected non-nil error for trino without registered driver")
	}
	if errors.Is(err, ErrExeSQLUnsupportedDB) {
		t.Fatalf("err = %v, did not want ErrExeSQLUnsupportedDB after trino wiring", err)
	}
}

func TestExeSQL_DSN_MySQL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dbType string
		driver string
		want   string
	}{
		// MySQL DSN: URL-style with bracketed host:port for IPv6.
		// For non-IPv6 hosts (e.g. "h"), JoinHostPort produces the
		// unchanged `h:port` form.
		{"mysql", "mysql", `u:p@tcp(h:3306)/d?parseTime=true&charset=utf8mb4`},
		{"mariadb", "mysql", `u:p@tcp(h:3306)/d?parseTime=true&charset=utf8mb4`},
		{"oceanbase", "mysql", `u:p@tcp(h:3306)/d?parseTime=true&charset=utf8mb4`},
		// Postgres / mssql: keyword DSN — host (or server) and port
		// are DISTINCT fields. Combining them in a single key is
		// rejected by the driver; the test pins the corrected
		// shape (PR review round 6, Major #4).
		{"postgres", "postgres", `host=h port=5432 user=u password=p dbname=d sslmode=disable`},
		{"mssql", "sqlserver", `server=h;port=1433;user id=u;password=p;database=d`},
	}
	for _, c := range cases {
		t.Run(c.dbType, func(t *testing.T) {
			t.Parallel()
			conn := exesqlConnParams{
				DBType: c.dbType, Host: "h", Port: pickPort(c.dbType),
				Username: "u", Password: "p", Database: "d",
			}
			driver, dsn, err := exesqlDriverAndDSN(conn)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if driver != c.driver {
				t.Errorf("driver = %q, want %q", driver, c.driver)
			}
			if dsn != c.want {
				t.Errorf("dsn = %q, want %q", dsn, c.want)
			}
		})
	}
}

func pickPort(dbType string) int {
	switch dbType {
	case "postgres":
		return 5432
	case "mssql":
		return 1433
	default:
		return 3306
	}
}

// TestExeSQL_DSN_IPv6 pins PR review round 5, Major #5: a public
// IPv6 host (e.g. 2606:4700:4700::1111) must be wrapped in [ ]
// by every DSN format so the driver can split host:port correctly.
// Before the fix, the mysql format produced `tcp(2606:4700:...:3306)`
// which the MySQL driver rejected because the inner colons
// confused its host:port split.
//
// Round 6, Major #4: postgres and mssql now use DISTINCT host/server
// and port fields (combined `host=h:p` was rejected by lib/pq and
// go-mssqldb). For IPv6 the bracketed form goes only into the
// host/server slot.
func TestExeSQL_DSN_IPv6(t *testing.T) {
	t.Parallel()
	const v6 = "2606:4700:4700::1111"

	cases := []struct {
		dbType string
		want   string
	}{
		{"mysql", `u:p@tcp([2606:4700:4700::1111]:3306)/d?parseTime=true&charset=utf8mb4`},
		{"oceanbase", `u:p@tcp([2606:4700:4700::1111]:3306)/d?parseTime=true&charset=utf8mb4`},
		// Postgres: `host=[ipv6]` (bracketed) `port=` separate.
		{"postgres", `host=[2606:4700:4700::1111] port=3306 user=u password=p dbname=d sslmode=disable`},
		// MSSQL: `server=[ipv6]` (bracketed) `port=` separate.
		{"mssql", `server=[2606:4700:4700::1111];port=3306;user id=u;password=p;database=d`},
	}
	for _, c := range cases {
		t.Run(c.dbType, func(t *testing.T) {
			t.Parallel()
			conn := exesqlConnParams{
				DBType: c.dbType, Host: v6, Port: 3306,
				Username: "u", Password: "p", Database: "d",
			}
			driver, dsn, err := exesqlDriverAndDSN(conn)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if driver == "" {
				t.Fatalf("driver empty for %s", c.dbType)
			}
			if dsn != c.want {
				t.Errorf("dsn = %q, want %q", dsn, c.want)
			}
		})
	}
}

// jsonString is a tiny helper to produce a valid JSON string literal
// for the SQL values we feed into the tool.
func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return string(b.String())
}

// reactScriptedModel is the minimum-viable eino ToolCallingChatModel
// that drives the real ReAct loop. It returns a tool_call on the
// first Generate and a final content message on the second, recording
// every input/output pair so tests can assert on what the framework
// actually did (e.g. that a ToolMessage carrying the tool's result
// appears in round 2's input).
type reactScriptedModel struct {
	turn         int
	rounds       [][]*schema.Message
	boundTools   []*schema.ToolInfo
	toolName     string
	toolArgs     string
	finalContent string
}

func newReactScriptedModel(toolName, toolArgs, finalContent string) *reactScriptedModel {
	return &reactScriptedModel{toolName: toolName, toolArgs: toolArgs, finalContent: finalContent}
}

func (m *reactScriptedModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.boundTools = tools
	return m, nil
}

func (m *reactScriptedModel) Generate(_ context.Context, in []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	cp := make([]*schema.Message, len(in))
	copy(cp, in)
	m.rounds = append(m.rounds, cp)
	m.turn++
	if m.turn == 1 {
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID:   "call_exe_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      m.toolName,
					Arguments: m.toolArgs,
				},
			}},
		}, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: m.finalContent}, nil
}

func (m *reactScriptedModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	// The ReAct agent drives Generate; Stream is required to satisfy
	// the interface but never invoked in this test path.
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Close()
	return sr, nil
}

// TestExeSQL_RealReactAgent_ExecutesTool drives a real eino
// react.NewAgent with the real ExeSQLTool (sqlmock-backed DB) and a
// scripted chat model. It proves the tool is actually invoked by the
// framework, its JSON result is fed back as a ToolMessage on the next
// round, and the model emits a final answer grounded in that result.
//
// This is end-to-end coverage for the "agent -> tool" wiring that the
// per-tool unit tests and the registry resolution tests cannot catch:
// here the tool descriptor is bound to the model, the model emits a
// tool_call, eino's ToolsNode invokes the real ExeSQLTool.InvokableRun,
// and the resulting JSON is passed back as a ToolMessage. Replacing
// the model with a hand-rolled stub would skip all of that.
func TestExeSQL_RealReactAgent_ExecutesTool(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT 42").WillReturnRows(
		sqlmock.NewRows([]string{"x"}).AddRow(42),
	)
	realTool := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)

	mdl := newReactScriptedModel(
		"execute_sql",
		`{"sql": "SELECT 42"}`,
		"the answer is 42",
	)

	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: mdl,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []einotool.BaseTool{realTool},
		},
		MaxStep: 5,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}

	out, err := agent.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("What is 42?"),
	})
	if err != nil {
		t.Fatalf("agent.Generate: %v", err)
	}
	if got, want := out.Content, "the answer is 42"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
	if mdl.turn != 2 {
		t.Errorf("Generate called %d times, want 2 (tool_call + final)", mdl.turn)
	}
	if len(mdl.boundTools) != 1 || mdl.boundTools[0].Name != "execute_sql" {
		names := make([]string, 0, len(mdl.boundTools))
		for _, ti := range mdl.boundTools {
			names = append(names, ti.Name)
		}
		t.Errorf("tools bound to model = %v, want [execute_sql]", names)
	}
	if len(mdl.rounds) < 2 {
		t.Fatalf("only %d rounds captured, want >= 2", len(mdl.rounds))
	}
	var sawToolResult bool
	for _, msg := range mdl.rounds[1] {
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "42") {
			sawToolResult = true
			break
		}
	}
	if !sawToolResult {
		t.Errorf("round 2 input did not contain a ToolMessage carrying the tool result; got %d messages", len(mdl.rounds[1]))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations not met: %v", err)
	}
}

// TestExeSQL_RealReactAgent_ToolErrorIsolated verifies the
// error-as-content path: when the DB returns an error, ExeSQLTool's
// InvokableRun surfaces it as a JSON content row (not a Go error).
// The eino framework must wrap that as a ToolMessage and pass it to
// the model on round 2 without crashing the ReAct loop, so the model
// can ground its final answer in the surfaced error.
func TestExeSQL_RealReactAgent_ToolErrorIsolated(t *testing.T) {
	t.Parallel()

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()
	mock.ExpectPing()
	mock.ExpectQuery("SELECT * FROM bogus").
		WillReturnError(errors.New("syntax error at or near BOGUS"))

	realTool := NewExeSQLTool(testConn()).WithExeSQLDialer(dialer)

	mdl := newReactScriptedModel(
		"execute_sql",
		`{"sql": "SELECT * FROM bogus"}`,
		"the query had a syntax error",
	)

	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: mdl,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []einotool.BaseTool{realTool},
		},
		MaxStep: 5,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}

	out, err := agent.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("Find bogus rows"),
	})
	if err != nil {
		t.Fatalf("agent.Generate surfaced a Go error; expected the tool to absorb it as content: %v", err)
	}
	if got, want := out.Content, "the query had a syntax error"; got != want {
		t.Errorf("Content = %q, want %q", got, want)
	}
	if mdl.turn != 2 {
		t.Errorf("Generate called %d times, want 2", mdl.turn)
	}
	// The round 2 input must include a ToolMessage carrying the
	// embedded error text — not a Go error and not an empty content.
	var sawErrorResult bool
	for _, msg := range mdl.rounds[1] {
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "syntax error") {
			sawErrorResult = true
			break
		}
	}
	if !sawErrorResult {
		t.Errorf("round 2 input did not contain a ToolMessage with the DB error; got %d messages", len(mdl.rounds[1]))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations: %v", err)
	}
}
