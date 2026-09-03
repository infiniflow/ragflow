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
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	gcpbigquery "cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

type fakeBigQueryClient struct {
	queries []*fakeBigQueryQuery
	rows    []map[string]any
	schema  []bigQueryField
	closed  bool
}

func (f *fakeBigQueryClient) Query(query string) bigQueryQuery {
	q := &fakeBigQueryQuery{
		text: query,
		readIterator: &fakeBigQueryRowIterator{
			rows:   f.rows,
			schema: f.schema,
		},
	}
	if strings.Contains(query, "FROM (") {
		q.runJob = &fakeBigQueryJob{status: &fakeBigQueryJobStatus{done: true, schema: f.schema}}
	}
	f.queries = append(f.queries, q)
	return q
}

func (f *fakeBigQueryClient) Close() error {
	f.closed = true
	return nil
}

type fakeBigQueryQuery struct {
	text            string
	dryRun          bool
	disableCache    bool
	maxBytes        int64
	jobTimeout      time.Duration
	parameters      []gcpbigquery.QueryParameter
	runJob          *fakeBigQueryJob
	readIterator    *fakeBigQueryRowIterator
	runErr, readErr error
}

func (f *fakeBigQueryQuery) Run(context.Context) (bigQueryJob, error) {
	if f.runErr != nil {
		return nil, f.runErr
	}
	if f.runJob == nil {
		return &fakeBigQueryJob{status: &fakeBigQueryJobStatus{done: true}}, nil
	}
	return f.runJob, nil
}

func (f *fakeBigQueryQuery) Read(context.Context) (bigQueryRowIterator, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	if f.readIterator == nil {
		return &fakeBigQueryRowIterator{}, nil
	}
	return f.readIterator, nil
}

func (f *fakeBigQueryQuery) SetDryRun(v bool)              { f.dryRun = v }
func (f *fakeBigQueryQuery) SetDisableQueryCache(v bool)   { f.disableCache = v }
func (f *fakeBigQueryQuery) SetMaxBytesBilled(v int64)     { f.maxBytes = v }
func (f *fakeBigQueryQuery) SetJobTimeout(v time.Duration) { f.jobTimeout = v }
func (f *fakeBigQueryQuery) SetParameters(v []gcpbigquery.QueryParameter) {
	f.parameters = append([]gcpbigquery.QueryParameter(nil), v...)
}

type fakeBigQueryJob struct {
	status *fakeBigQueryJobStatus
}

func (f *fakeBigQueryJob) Status(context.Context) (bigQueryJobStatus, error) {
	if f.status == nil {
		return &fakeBigQueryJobStatus{done: true}, nil
	}
	return f.status, nil
}

func (f *fakeBigQueryJob) Wait(context.Context) (bigQueryJobStatus, error) {
	return f.Status(context.Background())
}

func (f *fakeBigQueryJob) Read(context.Context) (bigQueryRowIterator, error) {
	return &fakeBigQueryRowIterator{}, nil
}

type fakeBigQueryJobStatus struct {
	done   bool
	err    error
	schema []bigQueryField
	bytes  int64
}

func (f *fakeBigQueryJobStatus) Done() bool { return f.done }
func (f *fakeBigQueryJobStatus) Err() error { return f.err }
func (f *fakeBigQueryJobStatus) Statistics() bigQueryJobStatistics {
	return &fakeBigQueryJobStatistics{schema: f.schema, bytes: f.bytes}
}

type fakeBigQueryJobStatistics struct {
	schema []bigQueryField
	bytes  int64
}

func (f *fakeBigQueryJobStatistics) QuerySchema() []bigQueryField { return f.schema }
func (f *fakeBigQueryJobStatistics) TotalBytesProcessed() int64   { return f.bytes }

type fakeBigQueryRowIterator struct {
	rows   []map[string]any
	schema []bigQueryField
	index  int
	page   int
}

func (f *fakeBigQueryRowIterator) Next(dst any) error {
	if f.index >= len(f.rows) {
		return io.EOF
	}
	row := f.rows[f.index]
	f.index++
	target, ok := dst.(*map[string]any)
	if !ok {
		return errors.New("fake iterator expects *map[string]any")
	}
	*target = row
	return nil
}

func (f *fakeBigQueryRowIterator) Schema() []bigQueryField { return f.schema }

func (f *fakeBigQueryRowIterator) SetPageSize(size int) { f.page = size }

func newFakeBigQueryConnector(t *testing.T, config map[string]any, fake *fakeBigQueryClient) *BigQueryConnector {
	t.Helper()
	if config == nil {
		config = map[string]any{
			"project_id":      "my-proj",
			"dataset_id":      "ds",
			"table_id":        "tbl",
			"content_columns": "name,description",
			"id_column":       "id",
			"credentials": map[string]any{
				"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
			},
		}
	}
	connector, err := NewBigQueryConnector(config)
	if err != nil {
		t.Fatalf("NewBigQueryConnector failed: %v", err)
	}
	connector.newClient = func(context.Context) (bigQueryClient, error) {
		return fake, nil
	}
	return connector
}

func bigQuerySchema() []bigQueryField {
	return []bigQueryField{
		{Name: "id", Type: "INT64"},
		{Name: "name", Type: "STRING"},
		{Name: "description", Type: "STRING"},
		{Name: "category", Type: "STRING"},
		{Name: "updated_at", Type: "TIMESTAMP"},
	}
}

func TestBigQueryConnectorValidate(t *testing.T) {
	fake := &fakeBigQueryClient{schema: bigQuerySchema()}
	connector := newFakeBigQueryConnector(t, nil, fake)
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if len(fake.queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(fake.queries))
	}
	if fake.queries[0].text != "SELECT 1" {
		t.Fatalf("connectivity query = %q", fake.queries[0].text)
	}
	if !fake.queries[1].dryRun || !fake.queries[1].disableCache {
		t.Fatalf("dry-run flags = dry:%v cache:%v", fake.queries[1].dryRun, fake.queries[1].disableCache)
	}
}

func TestBigQueryConnectorValidateMissingColumn(t *testing.T) {
	fake := &fakeBigQueryClient{schema: []bigQueryField{
		{Name: "id", Type: "INT64"},
		{Name: "name", Type: "STRING"},
	}}
	connector := newFakeBigQueryConnector(t, nil, fake)
	err := connector.Validate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("Validate error = %v, want missing description", err)
	}
}

func TestBigQueryConnectorOpenSyncFull(t *testing.T) {
	updatedAt := mustTime(t, "2026-01-02T03:04:05Z")
	fake := &fakeBigQueryClient{schema: bigQuerySchema(), rows: []map[string]any{
		{"id": int64(7), "name": "Hello/World", "description": "Some body", "category": "news", "updated_at": updatedAt},
	}}
	connector := newFakeBigQueryConnector(t, map[string]any{
		"project_id":       "my-proj",
		"dataset_id":       "ds",
		"table_id":         "tbl",
		"content_columns":  "name,description",
		"metadata_columns": "id,category,updated_at",
		"id_column":        "id",
		"timestamp_column": "updated_at",
		"credentials": map[string]any{
			"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
		},
	}, fake)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "bigquery:my-proj:ds.tbl:7" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Hello/World" {
		t.Fatalf("semantic id = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	blob := string(doc.Blob)
	if !strings.Contains(blob, "【name】:\nHello/World") || !strings.Contains(blob, "【description】:\nSome body") {
		t.Fatalf("blob = %q", blob)
	}
	if fake.queries[0].readIterator.page != defaultBigQueryPageSize {
		t.Fatalf("page size = %d, want %d", fake.queries[0].readIterator.page, defaultBigQueryPageSize)
	}
	if !strings.Contains(fake.queries[0].text, "ORDER BY ragflow_src.id ASC, ragflow_src.updated_at ASC") {
		t.Fatalf("sync query = %q, want stable ordering", fake.queries[0].text)
	}
	if !doc.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Metadata["category"] != "news" || doc.Metadata["id"] != "7" {
		t.Fatalf("metadata = %v", doc.Metadata)
	}
	if doc.Metadata["updated_at"] != updatedAt.Format(time.RFC3339) {
		t.Fatalf("metadata updated_at = %v", doc.Metadata["updated_at"])
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestBigQueryConnectorIncrementalParameters(t *testing.T) {
	fake := &fakeBigQueryClient{schema: []bigQueryField{{Name: "name", Type: "STRING"}}, rows: []map[string]any{{"name": "content"}}}
	connector := newFakeBigQueryConnector(t, map[string]any{
		"project_id":       "my-proj",
		"dataset_id":       "ds",
		"table_id":         "tbl",
		"content_columns":  "name",
		"timestamp_column": "updated_at",
		"credentials": map[string]any{
			"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
		},
	}, fake)
	start := mustTime(t, "2026-01-01T00:00:00Z")
	end := mustTime(t, "2026-01-02T00:00:00Z")
	if _, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
	}); err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	if len(fake.queries) != 1 {
		t.Fatalf("query count = %d", len(fake.queries))
	}
	query := fake.queries[0]
	if !strings.Contains(query.text, "ragflow_src.updated_at >= @start_cursor") ||
		!strings.Contains(query.text, "ragflow_src.updated_at <= @end_cursor") {
		t.Fatalf("query = %q", query.text)
	}
	if len(query.parameters) != 2 || query.parameters[0].Name != "start_cursor" || query.parameters[1].Name != "end_cursor" {
		t.Fatalf("parameters = %+v", query.parameters)
	}
	if !strings.Contains(query.text, "ORDER BY ragflow_src.updated_at ASC") {
		t.Fatalf("incremental query = %q, want stable ordering", query.text)
	}
}

func TestBigQueryConnectorMD5FallbackID(t *testing.T) {
	fake := &fakeBigQueryClient{
		schema: []bigQueryField{{Name: "name", Type: "STRING"}},
		rows:   []map[string]any{{"name": "content"}},
	}
	connector := newFakeBigQueryConnector(t, map[string]any{
		"project_id":      "my-proj",
		"dataset_id":      "ds",
		"table_id":        "tbl",
		"query":           "SELECT * FROM `my-proj.ds.tbl`",
		"content_columns": "name",
		"credentials": map[string]any{
			"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
		},
	}, fake)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	sum := md5.Sum([]byte("【name】:\ncontent"))
	wantPrefix := "bigquery:my-proj:query:" + hex.EncodeToString(sum[:])
	if batch.Documents[0].SourceID != wantPrefix {
		t.Fatalf("source id = %q, want %q", batch.Documents[0].SourceID, wantPrefix)
	}
}

func TestBigQueryConnectorResume(t *testing.T) {
	fake := &fakeBigQueryClient{schema: bigQuerySchema(), rows: []map[string]any{
		{"id": int64(1), "name": "one"},
		{"id": int64(2), "name": "two"},
		{"id": int64(3), "name": "three"},
	}}
	connector := newFakeBigQueryConnector(t, nil, fake)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{SourceID: "bigquery:my-proj:ds.tbl:2"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "bigquery:my-proj:ds.tbl:3" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestBigQueryConnectorResumeMissingAnchor(t *testing.T) {
	fake := &fakeBigQueryClient{schema: bigQuerySchema(), rows: []map[string]any{{"id": int64(1), "name": "one"}}}
	connector := newFakeBigQueryConnector(t, nil, fake)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{SourceID: "bigquery:my-proj:ds.tbl:9"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch error = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestBigQueryConnectorResumeCursorOnly(t *testing.T) {
	fake := &fakeBigQueryClient{schema: bigQuerySchema(), rows: []map[string]any{
		{"id": int64(2), "name": "two"},
		{"id": int64(3), "name": "three"},
	}}
	connector := newFakeBigQueryConnector(t, nil, fake)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: "bigquery:my-proj:ds.tbl:2"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "bigquery:my-proj:ds.tbl:3" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

func TestBigQueryConnectorResumeRequiresStableOrder(t *testing.T) {
	fake := &fakeBigQueryClient{
		schema: []bigQueryField{{Name: "name", Type: "STRING"}},
		rows:   []map[string]any{{"name": "content"}},
	}
	connector := newFakeBigQueryConnector(t, map[string]any{
		"project_id":      "my-proj",
		"dataset_id":      "ds",
		"table_id":        "tbl",
		"content_columns": "name",
		"credentials": map[string]any{
			"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
		},
	}, fake)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{SourceID: "bigquery:my-proj:ds.tbl:anchor"},
	})
	if err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("OpenSync error = %v, want ErrSyncResumeInvalid", err)
	}
	if session != nil {
		t.Fatal("OpenSync returned a session after resume validation failure")
	}
	if strings.Contains(fake.queries[0].text, "ORDER BY") {
		t.Fatalf("query = %q, want no synthetic ordering without stable columns", fake.queries[0].text)
	}
}

func TestBigQueryConnectorOpenPrune(t *testing.T) {
	fake := &fakeBigQueryClient{
		schema: []bigQueryField{{Name: "id", Type: "INT64"}},
		rows:   []map[string]any{{"id": int64(3)}, {"id": int64(4)}},
	}
	connector := newFakeBigQueryConnector(t, map[string]any{
		"project_id":      "my-proj",
		"dataset_id":      "ds",
		"table_id":        "tbl",
		"query":           "SELECT * FROM `my-proj.ds.tbl`",
		"content_columns": "name",
		"id_column":       "id",
		"credentials": map[string]any{
			"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
		},
	}, fake)
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 ||
		batch.Documents[0].SourceID != "bigquery:my-proj:query:3" ||
		batch.Documents[1].SourceID != "bigquery:my-proj:query:4" {
		t.Fatalf("slim documents = %+v", batch.Documents)
	}
	if !strings.Contains(fake.queries[0].text, "ORDER BY ragflow_src.id ASC") {
		t.Fatalf("prune query = %q, want stable ordering", fake.queries[0].text)
	}
}

func TestBigQueryConnectorRegistry(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("bigquery", map[string]any{
		"project_id":      "my-proj",
		"dataset_id":      "ds",
		"table_id":        "tbl",
		"content_columns": "name",
		"credentials": map[string]any{
			"service_account_json": `{"type":"service_account","project_id":"my-proj"}`,
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if connector == nil {
		t.Fatal("connector is nil")
	}
}

func TestBigQueryConnectorInvalidProjectID(t *testing.T) {
	_, err := NewBigQueryConnector(map[string]any{
		"project_id":      "bad project",
		"content_columns": "name",
	})
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("NewBigQueryConnector error = %v, want project_id validation", err)
	}
}

func TestBigQueryCursorSerialization(t *testing.T) {
	now := mustTime(t, "2026-01-02T03:04:05Z")
	serialized := serializeBigQueryCursor(now)
	if _, ok := serialized.(map[string]any); !ok {
		t.Fatalf("serialized cursor = %#v", serialized)
	}
	restored := deserializeBigQueryCursor(serialized)
	if !restored.(time.Time).Equal(now) {
		t.Fatalf("restored cursor = %v", restored)
	}
	if got := serializeBigQueryCursor(int64(42)); got != int64(42) {
		t.Fatalf("numeric cursor = %#v", got)
	}
	if got := deserializeBigQueryCursor("plain"); got != "plain" {
		t.Fatalf("plain cursor = %#v", got)
	}
	clock, err := civil.ParseTime("03:04:05")
	if err != nil {
		t.Fatalf("parse time cursor: %v", err)
	}
	restoredTime := deserializeBigQueryCursor(serializeBigQueryCursor(clock))
	if restoredTime != clock {
		t.Fatalf("restored time cursor = %#v", restoredTime)
	}
}
