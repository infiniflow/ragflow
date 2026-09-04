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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func withRestAPITestHooks(t *testing.T) {
	t.Helper()
	origLoopback := restAPISSRFAllowLoopback
	origTries := restAPIRetryTries
	origBaseDelay := restAPIRetryBaseDelay
	origMaxDelay := restAPIRetryMaxDelay
	origBackoff := restAPIRetryBackoff
	origJitter := restAPIRetryJitter
	orig429Waits := restAPI429MaxWaits
	orig429Wait := restAPI429DefaultWait
	restAPISSRFAllowLoopback = true
	restAPIRetryTries = 3
	restAPIRetryBaseDelay = time.Millisecond
	restAPIRetryMaxDelay = 10 * time.Millisecond
	restAPIRetryBackoff = 2
	restAPIRetryJitter = 0
	restAPI429MaxWaits = 3
	restAPI429DefaultWait = time.Millisecond
	t.Cleanup(func() {
		restAPISSRFAllowLoopback = origLoopback
		restAPIRetryTries = origTries
		restAPIRetryBaseDelay = origBaseDelay
		restAPIRetryMaxDelay = origMaxDelay
		restAPIRetryBackoff = origBackoff
		restAPIRetryJitter = origJitter
		restAPI429MaxWaits = orig429Waits
		restAPI429DefaultWait = orig429Wait
	})
}

func mustRestAPIConnector(t *testing.T, config map[string]any) *RestAPIConnector {
	t.Helper()
	c, err := NewRestAPIConnector(config)
	if err != nil {
		t.Fatalf("NewRestAPIConnector: %v", err)
	}
	return c
}

func TestNewRestAPIConnectorDefaults(t *testing.T) {
	withRestAPITestHooks(t)
	c := mustRestAPIConnector(t, map[string]any{
		"url":            "https://example.com/api",
		"content_fields": "title, body",
	})
	if c.cfg.Method != "GET" {
		t.Fatalf("method=%q want GET", c.cfg.Method)
	}
	if c.cfg.BatchSize != 2 {
		t.Fatalf("batch_size=%d want 2", c.cfg.BatchSize)
	}
	if c.cfg.MaxPages != 1000 {
		t.Fatalf("max_pages=%d want 1000", c.cfg.MaxPages)
	}
	if c.cfg.RequestDelay != 0.5 {
		t.Fatalf("request_delay=%v want 0.5", c.cfg.RequestDelay)
	}
	if len(c.cfg.ContentFields) != 2 || c.cfg.ContentFields[0] != "title" || c.cfg.ContentFields[1] != "body" {
		t.Fatalf("content_fields=%v", c.cfg.ContentFields)
	}
	if c.cfg.AuthType != "none" || c.cfg.PaginationType != "none" {
		t.Fatalf("auth=%q pagination=%q", c.cfg.AuthType, c.cfg.PaginationType)
	}
}

func TestNewRestAPIConnectorClampsNonPositiveBatchSize(t *testing.T) {
	withRestAPITestHooks(t)
	for _, batchSize := range []any{0, -1} {
		c := mustRestAPIConnector(t, map[string]any{
			"url":            "https://example.com/api",
			"content_fields": "title",
			"batch_size":     batchSize,
		})
		if c.cfg.BatchSize != restAPIDefaultBatchSize {
			t.Fatalf("batch_size=%v parsed to %d, want %d", batchSize, c.cfg.BatchSize, restAPIDefaultBatchSize)
		}
	}
}

type restAPITestReadCloser struct {
	reader   io.Reader
	closeErr error
}

func (b *restAPITestReadCloser) Read(p []byte) (int, error) { return b.reader.Read(p) }
func (b *restAPITestReadCloser) Close() error               { return b.closeErr }

func TestRestAPICloseIdleBodyPreservesBodyAndCloseError(t *testing.T) {
	closeErr := errors.New("close boom")
	body := &restAPITestReadCloser{reader: strings.NewReader("hello"), closeErr: closeErr}
	wrapped := &restAPICloseIdleBody{body: body, transport: &http.Transport{}}
	got, err := io.ReadAll(wrapped)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read data=%q err=%v", got, err)
	}
	if err := wrapped.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("close err=%v want %v", err, closeErr)
	}
}

func TestNewRestAPIConnectorValidationErrors(t *testing.T) {
	withRestAPITestHooks(t)
	tests := []struct {
		name   string
		config map[string]any
		want   string
	}{
		{name: "missing url", config: map[string]any{"content_fields": "title"}, want: "Invalid REST API config: url"},
		{name: "unsupported method", config: map[string]any{"url": "https://example.com", "method": "DELETE", "content_fields": "title"}, want: "Unsupported HTTP method 'DELETE'."},
		{name: "unsupported auth", config: map[string]any{"url": "https://example.com", "auth_type": "jwt", "content_fields": "title"}, want: "Unsupported auth_type 'jwt'."},
		{name: "unsupported pagination", config: map[string]any{"url": "https://example.com", "pagination_type": "bad", "content_fields": "title"}, want: "Unsupported pagination_type 'bad'."},
		{name: "missing content fields", config: map[string]any{"url": "https://example.com"}, want: "At least one content field must be configured (content_fields)."},
		{name: "zero max_pages", config: map[string]any{"url": "https://example.com", "max_pages": 0, "content_fields": "title"}, want: "max_pages must be a positive integer"},
		{name: "negative max_pages", config: map[string]any{"url": "https://example.com", "max_pages": -1, "content_fields": "title"}, want: "max_pages must be a positive integer"},
		{name: "bad scheme", config: map[string]any{"url": "ftp://example.com/x", "content_fields": "title"}, want: "Unsupported URL scheme"},
		{name: "localhost", config: map[string]any{"url": "http://localhost/x", "content_fields": "title"}, want: "localhost is blocked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRestAPIConnector(tt.config)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err=%v want contains %q", err, tt.want)
			}
			var valErr *ConnectorValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("err=%T want *ConnectorValidationError", err)
			}
		})
	}
}

func TestNewRestAPIConnectorBlocksPrivateAddress(t *testing.T) {
	restAPISSRFAllowLoopback = false
	defer func() { restAPISSRFAllowLoopback = false }()
	_, err := NewRestAPIConnector(map[string]any{
		"url":            "http://127.0.0.1:8080/api",
		"content_fields": "title",
	})
	if err == nil || !strings.Contains(err.Error(), "resolves to disallowed address") {
		t.Fatalf("err=%v want private address rejection", err)
	}
}

func TestRestAPITextToDict(t *testing.T) {
	dict := restAPITextToDict(map[string]any{"a": 1, "b": "x"})
	if dict["a"] != "1" || dict["b"] != "x" {
		t.Fatalf("dict=%v", dict)
	}
	jsonDict := restAPITextToDict(`{"a": "1", "b": "x"}`)
	if jsonDict["a"] != "1" || jsonDict["b"] != "x" {
		t.Fatalf("jsonDict=%v", jsonDict)
	}
	lines := restAPITextToDict("# comment\na=1\nb = x y\n")
	if lines["a"] != "1" || lines["b"] != "x y" {
		t.Fatalf("lines=%v", lines)
	}
}

func TestRestAPIAuthPreparation(t *testing.T) {
	withRestAPITestHooks(t)

	apiKey := mustRestAPIConnector(t, map[string]any{
		"url":            "https://example.com",
		"content_fields": "title",
		"auth_type":      "api_key_header",
		"auth_config":    map[string]any{"header_name": "X-Key"},
		"credentials":    map[string]any{"api_key": "secret"},
	})
	if err := apiKey.prepare(); err != nil {
		t.Fatalf("prepare api_key: %v", err)
	}
	if got := apiKey.authHeaders["X-Key"]; got != "secret" {
		t.Fatalf("X-Key=%q", got)
	}

	bearer := mustRestAPIConnector(t, map[string]any{
		"url":            "https://example.com",
		"content_fields": "title",
		"auth_type":      "bearer",
		"credentials":    map[string]any{"token": "tok"},
	})
	if err := bearer.prepare(); err != nil {
		t.Fatalf("prepare bearer: %v", err)
	}
	if got := bearer.authHeaders["Authorization"]; got != "Bearer tok" {
		t.Fatalf("Authorization=%q", got)
	}

	basic := mustRestAPIConnector(t, map[string]any{
		"url":            "https://example.com",
		"content_fields": "title",
		"auth_type":      "basic",
		"credentials":    map[string]any{"username": "u", "password": "p"},
	})
	if err := basic.prepare(); err != nil {
		t.Fatalf("prepare basic: %v", err)
	}
	if basic.basicAuth == nil || basic.basicAuth.username != "u" || basic.basicAuth.password != "p" {
		t.Fatalf("basicAuth=%+v", basic.basicAuth)
	}

	missing := mustRestAPIConnector(t, map[string]any{
		"url":            "https://example.com",
		"content_fields": "title",
		"auth_type":      "bearer",
	})
	err := missing.prepare()
	var credErr *ConnectorMissingCredentialError
	if !errors.As(err, &credErr) || credErr.Message != "REST API (bearer) requires 'token' in credentials" {
		t.Fatalf("err=%v want missing credential error", err)
	}
}

func TestRestAPIFieldExtraction(t *testing.T) {
	withRestAPITestHooks(t)
	c := mustRestAPIConnector(t, map[string]any{
		"url":                  "https://example.com",
		"content_fields":       "title",
		"field_type_hints":     map[string]any{"meta.count": "number", "meta.name": "string"},
		"field_default_values": map[string]any{"missing": "fallback"},
	})
	item := map[string]any{
		"title": "T",
		"meta":  map[string]any{"count": json.Number("2"), "name": 7},
		"tags":  []any{"a", "b"},
	}
	values := extractRestAPIFieldValues(item, "meta.count")
	if len(values) != 1 || values[0] != json.Number("2") {
		t.Fatalf("meta.count=%v", values)
	}
	if got := c.getTypedFieldValue("meta.count", item); got != int64(2) {
		t.Fatalf("typed meta.count=%v (%T)", got, got)
	}
	if got := c.getTypedFieldValue("meta.name", item); got != "7" {
		t.Fatalf("typed meta.name=%v", got)
	}
	tags := extractRestAPIFieldValues(item, "tags[*]")
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Fatalf("tags[*]=%v", tags)
	}
	if got := c.getTypedFieldValue("missing", item); got != "fallback" {
		t.Fatalf("default=%v", got)
	}
}

func TestRestAPIParseDatetime(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{input: "2026-08-14T10:00:00Z", want: "2026-08-14T10:00:00Z"},
		{input: "2026-08-14 10:00:00", want: "2026-08-14T10:00:00Z"},
		{input: "2026-08-14T18:00:00+08:00", want: "2026-08-14T10:00:00Z"},
		{input: "2026-08-14", want: "2026-08-14T00:00:00Z"},
		{input: time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC).Unix(), want: "2026-08-14T10:00:00Z"},
	}
	for _, tt := range tests {
		got := parseRestAPIDatetime(tt.input)
		if got == nil {
			t.Fatalf("parse(%v) = nil", tt.input)
		}
		if got.UTC().Format(time.RFC3339) != tt.want {
			t.Fatalf("parse(%v)=%s want %s", tt.input, got.UTC().Format(time.RFC3339), tt.want)
		}
	}
}

func TestRestAPIItemToDocument(t *testing.T) {
	withRestAPITestHooks(t)
	c := mustRestAPIConnector(t, map[string]any{
		"url":                  "https://example.com",
		"id_field":             "id",
		"content_fields":       "title",
		"metadata_fields":      "meta.count",
		"poll_timestamp_field": "updated",
	})
	item := map[string]any{
		"id":      "abc",
		"title":   "Hello <b>World</b>",
		"meta":    map[string]any{"count": json.Number("3")},
		"updated": "2026-08-14T10:00:00Z",
	}
	doc, err := c.itemToDocument(item)
	if err != nil {
		t.Fatalf("itemToDocument: %v", err)
	}
	wantID := restAPIHash128("rest_api:" + "abc")
	if doc.SourceID != wantID {
		t.Fatalf("SourceID=%s want %s", doc.SourceID, wantID)
	}
	if doc.SemanticIdentifier != "Hello World" {
		t.Fatalf("sem=%q", doc.SemanticIdentifier)
	}
	if string(doc.Blob) != "Hello World" {
		t.Fatalf("blob=%q", doc.Blob)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension=%q", doc.Extension)
	}
	if doc.Metadata["meta.count"] != int64(3) {
		t.Fatalf("metadata=%v", doc.Metadata)
	}
	wantTime := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	if !doc.UpdatedAt.Equal(wantTime) {
		t.Fatalf("UpdatedAt=%v want %v", doc.UpdatedAt, wantTime)
	}
	if doc.Fingerprint != contentFingerprint(doc.Blob) {
		t.Fatalf("fingerprint mismatch")
	}
}

func TestRestAPIHash128AndStableText(t *testing.T) {
	if got := restAPIStableText(map[string]any{"a": "x", "b": 1}); got != "{'a': 'x', 'b': 1}" {
		t.Fatalf("stable text dict=%q", got)
	}
	if got := restAPIStableText([]any{"a", 1, true, nil}); got != "['a', 1, True, None]" {
		t.Fatalf("stable text list=%q", got)
	}
	if got := restAPIQuoteString("a'b\n"); got != `'a\'b\n'` {
		t.Fatalf("quoted string=%q", got)
	}
	if got := restAPIValueText(json.Number("1.0")); got != "1.0" {
		t.Fatalf("restAPIValueText float=%q", got)
	}
	if got := restAPIValueText(json.Number("3")); got != "3" {
		t.Fatalf("restAPIValueText int=%q", got)
	}
	if got := restAPIHash128("rest_api:abc"); len(got) != 32 {
		t.Fatalf("hash length=%d want 32", len(got))
	}
}

func TestRestAPIJSONPathAndExtractItems(t *testing.T) {
	response := map[string]any{
		"data": map[string]any{
			"items": []any{
				map[string]any{"id": "1"},
				map[string]any{"id": "2"},
			},
		},
		"paging": map[string]any{"next": "cursor-1"},
	}
	items := restAPIExtractItems(response, "$.data.items")
	if len(items) != 2 {
		t.Fatalf("items=%d want 2", len(items))
	}
	if got := restAPIExtractNextCursor(response, map[string]any{"next_cursor_field": "paging.next"}); got != "" {
		t.Fatalf("next_cursor_field=%q want empty (field is top-level only)", got)
	}
	if got := restAPIExtractNextCursor(response, map[string]any{"next_cursor_path": "$.paging.next"}); got != "cursor-1" {
		t.Fatalf("next_cursor_path=%q", got)
	}
	fallback := restAPIExtractItems(map[string]any{"results": []any{map[string]any{"id": "9"}}}, "")
	if len(fallback) != 1 {
		t.Fatalf("fallback items=%d", len(fallback))
	}
}

func TestRestAPIContentTemplate(t *testing.T) {
	withRestAPITestHooks(t)
	c := mustRestAPIConnector(t, map[string]any{
		"url":              "https://example.com",
		"content_fields":   "title",
		"metadata_fields":  "meta.count",
		"content_template": "Title: {title} / Count: {meta_count}",
	})
	item := map[string]any{"title": "T", "meta": map[string]any{"count": json.Number("3")}}
	if got := c.renderContentTemplate(item); got != "Title: T / Count: 3" {
		t.Fatalf("rendered=%q", got)
	}
	broken := mustRestAPIConnector(t, map[string]any{
		"url":              "https://example.com",
		"content_fields":   "title",
		"content_template": "{",
	})
	if got := broken.renderContentTemplate(item); got != "T" {
		t.Fatalf("fallback rendered=%q", got)
	}
}

func TestRestAPIFetchPageIntegration(t *testing.T) {
	withRestAPITestHooks(t)

	var requests atomic.Int32
	var mu sync.Mutex
	var gotPath, gotAuth string
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		mu.Lock()
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items": [{"id": "1"}]}`))
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL + "/items/{tenant}",
		"query_params":   map[string]any{"tenant": "abc"},
		"content_fields": "title",
		"auth_type":      "bearer",
		"credentials":    map[string]any{"token": "tok"},
	})
	if _, err := c.fetchPage(t.Context(), map[string]any{"page": 1}); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	mu.Lock()
	path, auth, method := gotPath, gotAuth, gotMethod
	mu.Unlock()
	if path != "/items/abc" {
		t.Fatalf("path=%q want /items/abc", path)
	}
	if auth != "Bearer tok" {
		t.Fatalf("auth=%q", auth)
	}
	if method != "GET" {
		t.Fatalf("method=%q", method)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d", requests.Load())
	}
}

func TestRestAPIFetchPagePOST(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var gotMethod, gotBody, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"method":         "POST",
		"content_fields": "title",
		"request_body":   map[string]any{"q": "x"},
	})
	if _, err := c.fetchPage(t.Context(), nil); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	mu.Lock()
	method, contentType, body := gotMethod, gotContentType, gotBody
	mu.Unlock()
	if method != "POST" {
		t.Fatalf("method=%q", method)
	}
	if contentType != "application/json" {
		t.Fatalf("content-type=%q", contentType)
	}
	if !strings.Contains(body, `"q":"x"`) {
		t.Fatalf("body=%q", body)
	}
}

func TestRestAPIFetchPageErrorMapping(t *testing.T) {
	withRestAPITestHooks(t)
	tests := []struct {
		name       string
		status     int
		body       string
		wantType   any
		wantSubstr string
	}{
		{name: "unauthorized", status: 401, body: `{}`, wantType: &ConnectorMissingCredentialError{}, wantSubstr: "REST API authentication failed with status 401"},
		{name: "bad request", status: 400, body: `{}`, wantType: &ConnectorValidationError{}, wantSubstr: "REST API request failed with non-retriable client error status 400"},
		{name: "non json", status: 200, body: `not-json`, wantType: &ConnectorValidationError{}, wantSubstr: "REST API response is not valid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()
			c := mustRestAPIConnector(t, map[string]any{
				"url":            server.URL,
				"content_fields": "title",
			})
			_, err := c.fetchPage(t.Context(), nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Fatalf("err=%v want contains %q", err, tt.wantSubstr)
			}
			switch tt.wantType.(type) {
			case *ConnectorMissingCredentialError:
				var want *ConnectorMissingCredentialError
				if !errors.As(err, &want) {
					t.Fatalf("err=%T want missing credential error", err)
				}
			case *ConnectorValidationError:
				var want *ConnectorValidationError
				if !errors.As(err, &want) {
					t.Fatalf("err=%T want validation error", err)
				}
			}
		})
	}
}

func TestRestAPIFetchPageRetries(t *testing.T) {
	withRestAPITestHooks(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
	})
	if _, err := c.fetchPage(t.Context(), nil); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("attempts=%d want 3", attempts.Load())
	}
}

func TestRestAPIFetchPage429RetryAfter(t *testing.T) {
	withRestAPITestHooks(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
	})
	if _, err := c.fetchPage(t.Context(), nil); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts=%d want 2", attempts.Load())
	}
}

func TestRestAPIFetchPageRedirectStripsAuth(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var gotAuth, gotMethod string
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer final.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirector.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            redirector.URL,
		"method":         "POST",
		"content_fields": "title",
		"auth_type":      "bearer",
		"credentials":    map[string]any{"token": "tok"},
	})
	if _, err := c.fetchPage(t.Context(), nil); err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	mu.Lock()
	auth, method := gotAuth, gotMethod
	mu.Unlock()
	if auth != "" {
		t.Fatalf("cross-origin auth=%q want stripped", auth)
	}
	if method != "GET" {
		t.Fatalf("redirect method=%q want GET", method)
	}
}

func TestRestAPISyncSessionPagination(t *testing.T) {
	withRestAPITestHooks(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		page := r.URL.Query().Get("page")
		items := []any{}
		switch page {
		case "", "1":
			items = []any{
				map[string]any{"id": "1", "title": "One", "updated": "2026-08-14T10:00:00Z"},
				map[string]any{"id": "2", "title": "Two", "updated": "2026-08-14T11:00:00Z"},
			}
		case "2":
			items = []any{
				map[string]any{"id": "3", "title": "Three", "updated": "2026-08-14T12:00:00Z"},
				map[string]any{"id": "4", "title": "Four", "updated": "2026-08-14T13:00:00Z"},
			}
		default:
			items = []any{map[string]any{"id": "5", "title": "Five", "updated": "2026-08-14T14:00:00Z"}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "page",
		"pagination_config": map[string]any{"page_size": 2},
		"batch_size":        2,
		"request_delay":     0,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	var total int
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if len(batch.Documents) > 2 {
			t.Fatalf("batch size=%d", len(batch.Documents))
		}
		total += len(batch.Documents)
	}
	if total != 5 {
		t.Fatalf("total=%d want 5", total)
	}
	if requests.Load() != 3 {
		t.Fatalf("requests=%d want 3", requests.Load())
	}
}

func TestRestAPISyncSessionWindowFilter(t *testing.T) {
	withRestAPITestHooks(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []any{
				map[string]any{"id": "1", "title": "One", "updated": "2026-08-14T10:00:00Z"},
				map[string]any{"id": "2", "title": "Two", "updated": "2026-08-14T12:00:00Z"},
			},
		})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":                  server.URL,
		"content_fields":       "title",
		"id_field":             "id",
		"poll_timestamp_field": "updated",
	})
	start := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC)
	session, err := c.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("documents=%d want window-filtered id 2", len(batch.Documents))
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v want EOF", err)
	}
}

func TestRestAPIValidateLive(t *testing.T) {
	withRestAPITestHooks(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items": []}`))
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
	})
	if err := c.ValidateLive(context.Background()); err != nil {
		t.Fatalf("ValidateLive: %v", err)
	}
	if err := c.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestRestAPIValidateConnectorSetting(t *testing.T) {
	withRestAPITestHooks(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items": []}`))
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
	})
	if err := c.ValidateConnectorSetting(t.Context(), nil); err != nil {
		t.Fatalf("ValidateConnectorSetting: %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestRestAPIMaxPages(t *testing.T) {
	withRestAPITestHooks(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items": []any{map[string]any{"id": "1", "title": "One"}, map[string]any{"id": "2", "title": "Two"}},
		})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "page",
		"pagination_config": map[string]any{"page_size": 2},
		"max_pages":         1,
		"request_delay":     0,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 {
		t.Fatalf("documents=%d want 2", len(batch.Documents))
	}
	if _, err := session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v want EOF", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests=%d want 1", requests.Load())
	}
}

func TestRestAPIOpenPruneUnsupported(t *testing.T) {
	withRestAPITestHooks(t)
	c := mustRestAPIConnector(t, map[string]any{
		"url":            "https://example.com",
		"content_fields": "title",
	})
	_, err := c.OpenPrune(t.Context(), PruneRequest{})
	if !errors.Is(err, ErrPruneUnsupported) {
		t.Fatalf("err=%v want ErrPruneUnsupported", err)
	}
}

func restAPICheckpointCursor(t *testing.T, batch SyncBatch) restAPISyncCursor {
	t.Helper()
	if batch.Checkpoint == nil || batch.Checkpoint.Cursor == "" {
		t.Fatalf("checkpoint is missing")
	}
	var cursor restAPISyncCursor
	if err := json.Unmarshal([]byte(batch.Checkpoint.Cursor), &cursor); err != nil {
		t.Fatalf("decode checkpoint: %v", err)
	}
	return cursor
}

func TestRestAPIFetchPageServerErrorIsTransient(t *testing.T) {
	withRestAPITestHooks(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
	})
	_, err := c.fetchPage(t.Context(), nil)
	if err == nil || !strings.Contains(err.Error(), "http 500") {
		t.Fatalf("err=%v want message containing http 500", err)
	}
	if int(attempts.Load()) != restAPIRetryTries {
		t.Fatalf("attempts=%d want %d", attempts.Load(), restAPIRetryTries)
	}
}

func TestRestAPIFetchPage429ExhaustionIsTransient(t *testing.T) {
	withRestAPITestHooks(t)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
	})
	_, err := c.fetchPage(t.Context(), nil)
	if err == nil || !strings.Contains(err.Error(), "too many requests") {
		t.Fatalf("err=%v want message containing too many requests", err)
	}
	var rateErr *RateLimitTriedTooManyTimesError
	if !errors.As(err, &rateErr) {
		t.Fatalf("err=%T want RateLimitTriedTooManyTimesError", err)
	}
	if int(attempts.Load()) != restAPI429MaxWaits {
		t.Fatalf("attempts=%d want %d", attempts.Load(), restAPI429MaxWaits)
	}
}

func TestRestAPISyncSessionNoneNoCheckpoint(t *testing.T) {
	withRestAPITestHooks(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{
			map[string]any{"id": "1", "title": "One"},
			map[string]any{"id": "2", "title": "Two"},
			map[string]any{"id": "3", "title": "Three"},
		})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":            server.URL,
		"content_fields": "title",
		"id_field":       "id",
		"batch_size":     2,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	defer session.Close()

	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch: %v", err)
		}
		if batch.Checkpoint != nil {
			t.Fatalf("pagination none checkpoint=%+v want nil", batch.Checkpoint)
		}
	}
}

func TestRestAPISyncSessionPageResume(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		mu.Lock()
		requested = append(requested, page)
		mu.Unlock()
		items := []any{}
		switch page {
		case "1":
			items = []any{
				map[string]any{"id": "1", "title": "One"},
				map[string]any{"id": "2", "title": "Two"},
			}
		case "2":
			items = []any{
				map[string]any{"id": "3", "title": "Three"},
				map[string]any{"id": "4", "title": "Four"},
			}
		default:
			items = []any{map[string]any{"id": "5", "title": "Five"}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "page",
		"pagination_config": map[string]any{"page_size": 2},
		"batch_size":        2,
		"request_delay":     0,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	session.Close()
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 ||
		batch.Documents[0].SourceID != restAPIHash128("rest_api:1") ||
		batch.Documents[1].SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("documents=%v want ids 1,2", batch.Documents)
	}
	cursor := restAPICheckpointCursor(t, batch)
	if cursor.Page != 1 || cursor.SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("cursor=%+v want page 1 source id 2", cursor)
	}

	resumed, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: batch.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync: %v", err)
	}
	defer resumed.Close()
	var sourceIDs []string
	for {
		b, err := resumed.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("resumed NextBatch: %v", err)
		}
		for _, doc := range b.Documents {
			sourceIDs = append(sourceIDs, doc.SourceID)
		}
	}
	want := []string{restAPIHash128("rest_api:3"), restAPIHash128("rest_api:4"), restAPIHash128("rest_api:5")}
	if strings.Join(sourceIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("sourceIDs=%v want %v", sourceIDs, want)
	}
	mu.Lock()
	got := append([]string(nil), requested...)
	mu.Unlock()
	if len(got) != 4 || got[0] != "1" || got[1] != "1" || got[2] != "2" || got[3] != "3" {
		t.Fatalf("requested pages=%v want [1 1 2 3]", got)
	}
}

func TestRestAPISyncSessionOffsetResume(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		mu.Lock()
		requested = append(requested, offset)
		mu.Unlock()
		items := []any{}
		switch offset {
		case "0":
			items = []any{
				map[string]any{"id": "1", "title": "One"},
				map[string]any{"id": "2", "title": "Two"},
			}
		case "2":
			items = []any{
				map[string]any{"id": "3", "title": "Three"},
				map[string]any{"id": "4", "title": "Four"},
			}
		default:
			items = []any{map[string]any{"id": "5", "title": "Five"}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "offset",
		"pagination_config": map[string]any{"limit": 2},
		"batch_size":        2,
		"request_delay":     0,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	session.Close()
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 ||
		batch.Documents[0].SourceID != restAPIHash128("rest_api:1") ||
		batch.Documents[1].SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("documents=%v want ids 1,2", batch.Documents)
	}
	cursor := restAPICheckpointCursor(t, batch)
	if cursor.Offset != 0 || cursor.SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("cursor=%+v want offset 0 source id 2", cursor)
	}

	resumed, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: batch.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync: %v", err)
	}
	defer resumed.Close()
	var sourceIDs []string
	for {
		b, err := resumed.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("resumed NextBatch: %v", err)
		}
		for _, doc := range b.Documents {
			sourceIDs = append(sourceIDs, doc.SourceID)
		}
	}
	want := []string{restAPIHash128("rest_api:3"), restAPIHash128("rest_api:4"), restAPIHash128("rest_api:5")}
	if strings.Join(sourceIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("sourceIDs=%v want %v", sourceIDs, want)
	}
	mu.Lock()
	got := append([]string(nil), requested...)
	mu.Unlock()
	if len(got) != 4 || got[0] != "0" || got[1] != "0" || got[2] != "2" || got[3] != "4" {
		t.Fatalf("requested offsets=%v want [0 0 2 4]", got)
	}
}

func TestRestAPISyncSessionCursorResume(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		mu.Lock()
		requested = append(requested, cursor)
		mu.Unlock()
		resp := map[string]any{"items": []any{}}
		switch cursor {
		case "":
			resp["items"] = []any{
				map[string]any{"id": "1", "title": "One"},
				map[string]any{"id": "2", "title": "Two"},
			}
			resp["next_page_token"] = "t2"
		case "t2":
			resp["items"] = []any{
				map[string]any{"id": "3", "title": "Three"},
				map[string]any{"id": "4", "title": "Four"},
			}
			resp["next_page_token"] = "t3"
		default:
			resp["items"] = []any{map[string]any{"id": "5", "title": "Five"}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "cursor",
		"pagination_config": map[string]any{"next_cursor_field": "next_page_token"},
		"batch_size":        2,
		"request_delay":     0,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	session.Close()
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(batch.Documents) != 2 ||
		batch.Documents[0].SourceID != restAPIHash128("rest_api:1") ||
		batch.Documents[1].SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("documents=%v want ids 1,2", batch.Documents)
	}
	cursor := restAPICheckpointCursor(t, batch)
	if cursor.Cursor != "" || cursor.SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("cursor=%+v want empty cursor source id 2", cursor)
	}

	resumed, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: batch.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync: %v", err)
	}
	defer resumed.Close()
	var sourceIDs []string
	for {
		b, err := resumed.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("resumed NextBatch: %v", err)
		}
		for _, doc := range b.Documents {
			sourceIDs = append(sourceIDs, doc.SourceID)
		}
	}
	want := []string{restAPIHash128("rest_api:3"), restAPIHash128("rest_api:4"), restAPIHash128("rest_api:5")}
	if strings.Join(sourceIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("sourceIDs=%v want %v", sourceIDs, want)
	}
	mu.Lock()
	got := append([]string(nil), requested...)
	mu.Unlock()
	if len(got) != 4 || got[0] != "" || got[1] != "" || got[2] != "t2" || got[3] != "t3" {
		t.Fatalf("requested cursors=%v want [\"\" \"\" t2 t3]", got)
	}
}

func TestRestAPISyncSessionCheckpointResumesInsidePage(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		mu.Lock()
		requested = append(requested, page)
		mu.Unlock()
		items := []any{}
		switch page {
		case "1":
			items = []any{
				map[string]any{"id": "1", "title": "One"},
				map[string]any{"id": "2", "title": "Two"},
				map[string]any{"id": "3", "title": "Three"},
				map[string]any{"id": "4", "title": "Four"},
			}
		default:
			items = []any{map[string]any{"id": "5", "title": "Five"}}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "page",
		"pagination_config": map[string]any{"page_size": 4},
		"batch_size":        2,
		"request_delay":     0,
	})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync: %v", err)
	}
	first, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(first.Documents) != 2 {
		t.Fatalf("documents=%d want 2", len(first.Documents))
	}
	firstCursor := restAPICheckpointCursor(t, first)
	if firstCursor.Page != 1 || firstCursor.SourceID != restAPIHash128("rest_api:2") {
		t.Fatalf("first cursor=%+v want page 1 source id 2", firstCursor)
	}
	second, err := session.NextBatch(context.Background())
	session.Close()
	if err != nil {
		t.Fatalf("NextBatch: %v", err)
	}
	if len(second.Documents) != 2 {
		t.Fatalf("documents=%d want 2", len(second.Documents))
	}
	cursor := restAPICheckpointCursor(t, second)
	if cursor.Page != 1 || cursor.SourceID != restAPIHash128("rest_api:4") {
		t.Fatalf("cursor=%+v want page 1 source id 4", cursor)
	}

	resumed, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: second.Checkpoint})
	if err != nil {
		t.Fatalf("resume OpenSync: %v", err)
	}
	defer resumed.Close()
	b, err := resumed.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("resumed NextBatch: %v", err)
	}
	if len(b.Documents) != 1 || b.Documents[0].SourceID != restAPIHash128("rest_api:5") {
		t.Fatalf("resumed documents=%v want id 5", b.Documents)
	}
	if _, err := resumed.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v want EOF", err)
	}
	mu.Lock()
	got := append([]string(nil), requested...)
	mu.Unlock()
	if len(got) != 3 || got[0] != "1" || got[1] != "1" || got[2] != "2" {
		t.Fatalf("requested pages=%v want [1 1 2]", got)
	}
}

func TestRestAPISyncSessionResumeRejectsInvalidCheckpoint(t *testing.T) {
	withRestAPITestHooks(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "page",
		"pagination_config": map[string]any{"page_size": 2},
		"request_delay":     0,
	})
	mismatched, _ := json.Marshal(restAPISyncCursor{Offset: 2, SourceID: "anchor"})
	noAnchor, _ := json.Marshal(restAPISyncCursor{Page: 1})
	cases := []struct {
		name       string
		checkpoint *SyncCheckpoint
	}{
		{name: "missing cursor", checkpoint: &SyncCheckpoint{}},
		{name: "malformed cursor", checkpoint: &SyncCheckpoint{Cursor: "{"}},
		{name: "pagination mismatch", checkpoint: &SyncCheckpoint{Cursor: string(mismatched)}},
		{name: "missing source anchor", checkpoint: &SyncCheckpoint{Cursor: string(noAnchor)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: tc.checkpoint})
			if session != nil || err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
				t.Fatalf("OpenSync = session %v, err %v, want ErrSyncResumeInvalid", session, err)
			}
		})
	}
}

func TestRestAPISyncSessionResumeRejectsMissingAnchor(t *testing.T) {
	withRestAPITestHooks(t)
	var mu sync.Mutex
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		mu.Lock()
		requested = append(requested, page)
		mu.Unlock()
		items := []any{}
		switch page {
		case "1":
			items = []any{
				map[string]any{"id": "10", "title": "Ten"},
				map[string]any{"id": "11", "title": "Eleven"},
			}
		default:
			items = []any{
				map[string]any{"id": "2", "title": "Two"},
				map[string]any{"id": "3", "title": "Three"},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer server.Close()

	c := mustRestAPIConnector(t, map[string]any{
		"url":               server.URL,
		"content_fields":    "title",
		"id_field":          "id",
		"pagination_type":   "page",
		"pagination_config": map[string]any{"page_size": 2},
		"batch_size":        2,
		"request_delay":     0,
	})
	raw, _ := json.Marshal(restAPISyncCursor{Page: 1, SourceID: restAPIHash128("rest_api:2")})
	session, err := c.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{Cursor: string(raw)}})
	if err != nil {
		t.Fatalf("resume OpenSync: %v", err)
	}
	defer session.Close()
	if _, err := session.NextBatch(context.Background()); err == nil || !errors.Is(err, ErrSyncResumeInvalid) {
		t.Fatalf("NextBatch = %v, want ErrSyncResumeInvalid", err)
	}
	mu.Lock()
	got := append([]string(nil), requested...)
	mu.Unlock()
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("requested pages=%v want [1]", got)
	}
}
