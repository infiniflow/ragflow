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
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"
)

const (
	seafileDefaultBatchSize     = 2
	seafileDefaultSizeThreshold = 20 * 1024 * 1024
	seafileRequestTimeout       = 60 * time.Second
	seafileMaxRedirects         = 10
	seafileMaxResponseSize      = 32 * 1024 * 1024
)

const (
	seafileScopeAccount   = "account"
	seafileScopeLibrary   = "library"
	seafileScopeDirectory = "directory"
)

// SeaFileConnector reads files from a SeaFile server.
type SeaFileConnector struct {
	seafileURL       string
	syncScope        string
	repoID           string
	syncPath         string
	includeShared    bool
	batchSize        int
	sizeThreshold    int64
	accountToken     string
	repoToken        string
	username         string
	password         string
	currentUserEmail string
	httpClient       *http.Client

	authenticate    func(ctx context.Context, username, password string) (string, error)
	listLibraries   func(ctx context.Context) ([]seafileLibrary, error)
	getRepoInfo     func(ctx context.Context) (seafileRepoInfo, error)
	listDirectory   func(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error)
	getDownloadLink func(ctx context.Context, repoID, path string, useRepoToken bool) (string, error)
	download        func(ctx context.Context, rawURL string, maxSize int64) ([]byte, error)
}

type seafileLibrary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	OwnerEmail string `json:"owner_email"`
}

type seafileRepoInfo struct {
	ID   string
	Name string
}

type seafileDirent struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	ID    string `json:"id"`
	Size  int64  `json:"size"`
	MTime any    `json:"mtime"`
}

type seafileFetchReference struct {
	RepoID       string `json:"repo_id"`
	Path         string `json:"path"`
	UseRepoToken bool   `json:"use_repo_token"`
	Size         int64  `json:"size"`
}

type seafileHTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *seafileHTTPError) Error() string {
	return fmt.Sprintf("SeaFile API returned HTTP %d for %s: %s", e.Status, e.URL, strings.TrimSpace(e.Body))
}

type seafileFile struct {
	repoID    string
	repoName  string
	path      string
	name      string
	fileID    string
	size      int64
	updatedAt time.Time
}

// NewSeaFileConnector creates a SeaFile connector from Python-compatible config.
func NewSeaFileConnector(config map[string]any) (*SeaFileConnector, error) {
	credentials := configAnyMap(config["credentials"])
	syncScope := strings.TrimSpace(stringConfig(config["sync_scope"]))
	if syncScope == "" {
		syncScope = seafileScopeAccount
	}
	repoID := strings.TrimSpace(stringConfig(config["repo_id"]))
	connector := &SeaFileConnector{
		seafileURL:    strings.TrimRight(strings.TrimSpace(stringConfig(config["seafile_url"])), "/"),
		syncScope:     syncScope,
		repoID:        repoID,
		syncPath:      normalizeSeaFilePath(stringConfig(config["sync_path"])),
		includeShared: configBoolDefault(config["include_shared"], true),
		batchSize:     configInt(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])), seafileDefaultBatchSize),
		accountToken:  strings.TrimSpace(stringConfig(credentials["seafile_token"])),
		username:      strings.TrimSpace(stringConfig(credentials["username"])),
		password:      stringConfig(credentials["password"]),
		httpClient: &http.Client{
			Timeout: seafileRequestTimeout,
		},
	}
	connector.sizeThreshold = int64(configInt(config["size_threshold"], seafileDefaultSizeThreshold))
	if connector.sizeThreshold <= 0 {
		connector.sizeThreshold = seafileDefaultSizeThreshold
	}
	if connector.syncScope != seafileScopeAccount {
		connector.repoToken = strings.TrimSpace(stringConfig(credentials["repo_token"]))
	}

	connector.authenticate = connector.defaultAuthenticate
	connector.listLibraries = connector.defaultListLibraries
	connector.getRepoInfo = connector.defaultGetRepoInfo
	connector.listDirectory = connector.defaultListDirectory
	connector.getDownloadLink = connector.defaultGetDownloadLink
	connector.download = func(ctx context.Context, rawURL string, maxSize int64) ([]byte, error) {
		data, _, _, err := utility.FetchRemoteFileSafely(ctx, rawURL, maxSize)
		return data, err
	}
	if err := connector.validateStatic(); err != nil {
		return nil, err
	}
	return connector, nil
}

// Validate validates SeaFile settings and credentials.
func (c *SeaFileConnector) Validate(ctx context.Context) error {
	if err := c.validateStatic(); err != nil {
		return err
	}
	if err := validateSeaFileURLForSSRF(c.seafileURL); err != nil {
		return err
	}
	if err := c.ensureAccountToken(ctx); err != nil {
		return err
	}
	if c.accountToken != "" {
		if err := c.defaultValidateAccountToken(ctx); err != nil {
			return classifySeaFileError(err)
		}
	}
	if c.repoToken != "" {
		if _, err := c.defaultGetRepoInfo(ctx); err != nil {
			return classifySeaFileError(err)
		}
	} else if c.syncScope != seafileScopeAccount && c.accountToken != "" {
		if err := c.validateRepoAccessViaAccount(ctx); err != nil {
			return err
		}
	}

	switch c.syncScope {
	case seafileScopeAccount:
		if _, err := c.listLibraries(ctx); err != nil {
			return classifySeaFileError(err)
		}
	case seafileScopeLibrary:
		if _, err := c.getRepoInfo(ctx); err != nil {
			return classifySeaFileError(err)
		}
	case seafileScopeDirectory:
		if _, err := c.listDirectory(ctx, c.repoID, c.syncPath, c.repoToken != ""); err != nil {
			return classifySeaFileError(err)
		}
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved SeaFile config.
func (c *SeaFileConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	candidate, err := NewSeaFileConnector(request)
	if err != nil {
		return err
	}
	if c != nil && c.httpClient != nil {
		candidate.httpClient = c.httpClient
	}
	return candidate.Validate(ctx)
}

// OpenSync opens one SeaFile sync session.
func (c *SeaFileConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	if err := c.ensureAccountToken(ctx); err != nil {
		return nil, err
	}
	files, err := c.listFiles(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return seafileSourceID(files[i].repoID, files[i].path) < seafileSourceID(files[j].repoID, files[j].path)
	})

	documents := make([]SourceDocument, 0, len(files))
	for _, file := range files {
		if file.size > c.sizeThreshold {
			continue
		}
		document := c.sourceDocument(file)
		if !includeSeaFileDocument(request, document) {
			continue
		}
		documents = append(documents, document)
	}
	session := &seafileSyncSession{
		connector: c,
		documents: documents,
		batchSize: c.effectiveBatchSize(),
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete SeaFile prune snapshot session.
func (c *SeaFileConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	if err := c.ensureAccountToken(ctx); err != nil {
		return nil, err
	}
	files, err := c.listFiles(ctx)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(files))
	for _, file := range files {
		if file.size > c.sizeThreshold {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: seafileSourceID(file.repoID, file.path)})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	return &seafilePruneSession{
		documents: documents,
		batchSize: c.effectiveBatchSize(),
	}, nil
}

// Fetch downloads a SeaFile file referenced by a previous sync batch.
func (c *SeaFileConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch seafileFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, fmt.Errorf("invalid SeaFile fetch reference: %w", err)
	}
	if fetch.RepoID == "" || fetch.Path == "" {
		return nil, fmt.Errorf("SeaFile fetch reference is incomplete")
	}
	if fetch.Size < 0 || fetch.Size > c.sizeThreshold {
		return nil, fmt.Errorf("SeaFile file %s exceeds size threshold of %d bytes", fetch.Path, c.sizeThreshold)
	}
	link, err := c.getDownloadLink(ctx, fetch.RepoID, fetch.Path, fetch.UseRepoToken)
	if err != nil {
		return nil, fmt.Errorf("SeaFile download link for %s: %w", fetch.Path, err)
	}
	if strings.TrimSpace(link) == "" {
		return nil, fmt.Errorf("SeaFile returned no download link for %s", fetch.Path)
	}
	return c.download(ctx, link, c.sizeThreshold)
}

func (c *SeaFileConnector) validateStatic() error {
	if c == nil {
		return &ConnectorValidationError{Message: "SeaFile connector is nil"}
	}
	c.seafileURL = strings.TrimRight(strings.TrimSpace(c.seafileURL), "/")
	if c.seafileURL == "" {
		return &ConnectorValidationError{Message: "SeaFile server URL is required."}
	}
	parsed, err := url.Parse(c.seafileURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return &ConnectorValidationError{Message: "SeaFile server URL is invalid."}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &ConnectorValidationError{Message: "SeaFile server URL must use http or https."}
	}
	switch c.syncScope {
	case seafileScopeAccount:
	case seafileScopeLibrary, seafileScopeDirectory:
		if strings.TrimSpace(c.repoID) == "" {
			return &ConnectorValidationError{Message: fmt.Sprintf("sync_scope=%q requires 'repo_id'.", c.syncScope)}
		}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("unsupported sync_scope %q", c.syncScope)}
	}
	if c.syncScope == seafileScopeDirectory && c.syncPath == "/" {
		return &ConnectorValidationError{Message: "sync_scope='directory' requires a non-root 'sync_path'."}
	}
	if c.accountToken == "" && c.repoToken == "" && (c.username == "" || c.password == "") {
		return &ConnectorMissingCredentialError{Message: "SeaFile requires 'seafile_token', 'repo_token', or 'username'/'password'."}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "SeaFile batch_size must be a positive integer"}
	}
	if c.sizeThreshold <= 0 {
		c.sizeThreshold = seafileDefaultSizeThreshold
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: seafileRequestTimeout}
	}
	return nil
}

func (c *SeaFileConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return seafileDefaultBatchSize
}

func (c *SeaFileConnector) ensureAccountToken(ctx context.Context) error {
	if c.accountToken != "" {
		return nil
	}
	if c.username == "" || c.password == "" {
		return nil
	}
	token, err := c.authenticate(ctx, c.username, c.password)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return &ConnectorMissingCredentialError{Message: "SeaFile authentication did not return a token."}
	}
	c.accountToken = strings.TrimSpace(token)
	return nil
}

func (c *SeaFileConnector) defaultAuthenticate(ctx context.Context, username, password string) (string, error) {
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)
	reqCtx, cancel := context.WithTimeout(ctx, seafileRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.seafileURL+"/api2/auth-token/", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &ConnectorMissingCredentialError{Message: fmt.Sprintf("Failed to authenticate with SeaFile: %v", err)}
	}
	body, err := readSeaFileBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", &ConnectorMissingCredentialError{Message: fmt.Sprintf("SeaFile authentication failed with HTTP %d", resp.StatusCode)}
	}
	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", &ConnectorMissingCredentialError{Message: "SeaFile authentication response is not valid JSON."}
	}
	return strings.TrimSpace(tokenResponse.Token), nil
}

func (c *SeaFileConnector) defaultValidateAccountToken(ctx context.Context) error {
	var info struct {
		Email string `json:"email"`
	}
	if err := c.getJSON(ctx, "account/info/", false, nil, &info); err != nil {
		return err
	}
	c.currentUserEmail = info.Email
	return nil
}

func (c *SeaFileConnector) validateRepoAccessViaAccount(ctx context.Context) error {
	info, err := c.defaultGetRepoInfoViaAccount(ctx, c.repoID)
	if err != nil {
		return classifySeaFileError(err)
	}
	if info.ID == "" && info.Name == "" {
		return &ConnectorValidationError{Message: fmt.Sprintf("Library %q is not accessible with the account token.", c.repoID)}
	}
	if c.syncScope == seafileScopeDirectory {
		if _, err := c.listDirectory(ctx, c.repoID, c.syncPath, false); err != nil {
			return &ConnectorValidationError{Message: fmt.Sprintf("Directory %q does not exist in library %q.", c.syncPath, c.repoID)}
		}
	}
	return nil
}

func (c *SeaFileConnector) defaultListLibraries(ctx context.Context) ([]seafileLibrary, error) {
	if !c.includeShared && c.currentUserEmail == "" {
		if err := c.defaultValidateAccountToken(ctx); err != nil {
			return nil, err
		}
	}
	var libraries []seafileLibrary
	if err := c.getJSON(ctx, "repos/", false, nil, &libraries); err != nil {
		return nil, err
	}
	if !c.includeShared {
		filtered := libraries[:0]
		for _, library := range libraries {
			if library.Owner == c.currentUserEmail || library.OwnerEmail == c.currentUserEmail {
				filtered = append(filtered, library)
			}
		}
		libraries = filtered
	}
	return libraries, nil
}

func (c *SeaFileConnector) defaultGetRepoInfo(ctx context.Context) (seafileRepoInfo, error) {
	if c.repoToken != "" {
		var raw map[string]any
		if err := c.getJSON(ctx, "repo-info/", true, nil, &raw); err != nil {
			return seafileRepoInfo{}, err
		}
		return seafileRepoInfo{
			ID:   firstNonEmpty(stringConfig(raw["repo_id"]), c.repoID),
			Name: firstNonEmpty(stringConfig(raw["repo_name"]), c.repoID),
		}, nil
	}
	return c.defaultGetRepoInfoViaAccount(ctx, c.repoID)
}

func (c *SeaFileConnector) defaultGetRepoInfoViaAccount(ctx context.Context, repoID string) (seafileRepoInfo, error) {
	var raw map[string]any
	if err := c.getJSON(ctx, "repos/"+url.PathEscape(repoID)+"/", false, nil, &raw); err != nil {
		return seafileRepoInfo{}, err
	}
	return seafileRepoInfo{
		ID:   firstNonEmpty(stringConfig(raw["id"]), repoID),
		Name: firstNonEmpty(stringConfig(raw["name"]), repoID),
	}, nil
}

func (c *SeaFileConnector) defaultListDirectory(ctx context.Context, repoID, path string, useRepoToken bool) ([]seafileDirent, error) {
	var body []byte
	var err error
	if useRepoToken {
		body, err = c.getBody(ctx, "dir/", true, url.Values{"path": {path}})
	} else {
		body, err = c.getBody(ctx, "repos/"+url.PathEscape(repoID)+"/dir/", false, url.Values{"p": {path}})
	}
	if err != nil {
		return nil, err
	}
	var entries []seafileDirent
	if err := json.Unmarshal(body, &entries); err == nil {
		return entries, nil
	}
	var wrapped struct {
		DirentList []seafileDirent `json:"dirent_list"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil || wrapped.DirentList == nil {
		return nil, fmt.Errorf("SeaFile directory response is not a dirent list")
	}
	return wrapped.DirentList, nil
}

func (c *SeaFileConnector) defaultGetDownloadLink(ctx context.Context, repoID, path string, useRepoToken bool) (string, error) {
	var body []byte
	var err error
	if useRepoToken {
		body, err = c.getBody(ctx, "download-link/", true, url.Values{"path": {path}})
	} else {
		body, err = c.getBody(ctx, "repos/"+url.PathEscape(repoID)+"/file/", false, url.Values{"p": {path}, "reuse": {"1"}})
	}
	if err != nil {
		return "", err
	}
	var link string
	if err := json.Unmarshal(body, &link); err == nil {
		return strings.TrimSpace(link), nil
	}
	return strings.Trim(strings.TrimSpace(string(body)), `"`), nil
}

func (c *SeaFileConnector) getJSON(ctx context.Context, endpoint string, useRepoToken bool, query url.Values, out any) error {
	body, err := c.getBody(ctx, endpoint, useRepoToken, query)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("SeaFile API returned invalid JSON for %s", endpoint)}
	}
	return nil
}

func (c *SeaFileConnector) getBody(ctx context.Context, endpoint string, useRepoToken bool, query url.Values) ([]byte, error) {
	headers, err := c.requestHeaders(useRepoToken)
	if err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, seafileRequestTimeout)
	defer cancel()
	currentURL := c.apiURL(endpoint, useRepoToken, query)
	previousNetloc := restAPINetloc(currentURL)
	for hop := 0; hop <= seafileMaxRedirects; hop++ {
		hostname, pinIP, err := seafileAssertURLSafe(reqCtx, currentURL)
		if err != nil {
			return nil, &ConnectorValidationError{Message: "Unsafe SeaFile URL: " + err.Error()}
		}
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		transport := newRestAPIPinnedTransport(hostname, pinIP)
		client := &http.Client{
			Transport: transport,
			Timeout:   seafileRequestTimeout,
			// Redirects are handled manually so every hop is re-validated
			// for SSRF and DNS-pinned before a connection is made.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, err
		}
		if !restAPIIsRedirect(resp.StatusCode) {
			resp.Body = &restAPICloseIdleBody{body: resp.Body, transport: transport}
			body, err := readSeaFileBody(resp)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode >= 400 {
				return nil, &seafileHTTPError{Status: resp.StatusCode, Body: string(body), URL: req.URL.String()}
			}
			return body, nil
		}
		location := resp.Header.Get("Location")
		resp.Body.Close()
		transport.CloseIdleConnections()
		if location == "" {
			return nil, fmt.Errorf("SeaFile API redirect with empty Location header")
		}
		nextURL, err := restAPIResolveURL(currentURL, location)
		if err != nil {
			return nil, err
		}
		// Never forward credentials to a different host, matching Go's default
		// redirect handling.
		nextNetloc := restAPINetloc(nextURL)
		if nextNetloc != "" && nextNetloc != previousNetloc {
			headers = restAPIStripAuthHeaders(headers)
		}
		previousNetloc = nextNetloc
		currentURL = nextURL
	}
	return nil, fmt.Errorf("SeaFile API request exceeded %d redirects", seafileMaxRedirects)
}

func (c *SeaFileConnector) requestHeaders(useRepoToken bool) (map[string]string, error) {
	if useRepoToken {
		if c.repoToken == "" {
			return nil, &ConnectorMissingCredentialError{Message: "SeaFile repo token is not set."}
		}
		return map[string]string{
			"Authorization": "Bearer " + c.repoToken,
			"Accept":        "application/json",
		}, nil
	}
	if c.accountToken == "" {
		return nil, &ConnectorMissingCredentialError{Message: "SeaFile account token is not set."}
	}
	return map[string]string{
		"Authorization": "Token " + c.accountToken,
		"Accept":        "application/json",
	}, nil
}

func (c *SeaFileConnector) apiURL(endpoint string, useRepoToken bool, query url.Values) string {
	var base string
	if useRepoToken {
		base = c.seafileURL + "/api/v2.1/via-repo-token/" + strings.TrimPrefix(endpoint, "/")
	} else {
		base = c.seafileURL + "/api2/" + strings.TrimPrefix(endpoint, "/")
	}
	if len(query) == 0 {
		return base
	}
	return base + "?" + query.Encode()
}

func (c *SeaFileConnector) listFiles(ctx context.Context) ([]seafileFile, error) {
	libraries, err := c.resolveLibraries(ctx)
	if err != nil {
		return nil, err
	}
	var files []seafileFile
	for _, library := range libraries {
		root := c.rootPathForRepo(library.ID)
		listed, err := c.listFilesRecursive(ctx, library, root, map[string]struct{}{})
		if err != nil {
			return nil, err
		}
		files = append(files, listed...)
	}
	return files, nil
}

func (c *SeaFileConnector) resolveLibraries(ctx context.Context) ([]seafileLibrary, error) {
	if c.syncScope == seafileScopeAccount {
		libraries, err := c.listLibraries(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]seafileLibrary, 0, len(libraries))
		for _, library := range libraries {
			if library.ID != "" {
				out = append(out, seafileLibrary{ID: library.ID, Name: firstNonEmpty(library.Name, "Unknown")})
			}
		}
		return out, nil
	}
	info, err := c.getRepoInfo(ctx)
	if err != nil {
		return nil, err
	}
	id := firstNonEmpty(info.ID, c.repoID)
	name := firstNonEmpty(info.Name, c.repoID)
	return []seafileLibrary{{ID: id, Name: name}}, nil
}

func (c *SeaFileConnector) rootPathForRepo(repoID string) string {
	if c.syncScope == seafileScopeDirectory && repoID == c.repoID {
		return c.syncPath
	}
	return "/"
}

func (c *SeaFileConnector) listFilesRecursive(ctx context.Context, library seafileLibrary, path string, seen map[string]struct{}) ([]seafileFile, error) {
	if _, ok := seen[path]; ok {
		return nil, nil
	}
	seen[path] = struct{}{}
	entries, err := c.listDirectory(ctx, library.ID, path, c.repoToken != "")
	if err != nil {
		return nil, err
	}
	var files []seafileFile
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		entryPath := strings.TrimRight(path, "/") + "/" + name
		switch entry.Type {
		case "dir":
			children, err := c.listFilesRecursive(ctx, library, entryPath, seen)
			if err != nil {
				return nil, err
			}
			files = append(files, children...)
		case "file":
			files = append(files, seafileFile{
				repoID:    library.ID,
				repoName:  library.Name,
				path:      entryPath,
				name:      name,
				fileID:    entry.ID,
				size:      entry.Size,
				updatedAt: seafileParseMtime(entry.MTime),
			})
		}
	}
	return files, nil
}

func (c *SeaFileConnector) sourceDocument(file seafileFile) SourceDocument {
	fetch, _ := json.Marshal(seafileFetchReference{
		RepoID:       file.repoID,
		Path:         file.path,
		UseRepoToken: c.repoToken != "",
		Size:         file.size,
	})
	return SourceDocument{
		SourceID:           seafileSourceID(file.repoID, file.path),
		SemanticIdentifier: file.repoName + file.path,
		Extension:          strings.ToLower(filepath.Ext(file.name)),
		FetchRef:           &FetchReference{Key: string(fetch), SizeHint: file.size},
		UpdatedAt:          file.updatedAt,
		SizeBytes:          file.size,
		Metadata: map[string]any{
			"repo_id":   file.repoID,
			"repo_name": file.repoName,
			"path":      file.path,
			"file_id":   file.fileID,
		},
		Fingerprint: seafileFingerprint(file.repoID, file.path, file.fileID, file.size, file.updatedAt),
	}
}

func seafileSourceID(repoID, path string) string {
	return "seafile:" + repoID + ":" + path
}

func seafileFingerprint(repoID, path, fileID string, size int64, updatedAt time.Time) string {
	return stableFingerprint(map[string]any{
		"repo_id": repoID,
		"path":    path,
		"file_id": fileID,
		"size":    size,
		"mtime":   updatedAt.UTC().Format(time.RFC3339Nano),
	})
}

func includeSeaFileDocument(request SyncRequest, document SourceDocument) bool {
	if request.FromBeginning {
		return true
	}
	if len(request.Fingerprints) > 0 {
		stored, ok := request.Fingerprints[document.SourceID]
		return !ok || stored == "" || stored != document.Fingerprint
	}
	return !beforeOrAtWindowStart(document.UpdatedAt, request.WindowStart) && !afterWindowEnd(document.UpdatedAt, request.WindowEnd)
}

func normalizeSeaFilePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if value != "/" {
		value = strings.TrimRight(value, "/")
	}
	return value
}

func seafileParseMtime(raw any) time.Time {
	switch value := raw.(type) {
	case int:
		return time.Unix(int64(value), 0).UTC()
	case int64:
		return time.Unix(value, 0).UTC()
	case float64:
		return time.Unix(int64(value), 0).UTC()
	case json.Number:
		if seconds, err := value.Int64(); err == nil {
			return time.Unix(seconds, 0).UTC()
		}
	case string:
		text := strings.TrimSpace(value)
		if seconds, err := strconv.ParseInt(text, 10, 64); err == nil {
			return time.Unix(seconds, 0).UTC()
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func readSeaFileBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, seafileMaxResponseSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > seafileMaxResponseSize {
		return nil, fmt.Errorf("SeaFile API response exceeds maximum size of %d bytes", seafileMaxResponseSize)
	}
	return body, nil
}

func classifySeaFileError(err error) error {
	var httpErr *seafileHTTPError
	if !errors.As(err, &httpErr) {
		return err
	}
	switch httpErr.Status {
	case http.StatusUnauthorized:
		return &ConnectorMissingCredentialError{Message: "SeaFile account token is invalid or expired."}
	case http.StatusForbidden:
		return &ConnectorValidationError{Message: "SeaFile account lacks permission to access the requested library or directory."}
	case http.StatusNotFound:
		return &ConnectorValidationError{Message: "The requested SeaFile library or directory does not exist or is not accessible."}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("SeaFile validation failed: %v", httpErr)}
	}
}

func validateSeaFileURLForSSRF(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &ConnectorValidationError{Message: "Invalid SeaFile server URL."}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &ConnectorValidationError{Message: fmt.Sprintf("Unsupported SeaFile server URL scheme %q. Only http/https are allowed.", parsed.Scheme)}
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return &ConnectorValidationError{Message: "SeaFile server URL must include a hostname."}
	}
	if strings.EqualFold(hostname, "localhost") && !restAPISSRFAllowLoopback {
		return &ConnectorValidationError{Message: "SeaFile server URL hostname \"localhost\" is not allowed."}
	}
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return nil
	}
	if restAPISSRFAllowLoopback {
		allLoopback := true
		for _, addr := range addrs {
			if !addr.IsLoopback() {
				allLoopback = false
				break
			}
		}
		if allLoopback {
			return nil
		}
	}
	for _, addr := range addrs {
		if !restAPIIPIsGlobal(restAPIEffectiveIP(addr)) {
			return &ConnectorValidationError{Message: fmt.Sprintf(
				"SeaFile server URL %q resolves to disallowed address %s (localhost, private, link-local, reserved, or multicast addresses are blocked).",
				rawURL, addr)}
		}
	}
	return nil
}

// seafileAssertURLSafe validates a per-request SeaFile URL for SSRF and
// returns the hostname plus the first validated IP so the caller can pin DNS
// for the actual dial, preventing DNS rebinding between validation and the
// connection.
func seafileAssertURLSafe(ctx context.Context, rawURL string) (string, net.IP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("SeaFile URL is missing a host.")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, fmt.Errorf("Disallowed SeaFile URL scheme: %q. Only [http https] are allowed.", parsed.Scheme)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", nil, fmt.Errorf("SeaFile URL is missing a host.")
	}
	if strings.EqualFold(hostname, "localhost") && !restAPISSRFAllowLoopback {
		return "", nil, fmt.Errorf("SeaFile URL hostname %q is not allowed (localhost is blocked).", hostname)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", nil, fmt.Errorf("Could not resolve hostname %q: %w", hostname, err)
	}
	if len(addrs) == 0 {
		return "", nil, fmt.Errorf("Hostname %q resolved to no addresses.", hostname)
	}
	if restAPISSRFAllowLoopback {
		allLoopback := true
		for _, addr := range addrs {
			if !addr.IP.IsLoopback() {
				allLoopback = false
				break
			}
		}
		if allLoopback {
			return hostname, addrs[0].IP, nil
		}
		// Not all loopback — fall through to normal validation.
	}
	var first net.IP
	for _, addr := range addrs {
		if !restAPIIPIsGlobal(restAPIEffectiveIP(addr.IP)) {
			return "", nil, fmt.Errorf("SeaFile URL resolves to a non-public address (%s), which is not allowed.", addr.IP)
		}
		if first == nil {
			first = addr.IP
		}
	}
	if first == nil {
		return "", nil, fmt.Errorf("Hostname %q resolved to no addresses.", hostname)
	}
	return hostname, first, nil
}

type seafileSyncSession struct {
	connector *SeaFileConnector
	documents []SourceDocument
	batchSize int
	index     int
}

// NextBatch returns the next SeaFile source document batch.
func (s *seafileSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.documents) {
		return SyncBatch{}, io.EOF
	}
	batchSize := s.batchSize
	if batchSize <= 0 {
		batchSize = seafileDefaultBatchSize
	}
	end := s.index + batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := s.documents[s.index:end]
	s.index = end
	last := batch[len(batch)-1]
	return SyncBatch{Documents: batch, Checkpoint: seafileSyncCheckpoint(last)}, nil
}

// Close closes the SeaFile sync session.
func (s *seafileSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed SeaFile document body.
func (s *seafileSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *seafileSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("seafile sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, document := range s.documents {
		if document.SourceID == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("seafile resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

func seafileSyncCheckpoint(document SourceDocument) *SyncCheckpoint {
	updatedAt := document.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    document.SourceID,
		SourceID:  document.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type seafilePruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next SeaFile prune snapshot batch.
func (s *seafilePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	batchSize := s.batchSize
	if batchSize <= 0 {
		batchSize = seafileDefaultBatchSize
	}
	end := s.index + batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := PruneBatch{Documents: s.documents[s.index:end]}
	s.index = end
	return batch, nil
}

// Close closes the SeaFile prune session.
func (s *seafilePruneSession) Close() error {
	return nil
}
