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
	"context"
	"errors"
	"os"
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
	if p.sandboxTimeout != 120*time.Second {
		t.Errorf("sandboxTimeout = %v, want 120s", p.sandboxTimeout)
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
	err := p.Initialize(context.Background())
	if err == nil {
		t.Fatalf("Initialize with no creds: got nil error, want one")
	}
	if !strings.Contains(err.Error(), "E2B_API_KEY") {
		t.Errorf("err = %v, want to mention E2B_API_KEY", err)
	}
}

// TestE2BProvider_Initialize_WithAPIKey exercises the success
// path. This calls the e2b control plane; we skip when no API
// key is set (CI without secrets). The skip is loud via a clear
// log line so missing-secrets is visible in test output.
func TestE2BProvider_Initialize_WithAPIKey(t *testing.T) {
	apiKey := os.Getenv("E2B_API_KEY")
	if apiKey == "" {
		t.Skip("E2B_API_KEY not set — skipping network-dependent init check")
	}
	p := newE2BProviderFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !p.isInitialized() {
		t.Errorf("provider not flagged initialized after successful build")
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

	inst := &SandboxInstance{InstanceID: "x", Provider: ProviderE2B}
	if _, err := p.CreateInstance(context.Background(), "python"); err == nil {
		t.Errorf("CreateInstance before init: got nil error, want one")
	}
	if _, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil); err == nil {
		t.Errorf("ExecuteCode before init: got nil error, want one")
	}
	if err := p.DestroyInstance(context.Background(), inst); err == nil {
		t.Errorf("DestroyInstance before init: got nil error, want one")
	}
	if err := p.HealthCheck(context.Background()); err == nil {
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

	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "empty instance id",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: ""}, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "nil instance",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					nil, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "unsupported language",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: "x"}, "x", "ruby", 5, nil)
				return err
			},
			want: "unsupported language",
		},
		{
			name: "timeout too small",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
					&SandboxInstance{InstanceID: "x"}, "x", "python", 0, nil)
				return err
			},
			want: "timeout",
		},
		{
			name: "timeout too large",
			fn: func() error {
				_, err := p.ExecuteCode(context.Background(),
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
	if _, err := p.CreateInstance(context.Background(), "ruby"); err == nil {
		t.Errorf("CreateInstance(ruby): got nil error, want one")
	}
}

func TestE2BProvider_DestroyInstance_EmptyID(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromEnv()
	p.initialized = true
	if err := p.DestroyInstance(context.Background(), &SandboxInstance{InstanceID: ""}); err == nil {
		t.Errorf("DestroyInstance(empty id): got nil error, want one")
	}
	if err := p.DestroyInstance(context.Background(), nil); err == nil {
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

// TestE2BProvider_FullE2E_SkipWithoutKey is the integration
// test path. The body is skipped unless E2B_API_KEY is set, but
// the test always runs (so missing-secrets shows up in CI logs).
// When enabled, it creates a real sandbox, runs Python, kills it.
func TestE2BProvider_FullE2E_SkipWithoutKey(t *testing.T) {
	apiKey := os.Getenv("E2B_API_KEY")
	if apiKey == "" {
		t.Skip("E2B_API_KEY not set — skipping full E2E test (real network call)")
	}
	p := newE2BProviderFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := p.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	inst, err := p.CreateInstance(ctx, "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer func() { _ = p.DestroyInstance(ctx, inst) }()

	result, err := p.ExecuteCode(ctx, inst, "def main(): return 1+1", "python", 30, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0; stderr=%q", result.ExitCode, result.Stderr)
	}
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

// TestE2BProvider_AccessTokenFallback verifies Initialize
// accepts E2B_ACCESS_TOKEN as an alternative to E2B_API_KEY.
func TestE2BProvider_AccessTokenFallback(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	t.Setenv("E2B_ACCESS_TOKEN", "fake-token")
	p := newE2BProviderFromEnv()
	// We can't run a real e2b call here (no real token), but
	// Initialize should NOT fail with "E2B_API_KEY or
	// E2B_ACCESS_TOKEN is required". The error we'd see is the
	// SDK's auth error, which is what we want.
	err := p.Initialize(context.Background())
	if err == nil {
		t.Skip("Initialize succeeded — env-var fallback accepted; skipping further checks")
	}
	if errors.Is(err, errors.New("")) { // placeholder
		t.Errorf("unexpected error type: %v", err)
	}
	if strings.Contains(err.Error(), "E2B_API_KEY or E2B_ACCESS_TOKEN env var is required") {
		t.Errorf("Initialize still rejected the access token: %v", err)
	}
}
