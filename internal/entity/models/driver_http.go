//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NewDriverHTTPClient returns an *http.Client with the standard connection-pool
// settings used by every model driver. It clones http.DefaultTransport when
// possible so proxy settings (HTTP_PROXY / HTTPS_PROXY / NO_PROXY) and any
// process-wide transport customisation are inherited automatically.
//
// Drivers should call this once in their constructor and store the result in
// baseModel.httpClient; do not create a bare &http.Client{} inline.
func NewDriverHTTPClient() *http.Client {
	var t *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		t = dt.Clone()
	} else {
		t = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = 90 * time.Second
	t.DisableCompression = false
	t.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{Transport: t}
}

// PostJSONRequest marshals body to JSON, creates a POST request to url using
// client, and sets the Content-Type header to application/json.  If auth is
// non-empty it is set as the Authorization header verbatim (include the scheme,
// e.g. "Bearer sk-...").
//
// The caller is responsible for closing resp.Body.
func PostJSONRequest(ctx context.Context, client *http.Client, url, auth string, body map[string]interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return client.Do(req)
}

// ReadErrorBody reads all bytes from r and returns them as a string suitable
// for embedding in an error message. It never returns an error of its own;
// if reading fails it returns an empty string so the caller's error path stays
// simple.
func ReadErrorBody(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
