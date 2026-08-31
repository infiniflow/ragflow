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
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWebDAVBatchSize     = 2
	defaultWebDAVSizeThreshold = 20 * 1024 * 1024
	webdavRequestTimeout       = 60 * time.Second
	maxWebDAVResponseSize      = 32 * 1024 * 1024
	webdavPropfindBody         = `<?xml version="1.0" encoding="utf-8"?><propfind xmlns="DAV:"><allprop/></propfind>`
)

var (
	webdavTextExtensions = map[string]struct{}{
		".txt": {}, ".md": {}, ".mdx": {}, ".conf": {}, ".log": {}, ".json": {},
		".csv": {}, ".tsv": {}, ".xml": {}, ".yml": {}, ".yaml": {}, ".sql": {},
	}
	webdavDocumentExtensions = map[string]struct{}{
		".pdf": {}, ".docx": {}, ".pptx": {}, ".xlsx": {}, ".eml": {}, ".epub": {}, ".html": {},
	}
	webdavImageExtensions = map[string]struct{}{
		".jpg": {}, ".jpeg": {}, ".png": {}, ".gif": {}, ".bmp": {}, ".tiff": {}, ".webp": {},
	}
)

// WebDAVConnector reads files from a WebDAV server.
type WebDAVConnector struct {
	baseURL       string
	remotePath    string
	batchSize     int
	allowImages   bool
	username      string
	password      string
	sizeThreshold int64
	client        *webdavClient
	listFiles     func(ctx context.Context, target string) ([]webdavFile, error)
	downloadFile  func(ctx context.Context, fileURL string) ([]byte, error)
}

// NewWebDAVConnector creates a WebDAV connector from stored connector config.
func NewWebDAVConnector(config map[string]any) (*WebDAVConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	baseURL := strings.TrimRight(strings.TrimSpace(stringConfig(config["base_url"])), "/")
	remotePath := normalizeWebDAVPath(stringConfig(config["remote_path"]))
	username := strings.TrimSpace(stringConfig(credentials["username"]))
	password := stringConfig(credentials["password"])
	httpClient, err := newWebDAVHTTPClient(strings.TrimSpace(stringConfig(config["ca_cert_path"])))
	if err != nil {
		return nil, err
	}
	connector := &WebDAVConnector{
		baseURL:       baseURL,
		remotePath:    remotePath,
		batchSize:     configInt(config["batch_size"], defaultWebDAVBatchSize),
		allowImages:   configBoolDefault(config["allow_images"], false),
		username:      username,
		password:      password,
		sizeThreshold: webDAVSizeThreshold(),
		client: &webdavClient{
			baseURL:    baseURL,
			httpClient: httpClient,
		},
	}
	connector.client.username = username
	connector.client.password = password
	connector.listFiles = connector.client.listRecursive
	connector.downloadFile = connector.client.download
	return connector, nil
}

func newWebDAVHTTPClient(caCertPath string) (*http.Client, error) {
	client := &http.Client{Timeout: webdavRequestTimeout}
	if caCertPath == "" {
		return client, nil
	}

	caPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("failed to read WebDAV CA certificate %q: %v", caCertPath, err)}
	}
	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if !rootCAs.AppendCertsFromPEM(caPEM) {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("WebDAV CA certificate %q contains no valid PEM certificates", caCertPath)}
	}

	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("unsupported default HTTP transport type %T", http.DefaultTransport)
	}
	transport := defaultTransport.Clone()
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = &tls.Config{}
	} else {
		transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	transport.TLSClientConfig.RootCAs = rootCAs
	client.Transport = transport
	return client, nil
}

// Validate validates WebDAV settings and credentials by probing the remote path.
func (c *WebDAVConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("webdav connector is nil")
	}
	if c.baseURL == "" {
		return fmt.Errorf("WebDAV base URL is required")
	}
	if c.username == "" || c.password == "" {
		return fmt.Errorf("WebDAV requires username and password credentials")
	}
	if c.batchSize <= 0 {
		return fmt.Errorf("batch_size must be a positive integer")
	}

	testPath := c.remotePath
	if testPath != "/" {
		testPath = strings.TrimRight(testPath, "/") + "/"
	}
	_, err := c.client.propfind(ctx, testPath)
	if err == nil {
		return nil
	}
	var statusErr *webdavStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.status {
		case http.StatusUnauthorized:
			return fmt.Errorf("WebDAV credentials appear invalid or expired")
		case http.StatusForbidden:
			return fmt.Errorf("insufficient permissions to access path '%s' on WebDAV server", c.remotePath)
		case http.StatusNotFound:
			return fmt.Errorf("remote path '%s' does not exist on WebDAV server", c.remotePath)
		}
	}
	return fmt.Errorf("WebDAV validation failed for path '%s': %w", testPath, err)
}

// ValidateConnectorSetting validates WebDAV settings from an unsaved config.
func (c *WebDAVConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one WebDAV sync session.
func (c *WebDAVConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	files, err := c.listFiles(ctx, c.remotePath)
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

	nameCounts := webdavBasenameCounts(files)
	documents := make([]SourceDocument, 0, len(files))
	for _, file := range files {
		if !file.hasModified {
			file.modified = end
		}
		if !start.Before(file.modified) || file.modified.After(end) {
			continue
		}
		if !c.isAcceptedExtension(file) || !file.hasSize || file.size > c.sizeThreshold {
			continue
		}
		blob, err := c.downloadFile(ctx, file.url)
		if err != nil || len(blob) == 0 {
			continue
		}
		documents = append(documents, c.buildDocument(file, blob, nameCounts))
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })

	session := &webdavSyncSession{documents: documents, batchSize: c.batchSize}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete WebDAV prune snapshot session.
func (c *WebDAVConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	files, err := c.listFiles(ctx, c.remotePath)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0, len(files))
	for _, file := range files {
		if !c.isAcceptedExtension(file) || !file.hasSize || file.size > c.sizeThreshold {
			continue
		}
		documents = append(documents, SlimDocument{SourceID: c.sourceID(file.url)})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	return &webdavPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

func (c *WebDAVConnector) buildDocument(file webdavFile, blob []byte, nameCounts map[string]int) SourceDocument {
	return SourceDocument{
		SourceID:           c.sourceID(file.url),
		SemanticIdentifier: webdavSemanticIdentifier(file, c.remotePath, nameCounts),
		Extension:          webdavExtension(file.url),
		Blob:               blob,
		UpdatedAt:          file.modified,
		SizeBytes:          file.size,
		Fingerprint:        contentFingerprint(blob),
	}
}

func (c *WebDAVConnector) sourceID(fileURL string) string {
	return "webdav:" + c.baseURL + ":" + fileURL
}

func (c *WebDAVConnector) isAcceptedExtension(file webdavFile) bool {
	ext := webdavExtension(file.url)
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

type webdavSyncSession struct {
	documents []SourceDocument
	batchSize int
	index     int
}

// NextBatch returns the next WebDAV document batch.
func (s *webdavSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.documents) {
		return SyncBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batchDocuments := s.documents[s.index:end]
	batch := SyncBatch{Documents: batchDocuments, Checkpoint: webdavSyncCheckpoint(batchDocuments[len(batchDocuments)-1])}
	s.index = end
	return batch, nil
}

// Close closes the WebDAV sync session.
func (s *webdavSyncSession) Close() error {
	return nil
}

// applyResume advances past the last committed WebDAV document when retrying a task.
func (s *webdavSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("webdav sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, doc := range s.documents {
		if doc.SourceID == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("webdav resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

// webdavSyncCheckpoint returns a resume point after a committed WebDAV document.
func webdavSyncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    doc.SourceID,
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

type webdavPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

// NextBatch returns the next WebDAV prune snapshot batch.
func (s *webdavPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	if s.index >= len(s.documents) {
		return PruneBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	batch := PruneBatch{Documents: s.documents[s.index:end]}
	s.index = end
	return batch, nil
}

// Close closes the WebDAV prune session.
func (s *webdavPruneSession) Close() error {
	return nil
}

type webdavFile struct {
	url         string
	isDir       bool
	size        int64
	hasSize     bool
	modified    time.Time
	hasModified bool
}

type webdavClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// listRecursive lists every file under target using Depth-1 PROPFIND walks.
func (c *webdavClient) listRecursive(ctx context.Context, target string) ([]webdavFile, error) {
	var files []webdavFile
	visited := map[string]struct{}{}
	var walk func(current string) error
	walk = func(current string) error {
		key := webDAVPathKey(current)
		if _, ok := visited[key]; ok {
			return nil
		}
		visited[key] = struct{}{}

		items, err := c.propfind(ctx, current)
		if err != nil {
			return err
		}
		for _, item := range items {
			if webDAVPathKey(item.url) == key {
				continue // the collection itself
			}
			if item.isDir {
				if err := walk(item.url); err != nil {
					return err
				}
				continue
			}
			files = append(files, item)
		}
		return nil
	}
	if err := walk(target); err != nil {
		return nil, err
	}
	return files, nil
}

// propfind issues a Depth-1 PROPFIND and parses the multistatus response.
func (c *webdavClient) propfind(ctx context.Context, target string) ([]webdavFile, error) {
	resolved, err := c.resolve(target)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", resolved, bytes.NewReader([]byte(webdavPropfindBody)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, &webdavStatusError{status: resp.StatusCode, url: resolved}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxWebDAVResponseSize))
	if err != nil {
		return nil, err
	}
	return parseWebDAVMultistatus(data, c.baseURL)
}

// download fetches a file body over GET.
func (c *webdavClient) download(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &webdavStatusError{status: resp.StatusCode, url: fileURL}
	}
	blob, err := io.ReadAll(io.LimitReader(resp.Body, maxWebDAVResponseSize+1))
	if err != nil {
		return nil, err
	}
	if len(blob) > maxWebDAVResponseSize {
		return nil, fmt.Errorf("webdav response exceeds maximum size of %d bytes", maxWebDAVResponseSize)
	}
	return blob, nil
}

// resolve joins a server-relative path or href onto the configured base URL.
func (c *webdavClient) resolve(target string) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid WebDAV base URL")
	}
	return resolveWebDAVURL(c.baseURL, target)
}

// resolveWebDAVURL joins a target onto a WebDAV base URL without letting a
// leading slash discard a non-root mount path.
func resolveWebDAVURL(baseURL, target string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if ref.IsAbs() || ref.Host != "" {
		if ref.Scheme == "" {
			ref.Scheme = base.Scheme
		}
		if ref.Scheme != base.Scheme || ref.Host != base.Host {
			return "", fmt.Errorf("webdav href resolves to a different origin than base URL: %s", ref.String())
		}
		return ref.String(), nil
	}
	resolved := *base
	switch {
	case base.Path == "" || base.Path == "/":
		resolved.Path = ref.Path
	case ref.Path == base.Path || strings.HasPrefix(ref.Path, base.Path+"/"):
		resolved.Path = ref.Path
	default:
		resolved.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(ref.Path, "/")
	}
	resolved.RawPath = ""
	resolved.RawQuery = ref.RawQuery
	resolved.Fragment = ref.Fragment
	return resolved.String(), nil
}

type webdavStatusError struct {
	status int
	url    string
}

func (e *webdavStatusError) Error() string {
	return fmt.Sprintf("webdav request failed with status %d for %s", e.status, e.url)
}

type webdavMultistatus struct {
	Responses []webdavResponse `xml:"response"`
}

type webdavResponse struct {
	Href     string         `xml:"href"`
	Propstat webdavPropstat `xml:"propstat"`
}

type webdavPropstat struct {
	Prop webdavProp `xml:"prop"`
}

type webdavProp struct {
	ResourceType    webdavResourceType `xml:"resourcetype"`
	ContentLength   string             `xml:"getcontentlength"`
	GetLastModified string             `xml:"getlastmodified"`
}

type webdavResourceType struct {
	Collection *string `xml:"collection"`
}

// parseWebDAVMultistatus converts a PROPFIND multistatus document into files.
func parseWebDAVMultistatus(data []byte, baseURL string) ([]webdavFile, error) {
	var status webdavMultistatus
	if err := xml.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to parse WebDAV PROPFIND response: %w", err)
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid WebDAV base URL")
	}
	files := make([]webdavFile, 0, len(status.Responses))
	for _, response := range status.Responses {
		href := strings.TrimSpace(response.Href)
		if href == "" {
			continue
		}
		fileURL, err := resolveWebDAVURL(baseURL, href)
		if err != nil {
			continue
		}
		file := webdavFile{
			url:   fileURL,
			isDir: response.Propstat.Prop.ResourceType.Collection != nil,
		}
		if raw := strings.TrimSpace(response.Propstat.Prop.ContentLength); raw != "" {
			if size, err := strconv.ParseInt(raw, 10, 64); err == nil && size >= 0 {
				file.size = size
				file.hasSize = true
			}
		}
		if modified := parseFeedTime(response.Propstat.Prop.GetLastModified); !modified.IsZero() {
			file.modified = modified
			file.hasModified = true
		}
		files = append(files, file)
	}
	return files, nil
}

// webdavBasenameCounts counts basename occurrences across a listing.
func webdavBasenameCounts(files []webdavFile) map[string]int {
	counts := map[string]int{}
	for _, file := range files {
		counts[webdavBasename(file.url)]++
	}
	return counts
}

// webdavSemanticIdentifier prefers the file name, or the relative path when
// the name is not unique within one listing.
func webdavSemanticIdentifier(file webdavFile, remotePath string, nameCounts map[string]int) string {
	fileName := webdavBasename(file.url)
	if nameCounts[fileName] <= 1 {
		return fileName
	}
	parsed, err := url.Parse(file.url)
	if err != nil {
		return fileName
	}
	relative := strings.TrimPrefix(parsed.Path, remotePath)
	relative = strings.TrimPrefix(relative, "/")
	if relative == "" {
		return fileName
	}
	return strings.ReplaceAll(relative, "/", " / ")
}

func webdavBasename(fileURL string) string {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return ""
	}
	return path.Base(parsed.Path)
}

// webDAVPathKey normalizes a URL or server path to a comparable path key.
func webDAVPathKey(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	key := strings.TrimRight(parsed.Path, "/")
	if key == "" {
		return "/"
	}
	return key
}

func webdavExtension(fileURL string) string {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(path.Ext(parsed.Path))
}

// normalizeWebDAVPath normalizes a remote path the same way as the connector config.
func normalizeWebDAVPath(remotePath string) string {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" {
		return "/"
	}
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}
	if strings.HasSuffix(remotePath, "/") && remotePath != "/" {
		remotePath = strings.TrimRight(remotePath, "/")
	}
	return remotePath
}

// webDAVSizeThreshold returns the configured blob size threshold.
func webDAVSizeThreshold() int64 {
	if raw := strings.TrimSpace(os.Getenv("BLOB_STORAGE_SIZE_THRESHOLD")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultWebDAVSizeThreshold
}
