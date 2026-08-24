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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/utility"
)

const (
	defaultSalesforceAPIVersion = "v59.0"
	defaultSalesforceBatchSize  = 2
	salesforceRequestTimeout    = 60 * time.Second
	salesforceTokenExpiryMargin = 5 * time.Minute
)

// Default Salesforce CRM objects indexed when the config does not list any.
// Knowledge__kav is optional: orgs without Salesforce Knowledge silently skip it.
var salesforceDefaultObjects = []string{"Account", "Contact", "Opportunity", "Case", "Knowledge__kav"}

// salesforceOptionalObjects are silently skipped when absent from the org.
var salesforceOptionalObjects = map[string]bool{"Knowledge__kav": true}

// salesforceObjectUnavailableError reports that an SObject is genuinely
// absent or not queryable (HTTP 404, or 400 INVALID_TYPE) rather than a
// transient/permission failure. Callers skip such objects without failing
// the whole run; 403/5xx/429 must still abort.
type salesforceObjectUnavailableError struct {
	message string
}

func (e *salesforceObjectUnavailableError) Error() string { return e.message }

// salesforceHTTPError carries a non-2xx Salesforce REST response.
type salesforceHTTPError struct {
	status int
	body   string
}

func (e *salesforceHTTPError) Error() string {
	return fmt.Sprintf("Salesforce API returned HTTP %d: %s", e.status, e.body)
}

// salesforceToken is a cached OAuth2 client-credentials token.
type salesforceToken struct {
	AccessToken string
	InstanceURL string
	ExpiresAt   time.Time
}

// SalesforceConnector reads Salesforce CRM records through the REST + SOQL
// APIs. It authenticates with OAuth2 client-credentials against a Connected
// App, describes the configured SObjects, and uses SystemModstamp as the
// incremental cursor so re-syncs only fetch what changed.
type SalesforceConnector struct {
	instanceURL  string
	clientID     string
	clientSecret string
	objects      []string
	apiVersion   string
	batchSize    int

	clientMu    sync.Mutex
	accessToken string
	tokenExpiry time.Time
	httpClient  *http.Client
	now         func() time.Time

	acquireAccessToken func(ctx context.Context) (salesforceToken, error)
	doJSON             func(ctx context.Context, apiURL string, out any) error
}

// NewSalesforceConnector creates a Salesforce connector from config.
func NewSalesforceConnector(config map[string]any) (*SalesforceConnector, error) {
	credentials, _ := config["credentials"].(map[string]any)
	instanceURL := strings.TrimRight(strings.TrimSpace(stringConfig(credentials["instance_url"])), "/")
	objects := salesforceObjects(config["objects"])
	return &SalesforceConnector{
		instanceURL:  instanceURL,
		clientID:     strings.TrimSpace(stringConfig(credentials["client_id"])),
		clientSecret: stringConfig(credentials["client_secret"]),
		objects:      objects,
		apiVersion:   firstNonEmpty(stringConfig(config["api_version"]), defaultSalesforceAPIVersion),
		batchSize:    salesforceBatchSize(config["batch_size"]),
		httpClient:   http.DefaultClient,
		now:          time.Now,
	}, nil
}

// salesforceObjects normalizes a comma string or JSON list of object names.
func salesforceObjects(value any) []string {
	switch typed := value.(type) {
	case string:
		out := []string{}
		for _, part := range strings.Split(typed, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return out
		}
	case []any:
		out := []string{}
		for _, item := range typed {
			if part := strings.TrimSpace(fmt.Sprint(item)); part != "" {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return append([]string(nil), salesforceDefaultObjects...)
}

// salesforceBatchSize preserves explicit non-positive values so validation can
// reject them; only missing/unparseable values fall back to the default.
func salesforceBatchSize(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if v, err := typed.Int64(); err == nil {
			return int(v)
		}
	case string:
		if v, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return v
		}
	}
	return defaultSalesforceBatchSize
}

// Validate validates Salesforce connector settings and credentials.
func (c *SalesforceConnector) Validate(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("salesforce connector is nil")
	}
	if c.instanceURL == "" || c.clientID == "" || c.clientSecret == "" {
		return &ConnectorMissingCredentialError{Message: "Salesforce credentials are incomplete (instance_url, client_id, client_secret required)"}
	}
	if c.batchSize <= 0 {
		return &ConnectorValidationError{Message: "batch_size must be a positive integer"}
	}
	if _, err := c.token(ctx); err != nil {
		return err
	}

	var payload salesforceSObjectsResponse
	if err := c.getJSON(ctx, c.base()+"/sobjects", &payload); err != nil {
		var httpErr *salesforceHTTPError
		if errors.As(err, &httpErr) {
			switch httpErr.status {
			case http.StatusUnauthorized:
				return &ConnectorMissingCredentialError{Message: "Salesforce access token is invalid or expired."}
			case http.StatusForbidden:
				return &ConnectorValidationError{Message: "The Salesforce execution user lacks API access; enable the 'API Enabled' profile permission."}
			default:
				return &ConnectorValidationError{Message: fmt.Sprintf("Salesforce validation failed (HTTP %d): %s", httpErr.status, httpErr.body)}
			}
		}
		return err
	}

	queryable := map[string]bool{}
	for _, so := range payload.SObjects {
		if so.Name != "" {
			queryable[so.Name] = so.Queryable
		}
	}
	unknown := []string{}
	notQueryable := []string{}
	for _, obj := range c.objects {
		queryableFlag, ok := queryable[obj]
		if !ok {
			if salesforceOptionalObjects[obj] {
				continue
			}
			unknown = append(unknown, obj)
		} else if !queryableFlag {
			notQueryable = append(notQueryable, obj)
		}
	}
	if len(unknown) > 0 || len(notQueryable) > 0 {
		problems := []string{}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			problems = append(problems, fmt.Sprintf("unknown object(s): %s", strings.Join(unknown, ", ")))
		}
		if len(notQueryable) > 0 {
			sort.Strings(notQueryable)
			problems = append(problems, fmt.Sprintf("non-queryable object(s): %s", strings.Join(notQueryable, ", ")))
		}
		return &ConnectorValidationError{Message: "Salesforce 'objects' configuration is invalid — " + strings.Join(problems, "; ") + ". Check for typos and that the execution user has read access to each object."}
	}
	return nil
}

// ValidateConnectorSetting validates Salesforce settings from an unsaved config.
func (c *SalesforceConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.Validate(ctx)
}

// OpenSync opens one Salesforce sync session.
func (c *SalesforceConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	session := &salesforceSyncSession{
		connector:   c,
		objects:     c.objects,
		batchSize:   c.effectiveBatchSize(),
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
		cursors:     map[string]string{},
	}
	session.applyResume(request.Resume)
	return session, nil
}

// OpenPrune opens one complete Salesforce prune snapshot session.
func (c *SalesforceConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	return &salesforcePruneSession{
		connector: c,
		objects:   c.objects,
		batchSize: c.effectiveBatchSize(),
	}, nil
}

func (c *SalesforceConnector) effectiveBatchSize() int {
	if c.batchSize > 0 {
		return c.batchSize
	}
	return defaultSalesforceBatchSize
}

// base returns the Salesforce REST API base URL.
func (c *SalesforceConnector) base() string {
	return c.instanceURL + "/services/data/" + c.apiVersion
}

// token returns a cached or freshly acquired access token, preferring the
// canonical instance URL returned by the token endpoint.
func (c *SalesforceConnector) token(ctx context.Context) (string, error) {
	c.clientMu.Lock()
	if c.accessToken != "" && !c.cachedTokenExpiredLocked() {
		token := c.accessToken
		c.clientMu.Unlock()
		return token, nil
	}
	c.clientMu.Unlock()

	var cached salesforceToken
	var err error
	if c.acquireAccessToken != nil {
		cached, err = c.acquireAccessToken(ctx)
	} else {
		cached, err = c.requestAccessToken(ctx)
	}
	if err != nil {
		return "", err
	}
	if cached.AccessToken == "" {
		return "", &ConnectorMissingCredentialError{Message: "Salesforce token response did not contain access_token"}
	}
	if cached.InstanceURL != "" {
		c.instanceURL = strings.TrimRight(cached.InstanceURL, "/")
	}
	c.clientMu.Lock()
	c.accessToken = cached.AccessToken
	c.tokenExpiry = cached.ExpiresAt
	c.clientMu.Unlock()
	return cached.AccessToken, nil
}

func (c *SalesforceConnector) cachedTokenExpiredLocked() bool {
	return c.tokenExpiry.IsZero() || !c.currentTime().Add(salesforceTokenExpiryMargin).Before(c.tokenExpiry)
}

func (c *SalesforceConnector) invalidateToken(token string) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiry = time.Time{}
	}
}

func (c *SalesforceConnector) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// requestAccessToken performs the OAuth2 client-credentials exchange.
func (c *SalesforceConnector) requestAccessToken(ctx context.Context) (salesforceToken, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	requestCtx, cancel := context.WithTimeout(ctx, salesforceRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.instanceURL+"/services/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Salesforce token request failed: %v", err)}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Salesforce token request failed: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		detail := salesforceTokenErrorDetail(body)
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Failed to acquire Salesforce access token (HTTP %d): %s", resp.StatusCode, detail)}
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		InstanceURL string `json:"instance_url"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Salesforce token response is not JSON: %v", err)}
	}
	expiresAt := c.currentTime().Add(time.Hour)
	if payload.ExpiresIn > 0 {
		expiresAt = c.currentTime().Add(time.Duration(payload.ExpiresIn) * time.Second)
	}
	return salesforceToken{
		AccessToken: payload.AccessToken,
		InstanceURL: strings.TrimRight(payload.InstanceURL, "/"),
		ExpiresAt:   expiresAt,
	}, nil
}

func salesforceTokenErrorDetail(body []byte) string {
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.ErrorDescription != "" {
			return payload.ErrorDescription
		}
		if payload.Error != "" {
			return payload.Error
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

// getJSON GETs a Salesforce REST endpoint and decodes JSON into out.
func (c *SalesforceConnector) getJSON(ctx context.Context, apiURL string, out any) error {
	if c.doJSON != nil {
		return c.doJSON(ctx, apiURL, out)
	}
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		status, body, err := c.doGet(ctx, apiURL, token)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized && attempt == 0 {
			c.invalidateToken(token)
			token, err = c.token(ctx)
			if err != nil {
				return err
			}
			continue
		}
		if salesforceObjectUnavailable(status, body) {
			return &salesforceObjectUnavailableError{message: fmt.Sprintf("Salesforce object unavailable (HTTP %d): %s", status, strings.TrimSpace(string(body)))}
		}
		if status >= 400 {
			return &salesforceHTTPError{status: status, body: strings.TrimSpace(string(body))}
		}
		return json.Unmarshal(body, out)
	}
}

// doGet performs one authenticated GET with SSRF protection.
func (c *SalesforceConnector) doGet(ctx context.Context, apiURL, token string) (int, []byte, error) {
	hostname, resolvedIP, err := utility.AssertURLSafe(apiURL)
	if err != nil {
		return 0, nil, err
	}
	client := utility.PinnedHTTPClient(hostname, resolvedIP, salesforceRequestTimeout)
	requestCtx, cancel := context.WithTimeout(ctx, salesforceRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

// salesforceObjectUnavailable reports whether a response indicates a genuinely
// absent SObject (404, or 400 INVALID_TYPE) as opposed to a transient or
// permission failure.
func salesforceObjectUnavailable(status int, body []byte) bool {
	if status == http.StatusNotFound {
		return true
	}
	if status != http.StatusBadRequest {
		return false
	}
	var entries []struct {
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(body, &entries); err == nil {
		for _, entry := range entries {
			if entry.ErrorCode == "INVALID_TYPE" {
				return true
			}
		}
		return false
	}
	var single struct {
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(body, &single); err == nil {
		return single.ErrorCode == "INVALID_TYPE"
	}
	return strings.Contains(string(body), "INVALID_TYPE")
}

// describeFields returns field API names for an SObject, filtering out
// compound types (address, location) that SOQL cannot project directly.
func (c *SalesforceConnector) describeFields(ctx context.Context, obj string) ([]string, error) {
	var payload struct {
		Fields []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fields"`
	}
	if err := c.getJSON(ctx, c.base()+"/sobjects/"+url.PathEscape(obj)+"/describe", &payload); err != nil {
		return nil, err
	}
	fields := []string{}
	for _, field := range payload.Fields {
		if field.Type == "address" || field.Type == "location" {
			continue
		}
		if field.Name != "" {
			fields = append(fields, field.Name)
		}
	}
	hasID := false
	for _, name := range fields {
		if name == "Id" {
			hasID = true
			break
		}
	}
	if !hasID {
		fields = append([]string{"Id"}, fields...)
	}
	return fields, nil
}

// queryURL builds the SOQL query URL for one SObject page.
func (c *SalesforceConnector) queryURL(obj string, fields []string, since *time.Time, until *time.Time) string {
	fieldList := strings.Join(fields, ",")
	filters := []string{}
	if since != nil {
		filters = append(filters, "SystemModstamp > "+salesforceSOQLTime(*since))
	}
	if until != nil {
		filters = append(filters, "SystemModstamp <= "+salesforceSOQLTime(*until))
	}
	where := ""
	if len(filters) > 0 {
		where = " WHERE " + strings.Join(filters, " AND ")
	}
	soql := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY SystemModstamp ASC", fieldList, obj, where)
	return c.base() + "/query?q=" + url.QueryEscape(soql)
}

// salesforceSOQLTime formats a timestamp for a SOQL literal.
func salesforceSOQLTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// salesforceWindowEnd returns the inclusive upper window bound, or nil when
// the window end is unset (e.g. prune scans).
func salesforceWindowEnd(windowEnd time.Time) *time.Time {
	if windowEnd.IsZero() {
		return nil
	}
	until := windowEnd
	return &until
}

// parseSalesforceTime parses a Salesforce ISO-8601 timestamp.
func parseSalesforceTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty Salesforce timestamp")
	}
	layouts := []string{
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse Salesforce timestamp %q", value)
}

// salesforceSyncCursor is the per-object SystemModstamp cursor map.
type salesforceSyncCursor struct {
	Cursors map[string]string `json:"cursors,omitempty"`
}

// salesforceSyncSession streams Salesforce documents for one fixed sync window.
type salesforceSyncSession struct {
	connector   *SalesforceConnector
	objects     []string
	batchSize   int
	windowStart *time.Time
	windowEnd   time.Time
	cursors     map[string]string

	objectIndex int
	pageURL     string
	latestISO   string
	buffer      []salesforceBufferedDocument
}

type salesforceBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
}

// applyResume restores the per-object cursor map from a saved checkpoint.
func (s *salesforceSyncSession) applyResume(checkpoint *SyncCheckpoint) {
	if checkpoint == nil || checkpoint.Cursor == "" {
		return
	}
	var cursor salesforceSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return
	}
	if len(cursor.Cursors) > 0 {
		s.cursors = cursor.Cursors
	}
}

// NextBatch returns the next Salesforce document batch.
func (s *salesforceSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
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
		page, err := s.nextDocumentPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		if len(page) == 0 {
			if s.objectIndex >= len(s.objects) {
				if len(documents) == 0 {
					return SyncBatch{}, io.EOF
				}
				break
			}
			continue
		}
		remaining := s.batchSize - len(documents)
		if len(page) > remaining {
			for _, buffered := range page[:remaining] {
				documents = append(documents, buffered.document)
				checkpoint = buffered.checkpoint
			}
			s.buffer = append(s.buffer, page[remaining:]...)
			break
		}
		for _, buffered := range page {
			documents = append(documents, buffered.document)
			checkpoint = buffered.checkpoint
		}
	}
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close closes the Salesforce sync session.
func (s *salesforceSyncSession) Close() error {
	return nil
}

// nextDocumentPage streams the next batch of raw records for the current
// object, advancing to the next object when the current one is drained.
func (s *salesforceSyncSession) nextDocumentPage(ctx context.Context) ([]salesforceBufferedDocument, error) {
	for s.objectIndex < len(s.objects) {
		obj := s.objects[s.objectIndex]
		if s.pageURL == "" {
			fields, err := s.connector.describeFields(ctx, obj)
			if err != nil {
				var unavail *salesforceObjectUnavailableError
				if errors.As(err, &unavail) {
					s.objectIndex++
					continue
				}
				return nil, err
			}
			since := s.objSince(obj)
			until := salesforceWindowEnd(s.windowEnd)
			s.pageURL = s.connector.queryURL(obj, fields, since, until)
		}
		var page salesforceQueryPage
		if err := s.connector.getJSON(ctx, s.pageURL, &page); err != nil {
			var unavail *salesforceObjectUnavailableError
			if errors.As(err, &unavail) {
				// Object disappeared mid-query (e.g. Knowledge__kav org lacks
				// Knowledge). Skip it rather than abort the run.
				s.objectIndex++
				s.pageURL = ""
				continue
			}
			return nil, err
		}
		raw := make([]SourceDocument, 0, len(page.Records))
		for _, record := range page.Records {
			recID := stringRecordValue(record, "Id")
			if recID == "" {
				continue
			}
			modifiedStr := stringRecordValue(record, "SystemModstamp")
			if modifiedStr != "" {
				s.latestISO = modifiedStr
			}
			raw = append(raw, s.connector.recordToDocument(obj, record, recID, modifiedStr))
		}
		done := page.NextRecordsURL == ""
		// Capture the object's latest cursor before resetting session state.
		// The cursor only advances once the object is fully drained: the
		// object's final record checkpoint carries it, while earlier batches
		// keep the old cursor so a crash between them re-fetches the object
		// instead of skipping records that were never ingested.
		finalISO := s.latestISO
		if page.NextRecordsURL != "" {
			s.pageURL = s.connector.instanceURL + page.NextRecordsURL
		} else {
			s.objectIndex++
			s.pageURL = ""
			s.latestISO = ""
		}
		documents := make([]salesforceBufferedDocument, 0, len(raw))
		for index, doc := range raw {
			if done && index == len(raw)-1 && finalISO != "" {
				s.cursors[obj] = finalISO
			}
			documents = append(documents, salesforceBufferedDocument{
				document:   doc,
				checkpoint: s.syncCheckpoint(doc),
			})
		}
		if done && len(raw) == 0 && finalISO != "" {
			// Empty final page: still commit the drained object's cursor so
			// the next object's batches skip it on resume.
			s.cursors[obj] = finalISO
		}
		if len(documents) > 0 {
			return documents, nil
		}
	}
	return nil, nil
}

// objSince computes the per-object lower bound: the caller window or the
// persisted cursor, whichever is later.
func (s *salesforceSyncSession) objSince(obj string) *time.Time {
	var since *time.Time
	if s.windowStart != nil {
		since = s.windowStart
	}
	if iso := s.cursors[obj]; iso != "" {
		if t, err := parseSalesforceTime(iso); err == nil {
			if since == nil || t.After(*since) {
				since = &t
			}
		}
	}
	return since
}

// syncCheckpoint serializes the current per-object cursor map.
func (s *salesforceSyncSession) syncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	cursors := make(map[string]string, len(s.cursors))
	for obj, iso := range s.cursors {
		cursors[obj] = iso
	}
	data, err := json.Marshal(salesforceSyncCursor{Cursors: cursors})
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt
	return &SyncCheckpoint{
		Cursor:    string(data),
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

// recordToDocument converts a SOQL record into a SourceDocument.
func (c *SalesforceConnector) recordToDocument(obj string, record map[string]any, recID, modifiedStr string) SourceDocument {
	updatedAt := time.Now().UTC()
	if modifiedStr != "" {
		if t, err := parseSalesforceTime(modifiedStr); err == nil {
			updatedAt = t
		}
	}
	name := firstNonEmpty(
		stringRecordValue(record, "Name"),
		stringRecordValue(record, "Subject"),
		stringRecordValue(record, "Title"),
		fmt.Sprintf("%s/%s", obj, recID),
	)
	body := salesforceRecordToText(obj, record)
	blob := []byte(body)
	return SourceDocument{
		SourceID:           fmt.Sprintf("%s/%s", obj, recID),
		SemanticIdentifier: name,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata: map[string]any{
			"object":    obj,
			"record_id": recID,
			"web_url":   fmt.Sprintf("%s/%s", c.instanceURL, recID),
		},
		Fingerprint: contentFingerprint(blob),
	}
}

// salesforceRecordToText flattens a SOQL record into a deterministic
// plain-text body (sorted keys, stable value formatting) so content hashing is
// stable across polls.
func salesforceRecordToText(obj string, record map[string]any) string {
	lines := []string{"Salesforce " + obj}
	keys := make([]string, 0, len(record))
	for key := range record {
		if key == "attributes" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := record[key]
		if value == nil || value == "" {
			continue
		}
		if isMapOrSlice(value) {
			if data, err := json.Marshal(value); err == nil {
				lines = append(lines, fmt.Sprintf("%s: %s", key, string(data)))
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("%s: %v", key, value))
	}
	return strings.Join(lines, "\n")
}

func stringRecordValue(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func isMapOrSlice(value any) bool {
	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

// salesforceQueryPage is one SOQL query result page.
type salesforceQueryPage struct {
	Records        []map[string]any `json:"records"`
	NextRecordsURL string           `json:"nextRecordsUrl"`
}

// salesforceSObjectsResponse is the /sobjects global describe payload.
type salesforceSObjectsResponse struct {
	SObjects []struct {
		Name      string `json:"name"`
		Queryable bool   `json:"queryable"`
	} `json:"sobjects"`
}

// salesforcePruneSession streams a complete Salesforce slim snapshot.
type salesforcePruneSession struct {
	connector   *SalesforceConnector
	objects     []string
	batchSize   int
	objectIndex int
	pageURL     string
	buffer      []SlimDocument
}

// NextBatch returns the next Salesforce prune snapshot batch.
func (s *salesforcePruneSession) NextBatch(ctx context.Context) (PruneBatch, error) {
	documents := make([]SlimDocument, 0, s.batchSize)
	if len(s.buffer) > 0 {
		n := min(s.batchSize, len(s.buffer))
		documents = append(documents, s.buffer[:n]...)
		s.buffer = s.buffer[n:]
	}
	for len(documents) < s.batchSize {
		page, err := s.nextSlimPage(ctx)
		if err != nil {
			return PruneBatch{}, err
		}
		if len(page) == 0 {
			if s.objectIndex >= len(s.objects) {
				if len(documents) == 0 {
					return PruneBatch{}, io.EOF
				}
				break
			}
			continue
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

// Close closes the Salesforce prune session.
func (s *salesforcePruneSession) Close() error {
	return nil
}

// nextSlimPage streams the next page of slim IDs for the current object.
func (s *salesforcePruneSession) nextSlimPage(ctx context.Context) ([]SlimDocument, error) {
	for s.objectIndex < len(s.objects) {
		obj := s.objects[s.objectIndex]
		if s.pageURL == "" {
			s.pageURL = s.connector.queryURL(obj, []string{"Id"}, nil, nil)
		}
		var page salesforceQueryPage
		if err := s.connector.getJSON(ctx, s.pageURL, &page); err != nil {
			var unavail *salesforceObjectUnavailableError
			if errors.As(err, &unavail) {
				s.objectIndex++
				s.pageURL = ""
				continue
			}
			return nil, err
		}
		documents := make([]SlimDocument, 0, len(page.Records))
		for _, record := range page.Records {
			recID := stringRecordValue(record, "Id")
			if recID == "" {
				continue
			}
			documents = append(documents, SlimDocument{SourceID: fmt.Sprintf("%s/%s", obj, recID)})
		}
		if page.NextRecordsURL != "" {
			s.pageURL = s.connector.instanceURL + page.NextRecordsURL
		} else {
			s.objectIndex++
			s.pageURL = ""
		}
		if len(documents) > 0 {
			return documents, nil
		}
	}
	return nil, nil
}
