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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestServerTestConfigLoginRetriesWhileServerStarts covers the bootstrap retry
// that lets the Go API suite start right after `docker compose up -d` instead
// of waiting on a separate readiness gate. Without it, the very first
// connection is refused (or answered with a proxy 5xx) and TestMain would take
// the whole suite down before the server finished booting.
func TestServerTestConfigLoginRetriesWhileServerStarts(t *testing.T) {
	const notReadyReplies = 3
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) <= notReadyReplies {
			// What a booting deployment answers: TCP accepted, work not done.
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/users":
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
		case "/api/v1/auth/login":
			w.Header().Set("Authorization", "Bearer session-token")
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
		case "/api/v1/system/tokens":
			_, _ = io.WriteString(w, `{"code":0,"data":{"token":"api-key"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	previousWait, previousPoll := bootstrapWait, bootstrapPoll
	bootstrapWait, bootstrapPoll = 5*time.Second, 10*time.Millisecond
	t.Cleanup(func() { bootstrapWait, bootstrapPoll = previousWait, previousPoll })

	config := &ServerTestConfig{HostAddress: server.URL, APIVersion: "v1", Email: "qa@example.test", Password: "password"}
	if err := config.Login(); err != nil {
		t.Fatalf("Login did not retry through the boot phase: %v", err)
	}
	if config.APIKey != "api-key" {
		t.Fatalf("APIKey = %q, want the token from /system/tokens", config.APIKey)
	}
	if got := requests.Load(); got <= notReadyReplies {
		t.Fatalf("requests = %d, want more than the %d not-ready replies", got, notReadyReplies)
	}
}

// TestServerTestConfigLoginFailsFastOnRejection makes sure the retry does not
// turn a real rejection (wrong credentials) into a five-minute wait.
func TestServerTestConfigLoginFailsFastOnRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/users":
			_, _ = io.WriteString(w, `{"code":0,"data":null}`)
		case "/api/v1/auth/login":
			_, _ = io.WriteString(w, `{"code":100,"message":"Email or password error"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	previousWait, previousPoll := bootstrapWait, bootstrapPoll
	bootstrapWait, bootstrapPoll = 5*time.Second, 10*time.Millisecond
	t.Cleanup(func() { bootstrapWait, bootstrapPoll = previousWait, previousPoll })

	config := &ServerTestConfig{HostAddress: server.URL, APIVersion: "v1", Email: "qa@example.test", Password: "wrong"}
	started := time.Now()
	err := config.Login()
	if err == nil {
		t.Fatal("Login accepted a rejected sign-in")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Login waited %s on a rejected sign-in, want a fast failure", elapsed)
	}
	if !strings.Contains(err.Error(), "Email or password error") {
		t.Fatalf("error %q does not surface the server message", err)
	}
}

// TestRESTDefaultModelsPrepared verifies that the suite's own fixture
// (prepareDefaultModels, invoked from TestMain) took effect. Without this check
// a silently failing preparation would only surface as unrelated failures in
// the model-backed tests. Entries whose provider has no credential in the
// environment are reported and skipped, because the fixture deliberately
// leaves those providers untouched (that is what keeps the model-backed tests
// on their "no default model" skip path instead of having them fail on an
// unreachable model).
//
// The embedding default is not asserted: it resolves through the TEI Builtin
// bypass, which the test runner cannot observe (COMPOSE_PROFILES/TEI_MODEL live
// in the server container's environment). The dataset and memory suites cover
// it wherever it is available.
func TestRESTDefaultModelsPrepared(t *testing.T) {
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/models/default", nil, nil))["data"])
	models := array(t, data["models"])

	configured := map[string]apiObject{}
	for _, item := range models {
		model := object(t, item)
		if modelType, _ := model["model_type"].(string); modelType != "" {
			configured[modelType] = model
		}
	}

	credentials := map[string]string{}
	for _, provider := range []string{defaultChatProvider, defaultRerankProvider} {
		credentials[provider] = providerCredential(provider)
	}

	checked := 0
	for _, selection := range defaultModelSelections() {
		modelType, _ := selection["model_type"].(string)
		provider, _ := selection["model_provider"].(string)
		expected, ok := credentials[provider]
		if !ok {
			continue // Builtin embedding: covered by the dataset/memory suites.
		}
		if expected == "" {
			t.Logf("no %s credential in the environment; the default %s model is not asserted", provider, modelType)
			continue
		}
		model, exists := configured[modelType]
		if !exists {
			t.Fatalf("no default %s model configured although %s has a credential: %v", modelType, provider, models)
		}
		for _, key := range []string{"model_provider", "model_instance", "model_name"} {
			if model[key] != selection[key] {
				t.Fatalf("default %s %s = %v, want %v (configured defaults: %v)", modelType, key, model[key], selection[key], models)
			}
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no model credentials configured; the suite's model-backed tests skip for the same reason")
	}
}
