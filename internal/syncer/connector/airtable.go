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
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/utility"
)

const (
	airtableDefaultBatchSize     = 2
	airtableDefaultSizeThreshold = 10 * 1024 * 1024
	airtableAPIBaseURL           = "https://api.airtable.com/v0"
	airtableRequestTimeout       = 60 * time.Second
	airtableRetryCount           = 4
	airtableRetryBaseDelay       = 200 * time.Millisecond
	airtableMaxJSONResponseSize  = 32 * 1024 * 1024
	airtablePageSize             = 100
)

// AirtableConnector reads attachments from an Airtable table through the
// Airtable REST API using a personal access token.
type AirtableConnector struct {
	baseID        string
	tableNameOrID string
	lastModified  string
	accessToken   string
	batchSize     int
	sizeThreshold int64
	apiBaseURL    string
	httpClient    *http.Client

	listRecords  func(ctx context.Context, pageURL string) (airtableRecordPage, error)
	downloadFile func(ctx context.Context, rawURL string) ([]byte, error)
}

// NewAirtableConnector creates an Airtable connector from Python-compatible config.
func NewAirtableConnector(config map[string]any) (*AirtableConnector, error) {
	credentials := configAnyMap(config["credentials"])
	batchSize := airtableBatchSize(firstNonEmpty(stringConfig(config["sync_batch_size"]), stringConfig(config["batch_size"])))
	sizeThreshold := int64(configInt(config["size_threshold"], airtableDefaultSizeThreshold))
	if sizeThreshold <= 0 {
		sizeThreshold = airtableDefaultSizeThreshold
	}
	return &AirtableConnector{
		baseID:        strings.TrimSpace(stringConfig(config["base_id"])),
		tableNameOrID: strings.TrimSpace(stringConfig(config["table_name_or_id"])),
		lastModified:  strings.TrimSpace(stringConfig(config["last_modified_field"])),
		accessToken:   strings.TrimSpace(stringConfig(credentials["airtable_access_token"])),
		batchSize:     batchSize,
		sizeThreshold: sizeThreshold,
		apiBaseURL:    airtableAPIBaseURL,
		httpClient:    &http.Client{Timeout: airtableRequestTimeout},
	}, nil
}

func airtableBatchSize(value string) int {
	if strings.TrimSpace(value) == "" {
		return airtableDefaultBatchSize
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return airtableDefaultBatchSize
	}
	return parsed
}

// Validate validates Airtable config and probes the configured table.
func (c *AirtableConnector) Validate(ctx context.Context) error {
	if err := c.validateStatic(); err != nil {
		return err
	}
	if _, err := c.recordPage(ctx, c.recordsURL("", 1)); err != nil {
		var apiErr *airtableAPIError
		if errors.As(err, &apiErr) {
			switch apiErr.status {
			case http.StatusUnauthorized:
				return &ConnectorMissingCredentialError{Message: "Airtable access token is invalid or expired."}
			case http.StatusForbidden:
				return &ConnectorValidationError{Message: "Airtable token does not have permission to read this base or table."}
			case http.StatusNotFound:
				return &ConnectorValidationError{Message: "Airtable base or table was not found."}
			default:
				return &ConnectorValidationError{Message: fmt.Sprintf("Airtable validation failed (HTTP %d): %s", apiErr.status, apiErr.body)}
			}
		}
		return &ConnectorValidationError{Message: fmt.Sprintf("Airtable validation error: %v", err)}
	}
	return nil
}

// ValidateConnectorSetting validates an unsaved Airtable config through a
// candidate connector so request-derived credentials are used.
func (c *AirtableConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	candidate, err := NewAirtableConnector(request)
	if err != nil {
		return err
	}
	if c != nil {
		if c.httpClient != nil {
			candidate.httpClient = c.httpClient
		}
		if c.apiBaseURL != "" {
			candidate.apiBaseURL = c.apiBaseURL
		}
	}
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return candidate.Validate(ctx)
}

// OpenSync opens one Airtable sync session.
func (c *AirtableConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	session := &airtableSyncSession{
		connector: c,
		request:   request,
		batchSize: c.effectiveBatchSize(),
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete Airtable prune snapshot session.
func (c *AirtableConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	return &airtablePruneSession{
		connector: c,
		batchSize: c.effectiveBatchSize(),
	}, nil
}

func (c *AirtableConnector) validateStatic() error {
	if c == nil {
		return &ConnectorValidationError{Message: "Airtable connector is nil"}
	}
	if c.baseID == "" {
		return &ConnectorValidationError{Message: "Airtable base_id is required"}
	}
	if c.tableNameOrID == "" {
		return &ConnectorValidationError{Message: "Airtable table_name_or_id is required"}
	}
	if c.accessToken == "" {
		return &ConnectorMissingCredentialError{Message: "Airtable airtable_access_token is required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "Airtable connector batch_size must be a positive integer"}
	}
	return nil
}

func (c *AirtableConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return airtableDefaultBatchSize
}

func (c *AirtableConnector) recordsURL(offset string, pageSize int) string {
	query := url.Values{}
	if pageSize > 0 {
		query.Set("pageSize", strconv.Itoa(pageSize))
	}
	if offset != "" {
		query.Set("offset", offset)
	}
	base := strings.TrimRight(c.apiBaseURL, "/")
	return fmt.Sprintf("%s/%s/%s?%s", base, url.PathEscape(c.baseID), url.PathEscape(c.tableNameOrID), query.Encode())
}

func (c *AirtableConnector) recordPage(ctx context.Context, pageURL string) (airtableRecordPage, error) {
	if c.listRecords != nil {
		return c.listRecords(ctx, pageURL)
	}
	var page airtableRecordPage
	err := c.doJSON(ctx, pageURL, &page)
	return page, err
}

func (c *AirtableConnector) doJSON(ctx context.Context, apiURL string, out any) error {
	var lastErr error
	for attempt := 1; attempt <= airtableRetryCount; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, airtableRequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, apiURL, nil)
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.accessToken)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, airtableMaxJSONResponseSize+1))
			resp.Body.Close()
			cancel()
			if resp.StatusCode >= 400 {
				lastErr = &airtableAPIError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
				if !isAirtableRetryable(resp.StatusCode) {
					return lastErr
				}
			} else {
				if readErr != nil {
					return readErr
				}
				if int64(len(body)) > airtableMaxJSONResponseSize {
					return fmt.Errorf("Airtable API response exceeds maximum size of %d bytes", airtableMaxJSONResponseSize)
				}
				return json.Unmarshal(body, out)
			}
		}
		if attempt == airtableRetryCount {
			break
		}
		delay := time.Duration(attempt) * airtableRetryBaseDelay
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

func isAirtableRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500
}

// Fetch downloads a delayed Airtable attachment.
func (c *AirtableConnector) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	var fetch airtableFetchReference
	if err := json.Unmarshal([]byte(ref.Key), &fetch); err != nil {
		return nil, err
	}
	if fetch.RecordID == "" || fetch.AttachmentID == "" || fetch.URL == "" {
		return nil, fmt.Errorf("airtable fetch reference is incomplete")
	}
	if fetch.Size > c.sizeThreshold {
		return nil, fmt.Errorf("%s exceeds size threshold of %d", firstNonEmpty(fetch.Filename, fetch.AttachmentID), c.sizeThreshold)
	}
	return c.downloadAttachment(ctx, fetch.URL)
}

func (c *AirtableConnector) downloadAttachment(ctx context.Context, rawURL string) ([]byte, error) {
	if c.downloadFile != nil {
		return c.downloadFile(ctx, rawURL)
	}
	data, _, _, err := utility.FetchRemoteFileSafely(ctx, rawURL, c.sizeThreshold)
	return data, err
}

func (c *AirtableConnector) isAcceptedAttachment(recordID string, attachment airtableAttachment) bool {
	if recordID == "" || attachment.ID == "" || attachment.URL == "" || attachment.Filename == "" || attachment.CreatedTime == "" {
		return false
	}
	if attachment.Size < 0 || (attachment.Size > 0 && attachment.Size > c.sizeThreshold) {
		return false
	}
	return true
}

func (c *AirtableConnector) sourceDocument(recordID, fieldName string, attachment airtableAttachment) (SourceDocument, bool) {
	if !c.isAcceptedAttachment(recordID, attachment) {
		return SourceDocument{}, false
	}
	createdAt := parseOutlookTime(attachment.CreatedTime)
	if createdAt.IsZero() {
		return SourceDocument{}, false
	}
	fetch, _ := json.Marshal(airtableFetchReference{
		RecordID:     recordID,
		AttachmentID: attachment.ID,
		Filename:     attachment.Filename,
		URL:          attachment.URL,
		Size:         attachment.Size,
	})
	return SourceDocument{
		SourceID:           airtableSourceID(recordID, attachment.ID),
		SemanticIdentifier: attachment.Filename,
		Extension:          strings.ToLower(filepath.Ext(attachment.Filename)),
		FetchRef:           &FetchReference{Key: string(fetch), SizeHint: attachment.Size},
		UpdatedAt:          createdAt,
		SizeBytes:          attachment.Size,
		Metadata: map[string]any{
			"record_id":     recordID,
			"attachment_id": attachment.ID,
			"field_name":    fieldName,
			"filename":      attachment.Filename,
			"url":           attachment.URL,
			"created_time":  createdAt.UTC().Format(time.RFC3339Nano),
		},
		Fingerprint: airtableAttachmentFingerprint(recordID, fieldName, attachment),
	}, true
}

func (c *AirtableConnector) recordDocument(record airtableRecord) (SourceDocument, bool) {
	if strings.TrimSpace(record.ID) == "" {
		return SourceDocument{}, false
	}
	updatedAt := c.recordUpdatedAt(record)
	if updatedAt.IsZero() {
		return SourceDocument{}, false
	}
	blob := airtableRecordBlob(record)
	createdAt := parseOutlookTime(record.CreatedTime)
	metadata := map[string]any{
		"record_id":        record.ID,
		"base_id":          c.baseID,
		"table_name_or_id": c.tableNameOrID,
		"last_modified":    updatedAt.UTC().Format(time.RFC3339Nano),
	}
	if !createdAt.IsZero() {
		metadata["created_time"] = createdAt.UTC().Format(time.RFC3339Nano)
	}
	return SourceDocument{
		SourceID:           airtableRecordSourceID(record.ID),
		SemanticIdentifier: airtableRecordTitle(record),
		Extension:          ".json",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        airtableRecordFingerprint(record),
	}, true
}

func (c *AirtableConnector) recordUpdatedAt(record airtableRecord) time.Time {
	if field := strings.TrimSpace(c.lastModified); field != "" {
		if raw, ok := record.Fields[field]; ok {
			if updatedAt := parseOutlookTime(stringConfig(raw)); !updatedAt.IsZero() {
				return updatedAt
			}
		}
	}
	return parseOutlookTime(record.CreatedTime)
}

func airtableRecordBlob(record airtableRecord) []byte {
	blob, err := json.MarshalIndent(record.Fields, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return blob
}

func airtableRecordFingerprint(record airtableRecord) string {
	return contentFingerprint(airtableRecordBlob(record))
}

func airtableSourceID(recordID, attachmentID string) string {
	return "airtable:" + recordID + ":" + attachmentID
}

func airtableRecordSourceID(recordID string) string {
	return "airtable:" + recordID
}

func airtableRecordTitle(record airtableRecord) string {
	if name := strings.TrimSpace(stringConfig(record.Fields["Name"])); name != "" {
		return truncateRunes(name, 120)
	}
	fieldNames := make([]string, 0, len(record.Fields))
	for fieldName := range record.Fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)
	for _, fieldName := range fieldNames {
		value := strings.TrimSpace(stringConfig(record.Fields[fieldName]))
		if value != "" && len([]rune(value)) < 100 {
			return truncateRunes(value, 50)
		}
	}
	return "Record " + record.ID
}

func airtableAttachmentFingerprint(recordID, fieldName string, attachment airtableAttachment) string {
	return stableFingerprint(map[string]any{
		"record_id":     recordID,
		"field_name":    fieldName,
		"attachment_id": attachment.ID,
		"filename":      attachment.Filename,
		"size":          attachment.Size,
		"type":          attachment.Type,
		"url":           attachment.URL,
	})
}

func (c *AirtableConnector) includeAirtableRecord(request SyncRequest, record airtableRecord) bool {
	if request.FromBeginning {
		return true
	}
	updatedAt := c.recordUpdatedAt(record)
	if updatedAt.IsZero() {
		return false
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := airtableRecordFingerprint(record)
		stored, ok := request.Fingerprints[airtableRecordSourceID(record.ID)]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	if request.WindowStart != nil && updatedAt.Before(*request.WindowStart) {
		return false
	}
	if !request.WindowEnd.IsZero() && !updatedAt.Before(request.WindowEnd) {
		return false
	}
	return true
}

func includeAirtableAttachment(request SyncRequest, recordID string, attachment airtableAttachment) bool {
	if request.FromBeginning {
		return true
	}
	createdAt := parseOutlookTime(attachment.CreatedTime)
	if createdAt.IsZero() {
		return false
	}
	if len(request.Fingerprints) > 0 {
		fingerprint := airtableAttachmentFingerprint(recordID, attachment.FieldName, attachment)
		stored, ok := request.Fingerprints[airtableSourceID(recordID, attachment.ID)]
		return fingerprint == "" || !ok || stored == "" || stored != fingerprint
	}
	if request.WindowStart != nil && createdAt.Before(*request.WindowStart) {
		return false
	}
	if !request.WindowEnd.IsZero() && !createdAt.Before(request.WindowEnd) {
		return false
	}
	return true
}

// airtableSyncSession streams Airtable record pages and checkpoints after each
// emitted attachment.
type airtableSyncSession struct {
	connector *AirtableConnector
	request   SyncRequest
	batchSize int

	pageURL        string
	pending        []airtableBufferedDocument
	resumePageURL  string
	resumeOffset   int
	resumeSourceID string
	done           bool
}

// NextBatch returns the next Airtable source document batch.
func (s *airtableSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	documents := make([]SourceDocument, 0, s.batchSize)
	var checkpoint *SyncCheckpoint
	if len(s.pending) > 0 {
		n := min(s.batchSize, len(s.pending))
		for _, buffered := range s.pending[:n] {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
		s.pending = s.pending[n:]
	}
	for len(documents) < s.batchSize {
		if len(s.pending) == 0 {
			if s.done {
				if len(documents) == 0 {
					return SyncBatch{}, io.EOF
				}
				break
			}
			page, nextPageURL, err := s.nextDocumentPage(ctx)
			if err != nil {
				return SyncBatch{}, err
			}
			s.pending = page
			if nextPageURL == "" {
				s.done = true
			}
			if len(page) == 0 && !s.done {
				continue
			}
		}
		remaining := s.batchSize - len(documents)
		n := min(remaining, len(s.pending))
		for _, buffered := range s.pending[:n] {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
		s.pending = s.pending[n:]
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the Airtable sync session.
func (s *airtableSyncSession) Close() error {
	return nil
}

// Fetch downloads a delayed Airtable attachment for this sync session.
func (s *airtableSyncSession) Fetch(ctx context.Context, ref FetchReference) ([]byte, error) {
	return s.connector.Fetch(ctx, ref)
}

func (s *airtableSyncSession) nextDocumentPage(ctx context.Context) ([]airtableBufferedDocument, string, error) {
	pageURL := s.pageURL
	if pageURL == "" {
		pageURL = s.connector.recordsURL("", airtablePageSize)
	}
	page, err := s.connector.recordPage(ctx, pageURL)
	if err != nil {
		return nil, "", err
	}

	all := make([]airtableBufferedDocument, 0)
	included := make([]airtableBufferedDocument, 0)
	offset := 0
	for _, record := range page.Records {
		if recordDoc, ok := s.connector.recordDocument(record); ok {
			offset++
			buffered := airtableBufferedDocument{
				document:   recordDoc,
				checkpoint: s.checkpoint(airtableSyncCursor{PageURL: pageURL, Offset: offset, SourceID: recordDoc.SourceID}, recordDoc),
				offset:     offset,
			}
			all = append(all, buffered)
			if s.connector.includeAirtableRecord(s.request, record) {
				included = append(included, buffered)
			}
		}
		for _, attachment := range airtableAttachments(record) {
			if !s.connector.isAcceptedAttachment(record.ID, attachment) {
				continue
			}
			doc, ok := s.connector.sourceDocument(record.ID, attachment.FieldName, attachment)
			if !ok {
				continue
			}
			offset++
			buffered := airtableBufferedDocument{
				document:   doc,
				checkpoint: s.checkpoint(airtableSyncCursor{PageURL: pageURL, Offset: offset, SourceID: doc.SourceID}, doc),
				offset:     offset,
			}
			all = append(all, buffered)
			if includeAirtableAttachment(s.request, record.ID, attachment) {
				included = append(included, buffered)
			}
		}
	}

	documents, err := s.filterResumedDocuments(pageURL, all, included)
	if err != nil {
		return nil, "", err
	}
	nextPageURL := ""
	if page.Offset != "" {
		nextPageURL = s.connector.recordsURL(page.Offset, airtablePageSize)
		if nextPageURL == pageURL {
			return nil, "", fmt.Errorf("airtable sync pagination did not advance from %s", pageURL)
		}
	}
	s.pageURL = nextPageURL
	return documents, nextPageURL, nil
}

func (s *airtableSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("airtable sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor airtableSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("airtable sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" || cursor.PageURL == "" {
		return fmt.Errorf("airtable sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	s.pageURL = cursor.PageURL
	s.resumePageURL = cursor.PageURL
	s.resumeSourceID = sourceID
	s.resumeOffset = cursor.Offset
	return nil
}

func (s *airtableSyncSession) filterResumedDocuments(pageURL string, all, included []airtableBufferedDocument) ([]airtableBufferedDocument, error) {
	if s.resumeSourceID == "" {
		return included, nil
	}
	if pageURL != s.resumePageURL {
		return nil, fmt.Errorf("airtable resume page %q no longer matches checkpoint page %q: %w", pageURL, s.resumePageURL, ErrSyncResumeInvalid)
	}
	anchorOffset := -1
	for _, candidate := range all {
		if candidate.document.SourceID == s.resumeSourceID {
			anchorOffset = candidate.offset
			break
		}
	}
	if anchorOffset < 0 {
		return nil, fmt.Errorf("airtable resume anchor %q was not found on %s: %w", s.resumeSourceID, pageURL, ErrSyncResumeInvalid)
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

func (s *airtableSyncSession) checkpoint(cursor airtableSyncCursor, doc SourceDocument) *SyncCheckpoint {
	data, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{Cursor: string(data), UpdatedAt: &updatedAt, SourceID: doc.SourceID}
}

func (s *airtableSyncSession) clearResumeOffset() {
	s.resumePageURL = ""
	s.resumeSourceID = ""
	s.resumeOffset = 0
}

type airtableBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
	offset     int
}

type airtableSyncCursor struct {
	PageURL  string `json:"page_url"`
	Offset   int    `json:"offset,omitempty"`
	SourceID string `json:"source_id,omitempty"`
}

// airtablePruneSession streams the complete Airtable slim snapshot.
type airtablePruneSession struct {
	connector *AirtableConnector
	batchSize int
	pageURL   string
	buffer    []SlimDocument
	done      bool
}

// NextBatch returns the next Airtable prune snapshot batch.
func (s *airtablePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		if s.done {
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

// Close closes the Airtable prune session.
func (s *airtablePruneSession) Close() error {
	return nil
}

func (s *airtablePruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	pageURL := s.pageURL
	if pageURL == "" {
		pageURL = s.connector.recordsURL("", airtablePageSize)
	}
	page, err := s.connector.recordPage(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	documents := make([]SlimDocument, 0)
	for _, record := range page.Records {
		if _, ok := s.connector.recordDocument(record); ok {
			documents = append(documents, SlimDocument{SourceID: airtableRecordSourceID(record.ID)})
		}
		for _, attachment := range airtableAttachments(record) {
			if s.connector.isAcceptedAttachment(record.ID, attachment) {
				documents = append(documents, SlimDocument{SourceID: airtableSourceID(record.ID, attachment.ID)})
			}
		}
	}
	if page.Offset != "" {
		nextPageURL := s.connector.recordsURL(page.Offset, airtablePageSize)
		if nextPageURL == pageURL {
			return nil, fmt.Errorf("airtable prune pagination did not advance from %s", pageURL)
		}
		s.pageURL = nextPageURL
	} else {
		s.done = true
	}
	return documents, nil
}

func airtableAttachments(record airtableRecord) []airtableAttachment {
	attachments := []airtableAttachment{}
	fieldNames := make([]string, 0, len(record.Fields))
	for fieldName := range record.Fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)
	for _, fieldName := range fieldNames {
		raw := record.Fields[fieldName]
		list, ok := raw.([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			attachment, ok := airtableAttachmentFromMap(obj)
			if !ok {
				continue
			}
			attachment.FieldName = fieldName
			attachment.CreatedTime = record.CreatedTime
			attachments = append(attachments, attachment)
		}
	}
	return attachments
}

func airtableAttachmentFromMap(value map[string]any) (airtableAttachment, bool) {
	attachment := airtableAttachment{
		ID:       stringConfig(value["id"]),
		URL:      stringConfig(value["url"]),
		Filename: stringConfig(value["filename"]),
		Type:     stringConfig(value["type"]),
		Size:     airtableSizeValue(value["size"]),
	}
	if attachment.ID == "" || attachment.URL == "" || attachment.Filename == "" {
		return airtableAttachment{}, false
	}
	return attachment, true
}

func airtableSizeValue(value any) int64 {
	switch typed := value.(type) {
	case int:
		if typed >= 0 {
			return int64(typed)
		}
	case int64:
		if typed >= 0 {
			return typed
		}
	case float64:
		if typed >= 0 && typed == float64(int64(typed)) {
			return int64(typed)
		}
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil && parsed >= 0 {
			return parsed
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil && parsed >= 0 {
			return parsed
		}
	}
	return 0
}

type airtableRecordPage struct {
	Records []airtableRecord `json:"records"`
	Offset  string           `json:"offset,omitempty"`
}

type airtableRecord struct {
	ID          string         `json:"id"`
	CreatedTime string         `json:"createdTime"`
	Fields      map[string]any `json:"fields"`
}

type airtableAttachment struct {
	ID          string
	URL         string
	Filename    string
	Type        string
	Size        int64
	FieldName   string
	CreatedTime string
}

type airtableFetchReference struct {
	RecordID     string `json:"record_id"`
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename"`
	URL          string `json:"url"`
	Size         int64  `json:"size"`
}

type airtableAPIError struct {
	status int
	body   string
}

func (e *airtableAPIError) Error() string {
	return fmt.Sprintf("Airtable API returned HTTP %d: %s", e.status, e.body)
}
