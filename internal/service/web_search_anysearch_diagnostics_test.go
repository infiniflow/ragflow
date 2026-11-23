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

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/common"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func captureAnySearchLogs(t *testing.T, level zapcore.Level) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(level)
	previous := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = previous })
	return logs
}

func TestAnySearchProviderDiagnostics(t *testing.T) {
	t.Run("DIAG-429", func(t *testing.T) {
		logs := captureAnySearchLogs(t, zapcore.InfoLevel)
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Header.Get("Authorization") != "Bearer credential-secret" {
				t.Error("configured bearer missing")
			}
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"code":429,"request_id":"rid-rate","message":"credential-secret query-secret"}`)
		}))
		defer server.Close()
		result, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, "credential-secret", "query-secret")
		if err == nil || !strings.Contains(err.Error(), "HTTP status 429") || result != nil || calls != 1 {
			t.Fatalf("result=%v error=%v calls=%d", result, err, calls)
		}
		assertAnySearchErrorSafe(t, err)
		entries := logs.FilterMessage("AnySearch response").All()
		if len(entries) != 1 {
			t.Fatalf("ordinary diagnostic events = %d, want one safe request ID event", len(entries))
		}
		fields := entries[0].ContextMap()
		if fields["http_status"] != int64(429) || fields["business_code"] != int64(429) || fields["request_id"] != "rid-rate" {
			t.Fatalf("diagnostic fields = %#v", fields)
		}
		if !strings.Contains(err.Error(), "rid-rate") {
			t.Fatalf("returned HTTP failure lacks safe ID: %v", err)
		}
		for _, secret := range []string{"credential-secret", "query-secret"} {
			if strings.Contains(fmt.Sprint(logs.All()), secret) {
				t.Fatal("ordinary logs exposed request data")
			}
		}
	})
}

type diagnosticUnreadBody struct{ reads, closes int }

func (b *diagnosticUnreadBody) Read([]byte) (int, error) {
	b.reads++
	return 0, errors.New("body must stay unread")
}

func (b *diagnosticUnreadBody) Close() error { b.closes++; return nil }

func TestWebSearchRequestDiagnosticsOtherProviderControl(t *testing.T) {
	body := &diagnosticUnreadBody{}
	client := &http.Client{Transport: anySearchTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: body, Header: make(http.Header)}, nil
	})}
	result, err := webSearchRequest(t.Context(), client, http.MethodGet, "https://example.com", nil, nil)
	var status webSearchHTTPStatusError
	if !errors.As(err, &status) || int(status) != 429 || result != nil || body.reads != 0 || body.closes != 1 {
		t.Fatalf("body=%v error=%v reads=%d closes=%d", result, err, body.reads, body.closes)
	}
}

func TestAnySearchProviderDiagnosticsSafeIDs(t *testing.T) {
	for _, tc := range []struct {
		name, body, id string
		status         int
		redacted       bool
	}{
		{"success", `{"code":0,"request_id":"rid-success","data":{"results":[]}}`, "rid-success", 200, false},
		{"uuid", `{"code":0,"request_id":"56f9d6d5-2b01-4688-8d4b-28110daecb91","data":{"results":[]}}`, "56f9d6d5-2b01-4688-8d4b-28110daecb91", 200, false},
		{"business", `{"code":42,"request_id":"rid-business"}`, "rid-business", 200, false},
		{"missing", `{"code":0,"data":{"results":[]}}`, "", 200, false},
		{"malformed", `{`, "", 429, false},
		{"noninteger", `{"code":"0","request_id":"rid-code"}`, "rid-code", 200, false},
		{"key echo", `{"code":401,"request_id":"credential-secret"}`, "", 401, true},
		{"query echo", `{"code":400,"request_id":"query-secret"}`, "", 400, true},
		{"control", `{"code":400,"request_id":"rid\nunsafe"}`, "", 400, false},
		{"long", `{"code":400,"request_id":"` + strings.Repeat("a", 65) + `"}`, "", 400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureAnySearchLogs(t, zapcore.InfoLevel)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			_, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, "credential-secret", "query-secret")
			entries := logs.FilterMessage("AnySearch response").All()
			if len(entries) != 1 {
				t.Fatalf("events=%d", len(entries))
			}
			fields := entries[0].ContextMap()
			if fields["http_status"] != int64(tc.status) || (tc.id != "" && fields["request_id"] != tc.id) ||
				(tc.id == "" && (fields["request_id"] != nil || fields["request_id_unavailable"] != true)) ||
				(tc.redacted && fields["request_id_redacted"] != true) {
				t.Fatalf("fields=%v", fields)
			}
			if tc.id != "" && err != nil && !strings.Contains(err.Error(), tc.id) {
				t.Fatal("error lost the safe request ID")
			}
			if strings.Contains(fmt.Sprint(logs.All(), err), "credential-secret") || strings.Contains(fmt.Sprint(logs.All(), err), "query-secret") {
				t.Fatal("diagnostics exposed request data")
			}
		})
	}
}

func TestAnySearchProviderDiagnosticsWarnLevel(t *testing.T) {
	logs := captureAnySearchLogs(t, zapcore.WarnLevel)
	d := anySearchWebSearchDiagnostic{}
	d.observe(200, []byte(`{"code":0,"request_id":"rid-filtered"}`))
	if logs.Len() != 0 || d.requestID != "rid-filtered" {
		t.Fatal("info filtering changed safe diagnostic parsing")
	}
}

func TestAnySearchProviderDiagnosticsConcurrent(t *testing.T) {
	logs := captureAnySearchLogs(t, zapcore.InfoLevel)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		id := strings.Replace(body.Query, "query-", "rid-", 1)
		fmt.Fprintf(w, `{"code":0,"request_id":%q,"data":{"results":[]}}`, id)
	}))
	defer server.Close()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := retrieveAnySearchWebSearch(t.Context(), server.Client(), server.URL, fmt.Sprintf("key-%d", i), fmt.Sprintf("query-%d", i)); err != nil {
				t.Error("concurrent search failed")
			}
		}()
	}
	wg.Wait()
	if logs.FilterMessage("AnySearch response").Len() != 8 {
		t.Fatal("concurrent diagnostic lost an event")
	}
	ids := make(map[any]bool)
	for _, event := range logs.FilterMessage("AnySearch response").All() {
		ids[event.ContextMap()["request_id"]] = true
	}
	for i := range 8 {
		if !ids[fmt.Sprintf("rid-%d", i)] {
			t.Error("concurrent requests lost or mixed an ID")
		}
	}
}

func TestWebSearchRequestDiagnosticsFailureBody(t *testing.T) {
	for _, status := range []int{200, 429} {
		for _, oversized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/oversized=%v", status, oversized), func(t *testing.T) {
				var body io.ReadCloser
				if oversized {
					body = io.NopCloser(strings.NewReader(strings.Repeat("x", webSearchMaxResponseBytes+1)))
				} else {
					body = &diagnosticUnreadBody{}
				}
				client := &http.Client{Transport: anySearchTestTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}, nil
				})}
				observed := 0
				_, err := webSearchRequest(t.Context(), client, http.MethodGet, "https://example.com", nil, nil, func(got int, raw []byte) {
					observed++
					if got != status || raw != nil {
						t.Error("failed body exposed bytes or lost status")
					}
				})
				var httpErr webSearchHTTPStatusError
				if err == nil || observed != 1 || (status == 429 && (!errors.As(err, &httpErr) || int(httpErr) != status)) ||
					(status == 200 && oversized && !errors.Is(err, errWebSearchResponseTooLarge)) {
					t.Fatalf("error=%v events=%d", err, observed)
				}
				if reader, ok := body.(*diagnosticUnreadBody); ok && reader.closes != 1 {
					t.Fatal("response body not closed once")
				}
			})
		}
	}
}
