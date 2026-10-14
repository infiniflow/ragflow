//go:build integration

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

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func decodeResponseJSON(resp *http.Response) (map[string]interface{}, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func requireStatusCode(t *testing.T, resp *http.Response, expected int) map[string]interface{} {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("expected status code %d, got %d", expected, resp.StatusCode)
	}
	payload, err := decodeResponseJSON(resp)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return payload
}

func requireCode(t *testing.T, payload map[string]interface{}, expected int) {
	t.Helper()
	code, ok := payload["code"].(float64)
	if !ok || int(code) != expected {
		t.Fatalf("expected code %d, got %v, payload: %v", expected, payload["code"], payload)
	}
}

func requireCodeZero(t *testing.T, payload map[string]interface{}) {
	t.Helper()
	requireCode(t, payload, 0)
}

// mustJSON renders a request payload for logging.
func mustJSON(v interface{}) string {
	if v == nil {
		return "<nil>"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(b)
}

// doJSON runs one request through the shared config, logging the request URL,
// params and payload as well as the response status and body. The response body
// is buffered and restored so downstream helpers can decode it again.
func doJSON(t *testing.T, client *ServerTestConfig, method, path string, body, params map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()

	requestURL, err := client.buildURL(path, params)
	if err != nil {
		t.Fatalf("failed to build request URL for %s %s: %v", method, path, err)
	}
	t.Logf(">> %s %s params=%v payload=%s", method, requestURL, params, mustJSON(body))

	resp, err := client.RequestJSON(method, path, body, params, headers)
	if err != nil {
		t.Fatalf("request failed: %s %s: %v", method, path, err)
	}

	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("failed to read response body for %s %s: %v", method, path, err)
	}
	// Restore the body so callers can decode it again.
	resp.Body = io.NopCloser(bytes.NewReader(raw))

	t.Logf("<< %d %s\nresponse: %s", resp.StatusCode, method+" "+path, raw)
	return resp
}

func getJSON(t *testing.T, client *ServerTestConfig, path string, params map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()
	return doJSON(t, client, http.MethodGet, path, nil, params, headers)
}

func postJSON(t *testing.T, client *ServerTestConfig, path string, body map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()
	return doJSON(t, client, http.MethodPost, path, body, nil, headers)
}

func putJSON(t *testing.T, client *ServerTestConfig, path string, body map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()
	return doJSON(t, client, http.MethodPut, path, body, nil, headers)
}

func deleteJSON(t *testing.T, client *ServerTestConfig, path string, body map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()
	return doJSON(t, client, http.MethodDelete, path, body, nil, headers)
}

func patchJSON(t *testing.T, client *ServerTestConfig, path string, body map[string]interface{}, headers map[string]string) *http.Response {
	t.Helper()
	return doJSON(t, client, http.MethodPatch, path, body, nil, headers)
}
