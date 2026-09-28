package connector

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestGmailConnectorOpenSync verifies Gmail thread expansion and incremental query construction.
func TestGmailConnectorOpenSync(t *testing.T) {
	connector := newFixtureGmailConnector()
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-03T00:00:00Z")

	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if connector.lastQuery != "after:1767312001 before:1767398400" {
		t.Fatalf("query = %q", connector.lastQuery)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "thread-1" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Hello_World" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	blob := string(doc.Blob)
	if !strings.Contains(blob, "Body text") ||
		!strings.Contains(blob, "from: Alice Example <alice@example.com>") ||
		!strings.Contains(blob, "to: Bob <bob@example.com>") ||
		!strings.Contains(blob, "subject: Hello/World") {
		t.Fatalf("blob = %q", string(doc.Blob))
	}
	if !doc.UpdatedAt.Equal(mustTime(t, "2026-01-02T03:04:05Z")) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Metadata["external_user_emails"].([]string)[0] != "admin@example.com" {
		t.Fatalf("external user metadata = %v", doc.Metadata["external_user_emails"])
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestGmailConnectorOpenSyncResumesWithinPage verifies Gmail checkpoint resumes inside a list page.
func TestGmailConnectorOpenSyncResumesWithinPage(t *testing.T) {
	connector := newFixtureGmailConnector()
	connector.GmailConnector.batchSize = 2
	connector.GmailConnector.listThreadPage = func(ctx context.Context, userEmail, query, pageToken string, pageSize int) (gmailThreadListPage, error) {
		return gmailThreadListPage{Threads: []struct {
			ID string `json:"id"`
		}{{ID: "thread-1"}, {ID: "thread-2"}, {ID: "thread-3"}}}, nil
	}
	connector.GmailConnector.getThread = func(ctx context.Context, userEmail, threadID string) (gmailThread, error) {
		return gmailTestThread(threadID, "Fri, 02 Jan 2026 03:04:05 +0000")
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
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "thread-2" {
		t.Fatalf("first checkpoint = %+v, want thread-2", first.Checkpoint)
	}

	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	second, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resume NextBatch failed: %v", err)
	}
	if len(second.Documents) != 1 || second.Documents[0].SourceID != "thread-3" {
		t.Fatalf("resume documents = %+v, want thread-3", second.Documents)
	}
	if second.Checkpoint == nil || second.Checkpoint.SourceID != "thread-3" {
		t.Fatalf("resume checkpoint = %+v, want thread-3", second.Checkpoint)
	}
}

func TestGmailConnectorOpenSyncResumeRejectsMissingCheckpoint(t *testing.T) {
	connector := newFixtureGmailConnector()

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{}})
	if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
	}
}

// TestGmailConnectorResumeRejectsMissingRemoteAnchor verifies a deleted list item invalidates the saved anchor.
func TestGmailConnectorResumeRejectsMissingRemoteAnchor(t *testing.T) {
	connector := newFixtureGmailConnector()
	connector.GmailConnector.batchSize = 2
	connector.GmailConnector.listThreadPage = func(ctx context.Context, userEmail, query, pageToken string, pageSize int) (gmailThreadListPage, error) {
		return gmailThreadListPage{Threads: []struct {
			ID string `json:"id"`
		}{{ID: "thread-1"}, {ID: "thread-2"}, {ID: "thread-3"}}}, nil
	}
	connector.GmailConnector.getThread = func(ctx context.Context, userEmail, threadID string) (gmailThread, error) {
		return gmailTestThreadWithSubject(threadID, threadID, "Fri, 02 Jan 2026 03:04:05 +0000")
	}
	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if first.Checkpoint == nil || first.Checkpoint.SourceID != "thread-2" {
		t.Fatalf("first checkpoint = %+v, want thread-2", first.Checkpoint)
	}

	resumeListCalls := 0
	connector.GmailConnector.listThreadPage = func(ctx context.Context, userEmail, query, pageToken string, pageSize int) (gmailThreadListPage, error) {
		resumeListCalls++
		if pageToken != "" {
			t.Fatalf("resume used unexpected pageToken=%q", pageToken)
		}
		return gmailThreadListPage{Threads: []struct {
			ID string `json:"id"`
		}{{ID: "thread-1"}, {ID: "thread-3"}}}, nil
	}
	resumed, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync failed: %v", err)
	}
	_, err = resumed.NextBatch(context.Background())
	if err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
	if resumeListCalls == 0 {
		t.Fatalf("resume should have listed the current remote page")
	}
}

// TestGmailFingerprintStable verifies Gmail fingerprints are stable and content-sensitive.
func TestGmailFingerprintStable(t *testing.T) {
	thread := gmailThread{
		ID: "thread-1",
		Messages: []gmailMessage{{
			ID: "msg-1",
			Payload: gmailPayload{
				Headers: []gmailHeader{
					{Name: "From", Value: "Alice Example <alice@example.com>"},
					{Name: "To", Value: "Bob <bob@example.com>"},
					{Name: "Cc", Value: "Carol <carol@example.com>"},
					{Name: "Subject", Value: "Hello"},
					{Name: "Date", Value: "Fri, 02 Jan 2026 03:04:05 +0000"},
				},
				Parts: []gmailPart{{
					MimeType: "text/plain",
					Body:     gmailBody{Data: base64.RawURLEncoding.EncodeToString([]byte("Body text"))},
				}},
			},
		}},
	}
	doc1, ok := thread.toSourceDocument("admin@example.com")
	if !ok {
		t.Fatalf("thread did not produce document")
	}
	doc2, ok := thread.toSourceDocument("admin@example.com")
	if !ok {
		t.Fatalf("thread did not produce second document")
	}
	if doc1.Fingerprint == "" || doc1.Fingerprint != doc2.Fingerprint {
		t.Fatalf("fingerprint unstable: %q %q", doc1.Fingerprint, doc2.Fingerprint)
	}
	if doc1.Fingerprint != contentFingerprint(doc1.Blob) {
		t.Fatalf("fingerprint = %q, want content fingerprint", doc1.Fingerprint)
	}

	changed := thread
	changed.Messages = append([]gmailMessage(nil), thread.Messages...)
	changed.Messages[0].Payload.Headers = append([]gmailHeader(nil), thread.Messages[0].Payload.Headers...)
	changed.Messages[0].Payload.Headers[3].Value = "Hello v2"
	doc3, ok := changed.toSourceDocument("admin@example.com")
	if !ok {
		t.Fatalf("changed thread did not produce document")
	}
	if doc3.Fingerprint == doc1.Fingerprint {
		t.Fatalf("fingerprint did not change after subject update")
	}
}

// TestGmailOwnersMetadataSorted verifies owner metadata is deterministic.
func TestGmailOwnersMetadataSorted(t *testing.T) {
	owners := gmailOwnersMetadata(map[string]string{
		"carol@example.com": "Carol Example",
		"alice@example.com": "Alice Example",
	})
	if len(owners) != 2 {
		t.Fatalf("owners len = %d, want 2", len(owners))
	}
	if owners[0]["email"] != "alice@example.com" || owners[1]["email"] != "carol@example.com" {
		t.Fatalf("owners not sorted: %+v", owners)
	}
}

// TestGmailConnectorOpenPrune verifies Gmail prune emits thread IDs only.
func TestGmailConnectorOpenPrune(t *testing.T) {
	connector := newFixtureGmailConnector()
	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "thread-1" {
		t.Fatalf("unexpected prune batch: %+v", batch.Documents)
	}
}

// TestGmailConnectorClientForUserCachesByMailbox verifies clients are reused per user.
func TestGmailConnectorClientForUserCachesByMailbox(t *testing.T) {
	connector := &GmailConnector{}
	calls := map[string]int{}
	connector.httpClientForUser = func(ctx context.Context, userEmail string) (*http.Client, error) {
		calls[userEmail]++
		return &http.Client{}, nil
	}

	first, err := connector.clientForUser(t.Context(), "a@example.com")
	if err != nil {
		t.Fatalf("first client: %v", err)
	}
	second, err := connector.clientForUser(t.Context(), "a@example.com")
	if err != nil {
		t.Fatalf("second client: %v", err)
	}
	other, err := connector.clientForUser(t.Context(), "b@example.com")
	if err != nil {
		t.Fatalf("other client: %v", err)
	}
	if first != second {
		t.Fatalf("same mailbox did not reuse client")
	}
	if first == other {
		t.Fatalf("different mailboxes shared client")
	}
	if calls["a@example.com"] != 1 || calls["b@example.com"] != 1 {
		t.Fatalf("client creation calls = %+v", calls)
	}
}

type fixtureGmailConnector struct {
	*GmailConnector
	lastQuery string
}

func newFixtureGmailConnector() *fixtureGmailConnector {
	base, _ := NewGmailConnector(map[string]any{
		"batch_size": 1,
		"credentials": map[string]any{
			"google_primary_admin": "admin@example.com",
			"google_tokens":        `{"client_id":"client","client_secret":"secret","refresh_token":"refresh"}`,
		},
	})
	connector := &fixtureGmailConnector{GmailConnector: base}
	base.listUsers = func(ctx context.Context) ([]string, error) {
		return []string{"admin@example.com"}, nil
	}
	base.listThreadPage = func(ctx context.Context, userEmail, query, pageToken string, pageSize int) (gmailThreadListPage, error) {
		connector.lastQuery = query
		return gmailThreadListPage{Threads: []struct {
			ID string `json:"id"`
		}{{ID: "thread-1"}}}, nil
	}
	base.getThread = func(ctx context.Context, userEmail, threadID string) (gmailThread, error) {
		return gmailTestThread(threadID, "Fri, 02 Jan 2026 03:04:05 +0000")
	}
	return connector
}

func gmailTestThread(threadID, date string) (gmailThread, error) {
	return gmailTestThreadWithSubject(threadID, "Hello/World", date)
}

func gmailTestThreadWithSubject(threadID, subject, date string) (gmailThread, error) {
	return gmailThread{
		ID: threadID,
		Messages: []gmailMessage{{
			ID: "msg-1",
			Payload: gmailPayload{
				Headers: []gmailHeader{
					{Name: "From", Value: "Alice Example <alice@example.com>"},
					{Name: "To", Value: "Bob <bob@example.com>"},
					{Name: "Subject", Value: subject},
					{Name: "Date", Value: date},
				},
				Parts: []gmailPart{{
					MimeType: "text/plain",
					Body:     gmailBody{Data: base64.RawURLEncoding.EncodeToString([]byte("Body text"))},
				}},
			},
		}},
	}, nil
}

// TestGmailTimeRangeQuery verifies Python-compatible after and before seconds.
func TestGmailTimeRangeQuery(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	end := time.Unix(200, 0).UTC()
	if got := gmailTimeRangeQuery(&start, end); got != "after:101 before:200" {
		t.Fatalf("query = %q", got)
	}
}
