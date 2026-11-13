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

package sandbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	e2bsdk "github.com/eric642/e2b-go-sdk"
)

func TestE2BProvider_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromEnv()
	if p.ProviderType() != ProviderE2B {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderE2B)
	}
	langs := p.SupportedLanguages()
	want := map[string]bool{"python": true, "nodejs": true, "javascript": true}
	for _, l := range langs {
		if !want[l] {
			t.Errorf("unexpected language: %q", l)
		}
	}
}

// TestE2BProvider_Defaults exercises the env-var defaults. We
// capture and clear E2B_* so the test is independent of the host
// environment. Cannot use t.Parallel with t.Setenv.
func TestE2BProvider_Defaults(t *testing.T) {
	for _, k := range []string{"E2B_TEMPLATE", "E2B_TIMEOUT"} {
		t.Setenv(k, "")
	}
	p := newE2BProviderFromEnv()
	if p.template != e2bDefaultTemplate {
		t.Errorf("template = %q, want %q", p.template, e2bDefaultTemplate)
	}
	if p.sandboxTimeout != e2bDefaultSandboxTimeout {
		t.Errorf("sandboxTimeout = %v, want %v", p.sandboxTimeout, e2bDefaultSandboxTimeout)
	}
}

func TestE2BProvider_EnvOverride(t *testing.T) {
	t.Setenv("E2B_TEMPLATE", "custom-template")
	t.Setenv("E2B_TIMEOUT", "120")
	p := newE2BProviderFromEnv()
	if p.template != "custom-template" {
		t.Errorf("template = %q, want %q", p.template, "custom-template")
	}
	if p.requestTimeout != 120*time.Second {
		t.Errorf("requestTimeout = %v, want 120s", p.requestTimeout)
	}
	if p.sandboxTimeout != e2bDefaultSandboxTimeout {
		t.Errorf("sandboxTimeout = %v, want %v", p.sandboxTimeout, e2bDefaultSandboxTimeout)
	}
}

func TestE2BProvider_ConfigUsesSavedSettings(t *testing.T) {
	p := newE2BProviderFromConfig(map[string]any{
		"api_key":  "saved-key",
		"region":   "us",
		"template": "saved-template",
		"timeout":  42,
	})
	if p.apiKey != "saved-key" || p.region != "us" || p.template != "saved-template" {
		t.Fatalf("saved settings not retained: %#v", p)
	}
	if p.requestTimeout != 42*time.Second {
		t.Fatalf("requestTimeout = %v, want 42s", p.requestTimeout)
	}
}

func TestE2BProvider_SavedConfigDrivesSDKCreateRequest(t *testing.T) {
	t.Setenv("E2B_API_KEY", "env-key")
	t.Setenv("E2B_ACCESS_TOKEN", "")
	t.Setenv("E2B_DOMAIN", "env.example.test")

	var body map[string]any
	var apiKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/sandboxes/sbx-1" {
			_, _ = w.Write([]byte(`{"sandboxID":"sbx-1","templateID":"configured-template","envdVersion":"0.4.0","clientID":"client","cpuCount":1,"memoryMB":512,"diskSizeMB":1024,"state":"running","startedAt":"2026-01-01T00:00:00Z","endAt":"2026-01-01T00:05:00Z"}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/sandboxes" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		apiKey = r.Header.Get("X-API-Key")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sandboxID":"sbx-1","templateID":"configured-template","envdVersion":"0.4.0","clientID":"client"}`))
	}))
	defer server.Close()
	t.Setenv("E2B_API_URL", server.URL)

	p := newE2BProviderFromConfig(map[string]any{
		"api_key":         "saved-key",
		"region":          "us",
		"domain":          "configured.example.test",
		"template":        "configured-template",
		"timeout":         7,
		"sandbox_timeout": 123,
	})
	if err := p.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	inst, err := p.CreateInstance(t.Context(), "python")
	if err != nil {
		t.Fatal(err)
	}
	if inst.InstanceID != "sbx-1" {
		t.Fatalf("instance = %q", inst.InstanceID)
	}
	if apiKey != "saved-key" {
		t.Fatalf("X-API-Key = %q, want saved-key", apiKey)
	}
	if body["templateID"] != "configured-template" {
		t.Fatalf("templateID = %v, body=%v", body["templateID"], body)
	}
	if body["timeout"] != float64(123) {
		t.Fatalf("timeout = %v, body=%v", body["timeout"], body)
	}
	cfg := p.client.Config()
	if cfg.Domain != "configured.example.test" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
	if cfg.RequestTimeout != 7*time.Second {
		t.Fatalf("request timeout = %v", cfg.RequestTimeout)
	}
}

func TestE2BProvider_EURequiresExplicitDomain(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	t.Setenv("E2B_ACCESS_TOKEN", "")
	t.Setenv("E2B_DOMAIN", "")
	p := newE2BProviderFromConfig(map[string]any{"api_key": "saved-key", "region": "eu"})
	if err := p.Initialize(t.Context()); err == nil || !strings.Contains(err.Error(), "domain") {
		t.Fatalf("Initialize = %v, want missing EU domain error", err)
	}
}

func TestE2BProvider_EUUsesConfiguredDomain(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	t.Setenv("E2B_ACCESS_TOKEN", "")
	t.Setenv("E2B_DOMAIN", "")
	p := newE2BProviderFromConfig(map[string]any{
		"api_key": "saved-key",
		"region":  "eu",
		"domain":  "eu.example.test",
	})
	if err := p.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize = %v", err)
	}
}

func TestE2BProvider_RejectsUnknownRegion(t *testing.T) {
	p := newE2BProviderFromConfig(map[string]any{"api_key": "saved-key", "region": "mars"})
	if err := p.Initialize(t.Context()); err == nil || !strings.Contains(err.Error(), "unsupported region") {
		t.Fatalf("Initialize = %v, want unsupported region error", err)
	}
}

// TestE2BProvider_Initialize_MissingCreds verifies the provider
// refuses to initialize when neither E2B_API_KEY nor
// E2B_ACCESS_TOKEN is set. This replaces the v2 loud-fail sentinel
// path: the error now comes from Initialize, not from every op.
func TestE2BProvider_Initialize_MissingCreds(t *testing.T) {
	for _, k := range []string{"E2B_API_KEY", "E2B_ACCESS_TOKEN"} {
		t.Setenv(k, "")
	}
	p := newE2BProviderFromEnv()
	err := p.Initialize(t.Context())
	if err == nil {
		t.Fatalf("Initialize with no creds: got nil error, want one")
	}
	if !strings.Contains(err.Error(), "E2B_API_KEY") {
		t.Errorf("err = %v, want to mention E2B_API_KEY", err)
	}
}

// TestE2BProvider_AllOps_BeforeInit verifies the "not initialized"
// guard is in place for every operational method. The
// initialization order is "build client" → "set initialized
// true". Until the second step happens, the provider must
// refuse all ops with a clear error.
func TestE2BProvider_AllOps_BeforeInit(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromEnv()
	// Do NOT call Initialize.
	ctx := t.Context()

	inst := &SandboxInstance{InstanceID: "x", Provider: ProviderE2B}
	if _, err := p.CreateInstance(ctx, "python"); err == nil {
		t.Errorf("CreateInstance before init: got nil error, want one")
	}
	if _, err := p.ExecuteCode(ctx, inst, "x", "python", 5, nil); err == nil {
		t.Errorf("ExecuteCode before init: got nil error, want one")
	}
	if err := p.DestroyInstance(ctx, inst); err == nil {
		t.Errorf("DestroyInstance before init: got nil error, want one")
	}
	if err := p.HealthCheck(ctx); err == nil {
		t.Errorf("HealthCheck before init: got nil error, want one")
	}
}

func TestE2BProvider_ExecuteCode_RejectsBadInputs(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromEnv()
	// Force "initialized" without actually building the SDK client
	// — this lets us test the input-validation paths without
	// hitting the e2b control plane.
	p.initialized = true
	ctx := t.Context()

	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "empty instance id",
			fn: func() error {
				_, err := p.ExecuteCode(ctx,
					&SandboxInstance{InstanceID: ""}, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "nil instance",
			fn: func() error {
				_, err := p.ExecuteCode(ctx,
					nil, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "unsupported language",
			fn: func() error {
				_, err := p.ExecuteCode(ctx,
					&SandboxInstance{InstanceID: "x"}, "x", "ruby", 5, nil)
				return err
			},
			want: "unsupported language",
		},
		{
			name: "timeout too small",
			fn: func() error {
				_, err := p.ExecuteCode(ctx,
					&SandboxInstance{InstanceID: "x"}, "x", "python", 0, nil)
				return err
			},
			want: "timeout",
		},
		{
			name: "timeout too large",
			fn: func() error {
				_, err := p.ExecuteCode(ctx,
					&SandboxInstance{InstanceID: "x"}, "x", "python", 1000, nil)
				return err
			},
			want: "timeout",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatalf("got nil error, want one containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want to contain %q", err, tc.want)
			}
		})
	}
}

func TestE2BProvider_CreateInstance_UnsupportedLanguage(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromEnv()
	p.initialized = true
	ctx := t.Context()
	if _, err := p.CreateInstance(ctx, "ruby"); err == nil {
		t.Errorf("CreateInstance(ruby): got nil error, want one")
	}
}

func TestE2BProvider_DestroyInstance_EmptyID(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromEnv()
	p.initialized = true
	ctx := t.Context()
	if err := p.DestroyInstance(ctx, &SandboxInstance{InstanceID: ""}); err == nil {
		t.Errorf("DestroyInstance(empty id): got nil error, want one")
	}
	if err := p.DestroyInstance(ctx, nil); err == nil {
		t.Errorf("DestroyInstance(nil): got nil error, want one")
	}
}

// TestE2BProvider_BuildE2BExecutionResult is a small unit test
// for the result-mapping helper. It exercises the marker
// extraction path that the real execute flow uses.
func TestE2BProvider_BuildE2BExecutionResult(t *testing.T) {
	t.Parallel()
	// The helper takes a *e2b.CommandResult pointer; we build a
	// zero-value struct on the heap.
	cmdResult := makeFakeCommandResult("hello\n")
	res := buildE2BExecutionResult(cmdResult, "python", time.Now())
	if res == nil {
		t.Fatalf("buildE2BExecutionResult returned nil")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", res.Stdout, "hello\n")
	}
	if res.Metadata == nil {
		t.Errorf("Metadata is nil, want a map with language + structured_result")
	}
	if v, _ := res.Metadata["language"].(string); v != "python" {
		t.Errorf("Metadata[language] = %v, want python", res.Metadata["language"])
	}
}

// makeFakeCommandResult returns a *e2bsdk.CommandResult with the
// given stdout. The other fields are zero. This lets the test
// buildE2BExecutionResult test the mapping without spinning up
// the e2b SDK.
func makeFakeCommandResult(stdout string) *e2bsdk.CommandResult {
	return &e2bsdk.CommandResult{Stdout: stdout}
}

// TestE2BProvider_ProviderType_StaysDistinct ensures the
// three providers do not collide on the wire. The test would
// catch any future refactor that aliases them.
func TestE2BProvider_ProviderType_StaysDistinct(t *testing.T) {
	t.Parallel()
	seen := map[ProviderType]bool{}
	for _, p := range []SandboxProvider{
		newSelfManagedProviderFromEnv(),
		newAliyunProviderFromEnv(),
		newE2BProviderFromEnv(),
	} {
		if seen[p.ProviderType()] {
			t.Errorf("provider type %q seen twice", p.ProviderType())
		}
		seen[p.ProviderType()] = true
	}
}
