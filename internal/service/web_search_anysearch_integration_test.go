//go:build integration

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

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// This checks the live provider contract, not the full ordinary Chat pipeline.
func TestAnySearchLiveProvider(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	cases := []struct{ name, key string }{{name: "anonymous"}}
	if key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY")); key != "" {
		cases = append(cases, struct{ name, key string }{name: "keyed", key: key})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const query = "What is the capital of France?"
			var response struct {
				Code      *int   `json:"code"`
				RequestID string `json:"request_id"`
				Data      struct {
					Results []anySearchWebSearchResult `json:"results"`
				} `json:"data"`
			}
			requests, status := 0, 0
			client := *anySearchWebSearchHTTPClient
			client.Transport = anySearchTestTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.Method != http.MethodPost || r.URL.String() != anySearchWebSearchEndpoint {
					t.Error("request did not use the official search endpoint")
				}
				_, authPresent := r.Header["Authorization"]
				if authPresent != (tc.key != "") || (authPresent && r.Header.Get("Authorization") != "Bearer "+tc.key) {
					t.Error("authentication did not match the selected mode")
				}
				body, err := r.GetBody()
				if err != nil {
					return nil, err
				}
				var sent struct {
					Query      string `json:"query"`
					MaxResults int    `json:"max_results"`
				}
				err = json.NewDecoder(body).Decode(&sent)
				body.Close()
				if err != nil || sent.Query != query || sent.MaxResults != 6 {
					t.Error("live request query or result count differs from the contract")
				}
				res, err := http.DefaultTransport.RoundTrip(r)
				if err != nil {
					return nil, err
				}
				status = res.StatusCode
				if status < 200 || status >= 300 {
					return res, nil
				}
				data, err := io.ReadAll(io.LimitReader(res.Body, webSearchMaxResponseBytes+1))
				res.Body.Close()
				if err != nil {
					return nil, err
				}
				res.Body = io.NopCloser(bytes.NewReader(data))
				if len(data) <= webSearchMaxResponseBytes {
					_ = json.Unmarshal(data, &response)
				}
				return res, nil
			})
			original := anySearchWebSearchHTTPClient
			anySearchWebSearchHTTPClient = &client
			t.Cleanup(func() { anySearchWebSearchHTTPClient = original })
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			result, err := (&ChatPipelineService{}).retrieveWebSearch(ctx,
				resolveWebSearchProvider(map[string]interface{}{"web_search_provider": "anysearch", "anysearch_api_key": tc.key}), query)
			if err != nil {
				t.Fatalf("live provider failed: %v", err)
			}
			if requests != 1 || status < 200 || status >= 300 || response.Code == nil || *response.Code != 0 {
				t.Fatalf("requests=%d HTTP=%d successful_business_code=%t", requests, status, response.Code != nil && *response.Code == 0)
			}
			chunks, ok := result["chunks"].([]map[string]interface{})
			if !ok || len(chunks) == 0 || len(chunks) > 6 {
				t.Fatalf("live query delivered %d chunks, valid_payload=%t", len(chunks), ok)
			}
			for _, chunk := range chunks {
				matched := false
				for _, hit := range response.Data.Results {
					content := hit.Content
					if strings.TrimSpace(content) == "" {
						content = hit.Snippet
					}
					if chunk["url"] == strings.TrimSpace(hit.URL) && chunk["content_with_weight"] == strings.TrimSpace(content) && chunk["docnm_kwd"] == hit.Title {
						matched = true
						break
					}
				}
				if !matched {
					t.Error("delivered text/title/URL do not correlate with an actual upstream hit")
				}
				if id, ok := chunk["chunk_id"].(string); !ok || !strings.HasPrefix(id, "anysearch-") {
					t.Error("live citation is not scoped to AnySearch")
				}
			}
			requestID := "redacted"
			validID, _ := regexp.MatchString(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`, response.RequestID)
			if validID && (tc.key == "" || !strings.Contains(strings.ToLower(response.RequestID), strings.ToLower(tc.key))) {
				requestID = response.RequestID
			}
			t.Logf("requests=%d HTTP=%d code=0 request_id=%s upstream_hits=%d delivered_chunks=%d mode=%s", requests, status, requestID, len(response.Data.Results), len(chunks), tc.name)
		})
	}
}
