//go:build integration

package infinity

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apache/thrift/lib/go/thrift"

	"ragflow/internal/entity"
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
	col := entity.DeriveTableColumns([]string{`金额 "折扣" / %`})[0]
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
	countQuery := fmt.Sprintf("SELECT count(*) AS n FROM %s WHERE doc_id = 'published' AND available_int = 1 AND table_row_int = 1", table)
	counts, err := e.RunSQL(ctx, table, countQuery, []string{kb}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 1 || fmt.Sprint(counts[0]["n"]) != "1" {
		t.Fatalf("scoped COUNT = %#v", counts)
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

func TestTableSQLPreservesValuesAndStatistics(t *testing.T) {
	uri := os.Getenv("RAGFLOW_TEST_INFINITY_URI")
	if uri == "" {
		t.Skip("requires real Infinity")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	e, err := NewEngine(ctx, config.InfinityConfig{URI: uri, PostgresPort: 5432, DBName: "default_db"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	base := fmt.Sprintf("ragflow_sql_values_%d", time.Now().UnixNano())
	defer e.DropChunkStore(context.Background(), base, "test")
	if err := e.CreateChunkStore(ctx, base, "test", 2, "table"); err != nil {
		t.Fatal(err)
	}
	records := []map[string]interface{}{}
	for i, values := range []map[string]interface{}{
		{"amount": "100", "region": "A", "value": "A|B"},
		{"amount": "20", "region": "B", "value": "A\nB"},
		{"amount": "0.5", "region": "B", "value": ""},
		{"region": "B"},
		{"amount": "bad", "region": "bad"},
		{"value": "null", "amount": "null"},
		{"value": nil, "amount": nil},
		{"value": "  中文  "},
		{"amount": ""},
		{"amount": "1e400"},
		{"amount": "NaN"},
		{"amount": "2147483648"},
	} {
		records = append(records, map[string]interface{}{"id": fmt.Sprint(i), "doc_id": "doc", "content_with_weight": "row", "table_row_int": 1, "chunk_data": values})
	}
	if _, err := e.InsertChunks(ctx, records, base, "test"); err != nil {
		t.Fatal(err)
	}
	table := base + "_test"
	for _, tc := range []struct {
		name, query string
		want        []map[string]interface{}
		wantError   bool
	}{
		{"pipe", "SELECT json_extract_string(chunk_data,'$.value') AS value FROM " + table + " WHERE id='0'", []map[string]interface{}{{"value": "A|B"}}, false},
		{"newline", "SELECT json_extract_string(chunk_data,'$.value') AS value FROM " + table + " WHERE id='1'", []map[string]interface{}{{"value": "A\nB"}}, false},
		{"empty", "SELECT json_extract_string(chunk_data,'$.value') AS value FROM " + table + " WHERE id='2'", []map[string]interface{}{{"value": ""}}, false},
		{"missing", "SELECT json_extract_string(chunk_data,'$.amount') AS value FROM " + table + " WHERE id='3'", []map[string]interface{}{{"value": nil}}, false},
		{"literal_null", "SELECT json_extract_string(chunk_data,'$.value') AS value FROM " + table + " WHERE id='5'", []map[string]interface{}{{"value": "null"}}, false},
		{"json_null", "SELECT json_extract_string(chunk_data,'$.value') AS value FROM " + table + " WHERE id='6'", []map[string]interface{}{{"value": nil}}, false},
		{"spaces", "SELECT json_extract_string(chunk_data,'$.value') AS value FROM " + table + " WHERE id='7'", []map[string]interface{}{{"value": "  中文  "}}, false},
		{"null_filter", "SELECT COUNT(*) AS n FROM " + table + " WHERE id IN ('3','5','6') AND json_extract_isnull(chunk_data,'$.amount') = true", []map[string]interface{}{{"n": "2"}}, false},
		{"qualified_source", "SELECT " + table + ".doc_id,json_extract_string(chunk_data,'$.amount') AS value FROM " + table + " WHERE id='0' ORDER BY CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE)", []map[string]interface{}{{"doc_id": "doc", "value": "100"}}, false},
		{"group", "SELECT json_extract_string(chunk_data,'$.region') AS region,SUM(CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE)) AS total FROM " + table + " WHERE id IN ('0','1','2','3') GROUP BY json_extract_string(chunk_data,'$.region') ORDER BY json_extract_string(chunk_data,'$.region')", []map[string]interface{}{{"region": "A", "total": "100.000000"}, {"region": "B", "total": "20.500000"}}, false},
		{"group_count", "SELECT json_extract_string(chunk_data,'$.region') AS region,COUNT(*) AS n FROM " + table + " WHERE id IN ('0','1','2','3') GROUP BY json_extract_string(chunk_data,'$.region') ORDER BY json_extract_string(chunk_data,'$.region')", []map[string]interface{}{{"region": "A", "n": "1"}, {"region": "B", "n": "3"}}, false},
		{"case_order", "SELECT CASE WHEN id='0' THEN json_extract_string(chunk_data,'$.amount') ELSE 'other' END AS label FROM " + table + " WHERE id IN ('0','1') ORDER BY CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE)", nil, true},
		{"numeric_order", "SELECT json_extract_string(chunk_data,'$.amount') AS value FROM " + table + " WHERE id IN ('0','1','2') ORDER BY CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE)", []map[string]interface{}{{"value": "0.5"}, {"value": "20"}, {"value": "100"}}, false},
		{"invalid", "SELECT SUM(CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE)) AS total FROM " + table + " WHERE id='4'", nil, true},
		{"invalid_literal_null", "SELECT CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE) AS value FROM " + table + " WHERE id='5'", nil, true},
		{"invalid_empty", "SELECT CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE) AS value FROM " + table + " WHERE id='8'", nil, true},
		{"invalid_overflow", "SELECT CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE) AS value FROM " + table + " WHERE id='9'", nil, true},
		{"invalid_nan", "SELECT CAST(json_extract_string(chunk_data,'$.amount') AS DOUBLE) AS value FROM " + table + " WHERE id='10'", nil, true},
		{"invalid_integer_overflow", "SELECT CAST(json_extract_string(chunk_data,'$.amount') AS INTEGER) AS value FROM " + table + " WHERE id='11'", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := e.RunSQL(ctx, table, tc.query, []string{"test"}, "")
			if tc.wantError {
				if err == nil {
					t.Fatalf("invalid number accepted: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

// Reject one AddColumns RPC while forwarding the other requests to real Infinity.
// Keeping the connection usable exposes accidental Delete/Insert after DDL failure.
func TestTableColumnFailureStopsInsert(t *testing.T) {
	uri := os.Getenv("RAGFLOW_TEST_INFINITY_URI")
	if uri == "" {
		t.Skip("RAGFLOW_TEST_INFINITY_URI is not set")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	e, err := NewEngine(ctx, config.InfinityConfig{URI: uri, DBName: "default_db"})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, tc := range []struct {
		name   string
		failAt int
		want   string
	}{
		{"chunk_data", 1, "failed to add chunk_data column"},
		{"table_row_int", 2, "failed to add table row marker columns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := fmt.Sprintf("ragflow_ddl_test_%d", time.Now().UnixNano())
			const kb = "table"
			if err := e.CreateChunkStore(ctx, base, kb, 2, "naive"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := e.DropChunkStore(ctx, base, kb); err != nil {
					t.Error(err)
				}
			}()
			address, requests := rejectTableColumnRPC(t, uri, tc.failAt)
			client, err := NewInfinityClient(config.InfinityConfig{URI: address, DBName: "default_db"})
			if err != nil {
				t.Fatal(err)
			}
			writer := &Engine{client: client}
			defer writer.Close()
			_, err = writer.InsertChunks(ctx, []map[string]interface{}{{
				"id": "row", "doc_id": "published", "content_with_weight": "row",
				"table_row_int": 1, "chunk_data": map[string]any{"amount": "100"},
			}}, base, kb)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("InsertChunks error = %v, want %q", err, tc.want)
			}
			for _, method := range requests() {
				if method == "Delete" || method == "Insert" {
					t.Errorf("%s executed after failed column addition", method)
				}
			}
			before := len(requests())
			if _, err := writer.InsertChunks(ctx, []map[string]interface{}{{
				"id": "prose", "doc_id": "published", "content_with_weight": "prose",
			}}, base, kb); err != nil {
				t.Fatalf("ordinary batch requires no table columns: %v", err)
			}
			for _, method := range requests()[before:] {
				if method == "AddColumns" {
					t.Error("ordinary batch tried to add table columns")
				}
			}
		})
	}
}

func rejectTableColumnRPC(t *testing.T, upstream string, failAt int) (string, func() []string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var methods []string
	done := make(chan struct{})
	t.Cleanup(func() { listener.Close(); <-done })
	go func() {
		defer close(done)
		downstream, err := listener.Accept()
		if err != nil {
			return
		}
		defer downstream.Close()
		remote, err := net.DialTimeout("tcp", upstream, 5*time.Second)
		if err != nil {
			t.Errorf("proxy upstream: %v", err)
			return
		}
		defer remote.Close()
		downstream.SetDeadline(time.Now().Add(time.Minute))
		remote.SetDeadline(time.Now().Add(time.Minute))
		ctx := context.Background()
		additions := 0
		for {
			request, method, seq, err := readTableRPC(ctx, downstream)
			if err != nil {
				return
			}
			mu.Lock()
			methods = append(methods, method)
			mu.Unlock()
			if method == "AddColumns" {
				additions++
			}
			if method == "AddColumns" && additions == failAt {
				transport := thrift.NewStreamTransportW(downstream)
				protocol := thrift.NewTBinaryProtocolTransport(transport)
				if err := protocol.WriteMessageBegin(ctx, method, thrift.EXCEPTION, seq); err != nil {
					t.Error(err)
					return
				}
				if err := thrift.NewTApplicationException(thrift.INTERNAL_ERROR, "injected column failure").Write(ctx, protocol); err != nil {
					t.Error(err)
					return
				}
				if err := protocol.WriteMessageEnd(ctx); err != nil {
					t.Error(err)
					return
				}
				if err := protocol.Flush(ctx); err != nil {
					t.Error(err)
					return
				}
				continue
			}
			if _, err := remote.Write(request); err != nil {
				t.Error(err)
				return
			}
			response, _, _, err := readTableRPC(ctx, remote)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := downstream.Write(response); err != nil {
				return
			}
		}
	}()
	return listener.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), methods...)
	}
}

func readTableRPC(ctx context.Context, reader io.Reader) ([]byte, string, int32, error) {
	var message bytes.Buffer
	// No buffered read-ahead: capture exactly one Thrift message for forwarding.
	transport := &thrift.StreamTransport{Reader: io.TeeReader(reader, &message)}
	protocol := thrift.NewTBinaryProtocolTransport(transport)
	name, _, seq, err := protocol.ReadMessageBegin(ctx)
	if err != nil {
		return nil, "", 0, err
	}
	if err := protocol.Skip(ctx, thrift.STRUCT); err != nil {
		return nil, "", 0, err
	}
	if err := protocol.ReadMessageEnd(ctx); err != nil {
		return nil, "", 0, err
	}
	return message.Bytes(), name, seq, nil
}
