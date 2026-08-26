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
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type imapSearchCall struct {
	since  time.Time
	before time.Time
}

type fakeIMAPClient struct {
	mailboxes       []string
	current         string
	selected        []string
	searchByMailbox map[string][]uint32
	searchCalls     []imapSearchCall
	rawBySeq        map[uint32][]byte
	fetched         []uint32
	closed          bool
	listErr         error
	searchErr       error
	selectErrs      map[string]error
}

func (f *fakeIMAPClient) List(ctx context.Context) ([]string, error) {
	return f.mailboxes, f.listErr
}

func (f *fakeIMAPClient) SelectMailbox(ctx context.Context, mailbox string) error {
	f.selected = append(f.selected, mailbox)
	f.current = mailbox
	if f.selectErrs != nil {
		if err := f.selectErrs[mailbox]; err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeIMAPClient) Search(ctx context.Context, since, before time.Time) ([]uint32, error) {
	f.searchCalls = append(f.searchCalls, imapSearchCall{since: since, before: before})
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.searchByMailbox[f.current], nil
}

func (f *fakeIMAPClient) Fetch(ctx context.Context, seqNum uint32) ([]byte, error) {
	f.fetched = append(f.fetched, seqNum)
	return f.rawBySeq[seqNum], nil
}

func (f *fakeIMAPClient) Close() error {
	f.closed = true
	return nil
}

func rawIMAPEmail(id, subject, date, body string) []byte {
	var b strings.Builder
	b.WriteString("From: Sender <sender@example.com>\r\n")
	b.WriteString("To: Recipient <recipient@example.com>\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + date + "\r\n")
	b.WriteString("Message-ID: <" + id + "@example.com>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(body + "\r\n")
	return []byte(b.String())
}

func multipartIMAPEmail(id, subject, date, body, attachmentName string, attachmentContent []byte) []byte {
	var b strings.Builder
	b.WriteString("From: Sender <sender@example.com>\r\n")
	b.WriteString("To: Recipient <recipient@example.com>\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + date + "\r\n")
	b.WriteString("Message-ID: <" + id + "@example.com>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"BOUNDARY\"\r\n\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(body + "\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: application/pdf\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + attachmentName + "\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64.StdEncoding.EncodeToString(attachmentContent) + "\r\n")
	b.WriteString("--BOUNDARY--\r\n")
	return []byte(b.String())
}

func multipartIMAPEmailWithCharset(id, subject, date, charset string, body []byte) []byte {
	var b strings.Builder
	b.WriteString("From: Sender <sender@example.com>\r\n")
	b.WriteString("To: Recipient <recipient@example.com>\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + date + "\r\n")
	b.WriteString("Message-ID: <" + id + "@example.com>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"BOUNDARY\"\r\n\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: text/plain; charset=" + charset + "\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.Write(body)
	b.WriteString("\r\n--BOUNDARY--\r\n")
	return []byte(b.String())
}

func newTestIMAPConnector(client *fakeIMAPClient, batchSize int) *IMAPConnector {
	return &IMAPConnector{
		host:          "imap.example.com",
		port:          993,
		mailboxes:     []string{"INBOX"},
		username:      "user",
		password:      "pass",
		batchSize:     batchSize,
		sizeThreshold: defaultIMAPSizeThreshold,
		dial: func(ctx context.Context, host string, port int, username, password string) (imapClient, error) {
			return client, nil
		},
	}
}

func TestNewIMAPConnectorParsesConfig(t *testing.T) {
	connector, err := NewIMAPConnector(map[string]any{
		"imap_host":       " imap.example.com ",
		"imap_port":       993,
		"imap_mailbox":    []any{"INBOX", ""},
		"sync_batch_size": 7,
		"credentials": map[string]any{
			"imap_username": "user",
			"imap_password": "pass",
		},
	})
	if err != nil {
		t.Fatalf("NewIMAPConnector failed: %v", err)
	}
	if connector.host != "imap.example.com" {
		t.Fatalf("host = %q", connector.host)
	}
	if connector.port != 993 {
		t.Fatalf("port = %d", connector.port)
	}
	if len(connector.mailboxes) != 1 || connector.mailboxes[0] != "INBOX" {
		t.Fatalf("mailboxes = %v", connector.mailboxes)
	}
	if connector.batchSize != 7 {
		t.Fatalf("batch_size = %d", connector.batchSize)
	}
	if connector.username != "user" || connector.password != "pass" {
		t.Fatalf("credentials = %q %q", connector.username, connector.password)
	}
}

func TestIMAPValidate(t *testing.T) {
	client := &fakeIMAPClient{}
	connector := newTestIMAPConnector(client, 32)
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if !client.closed {
		t.Fatalf("expected the IMAP client to be closed after validation")
	}

	missing := newTestIMAPConnector(&fakeIMAPClient{}, 32)
	missing.host = ""
	if err := missing.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "imap_host") {
		t.Fatalf("Validate error = %v, want missing imap_host", err)
	}
}

func TestIMAPSyncInitialBatchAndResume(t *testing.T) {
	start := mustTime(t, "2026-01-02T12:00:00Z")
	end := mustTime(t, "2026-01-03T00:00:00Z")
	client := &fakeIMAPClient{
		mailboxes: []string{"INBOX"},
		searchByMailbox: map[string][]uint32{
			"INBOX": {1, 2},
		},
		rawBySeq: map[uint32][]byte{
			1: rawIMAPEmail("msg1", "Hello one", "Mon, 2 Jan 2026 12:30:00 +0000", "body one"),
			2: rawIMAPEmail("msg2", "Hello two", "Mon, 2 Jan 2026 22:00:00 +0000", "body two"),
		},
	}
	connector := newTestIMAPConnector(client, 1)

	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch1, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first NextBatch failed: %v", err)
	}
	if len(batch1.Documents) != 1 || batch1.Documents[0].SourceID != "<msg1@example.com>" {
		t.Fatalf("batch1 documents = %v", batch1.Documents)
	}
	if batch1.Checkpoint == nil || batch1.Checkpoint.Cursor == "" {
		t.Fatalf("batch1 checkpoint is nil")
	}
	if len(client.searchCalls) != 1 {
		t.Fatalf("searchCalls = %d, want 1", len(client.searchCalls))
	}
	if !client.searchCalls[0].since.Equal(start) {
		t.Fatalf("search since = %v, want %v", client.searchCalls[0].since, start)
	}
	wantBefore := end.AddDate(0, 0, 1)
	if !client.searchCalls[0].before.Equal(wantBefore) {
		t.Fatalf("search before = %v, want %v", client.searchCalls[0].before, wantBefore)
	}

	var cursor imapCursor
	if err := json.Unmarshal([]byte(batch1.Checkpoint.Cursor), &cursor); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	if cursor.CurrentMailbox == nil || cursor.CurrentMailbox.Mailbox != "INBOX" || len(cursor.CurrentMailbox.TodoEmailIDs) != 1 || cursor.CurrentMailbox.TodoEmailIDs[0] != "2" {
		t.Fatalf("cursor = %+v", cursor)
	}

	client.fetched = nil
	session2, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
		Resume:      batch1.Checkpoint,
	})
	if err != nil {
		t.Fatalf("OpenSync on resume failed: %v", err)
	}
	batch2, err := session2.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch failed: %v", err)
	}
	if len(batch2.Documents) != 1 || batch2.Documents[0].SourceID != "<msg2@example.com>" {
		t.Fatalf("resumed documents = %v", batch2.Documents)
	}
	if len(client.fetched) != 1 || client.fetched[0] != 2 {
		t.Fatalf("resumed fetch = %v, want [2]", client.fetched)
	}
}

func TestIMAPSyncResumeSelectsMailboxBeforeFetch(t *testing.T) {
	start := mustTime(t, "2026-01-02T12:00:00Z")
	end := mustTime(t, "2026-01-03T00:00:00Z")
	cursorData, err := json.Marshal(imapCursor{
		TodoMailboxes: []string{},
		HasMore:       true,
		CurrentMailbox: &imapMailboxCursor{
			Mailbox:      "INBOX",
			TodoEmailIDs: []string{"2"},
		},
	})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}

	// Use a fresh connection so the resumed session can only rely on its own
	// select call before fetching, matching a real resumed IMAP connection.
	client := &fakeIMAPClient{
		mailboxes: []string{"INBOX"},
		searchByMailbox: map[string][]uint32{
			"INBOX": {2},
		},
		rawBySeq: map[uint32][]byte{
			2: rawIMAPEmail("msg2", "Hello two", "Mon, 2 Jan 2026 22:00:00 +0000", "body two"),
		},
	}
	connector := newTestIMAPConnector(client, 32)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
		Resume:      &SyncCheckpoint{Cursor: string(cursorData)},
	})
	if err != nil {
		t.Fatalf("OpenSync on resume failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch failed: %v", err)
	}
	if len(client.selected) == 0 || client.selected[0] != "INBOX" {
		t.Fatalf("resumed select = %v, want INBOX selected before fetch", client.selected)
	}
	if len(client.fetched) != 1 || client.fetched[0] != 2 {
		t.Fatalf("resumed fetch = %v, want [2]", client.fetched)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "<msg2@example.com>" {
		t.Fatalf("resumed documents = %v", batch.Documents)
	}
}

func TestIMAPSyncResumeRejectsInvalidCheckpoint(t *testing.T) {
	client := &fakeIMAPClient{}
	connector := newTestIMAPConnector(client, 32)
	cases := map[string]*SyncCheckpoint{
		"missing":    {},
		"malformed":  {Cursor: "not-json"},
		"no-mailbox": {Cursor: `{"todo_mailboxes":[],"has_more":true}`},
		"no-email":   {Cursor: `{"todo_mailboxes":[],"has_more":true,"current_mailbox":{"mailbox":"INBOX","todo_email_ids":[]}}`},
		"completed":  {Cursor: `{"todo_mailboxes":["INBOX"],"has_more":false}`},
	}
	for name, checkpoint := range cases {
		t.Run(name, func(t *testing.T) {
			session, err := connector.OpenSync(context.Background(), SyncRequest{
				FromBeginning: true,
				Resume:        checkpoint,
			})
			if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("resume OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
			}
		})
	}
}

func TestIMAPSyncResumeRejectsStaleMailbox(t *testing.T) {
	cursorData, err := json.Marshal(imapCursor{
		TodoMailboxes: []string{"MISSING"},
		HasMore:       true,
	})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	connector := newTestIMAPConnector(&fakeIMAPClient{mailboxes: []string{"INBOX"}}, 32)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursorData)},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestIMAPSyncResumeRejectsStaleEmailState(t *testing.T) {
	cursorData, err := json.Marshal(imapCursor{
		TodoMailboxes: []string{},
		HasMore:       true,
		CurrentMailbox: &imapMailboxCursor{
			Mailbox:      "INBOX",
			TodoEmailIDs: []string{"3"},
		},
	})
	if err != nil {
		t.Fatalf("marshal cursor: %v", err)
	}
	client := &fakeIMAPClient{
		mailboxes: []string{"INBOX"},
		searchByMailbox: map[string][]uint32{
			"INBOX": {1, 2},
		},
	}
	connector := newTestIMAPConnector(client, 32)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		FromBeginning: true,
		Resume:        &SyncCheckpoint{Cursor: string(cursorData)},
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	defer session.Close()
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("resume NextBatch err = %v, want ErrSyncResumeInvalid", err)
	}
}

func TestIMAPSyncFiltersOutsideWindow(t *testing.T) {
	start := mustTime(t, "2026-01-02T00:00:00Z")
	end := mustTime(t, "2026-01-03T00:00:00Z")
	client := &fakeIMAPClient{
		mailboxes: []string{"INBOX"},
		searchByMailbox: map[string][]uint32{
			"INBOX": {1},
		},
		rawBySeq: map[uint32][]byte{
			1: rawIMAPEmail("old", "Old mail", "Thu, 1 Jan 2026 09:00:00 +0000", "old"),
		},
	}
	connector := newTestIMAPConnector(client, 32)
	session, err := connector.OpenSync(context.Background(), SyncRequest{
		WindowStart: &start,
		WindowEnd:   end,
	})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	if _, err := session.NextBatch(context.Background()); err != io.EOF {
		t.Fatalf("NextBatch error = %v, want io.EOF", err)
	}
}

func TestIMAPParseMessageWithAttachment(t *testing.T) {
	attachment := []byte("%PDF-1.4 fake")
	raw := multipartIMAPEmail("msg3", "With att", "Mon, 2 Jan 2026 09:00:00 +0000", "Hello body", "report.pdf", attachment)
	emailDoc, attachments, err := parseIMAPMessage(raw, defaultIMAPSizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if emailDoc.SourceID != "<msg3@example.com>" {
		t.Fatalf("email SourceID = %q", emailDoc.SourceID)
	}
	if string(emailDoc.Blob) != "Hello body" {
		t.Fatalf("email body = %q", emailDoc.Blob)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	attr := attachments[0]
	if attr.SourceID != "<msg3@example.com>#att:0:report.pdf" {
		t.Fatalf("attachment SourceID = %q", attr.SourceID)
	}
	if attr.Extension != ".pdf" {
		t.Fatalf("attachment extension = %q", attr.Extension)
	}
	if string(attr.Blob) != string(attachment) {
		t.Fatalf("attachment content mismatch")
	}
}

func TestIMAPParseMessageSkipsUnsupportedAttachment(t *testing.T) {
	var b strings.Builder
	b.WriteString("From: Sender <sender@example.com>\r\n")
	b.WriteString("To: Recipient <recipient@example.com>\r\n")
	b.WriteString("Subject: Mixed att\r\n")
	b.WriteString("Date: Mon, 2 Jan 2026 09:00:00 +0000\r\n")
	b.WriteString("Message-ID: <msg4@example.com>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"BOUNDARY\"\r\n\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString("Hello body\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: application/pdf\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"report.pdf\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fake")) + "\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: application/zip\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"archive.zip\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64.StdEncoding.EncodeToString([]byte("PK\x03\x04fake")) + "\r\n")
	b.WriteString("--BOUNDARY--\r\n")

	emailDoc, attachments, err := parseIMAPMessage([]byte(b.String()), defaultIMAPSizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if emailDoc.SourceID != "<msg4@example.com>" {
		t.Fatalf("email SourceID = %q", emailDoc.SourceID)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
	attr := attachments[0]
	if attr.SourceID != "<msg4@example.com>#att:0:report.pdf" {
		t.Fatalf("attachment SourceID = %q", attr.SourceID)
	}
	if attr.Extension != ".pdf" {
		t.Fatalf("attachment extension = %q", attr.Extension)
	}
}

func TestIMAPParseMessageAttachmentSourceIDsUnique(t *testing.T) {
	var b strings.Builder
	b.WriteString("From: Sender <sender@example.com>\r\n")
	b.WriteString("To: Recipient <recipient@example.com>\r\n")
	b.WriteString("Subject: Duplicate att names\r\n")
	b.WriteString("Date: Mon, 2 Jan 2026 09:00:00 +0000\r\n")
	b.WriteString("Message-ID: <msg9@example.com>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"BOUNDARY\"\r\n\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString("Hello body\r\n")
	for _, content := range []string{"%PDF-1.4 first", "%PDF-1.4 second"} {
		b.WriteString("--BOUNDARY\r\n")
		b.WriteString("Content-Type: application/pdf\r\n")
		b.WriteString("Content-Disposition: attachment; filename=\"report.pdf\"\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(content)) + "\r\n")
	}
	b.WriteString("--BOUNDARY--\r\n")

	emailDoc, attachments, err := parseIMAPMessage([]byte(b.String()), defaultIMAPSizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if emailDoc.SourceID != "<msg9@example.com>" {
		t.Fatalf("email SourceID = %q", emailDoc.SourceID)
	}
	if len(attachments) != 2 {
		t.Fatalf("attachments = %d, want 2", len(attachments))
	}
	want := []string{"<msg9@example.com>#att:0:report.pdf", "<msg9@example.com>#att:1:report.pdf"}
	for i, attr := range attachments {
		if attr.SourceID != want[i] {
			t.Fatalf("attachment %d SourceID = %q, want %q", i, attr.SourceID, want[i])
		}
	}
}

func TestIMAPParseMessageIgnoresOversizedTextAttachment(t *testing.T) {
	sizeThreshold := int64(16)
	var b strings.Builder
	b.WriteString("From: Sender <sender@example.com>\r\n")
	b.WriteString("To: Recipient <recipient@example.com>\r\n")
	b.WriteString("Subject: Oversized text attachment\r\n")
	b.WriteString("Date: Mon, 2 Jan 2026 09:00:00 +0000\r\n")
	b.WriteString("Message-ID: <msg5@example.com>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"BOUNDARY\"\r\n\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"oversized.txt\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(strings.Repeat("x", 1024) + "\r\n")
	b.WriteString("--BOUNDARY\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString("Hello body\r\n")
	b.WriteString("--BOUNDARY--\r\n")

	emailDoc, attachments, err := parseIMAPMessage([]byte(b.String()), sizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if string(emailDoc.Blob) != "Hello body" {
		t.Fatalf("email body = %q, want %q", emailDoc.Blob, "Hello body")
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments = %d, want 0 oversized text attachments", len(attachments))
	}
}

func TestIMAPParseMessageDecodesISO88591Body(t *testing.T) {
	raw := multipartIMAPEmailWithCharset("msg6", "Latin body", "Mon, 2 Jan 2026 09:00:00 +0000", "iso-8859-1", []byte("caf\xe9"))
	emailDoc, attachments, err := parseIMAPMessage(raw, defaultIMAPSizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if string(emailDoc.Blob) != "caf\u00e9" {
		t.Fatalf("email body = %q, want %q", emailDoc.Blob, "caf\u00e9")
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments = %d, want 0", len(attachments))
	}
}

func TestIMAPParseMessageDecodesWindows1252Body(t *testing.T) {
	raw := multipartIMAPEmailWithCharset("msg7", "Euro body", "Mon, 2 Jan 2026 09:00:00 +0000", "windows-1252", []byte("price: 5\x80"))
	emailDoc, attachments, err := parseIMAPMessage(raw, defaultIMAPSizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if string(emailDoc.Blob) != "price: 5\u20ac" {
		t.Fatalf("email body = %q, want %q", emailDoc.Blob, "price: 5\u20ac")
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments = %d, want 0", len(attachments))
	}
}

func TestIMAPParseMessageKeepsBodyOverAttachmentThreshold(t *testing.T) {
	sizeThreshold := int64(16)
	body := strings.Repeat("y", 64)
	raw := multipartIMAPEmailWithCharset("msg8", "Long body", "Mon, 2 Jan 2026 09:00:00 +0000", "utf-8", []byte(body))
	emailDoc, attachments, err := parseIMAPMessage(raw, sizeThreshold)
	if err != nil {
		t.Fatalf("parseIMAPMessage failed: %v", err)
	}
	if string(emailDoc.Blob) != body {
		t.Fatalf("email body = %q, want %q", emailDoc.Blob, body)
	}
	if len(attachments) != 0 {
		t.Fatalf("attachments = %d, want 0", len(attachments))
	}
}

func TestIMAPPruneReturnsSlimDocs(t *testing.T) {
	client := &fakeIMAPClient{
		mailboxes: []string{"INBOX"},
		searchByMailbox: map[string][]uint32{
			"INBOX": {1, 2},
		},
		rawBySeq: map[uint32][]byte{
			1: rawIMAPEmail("p1", "Prune one", "Mon, 2 Jan 2026 10:00:00 +0000", "one"),
			2: rawIMAPEmail("p2", "Prune two", "Mon, 2 Jan 2026 11:00:00 +0000", "two"),
		},
	}
	connector := newTestIMAPConnector(client, 1)
	prune, err := connector.OpenPrune(context.Background(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}

	batch1, err := prune.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("first prune batch failed: %v", err)
	}
	if len(batch1.Documents) != 1 || batch1.Documents[0].SourceID != "<p1@example.com>" {
		t.Fatalf("prune batch1 = %v", batch1.Documents)
	}
	batch2, err := prune.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("second prune batch failed: %v", err)
	}
	if len(batch2.Documents) != 1 || batch2.Documents[0].SourceID != "<p2@example.com>" {
		t.Fatalf("prune batch2 = %v", batch2.Documents)
	}
	if _, err := prune.NextBatch(context.Background()); err != io.EOF {
		t.Fatalf("final prune batch error = %v, want io.EOF", err)
	}
	if len(client.fetched) != 2 || client.fetched[0] != 1 || client.fetched[1] != 2 {
		t.Fatalf("prune fetch = %v, want [1 2]", client.fetched)
	}
}
