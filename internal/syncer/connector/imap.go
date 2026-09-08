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
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	netmail "net/mail"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message"
	// Register the charset decoder so text/* parts in non-UTF-8 charsets
	// (e.g. ISO-8859-1, Windows-1252) are decoded to UTF-8 on read.
	_ "github.com/emersion/go-message/charset"
	xhtml "golang.org/x/net/html"

	"ragflow/internal/utility"
)

const (
	defaultIMAPBatchSize     = 32
	defaultIMAPPort          = 993
	defaultIMAPSizeThreshold = 10 * 1024 * 1024
	imapDialTimeout          = 30 * time.Second
	imapCommandTimeout       = 30 * time.Second
)

// IMAPConnector reads email messages and attachments from an IMAP server.
type IMAPConnector struct {
	host          string
	port          int
	mailboxes     []string
	username      string
	password      string
	batchSize     int
	sizeThreshold int64

	dial func(ctx context.Context, host string, port int, username, password string) (imapClient, error)
}

// imapClient is the IMAP surface used by the connector. It is injected so unit
// tests can exercise the full connector without a live server.
type imapClient interface {
	List(ctx context.Context) ([]string, error)
	SelectMailbox(ctx context.Context, mailbox string) error
	Search(ctx context.Context, since, before time.Time) ([]uint32, error)
	Fetch(ctx context.Context, seqNum uint32) ([]byte, error)
	Close() error
}

// NewIMAPConnector creates an IMAP connector from the given config.
func NewIMAPConnector(config map[string]any) (*IMAPConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	threshold := int64(defaultIMAPSizeThreshold)
	if rawThreshold, err := strconv.ParseInt(os.Getenv("IMAP_CONNECTOR_SIZE_THRESHOLD"), 10, 64); err == nil && rawThreshold > 0 {
		threshold = rawThreshold
	}
	return &IMAPConnector{
		host:          strings.TrimSpace(stringConfig(config["imap_host"])),
		port:          configInt(config["imap_port"], defaultIMAPPort),
		mailboxes:     imapMailboxList(config["imap_mailbox"]),
		username:      strings.TrimSpace(stringConfig(credentials["imap_username"])),
		password:      stringConfig(credentials["imap_password"]),
		batchSize:     configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultIMAPBatchSize),
		sizeThreshold: threshold,
		dial:          dialRealIMAPClient,
	}, nil
}

// Validate validates IMAP connector settings and credentials.
func (c *IMAPConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("imap connector is nil")
	}
	if c.host == "" {
		return fmt.Errorf("Invalid connector settings: 'imap_host' must be provided")
	}
	if c.port <= 0 {
		return fmt.Errorf("Invalid connector settings: 'imap_port' must be a positive integer")
	}
	if c.username == "" {
		return fmt.Errorf("Missing imap_username in credentials")
	}
	if c.password == "" {
		return fmt.Errorf("Missing imap_password in credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	client, err := c.dial(ctx, c.host, c.port, c.username, c.password)
	if err != nil {
		return err
	}
	return client.Close()
}

// ValidateConnectorSetting validates IMAP settings from an unsaved config.
func (c *IMAPConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one IMAP sync session.
func (c *IMAPConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	client, err := c.dial(ctx, c.host, c.port, c.username, c.password)
	if err != nil {
		return nil, err
	}
	session := &imapSyncSession{
		connector:   c,
		client:      client,
		batchSize:   c.batchSize,
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
		hasMore:     true,
	}
	if err := session.applyResume(request.Resume); err != nil {
		_ = client.Close()
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete IMAP prune snapshot session.
func (c *IMAPConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	client, err := c.dial(ctx, c.host, c.port, c.username, c.password)
	if err != nil {
		return nil, err
	}
	return &imapPruneSession{connector: c, client: client, batchSize: c.batchSize, hasMore: true}, nil
}

type imapAttachment struct {
	filename    string
	contentType string
	content     []byte
}

type imapSyncSession struct {
	connector   *IMAPConnector
	client      imapClient
	batchSize   int
	windowStart *time.Time
	windowEnd   time.Time

	todoMailboxes   []string
	currentMailbox  string
	todoEmailIDs    []string
	selected        string
	hasMore         bool
	resumeValidated bool
}

// NextBatch returns the next IMAP document batch.
func (s *imapSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var lastDoc *SourceDocument
	for len(documents) < s.batchSize && s.hasMore {
		if err := s.ensureCurrentEmail(ctx); err != nil {
			return SyncBatch{}, err
		}
		if !s.hasMore {
			break
		}

		emailID := s.todoEmailIDs[0]
		s.todoEmailIDs = s.todoEmailIDs[1:]
		seq, err := strconv.ParseUint(emailID, 10, 32)
		if err != nil {
			continue
		}
		raw, err := s.client.Fetch(ctx, uint32(seq))
		if err != nil {
			return SyncBatch{}, err
		}
		emailDoc, attachments, err := parseIMAPMessage(raw, s.connector.sizeThreshold)
		if err != nil {
			log.Printf("imap: skip message seq %d in mailbox %q: %v", seq, s.currentMailbox, err)
			continue
		}
		if !s.inWindow(emailDoc.UpdatedAt) {
			continue
		}
		documents = append(documents, emailDoc)
		lastDoc = &emailDoc
		documents = append(documents, attachments...)
	}
	if len(documents) == 0 {
		return SyncBatch{}, io.EOF
	}
	return SyncBatch{Documents: documents, Checkpoint: s.checkpoint(lastDoc)}, nil
}

// Close closes the IMAP sync session.
func (s *imapSyncSession) Close() error {
	return s.client.Close()
}

// ensureCurrentEmail makes sure the session has a list of mailboxes and a
// current mailbox with remaining email IDs, advancing as needed.
func (s *imapSyncSession) ensureCurrentEmail(ctx context.Context) error {
	if err := s.validateResume(ctx); err != nil {
		return err
	}
	if s.todoMailboxes == nil {
		mailboxes, err := s.listMailboxes(ctx)
		if err != nil {
			return err
		}
		s.todoMailboxes = mailboxes
		if len(mailboxes) == 0 {
			s.hasMore = false
			return nil
		}
	}
	for {
		for s.currentMailbox == "" || len(s.todoEmailIDs) == 0 {
			if len(s.todoMailboxes) == 0 {
				s.hasMore = false
				return nil
			}
			mailbox := s.todoMailboxes[0]
			s.todoMailboxes = s.todoMailboxes[1:]
			emailIDs, err := s.searchMailbox(ctx, mailbox)
			if err != nil {
				return err
			}
			s.currentMailbox = mailbox
			s.todoEmailIDs = emailIDs
		}
		if s.selected == s.currentMailbox {
			return nil
		}
		if err := s.client.SelectMailbox(ctx, s.currentMailbox); err != nil {
			s.currentMailbox = ""
			s.todoEmailIDs = nil
			continue
		}
		s.selected = s.currentMailbox
		return nil
	}
}

func (s *imapSyncSession) validateResume(ctx context.Context) error {
	if s.resumeValidated || s.todoMailboxes == nil {
		return nil
	}
	mailboxes, err := s.listMailboxes(ctx)
	if err != nil {
		return err
	}
	current := make(map[string]struct{}, len(mailboxes))
	for _, mailbox := range mailboxes {
		current[mailbox] = struct{}{}
	}
	for _, mailbox := range s.todoMailboxes {
		if _, ok := current[mailbox]; !ok {
			return fmt.Errorf("imap resume mailbox %q was not found in the current listing: %w", mailbox, ErrSyncResumeInvalid)
		}
	}
	if s.currentMailbox != "" {
		if _, ok := current[s.currentMailbox]; !ok {
			return fmt.Errorf("imap resume mailbox %q was not found in the current listing: %w", s.currentMailbox, ErrSyncResumeInvalid)
		}
		emailIDs, err := s.searchMailbox(ctx, s.currentMailbox)
		if err != nil {
			return err
		}
		if !imapTodoEmailsMatch(s.todoEmailIDs, emailIDs) {
			return fmt.Errorf("imap resume email state no longer matches mailbox %q: %w", s.currentMailbox, ErrSyncResumeInvalid)
		}
	}
	s.resumeValidated = true
	return nil
}

func imapTodoEmailsMatch(todo, current []string) bool {
	if len(todo) > len(current) {
		return false
	}
	start := len(current) - len(todo)
	for index, emailID := range todo {
		if emailID != current[start+index] {
			return false
		}
	}
	return true
}

// listMailboxes returns configured mailboxes or discovers all mailboxes.
func (s *imapSyncSession) listMailboxes(ctx context.Context) ([]string, error) {
	if len(s.connector.mailboxes) > 0 {
		return s.connector.mailboxes, nil
	}
	mailboxes, err := s.client.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(mailboxes) == 0 {
		return []string{"INBOX"}, nil
	}
	return mailboxes, nil
}

// searchMailbox selects a mailbox and returns the email IDs in its window.
func (s *imapSyncSession) searchMailbox(ctx context.Context, mailbox string) ([]string, error) {
	if err := s.client.SelectMailbox(ctx, mailbox); err != nil {
		return nil, err
	}
	s.selected = mailbox
	start := time.Time{}
	if s.windowStart != nil {
		start = *s.windowStart
	}
	before := s.windowEnd.AddDate(0, 0, 1)
	nums, err := s.client.Search(ctx, start, before)
	if err != nil {
		return nil, err
	}
	emailIDs := make([]string, 0, len(nums))
	for _, num := range nums {
		emailIDs = append(emailIDs, strconv.FormatUint(uint64(num), 10))
	}
	return emailIDs, nil
}

func (s *imapSyncSession) inWindow(t time.Time) bool {
	start := time.Time{}
	if s.windowStart != nil {
		start = *s.windowStart
	}
	return t.After(start) && !t.After(s.windowEnd)
}

func (s *imapSyncSession) checkpoint(lastDoc *SourceDocument) *SyncCheckpoint {
	cursor := imapCursor{
		TodoMailboxes: s.todoMailboxes,
		HasMore:       s.hasMore,
	}
	if s.currentMailbox != "" {
		cursor.CurrentMailbox = &imapMailboxCursor{
			Mailbox:      s.currentMailbox,
			TodoEmailIDs: s.todoEmailIDs,
		}
	}
	data, _ := json.Marshal(cursor)
	checkpoint := &SyncCheckpoint{Cursor: string(data)}
	if lastDoc != nil {
		checkpoint.SourceID = lastDoc.SourceID
		updatedAt := lastDoc.UpdatedAt
		checkpoint.UpdatedAt = &updatedAt
	}
	return checkpoint
}

// applyResume restores a sync session from a previously committed checkpoint.
func (s *imapSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("imap sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor imapCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("imap sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	if !cursor.HasMore {
		return fmt.Errorf("imap sync checkpoint has no remaining work: %w", ErrSyncResumeInvalid)
	}
	if len(cursor.TodoMailboxes) == 0 && cursor.CurrentMailbox == nil {
		return fmt.Errorf("imap sync checkpoint has no mailbox anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.CurrentMailbox != nil {
		if cursor.CurrentMailbox.Mailbox == "" || len(cursor.CurrentMailbox.TodoEmailIDs) == 0 {
			return fmt.Errorf("imap sync checkpoint has no email anchor: %w", ErrSyncResumeInvalid)
		}
	}
	s.todoMailboxes = cursor.TodoMailboxes
	s.hasMore = cursor.HasMore
	if cursor.CurrentMailbox != nil {
		s.currentMailbox = cursor.CurrentMailbox.Mailbox
		s.todoEmailIDs = cursor.CurrentMailbox.TodoEmailIDs
	}
	s.resumeValidated = false
	return nil
}

type imapCursor struct {
	TodoMailboxes  []string           `json:"todo_mailboxes"`
	CurrentMailbox *imapMailboxCursor `json:"current_mailbox,omitempty"`
	HasMore        bool               `json:"has_more"`
}

type imapMailboxCursor struct {
	Mailbox      string   `json:"mailbox"`
	TodoEmailIDs []string `json:"todo_email_ids"`
}

type imapPruneSession struct {
	connector *IMAPConnector
	client    imapClient
	batchSize int

	todoMailboxes  []string
	currentMailbox string
	todoEmailIDs   []string
	hasMore        bool
	buffer         []SlimDocument
}

// NextBatch returns the next IMAP prune snapshot batch.
func (s *imapPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	for len(documents) < s.batchSize {
		if len(s.buffer) > 0 {
			n := min(s.batchSize-len(documents), len(s.buffer))
			documents = append(documents, s.buffer[:n]...)
			s.buffer = s.buffer[n:]
			continue
		}
		if !s.hasMore {
			break
		}
		if err := s.ensureCurrentEmail(ctx); err != nil {
			return PruneBatch{}, err
		}
		if !s.hasMore {
			break
		}
		emailID := s.todoEmailIDs[0]
		s.todoEmailIDs = s.todoEmailIDs[1:]
		seq, err := strconv.ParseUint(emailID, 10, 32)
		if err != nil {
			continue
		}
		raw, err := s.client.Fetch(ctx, uint32(seq))
		if err != nil {
			return PruneBatch{}, err
		}
		emailDoc, attachments, err := parseIMAPMessage(raw, s.connector.sizeThreshold)
		if err != nil {
			return PruneBatch{}, err
		}
		s.buffer = append(s.buffer, SlimDocument{SourceID: emailDoc.SourceID})
		for _, attachment := range attachments {
			s.buffer = append(s.buffer, SlimDocument{SourceID: attachment.SourceID})
		}
	}
	if len(documents) == 0 {
		return PruneBatch{}, io.EOF
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the IMAP prune session.
func (s *imapPruneSession) Close() error {
	return s.client.Close()
}

// Ensure the prune session has a current mailbox, listing mailboxes if needed.
func (s *imapPruneSession) ensureCurrentEmail(ctx context.Context) error {
	if s.todoMailboxes == nil {
		mailboxes, err := s.listMailboxes(ctx)
		if err != nil {
			return err
		}
		s.todoMailboxes = mailboxes
		if len(mailboxes) == 0 {
			s.hasMore = false
			return nil
		}
	}
	for s.currentMailbox == "" || len(s.todoEmailIDs) == 0 {
		if len(s.todoMailboxes) == 0 {
			s.currentMailbox = ""
			s.hasMore = false
			return nil
		}
		mailbox := s.todoMailboxes[0]
		s.todoMailboxes = s.todoMailboxes[1:]
		if err := s.client.SelectMailbox(ctx, mailbox); err != nil {
			return err
		}
		nums, err := s.client.Search(ctx, time.Time{}, time.Time{})
		if err != nil {
			return err
		}
		emailIDs := make([]string, 0, len(nums))
		for _, num := range nums {
			emailIDs = append(emailIDs, strconv.FormatUint(uint64(num), 10))
		}
		s.currentMailbox = mailbox
		s.todoEmailIDs = emailIDs
		s.hasMore = true
	}
	return nil
}

func (s *imapPruneSession) listMailboxes(ctx context.Context) ([]string, error) {
	if len(s.connector.mailboxes) > 0 {
		return s.connector.mailboxes, nil
	}
	mailboxes, err := s.client.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(mailboxes) == 0 {
		return []string{"INBOX"}, nil
	}
	return mailboxes, nil
}

// parseIMAPMessage parses a raw RFC 822 message into an email document and its
// attachment documents.
func parseIMAPMessage(raw []byte, sizeThreshold int64) (SourceDocument, []SourceDocument, error) {
	entity, err := message.Read(bytes.NewReader(raw))
	if err != nil {
		if !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err) {
			return SourceDocument{}, nil, err
		}
	}
	if entity == nil {
		return SourceDocument{}, nil, fmt.Errorf("imap: failed to read message")
	}

	body, attachments, err := walkIMAPParts(entity, sizeThreshold)
	if err != nil {
		return SourceDocument{}, nil, fmt.Errorf("imap: walk message parts: %w", err)
	}
	header := entity.Header
	subject := decodedIMAPHeader(header, "Subject")
	if subject == "" {
		subject = "Unknown Subject"
	}
	from := decodedIMAPHeader(header, "From")
	to := decodedIMAPHeader(header, "To")
	if to == "" {
		to = decodedIMAPHeader(header, "Delivered-To")
	}
	cc := decodedIMAPHeader(header, "Cc")
	dateString := decodedIMAPHeader(header, "Date")
	parsedDate := parseIMAPDate(dateString)
	date := parsedDate
	if date.IsZero() {
		date = time.Now().UTC()
	}

	messageID := decodedIMAPHeader(header, "Message-ID")
	if messageID == "" {
		messageID = buildGeneratedIMAPID(raw, subject, dateString, parsedDate, from, to, cc, body)
	}

	emailDoc := SourceDocument{
		SourceID:           messageID,
		SemanticIdentifier: subject,
		Extension:          ".txt",
		Blob:               []byte(body),
		UpdatedAt:          date,
		SizeBytes:          int64(len(body)),
		Fingerprint:        contentFingerprint([]byte(body)),
		Metadata:           map[string]any{},
	}

	attachmentDocs := make([]SourceDocument, 0, len(attachments))
	for index, attachment := range attachments {
		if utility.FilenameType(attachment.filename) == utility.FileTypeOTHER {
			continue
		}
		attachmentDocs = append(attachmentDocs, SourceDocument{
			SourceID:           messageID + "#att:" + strconv.Itoa(index) + ":" + attachment.filename,
			SemanticIdentifier: attachment.filename,
			Extension:          imapAttachmentExtension(attachment.filename),
			Blob:               attachment.content,
			UpdatedAt:          date,
			SizeBytes:          int64(len(attachment.content)),
			Fingerprint:        contentFingerprint(attachment.content),
			Metadata: map[string]any{
				"parent_email_id":         messageID,
				"parent_subject":          subject,
				"attachment_filename":     attachment.filename,
				"attachment_content_type": attachment.contentType,
			},
		})
	}
	return emailDoc, attachmentDocs, nil
}

// walkIMAPParts collects the first decodable text body and the attachments.
func walkIMAPParts(entity *message.Entity, sizeThreshold int64) (string, []imapAttachment, error) {
	var body string
	var htmlBody string
	var attachments []imapAttachment
	err := entity.Walk(func(path []int, part *message.Entity, partErr error) error {
		if partErr != nil {
			return partErr
		}
		if part == nil {
			return nil
		}
		if part.MultipartReader() != nil {
			return nil
		}
		disposition, dispositionParams, _ := part.Header.ContentDisposition()
		contentType, contentTypeParams, _ := part.Header.ContentType()
		dispositionLower := strings.ToLower(disposition)
		filename := firstNonEmpty(dispositionParams["filename"], contentTypeParams["name"])
		isAttachment := strings.HasPrefix(dispositionLower, "attachment") ||
			(strings.HasPrefix(dispositionLower, "inline") && filename != "")

		var payload []byte
		var err error
		if isAttachment {
			payload, err = io.ReadAll(io.LimitReader(part.Body, sizeThreshold+1))
		} else {
			payload, err = io.ReadAll(part.Body)
		}
		if err != nil {
			return err
		}

		if isAttachment {
			if len(payload) > 0 && int64(len(payload)) <= sizeThreshold {
				name := strings.TrimSpace(filename)
				if name == "" {
					name = "attachment.bin"
				}
				attachments = append(attachments, imapAttachment{
					filename:    name,
					contentType: contentType,
					content:     payload,
				})
			}
			// Walk only advances after the current part body is fully consumed;
			// drain anything beyond the capped read so the next part is reached.
			if _, err := io.Copy(io.Discard, part.Body); err != nil {
				return err
			}
			return nil
		}

		if !utf8.Valid(payload) {
			return nil
		}
		switch strings.ToLower(contentType) {
		case "text/plain":
			if body == "" {
				body = string(payload)
			}
		case "text/html":
			if htmlBody == "" {
				htmlBody = imapHTMLToText(string(payload))
			}
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if body == "" {
		body = htmlBody
	}
	return body, attachments, nil
}

func decodedIMAPHeader(header message.Header, key string) string {
	value, err := header.Text(key)
	if err != nil {
		value = header.Get(key)
	}
	return strings.TrimSpace(value)
}

func parseIMAPDate(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if parsed, err := netmail.ParseDate(value); err == nil {
		return parsed
	}
	for _, layout := range []string{time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func buildGeneratedIMAPID(raw []byte, subject, dateString string, parsedDate time.Time, from, to, cc, body string) string {
	if parsedDate.IsZero() {
		dateString = strings.TrimSpace(dateString)
	} else {
		dateString = parsedDate.UTC().Format("2006-01-02T15:04:05") + "+00:00"
	}
	rawDigest := sha256.Sum256(raw)
	bodyValue := []byte(body)
	bodyDigest := sha256.Sum256(bodyValue)
	material := strings.Join([]string{subject, dateString, from, to, cc,
		hex.EncodeToString(bodyDigest[:]), hex.EncodeToString(rawDigest[:])}, "\n")
	digest := sha256.Sum256([]byte(material))
	return "generated:" + hex.EncodeToString(digest[:])
}

func imapAttachmentExtension(filename string) string {
	if index := strings.LastIndex(filename, "."); index >= 0 {
		return filename[index:]
	}
	return ""
}

// imapHTMLToText flattens HTML into space-separated text.
func imapHTMLToText(value string) string {
	document, err := xhtml.Parse(strings.NewReader(value))
	if err != nil {
		return htmlToText(value)
	}
	parts := []string{}
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			switch strings.ToLower(node.Data) {
			case "script", "style":
				return
			}
		}
		if node.Type == xhtml.TextNode {
			if text := strings.TrimSpace(node.Data); text != "" {
				parts = append(parts, text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return strings.Join(parts, " ")
}

func imapMailboxList(value any) []string {
	switch typed := value.(type) {
	case []any:
		names := []string{}
		for _, item := range typed {
			if name := strings.TrimSpace(stringConfig(item)); name != "" {
				names = append(names, name)
			}
		}
		return names
	default:
		return splitCommaList(stringConfig(value))
	}
}

type realIMAPClient struct {
	client *imapclient.Client
}

type imapCommandResult[T any] struct {
	value T
	err   error
}

func runIMAPCommand[T any](ctx context.Context, client *imapclient.Client, run func() (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, imapCommandTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	done := make(chan imapCommandResult[T], 1)
	go func() {
		value, err := run()
		done <- imapCommandResult[T]{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = client.Close()
		var zero T
		return zero, ctx.Err()
	case result := <-done:
		return result.value, result.err
	}
}

func dialRealIMAPClient(ctx context.Context, host string, port int, username, password string) (imapClient, error) {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialCtx, cancelDial := context.WithTimeout(ctx, imapDialTimeout)
	defer cancelDial()
	rawConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName: host,
		NextProtos: []string{"imap"},
	})
	if err := tlsConn.HandshakeContext(dialCtx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	client := imapclient.New(tlsConn, nil)
	if _, err := runIMAPCommand(ctx, client, func() (struct{}, error) {
		return struct{}{}, client.Login(username, password).Wait()
	}); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// runIMAPCommand already closed the client on timeout; skip LOGOUT.
			_ = client.Close()
		} else {
			_ = client.Logout().Wait()
			_ = client.Close()
		}
		return nil, err
	}
	return &realIMAPClient{client: client}, nil
}

func (c *realIMAPClient) List(ctx context.Context) ([]string, error) {
	listed, err := runIMAPCommand(ctx, c.client, func() ([]*imap.ListData, error) {
		return c.client.List("", "*", nil).Collect()
	})
	if err != nil {
		return nil, err
	}
	mailboxes := []string{}
	for _, data := range listed {
		if data == nil || data.Mailbox == "" {
			continue
		}
		mailboxes = append(mailboxes, data.Mailbox)
	}
	return mailboxes, nil
}

func (c *realIMAPClient) SelectMailbox(ctx context.Context, mailbox string) error {
	// Send a real SELECT, not EXAMINE. go-imap emits EXAMINE when ReadOnly
	// is set, which some servers acknowledge without entering the selected
	// state. Fetches already use Peek, so a writable SELECT is read-safe.
	_, err := runIMAPCommand(ctx, c.client, func() (*imap.SelectData, error) {
		return c.client.Select(mailbox, nil).Wait()
	})
	return err
}

func (c *realIMAPClient) Search(ctx context.Context, since, before time.Time) ([]uint32, error) {
	criteria := &imap.SearchCriteria{}
	if !since.IsZero() {
		criteria.Since = since
	}
	if !before.IsZero() {
		criteria.Before = before
	}
	data, err := runIMAPCommand(ctx, c.client, func() (*imap.SearchData, error) {
		return c.client.Search(criteria, nil).Wait()
	})
	if err != nil {
		return nil, err
	}
	return data.AllSeqNums(), nil
}

func (c *realIMAPClient) Fetch(ctx context.Context, seqNum uint32) ([]byte, error) {
	buffers, err := runIMAPCommand(ctx, c.client, func() ([]*imapclient.FetchMessageBuffer, error) {
		return c.client.Fetch(imap.SeqSetNum(seqNum), &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{{Peek: true}},
		}).Collect()
	})
	if err != nil {
		return nil, err
	}
	for _, buffer := range buffers {
		if buffer == nil {
			continue
		}
		for _, section := range buffer.BodySection {
			if len(section.Bytes) > 0 {
				return section.Bytes, nil
			}
		}
	}
	return nil, nil
}

func (c *realIMAPClient) Close() error {
	_, logoutErr := runIMAPCommand(context.Background(), c.client, func() (struct{}, error) {
		return struct{}{}, c.client.Logout().Wait()
	})
	// Always close the connection even when LOGOUT times out or fails.
	_ = c.client.Close()
	return logoutErr
}
