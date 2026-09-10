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
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultGmailBatchSize = 32
	gmailItemsPerPage     = 100
	gmailRequestTimeout   = 60 * time.Second
	gmailOAuthTokenURL    = "https://oauth2.googleapis.com/token"
)

var gmailScopes = []string{
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/admin.directory.user.readonly",
	"https://www.googleapis.com/auth/admin.directory.group.readonly",
}

// GmailConnector reads Gmail threads from a Workspace domain or one Gmail account.
type GmailConnector struct {
	primaryAdminEmail string
	credentials       map[string]any
	batchSize         int
	clientMu          sync.Mutex
	clients           map[string]*http.Client
	httpClientForUser func(ctx context.Context, userEmail string) (*http.Client, error)
	listUsers         func(ctx context.Context) ([]string, error)
	listThreadPage    func(ctx context.Context, userEmail, query, pageToken string, pageSize int) (gmailThreadListPage, error)
	getThread         func(ctx context.Context, userEmail, threadID string) (gmailThread, error)
}

// NewGmailConnector creates a Gmail connector from Python-compatible config.
func NewGmailConnector(config map[string]any) (*GmailConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	return &GmailConnector{
		primaryAdminEmail: strings.TrimSpace(stringConfig(credentials["google_primary_admin"])),
		credentials:       credentials,
		batchSize:         configInt(config["batch_size"], defaultGmailBatchSize),
		clients:           map[string]*http.Client{},
	}, nil
}

// Validate validates Gmail connector settings and credentials.
func (c *GmailConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("gmail connector is nil")
	}
	if c.primaryAdminEmail == "" {
		return fmt.Errorf("Gmail connector is missing google_primary_admin")
	}
	if len(c.credentials) == 0 {
		return fmt.Errorf("Gmail connector is missing credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	_, err := c.getUserEmails(ctx)
	return err
}

// ValidateConnectorSetting validates Gmail settings from an unsaved config.
func (c *GmailConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Gmail sync session.
func (c *GmailConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	users, err := c.getUserEmails(ctx)
	if err != nil {
		return nil, err
	}
	query := ""
	if !request.FromBeginning && request.WindowStart != nil {
		query = gmailTimeRangeQuery(request.WindowStart, request.WindowEnd)
	}
	session := &gmailSyncSession{
		connector: c,
		users:     users,
		batchSize: c.batchSize,
		query:     query,
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Gmail prune snapshot session.
func (c *GmailConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	users, err := c.getUserEmails(ctx)
	if err != nil {
		return nil, err
	}
	return &gmailPruneSession{connector: c, users: users, batchSize: c.batchSize}, nil
}

// getUserEmails returns Workspace users or falls back to the primary account for personal Gmail.
func (c *GmailConnector) getUserEmails(ctx context.Context) ([]string, error) {
	if c.listUsers != nil {
		return c.listUsers(ctx)
	}
	client, err := c.clientForUser(ctx, c.primaryAdminEmail)
	if err != nil {
		return nil, err
	}

	domain := c.primaryAdminEmail
	if _, after, ok := strings.Cut(c.primaryAdminEmail, "@"); ok {
		domain = after
	}
	users := []string{}
	pageToken := ""
	for {
		query := url.Values{
			"domain":     {domain},
			"fields":     {"nextPageToken,users(primaryEmail)"},
			"maxResults": {"500"},
		}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var page gmailUsersPage
		err = c.getJSON(ctx, client, "https://admin.googleapis.com/admin/directory/v1/users?"+query.Encode(), &page)
		if err != nil {
			if httpErr, ok := err.(googleHTTPError); ok && httpErr.status == http.StatusNotFound {
				return []string{c.primaryAdminEmail}, nil
			}
			return nil, err
		}
		for _, user := range page.Users {
			if user.PrimaryEmail != "" {
				users = append(users, user.PrimaryEmail)
			}
		}
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	if len(users) == 0 {
		return []string{c.primaryAdminEmail}, nil
	}
	return users, nil
}

// listThreads returns one Gmail thread list page.
func (c *GmailConnector) listThreads(ctx context.Context, userEmail, queryText, pageToken string, pageSize int) (gmailThreadListPage, error) {
	if c.listThreadPage != nil {
		return c.listThreadPage(ctx, userEmail, queryText, pageToken, pageSize)
	}
	client, err := c.clientForUser(ctx, userEmail)
	if err != nil {
		return gmailThreadListPage{}, err
	}
	query := url.Values{
		"fields":     {"nextPageToken,threads(id)"},
		"maxResults": {fmt.Sprint(pageSize)},
	}
	if queryText != "" {
		query.Set("q", queryText)
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	var page gmailThreadListPage
	err = c.getJSON(ctx, client, "https://gmail.googleapis.com/gmail/v1/users/"+url.PathEscape(userEmail)+"/threads?"+query.Encode(), &page)
	return page, err
}

// loadThread returns one full Gmail thread.
func (c *GmailConnector) loadThread(ctx context.Context, userEmail, threadID string) (gmailThread, error) {
	if c.getThread != nil {
		return c.getThread(ctx, userEmail, threadID)
	}
	client, err := c.clientForUser(ctx, userEmail)
	if err != nil {
		return gmailThread{}, err
	}
	query := url.Values{"fields": {"id,messages(id,labelIds,payload(headers,parts(body(data),mimeType)))"}}
	var thread gmailThread
	err = c.getJSON(ctx, client, "https://gmail.googleapis.com/gmail/v1/users/"+url.PathEscape(userEmail)+"/threads/"+url.PathEscape(threadID)+"?"+query.Encode(), &thread)
	return thread, err
}

// clientForUser builds an authenticated Google client for a user.
func (c *GmailConnector) clientForUser(ctx context.Context, userEmail string) (*http.Client, error) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.clients == nil {
		c.clients = map[string]*http.Client{}
	}
	if client := c.clients[userEmail]; client != nil {
		return client, nil
	}

	var client *http.Client
	var err error
	if c.httpClientForUser != nil {
		client, err = c.httpClientForUser(ctx, userEmail)
	} else {
		var tokenSource oauth2.TokenSource
		tokenSource, err = c.tokenSource(ctx, userEmail)
		if err == nil {
			client = oauth2.NewClient(ctx, tokenSource)
		}
	}
	if err != nil {
		return nil, err
	}
	c.clients[userEmail] = client
	return client, nil
}

// tokenSource returns a Google OAuth token source for stored connector credentials.
func (c *GmailConnector) tokenSource(ctx context.Context, userEmail string) (oauth2.TokenSource, error) {
	if value := stringConfig(c.credentials["google_service_account_key"]); value != "" {
		config, err := google.JWTConfigFromJSON([]byte(value), gmailScopes...)
		if err != nil {
			return nil, err
		}
		config.Subject = userEmail
		return config.TokenSource(ctx), nil
	}

	tokenJSON := stringConfig(c.credentials["google_tokens"])
	if tokenJSON == "" {
		return nil, fmt.Errorf("Gmail connector credentials must include google_tokens or google_service_account_key")
	}
	var tokenData gmailOAuthToken
	if err := json.Unmarshal([]byte(tokenJSON), &tokenData); err != nil {
		return nil, err
	}
	if tokenData.ClientID == "" {
		tokenData.ClientID = os.Getenv("OAUTH_GOOGLE_DRIVE_CLIENT_ID")
	}
	if tokenData.ClientSecret == "" {
		tokenData.ClientSecret = os.Getenv("OAUTH_GOOGLE_DRIVE_CLIENT_SECRET")
	}
	if tokenData.ClientID == "" || tokenData.ClientSecret == "" || tokenData.RefreshToken == "" {
		return nil, fmt.Errorf("Gmail OAuth credentials are incomplete")
	}
	return (&oauth2.Config{
		ClientID:     tokenData.ClientID,
		ClientSecret: tokenData.ClientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: gmailOAuthTokenURL},
		Scopes:       gmailScopes,
	}).TokenSource(ctx, tokenData.token()), nil
}

// getJSON fetches and decodes a Google JSON API response.
func (c *GmailConnector) getJSON(ctx context.Context, client *http.Client, apiURL string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, gmailRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return googleHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type gmailSyncSession struct {
	connector       *GmailConnector
	users           []string
	userIndex       int
	pageToken       string
	batchSize       int
	query           string
	buffer          []gmailBufferedDocument
	resumePageToken string
	resumeOffset    int
	resumeSourceID  string
}

// NextBatch returns the next Gmail document batch.
func (s *gmailSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		for _, buffered := range s.buffer[:n] {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.userIndex >= len(s.users) {
			if len(documents) == 0 {
				return SyncBatch{}, io.EOF
			}
			break
		}
		batch, err := s.nextDocumentPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		remaining := s.batchSize - len(documents)
		if len(batch) > remaining {
			for _, buffered := range batch[:remaining] {
				documents = append(documents, buffered.document)
				checkpoint = buffered.checkpoint
			}
			s.buffer = append(s.buffer, batch[remaining:]...)
			break
		}
		for _, buffered := range batch {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the Gmail sync session.
func (s *gmailSyncSession) Close() error {
	return nil
}

// nextDocumentPage fetches one Gmail list page and expands threads.
func (s *gmailSyncSession) nextDocumentPage(ctx context.Context) ([]gmailBufferedDocument, error) {
	userEmail := s.users[s.userIndex]
	requestPageToken := s.pageToken
	page, err := s.connector.listThreads(ctx, userEmail, s.query, requestPageToken, gmailItemsPerPage)
	if err != nil {
		if isGmailDisabled(err) {
			s.advanceUser()
			return nil, nil
		}
		return nil, err
	}
	candidates := make([]gmailBufferedDocument, 0, len(page.Threads))
	pageOffset := 0
	for _, item := range page.Threads {
		thread, err := s.connector.loadThread(ctx, userEmail, item.ID)
		if err != nil {
			if isGmailDisabled(err) || isGoogleForbiddenOrNotFound(err) {
				continue
			}
			return nil, err
		}
		doc, ok := thread.toSourceDocument(userEmail)
		if ok {
			pageOffset++
			candidates = append(candidates, gmailBufferedDocument{
				document:   doc,
				checkpoint: gmailSyncCheckpoint(userEmail, requestPageToken, pageOffset, doc),
				offset:     pageOffset,
			})
		}
	}
	documents, err := s.filterResumedDocuments(requestPageToken, candidates)
	if err != nil {
		return nil, err
	}
	if page.NextPageToken == "" {
		s.advanceUser()
	} else {
		s.pageToken = page.NextPageToken
	}
	return documents, nil
}

// applyResume restores the Gmail source anchor when a checkpoint is available.
func (s *gmailSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("gmail sync cursor is missing: %w", ErrSyncResumeInvalid)
	}

	var cursor gmailSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("gmail sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" {
		return fmt.Errorf("gmail sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if cursor.UserEmail == "" {
		return fmt.Errorf("gmail sync cursor has no user anchor: %w", ErrSyncResumeInvalid)
	}
	for index, userEmail := range s.users {
		if userEmail != cursor.UserEmail {
			continue
		}
		s.userIndex = index
		s.pageToken = cursor.PageToken
		s.resumePageToken = cursor.PageToken
		s.resumeSourceID = sourceID
		if cursor.Offset > 0 {
			s.resumeOffset = cursor.Offset
		}
		return nil
	}
	return fmt.Errorf("gmail resume user %q was not found in the current listing: %w", cursor.UserEmail, ErrSyncResumeInvalid)
}

// filterResumedDocuments continues after the committed Gmail source anchor.
func (s *gmailSyncSession) filterResumedDocuments(pageToken string, candidates []gmailBufferedDocument) ([]gmailBufferedDocument, error) {
	if s.resumeSourceID == "" {
		return candidates, nil
	}
	if pageToken != s.resumePageToken {
		return nil, fmt.Errorf("gmail resume page %q no longer matches checkpoint page %q: %w", pageToken, s.resumePageToken, ErrSyncResumeInvalid)
	}
	for index, candidate := range candidates {
		if candidate.document.SourceID == s.resumeSourceID {
			s.clearResumeOffset()
			return candidates[index+1:], nil
		}
	}
	return nil, fmt.Errorf("gmail resume anchor %q was not found on page %q: %w", s.resumeSourceID, pageToken, ErrSyncResumeInvalid)
}

func (s *gmailSyncSession) clearResumeOffset() {
	s.resumeOffset = 0
	s.resumePageToken = ""
	s.resumeSourceID = ""
}

// advanceUser moves a Gmail session to the next mailbox.
func (s *gmailSyncSession) advanceUser() {
	s.userIndex++
	s.pageToken = ""
	s.clearResumeOffset()
}

type gmailBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
}

type gmailSyncCursor struct {
	UserEmail string `json:"user_email"`
	PageToken string `json:"page_token,omitempty"`
	Offset    int    `json:"offset"`
	SourceID  string `json:"source_id,omitempty"`
}

func gmailSyncCheckpoint(userEmail, pageToken string, offset int, doc SourceDocument) *SyncCheckpoint {
	cursor, err := json.Marshal(gmailSyncCursor{UserEmail: userEmail, PageToken: pageToken, Offset: offset, SourceID: doc.SourceID})
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    string(cursor),
		UpdatedAt: &updatedAt,
		SourceID:  doc.SourceID,
	}
}

type gmailPruneSession struct {
	connector *GmailConnector
	users     []string
	userIndex int
	pageToken string
	batchSize int
	buffer    []SlimDocument
}

// NextBatch returns the next Gmail prune snapshot batch.
func (s *gmailPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.userIndex >= len(s.users) {
			if len(documents) == 0 {
				return PruneBatch{}, io.EOF
			}
			break
		}
		batch, err := s.nextSlimPage(ctx)
		if err != nil {
			return PruneBatch{}, err
		}
		remaining := s.batchSize - len(documents)
		if len(batch) > remaining {
			documents = append(documents, batch[:remaining]...)
			s.buffer = append(s.buffer, batch[remaining:]...)
			break
		}
		documents = append(documents, batch...)
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the Gmail prune session.
func (s *gmailPruneSession) Close() error {
	return nil
}

// nextSlimPage fetches one Gmail thread ID page.
func (s *gmailPruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	userEmail := s.users[s.userIndex]
	page, err := s.connector.listThreads(ctx, userEmail, "", s.pageToken, gmailItemsPerPage)
	if err != nil {
		if isGmailDisabled(err) {
			s.advanceUser()
			return nil, nil
		}
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(page.Threads))
	for _, thread := range page.Threads {
		if thread.ID != "" {
			documents = append(documents, SlimDocument{SourceID: thread.ID})
		}
	}
	if page.NextPageToken == "" {
		s.advanceUser()
	} else {
		s.pageToken = page.NextPageToken
	}
	return documents, nil
}

// advanceUser moves a Gmail prune session to the next mailbox.
func (s *gmailPruneSession) advanceUser() {
	s.userIndex++
	s.pageToken = ""
}

type gmailUsersPage struct {
	NextPageToken string `json:"nextPageToken"`
	Users         []struct {
		PrimaryEmail string `json:"primaryEmail"`
	} `json:"users"`
}

type gmailThreadListPage struct {
	NextPageToken string `json:"nextPageToken"`
	Threads       []struct {
		ID string `json:"id"`
	} `json:"threads"`
}

type gmailThread struct {
	ID       string         `json:"id"`
	Messages []gmailMessage `json:"messages"`
}

// toSourceDocument converts a Gmail thread into the syncer model.
func (t gmailThread) toSourceDocument(userEmail string) (SourceDocument, bool) {
	if t.ID == "" || len(t.Messages) == 0 {
		return SourceDocument{}, false
	}
	sections := make([]string, 0, len(t.Messages))
	semanticIdentifier := ""
	updatedAt := time.Time{}
	metadata := map[string]any{}
	fromEmails := map[string]string{}
	otherEmails := map[string]string{}

	for _, message := range t.Messages {
		body, messageMetadata := message.toTextSection()
		if body != "" {
			sections = append(sections, body)
		}
		for key, value := range messageMetadata {
			metadata[key] = value
			if isGmailEmailHeader(key) {
				email, name := parseGmailAddress(stringConfig(value))
				if email == "" {
					continue
				}
				if key == "from" {
					fromEmails[email] = name
				} else {
					otherEmails[email] = name
				}
			}
		}
		if semanticIdentifier == "" {
			semanticIdentifier = sanitizeGmailName(stringConfig(messageMetadata["subject"]))
		}
		if value := stringConfig(messageMetadata["updated_at"]); value != "" {
			if parsed, err := mail.ParseDate(value); err == nil && parsed.UTC().After(updatedAt) {
				updatedAt = parsed.UTC()
			}
		}
	}
	if semanticIdentifier == "" {
		semanticIdentifier = "(no subject)"
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	blob := []byte(strings.Join(sections, "\n\n"))
	metadata["external_user_emails"] = []string{userEmail}
	metadata["primary_owners"] = gmailOwnersMetadata(fromEmails)
	metadata["secondary_owners"] = gmailOwnersMetadata(otherEmails)
	return SourceDocument{
		SourceID:           t.ID,
		SemanticIdentifier: semanticIdentifier,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}, true
}

type gmailMessage struct {
	ID       string       `json:"id"`
	LabelIDs []string     `json:"labelIds"`
	Payload  gmailPayload `json:"payload"`
}

// toTextSection converts a Gmail message to text and metadata.
func (m gmailMessage) toTextSection() (string, map[string]any) {
	metadata := map[string]any{}
	for _, header := range m.Payload.Headers {
		name := strings.ToLower(header.Name)
		if isGmailEmailHeader(name) {
			metadata[name] = header.Value
		}
		if name == "subject" {
			metadata["subject"] = header.Value
		}
		if name == "date" {
			metadata["updated_at"] = header.Value
		}
	}
	if len(m.LabelIDs) > 0 {
		metadata["labels"] = m.LabelIDs
	}

	var builder strings.Builder
	builder.WriteString(gmailPayloadText(m.Payload))
	for _, key := range []string{"from", "to", "cc", "bcc", "subject", "labels"} {
		if value, ok := metadata[key]; ok {
			builder.WriteString(fmt.Sprintf("%s: %v\n", key, value))
		}
	}
	return builder.String(), metadata
}

type gmailPayload struct {
	Headers  []gmailHeader `json:"headers"`
	Parts    []gmailPart   `json:"parts"`
	Body     gmailBody     `json:"body"`
	MimeType string        `json:"mimeType"`
}

type gmailPart struct {
	MimeType string      `json:"mimeType"`
	Body     gmailBody   `json:"body"`
	Parts    []gmailPart `json:"parts"`
}

type gmailBody struct {
	Data string `json:"data"`
}

type gmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// gmailPayloadText extracts text/plain body content from a Gmail payload.
func gmailPayloadText(payload gmailPayload) string {
	var builder strings.Builder
	if payload.MimeType == "text/plain" && payload.Body.Data != "" {
		builder.WriteString(decodeGmailBody(payload.Body.Data))
	}
	for _, part := range payload.Parts {
		builder.WriteString(gmailPartText(part))
	}
	return builder.String()
}

// gmailPartText extracts text/plain content recursively.
func gmailPartText(part gmailPart) string {
	var builder strings.Builder
	if part.MimeType == "text/plain" && part.Body.Data != "" {
		builder.WriteString(decodeGmailBody(part.Body.Data))
	}
	for _, child := range part.Parts {
		builder.WriteString(gmailPartText(child))
	}
	return builder.String()
}

// decodeGmailBody decodes Gmail's URL-safe base64 body format.
func decodeGmailBody(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return string(decoded)
	}
	if decoded, err := base64.URLEncoding.DecodeString(value); err == nil {
		return string(decoded)
	}
	return ""
}

// gmailTimeRangeQuery builds Python-compatible Gmail time range query text.
func gmailTimeRangeQuery(windowStart *time.Time, windowEnd time.Time) string {
	parts := []string{}
	if windowStart != nil && !windowStart.IsZero() {
		parts = append(parts, fmt.Sprintf("after:%d", windowStart.Unix()+1))
	}
	if !windowEnd.IsZero() {
		parts = append(parts, fmt.Sprintf("before:%d", windowEnd.Unix()))
	}
	return strings.Join(parts, " ")
}

// isGmailEmailHeader reports whether a metadata key is an email header of interest.
func isGmailEmailHeader(name string) bool {
	switch name {
	case "cc", "bcc", "from", "to":
		return true
	default:
		return false
	}
}

// parseGmailAddress extracts an email address and display name.
func parseGmailAddress(value string) (string, string) {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return strings.TrimSpace(value), ""
	}
	return address.Address, address.Name
}

// gmailOwnersMetadata converts email owners to compact metadata.
func gmailOwnersMetadata(owners map[string]string) []map[string]string {
	out := make([]map[string]string, 0, len(owners))
	emails := make([]string, 0, len(owners))
	for email := range owners {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	for _, email := range emails {
		name := owners[email]
		item := map[string]string{"email": email}
		if name != "" {
			parts := strings.Fields(name)
			if len(parts) > 1 {
				item["first_name"] = strings.Join(parts[:len(parts)-1], " ")
				item["last_name"] = parts[len(parts)-1]
			} else {
				item["last_name"] = name
			}
		}
		out = append(out, item)
	}
	return out
}

// sanitizeGmailName mirrors Python's sanitized filename intent.
func sanitizeGmailName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", `"`, "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(name)
}

// isGmailDisabled reports mailbox-not-provisioned Gmail failures.
func isGmailDisabled(err error) bool {
	if httpErr, ok := err.(googleHTTPError); ok {
		return httpErr.status == http.StatusBadRequest && (strings.Contains(httpErr.body, "Mail service not enabled") || strings.Contains(httpErr.body, "failedPrecondition"))
	}
	return false
}

// isGoogleForbiddenOrNotFound reports item-level Google visibility failures.
func isGoogleForbiddenOrNotFound(err error) bool {
	return isGooglePermissionDeniedOrNotFound(err)
}

func isGooglePermissionDeniedOrNotFound(err error) bool {
	if httpErr, ok := err.(googleHTTPError); ok {
		if httpErr.status == http.StatusNotFound {
			return true
		}
		return httpErr.status == http.StatusForbidden && !isGoogleRateLimited(err)
	}
	return false
}

func isGoogleRateLimited(err error) bool {
	httpErr, ok := err.(googleHTTPError)
	if !ok {
		return false
	}
	if httpErr.status == http.StatusTooManyRequests {
		return true
	}
	if httpErr.status != http.StatusForbidden {
		return false
	}
	if strings.Contains(httpErr.body, "rateLimitExceeded") || strings.Contains(httpErr.body, "userRateLimitExceeded") || strings.Contains(httpErr.body, "quotaExceeded") {
		return true
	}
	for _, reason := range googleErrorReasons(httpErr.body) {
		switch reason {
		case "rateLimitExceeded", "userRateLimitExceeded", "quotaExceeded", "dailyLimitExceeded", "RESOURCE_EXHAUSTED":
		}
	}
	return false
}

func googleErrorReasons(body string) []string {
	var response struct {
		Error struct {
			Errors []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
			Status string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return nil
	}
	reasons := make([]string, 0, len(response.Error.Errors)+1)
	for _, item := range response.Error.Errors {
		if item.Reason != "" {
			reasons = append(reasons, item.Reason)
		}
	}
	if response.Error.Status != "" {
		reasons = append(reasons, response.Error.Status)
	}
	return reasons
}

type googleHTTPError struct {
	status int
	body   string
}

// Error returns the Google API error text.
func (e googleHTTPError) Error() string {
	return fmt.Sprintf("Google API returned HTTP %d: %s", e.status, e.body)
}

type gmailOAuthToken struct {
	AccessToken  string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Expiry       string `json:"expiry"`
}

// token converts stored OAuth JSON to oauth2.Token.
func (t gmailOAuthToken) token() *oauth2.Token {
	token := &oauth2.Token{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken}
	if t.Expiry != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, t.Expiry); err == nil {
			token.Expiry = parsed
		}
	}
	return token
}
