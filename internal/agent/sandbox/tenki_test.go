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
	"strings"
	"testing"
	"time"

	tenkisdk "github.com/LuxorLabs/tenki-sdk-go/sandbox"
)

func TestTenkiProvider_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newTenkiProviderFromEnv()
	if p.ProviderType() != ProviderTenki {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderTenki)
	}
	langs := p.SupportedLanguages()
	want := map[string]bool{"python": true, "javascript": true}
	got := map[string]bool{}
	for _, l := range langs {
		if !want[l] {
			t.Errorf("unexpected language: %q", l)
		}
		got[l] = true
	}
	for l := range want {
		if !got[l] {
			t.Errorf("missing required language: %q", l)
		}
	}
}

// TestTenkiProvider_Defaults exercises the env-var defaults. We clear
// TENKI_* so the test is independent of the host environment. Cannot
// use t.Parallel with t.Setenv.
func TestTenkiProvider_Defaults(t *testing.T) {
	for _, k := range []string{"TENKI_IMAGE", "TENKI_TIMEOUT", "TENKI_ALLOW_OUTBOUND"} {
		t.Setenv(k, "")
	}
	p := newTenkiProviderFromEnv()
	if p.image != "" {
		t.Errorf("image = %q, want empty (SDK default image)", p.image)
	}
	if p.timeout != tenkiDefaultTimeout || p.maxLifetime != tenkiDefaultMaxLifetime {
		t.Errorf("timeouts = execution:%v lifetime:%v, want execution:%v lifetime:%v", p.timeout, p.maxLifetime, tenkiDefaultTimeout, tenkiDefaultMaxLifetime)
	}
	if p.allowOutbound {
		t.Errorf("allowOutbound = true, want false by default (network is opt-in)")
	}
}

func TestTenkiProvider_EnvOverride(t *testing.T) {
	t.Setenv("TENKI_IMAGE", "custom-image")
	t.Setenv("TENKI_TIMEOUT", "120")
	t.Setenv("TENKI_ALLOW_OUTBOUND", "true")
	p := newTenkiProviderFromEnv()
	if p.image != "custom-image" {
		t.Errorf("image = %q, want %q", p.image, "custom-image")
	}
	if p.timeout != 120*time.Second {
		t.Errorf("timeout = %v, want 120s", p.timeout)
	}
	if !p.allowOutbound {
		t.Errorf("allowOutbound = false, want true when TENKI_ALLOW_OUTBOUND=true")
	}
}

func TestTenkiProvider_ConfigUsesCanonicalKeysAndLimits(t *testing.T) {
	p := newTenkiProviderFromConfig(map[string]any{
		"api_key":            "tk_test",
		"base_url":           "https://example.test",
		"timeout":            "45",
		"max_lifetime":       "600",
		"cpu_cores":          4,
		"memory_mb":          4096,
		"disk_size_gb":       10,
		"max_output_bytes":   2048,
		"max_artifacts":      3,
		"max_artifact_bytes": 4096,
	})
	if p.apiKey != "tk_test" || p.baseURL != "https://example.test" {
		t.Fatalf("canonical config was not loaded: api_key=%q base_url=%q", p.apiKey, p.baseURL)
	}
	if p.timeout != 45*time.Second || p.maxLifetime != 10*time.Minute {
		t.Fatalf("timeouts = execution:%v lifetime:%v", p.timeout, p.maxLifetime)
	}
	if p.cpuCores != 4 || p.memoryMB != 4096 || p.diskSizeGB != 10 ||
		p.maxOutputBytes != 2048 || p.maxArtifacts != 3 || p.maxArtifactBytes != 4096 {
		t.Fatalf("resource limits were not loaded: %+v", p)
	}
	if legacy := newTenkiProviderFromConfig(map[string]any{"API_KEY": "legacy", "TIMEOUT": 7}); legacy.apiKey != "" || legacy.timeout != tenkiDefaultTimeout {
		t.Fatalf("uppercase compatibility keys must not be accepted: %+v", legacy)
	}
}

func TestTenkiProvider_OutputCapCountsRawBytes(t *testing.T) {
	if err := validateTenkiOutputSize("é", "x", 2); err == nil {
		t.Fatal("expected UTF-8 output to be rejected by byte cap")
	}
	if err := validateTenkiOutputSize("ok", "x", 3); err != nil {
		t.Fatalf("unexpected output-cap error: %v", err)
	}
}

// TestTenkiProvider_Initialize_MissingCreds verifies the provider
// refuses to initialize when TENKI_API_KEY is unset.
func TestTenkiProvider_Initialize_MissingCreds(t *testing.T) {
	t.Setenv("TENKI_API_KEY", "")
	p := newTenkiProviderFromEnv()
	err := p.Initialize(context.Background())
	if err == nil {
		t.Fatalf("Initialize with no creds: got nil error, want one")
	}
	if !strings.Contains(err.Error(), "TENKI_API_KEY") {
		t.Errorf("err = %v, want to mention TENKI_API_KEY", err)
	}
}

// TestTenkiProvider_AllOps_BeforeInit verifies the "not initialized"
// guard is in place for every operational method.
func TestTenkiProvider_AllOps_BeforeInit(t *testing.T) {
	t.Parallel()
	p := newTenkiProviderFromEnv()
	// Do NOT call Initialize.

	inst := &SandboxInstance{InstanceID: "x", Provider: ProviderTenki}
	if _, err := p.CreateInstance(t.Context(), "python"); err == nil {
		t.Errorf("CreateInstance before init: got nil error, want one")
	}
	if _, err := p.ExecuteCode(t.Context(), inst, "x", "python", 5, nil); err == nil {
		t.Errorf("ExecuteCode before init: got nil error, want one")
	}
	if err := p.DestroyInstance(t.Context(), inst); err == nil {
		t.Errorf("DestroyInstance before init: got nil error, want one")
	}
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Errorf("HealthCheck before init: got nil error, want one")
	}
}

func TestTenkiProvider_ExecuteCode_RejectsBadInputs(t *testing.T) {
	t.Parallel()
	p := newTenkiProviderFromEnv()
	// Force "initialized" without building the SDK client so we can
	// test the input-validation paths without a network call.
	p.initialized = true

	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "empty instance id",
			fn: func() error {
				_, err := p.ExecuteCode(t.Context(),
					&SandboxInstance{InstanceID: ""}, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "nil instance",
			fn: func() error {
				_, err := p.ExecuteCode(t.Context(),
					nil, "x", "python", 5, nil)
				return err
			},
			want: "instance id",
		},
		{
			name: "unsupported language",
			fn: func() error {
				_, err := p.ExecuteCode(t.Context(),
					&SandboxInstance{InstanceID: "x"}, "x", "ruby", 5, nil)
				return err
			},
			want: "unsupported language",
		},
		{
			name: "timeout too small",
			fn: func() error {
				_, err := p.ExecuteCode(t.Context(),
					&SandboxInstance{InstanceID: "x"}, "x", "python", -1, nil)
				return err
			},
			want: "timeout",
		},
		{
			name: "timeout too large",
			fn: func() error {
				_, err := p.ExecuteCode(t.Context(),
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

func TestTenkiProvider_CreateInstance_UnsupportedLanguage(t *testing.T) {
	t.Parallel()
	p := newTenkiProviderFromEnv()
	p.initialized = true
	if _, err := p.CreateInstance(t.Context(), "ruby"); err == nil {
		t.Errorf("CreateInstance(ruby): got nil error, want one")
	}
}

func TestTenkiProvider_DestroyInstance_EmptyID(t *testing.T) {
	t.Parallel()
	p := newTenkiProviderFromEnv()
	p.initialized = true
	if err := p.DestroyInstance(t.Context(), &SandboxInstance{InstanceID: ""}); err == nil {
		t.Errorf("DestroyInstance(empty id): got nil error, want one")
	}
	if err := p.DestroyInstance(t.Context(), nil); err == nil {
		t.Errorf("DestroyInstance(nil): got nil error, want one")
	}
}

// TestTenkiProvider_BuildTenkiExecutionResult unit-tests the
// result-mapping helper, including the marker-extraction path, using
// a hand-built SDK Result (no network).
func TestTenkiProvider_BuildTenkiExecutionResult(t *testing.T) {
	t.Parallel()
	r := &tenkisdk.Result{
		Stdout:   []byte("hello\n"),
		Stderr:   []byte(""),
		ExitCode: 0,
		Status:   tenkisdk.CommandStatusSucceeded,
	}
	res := buildTenkiExecutionResult(r, "python", time.Now())
	if res == nil {
		t.Fatalf("buildTenkiExecutionResult returned nil")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if res.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", res.Stdout, "hello\n")
	}
	if v, _ := res.Metadata["language"].(string); v != "python" {
		t.Errorf("Metadata[language] = %v, want python", res.Metadata["language"])
	}
}

// TestTenkiProvider_ProviderType_StaysDistinct ensures the tenki
// provider does not collide on the wire with the other providers.
func TestTenkiProvider_ProviderType_StaysDistinct(t *testing.T) {
	t.Parallel()
	seen := map[ProviderType]bool{}
	for _, p := range []SandboxProvider{
		newSelfManagedProviderFromEnv(),
		newAliyunProviderFromEnv(),
		newE2BProviderFromEnv(),
		newTenkiProviderFromEnv(),
	} {
		if seen[p.ProviderType()] {
			t.Errorf("provider type %q seen twice", p.ProviderType())
		}
		seen[p.ProviderType()] = true
	}
}
