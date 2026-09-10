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
	if err := c.getJSON(ctx, "/sobjects", &payload); err != nil {
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
	tmp, err := NewSalesforceConnector(request)
	if err != nil {
		return err
	}
	// Carry the receiver's transport/acquire stubs so tests can validate an
	// unsaved request without touching the network; production leaves them unset.
	tmp.acquireAccessToken = c.acquireAccessToken
	tmp.doJSON = c.doJSON
	return tmp.Validate(ctx)
}

// OpenSync opens one Salesforce sync session.
func (c *SalesforceConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	session := &salesforceSyncSession{
		connector:   c,
		objects:     c.objects,
		batchSize:   c.effectiveBatchSize(),
		windowStart: request.WindowStart,
		windowEnd:   request.WindowEnd,
		cursors:     map[string]salesforceObjectCursor{},
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
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

// instanceBaseURL returns the current canonical instance URL under the auth
// lock. It is only safe to read through this accessor once requests share the
// auth lock with token acquisition.
func (c *SalesforceConnector) instanceBaseURL() string {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	return c.instanceURL
}

// token returns a synchronized authentication snapshot: the access token, its
// expiry, and the canonical instance URL to build request targets from. The
// snapshot is captured atomically so callers never read or write instanceURL
// without the auth lock, and API URLs can be constructed from a single
// consistent view of the token exchange.
func (c *SalesforceConnector) token(ctx context.Context) (salesforceToken, error) {
	c.clientMu.Lock()
	if c.accessToken != "" && !c.cachedTokenExpiredLocked() {
		snap := salesforceToken{
			AccessToken: c.accessToken,
			InstanceURL: c.instanceURL,
			ExpiresAt:   c.tokenExpiry,
		}
		c.clientMu.Unlock()
		return snap, nil
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
		return salesforceToken{}, err
	}
	if cached.AccessToken == "" {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: "Salesforce token response did not contain access_token"}
	}
	c.clientMu.Lock()
	c.accessToken = cached.AccessToken
	c.tokenExpiry = cached.ExpiresAt
	if cached.InstanceURL != "" {
		c.instanceURL = strings.TrimRight(cached.InstanceURL, "/")
	}
	snap := salesforceToken{
		AccessToken: cached.AccessToken,
		InstanceURL: c.instanceURL,
		ExpiresAt:   cached.ExpiresAt,
	}
	c.clientMu.Unlock()
	return snap, nil
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

// salesforceHostAllowed reports whether a host is an approved Salesforce
// instance host. The configured and token-returned instance URLs must stay on
// Salesforce-owned domains so credentials are only ever transmitted to the
// intended provider.
func salesforceHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	return host == "salesforce.com" ||
		host == "force.com" ||
		strings.HasSuffix(host, ".salesforce.com") ||
		strings.HasSuffix(host, ".my.salesforce.com") ||
		strings.HasSuffix(host, ".force.com") ||
		strings.HasSuffix(host, ".lightning.force.com")
}

// requestAccessToken performs the OAuth2 client-credentials exchange, validating
// the token endpoint for SSRF, HTTPS, and the approved Salesforce host policy
// before any credentials are transmitted.
func (c *SalesforceConnector) requestAccessToken(ctx context.Context) (salesforceToken, error) {
	tokenURL := c.instanceBaseURL() + "/services/oauth2/token"
	hostname, resolvedIP, err := utility.AssertURLSafe(tokenURL)
	if err != nil {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Salesforce token request failed: %v", err)}
	}
	if !salesforceHostAllowed(hostname) {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: "Salesforce instance_url is not an approved Salesforce host"}
	}
	parsedURL, err := url.Parse(tokenURL)
	if err != nil || !strings.EqualFold(parsedURL.Scheme, "https") {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: "Salesforce OAuth token endpoint must use HTTPS"}
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}
	requestCtx, cancel := context.WithTimeout(ctx, salesforceRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return salesforceToken{}, &ConnectorMissingCredentialError{Message: fmt.Sprintf("Salesforce token request failed: %v", err)}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := utility.PinnedHTTPClient(hostname, resolvedIP, salesforceRequestTimeout)
	resp, err := client.Do(req)
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

// apiURL builds the full Salesforce REST URL for a service-relative path from
// the canonical instance URL captured in the authentication snapshot. The
// token exchange may publish a different canonical instance than the one the
// operator configured, so every request target is derived from the snapshot
// instead of stale connector state. Absolute pagination URLs returned by the
// query API are validated before use: they must use HTTPS on an approved
// Salesforce host, so a tampered URL can never receive the bearer token.
func (c *SalesforceConnector) apiURL(snap salesforceToken, path string) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		parsed, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("Salesforce pagination URL is invalid: %w", err)
		}
		if !strings.EqualFold(parsed.Scheme, "https") || !salesforceHostAllowed(parsed.Hostname()) {
			return "", fmt.Errorf("Salesforce pagination URL must use HTTPS on an approved Salesforce host")
		}
		return path, nil
	}
	if strings.HasPrefix(path, "/services/data/") {
		return snap.InstanceURL + path, nil
	}
	return snap.InstanceURL + "/services/data/" + c.apiVersion + path, nil
}

// getJSON GETs a Salesforce REST endpoint and decodes JSON into out. The apiURL
// is built only after token acquisition, from the canonical instance URL in the
// returned snapshot, and a 401 retry rebuilds it from the refreshed snapshot.
func (c *SalesforceConnector) getJSON(ctx context.Context, path string, out any) error {
	snap, err := c.token(ctx)
	if err != nil {
		return err
	}
	apiURL, err := c.apiURL(snap, path)
	if err != nil {
		return err
	}
	if c.doJSON != nil {
		return c.doJSON(ctx, apiURL, out)
	}
	for attempt := 0; ; attempt++ {
		status, body, err := c.doGet(ctx, apiURL, snap.AccessToken)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized && attempt == 0 {
			c.invalidateToken(snap.AccessToken)
			snap, err = c.token(ctx)
			if err != nil {
				return err
			}
			apiURL, err = c.apiURL(snap, path)
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
	parsed, err := url.Parse(apiURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || !salesforceHostAllowed(parsed.Hostname()) {
		return 0, nil, fmt.Errorf("Salesforce request URL must use HTTPS on an approved Salesforce host")
	}
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
// absent SObject as opposed to a transient, permission, routing, or API-version
// failure. A 404 is only treated as object-not-found when the response carries
// the structured Salesforce NOT_FOUND error; a bare 404 (e.g. a bad pagination
// URL or unknown route) must propagate as a normal HTTP error.
func salesforceObjectUnavailable(status int, body []byte) bool {
	if status == http.StatusNotFound {
		return salesforceNotFoundError(body)
	}
	if status != http.StatusBadRequest {
		return false
	}
	return salesforceInvalidTypeError(body)
}

// salesforceNotFoundError reports whether a response body carries the
// structured Salesforce "object not found" error code.
func salesforceNotFoundError(body []byte) bool {
	var entries []struct {
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(body, &entries); err == nil {
		for _, entry := range entries {
			if entry.ErrorCode == "NOT_FOUND" {
				return true
			}
		}
		return false
	}
	var single struct {
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(body, &single); err == nil {
		return single.ErrorCode == "NOT_FOUND"
	}
	return false
}

// salesforceInvalidTypeError reports whether a 400 response body carries the
// Salesforce INVALID_TYPE error, which describes an unknowable SOQL object.
func salesforceInvalidTypeError(body []byte) bool {
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
	if err := c.getJSON(ctx, "/sobjects/"+url.PathEscape(obj)+"/describe", &payload); err != nil {
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

// queryURL builds the SOQL query URL path for one SObject page. When a resume
// cursor is present, the WHERE clause re-fetches records strictly newer than the
// checkpoint plus same-instant records whose Id sorts after the checkpoint, so
// later records sharing the checkpoint timestamp are not skipped.
func (c *SalesforceConnector) queryURL(obj string, fields []string, cursor *salesforceObjectCursor, until *time.Time) string {
	fieldList := strings.Join(fields, ",")
	filters := []string{}
	if cursor != nil && cursor.SystemModstamp != "" {
		since := cursor.SystemModstamp
		if parsed, err := parseSalesforceTime(cursor.SystemModstamp); err == nil {
			since = salesforceSOQLTime(parsed)
		}
		clause := "SystemModstamp > " + since
		if cursor.Id != "" {
			clause = "(" + clause + " OR (SystemModstamp = " + since + " AND Id > '" + cursor.Id + "'))"
		}
		filters = append(filters, clause)
	}
	if until != nil {
		filters = append(filters, "SystemModstamp <= "+salesforceSOQLTime(*until))
	}
	where := ""
	if len(filters) > 0 {
		where = " WHERE " + strings.Join(filters, " AND ")
	}
	soql := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY SystemModstamp ASC, Id ASC", fieldList, obj, where)
	return "/query?q=" + url.QueryEscape(soql)
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

// salesforceObjectCursor is a per-object resume position keyed by the last
// ingested record. Storing both SystemModstamp and Id lets a resume re-fetch
// records that share the checkpoint timestamp but sort after it, so same-instant
// inserts are not lost across checkpoint boundaries.
type salesforceObjectCursor struct {
	SystemModstamp string `json:"system_modstamp,omitempty"`
	Id             string `json:"id,omitempty"`
}

// salesforceSyncCursor is the per-object composite cursor map.
type salesforceSyncCursor struct {
	Cursors map[string]salesforceObjectCursor `json:"cursors,omitempty"`
}

// salesforceSyncSession streams Salesforce documents for one fixed sync window.
type salesforceSyncSession struct {
	connector   *SalesforceConnector
	objects     []string
	batchSize   int
	windowStart *time.Time
	windowEnd   time.Time
	cursors     map[string]salesforceObjectCursor

	objectIndex int
	pageURL     string
	latestISO   string
	latestID    string
	buffer      []salesforceBufferedDocument

	resumeAnchor  *salesforceResumeAnchor
	resumeChecked bool
}

type salesforceBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
}

type salesforceResumeAnchor struct {
	sourceID     string
	object       string
	recordID     string
	objectCursor salesforceObjectCursor
}

// applyResume restores the per-object cursor map from a saved checkpoint.
// A checkpoint must carry a source anchor and a valid cursor for that object;
// remote anchor existence is checked by validateResume on the first NextBatch.
func (s *salesforceSyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil {
		return nil
	}
	if checkpoint.Cursor == "" {
		return fmt.Errorf("salesforce sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor salesforceSyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("salesforce sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	object, recordID, ok := salesforceSourceIDParts(checkpoint.SourceID)
	if !ok {
		return fmt.Errorf("salesforce sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	if !salesforceHasObject(s.objects, object) {
		return fmt.Errorf("salesforce resume anchor %q was not found in the current object listing: %w", checkpoint.SourceID, ErrSyncResumeInvalid)
	}
	if len(cursor.Cursors) == 0 {
		return fmt.Errorf("salesforce sync cursor has no object positions: %w", ErrSyncResumeInvalid)
	}
	for name, objectCursor := range cursor.Cursors {
		if !salesforceHasObject(s.objects, name) {
			return fmt.Errorf("salesforce sync cursor references unknown object %q: %w", name, ErrSyncResumeInvalid)
		}
		if objectCursor.SystemModstamp == "" || objectCursor.Id == "" {
			return fmt.Errorf("salesforce sync cursor has an invalid position for object %q: %w", name, ErrSyncResumeInvalid)
		}
		if _, err := parseSalesforceTime(objectCursor.SystemModstamp); err != nil {
			return fmt.Errorf("salesforce sync cursor has an invalid timestamp for object %q: %w", name, ErrSyncResumeInvalid)
		}
	}
	objectCursor, ok := cursor.Cursors[object]
	if !ok {
		return fmt.Errorf("salesforce sync cursor has no position for object %q: %w", object, ErrSyncResumeInvalid)
	}
	if objectCursor.Id != recordID {
		return fmt.Errorf("salesforce sync cursor does not match source anchor %q: %w", checkpoint.SourceID, ErrSyncResumeInvalid)
	}
	s.cursors = cursor.Cursors
	s.resumeAnchor = &salesforceResumeAnchor{
		sourceID:     checkpoint.SourceID,
		object:       object,
		recordID:     recordID,
		objectCursor: objectCursor,
	}
	return nil
}

// validateResume verifies the saved anchor still exists at the same position
// before the resumed session emits any documents.
func (s *salesforceSyncSession) validateResume(ctx context.Context) error {
	if s.resumeAnchor == nil || s.resumeChecked {
		return nil
	}
	s.resumeChecked = true
	anchor := s.resumeAnchor
	expectedModified, err := parseSalesforceTime(anchor.objectCursor.SystemModstamp)
	if err != nil {
		return fmt.Errorf("salesforce sync cursor has an invalid timestamp for object %q: %w", anchor.object, ErrSyncResumeInvalid)
	}
	if s.windowStart != nil && expectedModified.Before(*s.windowStart) {
		return fmt.Errorf("salesforce resume anchor %q is outside the sync window: %w", anchor.sourceID, ErrSyncResumeInvalid)
	}
	if !s.windowEnd.IsZero() && expectedModified.After(s.windowEnd) {
		return fmt.Errorf("salesforce resume anchor %q is outside the sync window: %w", anchor.sourceID, ErrSyncResumeInvalid)
	}

	soql := fmt.Sprintf("SELECT Id,SystemModstamp FROM %s WHERE Id = '%s'", anchor.object, salesforceDataLiteral(anchor.recordID))
	var page salesforceQueryPage
	if err := s.connector.getJSON(ctx, "/query?q="+url.QueryEscape(soql), &page); err != nil {
		var unavailable *salesforceObjectUnavailableError
		if errors.As(err, &unavailable) {
			return fmt.Errorf("salesforce resume object %q is no longer available: %w", anchor.object, ErrSyncResumeInvalid)
		}
		return err
	}
	for _, record := range page.Records {
		if stringRecordValue(record, "Id") != anchor.recordID {
			continue
		}
		modified, err := parseSalesforceTime(stringRecordValue(record, "SystemModstamp"))
		if err != nil || !modified.Equal(expectedModified) {
			return fmt.Errorf("salesforce resume anchor %q is no longer at the saved position: %w", anchor.sourceID, ErrSyncResumeInvalid)
		}
		return nil
	}
	return fmt.Errorf("salesforce resume anchor %q was not found in the current source: %w", anchor.sourceID, ErrSyncResumeInvalid)
}

// salesforceSourceIDParts splits the document SourceID anchor into an SObject
// name and record ID. Salesforce IDs do not contain slashes, so a SourceID with
// any extra separator or invalid character is not a usable resume anchor.
func salesforceSourceIDParts(sourceID string) (string, string, bool) {
	object, recordID, ok := strings.Cut(sourceID, "/")
	if !ok || !salesforceObjectNameValid(object) || !salesforceRecordIDValid(recordID) {
		return "", "", false
	}
	return object, recordID, true
}

func salesforceObjectNameValid(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

func salesforceRecordIDValid(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func salesforceDataLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}

func salesforceHasObject(objects []string, name string) bool {
	for _, object := range objects {
		if object == name {
			return true
		}
	}
	return false
}

// NextBatch returns the next Salesforce document batch.
func (s *salesforceSyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	if err := s.validateResume(ctx); err != nil {
		return SyncBatch{}, err
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
			cursor := s.objCursor(obj)
			until := salesforceWindowEnd(s.windowEnd)
			s.pageURL = s.connector.queryURL(obj, fields, cursor, until)
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
				s.latestID = recID
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
		finalID := s.latestID
		if page.NextRecordsURL != "" {
			s.pageURL = page.NextRecordsURL
		} else {
			s.objectIndex++
			s.pageURL = ""
			s.latestISO = ""
			s.latestID = ""
		}
		documents := make([]salesforceBufferedDocument, 0, len(raw))
		for index, doc := range raw {
			if done && index == len(raw)-1 && finalISO != "" {
				s.cursors[obj] = salesforceObjectCursor{SystemModstamp: finalISO, Id: finalID}
			}
			documents = append(documents, salesforceBufferedDocument{
				document:   doc,
				checkpoint: s.syncCheckpoint(doc),
			})
		}
		if done && len(raw) == 0 && finalISO != "" {
			// Empty final page: still commit the drained object's cursor so
			// the next object's batches skip it on resume.
			s.cursors[obj] = salesforceObjectCursor{SystemModstamp: finalISO, Id: finalID}
		}
		if len(documents) > 0 {
			return documents, nil
		}
	}
	return nil, nil
}

// objCursor computes the per-object resume cursor: the caller window or the
// persisted cursor, whichever is later. A window bound that falls after the
// persisted cursor drops the cursor's Id so same-instant records from the
// window are not skipped; otherwise the composite timestamp+Id cursor is kept.
func (s *salesforceSyncSession) objCursor(obj string) *salesforceObjectCursor {
	cur, ok := s.cursors[obj]
	if !ok || cur.SystemModstamp == "" {
		if s.windowStart != nil {
			return &salesforceObjectCursor{SystemModstamp: salesforceSOQLTime(*s.windowStart)}
		}
		return nil
	}
	cursorTime, err := parseSalesforceTime(cur.SystemModstamp)
	if err != nil {
		if s.windowStart != nil {
			return &salesforceObjectCursor{SystemModstamp: salesforceSOQLTime(*s.windowStart)}
		}
		return &cur
	}
	if s.windowStart != nil && cursorTime.Before(*s.windowStart) {
		return &salesforceObjectCursor{SystemModstamp: salesforceSOQLTime(*s.windowStart)}
	}
	return &cur
}

// syncCheckpoint serializes the current per-object cursor map.
func (s *salesforceSyncSession) syncCheckpoint(doc SourceDocument) *SyncCheckpoint {
	cursors := make(map[string]salesforceObjectCursor, len(s.cursors))
	for obj, cur := range s.cursors {
		cursors[obj] = cur
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
			"web_url":   fmt.Sprintf("%s/%s", c.instanceBaseURL(), recID),
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
			s.pageURL = page.NextRecordsURL
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
