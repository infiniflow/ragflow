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
	"crypto/md5" // #nosec G501 -- MD5 provides a deterministic identifier checksum, not security.
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/engine/redis"
)

const (
	maxIndexNameLength       = 64
	indexNameHashLength      = 4
	indexNameTruncationSpace = 8
)

type columnDefinition struct {
	name    string
	typeSQL string
}

var (
	identifierPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	metadataKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	ddlLocks           sync.Map
)

var chunkColumns = []columnDefinition{
	{"id", "VARCHAR(256) NOT NULL PRIMARY KEY"},
	{"kb_id", "VARCHAR(256) NOT NULL"},
	{"doc_id", "VARCHAR(256) NULL"},
	{"docnm_kwd", "VARCHAR(256) NULL"},
	{"doc_type_kwd", "VARCHAR(256) NULL"},
	{"title_tks", "VARCHAR(256) NULL"},
	{"title_sm_tks", "VARCHAR(256) NULL"},
	{"content_with_weight", "LONGTEXT NULL"},
	{"content_ltks", "LONGTEXT NULL"},
	{"content_sm_ltks", "LONGTEXT NULL"},
	{"pagerank_fea", "INTEGER NULL"},
	{"important_kwd", "ARRAY(VARCHAR(256)) NULL"},
	{"important_tks", "TEXT NULL"},
	{"question_kwd", "ARRAY(VARCHAR(1024)) NULL"},
	{"question_tks", "TEXT NULL"},
	{"tag_kwd", "ARRAY(VARCHAR(256)) NULL"},
	{"tag_feas", "JSON NULL"},
	{"available_int", "INTEGER NOT NULL DEFAULT 1"},
	{"create_time", "VARCHAR(19) NULL"},
	{"create_timestamp_flt", "DOUBLE NULL"},
	{"img_id", "VARCHAR(128) NULL"},
	{"position_int", "ARRAY(ARRAY(INTEGER)) NULL"},
	{"page_num_int", "ARRAY(INTEGER) NULL"},
	{"top_int", "ARRAY(INTEGER) NULL"},
	{"knowledge_graph_kwd", "VARCHAR(256) NULL"},
	{"source_id", "ARRAY(VARCHAR(256)) NULL"},
	{"entity_kwd", "VARCHAR(256) NULL"},
	{"entity_type_kwd", "VARCHAR(256) NULL"},
	{"from_entity_kwd", "VARCHAR(256) NULL"},
	{"to_entity_kwd", "VARCHAR(256) NULL"},
	{"weight_int", "INTEGER NULL"},
	{"weight_flt", "DOUBLE NULL"},
	{"entities_kwd", "ARRAY(VARCHAR(256)) NULL"},
	{"rank_flt", "DOUBLE NULL"},
	{"n_hop_with_weight", "LONGTEXT NULL"},
	{"removed_kwd", "VARCHAR(256) NULL DEFAULT 'N'"},
	{"raptor_kwd", "VARCHAR(256) NULL"},
	{"raptor_layer_int", "INTEGER NULL"},
	{"chunk_data", "JSON NULL"},
	{"metadata", "JSON NULL"},
	{"extra", "JSON NULL"},
	{"_order_id", "INTEGER NULL"},
	{"group_id", "VARCHAR(256) NULL"},
	{"mom_id", "VARCHAR(256) NULL"},
}

var chunkExtraColumns = selectColumnDefinitions(chunkColumns,
	"_order_id",
	"group_id",
	"mom_id",
	"chunk_data",
	"raptor_kwd",
	"raptor_layer_int",
	"n_hop_with_weight",
)

var memoryColumns = []columnDefinition{
	{"id", "VARCHAR(256) NOT NULL PRIMARY KEY"},
	{"message_id", "VARCHAR(256) NOT NULL"},
	{"message_type_kwd", "VARCHAR(64) NULL"},
	{"source_id", "VARCHAR(256) NULL"},
	{"memory_id", "VARCHAR(256) NOT NULL"},
	{"user_id", "VARCHAR(256) NULL"},
	{"agent_id", "VARCHAR(256) NULL"},
	{"session_id", "VARCHAR(256) NULL"},
	{"zone_id", "INTEGER NULL DEFAULT 0"},
	{"valid_at", "VARCHAR(64) NULL"},
	{"invalid_at", "VARCHAR(64) NULL"},
	{"forget_at", "VARCHAR(64) NULL"},
	{"status_int", "INTEGER NOT NULL DEFAULT 1"},
	{"content_ltks", "LONGTEXT NULL"},
	{"tokenized_content_ltks", "LONGTEXT NULL"},
}

var metadataColumns = []columnDefinition{
	{"id", "VARCHAR(256) NOT NULL PRIMARY KEY"},
	{"kb_id", "VARCHAR(256) NOT NULL"},
	{"meta_fields", "JSON NULL"},
}

var skillColumns = []columnDefinition{
	{"skill_id", "VARCHAR(256) NOT NULL PRIMARY KEY"},
	{"space_id", "VARCHAR(256) NULL"},
	{"folder_id", "VARCHAR(256) NULL"},
	{"name", "LONGTEXT NULL"},
	{"name_tks", "LONGTEXT NULL"},
	{"tags", "LONGTEXT NULL"},
	{"tags_tks", "LONGTEXT NULL"},
	{"description", "LONGTEXT NULL"},
	{"description_tks", "LONGTEXT NULL"},
	{"content", "LONGTEXT NULL"},
	{"content_tks", "LONGTEXT NULL"},
	{"version", "VARCHAR(64) NULL"},
	{"status", "VARCHAR(64) NULL"},
	{"create_time", "BIGINT NULL DEFAULT 0"},
	{"update_time", "BIGINT NULL DEFAULT 0"},
}

var chunkIndexColumns = []string{
	"kb_id", "doc_id", "available_int", "knowledge_graph_kwd", "entity_type_kwd", "removed_kwd",
}

var memoryIndexColumns = []string{"message_id", "memory_id", "status_int"}

var originalFullTextFields = []string{"docnm_kwd", "content_with_weight", "important_tks", "question_tks"}
var tokenizedFullTextFields = []string{"title_tks", "title_sm_tks", "important_tks", "question_tks", "content_ltks", "content_sm_ltks"}

// CreateChunkStore creates or upgrades the legacy shared tenant table. The
// dataset ID is a row-level discriminator for chunk and memory tables.
func (e *Engine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error {
	if err := validateIdentifier(baseName); err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(baseName, "skill_") || datasetID == "skill":
		if err := e.ensureTable(ctx, baseName, skillColumns, "ob_"); err != nil {
			return err
		}
		for _, field := range []string{"name_tks", "tags_tks", "description_tks", "content_tks"} {
			if err := e.ensureFullTextIndex(ctx, baseName, field, "ob_"); err != nil {
				return err
			}
		}
	case strings.HasPrefix(baseName, "memory_"):
		if err := e.ensureTable(ctx, baseName, memoryColumns, "ob_memory_"); err != nil {
			return err
		}
		for _, field := range memoryIndexColumns {
			if err := e.ensureRegularIndex(ctx, baseName, field, "ob_memory_"); err != nil {
				return err
			}
		}
		if e.flags.enableFullTextSearch {
			for _, field := range []string{"content_ltks", "tokenized_content_ltks"} {
				if err := e.ensureFullTextIndex(ctx, baseName, field, "ob_memory_"); err != nil {
					return err
				}
			}
		}
	default:
		if err := e.ensureTable(ctx, baseName, chunkColumns, "ob_"); err != nil {
			return err
		}
		for _, column := range chunkExtraColumns {
			if err := e.ensureColumn(ctx, baseName, column, "ob_"); err != nil {
				return err
			}
		}
		for _, field := range chunkIndexColumns {
			if err := e.ensureRegularIndex(ctx, baseName, field, "ob_"); err != nil {
				return err
			}
		}
		if e.flags.enableFullTextSearch {
			fields := tokenizedFullTextFields
			if e.flags.searchOriginalContent {
				fields = originalFullTextFields
			}
			for _, field := range fields {
				if err := e.ensureFullTextIndex(ctx, baseName, field, "ob_"); err != nil {
					return err
				}
			}
		}
	}
	return e.ensureVectorColumnAndIndex(ctx, baseName, vectorSize, lockPrefix(baseName))
}

func (e *Engine) ensureTable(ctx context.Context, tableName string, columns []columnDefinition, prefix string) error {
	return e.ensureTableWithLock(ctx, tableName, columns, prefix+"create_table_"+tableName)
}

func (e *Engine) ensureTableWithLock(ctx context.Context, tableName string, columns []columnDefinition, lockName string) error {
	return e.withDDLLock(ctx, lockName, func() (bool, error) {
		return e.tableExists(ctx, tableName)
	}, func() error {
		definitions := make([]string, 0, len(columns))
		for _, column := range columns {
			definitions = append(definitions, quoteIdentifier(column.name)+" "+column.typeSQL)
		}
		query := fmt.Sprintf("CREATE TABLE %s (%s) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ORGANIZATION=heap",
			quoteIdentifier(tableName), strings.Join(definitions, ", "))
		_, err := e.db.ExecContext(ctx, query)
		return err
	})
}

func (e *Engine) ensureColumn(ctx context.Context, tableName string, column columnDefinition, prefix string) error {
	return e.withDDLLock(ctx, prefix+"add_"+column.name+"_"+tableName, func() (bool, error) {
		return e.columnExists(ctx, tableName, column.name)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			quoteIdentifier(tableName), quoteIdentifier(column.name), column.typeSQL))
		return err
	})
}

func (e *Engine) ensureRegularIndex(ctx context.Context, tableName, columnName, prefix string) error {
	indexName := regularIndexName(tableName, columnName)
	return e.withDDLLock(ctx, prefix+"add_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("CREATE INDEX %s ON %s (%s)",
			quoteIdentifier(indexName), quoteIdentifier(tableName), quoteIdentifier(columnName)))
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

func (e *Engine) ensureFullTextIndex(ctx context.Context, tableName, columnName, prefix string) error {
	indexName := "fts_idx_" + columnName
	return e.withDDLLock(ctx, prefix+"add_fulltext_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("CREATE FULLTEXT INDEX %s ON %s (%s) WITH PARSER IK",
			quoteIdentifier(indexName), quoteIdentifier(tableName), quoteIdentifier(columnName)))
		return err
	})
}

func (e *Engine) ensureVectorColumnAndIndex(ctx context.Context, tableName string, vectorSize int, prefix string) error {
	if vectorSize <= 0 {
		return nil
	}
	columnName := fmt.Sprintf("q_%d_vec", vectorSize)
	if err := e.withDDLLock(ctx, prefix+"add_vector_column_"+tableName+"_"+columnName, func() (bool, error) {
		return e.columnExists(ctx, tableName, columnName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s VECTOR(%d) NULL",
			quoteIdentifier(tableName), quoteIdentifier(columnName), vectorSize))
		return err
	}); err != nil {
		return err
	}
	indexName := columnName + "_idx"
	return e.withDDLLock(ctx, prefix+"add_vector_idx_"+tableName+"_"+columnName, func() (bool, error) {
		return e.indexExists(ctx, tableName, indexName)
	}, func() error {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("CREATE VECTOR INDEX %s ON %s (%s) WITH (distance=cosine, type=hnsw, lib=vsag)",
			quoteIdentifier(indexName), quoteIdentifier(tableName), quoteIdentifier(columnName)))
		return err
	})
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

	timeoutSeconds := int64(60)
	if raw := strings.TrimSpace(os.Getenv("OB_DDL_TIMEOUT")); raw != "" {
		if parsed, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}
	timeout := time.Duration(timeoutSeconds) * time.Second
	distributed := redis.NewDistributedLock(lockName, "", timeout, timeout)
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
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?",
		e.dbName, tableName).Scan(&count)
	return count > 0, err
}

func (e *Engine) columnExists(ctx context.Context, tableName, columnName string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?",
		e.dbName, tableName, columnName).Scan(&count)
	return count > 0, err
}

func (e *Engine) indexExists(ctx context.Context, tableName, indexName string) (bool, error) {
	var count int
	err := e.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?",
		e.dbName, tableName, indexName).Scan(&count)
	return count > 0, err
}

func (e *Engine) findVectorColumn(ctx context.Context, tableName, expectedColumn string) (string, error) {
	query := "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME REGEXP '^q_[0-9]+_vec$'"
	args := []interface{}{e.dbName, tableName}
	if expectedColumn != "" {
		if !vectorColumnPattern.MatchString(expectedColumn) {
			return "", fmt.Errorf("invalid vector column: %s", expectedColumn)
		}
		query += " AND COLUMN_NAME = ?"
		args = append(args, expectedColumn)
	}
	query += " ORDER BY COLUMN_NAME LIMIT 1"
	var columnName string
	err := e.db.QueryRowContext(ctx, query, args...).Scan(&columnName)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return columnName, err
}

// ChunkStoreExists checks the shared physical table. CreateChunkStore performs
// the additive compatibility upgrade when a caller needs a new vector size.
func (e *Engine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	if err := validateIdentifier(baseName); err != nil {
		return false, err
	}
	exists, err := e.tableExists(ctx, baseName)
	if err != nil || !exists {
		return exists, err
	}
	kind := tableKind(baseName, datasetID)
	var indexColumns, fullTextColumns []string
	switch kind {
	case "memory":
		indexColumns = memoryIndexColumns
		if e.flags.enableFullTextSearch {
			fullTextColumns = []string{"content_ltks", "tokenized_content_ltks"}
		}
	case "skill":
		fullTextColumns = []string{"name_tks", "tags_tks", "description_tks", "content_tks"}
	default:
		indexColumns = chunkIndexColumns
		if e.flags.enableFullTextSearch {
			fullTextColumns = tokenizedFullTextFields
			if e.flags.searchOriginalContent {
				fullTextColumns = originalFullTextFields
			}
		}
		for _, column := range chunkExtraColumns {
			exists, err = e.columnExists(ctx, baseName, column.name)
			if err != nil || !exists {
				return exists, err
			}
		}
	}
	for _, column := range indexColumns {
		exists, err = e.indexExists(ctx, baseName, regularIndexName(baseName, column))
		if err != nil || !exists {
			return exists, err
		}
	}
	for _, column := range fullTextColumns {
		exists, err = e.indexExists(ctx, baseName, "fts_idx_"+column)
		if err != nil || !exists {
			return exists, err
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
		_, err = e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?",
			quoteIdentifier(baseName), quoteIdentifier(field)), datasetID)
		return err
	}
	_, err := e.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdentifier(baseName))
	return err
}

func lockPrefix(tableName string) string {
	if strings.HasPrefix(tableName, "memory_") {
		return "ob_memory_"
	}
	return "ob_"
}

func selectColumnDefinitions(columns []columnDefinition, names ...string) []columnDefinition {
	byName := make(map[string]columnDefinition, len(columns))
	for _, column := range columns {
		byName[column.name] = column
	}
	selected := make([]columnDefinition, 0, len(names))
	for _, name := range names {
		column, ok := byName[name]
		if !ok {
			panic("missing column definition: " + name)
		}
		selected = append(selected, column)
	}
	return selected
}

func quoteIdentifier(identifier string) string { return "`" + identifier + "`" }

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
