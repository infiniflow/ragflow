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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	onedriveGraphBase            = "https://graph.microsoft.com/v1.0"
	onedriveTokenURLFormat       = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	onedriveGraphScope           = "https://graph.microsoft.com/.default"
	onedriveDefaultBatchSize     = 2
	onedriveDefaultSizeThreshold = 20 * 1024 * 1024
	onedriveRequestTimeout       = 60 * time.Second
	onedriveRetryCount           = 4
	onedriveRetryBaseDelay       = 200 * time.Millisecond
	onedriveTokenExpiryMargin    = 5 * time.Minute
	onedriveMaxJSONResponseSize  = 16 * 1024 * 1024
)

// onedriveSupportedExtensions mirrors the extension set used by the Python
// OneDrive connector.
var onedriveSupportedExtensions = map[string]struct{}{
	".pdf":  {},
	".docx": {},
	".doc":  {},
	".xlsx": {},
	".xls":  {},
	".pptx": {},
	".ppt":  {},
	".txt":  {},
	".md":   {},
	".csv":  {},
}

// OneDriveConnector reads files from OneDrive / OneDrive for Business through
// the Microsoft Graph delta API with app-only client-credentials auth.
type OneDriveConnector struct {
	tenantID      string
	clientID      string
	clientSecret  string
	folderPath    string
	batchSize     int
	sizeThreshold int64
	graphBaseURL  string
	tokenURL      string

	clientMu    sync.Mutex
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	now         func() time.Time

	acquireAccessToken func(ctx context.Context) (string, error)
	getJSON            func(ctx context.Context, apiURL string, out any) error
	getBytes           func(ctx context.Context, apiURL string) ([]byte, error)
}

// NewOneDriveConnector creates a OneDrive connector from Python-compatible config.
func NewOneDriveConnector(config map[string]any) (*OneDriveConnector, error) {
	credentials := configAnyMap(config["credentials"])
	folderPath, err := normalizeOneDriveFolderPath(stringConfig(config["folder_path"]))
	if err != nil {
		return nil, err
	}
	batchSize := oneDriveBatchSize(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])))
	sizeThreshold := int64(configInt(config["size_threshold"], onedriveDefaultSizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = onedriveDefaultSizeThreshold
	}
	return &OneDriveConnector{
		tenantID:      strings.TrimSpace(stringConfig(credentials["tenant_id"])),
		clientID:      strings.TrimSpace(stringConfig(credentials["client_id"])),
		clientSecret:  stringConfig(credentials["client_secret"]),
		folderPath:    folderPath,
		batchSize:     batchSize,
		sizeThreshold: sizeThreshold,
		graphBaseURL:  onedriveGraphBase,
		tokenURL:      "",
		httpClient:    &http.Client{Timeout: onedriveRequestTimeout},
		now:           time.Now,
	}, nil
}

func oneDriveBatchSize(value string) int {
	if strings.TrimSpace(value) == "" {
		return onedriveDefaultBatchSize
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return onedriveDefaultBatchSize
	}
	return parsed
}

func normalizeOneDriveFolderPath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	segments := make([]string, 0, strings.Count(value, "/")+1)
	for _, segment := range strings.Split(value, "/") {
		if segment == "" {
			continue
		}
		if segment == ".." {
			return "", &ConnectorValidationError{Message: "folder_path must not contain '..' segments."}
		}
		segments = append(segments, segment)
	}
	if len(segments) == 0 {
		return "", nil
	}
	return "/" + strings.Join(segments, "/"), nil
}

// Validate validates OneDrive credentials, batch size, and Graph access.
func (c *OneDriveConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "OneDrive connector is nil"}
	}
	if c.tenantID == "" || c.clientID == "" || c.clientSecret == "" {
		return &ConnectorMissingCredentialError{Message: "OneDrive credentials are incomplete: tenant_id, client_id, and client_secret are required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "OneDrive connector batch_size must be a positive integer"}
	}
	if _, err := c.token(ctx); err != nil {
		return &ConnectorMissingCredentialError{Message: fmt.Sprintf("Failed to acquire OneDrive access token: %v", err)}
	}
	var page struct {
		Value []onedriveDrive `json:"value"`
	}
	if err := c.graphJSON(ctx, c.graphBaseURL+"/drives?$top=1", &page); err != nil {
		var httpErr *onedriveHTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.status {
			case http.StatusUnauthorized:
				return &ConnectorMissingCredentialError{Message: "OneDrive access token is invalid or expired."}
			case http.StatusForbidden:
				return &ConnectorValidationError{Message: "The service principal lacks the 'Files.Read.All' permission required by the OneDrive connector."}
			default:
				return &ConnectorValidationError{Message: fmt.Sprintf("OneDrive validation failed (HTTP %d): %s", httpErr.status, httpErr.body)}
			}
		}
		return &ConnectorValidationError{Message: fmt.Sprintf("OneDrive validation error: %v", err)}
	}
	if page.Value == nil {
		return &ConnectorValidationError{Message: "Unexpected response format from Microsoft Graph /drives."}
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved connector config through a
// freshly constructed connector so request-derived URLs and credentials are
// used instead of state copied from the receiver.
func (c *OneDriveConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	candidate, err := NewOneDriveConnector(request)
	if err != nil {
		return err
	}
	if c != nil {
		candidate.httpClient = c.httpClient
		candidate.now = c.now
		candidate.graphBaseURL = c.graphBaseURL
		candidate.tokenURL = c.tokenURL
	}
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return candidate.Validate(ctx)
}

// OpenSync opens one OneDrive sync session.
func (c *OneDriveConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	drives, err := c.listDrives(ctx)
	if err != nil {
		return nil, err
	}
	session := &onedriveSyncSession{
		connector:  c,
		request:    request,
		drives:     drives,
		batchSize:  c.effectiveBatchSize(),
		deltaLinks: map[string]string{},
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete OneDrive prune snapshot session.
func (c *OneDriveConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	drives, err := c.listDrives(ctx)
	if err != nil {
		return nil, err
	}
	return &onedrivePruneSession{
		connector: c,
		drives:    drives,
		batchSize: c.effectiveBatchSize(),
	}, nil
}

func (c *OneDriveConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return onedriveDefaultBatchSize
}

func (c *OneDriveConnector) listDrives(ctx context.Context) ([]string, error) {
	var driveIDs []string
	apiURL := c.graphBaseURL + "/drives"
	for apiURL != "" {
		var page onedriveDrivesPage
		if err := c.graphJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		for _, drive := range page.Value {
			if strings.TrimSpace(drive.ID) != "" {
				driveIDs = append(driveIDs, drive.ID)
			}
		}
		apiURL = page.NextLink
	}
	return uniqueSorted(driveIDs), nil
}

func (c *OneDriveConnector) deltaPage(ctx context.Context, apiURL string) (onedriveDeltaPage, error) {
	var page onedriveDeltaPage
	err := c.graphJSON(ctx, apiURL, &page)
	return page, err
}

func (c *OneDriveConnector) deltaURL(driveID string) string {
	if c.folderPath == "" {
		return fmt.Sprintf("%s/drives/%s/root/delta", c.graphBaseURL, url.PathEscape(driveID))
	}
	segments := strings.Split(strings.TrimPrefix(c.folderPath, "/"), "/")
	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		escaped = append(escaped, url.PathEscape(segment))
	}
	return fmt.Sprintf("%s/drives/%s/root:/%s:/delta", c.graphBaseURL, url.PathEscape(driveID), strings.Join(escaped, "/"))
}

// Fetch downloads a delayed OneDrive file body.
func (c *OneDriveConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch onedriveFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	if fetch.DriveID == "" || fetch.ItemID == "" {
		return nil, fmt.Errorf("onedrive fetch reference is incomplete")
	}
	if fetch.Size > c.sizeThreshold {
		return nil, fmt.Errorf("%s exceeds size threshold of %d", firstNonEmpty(fetch.Name, fetch.ItemID), c.sizeThreshold)
	}
	return c.downloadContent(ctx, fetch.DriveID, fetch.ItemID)
}

func (c *OneDriveConnector) downloadContent(ctx context.Context, driveID, itemID string) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/drives/%s/items/%s/content", c.graphBaseURL, url.PathEscape(driveID), url.PathEscape(itemID))
	return c.graphBytes(ctx, apiURL)
}

func (c *OneDriveConnector) isAcceptedItem(item onedriveDriveItem) bool {
	if strings.TrimSpace(item.ID) == "" || item.File == nil || item.Deleted != nil || item.Removed != nil {
		return false
	}
	extension := strings.ToLower(filepath.Ext(item.Name))
	if _, ok := onedriveSupportedExtensions[extension]; !ok {
		return false
	}
	if item.Size < 0 || (item.Size > 0 && item.Size > c.sizeThreshold) {
		return false
	}
	return true
}

func (c *OneDriveConnector) sourceDocument(driveID string, item onedriveDriveItem) (SourceDocument, bool) {
	if !c.isAcceptedItem(item) {
		return SourceDocument{}, false
	}
	name := firstNonEmpty(item.Name, item.ID)
	size := item.Size
	if size < 0 {
		size = 0
	}
	fetch, _ := json.Marshal(onedriveFetchReference{DriveID: driveID, ItemID: item.ID, Name: name, Size: size})
	updatedAt := parseOutlookTime(item.LastModifiedDateTime)
	if updatedAt.IsZero() {
		updatedAt = c.currentTime().UTC()
	}
	return SourceDocument{
		SourceID:           item.ID,
		SemanticIdentifier: name,
		Extension:          strings.ToLower(filepath.Ext(name)),
		FetchRef:           &FetchReference{Key: string(fetch), SizeHint: size},
		UpdatedAt:          updatedAt,
		SizeBytes:          size,
		Metadata: map[string]any{
			"drive_id":   driveID,
			"web_url":    item.WebURL,
			"created_by": item.CreatedBy.User.DisplayName,
		},
		Fingerprint: firstNonEmpty(item.ETag, item.CTag),
	}, true
}

// graphJSON fetches and decodes one Graph JSON response.
func (c *OneDriveConnector) graphJSON(ctx context.Context, apiURL string, out any) error {
	if c.getJSON != nil {
		return c.getJSON(ctx, apiURL, out)
	}
	body, err := c.doGraphRequest(ctx, apiURL, onedriveMaxJSONResponseSize)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// graphBytes fetches a raw Graph response body.
func (c *OneDriveConnector) graphBytes(ctx context.Context, apiURL string) ([]byte, error) {
	if c.getBytes != nil {
		return c.getBytes(ctx, apiURL)
	}
	return c.doGraphRequest(ctx, apiURL, c.sizeThreshold)
}

func (c *OneDriveConnector) doGraphRequest(ctx context.Context, apiURL string, maxBody int64) ([]byte, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	var lastErr error
	retriedUnauthorized := false
	for attempt := 1; attempt <= onedriveRetryCount; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, onedriveRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, apiURL, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
			resp.Body.Close()
			cancel()
			if resp.StatusCode >= 400 {
				lastErr = &onedriveHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
				if resp.StatusCode == http.StatusUnauthorized && !retriedUnauthorized {
					c.invalidateToken(token)
					token, err = c.token(ctx)
					if err != nil {
						return nil, err
					}
					retriedUnauthorized = true
					attempt--
					continue
				}
				if !isOneDriveRetryable(resp.StatusCode) {
					return nil, lastErr
				}
			} else {
				if readErr != nil {
					return nil, readErr
				}
				if int64(len(body)) > maxBody {
					return nil, fmt.Errorf("OneDrive API response from %s exceeds maximum size of %d bytes", apiURL, maxBody)
				}
				return body, nil
			}
		}
		if attempt == onedriveRetryCount {
			break
		}
		delay := time.Duration(attempt) * onedriveRetryBaseDelay
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func (c *OneDriveConnector) token(ctx context.Context) (string, error) {
	c.clientMu.Lock()
	if c.accessToken != "" && !c.cachedTokenExpiredLocked() {
		token := c.accessToken
		c.clientMu.Unlock()
		return token, nil
	}
	c.clientMu.Unlock()

	var cached onedriveCachedToken
	var err error
	if c.acquireAccessToken != nil {
		var token string
		token, err = c.acquireAccessToken(ctx)
		if err == nil {
			cached = onedriveCachedToken{accessToken: token, expiresAt: c.currentTime().Add(time.Hour)}
		}
	} else {
		cached, err = c.requestAccessToken(ctx)
	}
	if err != nil {
		return "", err
	}
	if cached.accessToken == "" {
		return "", fmt.Errorf("OneDrive token endpoint returned an empty access token")
	}
	c.clientMu.Lock()
	c.accessToken = cached.accessToken
	c.tokenExpiry = cached.expiresAt
	c.clientMu.Unlock()
	return cached.accessToken, nil
}

func (c *OneDriveConnector) cachedTokenExpiredLocked() bool {
	return c.tokenExpiry.IsZero() || !c.currentTime().Add(onedriveTokenExpiryMargin).Before(c.tokenExpiry)
}

func (c *OneDriveConnector) invalidateToken(token string) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *OneDriveConnector) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *OneDriveConnector) requestAccessToken(ctx context.Context) (onedriveCachedToken, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
		"scope":         {onedriveGraphScope},
	}
	requestCtx, cancel := context.WithTimeout(ctx, onedriveRequestTimeout)
	defer cancel()
	tokenURL := c.tokenURL
	if tokenURL == "" {
		tokenURL = fmt.Sprintf(onedriveTokenURLFormat, url.PathEscape(c.tenantID))
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return onedriveCachedToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return onedriveCachedToken{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return onedriveCachedToken{}, &onedriveHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	var token onedriveTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return onedriveCachedToken{}, err
	}
	if token.ExpiresIn <= 0 {
		return onedriveCachedToken{}, fmt.Errorf("OneDrive token endpoint returned invalid expires_in")
	}
	return onedriveCachedToken{
		accessToken: token.AccessToken,
		expiresAt:   c.currentTime().Add(time.Duration(token.ExpiresIn) * time.Second),
	}, nil
}

func isOneDriveRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

func includeOneDriveItem(request SyncRequest, item onedriveDriveItem) bool {
	if request.FromBeginning {
		return true
	}
	updatedAt := parseOutlookTime(item.LastModifiedDateTime)
	if updatedAt.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := firstNonEmpty(item.ETag, item.CTag)
		stored, ok := request.Fingerprints[item.ID]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

// onedriveSyncSession streams OneDrive delta pages and checkpoints after each
// emitted document.
type onedriveSyncSession struct {
	connector  *OneDriveConnector
	request    SyncRequest
	drives     []string
	driveIndex int
	pageURL    string
	batchSize  int
	deltaLinks map[string]string
	buffer     []onedriveBufferedDocument

	resumePageURL    string
	resumeSourceID   string
	resumeOffset     int
	completedCurrent bool
}

// NextBatch returns the next OneDrive source document batch.
func (s *onedriveSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
		if s.driveIndex >= len(s.drives) {
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

// Close closes the OneDrive sync session.
func (s *onedriveSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed OneDrive file body for this sync session.
func (s *onedriveSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *onedriveSyncSession) nextDocumentPage(ctx context.Context) ([]onedriveBufferedDocument, error) {
	s.completedCurrent = false
	driveID := s.drives[s.driveIndex]
	requestURL := s.pageURL
	if requestURL == "" {
		requestURL = s.connector.deltaURL(driveID)
	}
	page, err := s.connector.deltaPage(ctx, requestURL)
	if err != nil {
		return nil, err
	}
	if page.DeltaLink != "" {
		s.deltaLinks[driveID] = page.DeltaLink
	}

	all := make([]onedriveBufferedDocument, 0, len(page.Value))
	included := make([]onedriveBufferedDocument, 0, len(page.Value))
	pageOffset := 0
	for _, item := range page.Value {
		if !s.connector.isAcceptedItem(item) {
			continue
		}
		doc, ok := s.connector.sourceDocument(driveID, item)
		if !ok {
			continue
		}
		pageOffset++
		buffered := onedriveBufferedDocument{
			document:   doc,
			checkpoint: s.checkpoint(onedriveSyncCursor{DriveID: driveID, PageURL: requestURL, Offset: pageOffset, SourceID: doc.SourceID, DeltaLinks: s.deltaLinks}, doc),
			offset:     pageOffset,
		}
		all = append(all, buffered)
		if includeOneDriveItem(s.request, item) {
			included = append(included, buffered)
		}
	}
	documents, err := s.filterResumedDocuments(requestURL, all, included)
	if err != nil {
		return nil, err
	}
	if page.NextLink != "" {
		if page.NextLink == requestURL {
			return nil, fmt.Errorf("onedrive sync pagination did not advance from %s", requestURL)
		}
		s.pageURL = page.NextLink
		return documents, nil
	}
	s.advanceDrive()
	s.completedCurrent = true
	return documents, nil
}

func (s *onedriveSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("onedrive sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor onedriveSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("onedrive sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" || cursor.DriveID == "" || cursor.PageURL == "" {
		return fmt.Errorf("onedrive sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if len(cursor.DeltaLinks) > 0 {
		s.deltaLinks = cursor.DeltaLinks
	}
	for index, driveID := range s.drives {
		if driveID != cursor.DriveID {
			continue
		}
		s.driveIndex = index
		s.pageURL = cursor.PageURL
		s.resumePageURL = cursor.PageURL
		s.resumeSourceID = sourceID
		s.resumeOffset = cursor.Offset
		return nil
	}
	return fmt.Errorf("onedrive resume drive %q was not found in the current listing: %w", cursor.DriveID, ErrSyncResumeInvalid)
}

func (s *onedriveSyncSession) filterResumedDocuments(pageURL string, all, included []onedriveBufferedDocument) ([]onedriveBufferedDocument, error) {
	if s.resumeSourceID == "" {
		return included, nil
	}
	if pageURL != s.resumePageURL {
		return nil, fmt.Errorf("onedrive resume page %q no longer matches checkpoint page %q: %w", pageURL, s.resumePageURL, ErrSyncResumeInvalid)
	}
	anchorOffset := -1
	for _, candidate := range all {
		if candidate.document.SourceID == s.resumeSourceID {
			anchorOffset = candidate.offset
			break
		}
	}
	if anchorOffset < 0 {
		return nil, fmt.Errorf("onedrive resume anchor %q was not found on %s: %w", s.resumeSourceID, pageURL, ErrSyncResumeInvalid)
	}
	s.clearResumeOffset()
	filtered := included[:0]
	for _, candidate := range included {
		if candidate.offset > anchorOffset {
			filtered = append(filtered, candidate)
		}
	}
	return filtered, nil
}

func (s *onedriveSyncSession) checkpoint(cursor onedriveSyncCursor, doc SourceDocument) *SyncCheckpoint {
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{Cursor: string(data), UpdatedAt: &updatedAt, SourceID: doc.SourceID}
}

func (s *onedriveSyncSession) advanceDrive() {
	s.driveIndex++
	s.pageURL = ""
	s.clearResumeOffset()
}

func (s *onedriveSyncSession) clearResumeOffset() {
	s.resumePageURL = ""
	s.resumeSourceID = ""
	s.resumeOffset = 0
}

type onedriveBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
}

type onedriveSyncCursor struct {
	DriveID    string            `json:"drive_id"`
	PageURL    string            `json:"page_url"`
	Offset     int               `json:"offset,omitempty"`
	SourceID   string            `json:"source_id,omitempty"`
	DeltaLinks map[string]string `json:"delta_links,omitempty"`
}

// onedrivePruneSession streams the complete OneDrive slim snapshot.
type onedrivePruneSession struct {
	connector  *OneDriveConnector
	drives     []string
	driveIndex int
	pageURL    string
	batchSize  int
	buffer     []SlimDocument
}

// NextBatch returns the next OneDrive prune snapshot batch.
func (s *onedrivePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.driveIndex >= len(s.drives) {
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

// Close closes the OneDrive prune session.
func (s *onedrivePruneSession) Close() error {
	return nil
}

func (s *onedrivePruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	driveID := s.drives[s.driveIndex]
	requestURL := s.pageURL
	if requestURL == "" {
		requestURL = s.connector.deltaURL(driveID)
	}
	page, err := s.connector.deltaPage(ctx, requestURL)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(page.Value))
	for _, item := range page.Value {
		if s.connector.isAcceptedItem(item) {
			documents = append(documents, SlimDocument{SourceID: item.ID})
		}
	}
	if page.NextLink != "" {
		if page.NextLink == requestURL {
			return nil, fmt.Errorf("onedrive prune pagination did not advance from %s", requestURL)
		}
		s.pageURL = page.NextLink
	} else {
		s.driveIndex++
		s.pageURL = ""
	}
	return documents, nil
}

type onedriveDrivesPage struct {
	NextLink string          `json:"@odata.nextLink"`
	Value    []onedriveDrive `json:"value"`
}

type onedriveDrive struct {
	ID string `json:"id"`
}

type onedriveDeltaPage struct {
	NextLink  string              `json:"@odata.nextLink"`
	DeltaLink string              `json:"@odata.deltaLink"`
	Value     []onedriveDriveItem `json:"value"`
}

type onedriveDriveItem struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	Size                 int64             `json:"size"`
	WebURL               string            `json:"webUrl"`
	LastModifiedDateTime string            `json:"lastModifiedDateTime"`
	ETag                 string            `json:"eTag"`
	CTag                 string            `json:"cTag"`
	Folder               *onedriveFacet    `json:"folder,omitempty"`
	File                 *onedriveFacet    `json:"file,omitempty"`
	Deleted              map[string]any    `json:"deleted,omitempty"`
	Removed              map[string]any    `json:"@removed,omitempty"`
	CreatedBy            onedriveCreatedBy `json:"createdBy,omitempty"`
}

type onedriveFacet struct {
	ChildCount int `json:"childCount"`
}

type onedriveCreatedBy struct {
	User struct {
		DisplayName string `json:"displayName"`
	} `json:"user"`
}

type onedriveTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type onedriveCachedToken struct {
	accessToken string
	expiresAt   time.Time
}

type onedriveFetchReference struct {
	DriveID string `json:"drive_id"`
	ItemID  string `json:"item_id"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
}

type onedriveHTTPError struct {
	status int
	body   string
}

func (e *onedriveHTTPError) Error() string {
	return fmt.Sprintf("OneDrive API returned HTTP %d: %s", e.status, e.body)
}
