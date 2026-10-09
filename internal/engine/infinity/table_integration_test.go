//go:build integration

package infinity

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	ingestiontable "ragflow/internal/ingestion/table"
	"ragflow/internal/server/config"
)

func TestTableColumnSQLRoundTrip(t *testing.T) {
	uri := os.Getenv("RAGFLOW_TEST_INFINITY_URI")
	if uri == "" {
		t.Skip("RAGFLOW_TEST_INFINITY_URI is not set")
	}
	port := 5432
	if value := os.Getenv("RAGFLOW_TEST_INFINITY_POSTGRES_PORT"); value != "" {
		var err error
		port, err = strconv.Atoi(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	e, err := NewEngine(ctx, config.InfinityConfig{URI: uri, PostgresPort: port, DBName: "default_db"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	base := fmt.Sprintf("ragflow_column_test_%d", time.Now().UnixNano())
	kb := "table"
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := e.DropChunkStore(cleanupCtx, base, kb); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	// Start with an ordinary store to exercise adding the table columns on insert.
	if err := e.CreateChunkStore(ctx, base, kb, 2, "naive"); err != nil {
		t.Fatal(err)
	}
	col := ingestiontable.DeriveColumns([]string{`金额 "折扣" / %`})[0]
	chunks := []map[string]interface{}{
		{"id": "row", "doc_id": "published", "content_with_weight": "row", "available_int": 1, "table_row_int": 1, "chunk_data": map[string]any{col.DataKey: "100% 中文"}},
		{"id": "disabled", "doc_id": "published", "content_with_weight": "disabled", "available_int": 0, "table_row_int": 1, "chunk_data": map[string]any{col.DataKey: "disabled"}},
		{"id": "other", "doc_id": "other", "content_with_weight": "other", "available_int": 1, "table_row_int": 1, "chunk_data": map[string]any{col.DataKey: "other"}},
		{"id": "prose", "doc_id": "published", "content_with_weight": "prose", "available_int": 1},
	}
	if _, err := e.InsertChunks(ctx, chunks, base, kb); err != nil {
		t.Fatal(err)
	}
	table := buildChunkTableName(base, kb)
	query := fmt.Sprintf("SELECT json_extract_string(chunk_data, '$.%s') AS value FROM %s WHERE doc_id = 'published' AND available_int = 1 AND table_row_int = 1", col.DataKey, table)
	rows, err := e.RunSQL(ctx, table, query, []string{kb}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["value"]) != "100% 中文" {
		t.Fatalf("JSON SQL rows = %#v", rows)
	}
	// The engine's read path must return the marker used by body edits.
	row, err := e.GetChunk(ctx, base, "row", []string{kb})
	if err != nil {
		t.Fatal(err)
	}
	chunk, ok := row.(map[string]interface{})
	if !ok || fmt.Sprint(chunk["table_row_int"]) != "1" {
		t.Fatalf("row marker read back = %#v", row)
	}
	if err := e.UpdateChunks(ctx, map[string]interface{}{"id": "row", "doc_id": "published"}, map[string]interface{}{"table_row_int": 0}, base, kb); err != nil {
		t.Fatal(err)
	}
	rows, err = e.RunSQL(ctx, table, query, []string{kb}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("edited row remains in table SQL: %#v", rows)
	}
}
