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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/common"
)

// TestDriverCallsRetryProviderFailures pins that a driver built on the shared
// provider HTTP client retries transient failures according to
// common.LLMMaxRetries and still reports the provider's status when it gives up.
func TestDriverCallsRetryProviderFailures(t *testing.T) {
	withSSRFBypass(t)

	const okBody = `{"id":"r1","choices":[{"message":{"role":"assistant","content":"hi"}}]}`
	tests := []struct {
		name         string
		retries      int
		failStatus   int
		failures     int32
		wantRequests int32
		wantStatus   int // 0: the call must succeed
	}{
		{"recovers after a 503", 3, 503, 1, 2, 0},
		{"exhausted retries report the last status", 1, 503, 5, 2, 503},
		{"retries 0 disables", 0, 503, 1, 1, 503},
		{"auth errors are not retried", 3, 401, 1, 1, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { _ = common.SetLLMMaxRetries(common.DefaultLLMMaxRetries) })
			if err := common.SetLLMMaxRetries(tt.retries); err != nil {
				t.Fatal(err)
			}

			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) <= tt.failures {
					http.Error(w, "boom", tt.failStatus)
					return
				}
				_, _ = w.Write([]byte(okBody))
			}))
			defer srv.Close()

			m := NewOpenAIModel(map[string]string{"default": srv.URL}, URLSuffix{Chat: "chat/completions"})
			resp, err := m.ChatWithMessages(context.Background(), "m", []Message{{Role: "user", Content: "hello"}}, &APIConfig{ApiKey: ptr("test-key")}, nil, nil)

			if got := requests.Load(); got != tt.wantRequests {
				t.Errorf("requests = %d, want %d", got, tt.wantRequests)
			}
			if tt.wantStatus == 0 {
				if err != nil || resp == nil || resp.Answer == nil || *resp.Answer != "hi" {
					t.Fatalf("resp = %+v, err = %v, want a successful answer", resp, err)
				}
				return
			}
			var statusErr *APIStatusError
			if !errors.As(err, &statusErr) || statusErr.Status != tt.wantStatus {
				t.Fatalf("err = %v, want APIStatusError with status %d", err, tt.wantStatus)
			}
			if got, want := statusErr.Attempts, int(tt.wantRequests); (want > 1 && got != want) || (want == 1 && got != 0) {
				t.Errorf("Attempts = %d, want %d", got, want)
			}
			if tt.wantRequests > 1 && !strings.Contains(err.Error(), "gave up after") {
				t.Errorf("err = %q, want it to say the call was retried", err)
			}
		})
	}
}
