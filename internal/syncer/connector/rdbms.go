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
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultRDBMSBatchSize = 32
	defaultMySQLPort      = 3306
	defaultPostgresPort   = 5432

	rdbmsTypeMySQL      = "mysql"
	rdbmsTypePostgreSQL = "postgresql"
)

var (
	rdbmsOrderByPattern = regexp.MustCompile(`(?i)\border\s+by\b`)
	rdbmsFenceLanguages = map[string]bool{"sql": true, "tsql": true, "t-sql": true, "mssql": true, "mysql": true, "postgresql": true, "psql": true}
)

// RDBMSConnector imports MySQL rows as documents.
//
// It mirrors the Python RDBMSConnector: a custom SQL query runs verbatim,
// otherwise every table is loaded. Rows become documents whose content is
// built from the configured content columns (or every column), the id column
// (or an MD5 of the content) forms the stable document id, and the timestamp
// column drives incremental sync and the document update time.
type RDBMSConnector struct {
	host            string
	port            int
	database        string
	query           string
	contentColumns  []string
	metadataColumns []string
	idColumn        string
	timestampColumn string
	batchSize       int
	username        string
	password        string
	dbType          string

	openDB func(dsn string) (*sql.DB, error)
}

// NewRDBMSConnector creates a MySQL connector from Python-compatible config.
func NewRDBMSConnector(config map[string]any) (*RDBMSConnector, error) {
	return newRDBMSConnector(config, rdbmsTypeMySQL, defaultMySQLPort)
}

// NewPostgreSQLConnector creates a PostgreSQL connector from Python-compatible config.
func NewPostgreSQLConnector(config map[string]any) (*RDBMSConnector, error) {
	return newRDBMSConnector(config, rdbmsTypePostgreSQL, defaultPostgresPort)
}

func newRDBMSConnector(config map[string]any, dbType string, defaultPort int) (*RDBMSConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	driverName := "mysql"
	if dbType == rdbmsTypePostgreSQL {
		driverName = "pgx"
	}
	return &RDBMSConnector{
		host:            strings.TrimSpace(stringConfig(config["host"])),
		port:            configInt(config["port"], defaultPort),
		database:        strings.TrimSpace(stringConfig(config["database"])),
		query:           sanitizeRDBMSQuery(stringConfig(config["query"])),
		contentColumns:  splitRDBMSColumns(config["content_columns"]),
		metadataColumns: splitRDBMSColumns(config["metadata_columns"]),
		idColumn:        strings.TrimSpace(stringConfig(config["id_column"])),
		timestampColumn: strings.TrimSpace(stringConfig(config["timestamp_column"])),
		batchSize:       configInt(config["batch_size"], defaultRDBMSBatchSize),
		username:        strings.TrimSpace(stringConfig(credentials["username"])),
		password:        stringConfig(credentials["password"]),
		dbType:          dbType,
		openDB: func(dsn string) (*sql.DB, error) {
			return sql.Open(driverName, dsn)
		},
	}, nil
}

// Validate validates RDBMS connector settings and credentials.
func (c *RDBMSConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("rdbms connector is nil")
	}
	if c.username == "" {
		return fmt.Errorf("RDBMS (%s): missing username", c.dbType)
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
		return fmt.Errorf("Failed to connect to %s: %w", c.dbDisplayName(), err)
	}
	defer db.Close()
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("Failed to connect to %s: %w", c.dbDisplayName(), err)
	}
	return nil
}

// dbDisplayName returns the human-readable database name for error messages.
func (c *RDBMSConnector) dbDisplayName() string {
	if c.dbType == rdbmsTypePostgreSQL {
		return "PostgreSQL"
	}
	return "MySQL"
}

// OpenSync opens one RDBMS sync session.
func (c *RDBMSConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
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
	return &rdbmsSyncSession{connector: c, db: db, queries: queries, batchSize: c.batchSize}, nil
}

// OpenPrune opens one complete RDBMS prune snapshot session.
func (c *RDBMSConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
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
		queries = append(queries, c.buildSlimQuery(base))
	}
	return &rdbmsPruneSession{connector: c, db: db, queries: queries, batchSize: c.batchSize}, nil
}

// open builds a database connection with Python-compatible settings.
func (c *RDBMSConnector) open() (*sql.DB, error) {
	if c.dbType == rdbmsTypePostgreSQL {
		dsn := url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(c.username, c.password),
			Host:   fmt.Sprintf("%s:%d", c.host, c.port),
			Path:   "/" + url.PathEscape(c.database),
		}
		return c.openDB(dsn.String())
	}
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

// baseQueries returns the configured query or a SELECT per table.
func (c *RDBMSConnector) baseQueries(ctx context.Context, db *sql.DB) ([]string, error) {
	if c.query != "" {
		return []string{c.query}, nil
	}
	tablesQuery := "SHOW TABLES"
	if c.dbType == rdbmsTypePostgreSQL {
		tablesQuery = "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'"
	}
	rows, err := db.QueryContext(ctx, tablesQuery)
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
	queries := make([]string, 0, len(tables))
	for _, table := range tables {
		queries = append(queries, fmt.Sprintf("SELECT * FROM %s", table))
	}
	return queries, nil
}

// buildSyncQueries applies the incremental window when a timestamp column exists.
func (c *RDBMSConnector) buildSyncQueries(bases []string, request SyncRequest) []string {
	var start, end *time.Time
	if !request.FromBeginning {
		start = request.WindowStart
		end = &request.WindowEnd
	}
	if c.timestampColumn == "" || (start == nil && end == nil) {
		return bases
	}
	queries := make([]string, 0, len(bases))
	for _, base := range bases {
		queries = append(queries, c.buildTimeFilteredQuery(base, start, end))
	}
	return queries
}

// buildTimeFilteredQuery wraps the base query and appends timestamp bounds.
func (c *RDBMSConnector) buildTimeFilteredQuery(base string, start, end *time.Time) string {
	conditions := []string{}
	if start != nil {
		conditions = append(conditions, fmt.Sprintf("ragflow_src.%s >= %s", c.timestampColumn, formatRDBMSDatetime(c.dbType, *start)))
	}
	if end != nil {
		conditions = append(conditions, fmt.Sprintf("ragflow_src.%s <= %s", c.timestampColumn, formatRDBMSDatetime(c.dbType, *end)))
	}
	query := c.wrapQuery(base)
	if len(conditions) > 0 {
		query = query + " WHERE " + strings.Join(conditions, " AND ")
	}
	return query
}

// buildSlimQuery selects only the columns needed to identify documents.
func (c *RDBMSConnector) buildSlimQuery(base string) string {
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
	return fmt.Sprintf("SELECT %s FROM (%s) AS ragflow_src", strings.Join(selects, ", "), stripRDBMSOrderBy(base))
}

// wrapQuery wraps the base query as a derived table named ragflow_src.
func (c *RDBMSConnector) wrapQuery(base string) string {
	return fmt.Sprintf("SELECT * FROM (%s) AS ragflow_src", stripRDBMSOrderBy(base))
}

// stripRDBMSOrderBy removes a trailing top-level ORDER BY clause.
func stripRDBMSOrderBy(query string) string {
	cleaned := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
	matches := rdbmsOrderByPattern.FindAllStringIndex(cleaned, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		prefix := cleaned[:matches[i][0]]
		if strings.Count(prefix, "(") == strings.Count(prefix, ")") {
			return strings.TrimSpace(prefix)
		}
	}
	return cleaned
}

// formatRDBMSDatetime renders a UTC time as a SQL datetime literal, using the
// MySQL "YYYY-MM-DD HH:MM:SS" form or the ISO-8601 form PostgreSQL accepts.
func formatRDBMSDatetime(dbType string, value time.Time) string {
	if dbType == rdbmsTypePostgreSQL {
		return "'" + value.UTC().Format(time.RFC3339Nano) + "'"
	}
	return "'" + value.UTC().Format("2006-01-02 15:04:05") + "'"
}

// scanRow scans the current row into an ordered column map.
func (c *RDBMSConnector) scanRow(rows *sql.Rows) (map[string]any, []string, error) {
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
		row[column] = normalizeRDBMSValue(values[i])
	}
	return row, columns, nil
}

// normalizeRDBMSValue converts driver-specific values into plain strings so
// content and metadata rendering stay dialect-agnostic. Byte slices (MySQL
// text, PostgreSQL jsonb) and driver value types (PostgreSQL numeric) become
// their text form; time.Time passes through untouched.
func normalizeRDBMSValue(value any) any {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	if _, ok := value.(time.Time); ok {
		return value
	}
	if valuer, ok := value.(driver.Valuer); ok {
		if converted, err := valuer.Value(); err == nil {
			return normalizeRDBMSValue(converted)
		}
	}
	return value
}

// contentColumnsForRow resolves the content columns for a row, excluding the
// structural id and timestamp columns when no content columns are configured.
func (c *RDBMSConnector) contentColumnsForRow(row map[string]any, orderedColumns []string) []string {
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
func (c *RDBMSConnector) buildContent(row map[string]any, columns []string) string {
	parts := []string{}
	for _, column := range columns {
		value, ok := row[column]
		if !ok || value == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("【%s】:\n%s", column, renderRDBMSValue(value)))
	}
	return strings.Join(parts, "\n\n")
}

// buildDocumentID derives the stable document id, matching the Python format
// "<db_type>:<database>:<id value>" with an MD5 content fallback.
func (c *RDBMSConnector) buildDocumentID(row map[string]any, orderedColumns []string) string {
	if c.idColumn != "" {
		if value, ok := row[c.idColumn]; ok && value != nil {
			return fmt.Sprintf("%s:%s:%s", c.dbType, c.database, fmt.Sprint(value))
		}
	}
	content := c.buildContent(row, c.contentColumnsForRow(row, orderedColumns))
	sum := md5.Sum([]byte(content))
	return fmt.Sprintf("%s:%s:%s", c.dbType, c.database, hex.EncodeToString(sum[:]))
}

// rowToSourceDocument converts a database row into the syncer model.
func (c *RDBMSConnector) rowToSourceDocument(row map[string]any, orderedColumns []string) (SourceDocument, bool) {
	contentColumns := c.contentColumnsForRow(row, orderedColumns)
	content := c.buildContent(row, contentColumns)

	metadata := map[string]any{}
	for _, column := range c.metadataColumns {
		value, ok := row[column]
		if !ok || value == nil {
			continue
		}
		metadata[column] = formatRDBMSMetadataValue(value)
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
		Extension:          ".txt",
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

// renderRDBMSValue formats a row value for document content.
func renderRDBMSValue(value any) string {
	if typed, ok := value.(time.Time); ok {
		return typed.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprint(value)
}

// formatRDBMSMetadataValue formats a row value for metadata, mirroring Python's
// isoformat for datetimes and string rendering otherwise.
func formatRDBMSMetadataValue(value any) string {
	if typed, ok := value.(time.Time); ok {
		return typed.Format(time.RFC3339)
	}
	return fmt.Sprint(value)
}

// sanitizeRDBMSQuery tolerates queries pasted from a markdown code fence.
func sanitizeRDBMSQuery(raw string) string {
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
		if rdbmsFenceLanguages[strings.ToLower(strings.TrimSpace(head))] {
			query = strings.TrimSpace(tail)
		}
	}
	return query
}

// splitRDBMSColumns parses a comma-separated string or list column config.
func splitRDBMSColumns(value any) []string {
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

type rdbmsSyncSession struct {
	connector  *RDBMSConnector
	db         *sql.DB
	queries    []string
	queryIndex int
	rows       *sql.Rows
	batchSize  int
}

// NextBatch returns the next RDBMS document batch.
func (s *rdbmsSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		if s.rows == nil {
			if s.queryIndex >= len(s.queries) {
				if len(documents) == 0 {
					return SyncBatch{}, io.EOF
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
			documents = append(documents, doc)
		}
	}
	return SyncBatch{Documents: documents}, nil
}

// Close closes the RDBMS sync session.
func (s *rdbmsSyncSession) Close() error {
	s.closeRows()
	return s.db.Close()
}

// openNextQuery runs the next base query.
func (s *rdbmsSyncSession) openNextQuery(ctx context.Context) error {
	query := s.queries[s.queryIndex]
	s.queryIndex++
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("RDBMS query failed: %w", err)
	}
	s.rows = rows
	return nil
}

// closeRows releases the current result set.
func (s *rdbmsSyncSession) closeRows() {
	if s.rows != nil {
		s.rows.Close()
		s.rows = nil
	}
}

type rdbmsPruneSession struct {
	connector  *RDBMSConnector
	db         *sql.DB
	queries    []string
	queryIndex int
	rows       *sql.Rows
	batchSize  int
}

// NextBatch returns the next RDBMS prune snapshot batch.
func (s *rdbmsPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

// Close closes the RDBMS prune session.
func (s *rdbmsPruneSession) Close() error {
	s.closeRows()
	return s.db.Close()
}

// openNextQuery runs the next slim query.
func (s *rdbmsPruneSession) openNextQuery(ctx context.Context) error {
	query := s.queries[s.queryIndex]
	s.queryIndex++
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("RDBMS query failed: %w", err)
	}
	s.rows = rows
	return nil
}

// closeRows releases the current result set.
func (s *rdbmsPruneSession) closeRows() {
	if s.rows != nil {
		s.rows.Close()
		s.rows = nil
	}
}
