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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"ragflow/internal/utility"
)

func TestNewSalesforceConnectorDefaults(t *testing.T) {
	connector, err := NewSalesforceConnector(map[string]any{
		"credentials": map[string]any{
			"instance_url":  "https://your-domain.my.salesforce.com/",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewSalesforceConnector failed: %v", err)
	}
	if connector.instanceURL != "https://your-domain.my.salesforce.com" {
		t.Fatalf("instance url = %q", connector.instanceURL)
	}
	if connector.apiVersion != "v59.0" {
		t.Fatalf("api version = %q", connector.apiVersion)
	}
	if connector.batchSize != 2 {
		t.Fatalf("batch size = %d", connector.batchSize)
	}
	want := []string{"Account", "Contact", "Opportunity", "Case", "Knowledge__kav"}
	if len(connector.objects) != len(want) {
		t.Fatalf("objects = %v, want %v", connector.objects, want)
	}
	for i := range want {
		if connector.objects[i] != want[i] {
			t.Fatalf("objects = %v, want %v", connector.objects, want)
		}
	}
}

func TestNewSalesforceConnectorObjectsAndBatch(t *testing.T) {
	connector, err := NewSalesforceConnector(map[string]any{
		"objects":     "Account, Contact",
		"api_version": "v62.0",
		"batch_size":  5,
		"credentials": map[string]any{
			"instance_url":  "https://acme.my.salesforce.com",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewSalesforceConnector failed: %v", err)
	}
	if len(connector.objects) != 2 || connector.objects[0] != "Account" || connector.objects[1] != "Contact" {
		t.Fatalf("objects = %v", connector.objects)
	}
	if connector.apiVersion != "v62.0" {
		t.Fatalf("api version = %q", connector.apiVersion)
	}
	if connector.batchSize != 5 {
		t.Fatalf("batch size = %d", connector.batchSize)
	}
}

func TestSalesforceConnectorValidateMissingCredentials(t *testing.T) {
	connector, err := NewSalesforceConnector(map[string]any{"credentials": map[string]any{}})
	if err != nil {
		t.Fatalf("NewSalesforceConnector failed: %v", err)
	}
	var credErr *ConnectorMissingCredentialError
	if err := connector.Validate(context.Background()); !errors.As(err, &credErr) {
		t.Fatalf("Validate err = %v, want ConnectorMissingCredentialError", err)
	}
}

func TestSalesforceConnectorValidateRejectsNonPositiveBatch(t *testing.T) {
	connector, err := NewSalesforceConnector(map[string]any{
		"batch_size": 0,
		"credentials": map[string]any{
			"instance_url":  "https://acme.my.salesforce.com",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("NewSalesforceConnector failed: %v", err)
	}
	var valErr *ConnectorValidationError
	if err := connector.Validate(context.Background()); !errors.As(err, &valErr) {
		t.Fatalf("Validate err = %v, want ConnectorValidationError", err)
	}
}

func TestSalesforceConnectorValidateQueriesObjects(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	var probed bool
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if !strings.HasSuffix(apiURL, "/services/data/v59.0/sobjects") {
			t.Fatalf("validate url = %q", apiURL)
		}
		probed = true
		payload := map[string]any{
			"sobjects": []any{
				map[string]any{"name": "Account", "queryable": true},
				map[string]any{"name": "Contact", "queryable": true},
				map[string]any{"name": "Opportunity", "queryable": true},
				map[string]any{"name": "Case", "queryable": true},
				map[string]any{"name": "Knowledge__kav", "queryable": true},
			},
		}
		data, _ := json.Marshal(payload)
		return json.Unmarshal(data, out)
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !probed {
		t.Fatalf("Validate did not probe /sobjects")
	}
}

func TestSalesforceConnectorValidateUnknownObject(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.objects = []string{"Account", "Bogus"}
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		payload := map[string]any{
			"sobjects": []any{
				map[string]any{"name": "Account", "queryable": true},
			},
		}
		data, _ := json.Marshal(payload)
		return json.Unmarshal(data, out)
	}
	var valErr *ConnectorValidationError
	if err := connector.Validate(context.Background()); !errors.As(err, &valErr) {
		t.Fatalf("Validate err = %v, want ConnectorValidationError", err)
	}
}

func TestSalesforceConnectorValidateSkipsOptionalKnowledge(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		payload := map[string]any{
			"sobjects": []any{
				map[string]any{"name": "Account", "queryable": true},
				map[string]any{"name": "Contact", "queryable": true},
				map[string]any{"name": "Opportunity", "queryable": true},
				map[string]any{"name": "Case", "queryable": true},
				// Knowledge__kav absent: must be skipped silently.
			},
		}
		data, _ := json.Marshal(payload)
		return json.Unmarshal(data, out)
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
}

func TestSalesforceConnectorValidateMapsHTTPStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: &ConnectorMissingCredentialError{}},
		{name: "forbidden", status: http.StatusForbidden, want: &ConnectorValidationError{}},
		{name: "server error", status: http.StatusInternalServerError, want: &ConnectorValidationError{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connector := newSalesforceFixtureConnector()
			connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
				return &salesforceHTTPError{status: tc.status, body: "boom"}
			}
			err := connector.Validate(context.Background())
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Validate err = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate err = nil, want %T", tc.want)
			}
			switch tc.want.(type) {
			case *ConnectorMissingCredentialError:
				var want *ConnectorMissingCredentialError
				if !errors.As(err, &want) {
					t.Fatalf("Validate err = %v, want ConnectorMissingCredentialError", err)
				}
			case *ConnectorValidationError:
				var want *ConnectorValidationError
				if !errors.As(err, &want) {
					t.Fatalf("Validate err = %v, want ConnectorValidationError", err)
				}
			}
		})
	}
}

func TestSalesforceConnectorOpenSync(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = salesforceFixtureDoJSON(t)

	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
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
	doc := batch.Documents[0]
	if doc.SourceID != "Account/0015g00000Example1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Acme Corp" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-03T00:00:00Z")) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Metadata["object"] != "Account" || doc.Metadata["record_id"] != "0015g00000Example1" {
		t.Fatalf("metadata = %+v", doc.Metadata)
	}
	if doc.Metadata["web_url"] != "https://acme.my.salesforce.com/0015g00000Example1" {
		t.Fatalf("web_url = %v", doc.Metadata["web_url"])
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	blob := string(doc.Blob)
	if !strings.Contains(blob, "Salesforce Account") || !strings.Contains(blob, "Name: Acme Corp") {
		t.Fatalf("blob = %q", blob)
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "Account/0015g00000Example2" {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestSalesforceConnectorOpenSyncWindowAndPagination(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	var soql string
	var fieldsQueried string
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		parsed, err := url.Parse(apiURL)
		if err != nil {
			t.Fatalf("parse url: %v", err)
		}
		if strings.Contains(apiURL, "/sobjects/Account/describe") {
			// Compound address/location fields must be filtered from SOQL.
			fieldsQueried = ""
		}
		if strings.Contains(apiURL, "/query?") {
			soql, _ = url.QueryUnescape(parsed.Query().Get("q"))
		}
		return salesforceFixtureDoJSON(t)(ctx, apiURL, out)
	}

	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	for {
		_, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
	}
	if !strings.Contains(soql, "SystemModstamp > 2026-01-02T00:00:00Z") {
		t.Fatalf("soql missing since bound: %q", soql)
	}
	if !strings.Contains(soql, "SystemModstamp <= 2026-01-04T00:00:00Z") {
		t.Fatalf("soql missing until bound: %q", soql)
	}
	if !strings.Contains(soql, " ORDER BY SystemModstamp ASC") {
		t.Fatalf("soql missing ordering: %q", soql)
	}
	if strings.Contains(soql, "BillingAddress") || strings.Contains(soql, "Location__c") {
		t.Fatalf("soql must exclude compound fields: %q", soql)
	}
	_ = fieldsQueried
}

func TestSalesforceConnectorOpenSyncResume(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = salesforceFixtureDoJSON(t)

	// Drain the whole object so the final record's checkpoint advances the
	// per-object cursor; the syncer persists that checkpoint per batch.
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(first.Documents) != 2 {
		t.Fatalf("first documents len = %d, want 2", len(first.Documents))
	}
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("first session EOF = %v", err)
	}

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resume NextBatch = %v, want EOF (object already ingested)", err)
	}
}

func TestSalesforceConnectorOpenSyncResumeRejectsInvalidCheckpoint(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.objects = []string{"Account", "Contact"}
	validCursor, err := json.Marshal(salesforceSyncCursor{Cursors: map[string]salesforceObjectCursor{
		"Account": {SystemModstamp: "2026-01-03T01:00:00.000+0000", Id: "0015g00000Example2"},
	}})
	if err != nil {
		t.Fatalf("marshal valid cursor: %v", err)
	}
	validCursorJSON := string(validCursor)
	cases := []struct {
		name       string
		checkpoint *SyncCheckpoint
	}{
		{
			name:       "missing cursor",
			checkpoint: &SyncCheckpoint{SourceID: "Account/0015g00000Example2"},
		},
		{
			name:       "invalid cursor json",
			checkpoint: &SyncCheckpoint{Cursor: "not-json", SourceID: "Account/0015g00000Example2"},
		},
		{
			name:       "missing source anchor",
			checkpoint: &SyncCheckpoint{Cursor: validCursorJSON},
		},
		{
			name:       "malformed source anchor",
			checkpoint: &SyncCheckpoint{Cursor: validCursorJSON, SourceID: "Account"},
		},
		{
			name:       "anchor object not configured",
			checkpoint: &SyncCheckpoint{Cursor: validCursorJSON, SourceID: "Bogus/0015g00000Example2"},
		},
		{
			name: "no object positions",
			checkpoint: &SyncCheckpoint{
				Cursor:   `{"cursors":{}}`,
				SourceID: "Account/0015g00000Example2",
			},
		},
		{
			name: "missing anchor object cursor",
			checkpoint: &SyncCheckpoint{
				Cursor:   `{"cursors":{"Contact":{"system_modstamp":"2026-01-03T01:00:00.000+0000","id":"0035g00000Example2"}}}`,
				SourceID: "Account/0015g00000Example2",
			},
		},
		{
			name: "cursor references unknown object",
			checkpoint: &SyncCheckpoint{
				Cursor:   `{"cursors":{"Account":{"system_modstamp":"2026-01-03T01:00:00.000+0000","id":"0015g00000Example2"},"Bogus":{"system_modstamp":"2026-01-03T01:00:00.000+0000","id":"x"}}}`,
				SourceID: "Account/0015g00000Example2",
			},
		},
		{
			name: "object cursor missing fields",
			checkpoint: &SyncCheckpoint{
				Cursor:   `{"cursors":{"Account":{}}}`,
				SourceID: "Account/0015g00000Example2",
			},
		},
		{
			name: "object cursor invalid timestamp",
			checkpoint: &SyncCheckpoint{
				Cursor:   `{"cursors":{"Account":{"system_modstamp":"bad","id":"0015g00000Example2"}}}`,
				SourceID: "Account/0015g00000Example2",
			},
		},
		{
			name: "object cursor record id mismatch",
			checkpoint: &SyncCheckpoint{
				Cursor:   `{"cursors":{"Account":{"system_modstamp":"2026-01-03T01:00:00.000+0000","id":"0015g00000Other"}}}`,
				SourceID: "Account/0015g00000Example2",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: tc.checkpoint})
			if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
			}
		})
	}
}

func TestSalesforceConnectorOpenSyncResumeRejectsMissingAnchor(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		page := salesforceQueryPage{}
		data, _ := json.Marshal(page)
		return json.Unmarshal(data, out)
	}
	cursor, err := json.Marshal(salesforceSyncCursor{Cursors: map[string]salesforceObjectCursor{
		"Account": {SystemModstamp: "2026-01-03T01:00:00.000+0000", Id: "0015g00000Example2"},
	}})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursor), SourceID: "Account/0015g00000Example2"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	if _, err = session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestSalesforceConnectorOpenSyncResumeRejectsChangedAnchor(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if !strings.Contains(apiURL, "/query?") {
			t.Fatalf("unexpected url %s", apiURL)
		}
		page := salesforceQueryPage{Records: []map[string]any{{
			"Id":             "0015g00000Example2",
			"SystemModstamp": "2026-01-03T02:00:00.000+0000",
		}}}
		data, _ := json.Marshal(page)
		return json.Unmarshal(data, out)
	}
	cursor, err := json.Marshal(salesforceSyncCursor{Cursors: map[string]salesforceObjectCursor{
		"Account": {SystemModstamp: "2026-01-03T01:00:00.000+0000", Id: "0015g00000Example2"},
	}})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursor), SourceID: "Account/0015g00000Example2"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	if _, err = session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestSalesforceConnectorOpenSyncResumeRejectsUnavailableObject(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		return &salesforceObjectUnavailableError{message: "object gone"}
	}
	cursor, err := json.Marshal(salesforceSyncCursor{Cursors: map[string]salesforceObjectCursor{
		"Account": {SystemModstamp: "2026-01-03T01:00:00.000+0000", Id: "0015g00000Example2"},
	}})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursor), SourceID: "Account/0015g00000Example2"},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	if _, err = session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestSalesforceConnectorOpenSyncPaginatedResume(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.batchSize = 2
	records := []map[string]any{
		{"Id": "0015g00000Example1", "Name": "Acme Corp", "SystemModstamp": "2026-01-03T00:00:00.000+0000"},
		{"Id": "0015g00000Example2", "Name": "Globex", "SystemModstamp": "2026-01-03T01:00:00.000+0000"},
		{"Id": "0015g00000Example3", "Name": "Initech", "SystemModstamp": "2026-01-03T02:00:00.000+0000"},
	}
	var queryCalls int
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if strings.Contains(apiURL, "/sobjects/Account/describe") {
			return json.Unmarshal([]byte(`{"fields":[
				{"name":"Id","type":"id"},
				{"name":"Name","type":"string"},
				{"name":"SystemModstamp","type":"datetime"}
			]}`), out)
		}
		if strings.Contains(apiURL, "/query?") || strings.Contains(apiURL, "/query/01gExampleNext") {
			queryCalls++
			var page salesforceQueryPage
			if queryCalls == 1 {
				page = salesforceQueryPage{
					Records:        records[:2],
					NextRecordsURL: "/services/data/v59.0/query/01gExampleNext",
				}
			} else {
				// Second page: fixture returns the last record with done=true.
				// The per-object cursor must only advance on this final record.
				page = salesforceQueryPage{Records: records[2:]}
			}
			data, _ := json.Marshal(page)
			return json.Unmarshal(data, out)
		}
		t.Fatalf("unexpected url %s", apiURL)
		return nil
	}

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(first.Documents) != 2 || first.Documents[0].SourceID != "Account/0015g00000Example1" {
		t.Fatalf("first documents = %+v", first.Documents)
	}
	// The first batch ends before the object drains, so its checkpoint must
	// NOT advance the Account cursor yet (no committed cursor to resume from).
	if first.Checkpoint == nil {
		t.Fatalf("first checkpoint is nil")
	}
	var firstCursor salesforceSyncCursor
	if err := json.Unmarshal([]byte(first.Checkpoint.Cursor), &firstCursor); err != nil {
		t.Fatalf("parse first checkpoint cursor: %v", err)
	}
	if _, ok := firstCursor.Cursors["Account"]; ok {
		t.Fatalf("first batch checkpoint cursor advanced Account too early: %+v", firstCursor.Cursors)
	}

	second, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "Account/0015g00000Example3" {
		t.Fatalf("second documents = %+v", second.Documents)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("session EOF = %v", err)
	}
	// The final batch's checkpoint carries the advanced Account cursor.
	var finalCursor salesforceSyncCursor
	if err := json.Unmarshal([]byte(second.Checkpoint.Cursor), &finalCursor); err != nil {
		t.Fatalf("parse final checkpoint cursor: %v", err)
	}
	finalAccount := finalCursor.Cursors["Account"]
	if finalAccount.SystemModstamp != "2026-01-03T02:00:00.000+0000" || finalAccount.Id != "0015g00000Example3" {
		t.Fatalf("final checkpoint cursor = %+v, want Account advanced with composite cursor", finalCursor.Cursors)
	}

	// Resume from the second batch: the object is fully ingested, so the next
	// run must not re-emit its records. The fixture returns all three records
	// on its first query page for the resumed run; the since predicate derived
	// from the advanced cursor must filter them out.
	connector2 := newSalesforceFixtureConnector()
	connector2.batchSize = 2
	var resumedQueryCalls int
	connector2.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if strings.Contains(apiURL, "/sobjects/Account/describe") {
			return json.Unmarshal([]byte(`{"fields":[
				{"name":"Id","type":"id"},
				{"name":"Name","type":"string"},
				{"name":"SystemModstamp","type":"datetime"}
			]}`), out)
		}
		if strings.Contains(apiURL, "/query?") {
			parsed, _ := url.Parse(apiURL)
			soql, _ := url.QueryUnescape(parsed.Query().Get("q"))
			if strings.Contains(soql, "WHERE Id = '0015g00000Example3'") {
				page := salesforceQueryPage{Records: records[2:3]}
				data, _ := json.Marshal(page)
				return json.Unmarshal(data, out)
			}
			resumedQueryCalls++
			if !strings.Contains(soql, "SystemModstamp > 2026-01-03T02:00:00Z") {
				t.Fatalf("resumed soql missing since predicate: %q", soql)
			}
			var page salesforceQueryPage
			if strings.Contains(soql, "SystemModstamp > 2026-01-03T02:00:00Z") {
				// No records strictly newer than the cursor.
				page = salesforceQueryPage{Records: []map[string]any{}}
			} else {
				page = salesforceQueryPage{Records: records}
			}
			data, _ := json.Marshal(page)
			return json.Unmarshal(data, out)
		}
		t.Fatalf("unexpected url %s", apiURL)
		return nil
	}
	resumed, err := connector2.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: second.Checkpoint})
	if err != nil {
		t.Fatalf("resumed OpenSync failed: %v", err)
	}
	if _, err = resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("resumed NextBatch = %v, want EOF", err)
	}
}

func TestSalesforceConnectorOpenPrune(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = salesforceFixtureDoJSON(t)

	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	want := []string{"Account/0015g00000Example1", "Account/0015g00000Example2"}
	if len(got) != len(want) {
		t.Fatalf("prune documents = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("prune documents = %v, want %v", got, want)
		}
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("prune NextBatch EOF = %v", err)
	}
}

func TestSalesforceConnectorOpenSyncSkipsMissingObject(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.objects = []string{"Account", "Case"}
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if strings.Contains(apiURL, "/sobjects/Case/describe") {
			return &salesforceObjectUnavailableError{message: "Case unavailable"}
		}
		return salesforceFixtureDoJSON(t)(ctx, apiURL, out)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents len = %d, want 2 (Case skipped)", len(batch.Documents))
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestSalesforceConnectorOpenPruneSkipsMissingObject(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.objects = []string{"Account", "Case"}
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if strings.Contains(apiURL, "/query?") && strings.Contains(apiURL, "FROM+Case") {
			return &salesforceObjectUnavailableError{message: "Case unavailable"}
		}
		return salesforceFixtureDoJSON(t)(ctx, apiURL, out)
	}
	session, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	got := []string{}
	for _, doc := range batch.Documents {
		got = append(got, doc.SourceID)
	}
	if len(got) != 2 || got[0] != "Account/0015g00000Example1" {
		t.Fatalf("prune documents = %v, want Account records only", got)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("prune NextBatch EOF = %v", err)
	}
}

func TestSalesforceRecordToTextDeterministic(t *testing.T) {
	record := map[string]any{
		"Name":          "Acme Corp",
		"Industry":      "Software",
		"attributes":    map[string]any{"type": "Account"},
		"AnnualRevenue": 1000.5,
		"Description":   "",
	}
	text1 := salesforceRecordToText("Account", record)
	text2 := salesforceRecordToText("Account", record)
	if text1 != text2 {
		t.Fatalf("record text unstable: %q vs %q", text1, text2)
	}
	if !strings.HasPrefix(text1, "Salesforce Account\n") {
		t.Fatalf("record text = %q", text1)
	}
	if strings.Contains(text1, "attributes") {
		t.Fatalf("record text should skip attributes: %q", text1)
	}
	if strings.Contains(text1, "Description:") {
		t.Fatalf("record text should skip empty values: %q", text1)
	}
}

func TestParseSalesforceTime(t *testing.T) {
	cases := []string{
		"2026-01-03T00:00:00.000+0000",
		"2026-01-03T00:00:00+0000",
		"2026-01-03T00:00:00.000Z",
		"2026-01-03T00:00:00Z",
	}
	for _, value := range cases {
		parsed, err := parseSalesforceTime(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		if parsed.UTC() != mustTime(t, "2026-01-03T00:00:00Z") {
			t.Fatalf("parse %q = %s", value, parsed)
		}
	}
}

func TestSalesforceConnectorValidateConnectorSetting(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		payload := map[string]any{
			"sobjects": []any{
				map[string]any{"name": "Account", "queryable": true},
				map[string]any{"name": "Contact", "queryable": true},
				map[string]any{"name": "Opportunity", "queryable": true},
				map[string]any{"name": "Case", "queryable": true},
				map[string]any{"name": "Knowledge__kav", "queryable": true},
			},
		}
		data, _ := json.Marshal(payload)
		return json.Unmarshal(data, out)
	}
	// The receiver is a valid fixture; the unsaved request is what must be
	// validated. Use a request whose credential set is incomplete so the
	// temporary connector fails fast regardless of the receiver's state.
	request := map[string]any{
		"credentials": map[string]any{
			"instance_url": "https://acme.my.salesforce.com",
			"client_id":    "client",
			// client_secret intentionally omitted.
		},
	}
	var credErr *ConnectorMissingCredentialError
	if err := connector.ValidateConnectorSetting(context.Background(), request); !errors.As(err, &credErr) {
		t.Fatalf("ValidateConnectorSetting err = %v, want ConnectorMissingCredentialError for the request", err)
	}
}

func TestSalesforceConnectorValidateConnectorSettingUsesRequest(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	// Receiver itself would pass with only Account; the request lists Bogus, so
	// a successful validation must be derived from the request, not the receiver.
	connector.objects = []string{"Account"}
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		payload := map[string]any{
			"sobjects": []any{
				map[string]any{"name": "Account", "queryable": true},
			},
		}
		data, _ := json.Marshal(payload)
		return json.Unmarshal(data, out)
	}
	request := map[string]any{
		"objects": "Account, Bogus",
		"credentials": map[string]any{
			"instance_url":  "https://acme.my.salesforce.com",
			"client_id":     "client",
			"client_secret": "secret",
		},
	}
	var valErr *ConnectorValidationError
	if err := connector.ValidateConnectorSetting(context.Background(), request); !errors.As(err, &valErr) {
		t.Fatalf("ValidateConnectorSetting err = %v, want ConnectorValidationError for the unsaved request", err)
	}
}

func TestRegisterBuiltInsOpensSalesforce(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("salesforce", map[string]any{
		"credentials": map[string]any{
			"instance_url":  "https://acme.my.salesforce.com",
			"client_id":     "client",
			"client_secret": "secret",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig failed: %v", err)
	}
	if _, ok := connector.(*SalesforceConnector); !ok {
		t.Fatalf("connector type = %T, want *SalesforceConnector", connector)
	}
}

func TestSalesforceConnectorOpenSyncSameTimestampResume(t *testing.T) {
	ts := "2026-01-03T00:00:00.000+0000"
	records := []map[string]any{
		{"Id": "0015g00000Example1", "Name": "Acme Corp", "SystemModstamp": ts},
		{"Id": "0015g00000Example2", "Name": "Globex", "SystemModstamp": ts},
	}

	// First run with batchSize 1: each record becomes one batch and the object
	// only drains on the last one, so the persisted cursor advances to the
	// composite position (ts, Example2) of the final same-instant record.
	connector := newSalesforceFixtureConnector()
	connector.batchSize = 1
	connector.doJSON = salesforceFixtureRecordsDoJSON(t, records)
	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	var checkpoint *SyncCheckpoint
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
		if batch.Checkpoint != nil {
			checkpoint = batch.Checkpoint
		}
	}
	var curs salesforceSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &curs); err != nil {
		t.Fatalf("parse cursor: %v", err)
	}
	acct := curs.Cursors["Account"]
	if acct.SystemModstamp != ts || acct.Id != "0015g00000Example2" {
		t.Fatalf("cursor = %+v, want (ts, Example2)", acct)
	}

	// A third record created in the same instant but sorting after the
	// checkpoint by Id must be delivered on resume. The resumed fixture still
	// carries all records; the composite WHERE clause must select only the
	// same-instant record whose Id sorts after the cursor.
	resumedRecords := append(append([]map[string]any{}, records...), map[string]any{
		"Id": "0015g00000Example3", "Name": "Initech", "SystemModstamp": ts,
	})
	resumed := newSalesforceFixtureConnector()
	resumed.batchSize = 1
	var resumedSOQL string
	resumed.doJSON = func(ctx context.Context, apiURL string, out any) error {
		if strings.Contains(apiURL, "/query?") {
			parsed, err := url.Parse(apiURL)
			if err != nil {
				t.Fatalf("parse query url: %v", err)
			}
			resumedSOQL, _ = url.QueryUnescape(parsed.Query().Get("q"))
		}
		return salesforceFixtureRecordsDoJSON(t, resumedRecords)(ctx, apiURL, out)
	}
	resumedSession, err := resumed.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
	if err != nil {
		t.Fatalf("resumed OpenSync failed: %v", err)
	}
	var gotIDs []string
	for {
		batch, err := resumedSession.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("resumed NextBatch failed: %v", err)
		}
		for _, doc := range batch.Documents {
			gotIDs = append(gotIDs, doc.SourceID)
		}
	}
	if !strings.Contains(resumedSOQL, "AND Id > '0015g00000Example2'") {
		t.Fatalf("resumed SOQL missing same-instant Id boundary: %q", resumedSOQL)
	}
	if len(gotIDs) != 1 || gotIDs[0] != "Account/0015g00000Example3" {
		t.Fatalf("resumed documents = %v, want only the same-instant record after the cursor", gotIDs)
	}
}

func TestSalesforceObjectUnavailable404(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "structured NOT_FOUND",
			body: `[{"message":"sObject type 'Bogus' is not supported. If you intend to use a custom object, make sure it is enabled in \"Setup > Object Manager\" and \"API\" is enabled in the Object's Detail Page.","errorCode":"NOT_FOUND"}]`,
			want: true,
		},
		{
			name: "object level permissions NOT_FOUND",
			body: `[{"message":"Object type 'Case' is not supported. If you intend to use a custom object...","errorCode":"NOT_FOUND"}]`,
			want: true,
		},
		{
			name: "generic not found route",
			body: `404 page not found`,
			want: false,
		},
		{
			name: "empty body",
			body: ``,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := salesforceObjectUnavailable(http.StatusNotFound, []byte(tc.body)); got != tc.want {
				t.Fatalf("salesforceObjectUnavailable(404, %q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestSalesforceObjectUnavailable400InvalidType(t *testing.T) {
	body := `[{"message":"\nSELECT Account__c FROM Bogus\n                                 ^\nERROR at Row:1:Column:18\nsObject type 'Bogus' is not supported.","errorCode":"INVALID_TYPE"}]`
	if !salesforceObjectUnavailable(http.StatusBadRequest, []byte(body)) {
		t.Fatalf("expected 400 INVALID_TYPE to be unavailable")
	}
	if salesforceObjectUnavailable(http.StatusBadRequest, []byte(`bad request`)) {
		t.Fatalf("did not expect generic 400 to be unavailable")
	}
	if salesforceObjectUnavailable(http.StatusForbidden, []byte(`forbidden`)) {
		t.Fatalf("did not expect 403 to be unavailable")
	}
}

func TestSalesforceHostAllowed(t *testing.T) {
	allowed := []string{
		"acme.my.salesforce.com",
		"login.salesforce.com",
		"ACME.MY.SALESFORCE.COM",
		"instance.salesforce.com",
		"custom.force.com",
		"org.lightning.force.com",
		"salesforce.com",
		"force.com",
	}
	for _, host := range allowed {
		if !salesforceHostAllowed(host) {
			t.Fatalf("salesforceHostAllowed(%q) = false, want true", host)
		}
	}
	blocked := []string{
		"",
		"acme.my.salesforce.com.evil.com",
		"evil.example.com",
		"salesforce.com.evil.com",
		"force.com.attacker.io",
		"127.0.0.1",
	}
	for _, host := range blocked {
		if salesforceHostAllowed(host) {
			t.Fatalf("salesforceHostAllowed(%q) = true, want false", host)
		}
	}
}

func TestRequestAccessTokenRejectsNonSalesforceHost(t *testing.T) {
	origLookup := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		return []string{"93.184.216.34"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = origLookup })

	connector := &SalesforceConnector{
		instanceURL:  "https://evil.example.com",
		clientID:     "client",
		clientSecret: "secret",
		now:          time.Now,
	}
	var credErr *ConnectorMissingCredentialError
	if _, err := connector.requestAccessToken(context.Background()); !errors.As(err, &credErr) {
		t.Fatalf("requestAccessToken err = %v, want ConnectorMissingCredentialError for non-Salesforce host", err)
	}
}

func TestRequestAccessTokenRequiresHTTPS(t *testing.T) {
	origLookup := utility.LookupHost
	utility.LookupHost = func(host string) ([]string, error) {
		return []string{"93.184.216.34"}, nil
	}
	t.Cleanup(func() { utility.LookupHost = origLookup })

	connector := &SalesforceConnector{
		instanceURL:  "http://acme.my.salesforce.com",
		clientID:     "client",
		clientSecret: "secret",
		now:          time.Now,
	}
	var credErr *ConnectorMissingCredentialError
	if _, err := connector.requestAccessToken(context.Background()); !errors.As(err, &credErr) {
		t.Fatalf("requestAccessToken err = %v, want ConnectorMissingCredentialError for non-HTTPS token endpoint", err)
	}
}

func TestSalesforceConnectorAPIURLAbsoluteValidation(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	snap := salesforceToken{InstanceURL: "https://acme.my.salesforce.com"}

	// A well-formed absolute pagination URL on an approved HTTPS host is kept.
	valid := "https://instance2.my.salesforce.com/services/data/v59.0/query/01gX"
	got, err := connector.apiURL(snap, valid)
	if err != nil {
		t.Fatalf("apiURL(valid) err = %v", err)
	}
	if got != valid {
		t.Fatalf("apiURL(valid) = %q", got)
	}

	// Relative Salesforce API paths are still constructed from the snapshot.
	rel, err := connector.apiURL(snap, "/sobjects/Account/describe")
	if err != nil {
		t.Fatalf("apiURL(relative) err = %v", err)
	}
	wantRel := "https://acme.my.salesforce.com/services/data/v59.0/sobjects/Account/describe"
	if rel != wantRel {
		t.Fatalf("apiURL(relative) = %q, want %q", rel, wantRel)
	}

	// Absolute pagination URLs with a bad scheme or host must be rejected.
	for _, bad := range []string{
		"http://acme.my.salesforce.com/services/data/v59.0/query/01gX",
		"https://evil.example.com/services/data/v59.0/query/01gX",
		"https://acme.my.salesforce.com.evil.com/services/data/v59.0/query/01gX",
	} {
		if _, err := connector.apiURL(snap, bad); err == nil {
			t.Fatalf("apiURL(%q) succeeded, want error", bad)
		}
	}
}

func TestSalesforceConnectorGetJSONRejectsUnsafePaginationURL(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	connector.doJSON = func(ctx context.Context, apiURL string, out any) error {
		t.Fatalf("doJSON must not run for an unsafe pagination URL")
		return nil
	}
	var page salesforceQueryPage
	if err := connector.getJSON(context.Background(), "http://evil.example.com/services/data/v59.0/query/01gX", &page); err == nil {
		t.Fatalf("getJSON succeeded for an unsafe pagination URL, want error")
	}
}

func TestSalesforceConnectorDoGetRejectsUnsafeURL(t *testing.T) {
	connector := newSalesforceFixtureConnector()
	for _, bad := range []string{
		"http://acme.my.salesforce.com/services/data/v59.0/sobjects",
		"https://evil.example.com/services/data/v59.0/sobjects",
	} {
		if _, _, err := connector.doGet(context.Background(), bad, "token"); err == nil {
			t.Fatalf("doGet(%q) succeeded, want error", bad)
		}
	}
}

// newSalesforceFixtureConnector builds a connector with token acquisition
// short-circuited so unit tests never touch the network.
func newSalesforceFixtureConnector() *SalesforceConnector {
	connector := &SalesforceConnector{
		instanceURL:  "https://acme.my.salesforce.com",
		clientID:     "client",
		clientSecret: "secret",
		objects:      []string{"Account"},
		apiVersion:   defaultSalesforceAPIVersion,
		batchSize:    defaultSalesforceBatchSize,
		now:          time.Now,
	}
	connector.acquireAccessToken = func(ctx context.Context) (salesforceToken, error) {
		return salesforceToken{
			AccessToken: "token",
			InstanceURL: "https://acme.my.salesforce.com",
			ExpiresAt:   time.Now().Add(time.Hour),
		}, nil
	}
	return connector
}

// salesforceFixtureDoJSON serves describe + query responses for unit tests.
// The query endpoint emulates server-side SOQL filtering on SystemModstamp so
// resume tests behave like a real org.
func salesforceFixtureDoJSON(t *testing.T) func(ctx context.Context, apiURL string, out any) error {
	t.Helper()
	records := []map[string]any{
		{
			"Id":             "0015g00000Example1",
			"Name":           "Acme Corp",
			"Industry":       "Software",
			"SystemModstamp": "2026-01-03T00:00:00.000+0000",
		},
		{
			"Id":             "0015g00000Example2",
			"Name":           "Globex",
			"Industry":       "Hardware",
			"SystemModstamp": "2026-01-03T01:00:00.000+0000",
		},
	}
	return func(ctx context.Context, apiURL string, out any) error {
		var body string
		switch {
		case strings.Contains(apiURL, "/sobjects/Account/describe"):
			body = `{"fields":[
				{"name":"Id","type":"id"},
				{"name":"Name","type":"string"},
				{"name":"Industry","type":"string"},
				{"name":"AnnualRevenue","type":"currency"},
				{"name":"BillingAddress","type":"address"},
				{"name":"Location__c","type":"location"}
			]}`
		case strings.Contains(apiURL, "/query?"):
			parsed, err := url.Parse(apiURL)
			if err != nil {
				t.Fatalf("parse query url: %v", err)
			}
			soql, err := url.QueryUnescape(parsed.Query().Get("q"))
			if err != nil {
				t.Fatalf("unescape soql: %v", err)
			}
			filtered := []map[string]any{}
			for _, record := range records {
				if salesforceFixtureMatchesSOQL(t, soql, record) {
					filtered = append(filtered, record)
				}
			}
			payload := map[string]any{"totalSize": len(filtered), "done": true, "records": filtered}
			data, _ := json.Marshal(payload)
			return json.Unmarshal(data, out)
		default:
			t.Fatalf("unexpected api url %s", apiURL)
		}
		return json.Unmarshal([]byte(body), out)
	}
}

// salesforceFixtureRecordsDoJSON serves describe + query responses for an
// arbitrary record set, emulating server-side SOQL filtering on SystemModstamp
// (including the composite resume predicate).
func salesforceFixtureRecordsDoJSON(t *testing.T, records []map[string]any) func(ctx context.Context, apiURL string, out any) error {
	t.Helper()
	return func(ctx context.Context, apiURL string, out any) error {
		switch {
		case strings.Contains(apiURL, "/sobjects/Account/describe"):
			body := `{"fields":[
				{"name":"Id","type":"id"},
				{"name":"Name","type":"string"},
				{"name":"SystemModstamp","type":"datetime"}
			]}`
			return json.Unmarshal([]byte(body), out)
		case strings.Contains(apiURL, "/query?"):
			parsed, err := url.Parse(apiURL)
			if err != nil {
				t.Fatalf("parse query url: %v", err)
			}
			soql, err := url.QueryUnescape(parsed.Query().Get("q"))
			if err != nil {
				t.Fatalf("unescape soql: %v", err)
			}
			filtered := []map[string]any{}
			for _, record := range records {
				if salesforceFixtureMatchesSOQL(t, soql, record) {
					filtered = append(filtered, record)
				}
			}
			payload := map[string]any{"totalSize": len(filtered), "done": true, "records": filtered}
			data, _ := json.Marshal(payload)
			return json.Unmarshal(data, out)
		default:
			t.Fatalf("unexpected api url %s", apiURL)
		}
		return nil
	}
}

// salesforceFixtureMatchesSOQL applies the fixture's SystemModstamp predicates,
// including the composite resume clause `(SystemModstamp > X OR (SystemModstamp
// = X AND Id > 'Y'))` so same-instant records that sort after the checkpoint by
// Id are kept.
func salesforceFixtureMatchesSOQL(t *testing.T, soql string, record map[string]any) bool {
	t.Helper()
	lower := strings.ToLower(soql)
	if idx := strings.Index(lower, "where id = '"); idx >= 0 {
		value := strings.Trim(strings.TrimSpace(soql[idx+len("WHERE Id = '"):]), "'")
		id, _ := record["Id"].(string)
		return id == value
	}
	since := time.Time{}
	until := time.Time{}
	equalSince := false
	minID := ""
	if idx := strings.Index(lower, "systemmodstamp >"); idx >= 0 {
		rest := soql[idx+len("SystemModstamp > "):]
		value := strings.TrimSpace(strings.Split(rest, " ")[0])
		parsed, err := parseSalesforceTime(strings.Trim(value, "'"))
		if err != nil {
			t.Fatalf("parse soql since %q: %v", value, err)
		}
		since = parsed
	}
	if idx := strings.Index(lower, "systemmodstamp ="); idx >= 0 {
		rest := soql[idx+len("SystemModstamp = "):]
		value := strings.TrimSpace(strings.Split(rest, " ")[0])
		parsed, err := parseSalesforceTime(strings.Trim(value, "'"))
		if err != nil {
			t.Fatalf("parse soql equal timestamp %q: %v", value, err)
		}
		equalSince = true
		since = parsed
	}
	if idx := strings.Index(lower, "and id >"); idx >= 0 {
		value := soql[idx+len("AND Id > "):]
		minID = strings.Trim(strings.TrimSpace(value), "'")
	}
	if idx := strings.Index(lower, "systemmodstamp <= "); idx >= 0 {
		rest := soql[idx+len("SystemModstamp <= "):]
		value := strings.TrimSpace(strings.Split(rest, " ")[0])
		parsed, err := parseSalesforceTime(strings.Trim(value, "'"))
		if err != nil {
			t.Fatalf("parse soql until %q: %v", value, err)
		}
		until = parsed
	}
	modified, err := parseSalesforceTime(record["SystemModstamp"].(string))
	if err != nil {
		t.Fatalf("parse fixture timestamp: %v", err)
	}
	if !until.IsZero() && modified.After(until) {
		return false
	}
	if !since.IsZero() {
		if modified.After(since) {
			return true
		}
		if modified.Before(since) {
			return false
		}
		// Same instant as the checkpoint: keep records that sort after it by Id
		// only when a same-instant Id boundary is present.
		if equalSince && minID != "" {
			id, _ := record["Id"].(string)
			return id > minID
		}
		return false
	}
	return true
}
