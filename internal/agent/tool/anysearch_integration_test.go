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

package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// anySearchLiveRequest is what the recording transport saw of one real request.
// The key itself is never stored, only whether Authorization was sent and
// whether it carried the expected Bearer value.
type anySearchLiveRequest struct {
	Method, Path, RawQuery string
	HasAuthorization       bool
	BearerMatches          bool
	Body                   map[string]any
	Status                 int
	RequestID              string
}

type anySearchLiveTransport struct {
	key  string
	mu   sync.Mutex
	seen []anySearchLiveRequest
}

var anySearchUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (tr *anySearchLiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	seen := anySearchLiveRequest{Method: req.Method, Path: req.URL.Path, RawQuery: req.URL.RawQuery}
	_, seen.HasAuthorization = req.Header["Authorization"]
	seen.BearerMatches = seen.HasAuthorization && req.Header.Get("Authorization") == "Bearer "+tr.key
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			_ = json.NewDecoder(body).Decode(&seen.Body)
			body.Close()
		}
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	seen.Status = resp.StatusCode
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, anySearchMaxResponseBytes+1))
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	var reply struct {
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(data, &reply) == nil && anySearchUUID.MatchString(reply.RequestID) &&
		(tr.key == "" || !strings.Contains(strings.ToLower(reply.RequestID), strings.ToLower(tr.key))) {
		seen.RequestID = reply.RequestID
	}
	tr.mu.Lock()
	tr.seen = append(tr.seen, seen)
	tr.mu.Unlock()
	return resp, nil
}

// take returns and clears what was recorded since the last call.
func (tr *anySearchLiveTransport) take() []anySearchLiveRequest {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := tr.seen
	tr.seen = nil
	return out
}

func liveAnySearchTools(key string) (*AnySearchTool, *AnySearchDiscoverTool, *anySearchLiveTransport) {
	transport := &anySearchLiveTransport{key: key}
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: transport})
	return NewAnySearchToolWith(helper, key), NewAnySearchDiscoverToolWith(helper, key), transport
}

// TestAnySearchLive checks the real API contract through the tools: general and
// vertical search, a parallel batch with one invalid tag, and discovery. It
// runs anonymously, and keyed as well when ANYSEARCH_TEST_API_KEY is set.
func TestAnySearchLive(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	modes := []struct{ name, key string }{{name: "anonymous"}}
	if key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY")); key != "" {
		modes = append(modes, struct{ name, key string }{name: "keyed", key: key})
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			search, discover, transport := liveAnySearchTools(mode.key)
			ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
			defer cancel()
			wantAuth := mode.key != ""

			checkAuth := func(t *testing.T, requests []anySearchLiveRequest) {
				t.Helper()
				for _, request := range requests {
					if request.HasAuthorization != wantAuth || (wantAuth && !request.BearerMatches) {
						t.Errorf("%s %s: authorization present=%v matches=%v, want present=%v", request.Method, request.Path, request.HasAuthorization, request.BearerMatches, wantAuth)
					}
				}
			}
			run := func(t *testing.T, args string) anySearchSearchEnvelope {
				t.Helper()
				out, err := search.InvokableRun(ctx, args)
				if err != nil {
					t.Fatalf("InvokableRun: %v", err)
				}
				return decodeAnySearchEnvelope(t, out)
			}

			t.Run("general", func(t *testing.T) {
				env := run(t, `{"query":"golang context cancellation","max_results":3}`)
				requests := transport.take()
				checkAuth(t, requests)
				if env.Error != "" || len(env.Queries) != 1 || len(env.Queries[0].Results) == 0 || len(env.Queries[0].Results) > 3 {
					t.Fatalf("envelope = %+v", env)
				}
				if len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/v1/search" || requests[0].Status != 200 || requests[0].Body["tag"] != nil {
					t.Fatalf("requests = %+v, want one general POST /v1/search with HTTP 200", requests)
				}
				for _, result := range env.Queries[0].Results {
					if result["title"] == nil || result["url"] == nil {
						t.Errorf("result lacks title or url: %v", result)
					}
				}
				t.Logf("mode=%s step=general requests=%d HTTP=%d request_id=%s results=%d", mode.name, len(requests), requests[0].Status, requests[0].RequestID, len(env.Queries[0].Results))
			})

			t.Run("vertical", func(t *testing.T) {
				env := run(t, `{"query":"useEffect cleanup","tag":"code.doc","params":{"library":"react"},"max_results":2}`)
				requests := transport.take()
				checkAuth(t, requests)
				if env.Error != "" || len(env.Queries) != 1 || len(env.Queries[0].Results) == 0 {
					t.Fatalf("envelope = %+v", env)
				}
				params, _ := requests[0].Body["params"].(map[string]any)
				if len(requests) != 1 || requests[0].Body["tag"] != "code.doc" || params["library"] != "react" {
					t.Fatalf("requests = %+v, want one POST carrying tag code.doc and params.library react", requests)
				}
				t.Logf("mode=%s step=vertical requests=%d HTTP=%d request_id=%s results=%d", mode.name, len(requests), requests[0].Status, requests[0].RequestID, len(env.Queries[0].Results))
			})

			t.Run("batch with one invalid tag", func(t *testing.T) {
				env := run(t, `{"queries":[{"query":"golang generics","max_results":2},{"query":"useEffect","tag":"code.doc","params":{"library":"react"},"max_results":2},{"query":"x","tag":"nope.nope"}]}`)
				requests := transport.take()
				checkAuth(t, requests)
				if len(requests) != 3 {
					t.Fatalf("%d requests, want 3 independent POSTs", len(requests))
				}
				if env.Error != "" || len(env.Queries) != 3 {
					t.Fatalf("envelope = %+v, want three records and no top-level error", env)
				}
				if env.Queries[0].Error != "" || env.Queries[1].Error != "" || len(env.Queries[0].Results) == 0 || len(env.Queries[1].Results) == 0 {
					t.Errorf("successful queries were affected by the failure: %+v", env.Queries[:2])
				}
				if !strings.Contains(env.Queries[2].Error, "Invalid tag") || env.Queries[2].Index != 2 {
					t.Errorf("failed record = %+v, want the vendor's invalid tag error at index 2", env.Queries[2])
				}
				t.Logf("mode=%s step=batch requests=%d statuses=%d/%d/%d failed_error=%q", mode.name, len(requests), requests[0].Status, requests[1].Status, requests[2].Status, env.Queries[2].Error)
			})

			t.Run("discover", func(t *testing.T) {
				out, err := discover.InvokableRun(ctx, `{}`)
				if err != nil {
					t.Fatalf("discover: %v", err)
				}
				listing := decodeAnySearchDiscoverEnvelope(t, out)
				listRequests := transport.take()
				out, err = discover.InvokableRun(ctx, `{"domains":["code"]}`)
				if err != nil {
					t.Fatalf("discover(code): %v", err)
				}
				named := decodeAnySearchDiscoverEnvelope(t, out)
				namedRequests := transport.take()
				checkAuth(t, append(listRequests, namedRequests...))
				if listing.Error != "" || len(listing.Domains) < 10 || named.Error != "" || !strings.Contains(out, "code.doc") {
					t.Fatalf("listing = %+v named = %s", listing, out)
				}
				if len(listRequests) != 1 || listRequests[0].Method != http.MethodGet || listRequests[0].Path != "/v1/domains" ||
					len(namedRequests) != 1 || namedRequests[0].Path != "/v1/sub-domains" || namedRequests[0].RawQuery != "domain=code" {
					t.Fatalf("requests = %+v / %+v", listRequests, namedRequests)
				}
				t.Logf("mode=%s step=discover domains=%d list_request_id=%s sub_request_id=%s", mode.name, len(listing.Domains), listRequests[0].RequestID, namedRequests[0].RequestID)
			})
		})
	}
}

// TestAnySearchLiveInvalidKey sends one request with a key the service does not
// know. It must fail once, with the Bearer header sent, and never fall back to
// an anonymous retry.
func TestAnySearchLiveInvalidKey(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	const bogus = "pr-b-deliberately-invalid-test-key"
	search, _, transport := liveAnySearchTools(bogus)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	out, err := search.InvokableRun(ctx, `{"query":"golang"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchEnvelope(t, out)
	requests := transport.take()
	if len(requests) != 1 || !requests[0].BearerMatches {
		t.Fatalf("requests = %+v, want exactly one Bearer request and no anonymous retry", requests)
	}
	if env.Error == "" || !strings.Contains(env.Error, "HTTP 401") || strings.Contains(out, bogus) {
		t.Fatalf("envelope = %s, want an HTTP 401 error that does not echo the key", out)
	}
	t.Logf("requests=%d HTTP=%d error=%q", len(requests), requests[0].Status, env.Error)
}
