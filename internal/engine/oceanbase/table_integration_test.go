//go:build integration

package oceanbase

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/server/config"
)

func TestTableColumnSQLRoundTrip(t *testing.T) {
	host := os.Getenv("RAGFLOW_TEST_OCEANBASE_HOST")
	if host == "" {
		t.Skip("RAGFLOW_TEST_OCEANBASE_HOST is not set")
	}
	port, err := strconv.Atoi(envOr("RAGFLOW_TEST_OCEANBASE_PORT", "2881"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewEngine(envOr("RAGFLOW_TEST_OCEANBASE_ENGINE_TYPE", "oceanbase"), config.OceanBaseConnectionConfig{
		Host: host, Port: port, DBName: envOr("RAGFLOW_TEST_OCEANBASE_DBNAME", "test"), User: envOr("RAGFLOW_TEST_OCEANBASE_USER", "root@test"), Password: os.Getenv("RAGFLOW_TEST_OCEANBASE_PASSWORD"), MaxConnections: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	var version string
	if err := e.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s version: %s", e.GetType(), version)
	table := fmt.Sprintf("ragflow_column_test_%d", time.Now().UnixNano())
	kb := "table"
	defer cleanupChunkStore(t, e, table, "")
	if err := e.CreateChunkStore(ctx, table, kb, 2, "table"); err != nil {
		t.Fatal(err)
	}
	col := common.DeriveTableColumns([]string{`金额 "折扣" / %`})[0]
	chunks := []map[string]interface{}{
		{"id": "row", "doc_id": "published", "content_with_weight": "row", "available_int": 1, "table_row_int": 1, "chunk_data": map[string]any{col.DataKey: "100% 中文"}},
		{"id": "disabled", "doc_id": "published", "content_with_weight": "disabled", "available_int": 0, "table_row_int": 1, "chunk_data": map[string]any{col.DataKey: "disabled"}},
		{"id": "other", "doc_id": "published", "content_with_weight": "other", "available_int": 1, "table_row_int": 1, "chunk_data": map[string]any{col.DataKey: "other"}, "kb_id": "other"},
		{"id": "prose", "doc_id": "published", "content_with_weight": "prose", "available_int": 1},
	}
	if _, err := e.InsertChunks(ctx, chunks[:2], table, kb); err != nil {
		t.Fatal(err)
	}
	if _, err := e.InsertChunks(ctx, chunks[2:3], table, "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.InsertChunks(ctx, chunks[3:], table, kb); err != nil {
		t.Fatal(err)
	}
	query := fmt.Sprintf("SELECT json_extract_string(chunk_data, '$.%s') AS value FROM %s WHERE kb_id = 'table' AND doc_id = 'published' AND available_int = 1 AND table_row_int = 1", col.DataKey, table)
	rows, err := e.RunSQL(ctx, table, query, []string{kb}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["value"]) != "100% 中文" {
		t.Fatalf("JSON SQL rows = %#v", rows)
	}
	if err := e.UpdateChunks(ctx, map[string]interface{}{"id": "row", "doc_id": "published"}, map[string]interface{}{"table_row_int": 0}, table, kb); err != nil {
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
