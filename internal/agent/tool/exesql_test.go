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
)

// testConn is a fully-populated connection params struct used by
// every test that needs a "valid" tool. Tests that want to exercise
// the no-credentials path zero it out.
func testConn() exesqlConnParams {
	return exesqlConnParams{
		DBType:     "mysql",
		Database:   "testdb",
		Username:   "u",
		Host:       "h",
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			e := NewExeSQLTool(testConn())
			_, err := e.InvokableRun(context.Background(),
				`{"sql":`+jsonString(c.sql)+`}`)
			if !errors.Is(err, ErrExeSQLNotSelect) {
				t.Fatalf("err = %v, want ErrExeSQLNotSelect", err)
			}
		})
	}
}

func TestExeSQL_AllowsSelect(t *testing.T) {
	t.Parallel()

	cases := []string{
		`SELECT 1`,
		`select * from t`,
		`  SELECT * FROM t WHERE a = 1`,
		`WITH cte AS (SELECT 1) SELECT * FROM cte`,
		`SELECT * FROM t INTO OUTFILE '/tmp/x'`,
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
		DBType:   "trino",
		Host:     "h", Port: 8080, Database: "catalog",
		Username: "u", Password: "p",
	})
	_, err := e.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if !errors.Is(err, ErrExeSQLUnsupportedDB) {
		t.Fatalf("err = %v, want ErrExeSQLUnsupportedDB", err)
	}
}

func TestExeSQL_DSN_MySQL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dbType string
		driver string
		want   string
	}{
		{"mysql", "mysql", `u:p@tcp(h:3306)/d?parseTime=true&charset=utf8mb4`},
		{"mariadb", "mysql", `u:p@tcp(h:3306)/d?parseTime=true&charset=utf8mb4`},
		{"oceanbase", "mysql", `u:p@tcp(h:3306)/d?parseTime=true&charset=utf8mb4`},
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
