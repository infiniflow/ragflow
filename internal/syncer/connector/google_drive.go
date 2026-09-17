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
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultGoogleDriveBatchSize     = 32
	defaultGoogleDriveSizeThreshold = 64 * 1024 * 1024
	googleDriveItemsPerPage         = 100
	googleDriveRequestTimeout       = 60 * time.Second
	googleDriveOAuthTokenURL        = "https://oauth2.googleapis.com/token"
	googleDriveListRetryCount       = 4
	googleDriveListRetryBaseDelay   = 200 * time.Millisecond

	googleDriveFolderMimeType   = "application/vnd.google-apps.folder"
	googleDriveShortcutMimeType = "application/vnd.google-apps.shortcut"
)

var googleDriveScopes = []string{
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/drive.metadata.readonly",
	"https://www.googleapis.com/auth/admin.directory.group.readonly",
	"https://www.googleapis.com/auth/admin.directory.user.readonly",
}

var googleDriveNativeExports = map[string]googleDriveExport{
	"application/vnd.google-apps.document":     {mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", extension: ".docx"},
	"application/vnd.google-apps.spreadsheet":  {mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", extension: ".xlsx"},
	"application/vnd.google-apps.presentation": {mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", extension: ".pptx"},
}

// GoogleDriveConnector reads files from Google Drive.
type GoogleDriveConnector struct {
	includeSharedDrives      bool
	includeMyDrives          bool
	includeFilesSharedWithMe bool
	allowImages              bool
	sharedDriveIDs           []string
	myDriveEmails            []string
	sharedFolderIDs          []string
	specificUserEmails       []string
	specificRequests         bool
	primaryAdminEmail        string
	credentials              map[string]any
	batchSize                int
	sizeThreshold            int64
	clientsMu                sync.Mutex
	clients                  map[string]*http.Client
	httpClientForUser        func(ctx context.Context, userEmail string) (*http.Client, error)
	listUsers                func(ctx context.Context) ([]string, error)
	listDrives               func(ctx context.Context, userEmail string) ([]googleDriveDrive, error)
	listFiles                func(ctx context.Context, userEmail string, request googleDriveListRequest) (googleDriveFilePage, error)
	listFolders              func(ctx context.Context, userEmail, parentID string) ([]string, error)
	downloadFile             func(ctx context.Context, userEmail string, file googleDriveFile) ([]byte, string, error)
}

// NewGoogleDriveConnector creates a Google Drive connector from Python-compatible config.
func NewGoogleDriveConnector(config map[string]any) (*GoogleDriveConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	specificRequests := stringConfig(config["shared_drive_urls"]) != "" || stringConfig(config["my_drive_emails"]) != "" || stringConfig(config["shared_folder_urls"]) != ""
	includeSharedWithMe := configBoolDefault(config["include_files_shared_with_me"], false)
	if specificRequests {
		includeSharedWithMe = false
	}
	sizeThreshold := int64(configInt(config["size_threshold"], defaultGoogleDriveSizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = defaultGoogleDriveSizeThreshold
	}
	return &GoogleDriveConnector{
		includeSharedDrives:      !specificRequests && configBoolDefault(config["include_shared_drives"], false),
		includeMyDrives:          !specificRequests && configBoolDefault(config["include_my_drives"], false),
		includeFilesSharedWithMe: includeSharedWithMe,
		allowImages:              configBoolDefault(config["allow_images"], false),
		sharedDriveIDs:           googleDriveIDsFromURLs(stringConfig(config["shared_drive_urls"])),
		myDriveEmails:            splitCommaList(stringConfig(config["my_drive_emails"])),
		sharedFolderIDs:          googleDriveIDsFromURLs(stringConfig(config["shared_folder_urls"])),
		specificUserEmails:       splitCommaList(stringConfig(config["specific_user_emails"])),
		specificRequests:         specificRequests,
		primaryAdminEmail:        strings.TrimSpace(stringConfig(credentials["google_primary_admin"])),
		credentials:              credentials,
		batchSize:                configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), defaultGoogleDriveBatchSize),
		sizeThreshold:            sizeThreshold,
		clients:                  map[string]*http.Client{},
	}, nil
}

// Validate validates Google Drive connector settings and credentials.
func (c *GoogleDriveConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("google drive connector is nil")
	}
	if c.primaryAdminEmail == "" {
		return fmt.Errorf("Google Drive connector is missing google_primary_admin")
	}
	if len(c.credentials) == 0 {
		return fmt.Errorf("Google Drive connector is missing credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	if !c.hasRetrievalScope() {
		return fmt.Errorf("Nothing to index. Please specify include_shared_drives, include_my_drives, include_files_shared_with_me, shared_drive_urls, shared_folder_urls, or my_drive_emails")
	}
	_, err := c.clientForUser(ctx, c.primaryAdminEmail)
	return err
}

// ValidateConnectorSetting validates Google Drive settings from an unsaved config.
func (c *GoogleDriveConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	if err := c.Validate(ctx); err != nil {
		return err
	}
	client, err := c.clientForUser(ctx, c.primaryAdminEmail)
	if err != nil {
		return err
	}
	var about map[string]any
	return c.getJSON(ctx, client, "https://www.googleapis.com/drive/v3/about?fields=user", &about)
}

// OpenSync opens one Google Drive sync session.
func (c *GoogleDriveConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	scopes, err := c.buildScopes(ctx)
	if err != nil {
		return nil, err
	}
	session := &googleDriveSyncSession{
		connector:   c,
		scopes:      scopes,
		scopeIndex:  0,
		batchSize:   c.batchSize,
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Google Drive prune snapshot session.
func (c *GoogleDriveConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	scopes, err := c.buildScopes(ctx)
	if err != nil {
		return nil, err
	}
	return &googleDrivePruneSession{connector: c, scopes: scopes, batchSize: c.batchSize}, nil
}

// Fetch downloads a Google Drive file after the sync runner determines it changed.
func (c *GoogleDriveConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch googleDriveFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	file := googleDriveFile{
		ID:       fetch.FileID,
		Name:     fetch.Name,
		MimeType: fetch.MimeType,
		Size:     fetch.Size,
	}
	blob, _, err := c.fetchBlob(ctx, fetch.UserEmail, file)
	return blob, err
}

func (c *GoogleDriveConnector) hasRetrievalScope() bool {
	return c.includeSharedDrives || c.includeMyDrives || c.includeFilesSharedWithMe || len(c.sharedDriveIDs) > 0 || len(c.sharedFolderIDs) > 0 || len(c.myDriveEmails) > 0
}

func (c *GoogleDriveConnector) buildScopes(ctx context.Context) ([]googleDriveScope, error) {
	users, err := c.userEmails(ctx)
	if err != nil {
		return nil, err
	}
	scopes := make([]googleDriveScope, 0)
	includeSharedWithMe := c.effectiveIncludeSharedWithMe()
	for _, email := range users {
		if c.includeMyDrives || containsString(c.myDriveEmails, email) {
			scopes = append(scopes, googleDriveScope{userEmail: email, corpora: "user", includeSharedWithMe: includeSharedWithMe})
		}
	}
	if includeSharedWithMe && !c.includeMyDrives {
		for _, email := range users {
			scopes = append(scopes, googleDriveScope{userEmail: email, corpora: "user", sharedWithMeOnly: true})
		}
	}

	driveIDs := append([]string(nil), c.sharedDriveIDs...)
	if c.includeSharedDrives {
		allDrives, err := c.allDriveIDs(ctx, c.primaryAdminEmail)
		if err != nil {
			return nil, err
		}
		driveIDs = append(driveIDs, allDrives...)
	}
	driveIDs = uniqueSorted(driveIDs)
	for _, driveID := range driveIDs {
		scopes = append(scopes, googleDriveScope{userEmail: c.primaryAdminEmail, corpora: "drive", driveID: driveID})
	}
	for _, folderID := range c.sharedFolderIDs {
		scopes = append(scopes, googleDriveScope{userEmail: c.primaryAdminEmail, corpora: "folder", folderID: folderID})
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("Google Drive connector has no effective retrieval scope")
	}
	return scopes, nil
}

func (c *GoogleDriveConnector) userEmails(ctx context.Context) ([]string, error) {
	if len(c.specificUserEmails) > 0 {
		return uniqueSorted(c.specificUserEmails), nil
	}
	if len(c.myDriveEmails) > 0 {
		return uniqueSorted(c.myDriveEmails), nil
	}
	if c.listUsers != nil {
		return c.listUsers(ctx)
	}
	if !c.isServiceAccount() {
		return []string{c.primaryAdminEmail}, nil
	}
	client, err := c.clientForUser(ctx, c.primaryAdminEmail)
	if err != nil {
		return nil, err
	}
	domain := c.primaryAdminEmail
	if _, after, ok := strings.Cut(c.primaryAdminEmail, "@"); ok {
		domain = after
	}
	users := []string{c.primaryAdminEmail}
	for _, queryText := range []string{"isAdmin=true", "isAdmin=false"} {
		pageToken := ""
		for {
			query := url.Values{"domain": {domain}, "fields": {"nextPageToken,users(primaryEmail)"}, "maxResults": {"500"}, "query": {queryText}}
			if pageToken != "" {
				query.Set("pageToken", pageToken)
			}
			var page googleDriveUsersPage
			if err = c.getJSON(ctx, client, "https://admin.googleapis.com/admin/directory/v1/users?"+query.Encode(), &page); err != nil {
				return nil, err
			}
			for _, user := range page.Users {
				if user.PrimaryEmail != "" && !containsString(users, user.PrimaryEmail) {
					users = append(users, user.PrimaryEmail)
				}
			}
			if page.NextPageToken == "" {
				break
			}
			pageToken = page.NextPageToken
		}
	}
	return users, nil
}

func (c *GoogleDriveConnector) allDriveIDs(ctx context.Context, userEmail string) ([]string, error) {
	if c.listDrives != nil {
		drives, err := c.listDrives(ctx, userEmail)
		if err != nil {
			return nil, err
		}
		return googleDriveIDs(drives), nil
	}
	client, err := c.clientForUser(ctx, userEmail)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	pageToken := ""
	for {
		query := url.Values{"fields": {"nextPageToken,drives(id)"}, "pageSize": {"100"}}
		if c.isServiceAccount() {
			query.Set("useDomainAdminAccess", "true")
		}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var page googleDriveDrivePage
		if err = c.getJSON(ctx, client, "https://www.googleapis.com/drive/v3/drives?"+query.Encode(), &page); err != nil {
			return nil, err
		}
		ids = append(ids, googleDriveIDs(page.Drives)...)
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	return uniqueSorted(ids), nil
}

func (c *GoogleDriveConnector) listFilePage(ctx context.Context, scope googleDriveScope, pageToken string, windowStart *time.Time, windowEnd time.Time) (googleDriveFilePage, error) {
	if c.listFiles != nil {
		return c.listFiles(ctx, scope.userEmail, googleDriveListRequest{Scope: scope, PageToken: pageToken, WindowStart: windowStart, WindowEnd: windowEnd, PageSize: googleDriveItemsPerPage})
	}
	client, err := c.clientForUser(ctx, scope.userEmail)
	if err != nil {
		return googleDriveFilePage{}, err
	}
	query := url.Values{
		"fields":   {"nextPageToken,files(id,name,mimeType,modifiedTime,createdTime,webViewLink,shortcutDetails,owners(emailAddress),size,md5Checksum,fileExtension)"},
		"pageSize": {strconv.Itoa(googleDriveItemsPerPage)},
		"orderBy":  {"modifiedTime"},
		"q":        {googleDriveFileQuery(scope, windowStart, windowEnd)},
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	if scope.corpora == "drive" {
		query.Set("corpora", "drive")
		query.Set("driveId", scope.driveID)
		query.Set("supportsAllDrives", "true")
		query.Set("includeItemsFromAllDrives", "true")
	} else if scope.corpora == "folder" {
		query.Set("corpora", "allDrives")
		query.Set("supportsAllDrives", "true")
		query.Set("includeItemsFromAllDrives", "true")
	} else if scope.includeSharedWithMe && !scope.sharedWithMeOnly {
		query.Set("corpora", "allDrives")
		query.Set("supportsAllDrives", "true")
		query.Set("includeItemsFromAllDrives", "true")
	} else {
		query.Set("corpora", "user")
	}
	var page googleDriveFilePage
	err = c.getJSON(ctx, client, "https://www.googleapis.com/drive/v3/files?"+query.Encode(), &page)
	return page, err
}

func (c *GoogleDriveConnector) listFilePageWithRetry(ctx context.Context, scope googleDriveScope, pageToken string, windowStart *time.Time, windowEnd time.Time) (googleDriveFilePage, error) {
	var lastErr error
	for attempt := 1; attempt <= googleDriveListRetryCount; attempt++ {
		page, err := c.listFilePage(ctx, scope, pageToken, windowStart, windowEnd)
		if err == nil {
			return page, nil
		}
		lastErr = err
		if !isGoogleRateLimited(err) || attempt == googleDriveListRetryCount {
			break
		}
		delay := googleDriveListRetryBaseDelay * time.Duration(1<<(attempt-1))
		select {
		case <-ctx.Done():
			return googleDriveFilePage{}, ctx.Err()
		case <-time.After(delay):
		}
	}
	return googleDriveFilePage{}, lastErr
}

func (c *GoogleDriveConnector) listFolderIDs(ctx context.Context, userEmail, parentID string) ([]string, error) {
	if c.listFolders != nil {
		return c.listFolders(ctx, userEmail, parentID)
	}
	client, err := c.clientForUser(ctx, userEmail)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	pageToken := ""
	for {
		query := url.Values{
			"corpora":                   {"allDrives"},
			"fields":                    {"nextPageToken,files(id)"},
			"includeItemsFromAllDrives": {"true"},
			"supportsAllDrives":         {"true"},
			"pageSize":                  {strconv.Itoa(googleDriveItemsPerPage)},
			"q":                         {fmt.Sprintf("mimeType = '%s' and trashed = false and '%s' in parents", googleDriveFolderMimeType, parentID)},
		}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		var page googleDriveFilePage
		if err = c.getJSON(ctx, client, "https://www.googleapis.com/drive/v3/files?"+query.Encode(), &page); err != nil {
			return nil, err
		}
		for _, file := range page.Files {
			if file.ID != "" {
				ids = append(ids, file.ID)
			}
		}
		if page.NextPageToken == "" {
			break
		}
		pageToken = page.NextPageToken
	}
	return uniqueSorted(ids), nil
}

func (c *GoogleDriveConnector) fetchBlob(ctx context.Context, userEmail string, file googleDriveFile) ([]byte, string, error) {
	if c.downloadFile != nil {
		return c.downloadFile(ctx, userEmail, file)
	}
	if file.isFolderLike() {
		return nil, "", fmt.Errorf("Google Drive file %s is not downloadable", file.ID)
	}
	if file.isImage() && !c.allowImages {
		return nil, "", fmt.Errorf("Google Drive image %s is disabled", file.ID)
	}
	if file.sizeInt() > c.sizeThreshold && file.sizeInt() > 0 {
		return nil, "", fmt.Errorf("Google Drive file %s exceeds size threshold", file.ID)
	}
	client, err := c.clientForUser(ctx, userEmail)
	if err != nil {
		return nil, "", err
	}
	export, isNative := googleDriveNativeExports[file.MimeType]
	extension := file.extension()
	apiURL := "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(file.ID) + "?alt=media"
	if isNative {
		extension = export.extension
		apiURL = "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(file.ID) + "/export?mimeType=" + url.QueryEscape(export.mimeType)
	} else if strings.HasPrefix(file.MimeType, "application/vnd.google-apps") {
		extension = ".pdf"
		apiURL = "https://www.googleapis.com/drive/v3/files/" + url.PathEscape(file.ID) + "/export?mimeType=" + url.QueryEscape("application/pdf")
	}
	blob, err := c.getBytes(ctx, client, apiURL, c.sizeThreshold)
	return blob, extension, err
}

func (c *GoogleDriveConnector) clientForUser(ctx context.Context, userEmail string) (*http.Client, error) {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()
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

func (c *GoogleDriveConnector) tokenSource(ctx context.Context, userEmail string) (oauth2.TokenSource, error) {
	if value := stringConfig(c.credentials["google_service_account_key"]); value != "" {
		config, err := google.JWTConfigFromJSON([]byte(value), googleDriveScopes...)
		if err != nil {
			return nil, err
		}
		config.Subject = userEmail
		return config.TokenSource(ctx), nil
	}
	tokenJSON := stringConfig(c.credentials["google_tokens"])
	if tokenJSON == "" {
		return nil, fmt.Errorf("Google Drive connector credentials must include google_tokens or google_service_account_key")
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
		return nil, fmt.Errorf("Google Drive OAuth credentials are incomplete")
	}
	return (&oauth2.Config{
		ClientID:     tokenData.ClientID,
		ClientSecret: tokenData.ClientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: googleDriveOAuthTokenURL},
		Scopes:       googleDriveScopes,
	}).TokenSource(ctx, tokenData.token()), nil
}

func (c *GoogleDriveConnector) getJSON(ctx context.Context, client *http.Client, apiURL string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, googleDriveRequestTimeout)
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

func (c *GoogleDriveConnector) getBytes(ctx context.Context, client *http.Client, apiURL string, sizeThreshold int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, googleDriveRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, googleHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, sizeThreshold+1))
	if err != nil {
		return nil, err
	}
	if int64(len(blob)) > sizeThreshold {
		return nil, fmt.Errorf("Google Drive file exceeds size threshold")
	}
	return blob, nil
}

type googleDriveSyncSession struct {
	connector        *GoogleDriveConnector
	scopes           []googleDriveScope
	scopeIndex       int
	pageToken        string
	batchSize        int
	windowStart      *time.Time
	windowEnd        time.Time
	buffer           []googleDriveBufferedDocument
	seen             map[string]struct{}
	folderSeen       map[string]struct{}
	resumePageToken  string
	resumePageOffset int
	resumeSourceID   string
}

// NextBatch returns the next Google Drive document batch.
func (s *googleDriveSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	if s.folderSeen == nil {
		s.folderSeen = map[string]struct{}{}
	}
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
		if s.scopeIndex >= len(s.scopes) {
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

func (s *googleDriveSyncSession) nextDocumentPage(ctx context.Context) ([]googleDriveBufferedDocument, error) {
	scope := s.scopes[s.scopeIndex]
	requestPageToken := s.pageToken
	page, err := s.connector.listFilePageWithRetry(ctx, scope, requestPageToken, s.windowStart, s.windowEnd)
	if err != nil {
		if isGooglePermissionDeniedOrNotFound(err) {
			s.advanceScope()
			return nil, nil
		}
		return nil, err
	}
	candidates := make([]googleDriveBufferedDocument, 0, len(page.Files))
	pageOffset := 0
	for _, file := range page.Files {
		doc, ok := file.toSourceDocument(scope.userEmail, s.connector.allowImages)
		if !ok {
			continue
		}
		pageOffset++
		candidates = append(candidates, googleDriveBufferedDocument{
			document:   doc,
			checkpoint: s.syncCheckpoint(scope, requestPageToken, pageOffset, doc),
			offset:     pageOffset,
		})
	}
	docs, err := s.filterResumedDocuments(requestPageToken, candidates)
	if err != nil {
		return nil, err
	}
	docs = s.filterSeenDocuments(docs)
	if page.NextPageToken == "" {
		if err = s.finishScope(ctx, scope); err != nil {
			return nil, err
		}
	} else {
		s.pageToken = page.NextPageToken
	}
	return docs, nil
}

func (s *googleDriveSyncSession) finishScope(ctx context.Context, scope googleDriveScope) error {
	if scope.corpora == "folder" {
		childIDs, err := s.connector.listFolderIDs(ctx, scope.userEmail, scope.folderID)
		if err != nil {
			if !isGooglePermissionDeniedOrNotFound(err) {
				return err
			}
		}
		for _, folderID := range childIDs {
			if _, ok := s.folderSeen[folderID]; ok {
				continue
			}
			s.folderSeen[folderID] = struct{}{}
			s.scopes = append(s.scopes, googleDriveScope{userEmail: scope.userEmail, corpora: "folder", folderID: folderID})
		}
	}
	s.advanceScope()
	return nil
}

func (s *googleDriveSyncSession) advanceScope() {
	s.scopeIndex++
	s.pageToken = ""
	s.clearResumeOffset()
}

// Close closes the Google Drive sync session.
func (s *googleDriveSyncSession) Close() error {
	return nil
}

// Fetch downloads a Google Drive file for this sync session.
func (s *googleDriveSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *googleDriveSyncSession) filterSeenDocuments(candidates []googleDriveBufferedDocument) []googleDriveBufferedDocument {
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if _, ok := s.seen[candidate.document.SourceID]; ok {
			continue
		}
		s.seen[candidate.document.SourceID] = struct{}{}
		filtered = append(filtered, candidate)
	}
	return filtered
}

// applyResume restores the Google Drive source anchor when a checkpoint is available.
func (s *googleDriveSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("google drive sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor googleDriveSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("google drive sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" {
		return fmt.Errorf("google drive sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if len(cursor.Scopes) > 0 && cursor.ScopeIndex >= 0 && cursor.ScopeIndex < len(cursor.Scopes) {
		scopes := make([]googleDriveScope, 0, len(cursor.Scopes))
		for _, scope := range cursor.Scopes {
			scopes = append(scopes, scope.toScope())
		}
		s.scopes = scopes
		s.scopeIndex = cursor.ScopeIndex
		s.markFolderScopesSeen()
	} else if scope, ok := cursor.Scope.toScopeIfValid(); ok {
		if index := googleDriveScopeIndex(s.scopes, scope); index >= 0 {
			s.scopeIndex = index
		} else {
			s.scopes = append(s.scopes, scope)
			s.scopeIndex = len(s.scopes) - 1
		}
	} else {
		return fmt.Errorf("google drive sync cursor has no valid scope: %w", ErrSyncResumeInvalid)
	}
	s.pageToken = cursor.PageToken
	s.resumePageToken = cursor.PageToken
	s.resumeSourceID = sourceID
	if cursor.Offset > 0 {
		s.resumePageOffset = cursor.Offset
	}
	return nil
}

func (s *googleDriveSyncSession) filterResumedDocuments(pageToken string, candidates []googleDriveBufferedDocument) ([]googleDriveBufferedDocument, error) {
	if s.resumeSourceID == "" {
		return candidates, nil
	}
	if pageToken != s.resumePageToken {
		return nil, fmt.Errorf("google drive resume page %q no longer matches checkpoint page %q: %w", pageToken, s.resumePageToken, ErrSyncResumeInvalid)
	}
	for index, candidate := range candidates {
		if candidate.document.SourceID == s.resumeSourceID {
			s.clearResumeOffset()
			return candidates[index+1:], nil
		}
	}
	return nil, fmt.Errorf("google drive resume anchor %q was not found on page %q: %w", s.resumeSourceID, pageToken, ErrSyncResumeInvalid)
}

func (s *googleDriveSyncSession) clearResumeOffset() {
	s.resumePageOffset = 0
	s.resumePageToken = ""
	s.resumeSourceID = ""
}

func (s *googleDriveSyncSession) syncCheckpoint(scope googleDriveScope, pageToken string, offset int, doc SourceDocument) *SyncCheckpoint {
	scopes := make([]googleDriveScopeCursor, 0, len(s.scopes))
	for _, scope := range s.scopes {
		scopes = append(scopes, googleDriveScopeCursorFromScope(scope))
	}
	cursor, err := json.Marshal(googleDriveSyncCursor{
		Scopes:     scopes,
		Scope:      googleDriveScopeCursorFromScope(scope),
		ScopeIndex: s.scopeIndex,
		PageToken:  pageToken,
		Offset:     offset,
		SourceID:   doc.SourceID,
	})
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

func (s *googleDriveSyncSession) markFolderScopesSeen() {
	if s.folderSeen == nil {
		s.folderSeen = map[string]struct{}{}
	}
	for _, scope := range s.scopes {
		if scope.corpora == "folder" && scope.folderID != "" {
			s.folderSeen[scope.folderID] = struct{}{}
		}
	}
}

type googleDriveBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
}

type googleDriveSyncCursor struct {
	Scopes     []googleDriveScopeCursor `json:"scopes,omitempty"`
	Scope      googleDriveScopeCursor   `json:"scope"`
	ScopeIndex int                      `json:"scope_index"`
	PageToken  string                   `json:"page_token,omitempty"`
	Offset     int                      `json:"offset"`
	SourceID   string                   `json:"source_id,omitempty"`
}

type googleDriveScopeCursor struct {
	UserEmail           string `json:"user_email"`
	Corpora             string `json:"corpora"`
	DriveID             string `json:"drive_id,omitempty"`
	FolderID            string `json:"folder_id,omitempty"`
	IncludeSharedWithMe bool   `json:"include_shared_with_me,omitempty"`
	SharedWithMeOnly    bool   `json:"shared_with_me_only,omitempty"`
}

func googleDriveScopeCursorFromScope(scope googleDriveScope) googleDriveScopeCursor {
	return googleDriveScopeCursor{
		UserEmail:           scope.userEmail,
		Corpora:             scope.corpora,
		DriveID:             scope.driveID,
		FolderID:            scope.folderID,
		IncludeSharedWithMe: scope.includeSharedWithMe,
		SharedWithMeOnly:    scope.sharedWithMeOnly,
	}
}

func (c googleDriveScopeCursor) toScope() googleDriveScope {
	return googleDriveScope{
		userEmail:           c.UserEmail,
		corpora:             c.Corpora,
		driveID:             c.DriveID,
		folderID:            c.FolderID,
		includeSharedWithMe: c.IncludeSharedWithMe,
		sharedWithMeOnly:    c.SharedWithMeOnly,
	}
}

func (c googleDriveScopeCursor) toScopeIfValid() (googleDriveScope, bool) {
	if c.UserEmail == "" || c.Corpora == "" {
		return googleDriveScope{}, false
	}
	return c.toScope(), true
}

func googleDriveScopeIndex(scopes []googleDriveScope, target googleDriveScope) int {
	for index, scope := range scopes {
		if scope == target {
			return index
		}
	}
	return -1
}

type googleDrivePruneSession struct {
	connector  *GoogleDriveConnector
	scopes     []googleDriveScope
	scopeIndex int
	pageToken  string
	batchSize  int
	buffer     []SlimDocument
	seen       map[string]struct{}
	folderSeen map[string]struct{}
}

// NextBatch returns the next Google Drive prune snapshot batch.
func (s *googleDrivePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.seen == nil {
		s.seen = map[string]struct{}{}
	}
	if s.folderSeen == nil {
		s.folderSeen = map[string]struct{}{}
	}
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.scopeIndex >= len(s.scopes) {
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

func (s *googleDrivePruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	scope := s.scopes[s.scopeIndex]
	page, err := s.connector.listFilePageWithRetry(ctx, scope, s.pageToken, nil, time.Time{})
	if err != nil {
		if isGooglePermissionDeniedOrNotFound(err) {
			s.advanceScope()
			return nil, nil
		}
		return nil, err
	}
	docs := make([]SlimDocument, 0, len(page.Files))
	for _, file := range page.Files {
		sourceID, ok := file.sourceID()
		if !ok {
			continue
		}
		if _, ok = s.seen[sourceID]; ok {
			continue
		}
		s.seen[sourceID] = struct{}{}
		docs = append(docs, SlimDocument{SourceID: sourceID})
	}
	if page.NextPageToken == "" {
		if err = s.finishScope(ctx, scope); err != nil {
			return nil, err
		}
	} else {
		s.pageToken = page.NextPageToken
	}
	return docs, nil
}

func (s *googleDrivePruneSession) finishScope(ctx context.Context, scope googleDriveScope) error {
	if scope.corpora == "folder" {
		childIDs, err := s.connector.listFolderIDs(ctx, scope.userEmail, scope.folderID)
		if err != nil {
			if !isGooglePermissionDeniedOrNotFound(err) {
				return err
			}
		}
		for _, folderID := range childIDs {
			if _, ok := s.folderSeen[folderID]; ok {
				continue
			}
			s.folderSeen[folderID] = struct{}{}
			s.scopes = append(s.scopes, googleDriveScope{userEmail: scope.userEmail, corpora: "folder", folderID: folderID})
		}
	}
	s.advanceScope()
	return nil
}

func (s *googleDrivePruneSession) advanceScope() {
	s.scopeIndex++
	s.pageToken = ""
}

// Close closes the Google Drive prune session.
func (s *googleDrivePruneSession) Close() error {
	return nil
}

type googleDriveScope struct {
	userEmail           string
	corpora             string
	driveID             string
	folderID            string
	includeSharedWithMe bool
	sharedWithMeOnly    bool
}

type googleDriveListRequest struct {
	Scope       googleDriveScope
	PageToken   string
	WindowStart *time.Time
	WindowEnd   time.Time
	PageSize    int
}

type googleDriveFilePage struct {
	NextPageToken string            `json:"nextPageToken"`
	Files         []googleDriveFile `json:"files"`
}

type googleDriveDrivePage struct {
	NextPageToken string             `json:"nextPageToken"`
	Drives        []googleDriveDrive `json:"drives"`
}

type googleDriveDrive struct {
	ID string `json:"id"`
}

type googleDriveUsersPage struct {
	NextPageToken string `json:"nextPageToken"`
	Users         []struct {
		PrimaryEmail string `json:"primaryEmail"`
	} `json:"users"`
}

type googleDriveFile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	MimeType        string `json:"mimeType"`
	ModifiedTime    string `json:"modifiedTime"`
	CreatedTime     string `json:"createdTime"`
	WebViewLink     string `json:"webViewLink"`
	Size            string `json:"size"`
	MD5Checksum     string `json:"md5Checksum"`
	FileExtension   string `json:"fileExtension"`
	ShortcutDetails struct {
		TargetID       string `json:"targetId"`
		TargetMimeType string `json:"targetMimeType"`
	} `json:"shortcutDetails"`
	Owners []struct {
		EmailAddress string `json:"emailAddress"`
	} `json:"owners"`
}

func (f googleDriveFile) toSourceDocument(userEmail string, allowImages bool) (SourceDocument, bool) {
	if f.isFolderLike() || (f.isImage() && !allowImages) {
		return SourceDocument{}, false
	}
	sourceID, ok := f.sourceID()
	if !ok {
		return SourceDocument{}, false
	}
	updatedAt := f.updatedAt()
	fetch := googleDriveFetchReference{FileID: f.ID, UserEmail: userEmail, MimeType: f.MimeType, Name: f.Name, Size: f.Size}
	fetchKey, _ := json.Marshal(fetch)
	metadata := map[string]any{
		"file_id":       f.ID,
		"mime_type":     f.MimeType,
		"web_view_link": f.WebViewLink,
		"owners":        f.ownerEmails(),
	}
	return SourceDocument{
		SourceID:           sourceID,
		SemanticIdentifier: f.Name,
		Extension:          f.extension(),
		FetchRef:           &FetchReference{Key: string(fetchKey), SizeHint: f.sizeInt()},
		UpdatedAt:          updatedAt,
		SizeBytes:          f.sizeInt(),
		Metadata:           metadata,
		Fingerprint:        f.fingerprint(),
	}, true
}

func (f googleDriveFile) sourceID() (string, bool) {
	link := f.WebViewLink
	if link == "" && f.ID != "" {
		if template := googleDriveFallbackLinkTemplate(f.MimeType); template != "" {
			link = fmt.Sprintf(template, f.ID)
		} else {
			link = "https://drive.google.com/file/d/" + url.PathEscape(f.ID) + "/view"
		}
	}
	if link == "" {
		return "", false
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return strings.TrimSpace(link), true
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parts := strings.Split(strings.TrimRight(parsed.Path, "/"), "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last == "edit" || last == "view" || last == "preview" {
			parsed.Path = strings.TrimRight(strings.TrimSuffix(parsed.Path, "/"+last), "/")
		}
	}
	return parsed.String(), true
}

func (f googleDriveFile) fingerprint() string {
	return stableFingerprint(map[string]any{
		"id":            f.ID,
		"name":          f.Name,
		"mime_type":     f.MimeType,
		"modified_time": f.ModifiedTime,
		"created_time":  f.CreatedTime,
		"size":          f.Size,
		"md5":           f.MD5Checksum,
		"owners":        f.ownerEmails(),
	})
}

func (f googleDriveFile) updatedAt() time.Time {
	for _, value := range []string{f.ModifiedTime, f.CreatedTime} {
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func (f googleDriveFile) extension() string {
	if export, ok := googleDriveNativeExports[f.MimeType]; ok {
		return export.extension
	}
	if strings.HasPrefix(f.MimeType, "application/vnd.google-apps") {
		return ".pdf"
	}
	if f.FileExtension != "" {
		return "." + strings.TrimPrefix(f.FileExtension, ".")
	}
	if ext := path.Ext(f.Name); ext != "" {
		return ext
	}
	if exts, err := mime.ExtensionsByType(f.MimeType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}

func (f googleDriveFile) isFolderLike() bool {
	return f.MimeType == googleDriveFolderMimeType || f.MimeType == googleDriveShortcutMimeType
}

func (f googleDriveFile) isImage() bool {
	return strings.HasPrefix(f.MimeType, "image/") && f.MimeType != "image/bmp" && f.MimeType != "image/tiff" && f.MimeType != "image/gif" && f.MimeType != "image/svg+xml" && f.MimeType != "image/avif"
}

func (f googleDriveFile) sizeInt() int64 {
	if f.Size == "" {
		return 0
	}
	size, _ := strconv.ParseInt(f.Size, 10, 64)
	return size
}

func (f googleDriveFile) ownerEmails() []string {
	owners := make([]string, 0, len(f.Owners))
	for _, owner := range f.Owners {
		if owner.EmailAddress != "" {
			owners = append(owners, owner.EmailAddress)
		}
	}
	sort.Strings(owners)
	return owners
}

type googleDriveFetchReference struct {
	FileID    string `json:"file_id"`
	UserEmail string `json:"user_email"`
	MimeType  string `json:"mime_type"`
	Name      string `json:"name"`
	Size      string `json:"size"`
}

type googleDriveExport struct {
	mimeType  string
	extension string
}

func googleDriveQuoteLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `'`, `\'`)
}

func googleDriveFileQuery(scope googleDriveScope, windowStart *time.Time, windowEnd time.Time) string {
	parts := []string{fmt.Sprintf("mimeType != '%s'", googleDriveFolderMimeType), "trashed = false"}
	if scope.corpora == "folder" {
		parts = append(parts, fmt.Sprintf("'%s' in parents", googleDriveQuoteLiteral(scope.folderID)))
	}
	if scope.corpora == "user" {
		if scope.sharedWithMeOnly {
			parts = append(parts, "not 'me' in owners")
		} else if !scope.includeSharedWithMe {
			parts = append(parts, "'me' in owners")
		}
	}
	if windowStart != nil {
		start := windowStart.UTC().Format(time.RFC3339)
		parts = append(parts, fmt.Sprintf("(modifiedTime > '%s' or createdTime >= '%s')", start, start))
	}
	if !windowEnd.IsZero() {
		parts = append(parts, fmt.Sprintf("modifiedTime <= '%s'", windowEnd.UTC().Format(time.RFC3339)))
	}
	return strings.Join(parts, " and ")
}

func googleDriveIDsFromURLs(value string) []string {
	ids := []string{}
	for _, item := range splitCommaList(value) {
		parsed, err := url.Parse(item)
		if err != nil || parsed.Path == "" {
			ids = append(ids, strings.Trim(item, "/"))
			continue
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			ids = append(ids, parts[len(parts)-1])
		}
	}
	return uniqueSorted(ids)
}

func googleDriveIDs(drives []googleDriveDrive) []string {
	ids := make([]string, 0, len(drives))
	for _, drive := range drives {
		if drive.ID != "" {
			ids = append(ids, drive.ID)
		}
	}
	return ids
}

func googleDriveFallbackLinkTemplate(mimeType string) string {
	switch mimeType {
	case "application/vnd.google-apps.document":
		return "https://docs.google.com/document/d/%s/view"
	case "application/vnd.google-apps.spreadsheet":
		return "https://docs.google.com/spreadsheets/d/%s/view"
	case "application/vnd.google-apps.presentation":
		return "https://docs.google.com/presentation/d/%s/view"
	default:
		return ""
	}
}

func (c *GoogleDriveConnector) isServiceAccount() bool {
	return stringConfig(c.credentials["google_service_account_key"]) != ""
}

func (c *GoogleDriveConnector) effectiveIncludeSharedWithMe() bool {
	if c.isServiceAccount() && !c.specificRequests {
		return true
	}
	return c.includeFilesSharedWithMe
}

func splitCommaList(value string) []string {
	values := []string{}
	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
