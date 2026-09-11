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
	"strconv"
	"strings"
	"testing"
)

func TestZendeskConnectorConfigDefaultsAndValidation(t *testing.T) {
	connector, err := NewZendeskConnector(map[string]any{
		"credentials": map[string]any{
			"zendesk_subdomain": "support",
			"zendesk_email":     "alice@example.com",
			"zendesk_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("NewZendeskConnector: %v", err)
	}
	if connector.contentType != zendeskContentTypeArticles {
		t.Fatalf("content type = %q, want articles", connector.contentType)
	}
	if connector.batchSize != defaultZendeskBatchSize {
		t.Fatalf("batch size = %d, want %d", connector.batchSize, defaultZendeskBatchSize)
	}
	if connector.baseURL != "https://support.zendesk.com/api/v2" {
		t.Fatalf("base URL = %q", connector.baseURL)
	}

	tickets, err := NewZendeskConnector(map[string]any{
		"zendesk_content_type": "tickets",
		"batch_size":           7,
		"credentials": map[string]any{
			"zendesk_subdomain": "support",
			"zendesk_email":     "alice@example.com",
			"zendesk_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("NewZendeskConnector tickets: %v", err)
	}
	if tickets.contentType != zendeskContentTypeTickets || tickets.batchSize != 7 {
		t.Fatalf("tickets config = %#v", tickets)
	}

	_, err = NewZendeskConnector(map[string]any{"zendesk_content_type": "wiki"})
	var valErr *ConnectorValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("invalid content type error = %T %v, want ConnectorValidationError", err, err)
	}

	zeroBatch, err := NewZendeskConnector(map[string]any{
		"batch_size": 0,
		"credentials": map[string]any{
			"zendesk_subdomain": "support",
			"zendesk_email":     "alice@example.com",
			"zendesk_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("NewZendeskConnector zero batch: %v", err)
	}
	err = zeroBatch.Validate(context.Background())
	if !errors.As(err, &valErr) {
		t.Fatalf("zero batch validation error = %T %v, want ConnectorValidationError", err, err)
	}
}

func TestZendeskSubdomainValidation(t *testing.T) {
	tests := []struct {
		raw string
		ok  bool
	}{
		{raw: "support", ok: true},
		{raw: "support.zendesk.com", ok: true},
		{raw: "https://support.zendesk.com", ok: true},
		{raw: "evil.example.com", ok: false},
		{raw: "subdomain#evil.example.com", ok: false},
		{raw: "", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			_, err := NewZendeskConnector(map[string]any{
				"credentials": map[string]any{
					"zendesk_subdomain": tc.raw,
					"zendesk_email":     "alice@example.com",
					"zendesk_token":     "token",
				},
			})
			if tc.ok && err != nil {
				t.Fatalf("NewZendeskConnector(%q) error = %v", tc.raw, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("NewZendeskConnector(%q) accepted unsafe subdomain", tc.raw)
			}
		})
	}
}

func TestZendeskConnectorValidateUsesArticlePage(t *testing.T) {
	connector := newTestZendeskConnector(t, nil)
	calls := 0
	connector.doJSON = func(ctx context.Context, endpoint string, query url.Values, out any) error {
		calls++
		if endpoint != "help_center/articles" {
			t.Fatalf("validation endpoint = %q, want help_center/articles", endpoint)
		}
		if query.Get("page[size]") != "1" || query.Get("start_time") != "0" {
			t.Fatalf("validation query = %v", query)
		}
		if _, ok := out.(*zendeskArticlePage); !ok {
			t.Fatalf("validation output type = %T", out)
		}
		return nil
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("validation calls = %d, want 1", calls)
	}
}

func TestZendeskConnectorValidateUsesTicketsEndpoint(t *testing.T) {
	connector := newTestZendeskConnector(t, map[string]any{"zendesk_content_type": "tickets"})
	calls := 0
	connector.doJSON = func(ctx context.Context, endpoint string, query url.Values, out any) error {
		calls++
		if endpoint != "incremental/tickets.json" {
			t.Fatalf("validation endpoint = %q, want incremental/tickets.json", endpoint)
		}
		if query.Get("start_time") != "0" {
			t.Fatalf("validation query = %v", query)
		}
		if _, ok := out.(*zendeskTicketPage); !ok {
			t.Fatalf("validation output type = %T", out)
		}
		return nil
	}
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("validation calls = %d, want 1", calls)
	}
}

func TestZendeskConnectorValidateMapsHTTPStatus(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{status: http.StatusUnauthorized, want: &ConnectorMissingCredentialError{}},
		{status: http.StatusForbidden, want: &ConnectorValidationError{}},
		{status: http.StatusNotFound, want: &ConnectorValidationError{}},
		{status: http.StatusInternalServerError, want: &ConnectorValidationError{}},
	}
	for _, tc := range tests {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			connector := newTestZendeskConnector(t, nil)
			connector.doJSON = func(ctx context.Context, endpoint string, query url.Values, out any) error {
				return &zendeskAPIError{Status: tc.status, Message: "boom"}
			}
			err := connector.Validate(context.Background())
			var missing *ConnectorMissingCredentialError
			var validation *ConnectorValidationError
			switch tc.want.(type) {
			case *ConnectorMissingCredentialError:
				if !errors.As(err, &missing) {
					t.Fatalf("Validate err = %T %v, want ConnectorMissingCredentialError", err, err)
				}
			case *ConnectorValidationError:
				if !errors.As(err, &validation) {
					t.Fatalf("Validate err = %T %v, want ConnectorValidationError", err, err)
				}
			}
		})
	}
}

func TestZendeskConnectorOpenSyncArticles(t *testing.T) {
	t.Setenv("ZENDESK_CONNECTOR_SKIP_ARTICLE_LABELS", "internal, secret")
	connector := newTestZendeskConnector(t, nil)
	stub := &zendeskTestStub{
		t: t,
		articlePages: map[string]zendeskArticlePage{
			"": {
				Articles: []zendeskArticle{
					{ID: json.Number("1"), Title: "Draft", Body: "draft body", Draft: true},
					{ID: json.Number("2"), Title: "Empty", Body: "<p>  </p>"},
					{ID: json.Number("3"), Title: "Labeled", Body: "labeled body", LabelNames: []string{"internal"}},
					{
						ID:            json.Number("4"),
						Title:         "Good",
						Body:          "<p>Hello <b>World</b></p>",
						UpdatedAt:     "2026-01-02T00:00:00Z",
						LabelNames:    []string{"public"},
						ContentTagIDs: []json.Number{"t1"},
						AuthorID:      json.Number("10"),
					},
				},
				Meta: zendeskArticleMeta{HasMore: true, AfterCursor: "c1"},
			},
			"c1": {
				Articles: []zendeskArticle{
					{ID: json.Number("5"), Title: "Next", Body: "next body", UpdatedAt: "2026-01-05T00:00:00Z"},
				},
				Meta: zendeskArticleMeta{HasMore: false},
			},
		},
		contentTags: zendeskContentTagPage{
			Records: []zendeskContentTag{{ID: json.Number("t1"), Name: "Tutorial"}},
		},
		users: map[string]zendeskUser{
			"10": {Name: "Alice", Email: "alice@example.com"},
		},
	}
	connector.doJSON = stub.doJSON

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer func() { _ = session.Close() }()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(batch.Documents))
	}
	good := batch.Documents[0]
	if good.SourceID != "article:4" || good.SemanticIdentifier != "Good" || good.Extension != ".txt" {
		t.Fatalf("article doc = %#v", good)
	}
	if !strings.Contains(string(good.Blob), "Hello") || !strings.Contains(string(good.Blob), "World") {
		t.Fatalf("article body = %q", good.Blob)
	}
	if good.SizeBytes != int64(len(good.Blob)) || good.Fingerprint != contentFingerprint(good.Blob) {
		t.Fatalf("article size/fingerprint = %#v", good)
	}
	if !good.UpdatedAt.Equal(mustTime(t, "2026-01-02T00:00:00Z")) {
		t.Fatalf("article updated at = %v", good.UpdatedAt)
	}
	if got := good.Metadata["labels"]; len(got.([]string)) != 1 || got.([]string)[0] != "public" {
		t.Fatalf("article labels = %#v", got)
	}
	if got := good.Metadata["content_tags"]; len(got.([]string)) != 1 || got.([]string)[0] != "Tutorial" {
		t.Fatalf("article content tags = %#v", got)
	}
	if got := good.Metadata["primary_owners"]; len(got.([]map[string]string)) != 1 || got.([]map[string]string)[0]["email"] != "alice@example.com" {
		t.Fatalf("article owners = %#v", got)
	}
	if batch.Documents[1].SourceID != "article:5" {
		t.Fatalf("second document = %#v", batch.Documents[1])
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "article:5" {
		t.Fatalf("checkpoint = %#v", batch.Checkpoint)
	}
}

func TestZendeskConnectorOpenSyncFiltersFingerprint(t *testing.T) {
	connector := newTestZendeskConnector(t, nil)
	same := "same body"
	stub := &zendeskTestStub{
		t: t,
		articlePages: map[string]zendeskArticlePage{
			"": {
				Articles: []zendeskArticle{
					{ID: json.Number("1"), Title: "Same", Body: same, UpdatedAt: "2026-01-02T00:00:00Z"},
					{ID: json.Number("2"), Title: "Changed", Body: "changed body", UpdatedAt: "2026-01-03T00:00:00Z"},
					{ID: json.Number("3"), Title: "New", Body: "new body", UpdatedAt: "2026-01-04T00:00:00Z"},
				},
				Meta: zendeskArticleMeta{HasMore: false},
			},
		},
	}
	connector.doJSON = stub.doJSON
	request := SyncRequest{
		WindowEnd: mustTime(t, "2026-01-05T00:00:00Z"),
		Fingerprints: map[string]string{
			"article:1": contentFingerprint([]byte(htmlToText(same))),
			"article:2": "old-fingerprint",
		},
	}
	session, err := connector.OpenSync(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 || batch.Documents[0].SourceID != "article:2" || batch.Documents[1].SourceID != "article:3" {
		t.Fatalf("fingerprint filtered docs = %#v", batch.Documents)
	}
}

func TestZendeskConnectorOpenSyncWindowFilter(t *testing.T) {
	connector := newTestZendeskConnector(t, nil)
	stub := &zendeskTestStub{
		t: t,
		articlePages: map[string]zendeskArticlePage{
			"": {
				Articles: []zendeskArticle{
					{ID: json.Number("1"), Title: "Old", Body: "old", UpdatedAt: "2026-01-01T00:00:00Z"},
					{ID: json.Number("2"), Title: "Current", Body: "current", UpdatedAt: "2026-01-03T00:00:00Z"},
					{ID: json.Number("3"), Title: "Future", Body: "future", UpdatedAt: "2026-01-05T00:00:00Z"},
				},
				Meta: zendeskArticleMeta{HasMore: false},
			},
		},
	}
	connector.doJSON = stub.doJSON
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")
	session, err := connector.OpenSync(context.Background(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "article:2" {
		t.Fatalf("window filtered docs = %#v", batch.Documents)
	}
}

func TestZendeskConnectorOpenSyncResumeArticles(t *testing.T) {
	connector := newTestZendeskConnector(t, map[string]any{"batch_size": 1})
	stub := &zendeskTestStub{
		t: t,
		articlePages: map[string]zendeskArticlePage{
			"": {
				Articles: []zendeskArticle{
					{ID: json.Number("1"), Title: "One", Body: "one", UpdatedAt: "2026-01-01T00:00:00Z"},
					{ID: json.Number("2"), Title: "Two", Body: "two", UpdatedAt: "2026-01-02T00:00:00Z"},
				},
				Meta: zendeskArticleMeta{HasMore: false},
			},
		},
	}
	connector.doJSON = stub.doJSON
	firstSession, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("first OpenSync: %v", err)
	}
	first, err := firstSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "article:1" || first.Checkpoint == nil {
		t.Fatalf("first batch = %#v", first)
	}
	_ = firstSession.Close()

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        first.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resumed OpenSync: %v", err)
	}
	batch, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "article:2" {
		t.Fatalf("resumed batch = %#v", batch.Documents)
	}
	_ = resumed.Close()
}

func TestZendeskConnectorResumeRejectsInvalidCheckpoint(t *testing.T) {
	connector := newTestZendeskConnector(t, nil)
	connector.doJSON = func(ctx context.Context, endpoint string, query url.Values, out any) error {
		t.Fatalf("unexpected API call after invalid resume")
		return nil
	}
	cases := map[string]*SyncCheckpoint{
		"missing":   {},
		"malformed": {Cursor: "not-json"},
		"foreign": {
			Cursor:   `{"source_id":"zendesk_ticket_1"}`,
			SourceID: "zendesk_ticket_1",
		},
		"wrong-content-type": {
			Cursor:   `{"content_type":"tickets","source_id":"article:1"}`,
			SourceID: "article:1",
		},
	}
	for name, checkpoint := range cases {
		t.Run(name, func(t *testing.T) {
			session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: checkpoint})
			if err != nil {
				t.Fatalf("OpenSync: %v", err)
			}
			_, err = session.NextBatch(context.Background())
			if !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("NextBatch err = %v, want ErrSyncResumeInvalid", err)
			}
			_ = session.Close()
		})
	}
}

func TestZendeskConnectorResumeRejectsMissingArticleAnchor(t *testing.T) {
	connector := newTestZendeskConnector(t, nil)
	stub := &zendeskTestStub{
		t: t,
		articlePages: map[string]zendeskArticlePage{
			"": {
				Articles: []zendeskArticle{{ID: json.Number("1"), Title: "One", Body: "one"}},
				Meta:     zendeskArticleMeta{HasMore: false},
			},
		},
	}
	connector.doJSON = stub.doJSON
	cursor, err := json.Marshal(zendeskSyncCursor{ContentType: zendeskContentTypeArticles, SourceID: "article:999"})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursor), SourceID: "article:999"},
	})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer func() { _ = session.Close() }()
	_, err = session.NextBatch(context.Background())
	if !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestZendeskConnectorCheckpointUsesPageContainingLastDocument(t *testing.T) {
	t.Run("articles", func(t *testing.T) {
		connector := newTestZendeskConnector(t, map[string]any{"batch_size": 2})
		stub := &zendeskTestStub{
			t: t,
			articlePages: map[string]zendeskArticlePage{
				"": {
					Articles: []zendeskArticle{
						{ID: json.Number("1"), Title: "One", Body: "one", UpdatedAt: "2026-01-01T00:00:00Z"},
						{ID: json.Number("2"), Title: "Draft", Body: "draft", Draft: true},
					},
					Meta: zendeskArticleMeta{HasMore: true, AfterCursor: "c1"},
				},
				"c1": {
					Articles: []zendeskArticle{},
					Meta:     zendeskArticleMeta{HasMore: false},
				},
			},
		}
		connector.doJSON = stub.doJSON
		session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
		if err != nil {
			t.Fatalf("OpenSync: %v", err)
		}
		first, err := session.NextBatch(context.Background())
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if len(first.Documents) != 1 || first.Documents[0].SourceID != "article:1" || first.Checkpoint == nil {
			t.Fatalf("first batch = %#v", first)
		}
		var cursor zendeskSyncCursor
		if err := json.Unmarshal([]byte(first.Checkpoint.Cursor), &cursor); err != nil {
			t.Fatalf("decode cursor: %v", err)
		}
		if cursor.AfterCursor != "" {
			t.Fatalf("article cursor after_cursor = %q, want page containing article:1", cursor.AfterCursor)
		}
		_ = session.Close()

		resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
		if err != nil {
			t.Fatalf("resumed OpenSync: %v", err)
		}
		_, err = resumed.NextBatch(context.Background())
		if !errors.Is(err, io.EOF) {
			t.Fatalf("resumed NextBatch = %v, want EOF", err)
		}
		_ = resumed.Close()
	})

	t.Run("tickets", func(t *testing.T) {
		connector := newTestZendeskConnector(t, map[string]any{"zendesk_content_type": "tickets", "batch_size": 2})
		stub := &zendeskTestStub{
			t: t,
			ticketPages: map[int64]zendeskTicketPage{
				0: {
					Tickets: []zendeskTicket{
						{ID: json.Number("1"), Subject: "One", Status: "open", UpdatedAt: "2026-01-01T00:00:00Z"},
					},
					EndTime:     100,
					EndOfStream: false,
				},
				100: {
					Tickets:     []zendeskTicket{},
					EndTime:     200,
					EndOfStream: true,
				},
			},
			comments: map[string]zendeskCommentsPage{
				"tickets/1/comments": {},
			},
		}
		connector.doJSON = stub.doJSON
		session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
		if err != nil {
			t.Fatalf("OpenSync: %v", err)
		}
		first, err := session.NextBatch(context.Background())
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if len(first.Documents) != 1 || first.Documents[0].SourceID != "zendesk_ticket_1" || first.Checkpoint == nil {
			t.Fatalf("first batch = %#v", first)
		}
		var cursor zendeskSyncCursor
		if err := json.Unmarshal([]byte(first.Checkpoint.Cursor), &cursor); err != nil {
			t.Fatalf("decode cursor: %v", err)
		}
		if cursor.StartTime != 0 {
			t.Fatalf("ticket cursor start_time = %d, want page containing zendesk_ticket_1", cursor.StartTime)
		}
		_ = session.Close()

		resumed, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
		if err != nil {
			t.Fatalf("resumed OpenSync: %v", err)
		}
		_, err = resumed.NextBatch(context.Background())
		if !errors.Is(err, io.EOF) {
			t.Fatalf("resumed NextBatch = %v, want EOF", err)
		}
		_ = resumed.Close()
	})
}

func TestZendeskConnectorOpenSyncTickets(t *testing.T) {
	connector := newTestZendeskConnector(t, map[string]any{"zendesk_content_type": "tickets", "batch_size": 10})
	stub := &zendeskTestStub{
		t: t,
		ticketPages: map[int64]zendeskTicketPage{
			0: {
				Tickets: []zendeskTicket{
					{ID: json.Number("1"), Subject: "Deleted", Status: "deleted"},
					{
						ID:         json.Number("2"),
						Subject:    "Login",
						UpdatedAt:  "2026-01-02T00:00:00Z",
						Status:     "open",
						Priority:   "high",
						Tags:       []string{"prod"},
						TicketType: "question",
						Submitter:  json.Number("10"),
					},
				},
				EndTime:     100,
				EndOfStream: false,
			},
			100: {
				Tickets: []zendeskTicket{
					{ID: json.Number("3"), UpdatedAt: "2026-01-03T00:00:00Z", Status: "solved"},
				},
				EndTime:     200,
				EndOfStream: true,
			},
		},
		comments: map[string]zendeskCommentsPage{
			"tickets/2/comments": {
				Comments: []zendeskComment{
					{Body: "please help", CreatedAt: "2026-01-02T01:00:00Z", AuthorID: json.Number("20")},
				},
			},
			"tickets/3/comments": {},
		},
		users: map[string]zendeskUser{
			"10": {Name: "Alice", Email: "alice@example.com"},
			"20": {Name: "Bob", Email: "bob@example.com"},
		},
	}
	connector.doJSON = stub.doJSON

	session, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer func() { _ = session.Close() }()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(batch.Documents))
	}
	ticket := batch.Documents[0]
	if ticket.SourceID != "zendesk_ticket_2" || ticket.SemanticIdentifier != "Ticket #2: Login" {
		t.Fatalf("ticket doc = %#v", ticket)
	}
	body := string(ticket.Blob)
	for _, want := range []string{"Ticket Subject:\nLogin", "Comment by Bob at 2026-01-02T01:00:00Z:\nplease help"} {
		if !strings.Contains(body, want) {
			t.Fatalf("ticket body missing %q:\n%s", want, body)
		}
	}
	if ticket.Metadata["status"] != "open" || ticket.Metadata["priority"] != "high" {
		t.Fatalf("ticket metadata = %#v", ticket.Metadata)
	}
	if got := ticket.Metadata["tags"]; len(got.([]string)) != 1 || got.([]string)[0] != "prod" {
		t.Fatalf("ticket tags = %#v", got)
	}
	if got := ticket.Metadata["primary_owners"]; len(got.([]map[string]string)) != 1 || got.([]map[string]string)[0]["name"] != "Alice" {
		t.Fatalf("ticket owners = %#v", got)
	}
	if batch.Documents[1].SourceID != "zendesk_ticket_3" || batch.Documents[1].SemanticIdentifier != "Ticket #3: No Subject" {
		t.Fatalf("second ticket = %#v", batch.Documents[1])
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "zendesk_ticket_3" {
		t.Fatalf("ticket checkpoint = %#v", batch.Checkpoint)
	}
}

func TestZendeskConnectorOpenSyncResumeTickets(t *testing.T) {
	connector := newTestZendeskConnector(t, map[string]any{"zendesk_content_type": "tickets", "batch_size": 1})
	stub := &zendeskTestStub{
		t: t,
		ticketPages: map[int64]zendeskTicketPage{
			0: {
				Tickets: []zendeskTicket{
					{ID: json.Number("1"), Subject: "One", Status: "open", UpdatedAt: "2026-01-01T00:00:00Z"},
					{ID: json.Number("2"), Subject: "Two", Status: "open", UpdatedAt: "2026-01-02T00:00:00Z"},
				},
				EndTime:     100,
				EndOfStream: true,
			},
		},
		comments: map[string]zendeskCommentsPage{
			"tickets/1/comments": {},
			"tickets/2/comments": {},
		},
	}
	connector.doJSON = stub.doJSON
	firstSession, err := connector.OpenSync(context.Background(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("first OpenSync: %v", err)
	}
	first, err := firstSession.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch: %v", err)
	}
	if len(first.Documents) != 1 || first.Documents[0].SourceID != "zendesk_ticket_1" || first.Checkpoint == nil {
		t.Fatalf("first batch = %#v", first)
	}
	_ = firstSession.Close()

	resumed, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        first.Checkpoint,
	})
	if err != nil {
		t.Fatalf("resumed OpenSync: %v", err)
	}
	batch, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "zendesk_ticket_2" {
		t.Fatalf("resumed batch = %#v", batch.Documents)
	}
	_ = resumed.Close()
}

func TestZendeskConnectorPrune(t *testing.T) {
	t.Run("articles", func(t *testing.T) {
		connector := newTestZendeskConnector(t, nil)
		stub := &zendeskTestStub{
			t: t,
			articlePages: map[string]zendeskArticlePage{
				"": {
					Articles: []zendeskArticle{
						{ID: json.Number("1"), Title: "One", Body: "one"},
						{ID: json.Number("2"), Title: "Draft", Body: "draft", Draft: true},
						{ID: json.Number("3"), Title: "Three", Body: "three"},
					},
					Meta: zendeskArticleMeta{HasMore: true, AfterCursor: "c1"},
				},
				"c1": {
					Articles: []zendeskArticle{{ID: json.Number("4"), Title: "Four", Body: "four"}},
					Meta:     zendeskArticleMeta{HasMore: false},
				},
			},
		}
		connector.doJSON = stub.doJSON
		session, err := connector.OpenPrune(context.Background(), PruneRequest{})
		if err != nil {
			t.Fatalf("OpenPrune: %v", err)
		}
		defer func() { _ = session.Close() }()
		batch, err := session.NextBatch(context.Background())
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		got := zendeskSlimSourceIDs(batch.Documents)
		want := []string{"article:1", "article:3", "article:4"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("prune docs = %#v, want %#v", got, want)
		}
		if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
			t.Fatalf("final NextBatch = %v, want EOF", err)
		}
	})

	t.Run("tickets", func(t *testing.T) {
		connector := newTestZendeskConnector(t, map[string]any{"zendesk_content_type": "tickets"})
		stub := &zendeskTestStub{
			t: t,
			ticketPages: map[int64]zendeskTicketPage{
				0: {
					Tickets: []zendeskTicket{
						{ID: json.Number("1"), Subject: "Deleted", Status: "deleted"},
						{ID: json.Number("2"), Subject: "Two", Status: "open"},
						{ID: json.Number("3"), Subject: "Three", Status: "solved"},
					},
					EndTime:     100,
					EndOfStream: true,
				},
			},
		}
		connector.doJSON = stub.doJSON
		session, err := connector.OpenPrune(context.Background(), PruneRequest{})
		if err != nil {
			t.Fatalf("OpenPrune: %v", err)
		}
		defer func() { _ = session.Close() }()
		batch, err := session.NextBatch(context.Background())
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		got := zendeskSlimSourceIDs(batch.Documents)
		if strings.Join(got, ",") != "zendesk_ticket_2,zendesk_ticket_3" {
			t.Fatalf("prune docs = %#v", got)
		}
	})
}

func TestZendeskConnectorValidateConnectorSettingUsesRequestConfig(t *testing.T) {
	receiver := newTestZendeskConnector(t, nil)
	transport := &zendeskTestTransport{t: t}
	receiver.httpClient = &http.Client{Transport: transport}
	err := receiver.ValidateConnectorSetting(context.Background(), map[string]any{
		"zendesk_content_type": "tickets",
		"credentials": map[string]any{
			"zendesk_subdomain": "other",
			"zendesk_email":     "bob@example.com",
			"zendesk_token":     "token2",
		},
	})
	var missing *ConnectorMissingCredentialError
	if !errors.As(err, &missing) {
		t.Fatalf("ValidateConnectorSetting err = %T %v, want ConnectorMissingCredentialError", err, err)
	}
	if transport.calls != 1 {
		t.Fatalf("validation calls = %d, want 1", transport.calls)
	}
}

func TestZendeskConnectorRegisteredBuiltIn(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltIns(registry)
	connector, err := registry.OpenFromConfig("zendesk", map[string]any{
		"credentials": map[string]any{
			"zendesk_subdomain": "support",
			"zendesk_email":     "alice@example.com",
			"zendesk_token":     "token",
		},
	})
	if err != nil {
		t.Fatalf("OpenFromConfig: %v", err)
	}
	if _, ok := connector.(*ZendeskConnector); !ok {
		t.Fatalf("connector type = %T, want *ZendeskConnector", connector)
	}
}

func newTestZendeskConnector(t *testing.T, overrides map[string]any) *ZendeskConnector {
	t.Helper()
	config := map[string]any{
		"credentials": map[string]any{
			"zendesk_subdomain": "support",
			"zendesk_email":     "alice@example.com",
			"zendesk_token":     "token",
		},
	}
	for key, value := range overrides {
		config[key] = value
	}
	connector, err := NewZendeskConnector(config)
	if err != nil {
		t.Fatalf("NewZendeskConnector: %v", err)
	}
	return connector
}

type zendeskTestTransport struct {
	t     *testing.T
	calls int
}

func (tr *zendeskTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.t.Helper()
	tr.calls++
	wantURL := "https://other.zendesk.com/api/v2/incremental/tickets.json?start_time=0"
	if req.URL.String() != wantURL {
		tr.t.Fatalf("request URL = %q, want %q", req.URL.String(), wantURL)
	}
	username, password, ok := req.BasicAuth()
	if !ok || username != "bob@example.com/token" || password != "token2" {
		tr.t.Fatalf("request auth = username %q password %q ok %v", username, password, ok)
	}
	return &http.Response{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("bad auth")),
		Request:    req,
	}, nil
}

type zendeskTestStub struct {
	t            *testing.T
	articlePages map[string]zendeskArticlePage
	ticketPages  map[int64]zendeskTicketPage
	comments     map[string]zendeskCommentsPage
	users        map[string]zendeskUser
	contentTags  zendeskContentTagPage
}

func (s *zendeskTestStub) doJSON(ctx context.Context, endpoint string, query url.Values, out any) error {
	s.t.Helper()
	switch {
	case strings.HasPrefix(endpoint, "users/"):
		response := out.(*zendeskUserResponse)
		response.User = s.users[strings.TrimPrefix(endpoint, "users/")]
	case endpoint == "help_center/articles":
		page := out.(*zendeskArticlePage)
		*page = s.articlePages[query.Get("page[after]")]
	case endpoint == "guide/content_tags":
		page := out.(*zendeskContentTagPage)
		*page = s.contentTags
	case endpoint == "incremental/tickets.json":
		start, err := strconv.ParseInt(query.Get("start_time"), 10, 64)
		if err != nil {
			s.t.Fatalf("invalid ticket start_time %q: %v", query.Get("start_time"), err)
		}
		page := out.(*zendeskTicketPage)
		*page = s.ticketPages[start]
	case strings.HasPrefix(endpoint, "tickets/"):
		page := out.(*zendeskCommentsPage)
		*page = s.comments[endpoint]
	default:
		s.t.Fatalf("unexpected Zendesk endpoint %q", endpoint)
	}
	return nil
}

func zendeskSlimSourceIDs(documents []SlimDocument) []string {
	out := make([]string, 0, len(documents))
	for _, document := range documents {
		out = append(out, document.SourceID)
	}
	return out
}
