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
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultPostgresBatchSize      = 32
	defaultPostgresConnectTimeout = 30
)

// PostgreSQLConnector imports PostgreSQL rows as documents.
//
// It mirrors the Python RDBMSConnector's PostgreSQL dialect: a custom SQL
// query runs verbatim, otherwise every table in the public schema is loaded.
// Rows become documents whose content is built from the configured content
// columns (or every column), the id column (or an MD5 of the content) forms
// the stable document id, and the timestamp column drives incremental sync
// and the document update time.
type PostgreSQLConnector struct {
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
	sslmode         string
	connectTimeout  int

	openDB func(dsn string) (*sql.DB, error)
}

// NewPostgreSQLConnector creates a PostgreSQL connector from Python-compatible config.
func NewPostgreSQLConnector(config map[string]any) (*PostgreSQLConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	connector := &PostgreSQLConnector{
		host:            strings.TrimSpace(stringConfig(config["host"])),
		port:            configInt(config["port"], 5432),
		database:        strings.TrimSpace(stringConfig(config["database"])),
		idColumn:        strings.TrimSpace(stringConfig(config["id_column"])),
		timestampColumn: strings.TrimSpace(stringConfig(config["timestamp_column"])),
		batchSize:       configInt(config["batch_size"], defaultPostgresBatchSize),
		username:        strings.TrimSpace(stringConfig(credentials["username"])),
		password:        stringConfig(credentials["password"]),
		sslmode:         strings.TrimSpace(stringConfig(config["sslmode"])),
		connectTimeout:  configInt(config["connect_timeout"], defaultPostgresConnectTimeout),
		openDB: func(dsn string) (*sql.DB, error) {
			return sql.Open("pgx", dsn)
		},
	}
	if connector.sslmode == "" {
		connector.sslmode = "prefer"
	}
	connector.query = connector.sanitizeQuery(stringConfig(config["query"]))
	connector.contentColumns = connector.splitColumns(config["content_columns"])
	connector.metadataColumns = connector.splitColumns(config["metadata_columns"])
	return connector, nil
}

// Validate validates PostgreSQL connector settings and credentials.
func (c *PostgreSQLConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("postgresql connector is nil")
	}
	if c.username == "" {
		return fmt.Errorf("RDBMS (postgresql): missing username")
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
		return fmt.Errorf("Failed to connect to PostgreSQL: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("Failed to connect to PostgreSQL: %w", err)
	}
	return nil
}

// ValidateConnectorSetting validates PostgreSQL settings from an unsaved config.
func (c *PostgreSQLConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one PostgreSQL sync session.
func (c *PostgreSQLConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
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
	return &postgresSyncSession{connector: c, db: db, queries: queries, batchSize: c.batchSize}, nil
}

// OpenPrune opens one complete PostgreSQL prune snapshot session.
func (c *PostgreSQLConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
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
	return &postgresPruneSession{connector: c, db: db, queries: queries, batchSize: c.batchSize}, nil
}

// open builds a PostgreSQL connection from the connector settings. The DSN
// carries connector-controlled sslmode (default prefer, matching Python's
// psycopg2) and a finite connect_timeout so an unreachable host cannot hang
// a sync worker.
func (c *PostgreSQLConnector) open() (*sql.DB, error) {
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.username, c.password),
		Host:   fmt.Sprintf("%s:%d", c.host, c.port),
		Path:   "/" + url.PathEscape(c.database),
	}
	query := dsn.Query()
	query.Set("sslmode", c.sslmode)
	query.Set("connect_timeout", strconv.Itoa(c.connectTimeout))
	dsn.RawQuery = query.Encode()
	return c.openDB(dsn.String())
}

// baseQueries returns the configured query or a SELECT per table.
func (c *PostgreSQLConnector) baseQueries(ctx context.Context, db *sql.DB) ([]string, error) {
	if c.query != "" {
		return []string{c.query}, nil
	}
	rows, err := db.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'")
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
		queries = append(queries, fmt.Sprintf("SELECT * FROM \"public\".%s", quotePostgresIdentifier(table)))
	}
	return queries, nil
}

// quotePostgresIdentifier double-quotes an identifier for PostgreSQL, escaping
// any embedded double quotes, so catalog-discovered names with mixed case or
// special characters survive the unquoted lowercase folding.
func quotePostgresIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// buildSyncQueries applies the incremental window when a timestamp column exists.
func (c *PostgreSQLConnector) buildSyncQueries(bases []string, request SyncRequest) []string {
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
func (c *PostgreSQLConnector) buildTimeFilteredQuery(base string, start, end *time.Time) string {
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

// buildSlimQuery selects only the columns needed to identify documents.
func (c *PostgreSQLConnector) buildSlimQuery(base string) string {
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
func (c *PostgreSQLConnector) wrapQuery(base string) string {
	return fmt.Sprintf("SELECT * FROM (%s) AS ragflow_src", c.stripOrderBy(base))
}

// stripOrderBy removes a trailing top-level ORDER BY clause.
func (c *PostgreSQLConnector) stripOrderBy(query string) string {
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

// formatDatetime renders a UTC time as an ISO-8601 PostgreSQL literal.
func (c *PostgreSQLConnector) formatDatetime(value time.Time) string {
	return "'" + value.UTC().Format(time.RFC3339Nano) + "'"
}

// scanRow scans the current row into an ordered column map.
func (c *PostgreSQLConnector) scanRow(rows *sql.Rows) (map[string]any, []string, error) {
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

// normalizeValue converts driver-specific values into plain strings so
// content and metadata rendering stay dialect-agnostic. Byte slices (jsonb)
// and driver value types (numeric) become their text form; time.Time passes
// through untouched.
func (c *PostgreSQLConnector) normalizeValue(value any) any {
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
func (c *PostgreSQLConnector) contentColumnsForRow(row map[string]any, orderedColumns []string) []string {
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
func (c *PostgreSQLConnector) buildContent(row map[string]any, columns []string) string {
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
// "postgresql:<database>:<id value>" with an MD5 content fallback.
func (c *PostgreSQLConnector) buildDocumentID(row map[string]any, orderedColumns []string) string {
	if c.idColumn != "" {
		if value, ok := row[c.idColumn]; ok && value != nil {
			return fmt.Sprintf("postgresql:%s:%s", c.database, fmt.Sprint(value))
		}
	}
	content := c.buildContent(row, c.contentColumnsForRow(row, orderedColumns))
	sum := md5.Sum([]byte(content))
	return fmt.Sprintf("postgresql:%s:%s", c.database, hex.EncodeToString(sum[:]))
}

// rowToSourceDocument converts a database row into the syncer model.
func (c *PostgreSQLConnector) rowToSourceDocument(row map[string]any, orderedColumns []string) (SourceDocument, bool) {
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

// renderValue formats a row value for document content.
func (c *PostgreSQLConnector) renderValue(value any) string {
	if typed, ok := value.(time.Time); ok {
		return typed.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprint(value)
}

// formatMetadataValue formats a row value for metadata, mirroring Python's
// isoformat for datetimes and string rendering otherwise.
func (c *PostgreSQLConnector) formatMetadataValue(value any) string {
	if typed, ok := value.(time.Time); ok {
		return typed.Format(time.RFC3339)
	}
	return fmt.Sprint(value)
}

// sanitizeQuery tolerates queries pasted from a markdown code fence.
func (c *PostgreSQLConnector) sanitizeQuery(raw string) string {
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
func (c *PostgreSQLConnector) splitColumns(value any) []string {
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

type postgresSyncSession struct {
	connector  *PostgreSQLConnector
	db         *sql.DB
	queries    []string
	queryIndex int
	rows       *sql.Rows
	batchSize  int
}

// NextBatch returns the next PostgreSQL document batch.
func (s *postgresSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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

// Close closes the PostgreSQL sync session.
func (s *postgresSyncSession) Close() error {
	s.closeRows()
	return s.db.Close()
}

// openNextQuery runs the next base query.
func (s *postgresSyncSession) openNextQuery(ctx context.Context) error {
	query := s.queries[s.queryIndex]
	s.queryIndex++
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("PostgreSQL query failed: %w", err)
	}
	s.rows = rows
	return nil
}

// closeRows releases the current result set.
func (s *postgresSyncSession) closeRows() {
	if s.rows != nil {
		s.rows.Close()
		s.rows = nil
	}
}

type postgresPruneSession struct {
	connector  *PostgreSQLConnector
	db         *sql.DB
	queries    []string
	queryIndex int
	rows       *sql.Rows
	batchSize  int
}

// NextBatch returns the next PostgreSQL prune snapshot batch.
func (s *postgresPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

// Close closes the PostgreSQL prune session.
func (s *postgresPruneSession) Close() error {
	s.closeRows()
	return s.db.Close()
}

// openNextQuery runs the next slim query.
func (s *postgresPruneSession) openNextQuery(ctx context.Context) error {
	query := s.queries[s.queryIndex]
	s.queryIndex++
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("PostgreSQL query failed: %w", err)
	}
	s.rows = rows
	return nil
}

// closeRows releases the current result set.
func (s *postgresPruneSession) closeRows() {
	if s.rows != nil {
		s.rows.Close()
		s.rows = nil
	}
}
