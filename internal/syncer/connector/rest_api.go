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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// RestAPIConnector is a configuration-driven REST API data source connector.
//
// It mirrors common/data_source/rest_api_connector.py: the same config schema,
// auth derivation, SSRF protection, pagination, JSON extraction and
// item-to-document mapping. The connector is read-only over HTTP(S).
type RestAPIConnector struct {
	cfg         *restAPIConfig
	baseURL     string
	urlParams   map[string]string
	credentials map[string]any

	authHeaders map[string]string
	basicAuth   *restAPIBasicAuth
	prepared    bool
}

// restAPIConfig is the validated connector configuration.
type restAPIConfig struct {
	URL                string
	Method             string
	Headers            map[string]string
	QueryParams        map[string]string
	AuthType           string
	AuthConfig         map[string]any
	ItemsPath          string
	IDField            string
	ContentFields      []string
	MetadataFields     []string
	PaginationType     string
	PaginationConfig   map[string]any
	PollTimestampField string
	RequestBody        map[string]any
	FieldTypeHints     map[string]string
	FieldDefaultValues map[string]any
	ContentTemplate    string
	BatchSize          int
	MaxPages           int
	RequestDelay       float64
}

// restAPIBasicAuth carries HTTP basic credentials.
type restAPIBasicAuth struct {
	username string
	password string
}

const (
	restAPIDefaultBatchSize    = 2    // default per-batch document count
	restAPIDefaultMaxPages     = 1000 // default page fetch cap
	restAPIDefaultRequestDelay = 0.5
	restAPIRequestTimeout      = 60 * time.Second
	restAPIMaxRedirects        = 5
)

const (
	restAPIAuthNone         = "none"
	restAPIAuthAPIKeyHeader = "api_key_header"
	restAPIAuthBearer       = "bearer"
	restAPIAuthBasic        = "basic"

	restAPIPaginationNone   = "none"
	restAPIPaginationPage   = "page"
	restAPIPaginationOffset = "offset"
	restAPIPaginationCursor = "cursor"
)

// Connector error types let callers (e.g. the connector /test endpoint) map
// failures to stable response codes.
type (
	// ConnectorValidationError mirrors ConnectorValidationError.
	ConnectorValidationError struct{ Message string }
	// ConnectorMissingCredentialError mirrors ConnectorMissingCredentialError.
	ConnectorMissingCredentialError struct{ Message string }
	// RateLimitTriedTooManyTimesError mirrors RateLimitTriedTooManyTimesError.
	RateLimitTriedTooManyTimesError struct{ Message string }
)

func (e *ConnectorValidationError) Error() string        { return e.Message }
func (e *ConnectorMissingCredentialError) Error() string { return e.Message }
func (e *RateLimitTriedTooManyTimesError) Error() string { return e.Message }

// ErrPruneUnsupported is returned by OpenPrune for connectors that do not
// support deleted-file pruning; prune tasks then complete without deleting
// anything.
var ErrPruneUnsupported = errors.New("rest API connector does not support prune")

// Retry/backoff knobs for page fetches and rate limiting. They are package
// variables so tests can shrink the delays.
var (
	restAPIRetryTries     = 5
	restAPIRetryBaseDelay = time.Second
	restAPIRetryMaxDelay  = 30 * time.Second
	restAPIRetryBackoff   = 2
	restAPIRetryJitter    = time.Second
	restAPI429MaxWaits    = 30
	restAPI429DefaultWait = 30 * time.Second
)

// restAPISSRFAllowLoopback is a test hook that lets unit tests exercise the
// real HTTP path against httptest servers bound to loopback. Production code
// keeps it false so loopback/private endpoints stay blocked.
var restAPISSRFAllowLoopback bool

// NewRestAPIConnector parses a connector config and returns a connector. It
// performs schema validation and the base-URL SSRF check but performs no
// network I/O. Credentials are read from config["credentials"].
func NewRestAPIConnector(config map[string]any) (*RestAPIConnector, error) {
	cfg, err := parseRestAPIConfig(config)
	if err != nil {
		return nil, err
	}
	if err := validateRestAPIURLForSSRF(cfg.URL); err != nil {
		return nil, err
	}

	parsed, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, &ConnectorValidationError{Message: "Invalid REST API config: url"}
	}

	// Keep the raw path so "{key}" template placeholders survive URL parsing
	// (url.URL.String() would percent-encode them).
	path := parsed.RawPath
	if path == "" {
		path = parsed.Path
	}
	c := &RestAPIConnector{
		cfg:         cfg,
		baseURL:     parsed.Scheme + "://" + parsed.Host + path,
		urlParams:   restAPIURLQueryParams(parsed),
		credentials: restAPICredentialsFromConfig(config),
		authHeaders: map[string]string{},
	}
	return c, nil
}

// restAPICredentialsFromConfig extracts and normalizes stored credentials.
func restAPICredentialsFromConfig(config map[string]any) map[string]any {
	raw, ok := config["credentials"]
	if !ok || raw == nil {
		return map[string]any{}
	}
	if creds, ok := raw.(map[string]any); ok {
		return creds
	}
	if text, ok := raw.(string); ok {
		var creds map[string]any
		if json.Unmarshal([]byte(text), &creds) == nil && creds != nil {
			return creds
		}
	}
	return map[string]any{}
}

// parseRestAPIConfig mirrors RestAPIConnectorConfig + parse_storage_config.
func parseRestAPIConfig(config map[string]any) (*restAPIConfig, error) {
	cfg := &restAPIConfig{
		Method:         "GET",
		AuthType:       restAPIAuthNone,
		PaginationType: restAPIPaginationNone,
		BatchSize:      restAPIDefaultBatchSize,
		MaxPages:       restAPIDefaultMaxPages,
		RequestDelay:   restAPIDefaultRequestDelay,
	}

	cfg.URL = strings.TrimSpace(stringConfig(config["url"]))
	if cfg.URL == "" {
		return nil, &ConnectorValidationError{Message: "Invalid REST API config: url\n  Field required"}
	}
	if method := strings.ToUpper(strings.TrimSpace(stringConfig(config["method"]))); method != "" {
		cfg.Method = method
	}
	if cfg.Method != "GET" && cfg.Method != "POST" {
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Unsupported HTTP method '%s'.", cfg.Method)}
	}
	cfg.Headers = restAPITextToDict(config["headers"])
	cfg.QueryParams = restAPITextToDict(config["query_params"])

	if auth := strings.TrimSpace(stringConfig(config["auth_type"])); auth != "" {
		cfg.AuthType = auth
	}
	switch cfg.AuthType {
	case restAPIAuthNone, restAPIAuthAPIKeyHeader, restAPIAuthBearer, restAPIAuthBasic:
	default:
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Unsupported auth_type '%s'.", cfg.AuthType)}
	}
	cfg.AuthConfig = configAnyMap(config["auth_config"])
	cfg.ItemsPath = strings.TrimSpace(stringConfig(config["items_path"]))
	cfg.IDField = strings.TrimSpace(stringConfig(config["id_field"]))
	cfg.ContentFields = restAPICoerceFieldList(config["content_fields"])
	if len(cfg.ContentFields) == 0 {
		return nil, &ConnectorValidationError{Message: "At least one content field must be configured (content_fields)."}
	}
	cfg.MetadataFields = restAPICoerceFieldList(config["metadata_fields"])

	if pagination := strings.TrimSpace(stringConfig(config["pagination_type"])); pagination != "" {
		cfg.PaginationType = pagination
	}
	switch cfg.PaginationType {
	case restAPIPaginationNone, restAPIPaginationPage, restAPIPaginationOffset, restAPIPaginationCursor:
	default:
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Unsupported pagination_type '%s'.", cfg.PaginationType)}
	}
	cfg.PaginationConfig = configAnyMap(config["pagination_config"])
	cfg.PollTimestampField = strings.TrimSpace(stringConfig(config["poll_timestamp_field"]))

	cfg.RequestBody = configAnyMap(config["request_body"])
	if len(cfg.RequestBody) == 0 {
		cfg.RequestBody = configAnyMap(cfg.PaginationConfig["request_body"])
	}
	cfg.FieldTypeHints = configStringMap(config["field_type_hints"])
	cfg.FieldDefaultValues = configAnyMap(config["field_default_values"])
	cfg.ContentTemplate = stringConfig(config["content_template"])
	if v, ok := restAPIConfigInt(config["batch_size"]); ok {
		cfg.BatchSize = v
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = restAPIDefaultBatchSize
	}
	if v, ok := restAPIConfigInt(config["max_pages"]); ok {
		if v <= 0 {
			return nil, &ConnectorValidationError{Message: fmt.Sprintf("Invalid REST API config: max_pages must be a positive integer, got %d", v)}
		}
		cfg.MaxPages = v
	}
	if v := restAPIConfigFloat(config["request_delay"]); v >= 0 {
		cfg.RequestDelay = v
	}
	return cfg, nil
}

// restAPITextToDict mirrors _text_to_dict: dict, JSON string, or key=value
// lines are all accepted.
func restAPITextToDict(value any) map[string]string {
	if value == nil {
		return nil
	}
	if dict, ok := value.(map[string]any); ok {
		out := make(map[string]string, len(dict))
		for k, v := range dict {
			out[k] = fmt.Sprint(v)
		}
		return out
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(text), &parsed) == nil && parsed != nil {
		out := make(map[string]string, len(parsed))
		for k, v := range parsed {
			out[k] = fmt.Sprint(v)
		}
		return out
	}
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			out[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	return out
}

// restAPICoerceFieldList mirrors the content_fields/metadata_fields coercion.
func restAPICoerceFieldList(value any) []string {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		parts := strings.Split(text, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	if list, ok := value.([]any); ok {
		out := make([]string, 0, len(list))
		for _, item := range list {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func configAnyMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func configStringMap(value any) map[string]string {
	if m, ok := value.(map[string]any); ok {
		out := make(map[string]string, len(m))
		for k, v := range m {
			out[k] = stringConfig(v)
		}
		return out
	}
	return nil
}

// restAPIConfigInt reads an integer config value without the positive-only
// coercion of configInt so values like max_pages=0 round-trip faithfully.
func restAPIConfigInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		if v, err := typed.Int64(); err == nil {
			return int(v), true
		}
	case string:
		if v, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return v, true
		}
	}
	return 0, false
}

func restAPIConfigFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		if v, err := typed.Float64(); err == nil {
			return v
		}
	case string:
		if v, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
			return v
		}
	}
	return -1
}

// restAPIURLQueryParams splits the URL query string into a string map, keeping
// the last value per key.
func restAPIURLQueryParams(parsed *url.URL) map[string]string {
	if parsed.RawQuery == "" {
		return nil
	}
	values := parsed.Query()
	out := make(map[string]string, len(values))
	for k, list := range values {
		if len(list) > 0 {
			out[k] = list[len(list)-1]
		}
	}
	return out
}

// prepare builds auth headers from credentials, mirroring _build_auth.
func (c *RestAPIConnector) prepare() error {
	if c.prepared {
		return nil
	}
	c.authHeaders = map[string]string{}
	c.basicAuth = nil

	switch c.cfg.AuthType {
	case restAPIAuthNone:
	case restAPIAuthAPIKeyHeader:
		headerName := strings.TrimSpace(stringConfig(c.cfg.AuthConfig["header_name"]))
		apiKey := stringConfig(c.credentials["api_key"])
		if apiKey == "" {
			apiKey = stringConfig(c.cfg.AuthConfig["api_key_value"])
		}
		if apiKey == "" {
			apiKey = stringConfig(c.cfg.AuthConfig["api_key"])
		}
		if headerName == "" || apiKey == "" {
			return &ConnectorMissingCredentialError{Message: "REST API (api_key_header) requires 'header_name' in auth_config and 'api_key' in credentials"}
		}
		c.authHeaders[headerName] = apiKey
	case restAPIAuthBearer:
		token := stringConfig(c.credentials["token"])
		if token == "" {
			token = stringConfig(c.cfg.AuthConfig["token"])
		}
		if token == "" {
			return &ConnectorMissingCredentialError{Message: "REST API (bearer) requires 'token' in credentials"}
		}
		c.authHeaders["Authorization"] = "Bearer " + token
	case restAPIAuthBasic:
		username := stringConfig(c.credentials["username"])
		if username == "" {
			username = stringConfig(c.cfg.AuthConfig["username"])
		}
		password := stringConfig(c.credentials["password"])
		if password == "" {
			password = stringConfig(c.cfg.AuthConfig["password"])
		}
		if username == "" || password == "" {
			return &ConnectorMissingCredentialError{Message: "REST API (basic) requires 'username' and 'password'"}
		}
		c.basicAuth = &restAPIBasicAuth{username: username, password: password}
	default:
		return &ConnectorValidationError{Message: fmt.Sprintf("Unsupported auth_type: %s", c.cfg.AuthType)}
	}
	c.prepared = true
	return nil
}

// Validate validates configuration and credential presence without network
// I/O; live connectivity validation is performed by ValidateLive.
func (c *RestAPIConnector) Validate(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "rest API connector is nil"}
	}
	return c.prepare()
}

// ValidateLive mirrors RestAPIConnector.validate_config: schema validation plus
// a live single-page fetch with the stored credentials.
func (c *RestAPIConnector) ValidateLive(ctx context.Context) error {
	if c == nil {
		return &ConnectorValidationError{Message: "rest API connector is nil"}
	}
	if err := c.prepare(); err != nil {
		return err
	}
	params := map[string]any{}
	switch c.cfg.PaginationType {
	case restAPIPaginationPage:
		page := 1
		if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["start_page"]); ok {
			page = v
		}
		perPage, err := c.resolvePageSize()
		if err != nil {
			return err
		}
		c.applyPagePagination(params, page, perPage)
	case restAPIPaginationOffset:
		perPage, err := c.resolvePageSize()
		if err != nil {
			return err
		}
		offset := 0
		if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["start_offset"]); ok {
			offset = v
		}
		limit := perPage
		if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["limit"]); ok {
			limit = v
		}
		if limit <= 0 {
			limit = perPage
		}
		c.applyOffsetPagination(params, offset, limit)
	case restAPIPaginationCursor:
		if cursor := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["initial_cursor"])); cursor != "" {
			c.applyCursorPagination(params, cursor)
		}
	}
	_, err := c.fetchPage(ctx, params)
	return err
}

// ValidateConnectorSetting validates REST API settings from an unsaved config.
func (c *RestAPIConnector) ValidateConnectorSetting(ctx context.Context, request map[string]any) error {
	ctx, cancel := context.WithTimeout(ctx, connectorSettingValidationTimeout)
	defer cancel()
	return c.ValidateLive(ctx)
}

// OpenSync opens one sync session. Incremental runs filter documents to the
// request window (start <= updated_at < end).
func (c *RestAPIConnector) OpenSync(ctx context.Context, request SyncRequest) (SyncSession, error) {
	if err := c.prepare(); err != nil {
		return nil, err
	}
	var windowStart *time.Time
	if !request.FromBeginning && request.WindowStart != nil {
		ws := request.WindowStart.UTC()
		windowStart = &ws
	}
	session := &restAPISyncSession{
		connector:     c,
		iterator:      newRestAPIItemIterator(c),
		batchSize:     c.cfg.BatchSize,
		fromBeginning: request.FromBeginning,
		windowStart:   windowStart,
		windowEnd:     request.WindowEnd.UTC(),
	}
	if err := session.applyResume(request.Resume); err != nil {
		return nil, err
	}
	return session, nil
}

// OpenPrune reports prune as unsupported for sources that cannot enumerate a
// complete slim snapshot; prune tasks then complete as no-ops.
func (c *RestAPIConnector) OpenPrune(ctx context.Context, request PruneRequest) (PruneSession, error) {
	return nil, ErrPruneUnsupported
}

// ---------------------------------------------------------------------------
// HTTP fetching
// ---------------------------------------------------------------------------

// fetchPage fetches a single page with retry, mirroring _fetch_page.
func (c *RestAPIConnector) fetchPage(ctx context.Context, params map[string]any) (any, error) {
	delay := restAPIRetryBaseDelay
	var lastErr error
	for attempt := 0; attempt < restAPIRetryTries; attempt++ {
		response, err := c.fetchPageOnce(ctx, params)
		if err == nil {
			return response, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var (
			valErr  *ConnectorValidationError
			credErr *ConnectorMissingCredentialError
			rateErr *RateLimitTriedTooManyTimesError
		)
		if errors.As(err, &valErr) || errors.As(err, &credErr) || errors.As(err, &rateErr) {
			return nil, err
		}
		lastErr = err
		if attempt == restAPIRetryTries-1 {
			break
		}
		sleepFor := delay + time.Duration(rand.Float64()*float64(restAPIRetryJitter))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleepFor):
		}
		delay = time.Duration(float64(delay) * float64(restAPIRetryBackoff))
		if delay > restAPIRetryMaxDelay {
			delay = restAPIRetryMaxDelay
		}
	}
	return nil, lastErr
}

// fetchPageOnce performs one page fetch including the 429 wait loop and manual
// redirect handling with per-hop SSRF validation and DNS pinning.
func (c *RestAPIConnector) fetchPageOnce(ctx context.Context, params map[string]any) (any, error) {
	if err := c.prepare(); err != nil {
		return nil, err
	}
	merged := map[string]any{}
	for k, v := range c.urlParams {
		merged[k] = v
	}
	for k, v := range c.cfg.QueryParams {
		merged[k] = v
	}
	for k, v := range params {
		merged[k] = v
	}

	currentURL, queryParams := c.buildURLWithTemplates(merged)
	currentMethod := c.cfg.Method
	currentHeaders := make(map[string]string, len(c.cfg.Headers)+len(c.authHeaders))
	for k, v := range c.cfg.Headers {
		currentHeaders[k] = v
	}
	for k, v := range c.authHeaders {
		currentHeaders[k] = v
	}
	currentBody := c.cfg.RequestBody
	currentAuth := c.basicAuth
	previousNetloc := restAPINetloc(currentURL)

	for hop := 0; hop <= restAPIMaxRedirects; hop++ {
		hostname, pinIP, err := assertRestAPIURLSafe(ctx, currentURL)
		if err != nil {
			return nil, &ConnectorValidationError{Message: "Unsafe REST API URL: " + err.Error()}
		}

		var resp *http.Response
		var respErr error
		for wait := 0; wait < restAPI429MaxWaits; wait++ {
			resp, respErr = c.doRequest(ctx, currentMethod, currentURL, currentHeaders, queryParams, currentBody, currentAuth, hostname, pinIP)
			if respErr != nil {
				return nil, respErr
			}
			if resp.StatusCode != http.StatusTooManyRequests {
				break
			}
			retryAfter := restAPI429DefaultWait
			if raw := resp.Header.Get("Retry-After"); raw != "" {
				if seconds, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && seconds >= 0 {
					retryAfter = time.Duration(seconds) * time.Second
				}
			}
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryAfter):
			}
		}
		if respErr != nil {
			return nil, respErr
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			return nil, &RateLimitTriedTooManyTimesError{Message: fmt.Sprintf("REST API rate limited: exceeded '%d' retries (too many requests)", restAPI429MaxWaits)}
		}

		if restAPIIsRedirect(resp.StatusCode) {
			location := resp.Header.Get("Location")
			if location == "" {
				return c.handleResponse(resp)
			}
			nextURL, err := restAPIResolveURL(currentURL, location)
			if err != nil {
				resp.Body.Close()
				return nil, &ConnectorValidationError{Message: "Unsafe REST API URL: " + err.Error()}
			}
			nextNetloc := restAPINetloc(nextURL)
			if nextNetloc != "" && nextNetloc != previousNetloc {
				currentHeaders = restAPIStripAuthHeaders(currentHeaders)
				currentAuth = nil
			}
			previousNetloc = nextNetloc
			if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusSeeOther {
				currentMethod = "GET"
				currentBody = nil
				queryParams = nil
			}
			resp.Body.Close()
			currentURL = nextURL
			continue
		}
		return c.handleResponse(resp)
	}
	return nil, &ConnectorValidationError{Message: fmt.Sprintf("Exceeded %d redirects fetching %q", restAPIMaxRedirects, currentURL)}
}

func (c *RestAPIConnector) doRequest(
	ctx context.Context,
	method, rawURL string,
	headers map[string]string,
	params map[string]any,
	body map[string]any,
	auth *restAPIBasicAuth,
	hostname string,
	pinIP net.IP,
) (*http.Response, error) {
	transport := newRestAPIPinnedTransport(hostname, pinIP)
	client := &http.Client{
		Transport: transport,
		Timeout:   restAPIRequestTimeout,
		// Redirects are handled manually so auth headers can be stripped on
		// cross-origin hops and each hop is re-validated against SSRF.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	fullURL := rawURL
	if encoded := restAPIEncodeParams(params); encoded != "" {
		sep := "?"
		if strings.Contains(rawURL, "?") {
			sep = "&"
		}
		fullURL = rawURL + sep + encoded
	}

	var req *http.Request
	var err error
	switch method {
	case "GET":
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	case "POST":
		var payload []byte
		if body == nil {
			payload = []byte("{}")
		} else {
			payload, err = json.Marshal(body)
			if err != nil {
				return nil, &ConnectorValidationError{Message: "REST API request body is not valid JSON"}
			}
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(string(payload)))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
		}
	default:
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("Unsupported HTTP method: %s", method)}
	}
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if auth != nil {
		req.SetBasicAuth(auth.username, auth.password)
	}
	resp, err := client.Do(req)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	resp.Body = &restAPICloseIdleBody{body: resp.Body, transport: transport}
	return resp, nil
}

// restAPICloseIdleBody closes the underlying response body and then releases
// the pinned transport's idle connections, so the per-request transports do
// not leak keep-alive sockets during long paginated syncs.
type restAPICloseIdleBody struct {
	body      io.ReadCloser
	transport *http.Transport
}

func (b *restAPICloseIdleBody) Read(p []byte) (int, error) { return b.body.Read(p) }

func (b *restAPICloseIdleBody) Close() error {
	err := b.body.Close()
	b.transport.CloseIdleConnections()
	return err
}

func (c *RestAPIConnector) handleResponse(resp *http.Response) (any, error) {
	defer resp.Body.Close()
	status := resp.StatusCode
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return nil, &ConnectorMissingCredentialError{Message: fmt.Sprintf("REST API authentication failed with status %d", status)}
	case status >= 400 && status < 500 && status != http.StatusTooManyRequests:
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("REST API request failed with non-retriable client error status %d", status)}
	case status >= 500:
		// The "http <status>" wording lets the syncer's task-level retry
		// classifier (isTransientSyncError) recognize exhausted server errors.
		return nil, fmt.Errorf("REST API request failed with http %d", status)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, &ConnectorValidationError{Message: "REST API response is not valid JSON"}
	}
	return value, nil
}

// newRestAPIPinnedTransport resolves the target once (in assertRestAPIURLSafe)
// and pins that address for the actual connection, preventing DNS rebinding.
func newRestAPIPinnedTransport(hostname string, pinIP net.IP) *http.Transport {
	dialer := &net.Dialer{Timeout: restAPIRequestTimeout, KeepAlive: 30 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				port = "443"
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(pinIP.String(), port))
		},
		TLSClientConfig: &tls.Config{
			ServerName: hostname,
			MinVersion: tls.VersionTLS12,
		},
		ForceAttemptHTTP2: true,
	}
}

// buildURLWithTemplates substitutes {key} placeholders in the URL and returns
// the remaining query parameters.
func (c *RestAPIConnector) buildURLWithTemplates(params map[string]any) (string, map[string]any) {
	u := c.baseURL
	remaining := make(map[string]any, len(params))
	for k, v := range params {
		remaining[k] = v
	}
	for k, v := range params {
		placeholder := "{" + k + "}"
		if strings.Contains(u, placeholder) {
			u = strings.ReplaceAll(u, placeholder, restAPIParamString(v))
			delete(remaining, k)
		}
	}
	return u, remaining
}

func restAPIParamString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func restAPIEncodeParams(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}
	values := url.Values{}
	for k, v := range params {
		values.Set(k, restAPIParamString(v))
	}
	return values.Encode()
}

func restAPINetloc(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func restAPIResolveURL(base, location string) (string, error) {
	baseParsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(location)
	if err != nil {
		return "", err
	}
	return baseParsed.ResolveReference(ref).String(), nil
}

func restAPIIsRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

var restAPIAuthSensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"apikey":              {},
	"api-key":             {},
	"x-api-key":           {},
	"x-auth-token":        {},
}

func restAPIStripAuthHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if _, sensitive := restAPIAuthSensitiveHeaders[strings.ToLower(k)]; sensitive {
			continue
		}
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// SSRF protection
// ---------------------------------------------------------------------------

// validateRestAPIURLForSSRF performs quick deny-list checks plus DNS
// resolution. Resolution failure is logged and tolerated because the
// per-request check re-validates.
func validateRestAPIURLForSSRF(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &ConnectorValidationError{Message: "REST API connector URL must include a hostname."}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &ConnectorValidationError{Message: fmt.Sprintf("Unsupported URL scheme for REST API connector: %q. Only http/https are allowed.", parsed.Scheme)}
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return &ConnectorValidationError{Message: "REST API connector URL must include a hostname."}
	}
	if strings.EqualFold(hostname, "localhost") {
		return &ConnectorValidationError{Message: fmt.Sprintf("REST API connector URL hostname %q is not allowed (localhost is blocked).", hostname)}
	}
	if restAPISSRFAllowLoopback {
		return nil
	}
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		// DNS failure is not an SSRF condition by itself; the per-request
		// check will surface it if it matters.
		return nil
	}
	for _, addr := range addrs {
		if !restAPIIPIsGlobal(restAPIEffectiveIP(addr)) {
			return &ConnectorValidationError{Message: fmt.Sprintf(
				"REST API connector URL %q resolves to disallowed address %s (localhost, private, link-local, reserved, or multicast addresses are blocked).",
				rawURL, addr)}
		}
	}
	return nil
}

// assertRestAPIURLSafe mirrors ssrf_guard.assert_url_is_safe: every resolved
// address must be globally routable. It returns the hostname and the first
// validated IP so the caller can pin DNS.
func assertRestAPIURLSafe(ctx context.Context, rawURL string) (string, net.IP, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", nil, fmt.Errorf("URL is missing a host.")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, fmt.Errorf("Disallowed URL scheme: %q. Only [http https] are allowed.", parsed.Scheme)
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", nil, fmt.Errorf("URL is missing a host.")
	}
	if restAPISSRFAllowLoopback {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
		if err != nil {
			return "", nil, fmt.Errorf("Could not resolve hostname %q: %w", hostname, err)
		}
		if len(addrs) == 0 {
			return "", nil, fmt.Errorf("Hostname %q resolved to no addresses.", hostname)
		}
		return hostname, addrs[0].IP, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", nil, fmt.Errorf("Could not resolve hostname %q: %w", hostname, err)
	}
	var first net.IP
	for _, addr := range addrs {
		eff := restAPIEffectiveIP(addr.IP)
		if !restAPIIPIsGlobal(eff) {
			return "", nil, fmt.Errorf("URL resolves to a non-public address (%s), which is not allowed.", addr.IP)
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

// restAPIEffectiveIP returns the IPv4 equivalent for IPv4-mapped IPv6
// addresses, mirroring ssrf_guard._effective_ip.
func restAPIEffectiveIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil && len(ip) == net.IPv6len {
		return v4
	}
	return ip
}

// restAPIIPIsGlobal mirrors ipaddress.is_global for the address classes that
// matter in practice.
func restAPIIPIsGlobal(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		first := v4[0]
		// 0.0.0.0/8, 100.64.0.0/10 (CGNAT), 192.0.0.0/24 (IETF protocol
		// assignments), 198.18.0.0/15 (benchmarking), 240.0.0.0/4, broadcast.
		if first == 0 || first == 100 || (first == 192 && v4[1] == 0) ||
			(first == 198 && v4[1]&0xfe == 18) || first >= 240 {
			return false
		}
		return true
	}
	// IPv6 documentation range (2001:db8::/32) is not globally routable.
	if len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

// resolvePageSize mirrors _resolve_page_size.
func (c *RestAPIConnector) resolvePageSize() (int, error) {
	if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["page_size"]); ok && v > 0 {
		return v, nil
	}
	sizeParam := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["page_size_param"]))
	if sizeParam == "" {
		sizeParam = strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["limit_param"]))
	}
	if sizeParam != "" {
		for _, source := range []map[string]string{c.cfg.QueryParams, c.urlParams} {
			if raw, ok := source[sizeParam]; ok {
				if v, err := strconv.Atoi(raw); err == nil && v > 0 {
					return v, nil
				}
			}
		}
	}
	return c.cfg.BatchSize, nil
}

func (c *RestAPIConnector) applyPagePagination(params map[string]any, page, perPage int) {
	pageParam := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["page_param"]))
	if pageParam == "" {
		pageParam = "page"
	}
	params[pageParam] = page
	if sizeParam := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["page_size_param"])); sizeParam != "" {
		params[sizeParam] = perPage
	}
}

func (c *RestAPIConnector) applyOffsetPagination(params map[string]any, offset, limit int) {
	offsetParam := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["offset_param"]))
	if offsetParam == "" {
		offsetParam = "offset"
	}
	params[offsetParam] = offset
	if limitParam := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["limit_param"])); limitParam != "" {
		params[limitParam] = limit
	}
}

func (c *RestAPIConnector) applyCursorPagination(params map[string]any, cursor string) {
	cursorParam := strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["cursor_param"]))
	if cursorParam == "" {
		cursorParam = "cursor"
	}
	params[cursorParam] = cursor
}

// restAPIItemIterator mirrors _iter_items: it walks pages applying the
// configured pagination and stopping when the source reports no more items.
type restAPIItemIterator struct {
	c           *RestAPIConnector
	pageCount   int
	page        int
	offset      int
	limit       int
	perPage     int
	cursor      string
	finished    bool
	lastPos     *restAPISyncCursor
	seenCursors map[string]struct{}
}

func newRestAPIItemIterator(c *RestAPIConnector) *restAPIItemIterator {
	it := &restAPIItemIterator{c: c, seenCursors: map[string]struct{}{}}
	if perPage, err := c.resolvePageSize(); err == nil {
		it.perPage = perPage
	} else {
		it.perPage = c.cfg.BatchSize
	}
	if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["start_page"]); ok {
		it.page = v
	} else {
		it.page = 1
	}
	if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["start_offset"]); ok {
		it.offset = v
	}
	it.limit = it.perPage
	if v, ok := restAPIConfigInt(c.cfg.PaginationConfig["limit"]); ok {
		it.limit = v
	}
	if it.limit <= 0 {
		it.limit = it.perPage
	}
	it.cursor = strings.TrimSpace(stringConfig(c.cfg.PaginationConfig["initial_cursor"]))
	if it.cursor != "" {
		it.seenCursors[it.cursor] = struct{}{}
	}
	return it
}

// nextPage returns the items of the next page, or nil when iteration is done.
func (it *restAPIItemIterator) nextPage(ctx context.Context) ([]map[string]any, error) {
	if it.finished || it.pageCount >= it.c.cfg.MaxPages {
		return nil, nil
	}
	it.lastPos = it.positionCursor()
	params := map[string]any{}
	switch it.c.cfg.PaginationType {
	case restAPIPaginationPage:
		it.c.applyPagePagination(params, it.page, it.perPage)
	case restAPIPaginationOffset:
		it.c.applyOffsetPagination(params, it.offset, it.limit)
	case restAPIPaginationCursor:
		if it.cursor != "" {
			it.c.applyCursorPagination(params, it.cursor)
		}
	}
	if it.pageCount > 0 && it.c.cfg.RequestDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(it.c.cfg.RequestDelay * float64(time.Second))):
		}
	}

	response, err := it.c.fetchPage(ctx, params)
	if err != nil {
		var (
			valErr  *ConnectorValidationError
			credErr *ConnectorMissingCredentialError
		)
		if errors.As(err, &valErr) || errors.As(err, &credErr) {
			return nil, err
		}
		return nil, &ConnectorValidationError{Message: fmt.Sprintf("REST API page fetch failed: %v", err)}
	}

	items := restAPIExtractItems(response, it.c.cfg.ItemsPath)
	hasNext, reportsHasNext := restAPIExtractHasNextPage(response, it.c.cfg.PaginationConfig)
	if len(items) == 0 && !(it.c.cfg.PaginationType == restAPIPaginationCursor && reportsHasNext && hasNext) {
		it.finished = true
		return nil, nil
	}
	it.pageCount++

	switch it.c.cfg.PaginationType {
	case restAPIPaginationNone:
		it.finished = true
	case restAPIPaginationPage:
		if len(items) < it.perPage {
			it.finished = true
		} else {
			it.page++
		}
	case restAPIPaginationOffset:
		if len(items) < it.limit {
			it.finished = true
		} else {
			it.offset += it.limit
		}
	case restAPIPaginationCursor:
		if reportsHasNext && !hasNext {
			it.finished = true
			break
		}
		next := restAPIExtractNextCursor(response, it.c.cfg.PaginationConfig)
		if next == "" {
			it.finished = true
		} else if _, repeated := it.seenCursors[next]; repeated {
			return nil, fmt.Errorf("rest api pagination repeated cursor %q: %w", next, ErrSyncResumeInvalid)
		} else {
			it.seenCursors[next] = struct{}{}
			it.cursor = next
		}
	}
	return items, nil
}

// positionCursor returns the position of the page that was just fetched. It is
// nil for pagination_type=none, which never emits checkpoints.
func (it *restAPIItemIterator) positionCursor() *restAPISyncCursor {
	switch it.c.cfg.PaginationType {
	case restAPIPaginationPage:
		return &restAPISyncCursor{Page: it.page, SourceID: ""}
	case restAPIPaginationOffset:
		return &restAPISyncCursor{Offset: it.offset, SourceID: ""}
	case restAPIPaginationCursor:
		return &restAPISyncCursor{Cursor: it.cursor, SourceID: ""}
	}
	return nil
}

// currentPagePosition returns the position of the page fetched by the most
// recent nextPage call. It keeps the page position, not the next page.
func (it *restAPIItemIterator) currentPagePosition() *restAPISyncCursor {
	if it.lastPos == nil {
		return nil
	}
	pos := *it.lastPos
	return &pos
}

// ---------------------------------------------------------------------------
// JSON extraction
// ---------------------------------------------------------------------------

// restAPIExtractItems mirrors _extract_items.
func restAPIExtractItems(response any, itemsPath string) []map[string]any {
	var items []any
	if itemsPath != "" {
		matches, err := restAPIJSONPathMatches(response, itemsPath)
		if err != nil {
			return nil
		}
		if len(matches) == 0 {
			return nil
		}
		if len(matches) == 1 {
			if list, ok := matches[0].([]any); ok {
				items = list
			} else {
				items = matches
			}
		} else {
			items = matches
		}
	} else if list, ok := response.([]any); ok {
		items = list
	} else if dict, ok := response.(map[string]any); ok {
		for _, key := range []string{"items", "results", "data", "records"} {
			if value, ok := dict[key]; ok {
				if list, ok := value.([]any); ok {
					items = list
					break
				}
			}
		}
		if items == nil {
			for _, value := range dict {
				if list, ok := value.([]any); ok {
					items = list
					break
				}
			}
		}
	}

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapping, ok := item.(map[string]any); ok {
			out = append(out, mapping)
		}
	}
	return out
}

func restAPIExtractHasNextPage(response any, paginationConfig map[string]any) (bool, bool) {
	field := strings.TrimSpace(stringConfig(paginationConfig["has_next_page_field"]))
	if field == "" {
		return false, false
	}
	dict, ok := response.(map[string]any)
	if !ok {
		return false, false
	}
	value, ok := dict[field].(bool)
	return value, ok
}

// restAPIExtractNextCursor mirrors _extract_next_cursor.
func restAPIExtractNextCursor(response any, paginationConfig map[string]any) string {
	cursorPath := strings.TrimSpace(stringConfig(paginationConfig["next_cursor_path"]))
	if cursorPath == "" {
		field := strings.TrimSpace(stringConfig(paginationConfig["next_cursor_field"]))
		if field != "" {
			if dict, ok := response.(map[string]any); ok {
				if value, ok := dict[field]; ok && value != nil {
					return fmt.Sprint(value)
				}
			}
		}
		return ""
	}
	matches, err := restAPIJSONPathMatches(response, cursorPath)
	if err != nil || len(matches) == 0 || matches[0] == nil {
		return ""
	}
	return fmt.Sprint(matches[0])
}

const (
	restAPIJSONPathField = iota
	restAPIJSONPathIndex
	restAPIJSONPathWildcard
)

type restAPIJSONPathToken struct {
	kind      int
	key       string
	index     int
	recursive bool
}

// restAPIJSONPathMatches implements the commonly used subset of jsonpath: $,
// dot keys, [n], [*], and .. recursive descent.
func restAPIJSONPathMatches(root any, path string) ([]any, error) {
	tokens, err := parseRestAPIJSONPath(path)
	if err != nil {
		return nil, err
	}
	current := []any{root}
	for _, token := range tokens {
		next := []any{}
		for _, value := range current {
			if token.recursive {
				restAPIJSONPathCollectRecursive(value, token, &next)
				continue
			}
			switch token.kind {
			case restAPIJSONPathField:
				if dict, ok := value.(map[string]any); ok {
					if child, ok := dict[token.key]; ok {
						next = append(next, child)
					}
				}
			case restAPIJSONPathIndex:
				if list, ok := value.([]any); ok && token.index >= 0 && token.index < len(list) {
					next = append(next, list[token.index])
				}
			case restAPIJSONPathWildcard:
				switch typed := value.(type) {
				case []any:
					next = append(next, typed...)
				case map[string]any:
					for _, child := range typed {
						next = append(next, child)
					}
				}
			}
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return current, nil
}

func restAPIJSONPathCollectRecursive(value any, token restAPIJSONPathToken, out *[]any) {
	if token.kind != restAPIJSONPathField {
		return
	}
	if dict, ok := value.(map[string]any); ok {
		if child, ok := dict[token.key]; ok {
			*out = append(*out, child)
		}
		for _, nested := range dict {
			restAPIJSONPathCollectRecursive(nested, token, out)
		}
	}
	if list, ok := value.([]any); ok {
		for _, nested := range list {
			restAPIJSONPathCollectRecursive(nested, token, out)
		}
	}
}

func parseRestAPIJSONPath(path string) ([]restAPIJSONPathToken, error) {
	s := strings.TrimSpace(path)
	if strings.HasPrefix(s, "$") {
		s = s[1:]
	}
	var tokens []restAPIJSONPathToken
	pendingRecursive := false
	i := 0
	for i < len(s) {
		switch s[i] {
		case '.':
			if i+1 < len(s) && s[i+1] == '.' {
				pendingRecursive = true
				i += 2
			} else {
				i++
			}
		case '[':
			end := strings.IndexByte(s[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("unterminated bracket in JSONPath")
			}
			content := strings.TrimSpace(s[i+1 : i+end])
			token := restAPIJSONPathToken{recursive: pendingRecursive}
			pendingRecursive = false
			switch {
			case content == "*":
				token.kind = restAPIJSONPathWildcard
			default:
				if n, err := strconv.Atoi(content); err == nil {
					token.kind = restAPIJSONPathIndex
					token.index = n
				} else {
					token.kind = restAPIJSONPathField
					token.key = strings.Trim(content, "'\"")
				}
			}
			tokens = append(tokens, token)
			i += end + 1
		default:
			start := i
			for i < len(s) && s[i] != '.' && s[i] != '[' {
				i++
			}
			key := s[start:i]
			if key == "" {
				return nil, fmt.Errorf("empty JSONPath segment")
			}
			tokens = append(tokens, restAPIJSONPathToken{kind: restAPIJSONPathField, key: key, recursive: pendingRecursive})
			pendingRecursive = false
		}
	}
	return tokens, nil
}

// ---------------------------------------------------------------------------
// Field extraction and item-to-document mapping
// ---------------------------------------------------------------------------

var restAPIFieldSegmentRE = regexp.MustCompile(`^([^\[\]]+)(\[(\d+|\*)\])?$`)

// extractRestAPIFieldValues mirrors _extract_field_values: dot-notation with
// optional [index] / [*] segments.
func extractRestAPIFieldValues(item map[string]any, path string) []any {
	if path == "" {
		return nil
	}
	current := []any{item}
	for _, segment := range strings.Split(path, ".") {
		if segment == "" {
			return nil
		}
		match := restAPIFieldSegmentRE.FindStringSubmatch(segment)
		key := segment
		index := ""
		if match != nil {
			key = match[1]
			index = match[3]
		}
		next := []any{}
		for _, value := range current {
			dict, ok := value.(map[string]any)
			if !ok {
				continue
			}
			child, ok := dict[key]
			if !ok || child == nil {
				continue
			}
			if index == "" {
				next = append(next, child)
				continue
			}
			list, ok := child.([]any)
			if !ok {
				continue
			}
			if index == "*" {
				next = append(next, list...)
				continue
			}
			idx, err := strconv.Atoi(index)
			if err != nil {
				continue
			}
			if idx >= 0 && idx < len(list) {
				next = append(next, list[idx])
			}
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return current
}

func extractRestAPIField(item map[string]any, path string) any {
	values := extractRestAPIFieldValues(item, path)
	if len(values) == 0 {
		return nil
	}
	if len(values) == 1 {
		return values[0]
	}
	return values
}

// getTypedFieldValue mirrors _get_typed_field_value.
func (c *RestAPIConnector) getTypedFieldValue(path string, item map[string]any) any {
	values := extractRestAPIFieldValues(item, path)
	if len(values) == 0 {
		return c.cfg.FieldDefaultValues[path]
	}
	hint := c.cfg.FieldTypeHints[path]
	converted := make([]any, 0, len(values))
	for _, value := range values {
		if cv := convertRestAPITypedValue(value, hint); cv != nil {
			converted = append(converted, cv)
		}
	}
	if len(converted) == 0 {
		return nil
	}
	if len(converted) == 1 {
		return converted[0]
	}
	parts := make([]string, 0, len(converted))
	for _, value := range converted {
		parts = append(parts, coerceRestAPIToText(value))
	}
	return strings.Join(parts, ", ")
}

func convertRestAPITypedValue(value any, hint string) any {
	switch hint {
	case "string":
		return restAPIValueText(value)
	case "number":
		f, ok := restAPIToFloat(value)
		if !ok {
			return nil
		}
		if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
			return int64(f)
		}
		return f
	case "date":
		if t, ok := value.(time.Time); ok {
			return restAPITimeISO(t)
		}
		if t := parseRestAPIDatetime(value); t != nil {
			return restAPITimeISO(*t)
		}
		return restAPIValueText(value)
	default:
		return value
	}
}

func restAPIToFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		f, err := typed.Float64()
		return f, err == nil
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// extractTimestamp mirrors _extract_timestamp.
func (c *RestAPIConnector) extractTimestamp(item map[string]any) *time.Time {
	if c.cfg.PollTimestampField == "" {
		return nil
	}
	value := extractRestAPIField(item, c.cfg.PollTimestampField)
	if list, ok := value.([]any); ok && len(list) > 0 {
		value = list[0]
	}
	return parseRestAPIDatetime(value)
}

// parseRestAPIDatetime mirrors _parse_datetime.
func parseRestAPIDatetime(value any) *time.Time {
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case time.Time:
		t := typed.UTC()
		return &t
	case json.Number:
		if f, err := typed.Float64(); err == nil {
			return restAPITimestampFromFloat(f)
		}
	case float64:
		return restAPITimestampFromFloat(typed)
	case int:
		return restAPITimestampFromFloat(float64(typed))
	case int64:
		return restAPITimestampFromFloat(float64(typed))
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		layouts := []string{
			"2006-01-02T15:04:05.999999999Z",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, text); err == nil {
				t = t.UTC()
				return &t
			}
		}
		normalized := strings.ReplaceAll(text, " ", "T")
		if strings.HasSuffix(normalized, "Z") {
			normalized = normalized[:len(normalized)-1] + "+00:00"
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.999999999-07:00", "2006-01-02T15:04:05-07:00"} {
			if t, err := time.Parse(layout, normalized); err == nil {
				t = t.UTC()
				return &t
			}
		}
	}
	return nil
}

func restAPITimestampFromFloat(f float64) *time.Time {
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	t := time.Unix(sec, nsec).UTC()
	return &t
}

// itemToDocument mirrors _item_to_document.
func (c *RestAPIConnector) itemToDocument(item map[string]any) (SourceDocument, error) {
	var rawID string
	if c.cfg.IDField != "" {
		if v := c.getTypedFieldValue(c.cfg.IDField, item); v != nil {
			rawID = coerceRestAPIToText(v)
		} else {
			rawID = restAPIHash128("rest_api_item:" + restAPIStableText(item))
		}
	} else {
		rawID = restAPIHash128("rest_api_item:" + restAPIStableText(item))
	}
	docID := restAPIHash128("rest_api:" + rawID)

	var contentText string
	if c.cfg.ContentTemplate != "" {
		contentText = c.renderContentTemplate(item)
	} else {
		parts := []string{}
		for _, field := range c.cfg.ContentFields {
			if v := c.getTypedFieldValue(field, item); v != nil {
				if text := restAPIStripHTML(coerceRestAPIToText(v)); text != "" {
					parts = append(parts, text)
				}
			}
		}
		contentText = strings.Join(parts, "\n")
	}
	blob := []byte(contentText)

	var metadata map[string]any
	if len(c.cfg.MetadataFields) > 0 {
		metadata = map[string]any{}
		for _, field := range c.cfg.MetadataFields {
			if v := c.getTypedFieldValue(field, item); v != nil {
				metadata[field] = serializeRestAPIMetadataValue(v)
			}
		}
	}

	updatedAt := c.extractTimestamp(item)
	if updatedAt == nil {
		now := time.Now().UTC()
		updatedAt = &now
	}

	sem := rawID
	if len(c.cfg.ContentFields) > 0 {
		sem = restAPIValueText(extractRestAPIField(item, c.cfg.ContentFields[0]))
	}
	sem = restAPIStripHTML(sem)
	sem = strings.ReplaceAll(sem, "\n", " ")
	sem = strings.ReplaceAll(sem, "\r", " ")
	sem = strings.TrimSpace(sem)
	if len(sem) > 100 {
		sem = sem[:100]
	}
	if sem == "" {
		sem = docID
	}

	return SourceDocument{
		SourceID:           docID,
		SemanticIdentifier: sem,
		Extension:          ".txt",
		Blob:               blob,
		UpdatedAt:          *updatedAt,
		SizeBytes:          int64(len(blob)),
		Metadata:           metadata,
		Fingerprint:        contentFingerprint(blob),
	}, nil
}

// renderContentTemplate mirrors _render_content_template.
func (c *RestAPIConnector) renderContentTemplate(item map[string]any) string {
	values := map[string]string{}
	seen := map[string]bool{}
	fields := append(append([]string{}, c.cfg.ContentFields...), c.cfg.MetadataFields...)
	for _, field := range fields {
		if seen[field] {
			continue
		}
		seen[field] = true
		if v := c.getTypedFieldValue(field, item); v != nil {
			values[restAPIFieldToTemplateName(field)] = coerceRestAPIToText(v)
		}
	}
	rendered, ok := renderRestAPITemplateSafeMap(c.cfg.ContentTemplate, values)
	if !ok {
		parts := []string{}
		for _, field := range c.cfg.ContentFields {
			if p := coerceRestAPIToText(c.getTypedFieldValue(field, item)); p != "" {
				parts = append(parts, p)
			}
		}
		rendered = strings.Join(parts, "\n")
	}
	return restAPIStripHTML(rendered)
}

var restAPITemplateIndexRE = regexp.MustCompile(`\[\d+\]|\[\*\]`)

func restAPIFieldToTemplateName(field string) string {
	name := restAPITemplateIndexRE.ReplaceAllString(field, "")
	return strings.ReplaceAll(name, ".", "_")
}

// renderRestAPITemplateSafeMap renders {name} placeholders from values with a
// safe lookup: unknown names become the empty string; malformed placeholders
// make the whole render fail so the caller falls back to joined content.
func renderRestAPITemplateSafeMap(template string, values map[string]string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(template); {
		switch template[i] {
		case '{':
			if i+1 < len(template) && template[i+1] == '{' {
				b.WriteByte('{')
				i += 2
				continue
			}
			end := strings.IndexByte(template[i:], '}')
			if end < 0 {
				return "", false
			}
			token := template[i+1 : i+end]
			if !validRestAPITemplateKey(token) {
				return "", false
			}
			b.WriteString(values[token])
			i += end + 1
		case '}':
			if i+1 < len(template) && template[i+1] == '}' {
				b.WriteByte('}')
				i += 2
				continue
			}
			return "", false
		default:
			b.WriteByte(template[i])
			i++
		}
	}
	return b.String(), true
}

func validRestAPITemplateKey(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// restAPIHash128 mirrors api.utils.common.hash128 (xxhash128 hex digest).
func restAPIHash128(data string) string {
	return contentFingerprint([]byte(data))
}

// restAPIStableText renders a JSON value as deterministic text: single quotes,
// True/False/None, and matching float formatting. Map keys are sorted to keep
// the fallback document ID stable across runs.
func restAPIStableText(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return restAPIQuoteString(typed)
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			if f, err := typed.Float64(); err == nil {
				return restAPIFloatText(f)
			}
		}
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return restAPIFloatText(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, restAPIStableText(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, restAPIQuoteString(k)+": "+restAPIStableText(typed[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprint(value)
	}
}

// restAPIQuoteString renders a string as a single-quoted literal with the
// standard backslash escapes; printable non-ASCII characters are kept as-is.
func restAPIQuoteString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				if r < 0x100 {
					b.WriteString(fmt.Sprintf(`\x%02x`, r))
				} else {
					b.WriteString(fmt.Sprintf(`\u%04x`, r))
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// restAPIValueText renders the scalar and composite JSON values the connector deals with
// as plain text.
func restAPIValueText(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return typed
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			if f, err := typed.Float64(); err == nil {
				return restAPIFloatText(f)
			}
		}
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return restAPIFloatText(typed)
	default:
		return restAPIStableText(value)
	}
}

// restAPIFloatText renders floats in a stable textual form.
func restAPIFloatText(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	case f == math.Trunc(f) && math.Abs(f) < 1e16:
		return strconv.FormatFloat(f, 'f', 1, 64)
	default:
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

// coerceRestAPIToText mirrors _coerce_to_text.
func coerceRestAPIToText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			if f, err := typed.Float64(); err == nil {
				return restAPIFloatText(f)
			}
		}
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return restAPIFloatText(typed)
	case bool:
		if typed {
			return "True"
		}
		return "False"
	default:
		return pyJSONDumps(value)
	}
}

// serializeRestAPIMetadataValue mirrors _serialize_metadata_value.
func serializeRestAPIMetadataValue(value any) any {
	switch typed := value.(type) {
	case time.Time:
		return restAPITimeISO(typed)
	case json.Number:
		if f, err := typed.Float64(); err == nil {
			if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
				return int64(f)
			}
			return f
		}
		return typed.String()
	case int, int64, float64, bool, string:
		return value
	default:
		return pyJSONDumps(value)
	}
}

// pyJSONDumps mirrors json.dumps(value, ensure_ascii=False) with the default
// separators (", " and ": ").
func pyJSONDumps(value any) string {
	var b strings.Builder
	pyJSONDumpsTo(&b, value)
	return b.String()
}

func pyJSONDumpsTo(b *strings.Builder, value any) {
	switch typed := value.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		if typed {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case string:
		raw, _ := json.Marshal(typed)
		b.Write(raw)
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			if f, err := typed.Float64(); err == nil {
				b.WriteString(restAPIFloatText(f))
				return
			}
		}
		b.WriteString(typed.String())
	case int:
		b.WriteString(strconv.Itoa(typed))
	case int64:
		b.WriteString(strconv.FormatInt(typed, 10))
	case float64:
		b.WriteString(restAPIFloatText(typed))
	case []any:
		b.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				b.WriteString(", ")
			}
			pyJSONDumpsTo(b, item)
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteString(", ")
			}
			raw, _ := json.Marshal(k)
			b.Write(raw)
			b.WriteString(": ")
			pyJSONDumpsTo(b, typed[k])
		}
		b.WriteByte('}')
	default:
		b.WriteString(fmt.Sprint(value))
	}
}

var restAPITagRE = regexp.MustCompile(`<[^>]+>`)
var restAPIWhitespaceRE = regexp.MustCompile(`\s+`)

// restAPIStripHTML mirrors _strip_html.
func restAPIStripHTML(text string) string {
	if !strings.Contains(text, "<") || !strings.Contains(text, ">") {
		return text
	}
	cleaned := restAPITagRE.ReplaceAllString(text, " ")
	return strings.TrimSpace(restAPIWhitespaceRE.ReplaceAllString(cleaned, " "))
}

// restAPITimeISO renders a UTC time as an ISO 8601 string, keeping fractional
// seconds when non-zero.
func restAPITimeISO(t time.Time) string {
	t = t.UTC()
	base := t.Format("2006-01-02T15:04:05")
	if t.Nanosecond() != 0 {
		base += fmt.Sprintf(".%06d", t.Nanosecond()/1000)
	}
	return base + "+00:00"
}

// ---------------------------------------------------------------------------
// Sync session
// ---------------------------------------------------------------------------

// restAPISyncCursor is the resume position serialized into SyncCheckpoint.Cursor.
// It stores the current page to fetch and the source document that was last
// committed from that page; pagination_type=none never produces a checkpoint.
type restAPISyncCursor struct {
	Page     int    `json:"page,omitempty"`
	Offset   int    `json:"offset,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	SourceID string `json:"source_id,omitempty"`
}

// restAPIBufferedDocument carries the checkpoint position for an individual
// source document so a retried task can resume at the last emitted document.
type restAPIBufferedDocument struct {
	document   SourceDocument
	checkpoint *SyncCheckpoint
}

// restAPISyncSession streams items lazily so large APIs are not fully
// materialized in memory.
type restAPISyncSession struct {
	connector      *RestAPIConnector
	iterator       *restAPIItemIterator
	batchSize      int
	fromBeginning  bool
	windowStart    *time.Time
	windowEnd      time.Time
	pending        []restAPIBufferedDocument
	resumePosition *restAPISyncCursor
	resumeSourceID string
}

// applyResume restores the checkpoint page and source anchor. Cursors that are
// malformed, mismatch the configured pagination type, or carry no source anchor
// are invalid because resuming would silently skip changed data.
func (s *restAPISyncSession) applyResume(checkpoint *SyncCheckpoint) error {
	if checkpoint == nil || checkpoint.Cursor == "" {
		if checkpoint == nil {
			return nil
		}
		return fmt.Errorf("rest api sync cursor is missing: %w", ErrSyncResumeInvalid)
	}
	var cursor restAPISyncCursor
	if err := json.Unmarshal([]byte(checkpoint.Cursor), &cursor); err != nil {
		return fmt.Errorf("rest api sync cursor is invalid: %w", ErrSyncResumeInvalid)
	}
	sourceID := firstNonEmpty(cursor.SourceID, checkpoint.SourceID)
	if sourceID == "" {
		return fmt.Errorf("rest api sync checkpoint has no source anchor: %w", ErrSyncResumeInvalid)
	}
	switch s.connector.cfg.PaginationType {
	case restAPIPaginationPage:
		if cursor.Page <= 0 || cursor.Offset != 0 || cursor.Cursor != "" {
			return fmt.Errorf("rest api sync cursor does not match page pagination: %w", ErrSyncResumeInvalid)
		}
		s.iterator.page = cursor.Page
	case restAPIPaginationOffset:
		if cursor.Page != 0 || cursor.Cursor != "" {
			return fmt.Errorf("rest api sync cursor does not match offset pagination: %w", ErrSyncResumeInvalid)
		}
		s.iterator.offset = cursor.Offset
	case restAPIPaginationCursor:
		if cursor.Page != 0 || cursor.Offset != 0 {
			return fmt.Errorf("rest api sync cursor does not match cursor pagination: %w", ErrSyncResumeInvalid)
		}
		s.iterator.cursor = cursor.Cursor
	default:
		return fmt.Errorf("rest api sync checkpoint cannot resume pagination_type=none: %w", ErrSyncResumeInvalid)
	}
	position := cursor
	position.SourceID = ""
	s.resumePosition = &position
	s.resumeSourceID = sourceID
	return nil
}

// filterResumedDocuments drops every document at or before the checkpoint anchor.
// The anchor must still exist on the checkpoint page, otherwise the listing has
// changed and the runner should restart the whole fixed window.
func (s *restAPISyncSession) filterResumedDocuments(candidates []SourceDocument) ([]SourceDocument, error) {
	if s.resumeSourceID == "" {
		return candidates, nil
	}
	if s.resumePosition == nil || !restAPISyncCursorPositionEqual(s.iterator.currentPagePosition(), s.resumePosition) {
		return nil, fmt.Errorf("rest api sync resume page no longer matches checkpoint: %w", ErrSyncResumeInvalid)
	}
	for index, doc := range candidates {
		if doc.SourceID == s.resumeSourceID {
			s.resumePosition = nil
			s.resumeSourceID = ""
			return candidates[index+1:], nil
		}
	}
	return nil, fmt.Errorf("rest api resume anchor %q was not found on the checkpoint page: %w", s.resumeSourceID, ErrSyncResumeInvalid)
}

func restAPISyncCursorPositionEqual(left, right *restAPISyncCursor) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Page == right.Page && left.Offset == right.Offset && left.Cursor == right.Cursor
}

// restAPISyncCheckpoint records the current page position plus the document
// that was last emitted from it.
func restAPISyncCheckpoint(position *restAPISyncCursor, doc SourceDocument) *SyncCheckpoint {
	if position == nil {
		return nil
	}
	cursor := *position
	cursor.SourceID = doc.SourceID
	raw, err := json.Marshal(cursor)
	if err != nil {
		return nil
	}
	updatedAt := doc.UpdatedAt.UTC()
	return &SyncCheckpoint{
		Cursor:    string(raw),
		SourceID:  doc.SourceID,
		UpdatedAt: &updatedAt,
	}
}

// NextBatch returns the next batch of source documents. The batch checkpoint
// points back to the page of its last document, so a retried task re-fetches
// that page and skips through the saved source anchor.
func (s *restAPISyncSession) NextBatch(ctx context.Context) (SyncBatch, error) {
	for len(s.pending) < s.batchSize {
		items, err := s.iterator.nextPage(ctx)
		if err != nil {
			return SyncBatch{}, err
		}
		if items == nil {
			break
		}
		candidates := make([]SourceDocument, 0, len(items))
		for _, item := range items {
			doc, err := s.connector.itemToDocument(item)
			if err != nil {
				// Items that fail conversion are skipped with a warning.
				continue
			}
			if !s.fromBeginning && s.windowStart != nil {
				if !restAPIDocInTimeWindow(doc.UpdatedAt, *s.windowStart, s.windowEnd) {
					continue
				}
			}
			candidates = append(candidates, doc)
		}
		candidates, err = s.filterResumedDocuments(candidates)
		if err != nil {
			return SyncBatch{}, err
		}
		position := s.iterator.currentPagePosition()
		for _, doc := range candidates {
			s.pending = append(s.pending, restAPIBufferedDocument{
				document:   doc,
				checkpoint: restAPISyncCheckpoint(position, doc),
			})
		}
	}
	if len(s.pending) == 0 {
		return SyncBatch{}, io.EOF
	}
	end := s.batchSize
	if end > len(s.pending) {
		end = len(s.pending)
	}
	documents := make([]SourceDocument, 0, end)
	var checkpoint *SyncCheckpoint
	for _, buffered := range s.pending[:end] {
		documents = append(documents, buffered.document)
		if buffered.checkpoint != nil {
			checkpoint = buffered.checkpoint
		}
	}
	s.pending = s.pending[end:]
	return SyncBatch{Documents: documents, Checkpoint: checkpoint}, nil
}

// Close releases the session.
func (s *restAPISyncSession) Close() error {
	return nil
}

// restAPIDocInTimeWindow mirrors _doc_in_time_window: start <= dt < end.
func restAPIDocInTimeWindow(docUpdatedAt, start, end time.Time) bool {
	dt := docUpdatedAt.UTC()
	return !dt.Before(start) && dt.Before(end)
}
