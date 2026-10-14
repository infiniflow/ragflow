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
	"fmt"
	"net/http"
	"testing"
)

// requireRouteNotFound asserts the HandleNoRoute JSON contract for a request
// whose path (relative to the API base) is path. The query string must not
// appear in the message.
func requireRouteNotFound(t *testing.T, resp *http.Response, path string) {
	t.Helper()

	payload := requireStatusCode(t, resp, http.StatusNotFound)
	requireCode(t, payload, 404)
	if payload["error"] != "Not Found" {
		t.Fatalf("expected error %q, got %v, payload: %v", "Not Found", payload["error"], payload)
	}
	want := fmt.Sprintf("Not Found: /api/%s%s", TestConfig.APIVersion, path)
	if payload["message"] != want {
		t.Fatalf("expected message %q, got %v, payload: %v", want, payload["message"], payload)
	}
	data, ok := payload["data"]
	if !ok || data != nil {
		t.Fatalf("expected data null, got %v, payload: %v", payload["data"], payload)
	}
}

func TestRouteNotFoundReturnsJSON(t *testing.T) {
	resp := getJSON(t, TestConfig.NoAuth(), "/__missing_route__", nil, nil)
	requireRouteNotFound(t, resp, "/__missing_route__")
}
