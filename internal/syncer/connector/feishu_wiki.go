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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
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
	feishuOpenBaseURL        = "https://open.feishu.cn"
	feishuRequestTimeout     = 60 * time.Second
	defaultFeishuBatchSize   = 2
	maxFeishuBatchSize       = 10
	feishuNodePageSize       = 50
	defaultFeishuMaxFileSize = 50 * 1024 * 1024
	feishuMaxResponseBody    = 4 * 1024 * 1024
)

// feishuSupportedExtensions is the set of downloadable Wiki file extensions the
// connector accepts, mirroring the Python connector's allowlist.
var feishuSupportedExtensions = map[string]struct{}{
	".csv": {}, ".doc": {}, ".docx": {}, ".eml": {}, ".gif": {}, ".html": {},
	".jpeg": {}, ".jpg": {}, ".json": {}, ".md": {}, ".mdx": {}, ".pdf": {},
	".png": {}, ".ppt": {}, ".pptx": {}, ".tif": {}, ".txt": {}, ".xls": {},
	".xlsx": {},
}

// Retry/backoff knobs for Feishu API calls. They are package variables so tests
// can shrink the delays (mirrors rest_api.go).
var (
	feishuRetryTries     = 5
	feishuRetryBaseDelay = time.Second
	feishuRetryMaxDelay  = 30 * time.Second
	feishuRetryBackoff   = 2.0
	feishuRetryJitter    = time.Second
	feishu429MaxWaits    = 5
	feishu429DefaultWait = 5 * time.Second
)

// ErrFeishuWikiPruneUnsupported reports that the Feishu Wiki connector cannot
// reconcile deletions; prune tasks then complete without deleting anything.
var ErrFeishuWikiPruneUnsupported = fmt.Errorf("feishu wiki connector does not support prune: %w", ErrPruneUnsupported)

// FeishuWikiConnector reads downloadable attachment files from one Feishu Wiki
// subtree using an application's tenant credentials.
type FeishuWikiConnector struct {
	appID             string
	appSecret         string
	spaceID           string
	rootNodeToken     string
	includeExtensions map[string]struct{}
	includeKeywords   map[string]struct{}
	excludeKeywords   map[string]struct{}
	batchSize         int
	maxFileSizeBytes  int64
	baseURL           string
	httpClient        *http.Client

	mu                   sync.Mutex
	accessToken          string
	accessTokenExpiresAt time.Time
}

// NewFeishuWikiConnector creates a Feishu Wiki connector from Python-compatible
// config. It performs synchronous config validation but no network I/O.
func NewFeishuWikiConnector(config map[string]any) (*FeishuWikiConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	batchSize, err := feishuConfigInt(config["batch_size"], defaultFeishuBatchSize)
	if err != nil {
		return nil, &ConnectorValidationError{Message: "Feishu Wiki batch_size must be an integer"}
	}
	maxFileSizeBytes, err := feishuConfigInt64(config["max_file_size_bytes"], defaultFeishuMaxFileSize)
	if err != nil {
		return nil, &ConnectorValidationError{Message: "Feishu Wiki max_file_size_bytes must be an integer"}
	}
	connector := &FeishuWikiConnector{
		appID:             strings.TrimSpace(stringConfig(credentials["app_id"])),
		appSecret:         strings.TrimSpace(stringConfig(credentials["app_secret"])),
		spaceID:           strings.TrimSpace(stringConfig(config["space_id"])),
		rootNodeToken:     strings.TrimSpace(stringConfig(config["root_node_token"])),
		includeExtensions: feishuExtensionSet(config["include_extensions"]),
		includeKeywords:   feishuKeywordSet(config["include_keywords"]),
		excludeKeywords:   feishuKeywordSet(config["exclude_keywords"]),
		batchSize:         batchSize,
		maxFileSizeBytes:  maxFileSizeBytes,
		baseURL:           feishuOpenBaseURL,
		httpClient:        &http.Client{Timeout: feishuRequestTimeout},
	}
	if err := connector.validateConfig(); err != nil {
		return nil, err
	}
	return connector, nil
}

// validateConfig validates configuration values without network I/O.
func (c *FeishuWikiConnector) validateConfig() error {
	if c.appID == "" || c.appSecret == "" {
		return &ConnectorMissingCredentialError{Message: "Feishu Wiki"}
	}
	if c.spaceID == "" {
		return &ConnectorValidationError{Message: "Feishu Wiki space ID is required"}
	}
	if c.rootNodeToken == "" {
		return &ConnectorValidationError{Message: "Feishu Wiki root node token is required"}
	}
	if c.batchSize < 1 || c.batchSize > maxFeishuBatchSize {
		return &ConnectorValidationError{Message: fmt.Sprintf("Feishu Wiki batch_size must be between 1 and %d", maxFeishuBatchSize)}
	}
	if c.maxFileSizeBytes <= 0 {
		return &ConnectorValidationError{Message: "Feishu Wiki max_file_size_bytes must be a positive integer"}
	}
	for ext := range c.includeExtensions {
		if _, ok := feishuSupportedExtensions[ext]; !ok {
			return &ConnectorValidationError{Message: fmt.Sprintf("Unsupported Feishu Wiki file extension: %s", ext)}
		}
	}
	return nil
}

// Validate validates configuration and credentials by authenticating and
// listing the first page of the configured root node.
func (c *FeishuWikiConnector) Validate(ctx context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}
	if _, err := c.getAccessToken(ctx); err != nil {
		return err
	}
	if _, _, _, err := c.listChildPage(ctx, c.rootNodeToken, ""); err != nil {
		return fmt.Errorf("Feishu Wiki validation failed: %w", err)
	}
	return nil
}

// ValidateConnectorSetting validates Feishu Wiki settings from an unsaved config.
func (c *FeishuWikiConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Feishu Wiki synchronization session.
func (c *FeishuWikiConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	files, err := c.collectFiles(ctx)
	if err != nil {
		return nil, err
	}
	end := request.WindowEnd
	if end.IsZero() {
		end = time.Now().UTC()
	}
	accepted := make([]feishuWikiFile, 0, len(files))
	for _, file := range files {
		if request.WindowStart != nil {
			var updatedAt *time.Time
			if file.hasUpdatedAt {
				updatedAt = &file.updatedAt
			}
			if !feishuWithinWindow(updatedAt, request.WindowStart, &end) {
				continue
			}
		}
		if !c.matchesFilters(file.title) {
			continue
		}
		accepted = append(accepted, file)
	}
	sort.Slice(accepted, func(i, j int) bool {
		return accepted[i].sourceID(c.spaceID) < accepted[j].sourceID(c.spaceID)
	})
	session := &feishuWikiSyncSession{
		connector: c,
		files:     accepted,
		batchSize: c.batchSize,
		windowEnd: end,
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune reports prune as unsupported: ordinary polling does not reconcile
// source deletions (mirroring the Python connector).
func (c *FeishuWikiConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	return nil, ErrFeishuWikiPruneUnsupported
}

// feishuWikiFile is the metadata for one downloadable Wiki file node.
type feishuWikiFile struct {
	nodeToken       string
	objToken        string
	title           string
	objType         string
	parentNodeToken string
	updatedAt       time.Time
	hasUpdatedAt    bool
}

func (f feishuWikiFile) sourceID(spaceID string) string {
	return "feishu_wiki:" + spaceID + ":" + f.nodeToken
}

// feishuWikiSyncSession streams accepted Wiki files in fixed-size batches,
// resuming from a committed SourceID anchor.
type feishuWikiSyncSession struct {
	connector *FeishuWikiConnector
	files     []feishuWikiFile
	batchSize int
	windowEnd time.Time
	index     int
}

// NextBatch returns the next source document batch.
func (s *feishuWikiSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.files) {
		return SyncBatch{}, io.EOF
	}
	documents := make([]SourceDocument, 0, s.batchSize)
	var last feishuWikiFile
	for attempts := 0; attempts < s.batchSize && s.index < len(s.files); attempts++ {
		file := s.files[s.index]
		s.index++
		last = file
		document, err := s.connector.buildDocument(ctx, file, s.windowEnd)
		if err != nil {
			return SyncBatch{}, err
		}
		documents = append(documents, document)
	}
	return SyncBatch{Documents: documents, Checkpoint: feishuWikiCheckpoint(s.connector.spaceID, last)}, nil
}

// Close releases connector resources.
func (s *feishuWikiSyncSession) Close() error {
	return nil
}

// applyResume advances the session past the last committed SourceID anchor.
func (s *feishuWikiSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("feishu wiki sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, file := range s.files {
		if file.sourceID(s.connector.spaceID) == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("feishu wiki resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

func feishuWikiCheckpoint(spaceID string, file feishuWikiFile) *SyncCheckpoint {
	checkpoint := &SyncCheckpoint{
		Cursor:   file.sourceID(spaceID),
		SourceID: file.sourceID(spaceID),
	}
	if file.hasUpdatedAt {
		updatedAt := file.updatedAt
		checkpoint.UpdatedAt = &updatedAt
	}
	return checkpoint
}

// buildDocument downloads one file and maps it to a SourceDocument.
func (c *FeishuWikiConnector) buildDocument(ctx context.Context, file feishuWikiFile, fallbackUpdatedAt time.Time) (SourceDocument, error) {
	blob, err := c.downloadFile(ctx, file.objToken)
	if err != nil {
		return SourceDocument{}, err
	}
	updatedAt := fallbackUpdatedAt
	if file.hasUpdatedAt {
		updatedAt = file.updatedAt
	}
	parentNodeToken := file.parentNodeToken
	if parentNodeToken == "" {
		parentNodeToken = c.rootNodeToken
	}
	sha := sha256.Sum256(blob)
	shaHex := hex.EncodeToString(sha[:])
	return SourceDocument{
		SourceID:           file.sourceID(c.spaceID),
		SemanticIdentifier: firstNonEmpty(file.title, file.nodeToken),
		Extension:          strings.ToLower(path.Ext(file.title)),
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Fingerprint:        shaHex[:32],
		Metadata: map[string]any{
			"source":                 "feishu_wiki",
			"wiki_space_id":          c.spaceID,
			"wiki_node_token":        file.nodeToken,
			"wiki_object_token":      file.objToken,
			"wiki_object_type":       file.objType,
			"wiki_parent_node_token": parentNodeToken,
			"wiki_url":               fmt.Sprintf("https://feishu.cn/wiki/%s", file.nodeToken),
			"source_updated_at":      updatedAt.UTC().Format(time.RFC3339),
			"content_sha256":         shaHex,
		},
	}, nil
}

// matchesFilters applies the extension and keyword filters to a node title.
func (c *FeishuWikiConnector) matchesFilters(title string) bool {
	ext := strings.ToLower(path.Ext(title))
	if _, ok := feishuSupportedExtensions[ext]; !ok {
		return false
	}
	if len(c.includeExtensions) > 0 {
		if _, ok := c.includeExtensions[ext]; !ok {
			return false
		}
	}
	normalizedTitle := strings.ToLower(title)
	if len(c.includeKeywords) > 0 && !feishuContainsAny(normalizedTitle, c.includeKeywords) {
		return false
	}
	return !feishuContainsAny(normalizedTitle, c.excludeKeywords)
}

func feishuContainsAny(value string, terms map[string]struct{}) bool {
	for term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

// feishuWithinWindow reports whether a node update time falls in (start, end].
// Nodes without a timestamp are always included so content is not silently
// omitted.
func feishuWithinWindow(updatedAt *time.Time, start, end *time.Time) bool {
	if updatedAt == nil {
		return true
	}
	if start != nil && !start.Before(*updatedAt) {
		return false
	}
	return end == nil || !updatedAt.After(*end)
}

// collectFiles walks the configured Wiki subtree and returns all downloadable
// file nodes (filters are applied later so the window can be applied at open).
func (c *FeishuWikiConnector) collectFiles(ctx context.Context) ([]feishuWikiFile, error) {
	var files []feishuWikiFile
	stack := []string{c.rootNodeToken}
	for len(stack) > 0 {
		parent := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		pageToken := ""
		for {
			items, hasMore, next, err := c.listChildPage(ctx, parent, pageToken)
			if err != nil {
				return nil, err
			}
			for _, node := range items {
				nodeToken := feishuString(node["node_token"])
				objType := feishuString(node["obj_type"])
				objToken := feishuString(node["obj_token"])
				title := feishuString(node["title"])
				hasChild, _ := node["has_child"].(bool)
				if objType == "file" {
					updatedAt, hasUpdatedAt := feishuParseTimestamp(node["obj_edit_time"])
					files = append(files, feishuWikiFile{
						nodeToken:       nodeToken,
						objToken:        objToken,
						title:           title,
						objType:         objType,
						parentNodeToken: feishuString(node["parent_node_token"]),
						updatedAt:       updatedAt,
						hasUpdatedAt:    hasUpdatedAt,
					})
				}
				if hasChild && nodeToken != "" {
					stack = append(stack, nodeToken)
				}
			}
			if !hasMore {
				break
			}
			if next == "" {
				return nil, &ConnectorValidationError{Message: "Feishu Wiki pagination indicated more data without a page token"}
			}
			pageToken = next
		}
	}
	return files, nil
}

// listChildPage lists one page of child nodes for a parent node.
func (c *FeishuWikiConnector) listChildPage(ctx context.Context, parentNodeToken, pageToken string) ([]map[string]any, bool, string, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, false, "", err
	}
	endpoint := fmt.Sprintf("%s/open-apis/wiki/v2/spaces/%s/nodes?page_size=%d", c.baseURL, url.PathEscape(c.spaceID), feishuNodePageSize)
	if parentNodeToken != "" {
		endpoint += "&parent_node_token=" + url.QueryEscape(parentNodeToken)
	}
	if pageToken != "" {
		endpoint += "&page_token=" + url.QueryEscape(pageToken)
	}
	resp, err := c.feishuDoRequest(ctx, http.MethodGet, endpoint, nil, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return nil, false, "", err
	}
	payload, err := feishuDecodeResponse(resp, "list Wiki nodes")
	if err != nil {
		return nil, false, "", err
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return nil, false, "", &ConnectorValidationError{Message: "Feishu Wiki node response contained invalid data"}
	}
	items := feishuNodeItems(data["items"])
	hasMore, _ := data["has_more"].(bool)
	return items, hasMore, feishuString(data["page_token"]), nil
}

func feishuNodeItems(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	items := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if node, ok := item.(map[string]any); ok {
			items = append(items, node)
		}
	}
	return items
}

// getAccessToken returns a cached tenant access token, refreshing it when
// expired or absent.
func (c *FeishuWikiConnector) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.accessTokenExpiresAt) {
		return c.accessToken, nil
	}
	body, err := json.Marshal(map[string]string{"app_id": c.appID, "app_secret": c.appSecret})
	if err != nil {
		return "", err
	}
	resp, err := c.feishuDoRequest(ctx, http.MethodPost, c.baseURL+"/open-apis/auth/v3/tenant_access_token/internal", body, nil)
	if err != nil {
		return "", err
	}
	payload, err := feishuDecodeResponse(resp, "authenticate")
	if err != nil {
		return "", err
	}
	accessToken := feishuString(payload["tenant_access_token"])
	if accessToken == "" {
		return "", &ConnectorValidationError{Message: "Feishu authentication response did not contain a tenant access token"}
	}
	expiresIn := feishuIntDefault(payload["expire"], 7200)
	if expiresIn < 60 {
		expiresIn = 7200
	}
	c.accessToken = accessToken
	c.accessTokenExpiresAt = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	return accessToken, nil
}

// downloadFile streams one Wiki file, enforcing the configured size limit and
// rejecting Feishu JSON error envelopes so they are never ingested as content.
func (c *FeishuWikiConnector) downloadFile(ctx context.Context, objectToken string) ([]byte, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("%s/open-apis/drive/v1/files/%s/download", c.baseURL, url.PathEscape(objectToken))
	resp, err := c.feishuDoRequest(ctx, http.MethodGet, endpoint, nil, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, &ConnectorValidationError{Message: "Feishu permission denied while downloading a Wiki file"}
	}
	if resp.StatusCode >= 400 {
		code, message := feishuErrorEnvelope(resp)
		if code != 0 {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu API error %d (HTTP %d) while downloading a Wiki file: %s", code, resp.StatusCode, message)}
		}
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu HTTP %d while downloading a Wiki file", resp.StatusCode)}
	}

	if declared := resp.Header.Get("Content-Length"); declared != "" {
		if declaredSize, parseErr := strconv.ParseInt(strings.TrimSpace(declared), 10, 64); parseErr == nil && declaredSize > c.maxFileSizeBytes {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu Wiki file size exceeds the configured %d-byte limit", c.maxFileSizeBytes)}
		}
	}

	// Feishu can return HTTP 200 with a JSON error envelope (non-zero code)
	// instead of file bytes. A legitimate JSON file comes back with code 0 and
	// is returned as content below.
	if contentType := resp.Header.Get("Content-Type"); strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "application/json") {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, c.maxFileSizeBytes+1))
		if readErr != nil {
			return nil, &ConnectorValidationError{Message: "Failed to read a Feishu Wiki file"}
		}
		if int64(len(body)) > c.maxFileSizeBytes {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu Wiki file size exceeds the configured %d-byte limit", c.maxFileSizeBytes)}
		}
		var envelope struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(body, &envelope) == nil && envelope.Code != 0 {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu returned a JSON error envelope instead of Wiki file content (code %d): %s", envelope.Code, envelope.Msg)}
		}
		return body, nil
	}

	blob, readErr := io.ReadAll(io.LimitReader(resp.Body, c.maxFileSizeBytes+1))
	if readErr != nil {
		return nil, &ConnectorValidationError{Message: "Failed to download a Feishu Wiki file"}
	}
	if int64(len(blob)) > c.maxFileSizeBytes {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu Wiki file size exceeds the configured %d-byte limit", c.maxFileSizeBytes)}
	}
	return blob, nil
}

// feishuDoRequest performs one request with bounded 429/5xx retry, honoring
// Feishu's x-ogw-ratelimit-reset (and Retry-After) headers.
func (c *FeishuWikiConnector) feishuDoRequest(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) (*http.Response, error) {
	delay := feishuRetryBaseDelay
	var lastErr error
	rateWaits := 0
	for attempt := 0; attempt < feishuRetryTries; attempt++ {
		resp, err := c.feishuSend(ctx, method, endpoint, body, headers)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt == feishuRetryTries-1 {
				break
			}
			if !feishuSleep(ctx, delay+time.Duration(rand.Float64()*float64(feishuRetryJitter))) {
				return nil, ctx.Err()
			}
			delay = time.Duration(float64(delay) * feishuRetryBackoff)
			if delay > feishuRetryMaxDelay {
				delay = feishuRetryMaxDelay
			}
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			if rateWaits >= feishu429MaxWaits {
				return nil, &RateLimitTriedTooManyTimesError{Message: "Feishu rate limit exceeded while requesting " + endpoint}
			}
			rateWaits++
			retryAfter := feishu429DefaultWait
			if raw := resp.Header.Get("x-ogw-ratelimit-reset"); raw != "" {
				if seconds, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil && seconds >= 0 {
					retryAfter = time.Duration(seconds) * time.Second
				}
			} else if raw := resp.Header.Get("Retry-After"); raw != "" {
				if seconds, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil && seconds >= 0 {
					retryAfter = time.Duration(seconds) * time.Second
				}
			}
			if !feishuSleep(ctx, retryAfter) {
				return nil, ctx.Err()
			}
			continue
		}
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("Feishu HTTP %d while requesting %s", resp.StatusCode, endpoint)
			if attempt == feishuRetryTries-1 {
				break
			}
			if !feishuSleep(ctx, delay+time.Duration(rand.Float64()*float64(feishuRetryJitter))) {
				return nil, ctx.Err()
			}
			delay = time.Duration(float64(delay) * feishuRetryBackoff)
			if delay > feishuRetryMaxDelay {
				delay = feishuRetryMaxDelay
			}
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("Feishu request failed: %s", endpoint)
	}
	return nil, lastErr
}

// feishuSend performs one HTTP request without retry.
func (c *FeishuWikiConnector) feishuSend(ctx context.Context, method, endpoint string, body []byte, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func feishuSleep(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}

// feishuDecodeResponse decodes a JSON API response and enforces Feishu error
// semantics (permission/HTTP/business codes surfaced with code and msg).
func feishuDecodeResponse(resp *http.Response, operation string) (map[string]any, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, feishuMaxResponseBody+1))
	if err != nil {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Failed to read Feishu response while attempting to %s", operation)}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu returned an invalid response while attempting to %s", operation)}
	}
	code := feishuCode(payload["code"])
	message := feishuString(payload["msg"])
	if message == "" {
		message = "unknown Feishu API error"
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		detail := ""
		if message != "unknown Feishu API error" {
			detail = ": " + message
		}
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu permission denied (HTTP %d) while attempting to %s%s", resp.StatusCode, operation, detail)}
	}
	if resp.StatusCode >= 400 {
		if code != 0 {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu API error %d (HTTP %d) while attempting to %s: %s", code, resp.StatusCode, operation, message)}
		}
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu HTTP %d while attempting to %s", resp.StatusCode, operation)}
	}
	if code != 0 {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Feishu API error %d while attempting to %s: %s", code, operation, message)}
	}
	return payload, nil
}

// feishuErrorEnvelope reads a response body (bounded) and returns the Feishu
// business code and message for diagnostics.
func feishuErrorEnvelope(resp *http.Response) (int, string) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, feishuMaxResponseBody+1))
	if err != nil {
		return 0, ""
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return 0, ""
	}
	return envelope.Code, envelope.Msg
}

func feishuString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func feishuCode(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0
		}
		if parsed, err := strconv.Atoi(typed); err == nil {
			return parsed
		}
	}
	return 0
}

func feishuIntDefault(value any, fallback int) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

// feishuConfigInt parses a positive integer config value; empty/nil uses the
// fallback and an invalid non-empty value returns an error.
func feishuConfigInt(value any, fallback int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return fallback, nil
		}
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	}
	return 0, fmt.Errorf("invalid integer %v", value)
}

// feishuConfigInt64 parses an int64 config value; empty/nil uses the fallback.
func feishuConfigInt64(value any, fallback int64) (int64, error) {
	if value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return fallback, nil
		}
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case float64:
		return int64(typed), nil
	}
	return 0, fmt.Errorf("invalid integer %v", value)
}

// feishuNormalizeStrings flattens a comma string or a []any value into trimmed,
// casefolded strings.
func feishuNormalizeStrings(value any) []string {
	var raw []string
	switch typed := value.(type) {
	case string:
		raw = strings.Split(typed, ",")
	case []any:
		for _, item := range typed {
			raw = append(raw, fmt.Sprint(item))
		}
	case []string:
		raw = typed
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, strings.ToLower(item))
	}
	return out
}

func feishuExtensionSet(value any) map[string]struct{} {
	set := map[string]struct{}{}
	for _, term := range feishuNormalizeStrings(value) {
		term = strings.TrimLeft(term, ".")
		if term == "" {
			continue
		}
		set["."+term] = struct{}{}
	}
	return set
}

func feishuKeywordSet(value any) map[string]struct{} {
	set := map[string]struct{}{}
	for _, term := range feishuNormalizeStrings(value) {
		set[term] = struct{}{}
	}
	return set
}

func feishuParseTimestamp(value any) (time.Time, bool) {
	if value == nil {
		return time.Time{}, false
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseFloat(text, 64); err == nil {
		return time.Unix(int64(seconds), 0).UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}
