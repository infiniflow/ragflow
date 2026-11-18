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
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultOutlookBatchSize  = 32
	defaultOutlookFolder     = "inbox"
	outlookGraphBase         = "https://graph.microsoft.com/v1.0"
	outlookTokenURLFormat    = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	outlookGraphScope        = "https://graph.microsoft.com/.default"
	outlookRequestTimeout    = 60 * time.Second
	outlookRetryCount        = 4
	outlookRetryBaseDelay    = 200 * time.Millisecond
	outlookTokenExpiryMargin = 5 * time.Minute
)

var (
	outlookHTMLScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	outlookHTMLTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	outlookWhitespaceRE      = regexp.MustCompile(`[ \t]+`)
	outlookNewlineRE         = regexp.MustCompile(`\n{3,}`)
)

// OutlookConnector reads Microsoft 365 Outlook messages through Microsoft Graph.
type OutlookConnector struct {
	tenantID     string
	clientID     string
	clientSecret string
	folder       string
	userIDs      []string
	batchSize    int

	clientMu    sync.Mutex
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	now         func() time.Time

	acquireAccessToken func(ctx context.Context) (string, error)
	listUsers          func(ctx context.Context) ([]string, error)
	getDeltaPage       func(ctx context.Context, apiURL string) (outlookDeltaPage, error)
}

// NewOutlookConnector creates an Outlook connector from Python-compatible config.
func NewOutlookConnector(config map[string]any) (*OutlookConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	return &OutlookConnector{
		tenantID:     strings.TrimSpace(stringConfig(credentials["tenant_id"])),
		clientID:     strings.TrimSpace(stringConfig(credentials["client_id"])),
		clientSecret: stringConfig(credentials["client_secret"]),
		folder:       firstNonEmpty(stringConfig(config["folder"]), defaultOutlookFolder),
		userIDs:      splitCommaList(firstNonEmpty(stringConfig(config["user_ids"]), stringConfig(config["user_id"]))),
		batchSize:    configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultOutlookBatchSize),
		httpClient:   http.DefaultClient,
	}, nil
}

// Validate validates Outlook connector settings and credentials.
func (c *OutlookConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("outlook connector is nil")
	}
	if c.tenantID == "" || c.clientID == "" || c.clientSecret == "" {
		return fmt.Errorf("Outlook credentials are incomplete: tenant_id, client_id, and client_secret are required")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if c.folder == "" {
		return fmt.Errorf("Outlook folder is required")
	}
	if _, err := c.token(ctx); err != nil {
		return err
	}
	if len(c.userIDs) > 0 {
		return c.probeUser(ctx, c.userIDs[0])
	}
	_, err := c.users(ctx)
	return err
}

// ValidateConnectorSetting validates Outlook settings from an unsaved config.
func (c *OutlookConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

func (c *OutlookConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return defaultOutlookBatchSize
}

// OpenSync opens one Outlook sync session.
func (c *OutlookConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	users, err := c.users(ctx)
	if err != nil {
		return nil, err
	}
	session := &outlookSyncSession{
		connector:   c,
		users:       users,
		batchSize:   c.effectiveBatchSize(),
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
		deltaLinks:  map[string]string{},
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Outlook prune snapshot session.
func (c *OutlookConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	users, err := c.users(ctx)
	if err != nil {
		return nil, err
	}
	return &outlookPruneSession{connector: c, users: users, batchSize: c.effectiveBatchSize()}, nil
}

func (c *OutlookConnector) users(ctx context.Context) ([]string, error) {
	if len(c.userIDs) > 0 {
		return uniqueSorted(c.userIDs), nil
	}
	if c.listUsers != nil {
		return c.listUsers(ctx)
	}
	users := []string{}
	apiURL := outlookGraphBase + "/users?$select=id,userPrincipalName,mail"
	for apiURL != "" {
		var page outlookUsersPage
		if err := c.getJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		for _, user := range page.Value {
			if user.ID != "" && (user.Mail != "" || user.UserPrincipalName != "") {
				users = append(users, user.ID)
			}
		}
		apiURL = page.NextLink
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("Outlook connector found no mailbox users")
	}
	return users, nil
}

func (c *OutlookConnector) probeUser(ctx context.Context, userID string) error {
	apiURL := outlookGraphBase + "/users/" + url.PathEscape(userID) + "?$select=id"
	var out struct {
		ID string `json:"id"`
	}
	return c.getJSON(ctx, apiURL, &out)
}

func (c *OutlookConnector) deltaPage(ctx context.Context, apiURL string) (outlookDeltaPage, error) {
	if c.getDeltaPage != nil {
		return c.getDeltaPage(ctx, apiURL)
	}
	var page outlookDeltaPage
	err := c.getJSON(ctx, apiURL, &page)
	return page, err
}

func (c *OutlookConnector) getJSON(ctx context.Context, apiURL string, out any) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}

	var lastErr error
	retriedUnauthorized := false
	for attempt := 1; attempt <= outlookRetryCount; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, outlookRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, apiURL, nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
			resp.Body.Close()
			cancel()
			if resp.StatusCode < 400 {
				if readErr != nil {
					return readErr
				}
				return json.Unmarshal(body, out)
			}
			lastErr = outlookHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(bytes.TrimSpace(body)))}
			if resp.StatusCode == http.StatusUnauthorized && !retriedUnauthorized {
				c.invalidateToken(token)
				token, err = c.token(ctx)
				if err != nil {
					return err
				}
				retriedUnauthorized = true
				attempt--
				continue
			}
			if !isOutlookRetryable(resp.StatusCode) {
				return lastErr
			}
		}
		if attempt == outlookRetryCount {
			break
		}
		delay := time.Duration(attempt) * outlookRetryBaseDelay
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func (c *OutlookConnector) token(ctx context.Context) (string, error) {
	c.clientMu.Lock()
	if c.accessToken != "" && !c.cachedTokenExpiredLocked() {
		token := c.accessToken
		c.clientMu.Unlock()
		return token, nil
	}
	c.clientMu.Unlock()

	var cached outlookCachedToken
	var err error
	if c.acquireAccessToken != nil {
		token, tokenErr := c.acquireAccessToken(ctx)
		if tokenErr != nil {
			err = tokenErr
		} else {
			cached = outlookCachedToken{
				accessToken: token,
				expiresAt:   c.currentTime().Add(time.Hour),
			}
		}
	} else {
		cached, err = c.requestAccessToken(ctx)
	}
	if err != nil {
		return "", err
	}
	if cached.accessToken == "" {
		return "", fmt.Errorf("Outlook token endpoint returned an empty access token")
	}

	c.clientMu.Lock()
	c.accessToken = cached.accessToken
	c.tokenExpiry = cached.expiresAt
	c.clientMu.Unlock()
	return cached.accessToken, nil
}

func (c *OutlookConnector) cachedTokenExpiredLocked() bool {
	return c.tokenExpiry.IsZero() || !c.currentTime().Add(outlookTokenExpiryMargin).Before(c.tokenExpiry)
}

func (c *OutlookConnector) invalidateToken(token string) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *OutlookConnector) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *OutlookConnector) requestAccessToken(ctx context.Context) (outlookCachedToken, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
		"scope":         {outlookGraphScope},
	}
	requestCtx, cancel := context.WithTimeout(ctx, outlookRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, fmt.Sprintf(outlookTokenURLFormat, url.PathEscape(c.tenantID)), strings.NewReader(form.Encode()))
	if err != nil {
		return outlookCachedToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return outlookCachedToken{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return outlookCachedToken{}, outlookHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	var token outlookTokenResponse
	if err = json.Unmarshal(body, &token); err != nil {
		return outlookCachedToken{}, err
	}
	if token.ExpiresIn <= 0 {
		return outlookCachedToken{}, fmt.Errorf("Outlook token endpoint returned invalid expires_in")
	}
	return outlookCachedToken{
		accessToken: token.AccessToken,
		expiresAt:   c.currentTime().Add(time.Duration(token.ExpiresIn) * time.Second),
	}, nil
}

type outlookSyncSession struct {
	connector        *OutlookConnector
	users            []string
	userIndex        int
	pageURL          string
	batchSize        int
	windowStart      *time.Time
	windowEnd        time.Time
	deltaLinks       map[string]string
	buffer           []outlookBufferedDocument
	resumePageURL    string
	resumeOffset     int
	resumeSourceID   string
	completedCurrent bool
}

// NextBatch returns the next Outlook message batch.
func (s *outlookSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
		page, err := s.nextDocumentPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		if len(page) == 0 && s.completedCurrent {
			continue
		}
		remaining := s.batchSize - len(documents)
		if len(page) > remaining {
			for _, buffered := range page[:remaining] {
				documents = append(documents, buffered.document)
				checkpoint = buffered.checkpoint
			}
			s.buffer = append(s.buffer, page[remaining:]...)
			break
		}
		for _, buffered := range page {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the Outlook sync session.
func (s *outlookSyncSession) Close() error {
	return nil
}

func (s *outlookSyncSession) nextDocumentPage(ctx context.Context) ([]outlookBufferedDocument, error) {
	s.completedCurrent = false
	userID := s.users[s.userIndex]
	requestURL := s.pageURL
	if requestURL == "" {
		requestURL = s.startURL(userID)
	}
	page, err := s.connector.deltaPage(ctx, requestURL)
	if page.DeltaLink != "" {
		s.deltaLinks[userID] = page.DeltaLink
	}
	if err != nil {
		return nil, err
	}

	candidates := make([]outlookBufferedDocument, 0, len(page.Value))
	pageOffset := 0
	for _, message := range page.Value {
		if message.Removed != nil {
			continue
		}
		doc, ok := message.toSourceDocument(userID, s.connector.folder)
		if !ok {
			continue
		}
		if !s.inWindow(doc.UpdatedAt) {
			continue
		}
		pageOffset++
		candidates = append(candidates, outlookBufferedDocument{
			document: doc,
			checkpoint: s.checkpoint(outlookSyncCursor{
				UserID:     userID,
				PageURL:    requestURL,
				Offset:     pageOffset,
				SourceID:   doc.SourceID,
				DeltaLinks: s.deltaLinks,
			}, doc),
			offset: pageOffset,
		})
	}

	documents, err := s.filterResumedDocuments(requestURL, candidates)
	if err != nil {
		return nil, err
	}
	if page.NextLink != "" {
		s.pageURL = page.NextLink
		return documents, nil
	}
	s.advanceUser()
	s.completedCurrent = true
	return documents, nil
}

func (s *outlookSyncSession) startURL(userID string) string {
	if deltaLink := s.deltaLinks[userID]; deltaLink != "" {
		return deltaLink
	}
	query := url.Values{
		"$select": {"id,subject,body,receivedDateTime,from,toRecipients,ccRecipients,hasAttachments,conversationId,webLink"},
	}
	return outlookGraphBase + "/users/" + url.PathEscape(userID) + "/mailFolders/" + url.PathEscape(s.connector.folder) + "/messages/delta?" + query.Encode()
}

func (s *outlookSyncSession) inWindow(updatedAt time.Time) bool {
	if !s.windowEnd.IsZero() && updatedAt.After(s.windowEnd) {
		return false
	}
	if s.windowStart != nil && !updatedAt.IsZero() && updatedAt.Before(*s.windowStart) {
		return false
	}
	return true
}

func (s *outlookSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("outlook sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor outlookSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("outlook sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" {
		return fmt.Errorf("outlook sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if len(cursor.DeltaLinks) > 0 {
		s.deltaLinks = cursor.DeltaLinks
	}
	if cursor.UserID == "" {
		return fmt.Errorf("outlook sync cursor has no user anchor: %w", ErrSyncResumeInvalid)
	}
	for index, userID := range s.users {
		if userID != cursor.UserID {
			continue
		}
		s.userIndex = index
		s.pageURL = cursor.PageURL
		s.resumePageURL = cursor.PageURL
		s.resumeOffset = cursor.Offset
		s.resumeSourceID = sourceID
		return nil
	}
	return fmt.Errorf("outlook resume user %q was not found in the current listing: %w", cursor.UserID, ErrSyncResumeInvalid)
}

func (s *outlookSyncSession) filterResumedDocuments(pageURL string, candidates []outlookBufferedDocument) ([]outlookBufferedDocument, error) {
	if s.resumeSourceID == "" {
		return candidates, nil
	}
	if pageURL != s.resumePageURL {
		return nil, fmt.Errorf("outlook resume page no longer matches checkpoint page: %w", ErrSyncResumeInvalid)
	}
	for _, candidate := range candidates {
		if candidate.document.SourceID == s.resumeSourceID {
			filtered := candidates[:0]
			for _, remaining := range candidates {
				if remaining.offset > candidate.offset {
					filtered = append(filtered, remaining)
				}
			}
			s.clearResumeOffset()
			return filtered, nil
		}
	}
	return nil, fmt.Errorf("outlook resume anchor %q was not found on %s: %w", s.resumeSourceID, pageURL, ErrSyncResumeInvalid)
}

func (s *outlookSyncSession) checkpoint(cursor outlookSyncCursor, doc SourceDocument) *SyncCheckpoint {
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    string(data),
		UpdatedAt: &updatedAt,
		SourceID:  doc.SourceID,
	}
}

func (s *outlookSyncSession) advanceUser() {
	s.userIndex++
	s.pageURL = ""
	s.clearResumeOffset()
}

func (s *outlookSyncSession) clearResumeOffset() {
	s.resumePageURL = ""
	s.resumeOffset = 0
	s.resumeSourceID = ""
}

type outlookPruneSession struct {
	connector *OutlookConnector
	users     []string
	userIndex int
	pageURL   string
	batchSize int
	buffer    []SlimDocument
}

// NextBatch returns the next Outlook prune snapshot batch.
func (s *outlookPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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
		page, err := s.nextSlimPage(ctx)
		if err != nil {
			return PruneBatch{}, err
		}
		remaining := s.batchSize - len(documents)
		if len(page) > remaining {
			documents = append(documents, page[:remaining]...)
			s.buffer = append(s.buffer, page[remaining:]...)
			break
		}
		documents = append(documents, page...)
	}
	return PruneBatch{Documents: documents}, nil
}

// Close closes the Outlook prune session.
func (s *outlookPruneSession) Close() error {
	return nil
}

func (s *outlookPruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	userID := s.users[s.userIndex]
	requestURL := s.pageURL
	if requestURL == "" {
		query := url.Values{"$select": {"id"}}
		requestURL = outlookGraphBase + "/users/" + url.PathEscape(userID) + "/mailFolders/" + url.PathEscape(s.connector.folder) + "/messages/delta?" + query.Encode()
	}
	page, err := s.connector.deltaPage(ctx, requestURL)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(page.Value))
	for _, message := range page.Value {
		if message.Removed == nil && message.ID != "" {
			documents = append(documents, SlimDocument{SourceID: message.ID})
		}
	}
	if page.NextLink != "" {
		s.pageURL = page.NextLink
	} else {
		s.userIndex++
		s.pageURL = ""
	}
	return documents, nil
}

type outlookBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
}

type outlookSyncCursor struct {
	UserID     string            `json:"user_id"`
	PageURL    string            `json:"page_url,omitempty"`
	Offset     int               `json:"offset,omitempty"`
	SourceID   string            `json:"source_id,omitempty"`
	DeltaLinks map[string]string `json:"delta_links,omitempty"`
}

type outlookUsersPage struct {
	NextLink string `json:"@odata.nextLink"`
	Value    []struct {
		ID                string `json:"id"`
		UserPrincipalName string `json:"userPrincipalName"`
		Mail              string `json:"mail"`
	} `json:"value"`
}

type outlookDeltaPage struct {
	NextLink  string           `json:"@odata.nextLink"`
	DeltaLink string           `json:"@odata.deltaLink"`
	Value     []outlookMessage `json:"value"`
}

type outlookMessage struct {
	ID               string               `json:"id"`
	Subject          string               `json:"subject"`
	Body             outlookMessageBody   `json:"body"`
	ReceivedDateTime string               `json:"receivedDateTime"`
	From             outlookRecipientSlot `json:"from"`
	ToRecipients     []outlookRecipient   `json:"toRecipients"`
	CcRecipients     []outlookRecipient   `json:"ccRecipients"`
	HasAttachments   bool                 `json:"hasAttachments"`
	ConversationID   string               `json:"conversationId"`
	WebLink          string               `json:"webLink"`
	Removed          map[string]any       `json:"@removed"`
}

func (m outlookMessage) toSourceDocument(userID, folder string) (SourceDocument, bool) {
	if m.ID == "" {
		return SourceDocument{}, false
	}
	subject := firstNonEmpty(m.Subject, "(no subject)")
	bodyType := strings.ToLower(strings.TrimSpace(m.Body.ContentType))
	bodyText := m.Body.Content
	if bodyType == "html" {
		bodyText = stripOutlookHTML(bodyText)
	}
	updatedAt := parseOutlookTime(m.ReceivedDateTime)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	from := m.From.EmailAddress
	toRecipients := outlookRecipientAddresses(m.ToRecipients)
	ccRecipients := outlookRecipientAddresses(m.CcRecipients)

	lines := []string{
		fmt.Sprintf("From: %s <%s>", from.Name, from.Address),
		"To: " + strings.Join(toRecipients, ", "),
	}
	if len(ccRecipients) > 0 {
		lines = append(lines, "Cc: "+strings.Join(ccRecipients, ", "))
	}
	lines = append(lines, "Subject: "+subject, "", bodyText)
	blob := []byte(strings.Join(lines, "\n"))

	metadata := map[string]any{
		"user_id":         userID,
		"folder":          folder,
		"from":            from.Address,
		"to":              strings.Join(toRecipients, ","),
		"cc":              strings.Join(ccRecipients, ","),
		"has_attachments": fmt.Sprint(m.HasAttachments),
		"conversation_id": m.ConversationID,
		"web_link":        m.WebLink,
	}
	if from.Address != "" {
		metadata["primary_owners"] = []map[string]string{outlookOwnerMetadata(from)}
	}

	extension := ".txt"
	if bodyType == "html" {
		extension = ".html"
	}
	return SourceDocument{
		SourceID:           m.ID,
		SemanticIdentifier: subject,
		Extension:          extension,
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}, true
}

type outlookMessageBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type outlookRecipientSlot struct {
	EmailAddress outlookEmailAddress `json:"emailAddress"`
}

type outlookRecipient struct {
	EmailAddress outlookEmailAddress `json:"emailAddress"`
}

type outlookEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type outlookTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type outlookCachedToken struct {
	accessToken string
	expiresAt   time.Time
}

type outlookHTTPError struct {
	status int
	body   string
}

// Error returns the Microsoft Graph API error text.
func (e outlookHTTPError) Error() string {
	return fmt.Sprintf("Outlook API returned HTTP %d: %s", e.status, e.body)
}

func isOutlookRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

func outlookRecipientAddresses(recipients []outlookRecipient) []string {
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient.EmailAddress.Address != "" {
			out = append(out, recipient.EmailAddress.Address)
		}
	}
	return out
}

func outlookOwnerMetadata(address outlookEmailAddress) map[string]string {
	item := map[string]string{"email": address.Address}
	name := strings.TrimSpace(address.Name)
	if name == "" {
		return item
	}
	parts := strings.Fields(name)
	if len(parts) > 1 {
		item["first_name"] = strings.Join(parts[:len(parts)-1], " ")
		item["last_name"] = parts[len(parts)-1]
	} else {
		item["last_name"] = name
	}
	return item
}

func parseOutlookTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func stripOutlookHTML(value string) string {
	value = outlookHTMLScriptStyleRE.ReplaceAllString(value, "")
	value = outlookHTMLTagRE.ReplaceAllString(value, " ")
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = outlookWhitespaceRE.ReplaceAllString(value, " ")
	value = outlookNewlineRE.ReplaceAllString(value, "\n\n")
	return strings.TrimSpace(value)
}
