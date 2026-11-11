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
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// SharePoint connector constants.
const (
	sharepointGraphBase           = "https://graph.microsoft.com/v1.0"
	sharepointTokenURLFormat      = "https://login.microsoftonline.com/%s/oauth2/v2.0/token"
	sharepointGraphScope          = "https://graph.microsoft.com/.default"
	sharepointDefaultBatchSize    = 2
	sharepointRequestTimeout      = 60 * time.Second
	sharepointRetryCount          = 4
	sharepointRetryBaseDelay      = time.Second
	sharepointTokenExpiryMargin   = 5 * time.Minute
	sharepointMaxJSONResponseSize = 16 * 1024 * 1024
	sharepointDefaultMaxFileSize  = 256 * 1024 * 1024
)

// sharepointMaxFileSize caps downloaded file content. Package variable so
// tests can shrink it.
var sharepointMaxFileSize int64 = sharepointDefaultMaxFileSize

// SharePointConnector reads files from SharePoint document libraries through
// the Microsoft Graph API using an app-only (client-credentials) token.
type SharePointConnector struct {
	tenantID      string
	clientID      string
	clientSecret  string
	siteURL       string
	batchSize     int
	driveID       string
	rootFolderIDs []string
	excludeVideos bool
	filters       fileFilters

	clientMu    sync.Mutex
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	now         func() time.Time

	tokenURL     string
	graphBaseURL string
}

// NewSharePointConnector creates a SharePoint connector from connector config.
func NewSharePointConnector(config map[string]any) (*SharePointConnector, error) {
	credentials := configAnyMap(config["credentials"])

	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), sharepointDefaultBatchSize)
	if batchSize <= 0 {
		batchSize = sharepointDefaultBatchSize
	}

	c := &SharePointConnector{
		tenantID:     strings.TrimSpace(stringConfig(credentials["tenant_id"])),
		clientID:     strings.TrimSpace(stringConfig(credentials["client_id"])),
		clientSecret: stringConfig(credentials["client_secret"]),
		siteURL:      strings.TrimSpace(stringConfig(credentials["site_url"])),
		batchSize:    batchSize,
		httpClient:   &http.Client{Timeout: sharepointRequestTimeout},
		tokenURL:     "",
		graphBaseURL: sharepointGraphBase,
	}
	if err := c.configureFileSelection(config); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate validates SharePoint connector settings, credentials, and access.
func (c *SharePointConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "SharePoint connector is nil"}
	}
	if c.tenantID == "" || c.clientID == "" || c.clientSecret == "" || c.siteURL == "" {
		return &ConnectorMissingCredentialError{Message: "SharePoint credentials are incomplete: tenant_id, client_id, client_secret, and site_url are required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "SharePoint connector batch_size must be a positive integer"}
	}
	if _, err := c.token(ctx); err != nil {
		return &ConnectorMissingCredentialError{Message: fmt.Sprintf("Failed to acquire SharePoint access token: %v", err)}
	}
	site, err := c.resolveSite(ctx)
	if err != nil {
		return c.siteValidationError(err)
	}
	return c.validateScope(ctx, site.ID)
}

// ValidateConnectorSetting validates SharePoint settings from an unsaved config.
func (c *SharePointConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one SharePoint sync session.
func (c *SharePointConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	site, err := c.resolveSite(ctx)
	if err != nil {
		return nil, err
	}
	drives, err := c.syncDrives(ctx, site.ID)
	if err != nil {
		return nil, err
	}
	// Drives are processed in ID order so checkpoints advance deterministically.
	sort.SliceStable(drives, func(i, j int) bool {
		return drives[i].ID < drives[j].ID
	})
	session := &sharePointSyncSession{
		connector: c,
		request:   request,
		site:      site,
		drives:    drives,
		batchSize: c.batchSize,
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete SharePoint prune snapshot session.
func (c *SharePointConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.Validate(ctx); err != nil {
		return nil, err
	}
	site, err := c.resolveSite(ctx)
	if err != nil {
		return nil, err
	}
	drives, err := c.listDrives(ctx, site.ID)
	if err != nil {
		return nil, err
	}
	var documents []SlimDocument
	for _, drive := range drives {
		files, err := c.walkDriveFiles(ctx, drive.ID)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			documents = append(documents, SlimDocument{SourceID: sharePointDocID(drive.ID, file.ID)})
		}
	}
	return &sharePointPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

// ---------------------------------------------------------------------------
// Microsoft Graph client
// ---------------------------------------------------------------------------

type sharePointSite struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"webUrl"`
}

type sharePointDrive struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	DriveType string `json:"driveType"`
}

type sharePointItem struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name"`
	Size                 int64                  `json:"size"`
	WebURL               string                 `json:"webUrl"`
	LastModifiedDateTime string                 `json:"lastModifiedDateTime"`
	Folder               *sharePointFolderFacet `json:"folder,omitempty"`
	File                 *sharePointFileFacet   `json:"file,omitempty"`
	CTag                 string                 `json:"cTag"`
	ETag                 string                 `json:"eTag"`
	ParentReference      struct {
		ID string `json:"id"`
	} `json:"parentReference"`
	relativePath string
}

type sharePointFolderFacet struct {
	ChildCount int `json:"childCount"`
}

type sharePointFileFacet struct {
	MimeType string `json:"mimeType"`
}

type sharePointDrivesPage struct {
	NextLink string            `json:"@odata.nextLink"`
	Value    []sharePointDrive `json:"value"`
}

type sharePointChildrenPage struct {
	NextLink string           `json:"@odata.nextLink"`
	Value    []sharePointItem `json:"value"`
}

type sharePointTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type sharePointCachedToken struct {
	accessToken string
	expiresAt   time.Time
}

type sharePointGraphErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// sharePointAPIError marks a non-2xx Microsoft Graph response.
type sharePointAPIError struct {
	status int
	body   string
}

func (e *sharePointAPIError) Error() string {
	return fmt.Sprintf("SharePoint Graph API request failed with status %d: %s", e.status, e.body)
}

// siteAPIPath converts a site URL such as
// https://contoso.sharepoint.com/sites/MySite into the Graph relative path
// /sites/contoso.sharepoint.com:/sites/MySite.
func (c *SharePointConnector) siteAPIPath() string {
	parsed, err := url.Parse(c.siteURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	pathPart := strings.Trim(parsed.Path, "/")
	if pathPart == "" {
		return "/sites/" + parsed.Host
	}
	segments := strings.Split(pathPart, "/")
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = url.PathEscape(segment)
	}
	return "/sites/" + parsed.Host + ":/" + strings.Join(escaped, "/")
}

// resolveSite resolves the configured site URL through the Graph API.
func (c *SharePointConnector) resolveSite(ctx context.Context) (*sharePointSite, error) {
	apiPath := c.siteAPIPath()
	if apiPath == "" {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Invalid SharePoint site URL %q", c.siteURL)}
	}
	var site sharePointSite
	if err := c.getJSON(ctx, c.graphBaseURL+apiPath, &site); err != nil {
		return nil, err
	}
	if site.ID == "" {
		return nil, &ConnectorValidationError{Message: "Failed to access SharePoint site"}
	}
	return &site, nil
}

// listDrives lists every document library under the site, following
// @odata.nextLink pagination.
func (c *SharePointConnector) listDrives(ctx context.Context, siteID string) ([]sharePointDrive, error) {
	var drives []sharePointDrive
	apiURL := c.graphBaseURL + "/sites/" + url.PathEscape(siteID) + "/drives"
	for apiURL != "" {
		var page sharePointDrivesPage
		if err := c.getJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		drives = append(drives, page.Value...)
		apiURL = page.NextLink
	}
	return drives, nil
}

// listChildren lists the children of a drive item; an empty itemID refers to
// the drive root.
func (c *SharePointConnector) listChildren(ctx context.Context, driveID, itemID string) ([]sharePointItem, error) {
	var items []sharePointItem
	var apiURL string
	if itemID == "" {
		apiURL = c.graphBaseURL + "/drives/" + url.PathEscape(driveID) + "/root/children"
	} else {
		apiURL = c.graphBaseURL + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(itemID) + "/children"
	}
	for apiURL != "" {
		var page sharePointChildrenPage
		if err := c.getJSON(ctx, apiURL, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Value...)
		apiURL = page.NextLink
	}
	return items, nil
}

// walkDriveFiles depth-first walks a drive yielding file (non-folder) items.
func (c *SharePointConnector) walkDriveFiles(ctx context.Context, driveID string) ([]sharePointItem, error) {
	// Prune deliberately includes excluded files and files outside the sync
	// root. Editing filters or the root must not remove indexed documents.
	return c.walkFiles(ctx, driveID, "", fileFilters{})
}

func (c *SharePointConnector) walkFiles(ctx context.Context, driveID, rootID string, filters fileFilters) ([]sharePointItem, error) {
	if rootID != "" && len(filters.folderNames) > 0 {
		root, err := c.getItem(ctx, driveID, rootID)
		if err != nil {
			return nil, err
		}
		if filters.folderNames[root.Name] {
			return nil, nil
		}
	}
	var files []sharePointItem
	stack := []sharePointItem{{ID: rootID}}
	for len(stack) > 0 {
		folder := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		children, err := c.listChildren(ctx, driveID, folder.ID)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			child.relativePath = path.Join(folder.relativePath, child.Name)
			if child.Folder != nil {
				if !filters.excludesFolder(child.relativePath) {
					stack = append(stack, child)
				}
			} else if child.File != nil {
				files = append(files, child)
			}
		}
	}
	return files, nil
}

// downloadContent downloads a file's raw bytes.
func (c *SharePointConnector) downloadContent(ctx context.Context, driveID, itemID string) ([]byte, error) {
	apiURL := c.graphBaseURL + "/drives/" + url.PathEscape(driveID) + "/items/" + url.PathEscape(itemID) + "/content"
	return c.getBytes(ctx, apiURL)
}

// getJSON fetches and decodes one Graph JSON response.
func (c *SharePointConnector) getJSON(ctx context.Context, apiURL string, out any) error {
	body, err := c.doGraphRequest(ctx, apiURL, sharepointMaxJSONResponseSize)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// getBytes fetches a raw Graph response body (file content).
func (c *SharePointConnector) getBytes(ctx context.Context, apiURL string) ([]byte, error) {
	return c.doGraphRequest(ctx, apiURL, sharepointMaxFileSize)
}

// doGraphRequest issues one authorized Graph GET, retrying transient failures
// and refreshing an expired access token once on 401.
func (c *SharePointConnector) doGraphRequest(ctx context.Context, apiURL string, maxBody int64) ([]byte, error) {
	if err := c.validateGraphURL(apiURL); err != nil {
		return nil, err
	}
	token, err := c.token(ctx)
	if err != nil {
		return nil, err
	}

	var lastErr error
	retriedUnauthorized := false
	retryDelay := sharepointRetryBaseDelay
	for attempt := 1; attempt <= sharepointRetryCount; attempt++ {
		var retryAfter string
		requestCtx, cancel := context.WithTimeout(ctx, sharepointRequestTimeout)
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
			retryAfter = resp.Header.Get("Retry-After")
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
			resp.Body.Close()
			cancel()
			if resp.StatusCode >= 400 {
				lastErr = sharePointGraphError(resp.StatusCode, body)
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
				if !sharePointRetryable(resp.StatusCode) {
					return nil, lastErr
				}
			} else if readErr != nil {
				return nil, readErr
			} else if int64(len(body)) > maxBody {
				return nil, fmt.Errorf("SharePoint API response from %s exceeds maximum size of %d bytes", apiURL, maxBody)
			} else {
				return body, nil
			}
		}
		if attempt == sharepointRetryCount {
			break
		}
		delay := sharePointRetryDelay(retryAfter, retryDelay, time.Now())
		retryDelay = min(retryDelay*2, 30*time.Second)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func sharePointGraphError(status int, body []byte) error {
	message := strings.TrimSpace(string(body))
	var parsed sharePointGraphErrorBody
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
	}
	return &sharePointAPIError{status: status, body: message}
}

func sharePointRetryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// token returns a cached or freshly acquired app-only access token.
func (c *SharePointConnector) token(ctx context.Context) (string, error) {
	c.clientMu.Lock()
	if c.accessToken != "" && !c.cachedTokenExpiredLocked() {
		token := c.accessToken
		c.clientMu.Unlock()
		return token, nil
	}
	c.clientMu.Unlock()

	cached, err := c.requestAccessToken(ctx)
	if err != nil {
		return "", err
	}
	if cached.accessToken == "" {
		return "", &ConnectorMissingCredentialError{Message: "SharePoint token endpoint returned an empty access token"}
	}

	c.clientMu.Lock()
	c.accessToken = cached.accessToken
	c.tokenExpiry = cached.expiresAt
	c.clientMu.Unlock()
	return cached.accessToken, nil
}

func (c *SharePointConnector) cachedTokenExpiredLocked() bool {
	return c.tokenExpiry.IsZero() || !c.currentTime().Add(sharepointTokenExpiryMargin).Before(c.tokenExpiry)
}

func (c *SharePointConnector) invalidateToken(token string) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *SharePointConnector) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *SharePointConnector) requestAccessToken(ctx context.Context) (sharePointCachedToken, error) {
	tokenURL := c.tokenURL
	if tokenURL == "" {
		tokenURL = fmt.Sprintf(sharepointTokenURLFormat, url.PathEscape(c.tenantID))
	}
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
		"scope":         {sharepointGraphScope},
	}
	requestCtx, cancel := context.WithTimeout(ctx, sharepointRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return sharePointCachedToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return sharePointCachedToken{}, err
	}
	defer resp.Body.Close()
	const maxTokenResponseSize = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseSize+1))
	if err != nil {
		return sharePointCachedToken{}, err
	}
	if len(body) > maxTokenResponseSize {
		return sharePointCachedToken{}, fmt.Errorf("SharePoint token response exceeds maximum size of %d bytes", maxTokenResponseSize)
	}
	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(string(body))
		var parsed sharePointGraphErrorBody
		if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
			message = parsed.Error.Message
		}
		return sharePointCachedToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Failed to acquire SharePoint access token: %s", message)}
	}
	var token sharePointTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return sharePointCachedToken{}, err
	}
	if token.ExpiresIn <= 0 {
		return sharePointCachedToken{}, fmt.Errorf("SharePoint token endpoint returned invalid expires_in")
	}
	return sharePointCachedToken{
		accessToken: token.AccessToken,
		expiresAt:   c.currentTime().Add(time.Duration(token.ExpiresIn) * time.Second),
	}, nil
}

// siteValidationError maps Graph failures during settings validation.
func (c *SharePointConnector) siteValidationError(err error) error {
	var apiErr *sharePointAPIError
	if errors.As(err, &apiErr) {
		if apiErr.status == http.StatusUnauthorized || apiErr.status == http.StatusForbidden {
			return &ConnectorValidationError{Message: "Invalid credentials or insufficient permissions for SharePoint"}
		}
	}
	return &ConnectorValidationError{Message: fmt.Sprintf("SharePoint validation error: %v", err)}
}

// ---------------------------------------------------------------------------
// Document building
// ---------------------------------------------------------------------------

// sharePointDocID namespaces a Graph driveItem ID by its drive ID because
// driveItem IDs are only unique within a single drive.
func sharePointDocID(driveID, itemID string) string {
	return driveID + ":" + itemID
}

// sharePointExtension derives the file extension (with leading dot) from a
// file name, mirroring the connector's document shape.
func sharePointExtension(name string) string {
	if index := strings.LastIndex(name, "."); index >= 0 && index < len(name)-1 {
		return name[index:]
	}
	return ""
}

func (s *sharePointSyncSession) fileToDocument(drive sharePointDrive, file sharePointItem) SourceDocument {
	name := file.Name
	if name == "" {
		name = file.ID
	}
	modified := parseOutlookTime(file.LastModifiedDateTime)
	if modified.IsZero() {
		modified = time.Now().UTC()
	}

	return SourceDocument{
		SourceID:           sharePointDocID(drive.ID, file.ID),
		SemanticIdentifier: name,
		Extension:          sharePointExtension(name),
		FetchRef:           sharePointFetchRef(drive.ID, file),
		Fingerprint:        sharePointFingerprint(file),
		SkipReason:         s.connector.skipReason(file),
		UpdatedAt:          modified,
		SizeBytes:          file.Size,
		Metadata:           sharePointMetadata(drive, file),
	}
}

// ---------------------------------------------------------------------------
// Sync session with checkpoint resume
// ---------------------------------------------------------------------------

type sharePointSyncSession struct {
	connector *SharePointConnector
	request   SyncRequest
	site      *sharePointSite
	drives    []sharePointDrive
	batchSize int

	resumeAfterDriveID string
	driveIndex         int
	pending            []SourceDocument
	pendingCheckpoint  *SyncCheckpoint
	done               bool
}

// NextBatch returns the next SharePoint document batch.
func (s *sharePointSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for {
		if s.done {
			return SyncBatch{}, io.EOF
		}
		if len(s.pending) == 0 {
			if s.driveIndex >= len(s.drives) {
				s.done = true
				return SyncBatch{}, io.EOF
			}
			drive := s.drives[s.driveIndex]
			s.driveIndex++
			if drive.ID <= s.resumeAfterDriveID {
				continue
			}
			documents, err := s.driveDocuments(ctx, drive)
			if err != nil {
				return SyncBatch{}, err
			}
			if len(documents) == 0 {
				continue
			}
			s.pending = documents
			updatedAt := s.maxUpdatedAt(documents)
			checkpoint := &SyncCheckpoint{
				Cursor:    fmt.Sprintf("sharepoint_drive_%s", drive.ID),
				SourceID:  fmt.Sprintf("sharepoint_drive_%s", drive.ID),
				UpdatedAt: &updatedAt,
			}
			// The checkpoint only advances once the whole drive is flushed so
			// an interrupted run re-processes a partially committed drive.
			s.pendingCheckpoint = checkpoint
		}
		n := s.batchSize
		if n > len(s.pending) {
			n = len(s.pending)
		}
		chunk := s.pending[:n]
		s.pending = s.pending[n:]
		var checkpoint *SyncCheckpoint
		if len(s.pending) == 0 && s.pendingCheckpoint != nil {
			checkpoint = s.pendingCheckpoint
			s.pendingCheckpoint = nil
		}
		return SyncBatch{Documents: chunk, Checkpoint: checkpoint}, nil
	}
}

// Close closes the SharePoint sync session.
func (s *sharePointSyncSession) Close() error {
	return nil
}

func (s *sharePointSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	const prefix = "sharepoint_drive_"
	if sourceID == "" || !strings.HasPrefix(sourceID, prefix) {
		return fmt.Errorf("sharepoint sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	driveID := strings.TrimPrefix(sourceID, prefix)
	if driveID == "" {
		return fmt.Errorf("sharepoint sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for _, drive := range s.drives {
		if drive.ID == driveID {
			s.resumeAfterDriveID = driveID
			return nil
		}
	}
	return fmt.Errorf("sharepoint resume drive %q was not found in the current listing: %w", driveID, ErrSyncResumeInvalid)
}

func (s *sharePointSyncSession) maxUpdatedAt(documents []SourceDocument) time.Time {
	var max time.Time
	for _, document := range documents {
		if document.UpdatedAt.After(max) {
			max = document.UpdatedAt
		}
	}
	return max
}

// driveDocuments lists metadata only. The runner compares fingerprints before
// fetching individual bodies, so a whole library is never buffered as blobs.
func (s *sharePointSyncSession) driveDocuments(ctx context.Context, drive sharePointDrive) ([]SourceDocument, error) {
	files, err := s.connector.walkSelectedRoots(ctx, drive.ID)
	if err != nil {
		return nil, err
	}
	var documents []SourceDocument
	for _, file := range files {
		if !s.fileInWindow(file) {
			continue
		}
		documents = append(documents, s.fileToDocument(drive, file))
	}
	return documents, nil
}

// Enumerate older metadata too: expanding a root or removing an exclusion can
// admit files whose modification time predates the previous sync waterline.
// Fingerprints, rather than modification dates, prevent redundant downloads.
func (s *sharePointSyncSession) fileInWindow(file sharePointItem) bool {
	modified := parseOutlookTime(file.LastModifiedDateTime)
	if modified.IsZero() {
		return true
	}
	if !s.request.WindowEnd.IsZero() && modified.After(s.request.WindowEnd) {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Prune session
// ---------------------------------------------------------------------------

type sharePointPruneSession struct {
	documents []SlimDocument
	batchSize int
	offset    int
}

// NextBatch returns the next slim SharePoint snapshot batch.
func (s *sharePointPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.offset >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	n := s.batchSize
	if n <= 0 || n > len(s.documents)-s.offset {
		n = len(s.documents) - s.offset
	}
	batch := s.documents[s.offset : s.offset+n]
	s.offset += n
	return PruneBatch{Documents: batch}, nil
}

// Close closes the SharePoint prune session.
func (s *sharePointPruneSession) Close() error {
	return nil
}
