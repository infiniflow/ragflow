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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBoxBatchSize     = 2
	defaultBoxSizeThreshold = 20 * 1024 * 1024
	boxRequestTimeout       = 60 * time.Second
	boxAPIBaseURL           = "https://api.box.com/2.0"
	boxOAuthTokenURL        = "https://api.box.com/oauth2/token"
)

// BoxConnector reads files from one Box account.
type BoxConnector struct {
	folderID      string
	batchSize     int
	allowImages   bool
	sizeThreshold int64
	clientID      string
	clientSecret  string
	accessToken   string
	refreshToken  string

	apiBaseURL string
	tokenURL   string
	httpClient *http.Client

	tokenMu sync.Mutex

	listFolderItems func(ctx context.Context, folderID, marker string, limit int) (boxItemsPage, error)
	getFile         func(ctx context.Context, fileID string) (boxFile, error)
	downloadFile    func(ctx context.Context, fileID string, sizeThreshold int64) ([]byte, error)
}

// NewBoxConnector creates a Box connector from Python-compatible config.
func NewBoxConnector(config map[string]any) (*BoxConnector, error) {
	credentials := configAnyMap(config["credentials"])
	clientID, clientSecret, accessToken, refreshToken, err := boxCredentials(credentials)
	if err != nil {
		return nil, err
	}
	folderID := strings.TrimSpace(stringConfig(config["folder_id"]))
	if folderID == "" {
		folderID = "0"
	}
	batchSize := configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultBoxBatchSize)
	sizeThreshold := int64(configInt(config["size_threshold"], defaultBoxSizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultBoxSizeThreshold
	}
	connector := &BoxConnector{
		folderID:      folderID,
		batchSize:     batchSize,
		allowImages:   configBoolDefault(config["allow_images"], false),
		sizeThreshold: sizeThreshold,
		clientID:      clientID,
		clientSecret:  clientSecret,
		accessToken:   accessToken,
		refreshToken:  refreshToken,
		apiBaseURL:    boxAPIBaseURL,
		tokenURL:      boxOAuthTokenURL,
		httpClient: &http.Client{
			Timeout: boxRequestTimeout,
		},
	}
	connector.listFolderItems = connector.defaultListFolderItems
	connector.getFile = connector.defaultGetFile
	connector.downloadFile = connector.defaultDownloadFile
	return connector, nil
}

// Validate validates Box settings and credentials against the Box API.
func (c *BoxConnector) Validate(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}
	if err := c.getCurrentUser(ctx); err != nil {
		return fmt.Errorf("Box validation failed: %w", err)
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved Box config.
func (c *BoxConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	if c == nil {
		return fmt.Errorf("box connector is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	connector, err := NewBoxConnector(request)
	if err != nil {
		return err
	}
	connector.apiBaseURL = c.apiBaseURL
	connector.tokenURL = c.tokenURL
	connector.httpClient = c.httpClient
	connector.listFolderItems = c.listFolderItems
	connector.getFile = c.getFile
	connector.downloadFile = c.downloadFile
	if err := connector.getCurrentUser(ctx); err != nil {
		return fmt.Errorf("Box validation failed: %w", err)
	}
	return nil
}

// OpenSync opens one Box sync session without listing the source up front.
func (c *BoxConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	return &boxSyncSession{
		connector: c,
		request:   request,
		batchSize: c.batchSize,
	}, nil
}

// OpenPrune opens one complete Box prune snapshot session.
func (c *BoxConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateConfig(); err != nil {
		return nil, err
	}
	return &boxPruneSession{
		connector: c,
		batchSize: c.batchSize,
	}, nil
}

// Fetch downloads a Box file referenced by a previous sync batch.
func (c *BoxConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch boxFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	if strings.TrimSpace(fetch.FileID) == "" {
		return nil, fmt.Errorf("Box fetch reference has no file id")
	}
	blob, err := c.downloadFile(ctx, fetch.FileID, c.sizeThreshold)
	if err != nil {
		return nil, err
	}
	if int64(len(blob)) > c.sizeThreshold {
		return nil, fmt.Errorf("Box file %s exceeds size threshold", fetch.FileID)
	}
	return blob, nil
}

func (c *BoxConnector) validateConfig() error {
	if c == nil {
		return fmt.Errorf("box connector is nil")
	}
	if strings.TrimSpace(c.folderID) == "" {
		return fmt.Errorf("Box folder_id is required")
	}
	if strings.TrimSpace(c.clientID) == "" || strings.TrimSpace(c.clientSecret) == "" || strings.TrimSpace(c.accessToken) == "" || strings.TrimSpace(c.refreshToken) == "" {
		return fmt.Errorf("Box credentials must include client_id, client_secret, access_token, and refresh_token")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	return nil
}

func (c *BoxConnector) getCurrentUser(ctx context.Context) error {
	var user map[string]any
	return c.doRequest(ctx, http.MethodGet, c.apiBaseURL+"/users/me", &user)
}

func (c *BoxConnector) defaultListFolderItems(ctx context.Context, folderID, marker string, limit int) (boxItemsPage, error) {
	if limit <= 0 {
		limit = c.batchSize
		if limit <= 0 {
			limit = defaultBoxBatchSize
		}
	}
	endpoint := c.apiBaseURL + "/folders/" + url.PathEscape(folderID) + "/items?usemarker=true&limit=" + strconv.Itoa(limit)
	if marker != "" {
		endpoint += "&marker=" + url.QueryEscape(marker)
	}
	var page boxItemsPage
	err := c.doRequest(ctx, http.MethodGet, endpoint, &page)
	return page, err
}

func (c *BoxConnector) defaultGetFile(ctx context.Context, fileID string) (boxFile, error) {
	var file boxFile
	err := c.doRequest(ctx, http.MethodGet, c.apiBaseURL+"/files/"+url.PathEscape(fileID), &file)
	return file, err
}

func (c *BoxConnector) defaultDownloadFile(ctx context.Context, fileID string, sizeThreshold int64) ([]byte, error) {
	limit := sizeThreshold
	if limit <= 0 {
		limit = defaultBoxSizeThreshold
	}
	return c.getBytes(ctx, c.apiBaseURL+"/files/"+url.PathEscape(fileID)+"/content", limit)
}

func (c *BoxConnector) doRequest(ctx context.Context, method, endpoint string, out any) error {
	return c.doRequestOnce(ctx, method, endpoint, out, true)
}

func (c *BoxConnector) doRequestOnce(ctx context.Context, method, endpoint string, out any, allowRefresh bool) error {
	ctx, cancel := context.WithTimeout(ctx, boxRequestTimeout)
	defer cancel()
	token, err := c.currentAccessToken()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized && allowRefresh {
		_ = resp.Body.Close()
		if err := c.refreshAccessToken(ctx); err != nil {
			return fmt.Errorf("refresh Box access token: %w", err)
		}
		return c.doRequestOnce(ctx, method, endpoint, out, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return boxHTTPError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *BoxConnector) getBytes(ctx context.Context, endpoint string, limit int64) ([]byte, error) {
	return c.getBytesOnce(ctx, endpoint, limit, true)
}

func (c *BoxConnector) getBytesOnce(ctx context.Context, endpoint string, limit int64, allowRefresh bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, boxRequestTimeout)
	defer cancel()
	token, err := c.currentAccessToken()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && allowRefresh {
		_ = resp.Body.Close()
		if err := c.refreshAccessToken(ctx); err != nil {
			return nil, fmt.Errorf("refresh Box access token: %w", err)
		}
		return c.getBytesOnce(ctx, endpoint, limit, false)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, boxHTTPError(resp)
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(blob)) > limit {
		return nil, fmt.Errorf("Box file exceeds maximum size of %d bytes", limit)
	}
	return blob, nil
}

func (c *BoxConnector) currentAccessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.accessToken == "" {
		return "", fmt.Errorf("Box access token is empty")
	}
	return c.accessToken, nil
}

func (c *BoxConnector) refreshAccessToken(ctx context.Context) error {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.clientID == "" || c.clientSecret == "" || c.refreshToken == "" {
		return fmt.Errorf("Box OAuth credentials are incomplete")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", c.refreshToken)
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)

	ctx, cancel := context.WithTimeout(ctx, boxRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var token boxTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest || token.Error != "" {
		if token.ErrorDescription != "" {
			return fmt.Errorf("Box token refresh failed: %s", token.ErrorDescription)
		}
		if token.Error != "" {
			return fmt.Errorf("Box token refresh failed: %s", token.Error)
		}
		return fmt.Errorf("Box token refresh failed: HTTP %d", resp.StatusCode)
	}
	if token.AccessToken == "" {
		return fmt.Errorf("Box token refresh failed: empty access_token")
	}
	c.accessToken = token.AccessToken
	if token.RefreshToken != "" {
		c.refreshToken = token.RefreshToken
	}
	return nil
}

func (c *BoxConnector) collectFiles(ctx context.Context) ([]boxFile, error) {
	var files []boxFile
	seenFiles := map[string]struct{}{}
	seenFolders := map[string]struct{}{}
	var walk func(folderID, relativePath string) error
	walk = func(folderID, relativePath string) error {
		if _, ok := seenFolders[folderID]; ok {
			return nil
		}
		seenFolders[folderID] = struct{}{}
		marker := ""
		for {
			page, err := c.listFolderItems(ctx, folderID, marker, c.batchSize)
			if err != nil {
				return err
			}
			for _, entry := range page.Entries {
				switch entry.Type {
				case "file":
					if entry.ID == "" {
						continue
					}
					if _, ok := seenFiles[entry.ID]; ok {
						continue
					}
					file, err := c.getFile(ctx, entry.ID)
					if err != nil {
						return err
					}
					file.relativePath = relativePath
					seenFiles[entry.ID] = struct{}{}
					files = append(files, file)
				case "folder":
					if entry.ID == "" {
						continue
					}
					childPath := boxRelativePath(relativePath, entry.Name)
					if err := walk(entry.ID, childPath); err != nil {
						return err
					}
				}
			}
			if page.NextMarker == "" {
				break
			}
			marker = page.NextMarker
		}
		return nil
	}
	if err := walk(c.folderID, ""); err != nil {
		return nil, err
	}
	return files, nil
}

func (c *BoxConnector) isAcceptedFile(file boxFile) bool {
	if strings.TrimSpace(file.ID) == "" || strings.TrimSpace(file.Name) == "" {
		return false
	}
	if file.Size == nil || *file.Size < 0 || *file.Size > c.sizeThreshold {
		return false
	}
	extension := boxExtension(file.Name)
	if _, ok := webdavTextExtensions[extension]; ok {
		return true
	}
	if _, ok := webdavDocumentExtensions[extension]; ok {
		return true
	}
	if c.allowImages {
		if _, ok := webdavImageExtensions[extension]; ok {
			return true
		}
	}
	return false
}

func (c *BoxConnector) sourceDocument(file boxFile) (SourceDocument, bool) {
	if !c.isAcceptedFile(file) {
		return SourceDocument{}, false
	}
	size := int64(0)
	if file.Size != nil {
		size = *file.Size
	}
	fetchKey, _ := json.Marshal(boxFetchReference{FileID: file.ID, Name: file.Name, Size: size})
	metadata := map[string]any{
		"file_id": file.ID,
		"name":    file.Name,
		"url":     boxFileURL(file),
	}
	if len(file.Metadata) > 0 {
		var rawMetadata map[string]any
		if json.Unmarshal(file.Metadata, &rawMetadata) == nil && len(rawMetadata) > 0 {
			metadata["metadata"] = rawMetadata
		}
	}
	return SourceDocument{
		SourceID:           boxSourceID(file.ID),
		SemanticIdentifier: boxSemanticIdentifier(file),
		Extension:          boxExtension(file.Name),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: size},
		UpdatedAt:          file.updatedAt(),
		SizeBytes:          size,
		Metadata:           metadata,
		Fingerprint:        file.fingerprint(),
	}, true
}

func (c *BoxConnector) acceptedSlimDocuments(files []boxFile) []SlimDocument {
	documents := make([]SlimDocument, 0, len(files))
	for _, file := range files {
		if c.isAcceptedFile(file) {
			documents = append(documents, SlimDocument{SourceID: boxSourceID(file.ID)})
		}
	}
	return documents
}

type boxSyncSession struct {
	connector *BoxConnector
	request   SyncRequest
	batchSize int

	documents []SourceDocument
	index     int
	loaded    bool
}

// NextBatch returns the next Box source document batch.
func (s *boxSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if !s.loaded {
		if err := s.load(ctx); err != nil {
			return SyncBatch{}, err
		}
	}
	if s.index >= len(s.documents) {
		return SyncBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	documents := s.documents[s.index:end]
	s.index = end
	return SyncBatch{Documents: documents, Checkpoint: boxSyncCheckpoint(documents[len(documents)-1])}, nil
}

// Close closes the Box sync session.
func (s *boxSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed Box document body.
func (s *boxSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *boxSyncSession) load(ctx context.Context) error {
	files, err := s.connector.collectFiles(ctx)
	if err != nil {
		return err
	}
	documents := make([]SourceDocument, 0, len(files))
	for _, file := range files {
		if !includeBoxFile(s.request, file) {
			continue
		}
		document, ok := s.connector.sourceDocument(file)
		if ok {
			documents = append(documents, document)
		}
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	s.documents = documents
	s.loaded = true
	return s.applyResume(s.request.Resume)
}

// applyResume advances past the last committed Box document.
func (s *boxSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("box sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if !strings.HasPrefix(sourceID, "box:") || sourceID == "box:" {
		return fmt.Errorf("box sync checkpoint source %q is invalid: %w", sourceID, ErrSyncResumeInvalid)
	}
	for index, document := range s.documents {
		if document.SourceID == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("box resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

func boxSyncCheckpoint(document SourceDocument) *SyncCheckpoint {
	updatedAt := document.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    document.SourceID,
		SourceID:  document.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type boxPruneSession struct {
	connector *BoxConnector
	batchSize int

	documents []SlimDocument
	index     int
	loaded    bool
}

// NextBatch returns the next Box prune snapshot batch.
func (s *boxPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if !s.loaded {
		if err := s.load(ctx); err != nil {
			return PruneBatch{}, err
		}
	}
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	documents := s.documents[s.index:end]
	s.index = end
	return PruneBatch{Documents: documents}, nil
}

// Close closes the Box prune session.
func (s *boxPruneSession) Close() error {
	return nil
}

func (s *boxPruneSession) load(ctx context.Context) error {
	files, err := s.connector.collectFiles(ctx)
	if err != nil {
		return err
	}
	documents := s.connector.acceptedSlimDocuments(files)
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	s.documents = documents
	s.loaded = true
	return nil
}

type boxItemsPage struct {
	Entries    []boxItemEntry `json:"entries"`
	NextMarker string         `json:"next_marker"`
}

type boxItemEntry struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type boxFile struct {
	Type              string          `json:"type"`
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Size              *int64          `json:"size"`
	ETag              string          `json:"etag"`
	ModifiedAt        string          `json:"modified_at"`
	ContentModifiedAt string          `json:"content_modified_at"`
	CreatedAt         string          `json:"created_at"`
	WebLink           string          `json:"web_link"`
	Metadata          json.RawMessage `json:"metadata"`
	FileVersion       *boxFileVersion `json:"file_version"`

	relativePath string
}

type boxFileVersion struct {
	SHA1 string `json:"sha1"`
}

type boxTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type boxFetchReference struct {
	FileID string `json:"file_id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

func boxCredentials(credentials map[string]any) (clientID, clientSecret, accessToken, refreshToken string, err error) {
	value, ok := credentials["box_tokens"]
	if !ok || value == nil {
		return "", "", "", "", nil
	}
	var payload struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", "", "", "", nil
		}
		if err := json.Unmarshal([]byte(typed), &payload); err != nil {
			return "", "", "", "", fmt.Errorf("parse box_tokens: %w", err)
		}
	case map[string]any:
		payload.ClientID = stringConfig(typed["client_id"])
		payload.ClientSecret = stringConfig(typed["client_secret"])
		payload.AccessToken = stringConfig(typed["access_token"])
		payload.RefreshToken = stringConfig(typed["refresh_token"])
	default:
		return "", "", "", "", fmt.Errorf("box_tokens must be a JSON string or object")
	}
	return strings.TrimSpace(payload.ClientID), strings.TrimSpace(payload.ClientSecret), strings.TrimSpace(payload.AccessToken), strings.TrimSpace(payload.RefreshToken), nil
}

func boxRelativePath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + " / " + name
}

func boxSemanticIdentifier(file boxFile) string {
	if file.relativePath == "" {
		return file.Name
	}
	return file.relativePath + " / " + file.Name
}

func boxSourceID(fileID string) string {
	return "box:" + fileID
}

func boxExtension(name string) string {
	return strings.ToLower(path.Ext(name))
}

func boxFileURL(file boxFile) string {
	if file.WebLink != "" {
		return file.WebLink
	}
	return "https://app.box.com/file/" + url.PathEscape(file.ID)
}

func includeBoxFile(request SyncRequest, file boxFile) bool {
	if request.FromBeginning {
		return true
	}
	updatedAt := file.updatedAt()
	if updatedAt.IsZero() {
		return true
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := file.fingerprint()
		stored, ok := request.Fingerprints[boxSourceID(file.ID)]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	return !beforeOrAtWindowStart(updatedAt, request.WindowStart) && !afterWindowEnd(updatedAt, request.WindowEnd)
}

func (f boxFile) updatedAt() time.Time {
	for _, raw := range []string{f.ModifiedAt, f.ContentModifiedAt, f.CreatedAt} {
		if parsed := parseBoxTime(raw); !parsed.IsZero() {
			return parsed
		}
	}
	return time.Now().UTC()
}

func (f boxFile) fingerprint() string {
	sha1 := ""
	if f.FileVersion != nil {
		sha1 = f.FileVersion.SHA1
	}
	var size any
	if f.Size != nil {
		size = *f.Size
	}
	return stableFingerprint(map[string]any{
		"id":                  f.ID,
		"name":                f.Name,
		"path":                f.relativePath,
		"size":                size,
		"modified_at":         f.ModifiedAt,
		"content_modified_at": f.ContentModifiedAt,
		"etag":                f.ETag,
		"sha1":                sha1,
	})
}

func parseBoxTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func boxHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(data))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("Box request failed with status %d: %s", resp.StatusCode, message)
}
