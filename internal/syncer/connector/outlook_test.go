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

func TestOutlookConnectorOpenSync(t *testing.T) {
	connector := newFixtureOutlookConnector()
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-04T00:00:00Z")

	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "msg-1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Hello/World" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	blob := string(doc.Blob)
	if !strings.Contains(blob, "From: Alice Example <alice@example.com>") ||
		!strings.Contains(blob, "To: bob@example.com") ||
		!strings.Contains(blob, "Cc: carol@example.com") ||
		!strings.Contains(blob, "Subject: Hello/World") ||
		!strings.Contains(blob, "Body text") {
		t.Fatalf("blob = %q", blob)
	}
	if doc.Extension != ".html" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-02T03:04:05Z")) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Metadata["user_id"] != "user-1" || doc.Metadata["folder"] != "archive" {
		t.Fatalf("metadata = %+v", doc.Metadata)
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if batch.Checkpoint == nil || batch.Checkpoint.SourceID != "msg-1" {
		t.Fatalf("checkpoint = %+v", batch.Checkpoint)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

func TestOutlookConnectorOpenSyncResumesWithinPage(t *testing.T) {
	connector := newFixtureOutlookConnector()
	connector.OutlookConnector.batchSize = 2
	connector.OutlookConnector.getDeltaPage = func(ctx context.Context, apiURL string) (outlookDeltaPage, error) {
		return outlookDeltaPage{
			DeltaLink: "delta-1",
			Value: []outlookMessage{
				outlookTestMessage("msg-1", "One", "2026-01-02T03:04:05Z"),
				outlookTestMessage("msg-2", "Two", "2026-01-02T04:04:05Z"),
				outlookTestMessage("msg-3", "Three", "2026-01-02T05:04:05Z"),
			},
		}, nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
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
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "msg-2" {
		t.Fatalf("first checkpoint = %+v, want msg-2", first.Checkpoint)
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "msg-3" {
		t.Fatalf("resume documents = %+v, want msg-3", second.Documents)
	}
}

func TestOutlookConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newFixtureOutlookConnector()

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

func TestOutlookConnectorOpenSyncResumeRejectsMissingRemoteAnchor(t *testing.T) {
	connector := newFixtureOutlookConnector()
	connector.OutlookConnector.batchSize = 2
	connector.OutlookConnector.getDeltaPage = func(ctx context.Context, apiURL string) (outlookDeltaPage, error) {
		return outlookDeltaPage{
			DeltaLink: "delta-1",
			Value: []outlookMessage{
				outlookTestMessage("msg-1", "One", "2026-01-02T03:04:05Z"),
				outlookTestMessage("msg-2", "Two", "2026-01-02T04:04:05Z"),
				outlookTestMessage("msg-3", "Three", "2026-01-02T05:04:05Z"),
			},
		}, nil
	}

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "msg-2" {
		t.Fatalf("first checkpoint = %+v, want msg-2", first.Checkpoint)
	}

	connector.OutlookConnector.getDeltaPage = func(ctx context.Context, apiURL string) (outlookDeltaPage, error) {
		return outlookDeltaPage{
			DeltaLink: "delta-1",
			Value: []outlookMessage{
				outlookTestMessage("msg-1", "One", "2026-01-02T03:04:05Z"),
				outlookTestMessage("msg-3", "Three", "2026-01-02T05:04:05Z"),
			},
		}, nil
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	if _, err = resumed.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestOutlookConnectorOpenPrune(t *testing.T) {
	connector := newFixtureOutlookConnector()
	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 || batch.Documents[0].SourceID != "msg-1" || batch.Documents[1].SourceID != "old" {
		t.Fatalf("prune documents = %+v, want msg-1 and old", batch.Documents)
	}
}

func TestOutlookGetJSONRetriesTransientStatus(t *testing.T) {
	calls := 0
	connector := &OutlookConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		batchSize:    1,
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(`{"error":"try later"}`)), Header: http.Header{}}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"value":[]}`)), Header: http.Header{}}, nil
		})},
		acquireAccessToken: func(ctx context.Context) (string, error) { return "token", nil },
	}
	var page outlookDeltaPage
	if err := connector.getJSON(t.Context(), "https://graph.example.test/messages", &page); err != nil {
		t.Fatalf("getJSON failed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestOutlookGetJSONReadsBodyBeforeCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()

	connector := &OutlookConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		batchSize:    1,
		httpClient:   server.Client(),
		acquireAccessToken: func(ctx context.Context) (string, error) {
			return "token", nil
		},
	}
	var page outlookDeltaPage
	if err := connector.getJSON(t.Context(), server.URL+"/messages", &page); err != nil {
		t.Fatalf("getJSON failed: %v", err)
	}
}

func TestOutlookTokenRefreshesExpiredCachedToken(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tokenCalls := 0
	connector := &OutlookConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost || !strings.Contains(req.URL.Host, "login.microsoftonline.com") {
				t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
			}
			tokenCalls++
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"token-` + string(rune('0'+tokenCalls)) + `","expires_in":600}`)),
				Header:     http.Header{},
			}, nil
		})},
		now: func() time.Time { return now },
	}

	first, err := connector.token(context.Background())
	if err != nil {
		t.Fatalf("first token failed: %v", err)
	}
	if first != "token-1" {
		t.Fatalf("first token = %q, want token-1", first)
	}
	second, err := connector.token(context.Background())
	if err != nil {
		t.Fatalf("second token failed: %v", err)
	}
	if second != first || tokenCalls != 1 {
		t.Fatalf("cached token = %q, token calls = %d; want cached token-1 with one call", second, tokenCalls)
	}

	now = now.Add(6 * time.Minute)
	third, err := connector.token(context.Background())
	if err != nil {
		t.Fatalf("third token failed: %v", err)
	}
	if third != "token-2" || tokenCalls != 2 {
		t.Fatalf("refreshed token = %q, token calls = %d; want token-2 with two calls", third, tokenCalls)
	}
}

func TestOutlookGetJSONRefreshesTokenAfterUnauthorized(t *testing.T) {
	var tokenCalls int
	var graphCalls int
	authorizations := []string{}
	connector := &OutlookConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost {
				tokenCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"access_token":"token-` + string(rune('0'+tokenCalls)) + `","expires_in":3600}`)),
					Header:     http.Header{},
				}, nil
			}
			graphCalls++
			authorizations = append(authorizations, req.Header.Get("Authorization"))
			if graphCalls == 1 {
				return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":"expired"}`)), Header: http.Header{}}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"value":[]}`)), Header: http.Header{}}, nil
		})},
	}

	var page outlookDeltaPage
	if err := connector.getJSON(t.Context(), "https://graph.example.test/messages", &page); err != nil {
		t.Fatalf("getJSON failed: %v", err)
	}
	if tokenCalls != 2 {
		t.Fatalf("token calls = %d, want 2", tokenCalls)
	}
	if graphCalls != 2 {
		t.Fatalf("graph calls = %d, want 2", graphCalls)
	}
	wantAuthorizations := []string{"Bearer token-1", "Bearer token-2"}
	if len(authorizations) != len(wantAuthorizations) {
		t.Fatalf("authorizations = %v, want %v", authorizations, wantAuthorizations)
	}
	for i := range wantAuthorizations {
		if authorizations[i] != wantAuthorizations[i] {
			t.Fatalf("authorizations = %v, want %v", authorizations, wantAuthorizations)
		}
	}
}

func TestOutlookConnectorValidateConnectorSetting(t *testing.T) {
	connector := newFixtureOutlookConnector()
	var probed bool
	connector.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1.0/users/user-1" {
			t.Fatalf("path = %q, want /v1.0/users/user-1", req.URL.Path)
		}
		probed = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":"user-1"}`)),
		}, nil
	})}

	if err := connector.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
	if !probed {
		t.Fatalf("ValidateConnectorSetting did not probe Outlook user")
	}
}

type fixtureOutlookConnector struct {
	*OutlookConnector
}

func newFixtureOutlookConnector() *fixtureOutlookConnector {
	connector := &OutlookConnector{
		tenantID:     "tenant",
		clientID:     "client",
		clientSecret: "secret",
		folder:       "archive",
		userIDs:      []string{"user-1"},
		batchSize:    32,
		httpClient:   http.DefaultClient,
	}
	connector.acquireAccessToken = func(ctx context.Context) (string, error) {
		return "token", nil
	}
	connector.getDeltaPage = func(ctx context.Context, apiURL string) (outlookDeltaPage, error) {
		return outlookDeltaPage{
			DeltaLink: "delta-1",
			Value: []outlookMessage{
				outlookTestMessage("msg-1", "Hello/World", "2026-01-02T03:04:05Z"),
				{ID: "deleted", Removed: map[string]any{"reason": "deleted"}},
				outlookTestMessage("old", "Old", "2026-01-01T03:04:05Z"),
			},
		}, nil
	}
	return &fixtureOutlookConnector{OutlookConnector: connector}
}

func outlookTestMessage(id, subject, receivedAt string) outlookMessage {
	return outlookMessage{
		ID:               id,
		Subject:          subject,
		ReceivedDateTime: receivedAt,
		Body:             outlookMessageBody{ContentType: "html", Content: "<style>.x{}</style><p>Body <b>text</b></p>"},
		From:             outlookRecipientSlot{EmailAddress: outlookEmailAddress{Name: "Alice Example", Address: "alice@example.com"}},
		ToRecipients:     []outlookRecipient{{EmailAddress: outlookEmailAddress{Name: "Bob", Address: "bob@example.com"}}},
		CcRecipients:     []outlookRecipient{{EmailAddress: outlookEmailAddress{Name: "Carol", Address: "carol@example.com"}}},
		HasAttachments:   true,
		ConversationID:   "conv-1",
		WebLink:          "https://outlook.example.test/message/" + id,
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
