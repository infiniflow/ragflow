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

// Package tool — ExeSQL tool.
//
// ExeSQL lets an Agent component execute a SQL statement on a
// user-configured external database and return the rows as
// column→value maps. It mirrors the Python `agent/tools/exesql.py`
// semantics: per-invocation open/close of a fresh `database/sql` connection
// scoped to the tool's params (host/port/database/username/password),
// no ORM, no GORM — those layers are for RAGFlow's own metadata DB
// (`internal/dao`) and would be the wrong abstraction here.
//
// Why `database/sql` (not GORM, not sqlx):
//   - The Python equivalent uses `pymysql.connect()` / `psycopg2.connect()`
//     directly with no ORM in the path.
//   - ExeSQL returns `[]map[string]any` (dynamic schema, LLM-supplied
//     SQL), so there is nothing to struct-scan — sqlx's headline
//     feature would be unused.
//   - GORM is for object-relational mapping of RAGFlow's metadata
//     tables. Reusing `internal/dao`'s GORM here would couple the
//     tool to RAGFlow's own DB pool and require an `internal/dao`
//     connection for a tool that has no business touching RAGFlow's
//     metadata at all.
//   - `database/sql` is stdlib, no extra runtime dependency beyond
//     the per-driver package (`go-sql-driver/mysql`, `lib/pq`,
//     `denisenkom/go-mssqldb`).
package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	// SQL drivers — registered via their init() side effects.
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ExeSQL-specific errors. ErrExeSQLDAOMissing is surfaced when
// no DAO is registered. The current implementation routes
// through `database/sql` (see openSQLDB below).
// implementation in place, the error surface is:
//   - ErrExeSQLNotSelect: SQL failed the SELECT-only safety filter.
//   - ErrExeSQLNoCredentials: the tool has no db_type/host/etc. set
//     (caller forgot to wire the connection params).
//   - ErrExeSQLUnsupportedDB: db_type is one we have not ported yet
//     (trino, IBM DB2).
var (
	ErrExeSQLNotSelect = errors.New(
		"ExeSQL: only SELECT statements are allowed; " +
			"INSERT/UPDATE/DELETE/DDL is rejected for safety",
	)
	ErrExeSQLNoCredentials = errors.New(
		"ExeSQL: connection params not configured (db_type/host/port/database/username/password)",
	)
	ErrExeSQLUnsupportedDB = errors.New(
		"ExeSQL: db_type not yet supported in Go port (trino, IBM DB2 are pending)",
	)
)

const (
	exesqlToolName        = "execute_sql"
	exesqlToolDescription = "This is a tool that can execute SQL."

	exesqlDefaultMaxRecords = 1024
	exesqlDefaultTimeout    = 60 * time.Second
)

// exesqlConnParams captures the user-supplied DB connection details.
// These are tool-level config (set on the canvas node, not exposed
// to the LLM at function-call time), matching the Python ExeSQLParam
// fields. The LLM only sees `sql` and optional `database` in args.
type exesqlConnParams struct {
	DBType     string // mysql | postgres | mariadb | mssql | oceanbase
	Database   string
	Username   string
	Host       string
	Port       int
	Password   string
	MaxRecords int
}

// ExeSQLConnParams is the public alias of exesqlConnParams for
// external callers (e.g. internal/agent/component/Universe A
// delegation wrappers). The internal lowercase name stays for
// backward-compat with existing in-package callers.
type ExeSQLConnParams = exesqlConnParams

// NewExeSQLConnParams decodes a canvas-node params map into an
// ExeSQLConnParams. Returns an error if any required field
// (db_type, host, database, username) is missing.
//
// Callers (e.g. the Universe A exesqlComponent wrapper) build the
// params map from the canvas DSL; the tool-side decoding stays
// in this package so the schema lives next to the type.
func NewExeSQLConnParams(params map[string]any) (ExeSQLConnParams, error) {
	conn := ExeSQLConnParams{}
	if v, ok := params["db_type"].(string); ok {
		conn.DBType = v
	}
	if v, ok := params["database"].(string); ok {
		conn.Database = v
	}
	if v, ok := params["username"].(string); ok {
		conn.Username = v
	}
	if v, ok := params["host"].(string); ok {
		conn.Host = v
	}
	if v, ok := params["port"].(int); ok {
		conn.Port = v
	}
	if v, ok := params["password"].(string); ok {
		conn.Password = v
	}
	if v, ok := params["max_records"].(int); ok {
		conn.MaxRecords = v
	}
	if conn.DBType == "" || conn.Host == "" || conn.Username == "" || conn.Database == "" {
		return conn, fmt.Errorf("ExeSQL: missing required connection params (db_type/host/database/username)")
	}
	return conn, nil
}

// exesqlArgs is the JSON shape the model sends in. Matches the Python
// ExeSQLParam ToolMeta (sql is required, database is optional).
type exesqlArgs struct {
	SQL      string `json:"sql"`
	Database string `json:"database,omitempty"`
}

// exesqlResult is the JSON envelope returned to the model. The shape
// matches the Python tool's `rows` / `_ERROR` output convention so
// downstream nodes can pattern-match unchanged. `Columns` lets the
// caller preserve order even when `Rows` is empty.
type exesqlResult struct {
	Columns []string         `json:"columns,omitempty"`
	Rows    []map[string]any `json:"rows,omitempty"`
	Error   string           `json:"_ERROR,omitempty"`
}

// exesqlDialer abstracts `sql.Open` for test injection. Production code
// uses defaultExeSQLDialer (== sql.Open). Tests inject a dialer that
// returns a `*sql.DB` backed by DATA-DOG/go-sqlmock so no real DB is
// required.
//
// Returning a *sql.DB (not a Tx) means each statement gets its own
// fresh conn from the pool, matching the Python tool's
// open → execute → close lifecycle.
type exesqlDialer func(driver, dsn string) (*sql.DB, error)

func defaultExeSQLDialer(driver, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	// The Python tool does not pool — every call is a fresh connect.
	// Match that: set MaxOpenConns=1 and Close() in InvokableRun so
	// we never leak a connection. Timeouts are honoured by the caller's
	// context.
	db.SetMaxOpenConns(1)
	return db, nil
}

// ExeSQLTool is the ExeSQL tool.
// It validates SELECT-only at the parser level and executes the
// statement against a user-configured external DB via `database/sql`.
type ExeSQLTool struct {
	conn   exesqlConnParams
	dialer exesqlDialer
}

// NewExeSQLTool returns an ExeSQLTool wired to the given connection
// params. The dialer defaults to `sql.Open`; tests can pass a
// sqlmock-backed dialer via WithExeSQLDialer.
func NewExeSQLTool(conn exesqlConnParams) *ExeSQLTool {
	if conn.MaxRecords <= 0 {
		conn.MaxRecords = exesqlDefaultMaxRecords
	}
	return &ExeSQLTool{
		conn:   conn,
		dialer: defaultExeSQLDialer,
	}
}

// WithExeSQLDialer swaps the connection opener. Returns the tool for
// chaining. Used by tests; production code should leave it alone.
func (e *ExeSQLTool) WithExeSQLDialer(d exesqlDialer) *ExeSQLTool {
	if d != nil {
		e.dialer = d
	}
	return e
}

// Info returns the tool's metadata for the chat model. Mirrors the
// Python ExeSQLParam ToolMeta: only `sql` (and optional `database`)
// are visible to the LLM. Connection params are not exposed here —
// they're set on the tool instance, matching the Python convention
// where ExeSQLParam fields like `db_type` / `host` are tool
// configuration, not function-call arguments.
func (e *ExeSQLTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: exesqlToolName,
		Desc: exesqlToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"sql": {
				Type:     schema.String,
				Desc:     "The SQL statement to execute. Must be a SELECT (read-only).",
				Required: true,
			},
			"database": {
				Type:     schema.String,
				Desc:     "Optional target database / schema name. Overrides the tool's configured DB.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun validates the SQL, opens a fresh connection scoped to
// the tool's params, executes each semicolon-separated statement, and
// returns the rows. Per-statement errors do not abort the node: they
// are accumulated in the `Errors` slice of the response (the Python
// tool does the same — `sql_res.append({"content": msg})`).
func (e *ExeSQLTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if argumentsInJSON == "" {
		return exesqlErrorResult(errors.New("exesql: empty arguments")), errors.New("exesql: empty arguments")
	}
	var args exesqlArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return exesqlErrorResult(fmt.Errorf("exesql: parse arguments: %w", err)),
			fmt.Errorf("exesql: parse arguments: %w", err)
	}
	if strings.TrimSpace(args.SQL) == "" {
		return exesqlErrorResult(errors.New("exesql: empty sql")), errors.New("exesql: empty sql")
	}
	if !isSelectStatement(args.SQL) {
		return exesqlErrorResult(ErrExeSQLNotSelect), ErrExeSQLNotSelect
	}

	// Honor the per-call `database` override if the model supplied one;
	// fall back to the tool's configured DB.
	conn := e.conn
	if args.Database != "" {
		conn.Database = args.Database
	}
	if err := conn.check(); err != nil {
		return exesqlErrorResult(err), err
	}

	// The DB host/port are node-author-controlled and are connected to
	// server-side, so guard against SSRF (internal hosts, loopback, cloud
	// metadata) before any driver dispatch — mirroring the
	// `test_db_connection` endpoint guard. Connect to the validated,
	// resolved public IP so a later DNS change cannot rebind the host
	// to an internal address (mirrors agent/tools/exesql.py PR #15609).
	safeHost, ssrfErr := ValidateDBHost(conn.Host)
	if ssrfErr != nil {
		return exesqlErrorResult(ssrfErr), ssrfErr
	}
	conn.Host = safeHost

	driver, dsn, err := exesqlDriverAndDSN(conn)
	if err != nil {
		return exesqlErrorResult(err), err
	}

	db, err := e.dialer(driver, dsn)
	if err != nil {
		return exesqlErrorResult(fmt.Errorf("exesql: open %s: %w", driver, err)),
			fmt.Errorf("exesql: open %s: %w", driver, err)
	}
	defer db.Close()

	// Apply a wall-clock timeout if the caller did not provide one.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, exesqlDefaultTimeout)
		defer cancel()
	}

	if err := db.PingContext(ctx); err != nil {
		return exesqlErrorResult(fmt.Errorf("exesql: ping: %w", err)),
			fmt.Errorf("exesql: ping: %w", err)
	}

	res, err := exesqlExecute(ctx, db, args.SQL, conn.MaxRecords)
	if err != nil {
		return exesqlErrorResult(err), err
	}
	return exesqlMarshalResult(res)
}

// exesqlExecute splits the SQL on `;` (Python parity) and runs each
// statement independently. A failing statement is recorded as an
// error entry but does not abort subsequent statements — this is
// the same isolation guarantee the Python tool provides so that
// earlier results survive a bad statement later in the batch.
func exesqlExecute(ctx context.Context, db *sql.DB, sqlText string, maxRows int) (*exesqlResult, error) {
	stmts := splitSQLStatements(sqlText)
	res := &exesqlResult{}
	for _, stmt := range stmts {
		stmt = stripChunkIDMarkers(stmt)
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		cols, rows, err := exesqlQueryOne(ctx, db, stmt, maxRows)
		if err != nil {
			// Keep going; the Python tool appends {"content": msg}
			// and continues with the next statement.
			res.Rows = append(res.Rows, map[string]any{
				"content": "SQL Execution Failed: " + stmt + "\n" + err.Error(),
			})
			continue
		}
		if len(res.Columns) == 0 && len(cols) > 0 {
			res.Columns = cols
		}
		res.Rows = append(res.Rows, rows...)
	}
	if len(res.Rows) == 0 {
		// Mirror the Python tool's "no record" sentinel so downstream
		// nodes (VariableAggregator, Message) can match on it.
		// Trigger on zero row data; keep any columns that the first
		// statement populated so the schema survives.
		res.Rows = []map[string]any{{"content": "No record in the database!"}}
	}
	return res, nil
}

// exesqlQueryOne runs a single statement and returns columns + rows.
// Rows are returned as column→value maps with `time.Time` flattened
// to "YYYY-MM-DD" (Python pandas `.dt.strftime('%Y-%m-%d')` parity).
func exesqlQueryOne(ctx context.Context, db *sql.DB, stmt string, maxRows int) ([]string, []map[string]any, error) {
	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	out := make([]map[string]any, 0, 16)
	for rows.Next() {
		if len(out) >= maxRows {
			break
		}
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = exesqlNormalizeCell(raw[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return cols, out, nil
}

// exesqlNormalizeCell mirrors the Python tool's per-cell conversions:
//   - time.Time -> "YYYY-MM-DD" (Python: df[col].dt.strftime('%Y-%m-%d'))
//   - []byte  -> string (most drivers return text columns as []byte;
//     decoding to string gives JSON-friendly output and matches the
//     Python "df.to_dict(orient='records')" serialization).
//   - nil / NaN / Inf -> JSON null (Python: df.where(pd.notnull(df), None))
//   - everything else passes through unchanged.
func exesqlNormalizeCell(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case time.Time:
		return x.Format("2006-01-02")
	case []byte:
		return string(x)
	case float64:
		// JSON does not represent NaN / Inf; the Python tool drops
		// them via `convert_decimals` -> None.
		if isBadFloat(x) {
			return nil
		}
		return x
	}
	return v
}

func isBadFloat(f float64) bool {
	// NaN != NaN; Abs(Inf) > max finite. Avoid importing math.
	if f != f {
		return true
	}
	if f > 1e308 || f < -1e308 {
		return true
	}
	return false
}

// splitSQLStatements splits on `;`, ignoring semicolons inside string
// literals and line/block comments. This matches what the Python
// tool does with `sqls = sql.split(";")` — a naive split, but safe
// enough for read-only statements the LLM is expected to produce.
func splitSQLStatements(s string) []string {
	cleaned := stripSQLStrings(stripSQLComments(s))
	return strings.Split(cleaned, ";")
}

// stripChunkIDMarkers drops the [ID:123] tokens the RAGFlow chunker
// sometimes embeds in SQL strings (`re.sub(r"\[ID:[0-9]+\]", "", ...)`
// in the Python tool).
var exesqlChunkIDRe = regexp.MustCompile(`\[ID:[0-9]+\]`)

func stripChunkIDMarkers(s string) string { return exesqlChunkIDRe.ReplaceAllString(s, "") }

// exesqlErrorResult returns the JSON envelope for an error path.
func exesqlErrorResult(err error) string {
	b, _ := json.Marshal(exesqlResult{Error: err.Error()})
	return string(b)
}

func exesqlMarshalResult(r *exesqlResult) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("exesql: marshal result: %w", err)
	}
	return string(b), nil
}

// exesqlDriverAndDSN maps a (db_type, conn) tuple to the registered
// driver name and DSN. OceanBase reuses the MySQL driver with a
// utf8mb4 charset — same trick the Python tool pulls in
// `pymysql.connect(..., charset='utf8mb4')`.
//
// IPv6 safety: `ValidateDBHost` (PR #15609) can return a public IPv6
// literal (e.g. "2001:db8::1"). The MySQL driver requires bracketed
// host:port for IPv6 — we route the MySQL/OceanBase paths through
// net.JoinHostPort so an IPv6 host produces `tcp([2001:db8::1]:3306)`.
//
// Driver-specific format rules (PR review round 6, Major #4):
//   - mysql / oceanbase: `tcp(<host:port>)` URL form — host:port is
//     a single bracketed value (JoinHostPort handles IPv6).
//   - lib/pq: keyword=value DSN — `host=` and `port=` are DISTINCT
//     fields. Combining them as `host=h:p` is rejected by the driver.
//   - go-mssqldb (denisenkom): ADO-style DSN — `server=` and `port=`
//     are DISTINCT fields. `server=h:p;port=p` is also rejected.
func exesqlDriverAndDSN(c exesqlConnParams) (driver, dsn string, err error) {
	mysqlHostPort := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	switch strings.ToLower(c.DBType) {
	case "mysql", "mariadb":
		return "mysql", fmt.Sprintf(
			"%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4",
			c.Username, c.Password, mysqlHostPort, c.Database,
		), nil
	case "oceanbase":
		// OceanBase MySQL-compat mode: same driver, MySQL wire protocol.
		return "mysql", fmt.Sprintf(
			"%s:%s@tcp(%s)/%s?parseTime=true&charset=utf8mb4",
			c.Username, c.Password, mysqlHostPort, c.Database,
		), nil
	case "postgres", "postgresql":
		// lib/pq: keyword DSN — host and port are separate fields.
		// For IPv6, lib/pq accepts `host=[2001:db8::1]` (the bracketed
		// form is the documented IPv6 representation).
		pgHost := c.Host
		if strings.Contains(pgHost, ":") {
			pgHost = "[" + pgHost + "]"
		}
		return "postgres", fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			pgHost, c.Port, c.Username, c.Password, c.Database,
		), nil
	case "mssql", "sqlserver":
		// denisenkom/go-mssqldb: ADO-style DSN — server and port are
		// separate fields. For IPv6, the ADO form requires the
		// bracketed host. We use the bracketed form whenever the host
		// contains a colon (the unambiguous IPv6 marker).
		msHost := c.Host
		if strings.Contains(msHost, ":") {
			msHost = "[" + msHost + "]"
		}
		return "sqlserver", fmt.Sprintf(
			"server=%s;port=%d;user id=%s;password=%s;database=%s",
			msHost, c.Port, c.Username, c.Password, c.Database,
		), nil
	case "trino":
		return "trino", trinoDSN(c), nil
	case "ibm db2":
		return "", "", fmt.Errorf("%w: ibm db2", ErrExeSQLUnsupportedDB)
	default:
		return "", "", fmt.Errorf("ExeSQL: unknown db_type %q", c.DBType)
	}
}

// check returns ErrExeSQLNoCredentials when the required fields are
// missing. Mirrors the Python ExeSQLParam.check() but stripped of
// the UI-specific "empty value" messages.
func (c exesqlConnParams) check() error {
	if c.DBType == "" || c.Host == "" || c.Username == "" || c.Database == "" {
		return ErrExeSQLNoCredentials
	}
	if c.Port <= 0 {
		return fmt.Errorf("ExeSQL: invalid port %d", c.Port)
	}
	return nil
}

// ---------------------------------------------------------------------------
// SELECT-only safety validator
// ---------------------------------------------------------------------------

// leadingKeywordRe matches the first non-comment, non-whitespace keyword
// in a SQL statement. Comments (-- line, /* block */) and string literals
// are stripped before the match.
var leadingKeywordRe = regexp.MustCompile(`^[\s,;(]*([A-Za-z]+)`)

// nonSelectKeywords lists DML/DDL/DCL verbs the parser rejects. These
// are the only top-level forms we refuse; everything else (WITH ... SELECT,
// SELECT INTO, SHOW, DESCRIBE, EXPLAIN) is allowed because they're
// read-only.
var nonSelectKeywords = map[string]struct{}{
	"INSERT": {}, "UPDATE": {}, "DELETE": {}, "REPLACE": {},
	"TRUNCATE": {},
	"CREATE":   {}, "DROP": {}, "ALTER": {}, "RENAME": {},
	"GRANT": {}, "REVOKE": {},
	"LOCK": {}, "UNLOCK": {},
	"CALL": {}, "EXEC": {}, "EXECUTE": {},
	"COPY":   {},
	"VACUUM": {}, "ANALYZE": {},
	"SET": {}, "RESET": {},
	"USE":        {},
	"KILL":       {},
	"LOAD":       {},
	"CHECKPOINT": {},
	"BEGIN":      {}, "COMMIT": {}, "ROLLBACK": {}, "START": {},
	"SHUTDOWN": {},
}

// isSelectStatement returns true iff sql is a read-only statement. The
// heuristic is intentionally narrow: strip line + block comments and
// string literals, scan the first keyword, and reject if it's a
// DML/DDL/DCL verb. SQL parsers in Go stdlib don't exist; this matches
// the safety bar the Go shell needs.
func isSelectStatement(sql string) bool {
	cleaned := stripSQLComments(sql)
	cleaned = stripSQLStrings(cleaned)
	m := leadingKeywordRe.FindStringSubmatch(cleaned)
	if len(m) < 2 {
		return false
	}
	kw := strings.ToUpper(m[1])
	if _, bad := nonSelectKeywords[kw]; bad {
		return false
	}
	switch kw {
	case "SELECT", "WITH", "SHOW", "DESCRIBE", "DESC", "EXPLAIN", "PRAGMA":
		return true
	}
	// Unknown verb → conservative reject. The Python tool would forward
	// this; the Go shell declines to execute without a recognized form.
	return false
}

// stripSQLComments removes -- line comments and /* ... */ block comments.
// We don't try to handle nested comments (MySQL/PG/SQLite differ) — this
// is a best-effort guard for the SELECT validator, not a SQL parser.
func stripSQLComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '-' && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i += 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// stripSQLStrings removes single- and double-quoted string literals and
// replaces them with empty placeholders so that keywords inside string
// contents don't confuse the validator.
func stripSQLStrings(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	inStr := byte(0)
	for i < len(s) {
		c := s[i]
		if inStr != 0 {
			if c == inStr {
				if i+1 < len(s) && s[i+1] == inStr {
					b.WriteByte(' ')
					i += 2
					continue
				}
				inStr = 0
			}
			b.WriteByte(' ')
			i++
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			inStr = c
			b.WriteByte(' ')
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
