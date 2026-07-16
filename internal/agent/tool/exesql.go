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
	"sort"
	"strconv"
	"strings"
	"time"

	// SQL drivers — registered via their init() side effects.
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	agentruntime "ragflow/internal/agent/runtime"
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
		"ExeSQL: db_type not yet supported in Go port (trino, oracle, sqlite, IBM DB2 are pending)",
	)
)

const (
	exesqlToolName        = "execute_sql"
	exesqlToolDescription = "This is a tool that can execute SQL."

	exesqlDefaultSQL        = "{sys.query}"
	exesqlDefaultDBType     = "mysql"
	exesqlDefaultPort       = 3306
	exesqlDefaultMaxRecords = 1024
	exesqlDefaultTimeout    = 60 * time.Second
)

// exesqlConnParams mirrors Python's ExeSQLParam fields. SQL is the only
// model-emitted runtime input; the remaining fields are Canvas node
// configuration and are not exposed from Info.
type exesqlConnParams struct {
	SQL        string // model-emitted runtime input
	DBType     string // mysql | postgres | mariadb | mssql | oceanbase | sqlserver
	Database   string
	Username   string
	Host       string
	Port       int
	Password   string
	MaxRecords int
	Timeout    time.Duration
}

// ExeSQLConnParams is the public alias of exesqlConnParams for
// external callers (e.g. internal/agent/component/Universe A
// delegation wrappers). The internal lowercase name stays for
// backward-compat with existing in-package callers.
type ExeSQLConnParams = exesqlConnParams

// NewExeSQLConnParams decodes a canvas-node params map into an
// ExeSQLConnParams. Python defaults are applied before node values;
// host, database, and username remain required configuration.
func defaultExeSQLConnParams() exesqlConnParams {
	return exesqlConnParams{
		SQL:        exesqlDefaultSQL,
		DBType:     exesqlDefaultDBType,
		Port:       exesqlDefaultPort,
		MaxRecords: exesqlDefaultMaxRecords,
		Timeout:    exesqlDefaultTimeout,
	}
}

// Callers (e.g. the Universe A exesqlComponent wrapper) build the
// params map from the canvas DSL; the tool-side decoding stays
// in this package so the schema lives next to the type.
func NewExeSQLConnParams(params map[string]any) (ExeSQLConnParams, error) {
	conn := defaultExeSQLConnParams()
	if v, ok := params["sql"].(string); ok {
		conn.SQL = v
	}
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
	if v, ok := intParam(params, "port"); ok {
		conn.Port = v
	}
	if v, ok := params["password"].(string); ok {
		conn.Password = v
	}
	if v, ok := intParam(params, "max_records"); ok {
		conn.MaxRecords = v
	} else if v, ok := intParam(params, "top_n"); ok {
		conn.MaxRecords = v
	}
	if conn.DBType == "" || conn.Host == "" || conn.Username == "" || conn.Database == "" {
		return conn, fmt.Errorf("ExeSQL: missing required connection params (db_type/host/database/username)")
	}
	return conn, nil
}

// exesqlArgs is the JSON shape the model sends in. It matches Python's
// ExeSQLParam ToolMeta: SQL is the only runtime parameter.
type exesqlArgs struct {
	SQL string `json:"sql"`
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
// It validates read-only SQL before database access and executes the
// statement against a user-configured external DB via `database/sql`.
type ExeSQLTool struct {
	conn   exesqlConnParams
	dialer exesqlDialer
}

var _ ToolComponent = (*ExeSQLTool)(nil)

// NewExeSQLTool returns an ExeSQLTool wired to the given connection
// params. The dialer defaults to `sql.Open`; tests can pass a
// sqlmock-backed dialer via WithExeSQLDialer.
func NewExeSQLTool(conn exesqlConnParams) *ExeSQLTool {
	defaults := defaultExeSQLConnParams()
	if conn.SQL == "" {
		conn.SQL = defaults.SQL
	}
	if conn.DBType == "" {
		conn.DBType = defaults.DBType
	}
	if conn.Port == 0 {
		conn.Port = defaults.Port
	}
	if conn.MaxRecords == 0 {
		conn.MaxRecords = defaults.MaxRecords
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
// Python ExeSQLParam ToolMeta: only `sql` is visible to the LLM.
// Connection params are not exposed here —
// they're set on the tool instance, matching the Python convention
// where ExeSQLParam fields like `db_type` / `host` are tool
// configuration, not function-call arguments.
func (e *ExeSQLTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        exesqlToolName,
		Description: exesqlToolDescription,
		Parameters: map[string]ParameterInfo{
			"sql": {
				Type:        ParamTypeString,
				Description: "The SQL statement to execute. Must be a SELECT (read-only).",
				Required:    true,
			},
		},
	}
}

func (e *ExeSQLTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"sql": "SQL statement to execute (SELECT-only; DML/DDL rejected).",
		},
		Outputs: map[string]string{
			"formalized_content": "SQL result rendered as Markdown.",
			"json":               "Raw SQL statement results.",
		},
		InputForm: map[string]any{
			"sql": map[string]any{"name": "SQL", "type": "line"},
		},
	}
}

// InvokableRun validates the SQL, opens a fresh connection scoped to
// the tool's params, executes each semicolon-separated statement, and
// returns the rows. Per-statement errors do not abort the node: they
// are accumulated in the `Errors` slice of the response (the Python
// tool does the same — `sql_res.append({"content": msg})`).
func (e *ExeSQLTool) InvokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	if argumentsInJSON == "" {
		return exesqlErrorResult(errors.New("exesql: empty arguments")), errors.New("exesql: empty arguments")
	}
	args := exesqlArgs{SQL: e.conn.SQL}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return exesqlErrorResult(fmt.Errorf("exesql: parse arguments: %w", err)),
			fmt.Errorf("exesql: parse arguments: %w", err)
	}
	if state, _, stateErr := agentruntime.GetStateFromContext[*agentruntime.CanvasState](ctx); stateErr == nil && state != nil {
		resolved, resolveErr := agentruntime.ResolveTemplate(args.SQL, state)
		if resolveErr != nil {
			return exesqlErrorResult(resolveErr), resolveErr
		}
		args.SQL = resolved
	}
	if strings.TrimSpace(args.SQL) == "" {
		return exesqlErrorResult(errors.New("exesql: empty sql")), errors.New("exesql: empty sql")
	}
	conn := e.conn
	if err := validateExeSQLStatements(args.SQL, conn.DBType); err != nil {
		return exesqlErrorResult(ErrExeSQLNotSelect), ErrExeSQLNotSelect
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

	res, err := exesqlExecute(ctx, db, args.SQL, conn.DBType, conn.MaxRecords)
	if err != nil {
		return exesqlErrorResult(err), err
	}
	return exesqlMarshalResult(res)
}

func (e *ExeSQLTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	rows := envelopeSlice(envelope, "rows")
	columns := envelopeSlice(envelope, "columns")
	jsonResult := make([]any, 0, 1)
	if len(rows) == 1 {
		if row, ok := rows[0].(map[string]any); ok && len(row) == 1 {
			if _, hasContent := row["content"]; hasContent {
				jsonResult = append(jsonResult, row)
			}
		}
	}
	if len(jsonResult) == 0 && len(rows) > 0 {
		jsonResult = append(jsonResult, rows)
	}
	return map[string]any{
		"formalized_content": renderExeSQLMarkdown(columns, rows),
		"json":               jsonResult,
	}
}

func renderExeSQLMarkdown(columns, rows []any) string {
	if len(rows) == 0 {
		return ""
	}
	for _, value := range rows {
		if row, ok := value.(map[string]any); ok && len(row) == 1 {
			if message, exists := row["content"]; exists {
				return fmt.Sprint(message)
			}
		}
	}
	columnNames := make([]string, 0, len(columns))
	for _, column := range columns {
		columnNames = append(columnNames, fmt.Sprint(column))
	}
	if len(columnNames) == 0 {
		if first, ok := rows[0].(map[string]any); ok {
			for column := range first {
				columnNames = append(columnNames, column)
			}
			sort.Strings(columnNames)
		}
	}
	if len(columnNames) == 0 {
		return ""
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(columnNames, " | "))
	separators := make([]string, len(columnNames))
	for i := range separators {
		separators[i] = "---"
	}
	fmt.Fprintf(&builder, "| %s |\n", strings.Join(separators, " | "))
	for _, value := range rows {
		row, ok := value.(map[string]any)
		if !ok {
			continue
		}
		cells := make([]string, len(columnNames))
		for i, column := range columnNames {
			cells[i] = strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(row[column]), "|", "\\|"), "\n", "<br>")
		}
		fmt.Fprintf(&builder, "| %s |\n", strings.Join(cells, " | "))
	}
	return strings.TrimSuffix(builder.String(), "\n")
}

// exesqlExecute splits the SQL on statement-delimiting semicolons and runs
// each statement independently. A failing statement is recorded as an
// error entry but does not abort subsequent statements — this is
// the same isolation guarantee the Python tool provides so that
// earlier results survive a bad statement later in the batch.
func exesqlExecute(ctx context.Context, db *sql.DB, sqlText, dbType string, maxRows int) (*exesqlResult, error) {
	stmts := splitSQLStatements(sqlText, dbType)
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

// splitSQLStatements preserves the original SQL and splits only on delimiters
// outside strings, quoted identifiers, and comments. SQL quoting differs by
// database, so the scanner enables backslash escapes, dollar quotes, bracketed
// identifiers, and # comments only for dialects that support them.
func splitSQLStatements(s, dbType string) []string {
	masked, _ := maskSQLLiteralsAndComments(s, dbType)
	statements := make([]string, 0, strings.Count(masked, ";")+1)
	start := 0
	for i := range masked {
		if masked[i] != ';' {
			continue
		}
		statements = append(statements, s[start:i])
		start = i + 1
	}
	return append(statements, s[start:])
}

// validateExeSQLStatements rejects the entire batch before any database work.
// Each executable fragment is checked independently so a read-only first
// statement cannot hide a later write statement.
func validateExeSQLStatements(sqlText, dbType string) error {
	if _, executableComment := maskSQLLiteralsAndComments(sqlText, dbType); executableComment {
		return ErrExeSQLNotSelect
	}
	hasStatement := false
	for _, stmt := range splitSQLStatements(sqlText, dbType) {
		stmt = strings.TrimSpace(stripChunkIDMarkers(stmt))
		if stmt == "" {
			continue
		}
		hasStatement = true
		if !isReadOnlySQLStatement(stmt, dbType) {
			return ErrExeSQLNotSelect
		}
	}
	if !hasStatement {
		return ErrExeSQLNotSelect
	}
	return nil
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
	case "oracle":
		return "", "", fmt.Errorf("%w: oracle", ErrExeSQLUnsupportedDB)
	case "sqlite", "sqlite3":
		return "", "", fmt.Errorf("%w: sqlite", ErrExeSQLUnsupportedDB)
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

// writeCapableSQLKeywords are rejected anywhere in SELECT, WITH, and EXPLAIN
// statements. Checking every executable token prevents data-modifying CTEs
// such as `WITH t AS (DELETE ... RETURNING *) SELECT ...` from passing merely
// because their first keyword is WITH.
var writeCapableSQLKeywords = map[string]struct{}{
	"INSERT": {}, "UPDATE": {}, "DELETE": {}, "REPLACE": {},
	"MERGE":    {},
	"TRUNCATE": {},
	"CREATE":   {}, "DROP": {}, "ALTER": {}, "RENAME": {},
	"GRANT": {}, "REVOKE": {},
	"LOCK": {}, "UNLOCK": {},
	"CALL": {}, "EXEC": {}, "EXECUTE": {},
	"COPY":   {},
	"VACUUM": {},
	"SET":    {}, "RESET": {},
	"USE":        {},
	"KILL":       {},
	"LOAD":       {},
	"CHECKPOINT": {},
	"BEGIN":      {}, "COMMIT": {}, "ROLLBACK": {}, "START": {},
	"SHUTDOWN": {},
}

// isReadOnlySQLStatement validates executable tokens rather than only the
// leading keyword. Quoted text and comments are masked first, so write verbs
// in data values do not cause false rejections.
func isReadOnlySQLStatement(sql, dbType string) bool {
	masked, executableComment := maskSQLLiteralsAndComments(sql, dbType)
	if executableComment {
		return false
	}
	words := sqlKeywordRe.FindAllString(masked, -1)
	if len(words) == 0 {
		return false
	}
	for i := range words {
		words[i] = strings.ToUpper(words[i])
	}

	switch words[0] {
	case "SHOW", "DESCRIBE", "DESC", "PRAGMA":
		return true
	case "SELECT", "WITH", "EXPLAIN":
		for _, word := range words[1:] {
			if _, bad := writeCapableSQLKeywords[word]; bad {
				return false
			}
			switch word {
			case "INTO", "OUTFILE", "DUMPFILE":
				return false
			}
		}
		return true
	}
	return false
}

var sqlKeywordRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_$]*`)

// maskSQLLiteralsAndComments replaces non-executable SQL regions with spaces
// without changing byte offsets. The returned flag identifies MySQL/MariaDB
// executable comments, which are rejected rather than mistaken for comments.
func maskSQLLiteralsAndComments(s, dbType string) (string, bool) {
	masked := []byte(s)
	dialect := strings.ToLower(dbType)
	isMySQL := dialect == "mysql" || dialect == "mariadb" || dialect == "oceanbase"
	isPostgres := dialect == "postgres" || dialect == "postgresql"
	isMSSQL := dialect == "mssql" || dialect == "sqlserver"
	executableComment := false

	mask := func(start, end int) {
		for i := start; i < end; i++ {
			masked[i] = ' '
		}
	}

	for i := 0; i < len(s); {
		if i+1 < len(s) && s[i] == '-' && s[i+1] == '-' &&
			(!isMySQL || i+2 == len(s) || s[i+2] <= ' ') {
			end := i + 2
			for end < len(s) && s[end] != '\n' && s[end] != '\r' {
				end++
			}
			mask(i, end)
			i = end
			continue
		}
		if isMySQL && s[i] == '#' {
			end := i + 1
			for end < len(s) && s[end] != '\n' && s[end] != '\r' {
				end++
			}
			mask(i, end)
			i = end
			continue
		}
		if i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			if isMySQL && (i+2 < len(s) && s[i+2] == '!' ||
				i+3 < len(s) && s[i+2] == 'M' && s[i+3] == '!') {
				executableComment = true
			}
			end := scanSQLBlockComment(s, i, isPostgres)
			mask(i, end)
			i = end
			continue
		}
		if s[i] == '\'' || s[i] == '"' || s[i] == '`' {
			backslashEscapes := isMySQL || isPostgres && s[i] == '\'' && isPostgresEscapeString(s, i)
			end := scanSQLQuotedText(s, i, s[i], backslashEscapes)
			mask(i, end)
			i = end
			continue
		}
		if isMSSQL && s[i] == '[' {
			end := scanSQLQuotedText(s, i, ']', false)
			mask(i, end)
			i = end
			continue
		}
		if isPostgres && s[i] == '$' {
			if delimiter := postgresDollarQuoteDelimiter(s, i); delimiter != "" {
				end := i + len(delimiter)
				if closeAt := strings.Index(s[end:], delimiter); closeAt >= 0 {
					end += closeAt + len(delimiter)
				} else {
					end = len(s)
				}
				mask(i, end)
				i = end
				continue
			}
		}
		i++
	}
	return string(masked), executableComment
}

func scanSQLBlockComment(s string, start int, nested bool) int {
	depth := 1
	for i := start + 2; i < len(s); {
		if nested && i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < len(s) && s[i] == '*' && s[i+1] == '/' {
			depth--
			i += 2
			if depth == 0 {
				return i
			}
			continue
		}
		i++
	}
	return len(s)
}

func scanSQLQuotedText(s string, start int, quote byte, backslashEscapes bool) int {
	for i := start + 1; i < len(s); {
		if backslashEscapes && s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] != quote {
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == quote {
			i += 2
			continue
		}
		return i + 1
	}
	return len(s)
}

func isPostgresEscapeString(s string, quoteAt int) bool {
	if quoteAt == 0 || s[quoteAt-1] != 'E' && s[quoteAt-1] != 'e' {
		return false
	}
	return quoteAt == 1 || !isSQLIdentifierByte(s[quoteAt-2])
}

func postgresDollarQuoteDelimiter(s string, start int) string {
	if start > 0 && isSQLIdentifierByte(s[start-1]) {
		return ""
	}
	for i := start + 1; i < len(s); i++ {
		if s[i] == '$' {
			return s[start : i+1]
		}
		if i == start+1 {
			if !isSQLIdentifierStartByte(s[i]) {
				return ""
			}
			continue
		}
		if !isSQLIdentifierByte(s[i]) {
			return ""
		}
	}
	return ""
}

func isSQLIdentifierStartByte(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func isSQLIdentifierByte(b byte) bool {
	return isSQLIdentifierStartByte(b) || b >= '0' && b <= '9' || b == '$'
}
