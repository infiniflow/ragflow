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

package service

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestExchangeGoogleWebOAuthCodeRejectsCustomTokenURI(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"unexpected"}`))
	}))
	defer server.Close()

	clientConfig := map[string]interface{}{
		"web": map[string]interface{}{
			"client_id":     "client-id",
			"client_secret": "client-secret",
			"token_uri":     server.URL,
		},
	}
	_, err := exchangeGoogleWebOAuthCode(clientConfig, nil, "http://localhost/callback", "code", "verifier")
	if err == nil {
		t.Fatal("exchangeGoogleWebOAuthCode accepted a custom token URI")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("custom token URI received %d requests, want none", got)
	}
}
