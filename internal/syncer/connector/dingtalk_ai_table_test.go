package connector

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
						{"id": "rec-1", "fields": map[string]any{"Title": "Roadmap", "Count": 3}},
					},
				})
				return
			}
			writeJSON(t, w, map[string]any{
				"records": []map[string]any{
					{"id": "rec-2", "fields": map[string]any{"Title": "Budget"}},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	connector, err := NewDingTalkAITableConnector(map[string]any{
		"table_id":     "table-1",
		"operator_id":  "operator-1",
		"batch_size":   1,
		"api_base_url": server.URL,
		"credentials":  map[string]any{"access_token": "token-1"},
	})
	if err != nil {
		t.Fatalf("NewDingTalkAITableConnector failed: %v", err)
	}
	now := time.Date(2026, 8, 20, 1, 2, 3, 0, time.UTC)
	connector.now = func() time.Time { return now }

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, WindowEnd: now.Add(time.Hour)})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()

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
	defer session.Close()

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

func TestDingTalkAITableConnectorValidateConnectorSetting(t *testing.T) {
	connector, err := NewDingTalkAITableConnector(map[string]any{
		"table_id":    "table-1",
		"operator_id": "operator-1",
		"credentials": map[string]any{},
	})
	if err != nil {
		t.Fatalf("NewDingTalkAITableConnector failed: %v", err)
	}

	err = connector.ValidateConnectorSetting(context.Background(), nil)
	var credErr *ConnectorMissingCredentialError
	if !errors.As(err, &credErr) {
		t.Fatalf("error = %v, want ConnectorMissingCredentialError", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}
