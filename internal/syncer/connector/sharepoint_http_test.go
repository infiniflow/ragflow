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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharePointRetriesTruncatedBody(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovers", true: "exhausted"}[persistent], func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				w.Header().Set("Content-Length", "8")
				w.Header().Set("Retry-After", "0")
				if persistent || call == 1 {
					w.Write([]byte("bad"))
					return
				}
				w.Write([]byte("complete"))
			}))
			defer server.Close()
			c := mustSharePointConnector(t, server.URL, nil)
			c.accessToken, c.tokenExpiry = "token", time.Now().Add(time.Hour)
			body, err := c.doGraphRequest(t.Context(), server.URL+"/v1.0/drives/drive/items/file/content", 1024)
			if persistent {
				if !errors.Is(err, io.ErrUnexpectedEOF) || calls.Load() != sharepointRetryCount || len(body) != 0 {
					t.Fatalf("body=%q error=%v attempts=%d", body, err, calls.Load())
				}
				if !strings.Contains(err.Error(), "/items/file/content") {
					t.Fatalf("missing request identity: %v", err)
				}
			} else if err != nil || string(body) != "complete" || calls.Load() != 2 {
				t.Fatalf("body=%q error=%v attempts=%d", body, err, calls.Load())
			}
		})
	}
}

func TestSharePointRequestErrorRedactsURL(t *testing.T) {
	err := sharePointRequestError("https://graph.microsoft.com/v1.0/drives/d/items/f/content?secret=hidden",
		&url.Error{Op: "Get", URL: "https://download.example/file?token=private", Err: io.ErrUnexpectedEOF})
	if !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "/items/f/content") {
		t.Fatalf("lost error or identity: %v", err)
	}
	for _, secret := range []string{"hidden", "private", "download.example", "?"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("URL data leaked: %v", err)
		}
	}
}
