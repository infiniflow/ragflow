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

package opengauss

import (
	"context"
	"crypto/md5" // #nosec G501 -- MD5 provides a deterministic identifier checksum, not security.
	"crypto/sha256"
	"database/sql"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/kvrocks"

	"go.uber.org/zap"
)

const (
	maxIndexNameLength       = 63
	indexNameHashLength      = 4
	indexNameTruncationSpace = 8
)

// columnDefinition is one column of the static table schemas. defaultSQL is a
// rendered SQL literal ("" means no DEFAULT clause). Text columns default to
// ” instead of NULL so that the ES-parity exists-filter can treat an empty
// string as "field absent".
type columnDefinition struct {
	name       string
	typeSQL    string
	defaultSQL string
}

// columnMeta describes a column as reported by information_schema.
type columnMeta struct {
	name          string
	dataType      string
	columnDefault sql.NullString
	nullable      string
}

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

	textTypeRegex = regexp.MustCompile(`(?i)^(character varying|varchar|character|char|text)`)
	ddlLocks      sync.Map
)

// Chunk tables use (kb_id, id) as their primary key.
var chunkColumns = []columnDefinition{
	{"id", "varchar(256)", "''"},
	{"kb_id", "varchar(256)", "''"},
	{"doc_id", "varchar(256)", "''"},
	{"doc_type_kwd", "text", "''"},
	{"docnm_kwd", "text", "''"},
	{"title_tks", "text", "''"},
	{"title_sm_tks", "text", "''"},
	{"content_with_weight", "text", "''"},
	{"content_ltks", "text", "''"},
	{"content_sm_ltks", "text", "''"},
	{"pagerank_fea", "double precision", "0.0"},
	{"important_kwd", "text", "''"},
	{"important_tks", "text", "''"},
	{"question_kwd", "text", "''"},
	{"question_tks", "text", "''"},
	{"tag_kwd", "text", "''"},
	{"tag_feas", "text", "''"},
	{"available_int", "integer", "1"},
	{"create_time", "varchar(32)", "''"},
	{"create_timestamp_flt", "double precision", "0.0"},
	{"img_id", "varchar(128)", "''"},
	{"position_int", "integer[]", ""},
	{"page_num_int", "integer[]", ""},
	{"top_int", "integer[]", ""},
	{"knowledge_graph_kwd", "varchar(256)", "''"},
	{"source_id", "text", "''"},
	{"entity_kwd", "text", "''"},
	{"entity_type_kwd", "text", "''"},
	{"from_entity_kwd", "text", "''"},
	{"to_entity_kwd", "text", "''"},
	{"weight_int", "integer", "0"},
	{"weight_flt", "double precision", "0.0"},
	{"entities_kwd", "text", "''"},
	{"rank_flt", "double precision", "0.0"},
	{"removed_kwd", "text", "'N'"},
	{"raptor_kwd", "text", "''"},
	{"raptor_layer_int", "integer", "0"},
	{"metadata", "text", "''"},
	{"extra", "text", "''"},
	{"chunk_order_int", "integer", "0"},
	{"compile_kwd", "text", "''"},
	{"toc_kwd", "text", "''"},
	{"mom_id", "varchar(128)", "''"},
	{"mom_with_weight", "text", "''"},
}

var memoryColumns = []columnDefinition{
	{"id", "varchar(256)", "''"},
	{"message_id", "varchar(256)", "''"},
	{"message_type_kwd", "varchar(64)", "''"},
	{"source_id", "text", "''"},
	{"memory_id", "varchar(256)", "''"},
	{"user_id", "varchar(256)", "''"},
	{"agent_id", "varchar(256)", "''"},
	{"session_id", "varchar(256)", "''"},
	{"zone_id", "integer", "0"},
	{"valid_at", "varchar(64)", "''"},
	{"invalid_at", "varchar(64)", "''"},
	{"forget_at", "varchar(64)", "''"},
	{"status_int", "integer", "1"},
	{"content_ltks", "text", "''"},
	{"tokenized_content_ltks", "text", "''"},
}

// Metadata tables store one row per document.
var metadataColumns = []columnDefinition{
	{"id", "varchar(128)", "''"},
	{"kb_id", "varchar(128)", "''"},
	{"doc_id", "varchar(128)", "''"},
	{"create_time", "varchar(32)", "''"},
	{"create_timestamp_flt", "double precision", "0.0"},
	{"meta_fields", "text", "'{}'"},
}

var skillColumns = []columnDefinition{
	{"skill_id", "varchar(256)", "''"},
	{"space_id", "varchar(256)", "''"},
	{"folder_id", "varchar(256)", "''"},
	{"name", "text", "''"},
	{"name_tks", "text", "''"},
	{"tags", "text", "''"},
	{"tags_tks", "text", "''"},
	{"description", "text", "''"},
	{"description_tks", "text", "''"},
	{"content", "text", "''"},
	{"content_tks", "text", "''"},
	{"version", "varchar(64)", "''"},
	{"status", "varchar(64)", "''"},
	{"create_time", "bigint", "0"},
	{"update_time", "bigint", "0"},
}

var chunkIndexColumns = []string{
	"kb_id", "doc_id", "available_int", "knowledge_graph_kwd", "entity_type_kwd", "removed_kwd",
}

var memoryIndexColumns = []string{"message_id", "memory_id", "status_int"}

var fullTextFields = []string{
	"title_tks", "title_sm_tks", "important_kwd", "important_tks",
	"question_tks", "content_ltks", "content_sm_ltks",
}

var skillFullTextFields = []string{"name_tks", "tags_tks", "description_tks", "content_tks"}

// CreateChunkStore creates or upgrades the shared baseName table. The dataset
// ID is a row-level discriminator for chunk and memory tables.
func (e *Engine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID, language string) error {
	baseName = physicalTableName(baseName)
	if err := validateIdentifier(baseName); err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(baseName, "skill_") || datasetID == "skill":
		if err := e.ensureTable(ctx, baseName, skillColumns, false); err != nil {
			return err
		}
		if err := e.ensureBM25Indexes(ctx, baseName, skillFullTextFields); err != nil {
			return err
		}
	case strings.HasPrefix(baseName, "memory_"):
		if err := e.ensureTable(ctx, baseName, memoryColumns, false); err != nil {
			return err
		}
		if err := e.ensureBM25Index(ctx, baseName, "tokenized_content_ltks"); err != nil {
			return err
		}
		for _, field := range memoryIndexColumns {
			if err := e.ensureRegularIndex(ctx, baseName, field); err != nil {
				return err
			}
		}
	default:
		if err := e.ensureTable(ctx, baseName, chunkColumns, true); err != nil {
			return err
		}
		for _, field := range chunkIndexColumns {
			if err := e.ensureRegularIndex(ctx, baseName, field); err != nil {
				return err
			}
		}
		if err := e.ensureBM25Indexes(ctx, baseName, chunkBM25Fields); err != nil {
			return err
		}
	}
	return e.ensureVectorColumnAndIndex(ctx, baseName, vectorSize)
}

// ensureTable creates tableName with the given columns under the DDL lock,
// turning an already-present table into a no-op.
func (e *Engine) ensureTable(ctx context.Context, tableName string, columns []columnDefinition, composite bool) error {
	primaryKey := primaryKeyColumn(columns)
	return e.withDDLLock(ctx, "og_create_table_"+tableName, func() (bool, error) {
		return e.tableExists(ctx, tableName)
	}, func() error {
		definitions := make([]string, 0, len(columns)+1)
		for _, column := range columns {
			definition := quoteIdent(column.name) + " " + column.typeSQL
			if composite && (column.name == "kb_id" || column.name == "id") {
				definition += " NOT NULL"
			} else if column.name == primaryKey {
				definition += " NOT NULL PRIMARY KEY"
			} else if column.defaultSQL != "" {
				definition += " DEFAULT " + column.defaultSQL
			}
			definitions = append(definitions, definition)
		}
		if composite {
			definitions = append(definitions, "PRIMARY KEY (kb_id, id)")
		}
		query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)",
			quoteIdent(tableName), strings.Join(definitions, ", "))
		query += " WITH (storage_type=ASTORE)"
		_, err := e.db.ExecContext(ctx, query)
		return err
	})
}

// primaryKeyColumn picks the primary key column for a column set: id, or
// skill_id for skill tables.
func primaryKeyColumn(columns []columnDefinition) string {
	for _, column := range columns {
		switch column.name {
		case "id", "skill_id":
			return column.name
		}
	}
	return ""
}

// ensureColumn adds one column to a table under the DDL lock when missing.
func (e *Engine) ensureColumn(ctx context.Context, tableName string, column columnDefinition) error {
	return e.withDDLLock(ctx, "og_add_"+column.name+"_"+tableName, func() (bool, error) {
		return e.columnExists(ctx, tableName, column.name)
	}, func() error {
		definition := quoteIdent(column.name) + " " + column.typeSQL
		if column.defaultSQL != "" {
			definition += " DEFAULT " + column.defaultSQL
		}
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s",
			quoteIdent(tableName), definition))
		return err
	})
}

// ensureDynamicColumns provisions document fields and indexes new vector dimensions.
func (e *Engine) ensureDynamicColumns(ctx context.Context, tableName, kind string, fieldNames []string) error {
	definitions := chunkColumns
	switch kind {
	case "memory":
		definitions = memoryColumns
	case "skill":
		definitions = skillColumns
	}
	static := make(map[string]bool, len(definitions))
	for _, column := range definitions {
		static[column.name] = true
	}
	for _, field := range fieldNames {
		if static[field] || !validFieldName(field) {
			continue
		}
		if match := vectorColumnPattern.FindStringSubmatch(field); match != nil {
			dim, err := strconv.Atoi(match[1])
			if err != nil || dim <= 0 {
				return fmt.Errorf("invalid vector column %q", field)
			}
			if err := e.ensureVectorColumnAndIndex(ctx, tableName, dim); err != nil {
				return err
			}
			continue
		}
		if err := e.ensureColumn(ctx, tableName, columnDefinition{name: field, typeSQL: "text", defaultSQL: "''"}); err != nil {
			return err
		}
	}
	return nil
}

// validFieldName reports whether name matches the identifier pattern every
// table, column, and index component must satisfy.
func validFieldName(field string) bool {
	return identifierPattern.MatchString(field)
}

// ensureRegularIndex creates a b-tree index on one column when absent,
// serialized by the DDL lock.
func (e *Engine) ensureRegularIndex(ctx context.Context, tableName, columnName string) error {
	indexName := regularIndexName(tableName, columnName)
	return e.withDDLLock(ctx, "og_add_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
			quoteIdent(indexName), quoteIdent(tableName), quoteIdent(columnName)))
		return err
	})
}

// ensureVectorColumnAndIndex provisions the q_N_vec column for an embedding
// size and its HNSW graph index.
func (e *Engine) ensureVectorColumnAndIndex(ctx context.Context, tableName string, vectorSize int) error {
	if vectorSize <= 0 {
		return nil
	}
	columnName := common.VectorColumnName(vectorSize)
	if err := e.ensureColumn(ctx, tableName, columnDefinition{
		name:    columnName,
		typeSQL: fmt.Sprintf("vector(%d)", vectorSize),
	}); err != nil {
		return err
	}
	indexName := regularIndexName(tableName, columnName+"_graph")
	return e.withDDLLock(ctx, "og_add_vector_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s vector_cosine_ops) WITH (m=16, ef_construction=64)",
			quoteIdent(indexName), quoteIdent(tableName), quoteIdent(columnName)))
		return err
	})
}

// regularIndexName builds ix_<table>_<column>, hashing the tail when the
// name would exceed the kernel's index-name length limit.
func regularIndexName(tableName, columnName string) string {
	indexName := fmt.Sprintf("ix_%s_%s", tableName, columnName)
	if len(indexName) <= maxIndexNameLength {
		return indexName
	}
	digest := fmt.Sprintf("%x", md5.Sum([]byte(indexName))) // #nosec G401 -- This is a non-security identifier checksum.
	suffix := "_" + digest[len(digest)-indexNameHashLength:]
	return indexName[:maxIndexNameLength-indexNameTruncationSpace] + suffix
}

// withDDLLock serializes a check-then-act DDL step on one object per
// process: probe under the lock and act only when the probe misses.
func (e *Engine) withDDLLock(ctx context.Context, lockName string, check func() (bool, error), action func() error) error {
	value, _ := ddlLocks.LoadOrStore(lockName, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
		defer func() { <-lock }()
	case <-ctx.Done():
		return ctx.Err()
	}

	exists, err := check()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	timeout := 60 * time.Second
	distributed := kvrocks.NewDistributedLock(lockName, "", timeout, timeout)
	acquired := false
	if distributed != nil {
		acquired, err = distributed.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("acquire distributed DDL lock %s: %w", lockName, err)
		}
	}
	if distributed != nil && !acquired {
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
	waitForLock:
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
				return fmt.Errorf("timeout waiting for DDL %s", lockName)
			case <-ticker.C:
				exists, err = check()
				if err != nil {
					return err
				}
				if exists {
					return nil
				}
				acquired, err = distributed.Acquire(ctx)
				if err != nil {
					return fmt.Errorf("acquire distributed DDL lock %s: %w", lockName, err)
				}
				if acquired {
					break waitForLock
				}
			}
		}
	}
	if distributed != nil {
		defer distributed.Release(ctx)
		exists, err = check()
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}
	if err := action(); err != nil && !isDuplicateDDLError(err) {
		return fmt.Errorf("DDL %s: %w", lockName, err)
	}
	exists, err = check()
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("DDL %s completed without creating the requested object", lockName)
	}
	return nil
}

// tableExists probes the current schema for a table by name.
func (e *Engine) tableExists(ctx context.Context, tableName string) (bool, error) {
	if !validFieldName(tableName) {
		return false, fmt.Errorf("opengauss: invalid table name %q", tableName)
	}
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1",
		tableName).Scan(&count)
	return count > 0, err
}

// columnExists probes a table for a column by name.
func (e *Engine) columnExists(ctx context.Context, tableName, columnName string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2",
		tableName, columnName).Scan(&count)
	return count > 0, err
}

// indexExists probes pg_indexes for a named index.
func (e *Engine) indexExists(ctx context.Context, tableName, indexName string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2",
		tableName, indexName).Scan(&count)
	return count > 0, err
}

// listTableColumns returns the live column set of a table, or nil when the
// table does not exist. Callers use it both to filter SELECT fields down to
// existing columns and to drive the ES-parity exists-filter semantics.
func (e *Engine) listTableColumns(ctx context.Context, tableName string) (map[string]columnMeta, error) {
	exists, err := e.tableExists(ctx, tableName)
	if err != nil || !exists {
		return nil, err
	}
	rows, err := e.db.QueryContext(ctx,
		"SELECT column_name, data_type, column_default, is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1",
		tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]columnMeta)
	for rows.Next() {
		var meta columnMeta
		if err := rows.Scan(&meta.name, &meta.dataType, &meta.columnDefault, &meta.nullable); err != nil {
			return nil, err
		}
		columns[meta.name] = meta
	}
	return columns, rows.Err()
}

// isTextColumn reports whether a column uses a character type, which decides
// the ES-parity empty-string handling in filters.
func isTextColumn(dataType string) bool {
	return textTypeRegex.MatchString(strings.TrimSpace(dataType))
}

// ChunkStoreExists checks the shared physical table and its static contract
// (base columns plus the indexes CreateChunkStore would build). Dynamic
// columns are not part of the check.
func (e *Engine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	baseName = physicalTableName(baseName)
	if err := validateIdentifier(baseName); err != nil {
		return false, err
	}
	exists, err := e.tableExists(ctx, baseName)
	if err != nil || !exists {
		return exists, err
	}
	kind := tableKind(baseName, datasetID)
	indexColumns := chunkIndexColumns
	switch kind {
	case "memory":
		indexColumns = memoryIndexColumns
	case "skill":
		indexColumns = nil
	}
	for _, column := range indexColumns {
		exists, err = e.indexExists(ctx, baseName, regularIndexName(baseName, column))
		if err != nil || !exists {
			return exists, err
		}
	}
	return e.bm25IndexesExist(ctx, baseName, bm25Fields(kind))
}

// DropChunkStore keeps shared chunk and memory tables alive when only one
// dataset is removed. Skill tables and explicitly unscoped calls are dropped.
func (e *Engine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	baseName = physicalTableName(baseName)
	if err := validateIdentifier(baseName); err != nil {
		return err
	}
	if datasetID != "" && datasetID != "skill" {
		exists, err := e.tableExists(ctx, baseName)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
		field := "kb_id"
		if strings.HasPrefix(baseName, "memory_") {
			field = "memory_id"
		}
		result, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = $1",
			quoteIdent(baseName), quoteIdent(field)), datasetID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err == nil && affected > 0 {
			common.Info("openGauss dropped chunk store rows",
				zap.String("table", baseName), zap.String("dataset", datasetID), zap.Int64("rows", affected))
		}
		return nil
	}
	_, err := e.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdent(baseName))
	return err
}

// physicalTableName preserves the identity of skill spaces with long names.
func physicalTableName(name string) string {
	if len(name) > maxIndexNameLength && strings.HasPrefix(name, "skill_") {
		sum := sha256.Sum256([]byte(name))
		return fmt.Sprintf("skill_%x", sum[:16])
	}
	return name
}

// quoteIdent double-quotes an SQL identifier, escaping embedded quotes.
func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// validateIdentifier rejects empty names or names outside the identifier
// pattern before they reach any SQL text.
func validateIdentifier(identifier string) error {
	if identifier == "" || !identifierPattern.MatchString(identifier) {
		return fmt.Errorf("invalid SQL identifier: %q", identifier)
	}
	return nil
}

// isDuplicateDDLError reports whether a DDL failure is a benign
// already-exists race rather than a real error.
func isDuplicateDDLError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "already exists")
}

var chunkBM25Fields = append(slices.Clone(fullTextFields), "entity_kwd", "from_entity_kwd", "to_entity_kwd", "content_with_weight")

func bm25Fields(kind string) []string {
	switch kind {
	case "memory":
		return []string{"tokenized_content_ltks"}
	case "skill":
		return skillFullTextFields
	default:
		return chunkBM25Fields
	}
}

func (e *Engine) bm25IndexesExist(ctx context.Context, table string, fields []string) (bool, error) {
	args := []interface{}{table}
	for _, field := range fields {
		args = append(args, regularIndexName(table, field+"_bm25"))
	}
	var count int
	err := e.db.QueryRowContext(ctx, "SELECT count(*) FROM pg_indexes WHERE schemaname=current_schema() AND tablename=$1 AND indexname IN ("+placeholderList(len(fields), 1)+")", args...).Scan(&count)
	return count == len(fields), err
}

// Ensure indexes on existing tables before a query first uses a new field.
func (e *Engine) ensureBM25Indexes(ctx context.Context, table string, fields []string) error {
	ready, err := e.bm25IndexesExist(ctx, table, fields)
	if ready || err != nil {
		return err
	}
	for _, field := range fields {
		if err := e.ensureBM25Index(ctx, table, field); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) ensureBM25Index(ctx context.Context, table, field string) error {
	index := regularIndexName(table, field+"_bm25")
	return e.withDDLLock(ctx, index, func() (bool, error) { return e.indexExists(ctx, table, index) }, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s USING bm25 ((%s::text))", quoteIdent(index), quoteIdent(table), quoteIdent(field)))
		return err
	})
}
