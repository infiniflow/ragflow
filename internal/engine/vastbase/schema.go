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
	"crypto/md5" // #nosec G501 -- MD5 provides a deterministic identifier checksum, not security.
	"database/sql"
	"fmt"
	"regexp"
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
	vectorDimRegex    = regexp.MustCompile(`^q_(\d+)_vec$`)
	textTypeRegex     = regexp.MustCompile(`(?i)^(character varying|varchar|character|char|text)`)
	ddlLocks          sync.Map
)

// chunkColumns mirrors conf/vastbase_mapping.json from the Python connector:
// column names are the ES field names verbatim, with PG types.
var chunkColumns = []columnDefinition{
	{"id", "varchar(256)", "''"},
	{"kb_id", "varchar(256)", "''"},
	{"doc_id", "varchar(256)", "''"},
	{"doc_type_kwd", "varchar(256)", "''"},
	{"docnm_kwd", "text", "''"},
	{"title_tks", "text", "''"},
	{"title_sm_tks", "text", "''"},
	{"content_with_weight", "text", "''"},
	{"content_ltks", "text", "''"},
	{"content_sm_ltks", "text", "''"},
	{"pagerank_fea", "integer", "0"},
	{"important_kwd", "text", "''"},
	{"important_tks", "text", "''"},
	{"question_kwd", "text", "''"},
	{"question_tks", "text", "''"},
	{"tag_kwd", "varchar(256)", "''"},
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
	{"entity_kwd", "varchar(256)", "''"},
	{"entity_type_kwd", "varchar(256)", "''"},
	{"from_entity_kwd", "varchar(256)", "''"},
	{"to_entity_kwd", "varchar(256)", "''"},
	{"weight_int", "integer", "0"},
	{"weight_flt", "double precision", "0.0"},
	{"entities_kwd", "text", "''"},
	{"rank_flt", "double precision", "0.0"},
	{"removed_kwd", "varchar(256)", "'N'"},
	{"raptor_kwd", "varchar(32)", "''"},
	{"raptor_layer_int", "integer", "0"},
	{"metadata", "text", "''"},
	{"extra", "text", "''"},
	{"chunk_order_int", "integer", "0"},
	{"compile_kwd", "varchar(32)", "''"},
	{"toc_kwd", "varchar(32)", "''"},
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

// metadataColumns mirrors conf/doc_meta_vastbase_mapping.json. Tables are
// per-tenant (ragflow_doc_meta_{tenantID}), so kb_id is not a discriminator
// here; it is a plain attribute column.
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

// fullTextFields are the chunk columns covered by the full-text index in both
// compatibility modes.
var fullTextFields = []string{
	"title_tks", "title_sm_tks", "important_kwd", "important_tks",
	"question_tks", "content_ltks", "content_sm_ltks",
}

var skillFullTextFields = []string{"name_tks", "tags_tks", "description_tks", "content_tks"}

// CreateChunkStore creates or upgrades the shared baseName table. The dataset
// ID is a row-level discriminator for chunk and memory tables.
func (e *Engine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error {
	if err := validateIdentifier(baseName); err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(baseName, "skill_") || datasetID == "skill":
		if err := e.ensureTable(ctx, baseName, skillColumns); err != nil {
			return err
		}
		if err := e.ensureFullTextIndexes(ctx, baseName, skillFullTextFields); err != nil {
			return err
		}
	case strings.HasPrefix(baseName, "memory_"):
		if err := e.ensureTable(ctx, baseName, memoryColumns); err != nil {
			return err
		}
		for _, field := range memoryIndexColumns {
			if err := e.ensureRegularIndex(ctx, baseName, field); err != nil {
				return err
			}
		}
	default:
		if err := e.ensureTable(ctx, baseName, chunkColumns); err != nil {
			return err
		}
		for _, field := range chunkIndexColumns {
			if err := e.ensureRegularIndex(ctx, baseName, field); err != nil {
				return err
			}
		}
		if err := e.ensureFullTextIndexes(ctx, baseName, fullTextFields); err != nil {
			return err
		}
	}
	return e.ensureVectorColumnAndIndex(ctx, baseName, vectorSize)
}

func (e *Engine) ensureTable(ctx context.Context, tableName string, columns []columnDefinition) error {
	primaryKey := primaryKeyColumn(columns)
	return e.withDDLLock(ctx, "vb_create_table_"+tableName, func() (bool, error) {
		return e.tableExists(ctx, tableName)
	}, func() error {
		definitions := make([]string, 0, len(columns)+1)
		for _, column := range columns {
			definition := quoteIdent(column.name) + " " + column.typeSQL
			if column.name == primaryKey {
				definition += " NOT NULL PRIMARY KEY"
			} else if column.defaultSQL != "" {
				definition += " DEFAULT " + column.defaultSQL
			}
			definitions = append(definitions, definition)
		}
		query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)",
			quoteIdent(tableName), strings.Join(definitions, ", "))
		_, err := e.db.ExecContext(ctx, query)
		return err
	})
}

func primaryKeyColumn(columns []columnDefinition) string {
	for _, column := range columns {
		switch column.name {
		case "id", "skill_id":
			return column.name
		}
	}
	return ""
}

func (e *Engine) ensureColumn(ctx context.Context, tableName string, column columnDefinition) error {
	return e.withDDLLock(ctx, "vb_add_"+column.name+"_"+tableName, func() (bool, error) {
		return e.columnExists(ctx, tableName, column.name)
	}, func() error {
		definition := quoteIdent(column.name) + " " + column.typeSQL
		if column.defaultSQL != "" {
			definition += " DEFAULT " + column.defaultSQL
		}
		// No IF NOT EXISTS: Vastbase rejects "ADD COLUMN IF NOT EXISTS" with a
		// syntax error (openGauss-derived kernels lack the clause). withDDLLock
		// already probes columnExists under the lock and tolerates the
		// already-exists race error.
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s",
			quoteIdent(tableName), definition))
		return err
	})
}

// ensureDynamicColumns adds columns for chunk fields the static schema does
// not know, mirroring ES dynamic mapping: q_(\d+)_vec becomes floatvector(N),
// anything else becomes varchar(256) with ” default. Known static columns are
// skipped so their declared types win.
func (e *Engine) ensureDynamicColumns(ctx context.Context, tableName string, fieldNames []string) error {
	static := make(map[string]bool, len(chunkColumns)+len(memoryColumns)+len(skillColumns))
	for _, column := range chunkColumns {
		static[column.name] = true
	}
	for _, column := range memoryColumns {
		static[column.name] = true
	}
	for _, column := range skillColumns {
		static[column.name] = true
	}
	for _, field := range fieldNames {
		if static[field] || !validFieldName(field) {
			continue
		}
		if err := e.ensureColumn(ctx, tableName, dynamicColumnDefinition(field)); err != nil {
			return err
		}
	}
	return nil
}

func dynamicColumnDefinition(field string) columnDefinition {
	if match := vectorDimRegex.FindStringSubmatch(field); match != nil {
		dim, _ := strconv.Atoi(match[1])
		return columnDefinition{name: field, typeSQL: fmt.Sprintf("floatvector(%d)", dim)}
	}
	return columnDefinition{name: field, typeSQL: "varchar(256)", defaultSQL: "''"}
}

func validFieldName(field string) bool {
	return identifierPattern.MatchString(field)
}

func (e *Engine) ensureRegularIndex(ctx context.Context, tableName, columnName string) error {
	indexName := regularIndexName(tableName, columnName)
	return e.withDDLLock(ctx, "vb_add_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)",
			quoteIdent(indexName), quoteIdent(tableName), quoteIdent(columnName)))
		return err
	})
}

// ensureFullTextIndexes creates the full-text structure for the compatibility
// mode. PG mode builds one GIN index over to_tsvector('cn_tokenizer', ...)
// expressions (failure is tolerated — vector search still works); B mode adds
// one "fulltext" index per field, tolerating already-exists.
func (e *Engine) ensureFullTextIndexes(ctx context.Context, tableName string, fields []string) error {
	if e.dbCompatibility == compatB {
		for _, field := range fields {
			indexName := regularIndexName(tableName, field+"_fulltext")
			err := e.withDDLLock(ctx, "vb_add_fulltext_idx_"+tableName+"_"+field, func() (bool, error) {
				return e.indexExists(ctx, tableName, indexName)
			}, func() error {
				_, err := e.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD INDEX %s USING \"fulltext\" (%s)",
					quoteIdent(tableName), quoteIdent(indexName), quoteIdent(field)))
				return err
			})
			if err != nil {
				// Python-parity degradation: a failed full-text index only
				// costs BM25 search (vector search works without it), so
				// ingestion must not be blocked on it. ChunkStoreExists still
				// requires the index, so later batches retry and self-heal
				// once the server accepts the DDL.
				common.Warn("Vastbase full-text index creation failed; vector search works without it",
					zap.String("table", tableName), zap.String("field", field), zap.Error(err))
			}
		}
		return nil
	}
	indexName := "text_gin_idx_" + tableName
	exists, err := e.indexExists(ctx, tableName, indexName)
	if err != nil || exists {
		return err
	}
	expressions := make([]string, 0, len(fields))
	for _, field := range fields {
		expressions = append(expressions, fmt.Sprintf("to_tsvector('cn_tokenizer', %s)", quoteIdent(field)))
	}
	query := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s USING gin (%s)",
		quoteIdent(indexName), quoteIdent(tableName), strings.Join(expressions, ", "))
	if _, err := e.db.ExecContext(ctx, query); err != nil {
		common.Warn("Vastbase PG full-text index creation failed; vector search works without it",
			zap.String("table", tableName), zap.Error(err))
	}
	return nil
}

func (e *Engine) ensureVectorColumnAndIndex(ctx context.Context, tableName string, vectorSize int) error {
	if vectorSize <= 0 {
		return nil
	}
	columnName := fmt.Sprintf("q_%d_vec", vectorSize)
	if err := e.ensureColumn(ctx, tableName, columnDefinition{
		name:    columnName,
		typeSQL: fmt.Sprintf("floatvector(%d)", vectorSize),
	}); err != nil {
		return err
	}
	indexName := regularIndexName(tableName, columnName+"_graph")
	return e.withDDLLock(ctx, "vb_add_vector_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf(
			"CREATE INDEX IF NOT EXISTS %s ON %s USING graph_index (%s floatvector_cosine_ops) WITH (m=16, ef_construction=50)",
			quoteIdent(indexName), quoteIdent(tableName), quoteIdent(columnName)))
		return err
	})
}

func regularIndexName(tableName, columnName string) string {
	indexName := fmt.Sprintf("ix_%s_%s", tableName, columnName)
	if len(indexName) <= maxIndexNameLength {
		return indexName
	}
	digest := fmt.Sprintf("%x", md5.Sum([]byte(indexName))) // #nosec G401 -- This is a non-security identifier checksum.
	suffix := "_" + digest[len(digest)-indexNameHashLength:]
	return indexName[:maxIndexNameLength-indexNameTruncationSpace] + suffix
}

func (e *Engine) withDDLLock(ctx context.Context, lockName string, check func() (bool, error), action func() error) error {
	value, _ := ddlLocks.LoadOrStore(lockName, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	exists, err := check()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	timeout := 60 * time.Second
	distributed := kvrocks.NewDistributedLock(lockName, "", timeout, timeout)
	if distributed != nil && !distributed.Acquire(ctx) {
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
				if distributed.Acquire(ctx) {
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

func (e *Engine) tableExists(ctx context.Context, tableName string) (bool, error) {
	if !validFieldName(tableName) {
		return false, fmt.Errorf("vastbase: invalid table name %q", tableName)
	}
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1",
		tableName).Scan(&count)
	return count > 0, err
}

func (e *Engine) columnExists(ctx context.Context, tableName, columnName string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2",
		tableName, columnName).Scan(&count)
	return count > 0, err
}

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
	if err := validateIdentifier(baseName); err != nil {
		return false, err
	}
	exists, err := e.tableExists(ctx, baseName)
	if err != nil || !exists {
		return exists, err
	}
	kind := tableKind(baseName, datasetID)
	var indexColumns []string
	fullText := false
	switch kind {
	case "memory":
		indexColumns = memoryIndexColumns
	case "skill":
		fullText = true
	default:
		indexColumns = chunkIndexColumns
		fullText = true
	}
	for _, column := range indexColumns {
		exists, err = e.indexExists(ctx, baseName, regularIndexName(baseName, column))
		if err != nil || !exists {
			return exists, err
		}
	}
	if fullText {
		if e.dbCompatibility == compatB {
			for _, field := range fullTextFields {
				exists, err = e.indexExists(ctx, baseName, regularIndexName(baseName, field+"_fulltext"))
				if err != nil || !exists {
					return exists, err
				}
			}
		} else {
			exists, err = e.indexExists(ctx, baseName, "text_gin_idx_"+baseName)
			if err != nil || !exists {
				return exists, err
			}
		}
	}
	return true, nil
}

// DropChunkStore keeps shared chunk and memory tables alive when only one
// dataset is removed. Skill tables and explicitly unscoped calls are dropped.
func (e *Engine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
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
			common.Info("Vastbase dropped chunk store rows",
				zap.String("table", baseName), zap.String("dataset", datasetID), zap.Int64("rows", affected))
		}
		return nil
	}
	_, err := e.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdent(baseName))
	return err
}

func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func validateIdentifier(identifier string) error {
	if identifier == "" || !identifierPattern.MatchString(identifier) {
		return fmt.Errorf("invalid SQL identifier: %q", identifier)
	}
	return nil
}

func isDuplicateDDLError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "already exists")
}
