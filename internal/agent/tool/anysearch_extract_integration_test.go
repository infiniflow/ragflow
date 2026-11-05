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
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func liveAnySearchExtract(key string) (*AnySearchExtractTool, *anySearchLiveTransport) {
	transport := &anySearchLiveTransport{key: key}
	helper := NewHTTPHelper().WithClient(&http.Client{Transport: transport})
	return NewAnySearchExtractToolWith(helper, key), transport
}

// TestAnySearchExtractLive checks the real /v1/extract contract through the
// tool: an HTML page, a JSON URL, a Markdown file, an unsupported PDF, both
// truncation signals, and two urls the tool must refuse without any request. It
// runs anonymously, and keyed as well when ANYSEARCH_TEST_API_KEY is set.
func TestAnySearchExtractLive(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	modes := []struct{ name, key string }{{name: "anonymous"}}
	if key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY")); key != "" {
		modes = append(modes, struct{ name, key string }{name: "keyed", key: key})
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			extract, transport := liveAnySearchExtract(mode.key)
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()
			wantAuth := mode.key != ""

			// read runs one extraction and checks the request it caused: how many
			// requests, the auth header, the method, path and body.
			read := func(t *testing.T, pageURL string, wantRequests int) (anySearchExtractEnvelope, []anySearchLiveRequest) {
				t.Helper()
				out, err := extract.InvokableRun(ctx, anySearchExtractCall(pageURL))
				if err != nil {
					t.Fatalf("InvokableRun: %v", err)
				}
				env := decodeAnySearchExtractEnvelope(t, out)
				requests := transport.take()
				if len(requests) != wantRequests {
					t.Fatalf("%d requests, want %d: %+v", len(requests), wantRequests, requests)
				}
				for _, request := range requests {
					if request.HasAuthorization != wantAuth || (wantAuth && !request.BearerMatches) {
						t.Errorf("authorization present=%v matches=%v, want present=%v", request.HasAuthorization, request.BearerMatches, wantAuth)
					}
					if request.Method != http.MethodPost || request.Path != "/v1/extract" || request.Body["url"] != pageURL || len(request.Body) != 1 {
						t.Errorf("request = %+v, want POST /v1/extract with only url %q", request, pageURL)
					}
				}
				if mode.key != "" && strings.Contains(out, mode.key) {
					t.Errorf("the key leaked into the result")
				}
				return env, requests
			}
			logStep := func(t *testing.T, step string, env anySearchExtractEnvelope, requests []anySearchLiveRequest) {
				status, requestID := 0, ""
				if len(requests) > 0 {
					status, requestID = requests[0].Status, requests[0].RequestID
				}
				t.Logf("mode=%s step=%s requests=%d HTTP=%d request_id=%s content_chars=%d truncated=%v error=%q", mode.name, step, len(requests), status, requestID, env.ContentChars, env.Truncated, env.Error)
			}

			t.Run("html", func(t *testing.T) {
				env, requests := read(t, "https://example.com/", 1)
				if env.Error != "" || env.Title != "Example Domain" || !strings.Contains(env.Content, "documentation examples") || env.Truncated || requests[0].Status != 200 {
					t.Fatalf("envelope = %+v, HTTP %d", env, requests[0].Status)
				}
				logStep(t, "html", env, requests)
			})

			t.Run("json", func(t *testing.T) {
				env, requests := read(t, "https://api.github.com/repos/infiniflow/ragflow", 1)
				if env.Error != "" || !strings.Contains(env.Content, `"full_name"`) || env.ContentChars == 0 {
					t.Fatalf("envelope = %+v", env)
				}
				logStep(t, "json", env, requests)
			})

			t.Run("markdown", func(t *testing.T) {
				env, requests := read(t, "https://raw.githubusercontent.com/infiniflow/ragflow/main/README.md", 1)
				if env.Error != "" || !strings.Contains(env.Content, "RAGFlow") || env.ContentChars == 0 {
					t.Fatalf("envelope = %+v", env)
				}
				logStep(t, "markdown", env, requests)
			})

			t.Run("pdf is unsupported", func(t *testing.T) {
				env, requests := read(t, "https://www.w3.org/WAI/ER/tests/xhtml/testfiles/resources/pdf/dummy.pdf", 1)
				if !strings.Contains(env.Error, "HTTP 422") || env.Content != "" || env.RequestID == "" || requests[0].Status != 422 {
					t.Fatalf("envelope = %+v, HTTP %d, want a vendor HTTP 422 with its request id and no content", env, requests[0].Status)
				}
				logStep(t, "pdf", env, requests)
			})

			t.Run("plain text ends with the vendor marker", func(t *testing.T) {
				env, requests := read(t, "https://www.rfc-editor.org/rfc/rfc9110.txt", 1)
				if env.Error != "" || !env.Truncated || !strings.HasSuffix(env.Content, anySearchExtractTruncationMarker) || env.ContentChars != 50000 {
					t.Fatalf("envelope = truncated=%v chars=%d error=%q, want truncated by the marker at 50,000 characters", env.Truncated, env.ContentChars, env.Error)
				}
				logStep(t, "txt-marker", env, requests)
			})

			t.Run("html is truncated without a marker", func(t *testing.T) {
				env, requests := read(t, "https://www.rfc-editor.org/rfc/rfc9110.html", 1)
				if env.Error != "" || !env.Truncated || strings.HasSuffix(env.Content, anySearchExtractTruncationMarker) || env.ContentChars < anySearchExtractTruncationFloor {
					t.Fatalf("envelope = truncated=%v chars=%d error=%q, want truncated by the floor with no marker", env.Truncated, env.ContentChars, env.Error)
				}
				logStep(t, "html-floor", env, requests)
			})

			t.Run("refused urls make no request", func(t *testing.T) {
				for _, pageURL := range []string{"not-a-url", "http://127.0.0.1:8080/"} {
					env, requests := read(t, pageURL, 0)
					if env.Error == "" || env.Content != "" {
						t.Errorf("%s: envelope = %+v, want a client-side error", pageURL, env)
					}
					logStep(t, "refused", env, requests)
				}
			})
		})
	}
}

// TestAnySearchExtractLiveInvalidKey sends one request with a key the service
// does not know. It must fail once, with the Bearer header sent, and never fall
// back to an anonymous retry.
func TestAnySearchExtractLiveInvalidKey(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	const bogus = "pr-c-deliberately-invalid-test-key"
	extract, transport := liveAnySearchExtract(bogus)
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	out, err := extract.InvokableRun(ctx, anySearchExtractCall("https://example.com/"))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	env := decodeAnySearchExtractEnvelope(t, out)
	requests := transport.take()
	if len(requests) != 1 || !requests[0].BearerMatches {
		t.Fatalf("requests = %+v, want exactly one Bearer request and no anonymous retry", requests)
	}
	if !strings.Contains(env.Error, "HTTP 401") || env.Content != "" || strings.Contains(out, bogus) {
		t.Fatalf("envelope = %s, want an HTTP 401 error that does not echo the key", out)
	}
	t.Logf("requests=%d HTTP=%d error=%q", len(requests), requests[0].Status, env.Error)
}
