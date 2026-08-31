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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewAirtableConnectorParsesConfig(t *testing.T) {
	connector, err := NewAirtableConnector(map[string]any{
		"credentials": map[string]any{
			"airtable_access_token": "token",
		},
		"base_id":             "base 1",
		"table_name_or_id":    "My Table",
		"last_modified_field": "Modified",
		"batch_size":          "0",
		"sync_batch_size":     "3",
		"size_threshold":      "123",
	})
	if err != nil {
		t.Fatalf("NewAirtableConnector failed: %v", err)
	}
	if connector.baseID != "base 1" || connector.tableNameOrID != "My Table" || connector.accessToken != "token" {
		t.Fatalf("config = %q/%q/%q", connector.baseID, connector.tableNameOrID, connector.accessToken)
	}
	if connector.batchSize != 3 || connector.sizeThreshold != 123 {
		t.Fatalf("batch/threshold = %d/%d, want 3/123", connector.batchSize, connector.sizeThreshold)
	}
	if connector.lastModified != "Modified" {
		t.Fatalf("last modified field = %q", connector.lastModified)
	}
}

func TestNewAirtableConnectorDefaults(t *testing.T) {
	connector, err := NewAirtableConnector(nil)
	if err != nil {
		t.Fatalf("NewAirtableConnector failed: %v", err)
	}
	if connector.batchSize != airtableDefaultBatchSize || connector.sizeThreshold != airtableDefaultSizeThreshold {
		t.Fatalf("defaults = batch %d threshold %d", connector.batchSize, connector.sizeThreshold)
	}
}

func TestAirtableValidateRejectsMissingFields(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.accessToken = ""
	err := connector.Validate(context.Background())
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("Validate error = %v, want ConnectorMissingCredentialError", err)
	}

	connector = newAirtableTestConnector()
	connector.baseID = ""
	if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "base_id") {
		t.Fatalf("missing base error = %v", err)
	}

	connector = newAirtableTestConnector()
	connector.batchSize = 0
	if err := connector.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "batch_size") {
		t.Fatalf("batch error = %v", err)
	}
}

func TestAirtableValidateProbesTable(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		if !strings.Contains(pageURL, "pageSize=1") || !strings.Contains(pageURL, "/base%201/My%20Table") {
			t.Fatalf("validation URL = %q", pageURL)
		}
		return airtableRecordPage{}, nil
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestAirtableValidateClassifiesAPIErrors(t *testing.T) {
	tests := []struct {
		status int
		want   any
	}{
		{http.StatusUnauthorized, &ConnectorMissingCredentialError{}},
		{http.StatusForbidden, &ConnectorValidationError{}},
		{http.StatusNotFound, &ConnectorValidationError{}},
		{http.StatusBadGateway, &ConnectorValidationError{}},
	}
	for _, test := range tests {
		connector := newAirtableTestConnector()
		connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
			return airtableRecordPage{}, &airtableAPIError{status: test.status, body: "api error"}
		}
		err := connector.Validate(context.Background())
		switch test.want.(type) {
		case *ConnectorMissingCredentialError:
			var missing *ConnectorMissingCredentialError
			if !errors.As(err, &missing) {
				t.Fatalf("status %d error = %v, want ConnectorMissingCredentialError", test.status, err)
			}
		case *ConnectorValidationError:
			var validation *ConnectorValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("status %d error = %v, want ConnectorValidationError", test.status, err)
			}
		}
	}
}

func TestAirtableValidateConnectorSettingUsesCandidateConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/base/table" || r.URL.Query().Get("pageSize") != "1" {
			t.Errorf("unexpected validation URL %s", r.URL.String())
			return
		}
		if r.Header.Get("Authorization") != "Bearer request-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			return
		}
		w.Write([]byte(`{"records":[]}`))
	}))
	defer server.Close()

	receiver := &AirtableConnector{
		apiBaseURL: server.URL + "/v0",
		httpClient: server.Client(),
	}
	err := receiver.ValidateConnectorSetting(context.Background(), map[string]any{
		"base_id":          "base",
		"table_name_or_id": "table",
		"credentials": map[string]any{
			"airtable_access_token": "request-token",
		},
	})
	if err != nil {
		t.Fatalf("ValidateConnectorSetting failed: %v", err)
	}
}

func TestAirtableOpenSyncUsesAttachmentsAndFetch(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.batchSize = 3
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		return airtableRecordPage{Records: []airtableRecord{airtableTestRecord()}}, nil
	}
	var fetchedURL string
	connector.downloadFile = func(ctx context.Context, rawURL string) ([]byte, error) {
		fetchedURL = rawURL
		if rawURL != "https://example.test/a.pdf" {
			t.Fatalf("fetch URL = %q", rawURL)
		}
		return []byte("pdf-body"), nil
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 3 {
		t.Fatalf("documents len = %d, want 3", len(batch.Documents))
	}
	recordDoc := batch.Documents[0]
	if recordDoc.SourceID != "airtable:rec-1" || recordDoc.Extension != ".json" || len(recordDoc.Blob) == 0 {
		t.Fatalf("record document = %+v", recordDoc)
	}
	doc := batch.Documents[1]
	if doc.SourceID != "airtable:rec-1:att-1" || doc.SemanticIdentifier != "report.PDF" || doc.Extension != ".pdf" {
		t.Fatalf("document shape = %+v", doc)
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-02T03:04:05Z")) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Fingerprint == "" || doc.FetchRef == nil {
		t.Fatalf("fingerprint/fetchref = %q/%+v", doc.Fingerprint, doc.FetchRef)
	}
	if doc.Metadata["record_id"] != "rec-1" || doc.Metadata["attachment_id"] != "att-1" || doc.Metadata["field_name"] != "Attachments" {
		t.Fatalf("metadata = %+v", doc.Metadata)
	}

	fetcher, ok := session.(Fetcher)
	if !ok {
		t.Fatalf("session does not implement Fetcher")
	}
	blob, err := fetcher.Fetch(context.Background(), *batch.Documents[1].FetchRef)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "pdf-body" || fetchedURL == "" {
		t.Fatalf("fetch blob = %q, url = %q", blob, fetchedURL)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestAirtableOpenSyncWindowFilter(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.batchSize = 10
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		return airtableRecordPage{Records: []airtableRecord{
			airtableTestRecordWithIDTime("before", "2026-01-01T00:00:00Z", "before.pdf"),
			airtableTestRecordWithIDTime("inside", "2026-01-03T00:00:00Z", "inside.pdf"),
			airtableTestRecordWithIDTime("after", "2026-01-05T00:00:00Z", "after.pdf"),
		}}, nil
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: airtableMustTimePointer(t, "2026-01-02T00:00:00Z"),
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2", len(batch.Documents))
	}
	got := map[string]bool{}
	for _, doc := range batch.Documents {
		got[doc.SourceID] = true
	}
	if !got["airtable:inside"] || !got["airtable:inside:att-1"] {
		t.Fatalf("documents = %+v, want record and attachment", got)
	}
}

func TestAirtableOpenSyncWindowUsesLastModifiedField(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.lastModified = "Modified"
	connector.batchSize = 10
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		inside := airtableTestRecordWithIDTime("inside", "2026-01-01T00:00:00Z", "inside.pdf")
		inside.Fields["Modified"] = "2026-01-03T00:00:00Z"
		after := airtableTestRecordWithIDTime("after", "2026-01-01T00:00:00Z", "after.pdf")
		after.Fields["Modified"] = "2026-01-05T00:00:00Z"
		fallback := airtableTestRecordWithIDTime("fallback", "2026-01-01T00:00:00Z", "fallback.pdf")
		return airtableRecordPage{Records: []airtableRecord{inside, after, fallback}}, nil
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: airtableMustTimePointer(t, "2026-01-02T00:00:00Z"),
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "airtable:inside" {
		t.Fatalf("documents = %#v, want inside record only", batch.Documents)
	}
	if !batch.Documents[0].UpdatedAt.Equal(mustTime(t, "2026-01-03T00:00:00Z")) {
		t.Fatalf("record updated at = %s, want 2026-01-03T00:00:00Z", batch.Documents[0].UpdatedAt)
	}
}

func TestAirtableRecordDocumentFallsBackToCreatedTime(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.lastModified = "Modified"
	withField := airtableRecord{
		ID:          "rec-field",
		CreatedTime: "2026-01-01T00:00:00Z",
		Fields: map[string]any{
			"Modified": "2026-01-03T00:00:00Z",
		},
	}
	doc, ok := connector.recordDocument(withField)
	if !ok || !doc.UpdatedAt.Equal(mustTime(t, "2026-01-03T00:00:00Z")) {
		t.Fatalf("recordDocument with field = ok %v, updated %s", ok, doc.UpdatedAt)
	}
	if doc.Metadata["last_modified"] != "2026-01-03T00:00:00Z" {
		t.Fatalf("last_modified metadata = %v", doc.Metadata["last_modified"])
	}

	withoutField := airtableRecord{
		ID:          "rec-created",
		CreatedTime: "2026-01-02T00:00:00Z",
		Fields:      map[string]any{},
	}
	doc, ok = connector.recordDocument(withoutField)
	if !ok || !doc.UpdatedAt.Equal(mustTime(t, "2026-01-02T00:00:00Z")) {
		t.Fatalf("recordDocument fallback = ok %v, updated %s", ok, doc.UpdatedAt)
	}
}

func TestAirtableOpenSyncFingerprintFilter(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.batchSize = 10
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		return airtableRecordPage{Records: []airtableRecord{
			airtableTestRecordWithIDTime("changed", "2026-01-03T00:00:00Z", "changed.pdf"),
			airtableTestRecordWithIDTime("same", "2026-01-03T00:00:00Z", "same.pdf"),
			airtableTestRecordWithIDTime("missing", "2026-01-03T00:00:00Z", "missing.pdf"),
		}}, nil
	}
	request := SyncRequest{
		WindowStart: airtableMustTimePointer(t, "2026-01-02T00:00:00Z"),
		WindowEnd:   mustTime(t, "2026-01-04T00:00:00Z"),
		Fingerprints: map[string]string{
			"airtable:changed:att-1": "old",
			"airtable:same:att-1":    airtableAttachmentFingerprint("same", "Attachments", airtableTestAttachment("same.pdf")),
		},
	}
	session, err := connector.OpenSync(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	if len(got) != 5 {
		t.Fatalf("documents = %v, want 5", got)
	}
	want := map[string]bool{
		"airtable:changed":       true,
		"airtable:same":          true,
		"airtable:missing":       true,
		"airtable:changed:att-1": true,
		"airtable:missing:att-1": true,
	}
	for _, sourceID := range got {
		delete(want, sourceID)
	}
	if len(want) != 0 {
		t.Fatalf("documents missing %v; got %v", want, got)
	}
}

func TestAirtableOpenSyncResumeWithinPage(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.batchSize = 4
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		return airtableRecordPage{Records: []airtableRecord{
			airtableTestRecordWithIDTime("rec-1", "2026-01-01T00:00:00Z", "a.pdf"),
			airtableTestRecordWithIDTime("rec-2", "2026-01-02T00:00:00Z", "b.pdf"),
			airtableTestRecordWithIDTime("rec-3", "2026-01-03T00:00:00Z", "c.pdf"),
		}}, nil
	}
	first, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("first OpenSync failed: %v", err)
	}
	batch, err := first.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 4 || batch.Checkpoint == nil || batch.Checkpoint.SourceID != "airtable:rec-2:att-1" {
		t.Fatalf("first batch = %+v", batch)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: batch.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 2 {
		t.Fatalf("resume documents = %+v, want 2", second.Documents)
	}
	got := map[string]bool{}
	for _, doc := range second.Documents {
		got[doc.SourceID] = true
	}
	if !got["airtable:rec-3"] || !got["airtable:rec-3:att-1"] {
		t.Fatalf("resume documents = %+v, want rec-3 record and attachment", got)
	}
}

func TestAirtableOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newAirtableTestConnector()
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestAirtableOpenSyncResumeRejectsMissingAnchor(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		return airtableRecordPage{Records: []airtableRecord{
			airtableTestRecordWithIDTime("rec-1", "2026-01-01T00:00:00Z", "a.pdf"),
			airtableTestRecordWithIDTime("rec-3", "2026-01-03T00:00:00Z", "c.pdf"),
		}}, nil
	}
	checkpoint := &SyncCheckpoint{
		Cursor: `{"page_url":"` + connector.recordsURL("", airtablePageSize) + `","source_id":"airtable:rec-2:att-1"}`,
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestAirtableOpenPrune(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.batchSize = 1
	connector.listRecords = func(ctx context.Context, pageURL string) (airtableRecordPage, error) {
		if strings.Contains(pageURL, "offset=next") {
			return airtableRecordPage{Records: []airtableRecord{
				airtableTestRecordWithIDTime("rec-2", "2026-01-02T00:00:00Z", "b.pdf"),
			}}, nil
		}
		return airtableRecordPage{
			Records: []airtableRecord{
				airtableTestRecordWithIDTime("rec-1", "2026-01-01T00:00:00Z", "a.pdf"),
				{ID: "rec-invalid", CreatedTime: "2026-01-01T00:00:00Z", Fields: map[string]any{"Attachments": []any{"not-a-map"}}},
				{ID: "rec-missing", CreatedTime: "2026-01-01T00:00:00Z", Fields: map[string]any{"Attachments": []any{map[string]any{"id": "att-missing", "filename": "missing.pdf"}}}},
			},
			Offset: "next",
		}, nil
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	var got []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
		for _, doc := range batch.Documents {
			got = append(got, doc.SourceID)
		}
	}
	if len(got) != 6 {
		t.Fatalf("prune documents = %v, want 6", got)
	}
	want := map[string]bool{
		"airtable:rec-1":       true,
		"airtable:rec-1:att-1": true,
		"airtable:rec-invalid": true,
		"airtable:rec-missing": true,
		"airtable:rec-2":       true,
		"airtable:rec-2:att-1": true,
	}
	for _, sourceID := range got {
		delete(want, sourceID)
	}
	if len(want) != 0 {
		t.Fatalf("prune documents missing %v; got %v", want, got)
	}
}

func TestAirtablePrunePaginationStall(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.listRecords = func(ctx context.Context, url string) (airtableRecordPage, error) {
		return airtableRecordPage{Offset: "same", Records: []airtableRecord{
			airtableTestRecordWithIDTime("rec-1", "2026-01-01T00:00:00Z", "a.pdf"),
		}}, nil
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	pruneSession := session.(*airtablePruneSession)
	pruneSession.pageURL = connector.recordsURL("same", airtablePageSize)
	if _, err := pruneSession.NextBatch(context.Background()); err == nil || !strings.Contains(err.Error(), "did not advance") {
		t.Fatalf("prune NextBatch err = %v, want stalled pagination error", err)
	}
}

func TestAirtableFetch(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.sizeThreshold = 5
	ref := FetchReference{Key: `{"record_id":"rec-1","attachment_id":"att-1","filename":"big.pdf","url":"https://example.test/big","size":10}`}
	if _, err := connector.Fetch(context.Background(), ref); err == nil || !strings.Contains(err.Error(), "exceeds size threshold") {
		t.Fatalf("oversize Fetch err = %v", err)
	}

	var fetchedURL string
	connector.downloadFile = func(ctx context.Context, rawURL string) ([]byte, error) {
		fetchedURL = rawURL
		return []byte("hello"), nil
	}
	ref.Key = `{"record_id":"rec-1","attachment_id":"att-1","filename":"ok.pdf","url":"https://example.test/ok","size":5}`
	blob, err := connector.Fetch(context.Background(), ref)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if string(blob) != "hello" || fetchedURL != "https://example.test/ok" {
		t.Fatalf("fetch blob = %q, url = %q", blob, fetchedURL)
	}
}

func TestAirtableRecordsURL(t *testing.T) {
	connector := newAirtableTestConnector()
	connector.baseID = "base 1"
	connector.tableNameOrID = "My Table"
	got := connector.recordsURL("tok", 100)
	if !strings.Contains(got, "/base%201/My%20Table?") || !strings.Contains(got, "pageSize=100") || !strings.Contains(got, "offset=tok") {
		t.Fatalf("records URL = %q", got)
	}
}

func TestAirtableDoJSONRetriesTransientStatus(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"records":[]}`))
	}))
	defer server.Close()

	connector := newAirtableTestConnector()
	connector.apiBaseURL = server.URL + "/v0"
	connector.httpClient = server.Client()
	var page airtableRecordPage
	if err := connector.doJSON(context.Background(), connector.recordsURL("", 1), &page); err != nil {
		t.Fatalf("doJSON failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestAirtableDoJSONReadsBodyBeforeCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`{"records":[]}`))
	}))
	defer server.Close()

	connector := newAirtableTestConnector()
	connector.apiBaseURL = server.URL + "/v0"
	connector.httpClient = server.Client()
	var page airtableRecordPage
	if err := connector.doJSON(context.Background(), connector.recordsURL("", 1), &page); err != nil {
		t.Fatalf("doJSON failed: %v", err)
	}
}

func newAirtableTestConnector() *AirtableConnector {
	return &AirtableConnector{
		baseID:        "base 1",
		tableNameOrID: "My Table",
		accessToken:   "token",
		batchSize:     airtableDefaultBatchSize,
		sizeThreshold: airtableDefaultSizeThreshold,
		apiBaseURL:    airtableAPIBaseURL,
		httpClient:    &http.Client{Timeout: time.Second},
	}
}

func airtableTestRecord() airtableRecord {
	return airtableRecord{
		ID:          "rec-1",
		CreatedTime: "2026-01-02T03:04:05.000Z",
		Fields: map[string]any{
			"Attachments": []any{
				map[string]any{"id": "att-1", "url": "https://example.test/a.pdf", "filename": "report.PDF", "size": float64(5), "type": "application/pdf"},
				"not-an-attachment",
				map[string]any{"id": "att-2", "url": "https://example.test/b.txt", "filename": "notes.txt", "size": float64(3)},
			},
			"Tags": []any{"one", "two"},
		},
	}
}

func airtableTestRecordWithIDTime(recordID, createdTime, filename string) airtableRecord {
	return airtableRecord{
		ID:          recordID,
		CreatedTime: createdTime,
		Fields: map[string]any{
			"Attachments": []any{
				map[string]any{"id": "att-1", "url": "https://example.test/" + filename, "filename": filename, "size": float64(1)},
			},
		},
	}
}

func airtableTestAttachment(filename string) airtableAttachment {
	return airtableAttachment{
		ID:          "att-1",
		URL:         "https://example.test/" + filename,
		Filename:    filename,
		Size:        1,
		FieldName:   "Attachments",
		CreatedTime: "2026-01-03T00:00:00Z",
	}
}

func airtableMustTimePointer(t *testing.T, value string) *time.Time {
	t.Helper()
	parsed := mustTime(t, value)
	return &parsed
}
