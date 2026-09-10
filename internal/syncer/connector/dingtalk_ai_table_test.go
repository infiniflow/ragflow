package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestDingTalkAITableConnectorOpenSync(t *testing.T) {
	var recordRequests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-acs-dingtalk-access-token") != "token-1" {
			t.Fatalf("access token header = %q", r.Header.Get("x-acs-dingtalk-access-token"))
		}
		if r.URL.Query().Get("operatorId") != "operator-1" {
			t.Fatalf("operatorId = %q", r.URL.Query().Get("operatorId"))
		}
		switch r.URL.Path {
		case "/v1.0/notable/bases/table-1/sheets":
			if r.Method != http.MethodGet {
				t.Fatalf("sheets method = %s", r.Method)
			}
			writeJSON(t, w, map[string]any{
				"value": []map[string]any{
					{"id": "sheet-1", "name": "Projects"},
				},
			})
		case "/v1.0/notable/bases/table-1/sheets/sheet-1/records/list":
			if r.Method != http.MethodPost {
				t.Fatalf("records method = %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			recordRequests = append(recordRequests, body)
			if body["nextToken"] == nil {
				writeJSON(t, w, map[string]any{
					"nextToken": "page-2",
					"records": []map[string]any{
						{"id": "rec-1", "lastModifiedTime": int64(1780000000000), "fields": map[string]any{"Title": "Roadmap", "Count": 3}},
					},
				})
				return
			}
			writeJSON(t, w, map[string]any{
				"records": []map[string]any{
					{"id": "rec-2", "lastModifiedTime": int64(1780000001000), "fields": map[string]any{"zTitle": "Wrong", "aTitle": "Budget"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	connector, err := NewDingTalkAITableConnector(map[string]any{
		"table_id":    "table-1",
		"operator_id": "operator-1",
		"batch_size":  1,
		"credentials": map[string]any{"access_token": "token-1"},
	})
	if err != nil {
		t.Fatalf("NewDingTalkAITableConnector failed: %v", err)
	}
	connector.httpClient = dingTalkAITableTestHTTPClient(t, server)

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch first failed: %v", err)
	}
	if len(first.Documents) != 1 {
		t.Fatalf("first batch documents = %d, want 1", len(first.Documents))
	}
	doc := first.Documents[0]
	if doc.SourceID != "dingtalk_ai_table:table-1:sheet-1:rec-1" {
		t.Fatalf("SourceID = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Projects - Roadmap" {
		t.Fatalf("SemanticIdentifier = %q", doc.SemanticIdentifier)
	}
	if got := doc.UpdatedAt; !got.Equal(time.UnixMilli(1780000000000).UTC()) {
		t.Fatalf("UpdatedAt = %s, want %s", got, time.UnixMilli(1780000000000).UTC())
	}
	if doc.Extension != ".json" || doc.SizeBytes != int64(len(doc.Blob)) || doc.Fingerprint == "" {
		t.Fatalf("document shape = ext %q size %d fingerprint %q", doc.Extension, doc.SizeBytes, doc.Fingerprint)
	}
	if got := doc.Metadata["record_id"]; got != "rec-1" {
		t.Fatalf("record_id metadata = %v", got)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != doc.SourceID {
		t.Fatalf("checkpoint = %#v", first.Checkpoint)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch second failed: %v", err)
	}
	if got := second.Documents[0].SourceID; got != "dingtalk_ai_table:table-1:sheet-1:rec-2" {
		t.Fatalf("second SourceID = %q", got)
	}
	if got := second.Documents[0].SemanticIdentifier; got != "Projects - Budget" {
		t.Fatalf("second SemanticIdentifier = %q", got)
	}
	_, err = session.NextBatch(context.Background())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("final error = %v, want EOF", err)
	}
	if len(recordRequests) != 2 || recordRequests[0]["maxResults"] != float64(100) || recordRequests[1]["nextToken"] != "page-2" {
		t.Fatalf("record requests = %#v", recordRequests)
	}
}

func TestDingTalkAITableConnectorOpenPrune(t *testing.T) {
	connector := &DingTalkAITableConnector{
		tableID:     "table-1",
		operatorID:  "operator-1",
		accessToken: "token-1",
		batchSize:   10,
		apiBaseURL:  dingTalkAITableAPIBaseURL,
		getSheets: func(ctx context.Context) ([]dingTalkAITableSheet, error) {
			return []dingTalkAITableSheet{{ID: "sheet-1", Name: "Projects"}}, nil
		},
		listRecords: func(ctx context.Context, sheetID, nextToken string, maxResults int) ([]dingTalkAITableRecord, string, error) {
			if nextToken == "" {
				return []dingTalkAITableRecord{{ID: "rec-1"}, {ID: ""}}, "next", nil
			}
			return []dingTalkAITableRecord{{ID: "rec-2"}}, "", nil
		},
	}

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{batch.Documents[0].SourceID, batch.Documents[1].SourceID}
	want := []string{
		"dingtalk_ai_table:table-1:sheet-1:rec-1",
		"dingtalk_ai_table:table-1:sheet-1:rec-2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slim ids = %#v, want %#v", got, want)
	}
}

func TestDingTalkAITableConnectorOpenSyncFiltersByRecordLastModifiedTime(t *testing.T) {
	windowStart := time.UnixMilli(1780000000000).UTC()
	connector := &DingTalkAITableConnector{
		tableID:     "table-1",
		operatorID:  "operator-1",
		accessToken: "token-1",
		batchSize:   10,
		apiBaseURL:  dingTalkAITableAPIBaseURL,
		getSheets: func(ctx context.Context) ([]dingTalkAITableSheet, error) {
			return []dingTalkAITableSheet{{ID: "sheet-1", Name: "Projects"}}, nil
		},
		listRecords: func(ctx context.Context, sheetID, nextToken string, maxResults int) ([]dingTalkAITableRecord, string, error) {
			return []dingTalkAITableRecord{
				{ID: "old", LastModifiedTime: windowStart.Add(-time.Millisecond).UnixMilli(), Fields: map[string]any{"Title": "Old"}},
				{ID: "inside", LastModifiedTime: windowStart.Add(time.Millisecond).UnixMilli(), Fields: map[string]any{"Title": "Inside"}},
				{ID: "missing", Fields: map[string]any{"Title": "Missing"}},
				{ID: "invalid", LastModifiedTime: "invalid", Fields: map[string]any{"Title": "Invalid"}},
			}, "", nil
		},
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &windowStart, WindowEnd: windowStart.Add(time.Hour)})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "dingtalk_ai_table:table-1:sheet-1:inside" {
		t.Fatalf("documents = %#v, want only inside record", batch.Documents)
	}
}

func TestDingTalkAITableConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := &DingTalkAITableConnector{
		tableID:     "table-1",
		operatorID:  "operator-1",
		accessToken: "token-1",
		batchSize:   10,
		apiBaseURL:  dingTalkAITableAPIBaseURL,
		getSheets: func(ctx context.Context) ([]dingTalkAITableSheet, error) {
			return []dingTalkAITableSheet{{ID: "sheet-1", Name: "Projects"}}, nil
		},
		listRecords: func(ctx context.Context, sheetID, nextToken string, maxResults int) ([]dingTalkAITableRecord, string, error) {
			return []dingTalkAITableRecord{
				{ID: "rec-1", LastModifiedTime: time.UnixMilli(1780000000000).UnixMilli(), Fields: map[string]any{"Title": "Roadmap"}},
			}, "", nil
		},
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestDingTalkAITableConnectorValidateConnectorSetting(t *testing.T) {
	config := map[string]any{
		"table_id":    "table-1",
		"operator_id": "operator-1",
		"credentials": map[string]any{},
	}
	connector, err := NewDingTalkAITableConnector(config)
	if err != nil {
		t.Fatalf("NewDingTalkAITableConnector failed: %v", err)
	}

	err = connector.ValidateConnectorSetting(context.Background(), config)
	var credErr *ConnectorMissingCredentialError
	if !errors.As(err, &credErr) {
		t.Fatalf("error = %v, want ConnectorMissingCredentialError", err)
	}
}

func TestDingTalkAITableConnectorRejectsUnapprovedAPIBaseURL(t *testing.T) {
	connector := &DingTalkAITableConnector{
		tableID:     "table-1",
		operatorID:  "operator-1",
		accessToken: "token-1",
		batchSize:   10,
		apiBaseURL:  "http://example.com",
		httpClient:  http.DefaultClient,
	}

	err := connector.doJSON(context.Background(), http.MethodGet, "/v1.0/notable/bases/table-1/sheets", nil, nil)
	var validationErr *ConnectorValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ConnectorValidationError", err)
	}
}

func dingTalkAITableTestHTTPClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return &http.Client{Transport: dingTalkAITableRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme != "https" || req.URL.Host != "api.dingtalk.com" {
			t.Fatalf("request URL = %s, want https://api.dingtalk.com", req.URL.String())
		}
		cloned := req.Clone(req.Context())
		cloned.URL.Scheme = target.Scheme
		cloned.URL.Host = target.Host
		cloned.Host = target.Host
		return http.DefaultTransport.RoundTrip(cloned)
	})}
}

type dingTalkAITableRoundTripFunc func(req *http.Request) (*http.Response, error)

func (f dingTalkAITableRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}
