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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

// newNaceForTest creates a NaceModel instance for testing.
func newNaceForTest(baseURL string) *NaceModel {
	return NewNaceModel(
		map[string]string{"default": baseURL},
		URLSuffix{Decision: "systemone", Models: "models"},
	)
}

// TestNaceName tests the driver name.
func TestNaceName(t *testing.T) {
	if got := newNaceForTest("http://unused").Name(); got != "nace" {
		t.Errorf("Name()=%q", got)
	}
}

// TestNaceFactory tests creating the driver from factory.
func TestNaceFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("nace", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*NaceModel); !ok {
		t.Fatalf("driver type=%T, want *NaceModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*NaceModel); !ok {
		t.Fatal("NewInstance did not return *NaceModel")
	}
}

// The shipped config must resolve to the Nace driver and declare the System
// One decision path. CreateModelDriver dispatches on strings.ToLower(name), so
// the "Nace" provider name has to match the "nace" case; anything else silently
// falls through to DummyModel, and a missing suffix would send decide calls to
// the bare base URL.
func TestNaceProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "nace.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Nace")
	if provider == nil {
		t.Fatal("Nace provider not found")
	}
	if _, ok := provider.ModelDriver.(*NaceModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.NaceModel", provider.ModelDriver)
	}
	if provider.URLSuffix.Decision != "systemone" {
		t.Errorf("url_suffix.decision=%q, want systemone", provider.URLSuffix.Decision)
	}
	names := make([]string, 0, len(provider.Models))
	for _, model := range provider.Models {
		names = append(names, model.Name)
		if !model.ModelTypeMap["decision"] {
			t.Errorf("%s is not a decision model: %v", model.Name, model.ModelTypes)
		}
	}
	if len(names) != 2 || names[0] != "drex-max" || names[1] != "drex-v2.0" {
		t.Errorf("models=%v, want [drex-max drex-v2.0]", names)
	}
}

// TestNaceListModelsSkipsAliases tests model listing against Nace's own
// catalog shape, and that names which only point to another model are not
// offered.
func TestNaceListModelsSkipsAliases(t *testing.T) {
	withSSRFBypass(t)
	dir, restore := setupProviderTestDir(t, "nace.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "drex-v2.0", "release_date": "2026-10-06", "alias_for": nil},
				{"name": "drex-max", "release_date": "2026-10-13", "alias_for": nil},
				{"name": "drex-latest", "release_date": "2026-10-06", "alias_for": "drex-v2.0"},
				{"name": "drex-v1.0", "release_date": "2026-09-24", "alias_for": "drex-max"},
			},
			"request_id": "req_list",
		})
	}))
	defer srv.Close()

	model := newNaceForTest(srv.URL + "/v1")
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "drex-v2.0,drex-max" {
		t.Errorf("models=%v", list)
	}
	for _, item := range list {
		if len(item.ModelTypes) != 1 || item.ModelTypes[0] != "decision" {
			t.Errorf("%s types=%v, want [decision]", item.Name, item.ModelTypes)
		}
		if item.ContextLength == nil || *item.ContextLength != 262144 {
			t.Errorf("%s context_length=%v, want 262144", item.Name, item.ContextLength)
		}
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// A rejected key must fail the listing and the connection check rather than
// offer models that cannot be called.
func TestNaceListModelsFailsOnBadKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"invalid api key"},"request_id":"req_bad"}`))
	}))
	defer srv.Close()

	model := newNaceForTest(srv.URL + "/v1")
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err == nil {
		t.Fatalf("ListModels returned %v, want an error", list)
	}
	if list != nil {
		t.Errorf("models=%v, want nil on failure", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil {
		t.Fatal("CheckConnection did not fail")
	}
}

// TestNaceDecide tests the decide call with the payload Categorize sends and
// the response Nace returns, whose request id is named request_id.
func TestNaceDecide(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		var request struct {
			Model     string                                         `json:"model"`
			State     map[string]string                              `json:"state"`
			Questions map[string]struct{ Type, Instructions string } `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "drex-max" || request.State["query"] != "my printer will not turn on" || len(request.Questions) != 2 || request.Questions["category_0"].Type != "noul" || request.Questions["category_0"].Instructions != "Is \"Hardware fault\" the single best category for the current query in the state?" {
			t.Errorf("incorrect decision payload: %+v", request)
		}
		_, _ = w.Write([]byte(`{"model":"drex-max","answers":{"category_0":{"type":"noul","noul":0.918},"category_1":{"type":"noul","noul":0.0015}},"usage":{"input_tokens":208,"output_tokens":75},"evaluation_time_ms":79.5,"request_id":"req_6861473053f6606cda19ed4affd42b81"}`))
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	name := "drex-max"
	result, err := newNaceForTest(srv.URL+"/v1").Decide(t.Context(), &name, &DecideRequest{
		State: map[string]string{"query": "my printer will not turn on", "conversation": ""},
		Questions: map[string]Question{
			"category_0": Noul{Instructions: "Is \"Hardware fault\" the single best category for the current query in the state?"},
			"category_1": Noul{Instructions: "Is \"Billing question\" the single best category for the current query in the state?"},
		},
	}, &APIConfig{ApiKey: &testAPIKey}, usage)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if got := result.Answers["category_0"]; got.Noul == nil || *got.Noul != 0.918 {
		t.Errorf("category_0=%+v", got)
	}
	if got := result.Answers["category_1"]; got.Noul == nil || *got.Noul != 0.0015 {
		t.Errorf("category_1=%+v", got)
	}
	assertModelUsage(t, usage, 208, 75, 283)
	if usage.RequestID != "req_6861473053f6606cda19ed4affd42b81" {
		t.Errorf("RequestID=%q", usage.RequestID)
	}
}

// Nace serves no chat completions; the slot is the decision one.
func TestNaceRefusesChat(t *testing.T) {
	if _, err := newNaceForTest("http://unused").ChatWithMessages(
		context.Background(), "drex-max", nil, nil, nil, nil,
	); err == nil {
		t.Fatal("ChatWithMessages did not refuse")
	}
}

// The Authorization header must not follow a redirect to another origin, even
// to the same hostname on another port or scheme, which net/http would allow.
func TestNaceDoesNotForwardKeyOnCrossOriginRedirect(t *testing.T) {
	withSSRFBypass(t)
	var leaked int
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked++
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer other.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v1/models", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	model := newNaceForTest(origin.URL + "/v1")
	if _, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil {
		t.Fatal("ListModels followed a cross-origin redirect")
	}
	if leaked != 0 {
		t.Fatalf("Authorization reached the redirect target %d time(s)", leaked)
	}
}

func TestNaceFollowsSameOriginRedirect(t *testing.T) {
	withSSRFBypass(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v1/models2", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/v1/models2", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("same-origin redirect dropped Authorization")
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"drex-max"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	list, err := newNaceForTest(srv.URL+"/v1").ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil || len(list) != 1 {
		t.Fatalf("ListModels = %v, %v", list, err)
	}
}

func TestSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"https://console.nace.ai", "https://console.nace.ai/v1", true},
		{"https://console.nace.ai", "https://CONSOLE.nace.ai:443/x", true},
		{"https://console.nace.ai", "http://console.nace.ai", false},
		{"https://console.nace.ai", "https://console.nace.ai:8443", false},
		{"https://console.nace.ai", "https://nace.ai", false},
		{"", "https://console.nace.ai", false},
	} {
		if got := SameOrigin(tc.a, tc.b); got != tc.want {
			t.Errorf("SameOrigin(%q, %q)=%t, want %t", tc.a, tc.b, got, tc.want)
		}
	}
}
