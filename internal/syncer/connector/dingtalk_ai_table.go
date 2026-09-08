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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zeebo/xxh3"
)

const (
	defaultDingTalkAITableBatchSize = 32
	dingTalkAITableAPIBaseURL       = "https://api.dingtalk.com"
	dingTalkAITableDocIDPrefix      = "dingtalk_ai_table:"
	dingTalkAITableRequestTimeout   = 60 * time.Second
)

// DingTalkAITableConnector reads records from DingTalk AI Table.
type DingTalkAITableConnector struct {
	tableID     string
	operatorID  string
	accessToken string
	batchSize   int
	apiBaseURL  string
	httpClient  *http.Client

	getSheets   func(ctx context.Context) ([]dingTalkAITableSheet, error)
	listRecords func(ctx context.Context, sheetID, nextToken string, maxResults int) ([]dingTalkAITableRecord, string, error)
}

// NewDingTalkAITableConnector creates a DingTalk AI Table connector from
// Python-compatible config.
func NewDingTalkAITableConnector(config map[string]any) (*DingTalkAITableConnector, error) {
	credentials := configAnyMap(config["credentials"])
	return &DingTalkAITableConnector{
		tableID:     strings.TrimSpace(stringConfig(config["table_id"])),
		operatorID:  strings.TrimSpace(stringConfig(config["operator_id"])),
		accessToken: stringConfig(credentials["access_token"]),
		batchSize:   configInt(config["batch_size"], defaultDingTalkAITableBatchSize),
		apiBaseURL:  dingTalkAITableAPIBaseURL,
		httpClient:  &http.Client{Timeout: dingTalkAITableRequestTimeout},
	}, nil
}

// Validate validates DingTalk AI Table settings and credentials.
func (c *DingTalkAITableConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("DingTalk AI Table connector is nil")
	}
	if c.tableID == "" {
		return &ConnectorValidationError{Message: "DingTalk AI Table table_id is required"}
	}
	if c.operatorID == "" {
		return &ConnectorValidationError{Message: "DingTalk AI Table operator_id is required"}
	}
	if c.accessToken == "" {
		return &ConnectorMissingCredentialError{Message: "DingTalk access_token is required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	if err := validateDingTalkAITableAPIBaseURL(c.apiBaseURL); err != nil {
		return err
	}
	if _, err := c.loadSheets(ctx); err != nil {
		return &ConnectorValidationError{Message: fmt.Sprintf("DingTalk Notable credential validation failed: %v", err)}
	}
	return nil
}

// ValidateConnectorSetting validates DingTalk AI Table settings from an
// unsaved config.
func (c *DingTalkAITableConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	candidate, err := NewDingTalkAITableConnector(request)
	if err != nil {
		return err
	}
	return candidate.Validate(ctx)
}

// OpenSync opens one DingTalk AI Table sync session.
func (c *DingTalkAITableConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	documents, err := c.collectDocuments(ctx, request)
	if err != nil {
		return nil, err
	}
	session := &dingTalkAITableSyncSession{documents: documents, batchSize: c.batchSize}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune opens one complete DingTalk AI Table prune snapshot session.
func (c *DingTalkAITableConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	if err := c.validateStatic(); err != nil {
		return nil, err
	}
	documents, err := c.collectSlimDocuments(ctx)
	if err != nil {
		return nil, err
	}
	return &dingTalkAITablePruneSession{documents: documents, batchSize: c.batchSize}, nil
}

func (c *DingTalkAITableConnector) validateStatic() error {
	if c == nil {
		return fmt.Errorf("DingTalk AI Table connector is nil")
	}
	if c.tableID == "" {
		return &ConnectorValidationError{Message: "DingTalk AI Table table_id is required"}
	}
	if c.operatorID == "" {
		return &ConnectorValidationError{Message: "DingTalk AI Table operator_id is required"}
	}
	if c.accessToken == "" {
		return &ConnectorMissingCredentialError{Message: "DingTalk access_token is required"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	if err := validateDingTalkAITableAPIBaseURL(c.apiBaseURL); err != nil {
		return err
	}
	return nil
}

func (c *DingTalkAITableConnector) collectDocuments(ctx context.Context, request SyncRequest) ([]SourceDocument, error) {
	sheets, err := c.loadSheets(ctx)
	if err != nil {
		return nil, err
	}
	documents := []SourceDocument{}
	for _, sheet := range sheets {
		records, err := c.loadRecords(ctx, sheet.ID)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			if record.ID == "" {
				continue
			}
			doc, hasUpdatedAt := c.recordDocument(sheet, record)
			if !hasUpdatedAt && dingTalkAITableRequiresTimestamp(request) {
				continue
			}
			if hasUpdatedAt && !dingTalkAITableInWindow(doc.UpdatedAt, request) {
				continue
			}
			documents = append(documents, doc)
		}
	}
	sort.SliceStable(documents, func(i, j int) bool {
		return documents[i].SourceID < documents[j].SourceID
	})
	return documents, nil
}

func (c *DingTalkAITableConnector) collectSlimDocuments(ctx context.Context) ([]SlimDocument, error) {
	sheets, err := c.loadSheets(ctx)
	if err != nil {
		return nil, err
	}
	documents := []SlimDocument{}
	for _, sheet := range sheets {
		nextToken := ""
		for {
			records, token, err := c.loadRecordPage(ctx, sheet.ID, nextToken, 100)
			if err != nil {
				return nil, err
			}
			for _, record := range records {
				if record.ID != "" {
					documents = append(documents, SlimDocument{SourceID: c.documentID(sheet.ID, record.ID)})
				}
			}
			if token == "" {
				break
			}
			nextToken = token
		}
	}
	sort.SliceStable(documents, func(i, j int) bool {
		return documents[i].SourceID < documents[j].SourceID
	})
	return documents, nil
}

func (c *DingTalkAITableConnector) loadSheets(ctx context.Context) ([]dingTalkAITableSheet, error) {
	if c.getSheets != nil {
		return c.getSheets(ctx)
	}
	values := url.Values{}
	values.Set("operatorId", c.operatorID)
	var response dingTalkAITableSheetsResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1.0/notable/bases/%s/sheets?%s", url.PathEscape(c.tableID), values.Encode()), nil, &response); err != nil {
		return nil, err
	}
	sheets := make([]dingTalkAITableSheet, 0, len(response.Value))
	for _, sheet := range response.Value {
		if sheet.ID == "" {
			continue
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

func (c *DingTalkAITableConnector) loadRecords(ctx context.Context, sheetID string) ([]dingTalkAITableRecord, error) {
	records := []dingTalkAITableRecord{}
	nextToken := ""
	for {
		page, token, err := c.loadRecordPage(ctx, sheetID, nextToken, 100)
		if err != nil {
			return nil, err
		}
		records = append(records, page...)
		if token == "" {
			break
		}
		nextToken = token
	}
	return records, nil
}

func (c *DingTalkAITableConnector) loadRecordPage(ctx context.Context, sheetID, nextToken string, maxResults int) ([]dingTalkAITableRecord, string, error) {
	if c.listRecords != nil {
		return c.listRecords(ctx, sheetID, nextToken, maxResults)
	}
	values := url.Values{}
	values.Set("operatorId", c.operatorID)
	body := map[string]any{"maxResults": maxResults}
	if nextToken != "" {
		body["nextToken"] = nextToken
	}
	path := fmt.Sprintf("/v1.0/notable/bases/%s/sheets/%s/records/list?%s", url.PathEscape(c.tableID), url.PathEscape(sheetID), values.Encode())
	var response dingTalkAITableRecordsResponse
	if err := c.doJSON(ctx, http.MethodPost, path, body, &response); err != nil {
		return nil, "", err
	}
	return response.Records, response.NextToken, nil
}

func (c *DingTalkAITableConnector) doJSON(ctx context.Context, method, path string, body any, out any) error {
	if err := validateDingTalkAITableAPIBaseURL(c.apiBaseURL); err != nil {
		return err
	}
	target := c.apiBaseURL + path
	parsed, err := url.Parse(target)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Host != "api.dingtalk.com" {
		return &ConnectorValidationError{Message: "DingTalk AI Table requests must target https://api.dingtalk.com"}
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	req.Header.Set("x-acs-dingtalk-access-token", c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DingTalk API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if len(data) == 0 || out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode DingTalk API response: %w", err)
	}
	return nil
}

func validateDingTalkAITableAPIBaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "api.dingtalk.com" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return &ConnectorValidationError{Message: "DingTalk AI Table API base URL must be https://api.dingtalk.com"}
	}
	return nil
}

func (c *DingTalkAITableConnector) recordDocument(sheet dingTalkAITableSheet, record dingTalkAITableRecord) (SourceDocument, bool) {
	blob, err := json.MarshalIndent(record.Fields, "", "  ")
	if err != nil {
		blob = []byte("{}")
	}
	updatedAt, hasUpdatedAt := parseDingTalkAITableLastModifiedTime(record.LastModifiedTime)
	semanticIdentifier := fmt.Sprintf("%s - Record %s", sheet.Name, record.ID)
	fieldNames := make([]string, 0, len(record.Fields))
	for name := range record.Fields {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)
	for _, name := range fieldNames {
		value := record.Fields[name]
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" && len([]rune(text)) < 100 {
				semanticIdentifier = fmt.Sprintf("%s - %s", sheet.Name, truncateRunes(text, 50))
				break
			}
		}
	}
	metadata := map[string]any{
		"table_id":   c.tableID,
		"sheet_id":   sheet.ID,
		"sheet_name": sheet.Name,
		"record_id":  record.ID,
	}
	return SourceDocument{
		SourceID:           c.documentID(sheet.ID, record.ID),
		SemanticIdentifier: semanticIdentifier,
		Extension:          ".json",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        dingTalkAITableFingerprint(blob),
	}, hasUpdatedAt
}

func (c *DingTalkAITableConnector) documentID(sheetID, recordID string) string {
	return dingTalkAITableDocIDPrefix + c.tableID + ":" + sheetID + ":" + recordID
}

func dingTalkAITableInWindow(updatedAt time.Time, request SyncRequest) bool {
	if !request.FromBeginning && request.WindowStart != nil && updatedAt.Before(*request.WindowStart) {
		return false
	}
	if !request.WindowEnd.IsZero() && updatedAt.After(request.WindowEnd) {
		return false
	}
	return true
}

func dingTalkAITableRequiresTimestamp(request SyncRequest) bool {
	return !request.FromBeginning && request.WindowStart != nil
}

func parseDingTalkAITableLastModifiedTime(value any) (time.Time, bool) {
	var millis int64
	switch typed := value.(type) {
	case int:
		millis = int64(typed)
	case int64:
		millis = typed
	case float64:
		millis = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return time.Time{}, false
		}
		millis = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return time.Time{}, false
		}
		millis = parsed
	default:
		return time.Time{}, false
	}
	if millis <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, millis*int64(time.Millisecond)).UTC(), true
}

func dingTalkAITableFingerprint(blob []byte) string {
	sum := xxh3.Hash128(blob).Bytes()
	return hex.EncodeToString(sum[:])
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

type dingTalkAITableSheet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type dingTalkAITableRecord struct {
	ID               string         `json:"id"`
	Fields           map[string]any `json:"fields"`
	LastModifiedTime any            `json:"lastModifiedTime"`
}

type dingTalkAITableSheetsResponse struct {
	Value []dingTalkAITableSheet `json:"value"`
}

type dingTalkAITableRecordsResponse struct {
	Records   []dingTalkAITableRecord `json:"records"`
	NextToken string                  `json:"nextToken"`
}

type dingTalkAITableSyncSession struct {
	documents []SourceDocument
	batchSize int
	index     int
}

func (s *dingTalkAITableSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if s.index >= len(s.documents) {
		return SyncBatch{}, io.EOF
	}
	end := s.index + s.batchSize
	if end > len(s.documents) {
		end = len(s.documents)
	}
	documents := s.documents[s.index:end]
	batch := SyncBatch{Documents: documents, Checkpoint: dingTalkAITableCheckpoint(documents[len(documents)-1])}
	s.index = end
	return batch, nil
}

func (s *dingTalkAITableSyncSession) Close() error {
	return nil
}

func (s *dingTalkAITableSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	sourceID := firstNonEmpty(checkpoint.SourceID, checkpoint.Cursor)
	if sourceID == "" {
		return fmt.Errorf("dingtalk AI table sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	for index, doc := range s.documents {
		if doc.SourceID == sourceID {
			s.index = index + 1
			return nil
		}
	}
	return fmt.Errorf("dingtalk AI table resume anchor %q was not found in the current listing: %w", sourceID, ErrSyncResumeInvalid)
}

func dingTalkAITableCheckpoint(doc SourceDocument) *SyncCheckpoint {
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{Cursor: doc.SourceID, SourceID: doc.SourceID, UpdatedAt: &updatedAt}
}

type dingTalkAITablePruneSession struct {
	documents []SlimDocument
	batchSize int
	index     int
}

func (s *dingTalkAITablePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
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

func (s *dingTalkAITablePruneSession) Close() error {
	return nil
}
