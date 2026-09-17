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
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	zoteroAPIBaseURL         = "https://api.zotero.org"
	defaultZoteroBatchSize   = 4
	zoteroRequestTimeout     = 120 * time.Second
	zoteroDefaultPageSize    = 100
	zoteroMaxAttachmentBytes = 100 * 1024 * 1024
	zoteroStorageModeZotero  = "zotero_storage"
	zoteroStorageModeWebDAV  = "webdav"
	zoteroMaxFileRedirects   = 10
)

// ZoteroConnector syncs PDF attachments from a Zotero library.
type ZoteroConnector struct {
	userID      string
	apiKey      string
	storageMode string
	webdavURL   string
	webdavUser  string
	webdavPass  string
	batchSize   int
	httpClient  *http.Client
	listItems   func(ctx context.Context, start int) ([]zoteroAPIItem, int, error)
	downloadPDF func(ctx context.Context, attachment zoteroAPIItem) ([]byte, string, error)
}

type zoteroAPIItem struct {
	Key  string         `json:"key"`
	Data zoteroItemData `json:"data"`
}

type zoteroItemData struct {
	ItemType     string `json:"itemType"`
	Title        string `json:"title"`
	Filename     string `json:"filename"`
	ContentType  string `json:"contentType"`
	LinkMode     string `json:"linkMode"`
	DateModified string `json:"dateModified"`
	ParentItem   string `json:"parentItem"`
}

type zoteroPDFRecord struct {
	attachment zoteroAPIItem
	updatedAt  time.Time
	title      string
}

// NewZoteroConnector creates a Zotero connector from stored connector config.
func NewZoteroConnector(config map[string]any) (*ZoteroConnector, error) {
	credentials := configAnyMap(config["credentials"])
	storageMode := strings.TrimSpace(firstNonEmpty(stringConfig(config["storage_mode"]), zoteroStorageModeZotero))
	if storageMode != zoteroStorageModeZotero && storageMode != zoteroStorageModeWebDAV {
		return nil, &ConnectorValidationError{Message: "storage_mode must be 'zotero_storage' or 'webdav'"}
	}
	webdavURL := strings.TrimRight(strings.TrimSpace(stringConfig(config["webdav_url"])), "/")
	connector := &ZoteroConnector{
		userID:      strings.TrimSpace(firstNonEmpty(stringConfig(config["zotero_user_id"]), stringConfig(credentials["zotero_user_id"]))),
		apiKey:      strings.TrimSpace(stringConfig(credentials["zotero_api_key"])),
		storageMode: storageMode,
		webdavURL:   webdavURL,
		webdavUser:  strings.TrimSpace(stringConfig(credentials["webdav_username"])),
		webdavPass:  stringConfig(credentials["webdav_password"]),
		batchSize:   configInt(config["batch_size"], defaultZoteroBatchSize),
		httpClient: &http.Client{
			Timeout: zoteroRequestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	connector.listItems = connector.defaultListAttachmentItems
	connector.downloadPDF = connector.defaultDownloadPDF
	return connector, nil
}

// Validate validates Zotero settings and credentials.
func (c *ZoteroConnector) Validate(ctx context.Context) error {
	if err := c.validateStatic(); err != nil {
		return err
	}
	if err := c.validateWebDAVAccess(ctx); err != nil {
		return err
	}
	if _, _, err := c.listItems(ctx, 0); err != nil {
		return classifyZoteroError(err)
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved Zotero config.
func (c *ZoteroConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	connector, err := NewZoteroConnector(request)
	if err != nil {
		return err
	}
	connector.httpClient = c.httpClient
	return connector.Validate(ctx)
}

// OpenSync opens one Zotero sync session.
func (c *ZoteroConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	if err := c.validateWebDAVAccess(ctx); err != nil {
		return nil, err
	}
	records, err := c.collectPDFRecords(ctx, request)
	if err != nil {
		return nil, classifyZoteroError(err)
	}
	sort.Slice(records, func(i, j int) bool {
		return c.sourceID(records[i].attachment.Key) < c.sourceID(records[j].attachment.Key)
	})
	session := &zoteroSyncSession{connector: c, records: records, batchSize: c.batchSize}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Zotero prune snapshot session.
func (c *ZoteroConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	if err := c.validateWebDAVAccess(ctx); err != nil {
		return nil, err
	}
	records, err := c.collectPDFRecords(ctx, SyncRequest{FromBeginning: true})
	if err != nil {
		return nil, classifyZoteroError(err)
	}
	documents := make([]SlimDocument, 0, len(records))
	for _, record := range records {
		documents = append(documents, SlimDocument{SourceID: c.sourceID(record.attachment.Key)})
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].SourceID < documents[j].SourceID })
	return &zoteroPruneSession{documents: documents, batchSize: c.batchSize}, nil
}

func (c *ZoteroConnector) validateStatic() error {
	if c == nil {
		return &ConnectorValidationError{Message: "Zotero connector is nil"}
	}
	if c.userID == "" {
		return &ConnectorMissingCredentialError{Message: "Zotero user ID is required"}
	}
	if c.apiKey == "" {
		return &ConnectorMissingCredentialError{Message: "Zotero API key is required"}
	}
	if c.storageMode == zoteroStorageModeWebDAV {
		if c.webdavURL == "" {
			return &ConnectorValidationError{Message: "webdav_url is required when storage_mode is webdav"}
		}
		parsed, err := url.Parse(c.webdavURL)
		if err != nil || parsed.Scheme != "https" {
			return &ConnectorValidationError{Message: "WebDAV URL must use HTTPS"}
		}
		if c.webdavUser == "" {
			return &ConnectorMissingCredentialError{Message: "WebDAV username is required when storage_mode is webdav"}
		}
		if c.webdavPass == "" {
			return &ConnectorMissingCredentialError{Message: "WebDAV password is required when storage_mode is webdav"}
		}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	return nil
}

func (c *ZoteroConnector) validateWebDAVAccess(ctx context.Context) error {
	if c.storageMode != zoteroStorageModeWebDAV {
		return nil
	}
	probeURL := strings.TrimRight(c.webdavURL, "/") + "/zotero/"
	_, _, err := assertRestAPIURLSafe(ctx, probeURL)
	if err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("WebDAV URL is not allowed: %v", err)}
	}
	return nil
}

func (c *ZoteroConnector) collectPDFRecords(ctx context.Context, request SyncRequest) ([]zoteroPDFRecord, error) {
	records := make([]zoteroPDFRecord, 0)
	start := 0
	for {
		items, total, err := c.listItems(ctx, start)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !zoteroIsPDFAttachment(item) {
				continue
			}
			updatedAt, err := zoteroParseTime(item.Data.DateModified)
			if err != nil {
				continue
			}
			if !request.FromBeginning && request.WindowStart != nil {
				if !updatedAt.After(*request.WindowStart) {
					continue
				}
			}
			if !request.WindowEnd.IsZero() && updatedAt.After(request.WindowEnd) {
				continue
			}
			records = append(records, zoteroPDFRecord{
				attachment: item,
				updatedAt:  updatedAt,
				title:      zoteroAttachmentTitle(item),
			})
		}
		start += len(items)
		if len(items) == 0 || start >= total {
			break
		}
	}
	return records, nil
}

func (c *ZoteroConnector) buildDocument(record zoteroPDFRecord, blob []byte, filename string) SourceDocument {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		ext = ".pdf"
	}
	return SourceDocument{
		SourceID:           c.sourceID(record.attachment.Key),
		SemanticIdentifier: record.title,
		Extension:          strings.TrimPrefix(ext, "."),
		Blob:               blob,
		UpdatedAt:          record.updatedAt,
		SizeBytes:          int64(len(blob)),
		Fingerprint:        contentFingerprint(blob),
		Metadata: map[string]any{
			"source":          "zotero",
			"zotero_item_key": record.attachment.Key,
			"zotero_parent":   record.attachment.Data.ParentItem,
			"filename":        filename,
		},
	}
}

func (c *ZoteroConnector) sourceID(attachmentKey string) string {
	return "zotero:" + c.userID + ":" + attachmentKey
}

func (c *ZoteroConnector) defaultListAttachmentItems(ctx context.Context, start int) ([]zoteroAPIItem, int, error) {
	endpoint := fmt.Sprintf("%s/users/%s/items", zoteroAPIBaseURL, c.userID)
	query := fmt.Sprintf("?itemType=attachment&format=json&limit=%d&start=%d", zoteroDefaultPageSize, start)
	body, headers, err := c.doZoteroRequest(ctx, endpoint+query, http.MethodGet, nil)
	if err != nil {
		return nil, 0, err
	}
	var items []zoteroAPIItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, 0, err
	}
	total := start + len(items)
	if headers != nil {
		if raw := strings.TrimSpace(headers.Get("Total-Results")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > total {
				total = parsed
			}
		}
	}
	return items, total, nil
}

func (c *ZoteroConnector) doZoteroRequest(ctx context.Context, url string, method string, payload []byte) ([]byte, http.Header, error) {
	requestCtx, cancel := context.WithTimeout(ctx, zoteroRequestTimeout)
	defer cancel()
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(requestCtx, method, url, body)
	if err != nil {
		return nil, nil, err
	}
	if hostAllowsZoteroAPIKey(req.URL.Hostname()) {
		req.Header.Set("Zotero-API-Key", c.apiKey)
	}
	req.Header.Set("Zotero-API-Version", "3")
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		io.Copy(io.Discard, resp.Body)
		return nil, resp.Header.Clone(), &ConnectorValidationError{Message: "Unexpected redirect from Zotero API"}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, zoteroMaxAttachmentBytes+1024))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.Header.Clone(), &zoteroHTTPError{Status: resp.StatusCode, Body: string(data), URL: url}
	}
	return data, resp.Header.Clone(), nil
}

func (c *ZoteroConnector) defaultDownloadPDF(ctx context.Context, attachment zoteroAPIItem) ([]byte, string, error) {
	filename := strings.TrimSpace(attachment.Data.Filename)
	if filename == "" {
		filename = attachment.Data.Title + ".pdf"
	}
	switch c.storageMode {
	case zoteroStorageModeWebDAV:
		return c.downloadPDFViaWebDAV(ctx, attachment.Key, filename)
	default:
		return c.downloadPDFViaZoteroAPI(ctx, attachment.Key, filename)
	}
}

func (c *ZoteroConnector) downloadPDFViaZoteroAPI(ctx context.Context, attachmentKey, filename string) ([]byte, string, error) {
	fileURL := fmt.Sprintf("%s/users/%s/items/%s/file", zoteroAPIBaseURL, c.userID, attachmentKey)
	data, err := c.downloadAuthedURL(ctx, fileURL)
	if err != nil {
		return nil, "", err
	}
	return data, filename, nil
}

func (c *ZoteroConnector) downloadAuthedURL(ctx context.Context, rawURL string) ([]byte, error) {
	currentURL := rawURL
	for redirects := 0; redirects < zoteroMaxFileRedirects; redirects++ {
		data, err := c.getPinnedURL(ctx, currentURL, func(req *http.Request) {
			req.Header.Set("Zotero-API-Version", "3")
			if hostAllowsZoteroAPIKey(req.URL.Hostname()) {
				req.Header.Set("Zotero-API-Key", c.apiKey)
			}
		})
		if err != nil {
			var redirect redirectError
			if errors.As(err, &redirect) {
				currentURL = redirect.Location
				continue
			}
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("stopped after too many Zotero file redirects")
}

type redirectError struct {
	Location string
}

func (e redirectError) Error() string {
	return "HTTP redirect to " + e.Location
}

func hostAllowsZoteroAPIKey(hostname string) bool {
	return strings.EqualFold(strings.TrimSuffix(hostname, "."), "api.zotero.org")
}

func (c *ZoteroConnector) getPinnedURL(ctx context.Context, rawURL string, authorize func(*http.Request)) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, fmt.Errorf("Zotero download URL must use HTTPS")
	}
	hostname, pinIP, err := assertRestAPIURLSafe(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, zoteroRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if authorize != nil {
		authorize(req)
	}
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	transport := newRestAPIPinnedTransport(hostname, pinIP)
	httpClient := *client
	httpClient.Transport = transport
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location == "" {
			return nil, fmt.Errorf("Zotero file redirect missing Location header")
		}
		next, err := resp.Request.URL.Parse(location)
		if err != nil {
			return nil, err
		}
		return nil, redirectError{Location: next.String()}
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &zoteroHTTPError{Status: resp.StatusCode, Body: string(body), URL: rawURL}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, zoteroMaxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > zoteroMaxAttachmentBytes {
		return nil, fmt.Errorf("Zotero attachment exceeds maximum size")
	}
	return data, nil
}

func (c *ZoteroConnector) downloadPDFViaWebDAV(ctx context.Context, attachmentKey, filename string) ([]byte, string, error) {
	zipURL := strings.TrimRight(c.webdavURL, "/") + "/zotero/" + attachmentKey + ".zip"
	data, err := c.getPinnedURL(ctx, zipURL, func(req *http.Request) {
		req.SetBasicAuth(c.webdavUser, c.webdavPass)
	})
	if err != nil {
		return nil, "", err
	}
	return extractPDFFromZip(data, filename)
}

func extractPDFFromZip(zipBytes []byte, fallbackName string) ([]byte, string, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, "", nil
	}
	var pdfName string
	var pdfData []byte
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(file.Name)
		if !strings.HasSuffix(strings.ToLower(name), ".pdf") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, "", err
		}
		data, err := io.ReadAll(io.LimitReader(rc, zoteroMaxAttachmentBytes+1))
		rc.Close()
		if err != nil {
			return nil, "", err
		}
		if int64(len(data)) > zoteroMaxAttachmentBytes {
			continue
		}
		if len(data) == 0 {
			continue
		}
		pdfName = name
		pdfData = data
		break
	}
	if len(pdfData) == 0 {
		return nil, "", fmt.Errorf("no PDF found in Zotero WebDAV archive for %s", fallbackName)
	}
	return pdfData, pdfName, nil
}

func zoteroIsPDFAttachment(item zoteroAPIItem) bool {
	if item.Data.ItemType != "attachment" {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(item.Data.ContentType))
	if strings.Contains(contentType, "pdf") {
		return true
	}
	filename := strings.ToLower(item.Data.Filename)
	return strings.HasSuffix(filename, ".pdf")
}

func zoteroAttachmentTitle(item zoteroAPIItem) string {
	title := strings.TrimSpace(item.Data.Title)
	if title != "" {
		return title
	}
	if name := strings.TrimSpace(item.Data.Filename); name != "" {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}
	return "Zotero attachment " + item.Key
}

func zoteroParseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("missing timestamp")
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported Zotero timestamp %q", value)
}

type zoteroHTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *zoteroHTTPError) Error() string {
	return fmt.Sprintf("Zotero API returned HTTP %d for %s: %s", e.Status, e.URL, strings.TrimSpace(e.Body))
}

func classifyZoteroError(err error) error {
	if err == nil {
		return nil
	}
	var httpErr *zoteroHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &ConnectorMissingCredentialError{Message: "Zotero API key is invalid or expired"}
		case http.StatusNotFound:
			return &ConnectorValidationError{Message: "Zotero library or attachment was not found"}
		default:
			return &ConnectorValidationError{Message: httpErr.Error()}
		}
	}
	return err
}

type zoteroSyncSession struct {
	connector        *ZoteroConnector
	records          []zoteroPDFRecord
	batchSize        int
	index            int
	downloadFailures int
}

func (s *zoteroSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	for len(documents) < s.batchSize && s.index < len(s.records) {
		record := s.records[s.index]
		blob, filename, err := s.connector.downloadPDF(ctx, record.attachment)
		if err != nil || len(blob) == 0 {
			s.downloadFailures++
			slog.Warn(
				"zotero: attachment download failed",
				"item_key", record.attachment.Key,
				"source_id", s.connector.sourceID(record.attachment.Key),
				"error", err,
			)
			if len(documents) > 0 {
				break
			}
			s.index++
			continue
		}
		if int64(len(blob)) > zoteroMaxAttachmentBytes {
			s.downloadFailures++
			slog.Warn(
				"zotero: skipping oversized attachment",
				"item_key", record.attachment.Key,
				"source_id", s.connector.sourceID(record.attachment.Key),
				"bytes", len(blob),
			)
			if len(documents) > 0 {
				break
			}
			s.index++
			continue
		}
		documents = append(documents, s.connector.buildDocument(record, blob, filename))
		s.index++
	}
	if len(documents) == 0 {
		if s.index >= len(s.records) {
			if s.downloadFailures > 0 {
				slog.Warn("zotero: sync completed with attachment download failures", "count", s.downloadFailures)
			}
			return SyncBatch{}, io.EOF
		}
		return SyncBatch{}, fmt.Errorf("zotero: failed to download attachment %q", s.records[s.index].attachment.Key)
	}
	return SyncBatch{Documents: documents, Checkpoint: zoteroSyncCheckpoint(documents[len(documents)-1])}, nil
}

func (s *zoteroSyncSession) Close() error { return nil }

func (s *zoteroSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("zotero sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, record := range s.records {
		if s.connector.sourceID(record.attachment.Key) == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("zotero resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

func zoteroSyncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{Cursor: doc.SourceID, SourceID: doc.SourceID, UpdatedAt: &updatedAt}
}

type zoteroPruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

func (s *zoteroPruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

func (s *zoteroPruneSession) Close() error { return nil }
