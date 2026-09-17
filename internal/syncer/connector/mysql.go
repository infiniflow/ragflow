//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"crypto/md5"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const defaultMySQLBatchSize = 32

// MySQLConnector imports MySQL rows as documents.
//
// It mirrors the Python RDBMSConnector's MySQL dialect: a custom SQL query
// runs verbatim, otherwise every table is loaded. Rows become documents whose
// content is built from the configured content columns (or every column), the
// id column (or an MD5 of the content) forms the stable document id, and the
// timestamp column drives incremental sync and the document update time.
type MySQLConnector struct {
	host            string
	port            int
	database        string
	query           string
	contentColumns  []string
	metadataColumns []string
	idColumn        string
	timestampColumn string
	fileExtension   string
	batchSize       int
	username        string
	password        string

	openDB func(dsn string) (*sql.DB, error)
}

// NewMySQLConnector creates a MySQL connector from Python-compatible config.
func NewMySQLConnector(config map[string]any) (*MySQLConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	connector := &MySQLConnector{
		host:            strings.TrimSpace(stringConfig(config["host"])),
		port:            configInt(config["port"], 3306),
		database:        strings.TrimSpace(stringConfig(config["database"])),
		idColumn:        strings.TrimSpace(stringConfig(config["id_column"])),
		timestampColumn: strings.TrimSpace(stringConfig(config["timestamp_column"])),
		fileExtension:   fileExtensionFromConfig(config["file_extension"]),
		batchSize:       configInt(config["batch_size"], defaultMySQLBatchSize),
		username:        strings.TrimSpace(stringConfig(credentials["username"])),
		password:        stringConfig(credentials["password"]),
	}
	// Production dials through the SSRF-guarded, DNS-pinned openDB. Tests
	// replace it with an injected openDB that avoids the real network.
	connector.openDB = func(dsn string) (*sql.DB, error) {
		return connector.openPinned(dsn)
	}
	connector.query = connector.sanitizeQuery(stringConfig(config["query"]))
	connector.contentColumns = connector.splitColumns(config["content_columns"])
	connector.metadataColumns = connector.splitColumns(config["metadata_columns"])
	return connector, nil
}

// Validate validates MySQL connector settings and credentials.
func (c *MySQLConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("mysql connector is nil")
	}
	if c.username == "" {
		return fmt.Errorf("RDBMS (mysql): missing username")
	}
	if c.host == "" {
		return fmt.Errorf("Database host is required")
	}
	if c.database == "" {
		return fmt.Errorf("Database name is required")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	db, err := c.open()
	if err != nil {
		return fmt.Errorf("Failed to connect to MySQL: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("Failed to connect to MySQL: %w", err)
	}
	return nil
}

// ValidateConnectorSetting validates MySQL settings from an unsaved config.
func (c *MySQLConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one MySQL sync session.
func (c *MySQLConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	db, err := c.open()
	if err != nil {
		return nil, err
	}
	bases, err := c.baseQueries(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	queries := c.buildSyncQueries(bases, request)
	orderColumn := c.syncOrderColumn(request)
	session := &mysqlSyncSession{
		connector:         c,
		db:                db,
		batchSize:         c.batchSize,
		orderColumn:       orderColumn,
		checkpointEnabled: orderColumn != "",
		lastDocQuery:      -1,
	}
	for _, q := range queries {
		session.queries = append(session.queries, q.sql)
		session.queryNames = append(session.queryNames, q.name)
		session.orderedFlags = append(session.orderedFlags, q.ordered)
		session.fallbackQueries = append(session.fallbackQueries, q.fallback)
	}
	if err := session.applyResume(request.Resume); err != nil {
		db.Close()
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete MySQL prune snapshot session.
func (c *MySQLConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	db, err := c.open()
	if err != nil {
		return nil, err
	}
	bases, err := c.baseQueries(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	queries := make([]string, 0, len(bases))
	for _, base := range bases {
		queries = append(queries, c.buildSlimQuery(base.sql))
	}
	return &mysqlPruneSession{connector: c, db: db, queries: queries, batchSize: c.batchSize}, nil
}

// open builds a MySQL connection with Python-compatible settings. The default
// openDB (wired in NewMySQLConnector) validates the host against the shared
// SSRF guard and pins the dial; tests inject an openDB that bypasses the real
// network.
func (c *MySQLConnector) open() (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User = c.username
	cfg.Passwd = c.password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", c.host, c.port)
	cfg.DBName = c.database
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	return c.openDB(cfg.FormatDSN())
}

// openPinned is the production openDB: it validates the configured host with
// the shared host-type SSRF guard and routes the connection through a custom
// network whose dialer is pinned to the validated IP, closing the
// DNS-rebinding window between validation and the TCP connect. The DSN keeps
// the original hostname (TLS ServerName / host-based routing), while
// go-sql-driver resolves the custom network through mysql.RegisterDialContext.
func (c *MySQLConnector) openPinned(_ string) (*sql.DB, error) {
	pinIP, err := assertConnectorHostSafe(c.host)
	if err != nil {
		return nil, err
	}
	network := mysqlPinnedNetwork(c.host, c.port, pinIP)
	mysql.RegisterDialContext(network, mysqlPinnedDial(pinIP, c.port))
	cfg := mysql.NewConfig()
	cfg.User = c.username
	cfg.Passwd = c.password
	cfg.Net = network
	cfg.Addr = net.JoinHostPort(c.host, strconv.Itoa(c.port))
	cfg.DBName = c.database
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	return sql.Open("mysql", cfg.FormatDSN())
}

// mysqlPinnedNetwork returns a deterministic custom network name for a
// host/port/IP pin. Each distinct pin gets its own registered network, so a DNS
// change can never make one connector reuse another connector's dialer. The
// name is hex-only so it round-trips through go-sql-driver's DSN parser.
func mysqlPinnedNetwork(host string, port int, pinIP net.IP) string {
	sum := md5.Sum([]byte(fmt.Sprintf("%s:%d:%s", host, port, pinIP.String())))
	return "pinned-" + hex.EncodeToString(sum[:8])
}

// mysqlPinnedDial returns a go-sql-driver dialer that connects every dial on
// its registered network to pinIP:port, ignoring the host the driver parsed
// from the DSN.
func mysqlPinnedDial(pinIP net.IP, port int) mysql.DialContextFunc {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(pinIP.String(), strconv.Itoa(port)))
	}
}

// baseQueries returns the configured query or a SELECT per table. Table names
// are sorted so the sync stream order is stable across runs and a resume
// cursor can reliably skip already-processed tables.
func (c *MySQLConnector) baseQueries(ctx context.Context, db *sql.DB) ([]rdbmsQuery, error) {
	if c.query != "" {
		return []rdbmsQuery{{name: "", sql: c.query}}, nil
	}
	rows, err := db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(tables)
	queries := make([]rdbmsQuery, 0, len(tables))
	for _, table := range tables {
		queries = append(queries, rdbmsQuery{name: table, sql: fmt.Sprintf("SELECT * FROM %s", table)})
	}
	return queries, nil
}

// buildSyncQueries applies the incremental window and a stable ordering when
// one is available, so a checkpoint can resume the stream from an anchor.
// Each query carries an unordered fallback used when a custom SQL query does
// not expose the configured ordering column.
func (c *MySQLConnector) buildSyncQueries(bases []rdbmsQuery, request SyncRequest) []rdbmsSyncQuery {
	queries := make([]rdbmsSyncQuery, 0, len(bases))
	switch {
	case !request.FromBeginning && c.timestampColumn != "":
		start := request.WindowStart
		end := &request.WindowEnd
		for _, base := range bases {
			queries = append(queries, rdbmsSyncQuery{
				name:     base.name,
				sql:      c.buildTimeFilteredOrderedQuery(base.sql, start, end),
				ordered:  true,
				fallback: c.buildTimeFilteredQuery(base.sql, start, end),
			})
		}
	case request.FromBeginning && c.idColumn != "":
		for _, base := range bases {
			queries = append(queries, rdbmsSyncQuery{
				name:     base.name,
				sql:      c.buildOrderedQuery(base.sql, c.idColumn),
				ordered:  true,
				fallback: c.wrapQuery(base.sql),
			})
		}
	default:
		for _, base := range bases {
			queries = append(queries, rdbmsSyncQuery{name: base.name, sql: base.sql})
		}
	}
	return queries
}

// syncOrderColumn returns the ordering key that makes this sync window
// deterministic, or "" when the connector cannot checkpoint/resume the
// stream (no stable ordering key). Incremental windows order by timestamp
// plus id so rows sharing a timestamp still resume deterministically.
func (c *MySQLConnector) syncOrderColumn(request SyncRequest) string {
	switch {
	case !request.FromBeginning && c.timestampColumn != "" && c.idColumn != "":
		return c.timestampColumn + "," + c.idColumn
	case request.FromBeginning && c.idColumn != "":
		return c.idColumn
	}
	return ""
}

// buildOrderedQuery wraps the base query and orders it by a stable column so
// connector sync can resume from a checkpoint.
func (c *MySQLConnector) buildOrderedQuery(base, orderColumn string) string {
	return c.wrapQuery(base) + " ORDER BY ragflow_src." + orderColumn + " ASC"
}

// buildTimeFilteredQuery wraps the base query and appends timestamp bounds.
func (c *MySQLConnector) buildTimeFilteredQuery(base string, start, end *time.Time) string {
	conditions := []string{}
	if start != nil {
		conditions = append(conditions, fmt.Sprintf("ragflow_src.%s >= %s", c.timestampColumn, c.formatDatetime(*start)))
	}
	if end != nil {
		conditions = append(conditions, fmt.Sprintf("ragflow_src.%s <= %s", c.timestampColumn, c.formatDatetime(*end)))
	}
	query := c.wrapQuery(base)
	if len(conditions) > 0 {
		query = query + " WHERE " + strings.Join(conditions, " AND ")
	}
	return query
}

// buildTimeFilteredOrderedQuery is the incremental query plus a deterministic
// ORDER BY on the timestamp and id columns, which resume relies on. Without a
// configured id column the order is timestamp-only and the stream is not
// checkpointed.
func (c *MySQLConnector) buildTimeFilteredOrderedQuery(base string, start, end *time.Time) string {
	query := c.buildTimeFilteredQuery(base, start, end) + " ORDER BY ragflow_src." + c.timestampColumn + " ASC"
	if c.idColumn != "" {
		query += ", ragflow_src." + c.idColumn + " ASC"
	}
	return query
}

// buildSlimQuery selects only the columns needed to identify documents.
func (c *MySQLConnector) buildSlimQuery(base string) string {
	columns := []string{}
	if c.idColumn != "" {
		columns = []string{c.idColumn}
	} else {
		columns = c.contentColumns
	}
	if len(columns) == 0 {
		return c.wrapQuery(base)
	}
	selects := make([]string, 0, len(columns))
	for _, column := range columns {
		selects = append(selects, fmt.Sprintf("ragflow_src.%s", column))
	}
	return fmt.Sprintf("SELECT %s FROM (%s) AS ragflow_src", strings.Join(selects, ", "), c.stripOrderBy(base))
}

// wrapQuery wraps the base query as a derived table named ragflow_src.
func (c *MySQLConnector) wrapQuery(base string) string {
	return fmt.Sprintf("SELECT * FROM (%s) AS ragflow_src", c.stripOrderBy(base))
}

// stripOrderBy removes a trailing top-level ORDER BY clause.
func (c *MySQLConnector) stripOrderBy(query string) string {
	pattern := regexp.MustCompile(`(?i)\border\s+by\b`)
	cleaned := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
	matches := pattern.FindAllStringIndex(cleaned, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		prefix := cleaned[:matches[i][0]]
		if strings.Count(prefix, "(") == strings.Count(prefix, ")") {
			return strings.TrimSpace(prefix)
		}
	}
	return cleaned
}

// formatDatetime renders a UTC time as a MySQL datetime literal.
func (c *MySQLConnector) formatDatetime(value time.Time) string {
	return "'" + value.UTC().Format("2006-01-02 15:04:05") + "'"
}

// scanRow scans the current row into an ordered column map.
func (c *MySQLConnector) scanRow(rows *sql.Rows) (map[string]any, []string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return nil, nil, err
	}
	row := make(map[string]any, len(columns))
	for i, column := range columns {
		row[column] = c.normalizeValue(values[i])
	}
	return row, columns, nil
}

// normalizeValue converts driver byte slices to strings.
func (c *MySQLConnector) normalizeValue(value any) any {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	if _, ok := value.(time.Time); ok {
		return value
	}
	if valuer, ok := value.(driver.Valuer); ok {
		if converted, err := valuer.Value(); err == nil {
			return c.normalizeValue(converted)
		}
	}
	return value
}

// contentColumnsForRow resolves the content columns for a row, excluding the
// structural id and timestamp columns when no content columns are configured.
func (c *MySQLConnector) contentColumnsForRow(row map[string]any, orderedColumns []string) []string {
	if len(c.contentColumns) > 0 {
		return c.contentColumns
	}
	excluded := map[string]bool{}
	if c.idColumn != "" {
		excluded[c.idColumn] = true
	}
	if c.timestampColumn != "" {
		excluded[c.timestampColumn] = true
	}
	columns := make([]string, 0, len(orderedColumns))
	for _, column := range orderedColumns {
		if _, ok := row[column]; ok && !excluded[column] {
			columns = append(columns, column)
		}
	}
	return columns
}

// buildContent renders the document content from the resolved content columns.
func (c *MySQLConnector) buildContent(row map[string]any, columns []string) string {
	parts := []string{}
	for _, column := range columns {
		value, ok := row[column]
		if !ok || value == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("【%s】:\n%s", column, c.renderValue(value)))
	}
	return strings.Join(parts, "\n\n")
}

// buildDocumentID derives the stable document id, matching the Python format
// "mysql:<database>:<id value>" with an MD5 content fallback.
func (c *MySQLConnector) buildDocumentID(row map[string]any, orderedColumns []string) string {
	if c.idColumn != "" {
		if value, ok := row[c.idColumn]; ok && value != nil {
			return fmt.Sprintf("mysql:%s:%s", c.database, fmt.Sprint(value))
		}
	}
	content := c.buildContent(row, c.contentColumnsForRow(row, orderedColumns))
	sum := md5.Sum([]byte(content))
	return fmt.Sprintf("mysql:%s:%s", c.database, hex.EncodeToString(sum[:]))
}

// rowToSourceDocument converts a database row into the syncer model.
func (c *MySQLConnector) rowToSourceDocument(row map[string]any, orderedColumns []string) (SourceDocument, bool) {
	contentColumns := c.contentColumnsForRow(row, orderedColumns)
	content := c.buildContent(row, contentColumns)

	metadata := map[string]any{}
	for _, column := range c.metadataColumns {
		value, ok := row[column]
		if !ok || value == nil {
			continue
		}
		metadata[column] = c.formatMetadataValue(value)
	}

	updatedAt := time.Now().UTC()
	if c.timestampColumn != "" {
		if ts, ok := row[c.timestampColumn].(time.Time); ok {
			updatedAt = ts.UTC()
		}
	}

	semanticID := "database_record"
	if len(contentColumns) > 0 {
		if value, ok := row[contentColumns[0]]; ok && value != nil {
			semanticID = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(value), "\n", " "), "\r", " "))
			if semanticID == "" {
				semanticID = "database_record"
			} else if len(semanticID) > 100 {
				semanticID = semanticID[:100]
			}
		}
	}

	sourceID := c.buildDocumentID(row, orderedColumns)
	blob := []byte(content)
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: semanticID,
		Extension:          c.fileExtension,
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint: stableFingerprint(map[string]any{
			"id":       sourceID,
			"content":  content,
			"metadata": metadata,
		}),
	}, true
}

// renderValue formats a row value for document content.
func (c *MySQLConnector) renderValue(value any) string {
	if typed, ok := value.(time.Time); ok {
		return typed.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprint(value)
}

// formatMetadataValue formats a row value for metadata, mirroring Python's
// isoformat for datetimes and string rendering otherwise.
func (c *MySQLConnector) formatMetadataValue(value any) string {
	if typed, ok := value.(time.Time); ok {
		return typed.Format(time.RFC3339)
	}
	return fmt.Sprint(value)
}

// sanitizeQuery tolerates queries pasted from a markdown code fence.
func (c *MySQLConnector) sanitizeQuery(raw string) string {
	fenceLanguages := map[string]bool{"sql": true, "tsql": true, "t-sql": true, "mssql": true, "mysql": true, "postgresql": true, "psql": true}
	query := strings.TrimSpace(raw)
	if query == "" {
		return ""
	}
	if strings.HasPrefix(query, "```") {
		query = query[3:]
		if strings.HasSuffix(query, "```") {
			query = query[:len(query)-3]
		}
		query = strings.TrimSpace(query)
	}
	if head, tail, found := strings.Cut(query, "\n"); found {
		if fenceLanguages[strings.ToLower(strings.TrimSpace(head))] {
			query = strings.TrimSpace(tail)
		}
	}
	return query
}

// splitColumns parses a comma-separated string or list column config.
func (c *MySQLConnector) splitColumns(value any) []string {
	switch typed := value.(type) {
	case string:
		parts := strings.Split(typed, ",")
		columns := make([]string, 0, len(parts))
		for _, part := range parts {
			if column := strings.TrimSpace(part); column != "" {
				columns = append(columns, column)
			}
		}
		return columns
	case []any:
		columns := make([]string, 0, len(typed))
		for _, item := range typed {
			if column := strings.TrimSpace(stringConfig(item)); column != "" {
				columns = append(columns, column)
			}
		}
		return columns
	}
	return nil
}

type mysqlSyncSession struct {
	connector  *MySQLConnector
	db         *sql.DB
	queries    []string
	queryNames []string
	// orderedFlags[i] reports whether queries[i] carries a stable ORDER BY.
	orderedFlags []bool
	// fallbackQueries[i] is the unordered variant of queries[i], used when a
	// custom SQL query does not expose the configured ordering column.
	fallbackQueries []string
	queryIndex      int
	// lastDocQuery is the index of the query that produced the most recently
	// appended document, used to checkpoint against the right query name even
	// when later queries in the batch contributed no documents.
	lastDocQuery int
	rows         *sql.Rows
	batchSize    int

	orderColumn       string
	checkpointEnabled bool
	orderable         bool
	resume            *rdbmsResumeCursor
	resumePending     bool
}

// NextBatch returns the next MySQL document batch.
func (s *mysqlSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		if s.rows == nil {
			if s.queryIndex >= len(s.queries) {
				if len(documents) == 0 {
					return s.endOfStream()
				}
				break
			}
			if err := s.openNextQuery(ctx); err != nil {
				return SyncBatch{}, err
			}
		}
		if !s.rows.Next() {
			if err := s.rows.Err(); err != nil {
				s.closeRows()
				return SyncBatch{}, err
			}
			s.closeRows()
			continue
		}
		row, columns, err := s.connector.scanRow(s.rows)
		if err != nil {
			// Skip rows that fail to convert (mirrors Python).
			continue
		}
		if doc, ok := s.connector.rowToSourceDocument(row, columns); ok {
			if !s.includeResumed(doc) {
				continue
			}
			documents = append(documents, doc)
			s.lastDocQuery = s.queryIndex - 1
		}
	}
	if len(documents) == 0 {
		return s.endOfStream()
	}
	return SyncBatch{Documents: documents, Checkpoint: s.batchCheckpoint(documents[len(documents)-1])}, nil
}

// Close closes the MySQL sync session.
func (s *mysqlSyncSession) Close() error {
	s.closeRows()
	return s.db.Close()
}

// openNextQuery runs the next base query. When a custom SQL query does not
// expose the configured ordering column (MySQL error 1054), it falls back to
// the unordered query and stops checkpointing so the remaining stream is never
// resumed against a non-deterministic order. A pending resume never falls back:
// the ordering that produced the anchor is gone, so the window restarts.
func (s *mysqlSyncSession) openNextQuery(ctx context.Context) error {
	idx := s.queryIndex
	s.queryIndex++
	rows, err := s.db.QueryContext(ctx, s.queries[idx])
	if err != nil {
		if s.orderedFlags[idx] && isMySQLUnknownColumn(err) {
			if s.resumePending {
				return fmt.Errorf("MySQL sync resume query lost its ordering column: %w", ErrSyncResumeInvalid)
			}
			s.checkpointEnabled = false
			rows, err = s.db.QueryContext(ctx, s.fallbackQueries[idx])
			if err != nil {
				return fmt.Errorf("MySQL query failed: %w", err)
			}
			s.orderable = false
			s.rows = rows
			return nil
		}
		return fmt.Errorf("MySQL query failed: %w", err)
	}
	s.orderable = s.orderedFlags[idx]
	s.rows = rows
	return nil
}

// closeRows releases the current result set.
func (s *mysqlSyncSession) closeRows() {
	if s.rows != nil {
		s.rows.Close()
		s.rows = nil
	}
}

// applyResume positions the session after the last committed batch. The
// cursor's query and ordering column must still exist, otherwise the runner
// restarts the task window.
func (s *mysqlSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	cursor, err := parseRDBMSCursor(checkpoint.Cursor)
	if err != nil {
		return err
	}
	if s.orderColumn == "" || cursor.Order != s.orderColumn {
		return fmt.Errorf("MySQL sync resume ordering changed from %q to %q: %w", cursor.Order, s.orderColumn, ErrSyncResumeInvalid)
	}
	idx := -1
	for i, name := range s.queryNames {
		if name == cursor.Query {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("MySQL sync resume query %q no longer exists: %w", cursor.Query, ErrSyncResumeInvalid)
	}
	s.queryIndex = idx
	s.resume = &cursor
	s.resumePending = true
	return nil
}

// includeResumed reports whether doc should be emitted. While a resume is
// pending, every row before (and including) the anchor is skipped because it
// was already committed.
func (s *mysqlSyncSession) includeResumed(doc SourceDocument) bool {
	if !s.resumePending {
		return true
	}
	if s.resume != nil && doc.SourceID == s.resume.SourceID {
		s.resumePending = false
		return false
	}
	return false
}

// batchCheckpoint builds the checkpoint for a batch whose last row is doc.
// Batches from a non-deterministic (unordered) query never carry a checkpoint.
func (s *mysqlSyncSession) batchCheckpoint(doc SourceDocument) *SyncCheckpoint {
	if !s.checkpointEnabled || !s.orderable {
		return nil
	}
	queryName := ""
	if idx := s.lastDocQuery; idx >= 0 && idx < len(s.queryNames) {
		queryName = s.queryNames[idx]
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    encodeRDBMSCursor(queryName, s.orderColumn, doc.SourceID),
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

// endOfStream returns io.EOF when the stream is exhausted, or
// ErrSyncResumeInvalid when a pending resume anchor was never found.
func (s *mysqlSyncSession) endOfStream() (SyncBatch, error) {
	if s.resumePending {
		anchor := ""
		if s.resume != nil {
			anchor = s.resume.SourceID
		}
		return SyncBatch{}, fmt.Errorf("MySQL resume anchor %q was not found in the current result: %w", anchor, ErrSyncResumeInvalid)
	}
	return SyncBatch{}, io.EOF
}

// isMySQLUnknownColumn reports whether err is MySQL error 1054 (unknown column
// in the order/result), used to detect custom queries that do not expose the
// configured ordering column.
func isMySQLUnknownColumn(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1054
}

type mysqlPruneSession struct {
	connector  *MySQLConnector
	db         *sql.DB
	queries    []string
	queryIndex int
	rows       *sql.Rows
	batchSize  int
}

// NextBatch returns the next MySQL prune snapshot batch.
func (s *mysqlPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		if s.rows == nil {
			if s.queryIndex >= len(s.queries) {
				if len(documents) == 0 {
					return PruneBatch{}, io.EOF
				}
				break
			}
			if err := s.openNextQuery(ctx); err != nil {
				return PruneBatch{}, err
			}
		}
		if !s.rows.Next() {
			if err := s.rows.Err(); err != nil {
				s.closeRows()
				return PruneBatch{}, err
			}
			s.closeRows()
			continue
		}
		row, columns, err := s.connector.scanRow(s.rows)
		if err != nil {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: s.connector.buildDocumentID(row, columns)})
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the MySQL prune session.
func (s *mysqlPruneSession) Close() error {
	s.closeRows()
	return s.db.Close()
}

// openNextQuery runs the next slim query.
func (s *mysqlPruneSession) openNextQuery(ctx context.Context) error {
	query := s.queries[s.queryIndex]
	s.queryIndex++
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("MySQL query failed: %w", err)
	}
	s.rows = rows
	return nil
}

// closeRows releases the current result set.
func (s *mysqlPruneSession) closeRows() {
	if s.rows != nil {
		s.rows.Close()
		s.rows = nil
	}
}
