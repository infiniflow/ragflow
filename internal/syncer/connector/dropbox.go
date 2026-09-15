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
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const (
	defaultDropboxBatchSize      = 32
	maxDropboxBatchSize          = 1_000
	defaultDropboxSizeThreshold  = 20 * 1024 * 1024
	dropboxRequestTimeout        = 60 * time.Second
	maxDropboxDownloadSize       = 64 * 1024 * 1024
	defaultDropboxAPIBaseURL     = "https://api.dropboxapi.com/2"
	defaultDropboxContentBaseURL = "https://content.dropboxapi.com/2"
)

// DropboxConnector reads files from one Dropbox account.
type DropboxConnector struct {
	accessToken    string
	batchSize      int
	allowImages    bool
	sizeThreshold  int64
	apiBaseURL     string
	contentBaseURL string
	httpClient     *http.Client
}

// NewDropboxConnector creates a Dropbox connector from Python-compatible config.
func NewDropboxConnector(config map[string]any) (*DropboxConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	connector := &DropboxConnector{
		accessToken:    strings.TrimSpace(stringConfig(credentials["dropbox_access_token"])),
		batchSize:      configInt(config["batch_size"], defaultDropboxBatchSize),
		allowImages:    configBoolDefault(config["allow_images"], false),
		sizeThreshold:  dropboxSizeThreshold(),
		apiBaseURL:     defaultDropboxAPIBaseURL,
		contentBaseURL: defaultDropboxContentBaseURL,
		httpClient: &http.Client{
			Timeout: dropboxRequestTimeout,
		},
	}
	return connector, nil
}

// Validate validates Dropbox settings and credentials by listing the account root.
func (c *DropboxConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("dropbox connector is nil")
	}
	if c.accessToken == "" {
		return fmt.Errorf("Dropbox access token is required")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}
	_, err := c.listFolder(ctx, dropboxListFolderRequest{
		Path:                        "",
		Recursive:                   false,
		IncludeNonDownloadableFiles: false,
		Limit:                       1,
	})
	if err != nil {
		return fmt.Errorf("Dropbox validation failed: %w", err)
	}
	return nil
}

// ValidateConnectorSetting validates Dropbox settings from an unsaved config.
func (c *DropboxConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Dropbox sync session.
func (c *DropboxConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	files, err := c.listAllFiles(ctx)
	if err != nil {
		return nil, err
	}
	start := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if request.WindowStart != nil {
		start = *request.WindowStart
	}
	end := request.WindowEnd
	if end.IsZero() {
		end = time.Now().UTC()
	}

	nameCounts := dropboxNameCounts(files)
	acceptedFiles := make([]dropboxFileMetadata, 0, len(files))
	for _, file := range files {
		updatedAt := file.updatedAt()
		if !request.FromBeginning && !start.Before(updatedAt) {
			continue
		}
		if updatedAt.After(end) {
			continue
		}
		if !c.isAcceptedFile(file) {
			continue
		}
		acceptedFiles = append(acceptedFiles, file)
	}
	sort.Slice(acceptedFiles, func(i, j int) bool { return acceptedFiles[i].sourceID() < acceptedFiles[j].sourceID() })

	session := &dropboxSyncSession{
		connector:  c,
		files:      acceptedFiles,
		nameCounts: nameCounts,
		batchSize:  positiveDropboxBatchSize(c.batchSize),
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Dropbox prune snapshot session.
func (c *DropboxConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	files, err := c.listAllFiles(ctx)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(files))
	for _, file := range files {
		if !c.isAcceptedFile(file) {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: file.sourceID()})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	return &dropboxPruneSession{documents: documents, batchSize: positiveDropboxBatchSize(c.batchSize)}, nil
}

func (c *DropboxConnector) buildDocument(file dropboxFileMetadata, blob []byte, nameCounts map[string]int) SourceDocument {
	return SourceDocument{
		SourceID:           file.sourceID(),
		SemanticIdentifier: dropboxSemanticIdentifier(file, nameCounts),
		Extension:          dropboxExtension(file.Name),
		Blob:               blob,
		UpdatedAt:          file.updatedAt(),
		SizeBytes:          file.Size,
		Fingerprint:        contentFingerprint(blob),
		Metadata: map[string]any{
			"path": file.pathDisplay(),
		},
	}
}

func (c *DropboxConnector) isAcceptedFile(file dropboxFileMetadata) bool {
	if file.Tag != "file" {
		return false
	}
	if file.Size < 0 || file.Size > c.sizeThreshold {
		return false
	}
	ext := dropboxExtension(file.Name)
	if _, ok := webdavTextExtensions[ext]; ok {
		return true
	}
	if _, ok := webdavDocumentExtensions[ext]; ok {
		return true
	}
	if c.allowImages {
		if _, ok := webdavImageExtensions[ext]; ok {
			return true
		}
	}
	return false
}

func (c *DropboxConnector) listAllFiles(ctx context.Context) ([]dropboxFileMetadata, error) {
	page, err := c.listFolder(ctx, dropboxListFolderRequest{
		Path:                        "",
		Recursive:                   true,
		IncludeNonDownloadableFiles: false,
	})
	if err != nil {
		return nil, err
	}
	files := dropboxFilesFromEntries(page.Entries)
	for page.HasMore {
		page, err = c.listFolderContinue(ctx, page.Cursor)
		if err != nil {
			return nil, err
		}
		files = append(files, dropboxFilesFromEntries(page.Entries)...)
	}
	return files, nil
}

func (c *DropboxConnector) listFolder(ctx context.Context, request dropboxListFolderRequest) (dropboxListFolderResponse, error) {
	if request.Limit == 0 {
		request.Limit = 2000
	}
	var response dropboxListFolderResponse
	err := c.postJSON(ctx, c.apiBaseURL+"/files/list_folder", request, &response)
	return response, err
}

func (c *DropboxConnector) listFolderContinue(ctx context.Context, cursor string) (dropboxListFolderResponse, error) {
	var response dropboxListFolderResponse
	err := c.postJSON(ctx, c.apiBaseURL+"/files/list_folder/continue", map[string]string{"cursor": cursor}, &response)
	return response, err
}

func (c *DropboxConnector) postJSON(ctx context.Context, endpoint string, body any, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return dropboxHTTPError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *DropboxConnector) downloadFile(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.contentBaseURL+"/files/download", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Dropbox-API-Arg", dropboxAPIArgHeader(path))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, dropboxHTTPError(resp)
	}
	limit := c.sizeThreshold
	if limit <= 0 || limit > maxDropboxDownloadSize {
		limit = maxDropboxDownloadSize
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(blob)) > limit {
		return nil, fmt.Errorf("Dropbox file exceeds maximum size of %d bytes", limit)
	}
	return blob, nil
}

type dropboxSyncSession struct {
	connector  *DropboxConnector
	files      []dropboxFileMetadata
	nameCounts map[string]int
	batchSize  int
	index      int
}

// NextBatch returns the next Dropbox document batch.
func (s *dropboxSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.files) {
		return SyncBatch{}, io.EOF
	}
	batchSize := positiveDropboxBatchSize(s.batchSize)
	documents := make([]SourceDocument, 0, batchSize)
	var lastAccepted dropboxFileMetadata
	for attempts := 0; attempts < batchSize && s.index < len(s.files); attempts++ {
		file := s.files[s.index]
		s.index++
		lastAccepted = file
		blob, err := s.connector.downloadFile(ctx, file.downloadPath())
		if err != nil || len(blob) == 0 {
			continue
		}
		documents = append(documents, s.connector.buildDocument(file, blob, s.nameCounts))
	}
	if len(documents) == 0 {
		return SyncBatch{Checkpoint: dropboxSyncCheckpoint(lastAccepted)}, nil
	}
	return SyncBatch{Documents: documents, Checkpoint: dropboxSyncCheckpoint(lastAccepted)}, nil
}

// Close closes the Dropbox sync session.
func (s *dropboxSyncSession) Close() error {
	return nil
}

func (s *dropboxSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("dropbox sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, file := range s.files {
		if file.sourceID() == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("dropbox resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

func dropboxSyncCheckpoint(file dropboxFileMetadata) *SyncCheckpoint {
	updatedAt := file.updatedAt()
	return &SyncCheckpoint{
		Cursor:    file.sourceID(),
		SourceID:  file.sourceID(),
		UpdatedAt: &updatedAt,
	}
}

type dropboxPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next Dropbox prune snapshot batch.
func (s *dropboxPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	batchSize := positiveDropboxBatchSize(s.batchSize)
	end := s.index + batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := PruneBatch{Documents: s.documents[s.index:end]}
	s.index = end
	return batch, nil
}

// Close closes the Dropbox prune session.
func (s *dropboxPruneSession) Close() error {
	return nil
}

func positiveDropboxBatchSize(batchSize int) int {
	if batchSize <= 0 {
		return defaultDropboxBatchSize
	}
	if batchSize > maxDropboxBatchSize {
		return maxDropboxBatchSize
	}
	return batchSize
}

func dropboxAPIArgHeader(path string) string {
	var builder strings.Builder
	builder.Grow(len(path) + len(`{"path":""}`))
	builder.WriteString(`{"path":"`)
	for _, r := range path {
		writeDropboxJSONStringRune(&builder, r)
	}
	builder.WriteString(`"}`)
	return builder.String()
}

func writeDropboxJSONStringRune(builder *strings.Builder, r rune) {
	switch r {
	case '\\', '"':
		builder.WriteByte('\\')
		builder.WriteRune(r)
		return
	}
	if r < 0x20 || r == 0x7f {
		writeDropboxJSONUnicodeEscape(builder, uint16(r))
		return
	}
	if r < 0x7f {
		builder.WriteRune(r)
		return
	}
	if r <= 0xffff {
		writeDropboxJSONUnicodeEscape(builder, uint16(r))
		return
	}
	high, low := utf16.EncodeRune(r)
	writeDropboxJSONUnicodeEscape(builder, uint16(high))
	writeDropboxJSONUnicodeEscape(builder, uint16(low))
}

func writeDropboxJSONUnicodeEscape(builder *strings.Builder, value uint16) {
	const hex = "0123456789abcdef"
	builder.WriteString(`\u`)
	builder.WriteByte(hex[value>>12&0xf])
	builder.WriteByte(hex[value>>8&0xf])
	builder.WriteByte(hex[value>>4&0xf])
	builder.WriteByte(hex[value&0xf])
}

type dropboxListFolderRequest struct {
	Path                        string `json:"path"`
	Recursive                   bool   `json:"recursive"`
	IncludeNonDownloadableFiles bool   `json:"include_non_downloadable_files"`
	Limit                       int    `json:"limit,omitempty"`
}

type dropboxListFolderResponse struct {
	Entries []dropboxEntry `json:"entries"`
	Cursor  string         `json:"cursor"`
	HasMore bool           `json:"has_more"`
}

type dropboxEntry struct {
	Tag            string `json:".tag"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	PathDisplay    string `json:"path_display"`
	PathLower      string `json:"path_lower"`
	ClientModified string `json:"client_modified"`
	ServerModified string `json:"server_modified"`
	Size           int64  `json:"size"`
}

type dropboxFileMetadata struct {
	Tag            string
	ID             string
	Name           string
	PathDisplay    string
	PathLower      string
	ClientModified string
	ServerModified string
	Size           int64
}

func dropboxFilesFromEntries(entries []dropboxEntry) []dropboxFileMetadata {
	files := make([]dropboxFileMetadata, 0, len(entries))
	for _, entry := range entries {
		if entry.Tag != "file" {
			continue
		}
		files = append(files, dropboxFileMetadata{
			Tag:            entry.Tag,
			ID:             entry.ID,
			Name:           entry.Name,
			PathDisplay:    entry.PathDisplay,
			PathLower:      entry.PathLower,
			ClientModified: entry.ClientModified,
			ServerModified: entry.ServerModified,
			Size:           entry.Size,
		})
	}
	return files
}

func (f dropboxFileMetadata) sourceID() string {
	return "dropbox:" + f.ID
}

func (f dropboxFileMetadata) pathDisplay() string {
	return firstNonEmpty(f.PathDisplay, f.PathLower, "/"+f.Name)
}

func (f dropboxFileMetadata) downloadPath() string {
	if f.PathDisplay != "" {
		return f.PathDisplay
	}
	if f.PathLower != "" {
		return f.PathLower
	}
	return f.ID
}

func (f dropboxFileMetadata) updatedAt() time.Time {
	if parsed := parseDropboxTime(f.ClientModified); !parsed.IsZero() {
		return parsed
	}
	if parsed := parseDropboxTime(f.ServerModified); !parsed.IsZero() {
		return parsed
	}
	return time.Unix(0, 0).UTC()
}

func parseDropboxTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05Z", value); err == nil {
		return parsed.UTC()
	}
	return parseFeedTime(value)
}

func dropboxNameCounts(files []dropboxFileMetadata) map[string]int {
	counts := map[string]int{}
	for _, file := range files {
		counts[file.Name]++
	}
	return counts
}

func dropboxSemanticIdentifier(file dropboxFileMetadata, nameCounts map[string]int) string {
	if nameCounts[file.Name] <= 1 {
		return file.Name
	}
	relative := strings.TrimPrefix(file.pathDisplay(), "/")
	if relative == "" {
		return file.Name
	}
	return strings.ReplaceAll(relative, "/", " / ")
}

func dropboxExtension(name string) string {
	return strings.ToLower(path.Ext(name))
}

func dropboxHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(data))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("Dropbox request failed with status %d: %s", resp.StatusCode, message)
}

func dropboxSizeThreshold() int64 {
	if raw := strings.TrimSpace(os.Getenv("BLOB_STORAGE_SIZE_THRESHOLD")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultDropboxSizeThreshold
}
