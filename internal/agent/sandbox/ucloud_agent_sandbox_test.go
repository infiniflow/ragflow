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
)

func TestUCloudAgentSandboxProvider_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newUCloudAgentSandboxProviderFromConfig(map[string]any{})
	if p.ProviderType() != ProviderUCloudAgentSandbox {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderUCloudAgentSandbox)
	}

	want := map[string]bool{"python": true, "javascript": true}
	got := map[string]bool{}
	for _, language := range p.SupportedLanguages() {
		if !want[language] {
			t.Errorf("unexpected language: %q", language)
		}
		got[language] = true
	}
	for language := range want {
		if !got[language] {
			t.Errorf("missing required language: %q", language)
		}
	}
}

func TestUCloudAgentSandboxProvider_Defaults(t *testing.T) {
	t.Parallel()
	p := newUCloudAgentSandboxProviderFromConfig(map[string]any{})

	if p.region != ucloudAgentSandboxDefaultRegion {
		t.Errorf("region = %q, want %q", p.region, ucloudAgentSandboxDefaultRegion)
	}
	if p.template != ucloudAgentSandboxDefaultTemplate {
		t.Errorf("template = %q, want %q", p.template, ucloudAgentSandboxDefaultTemplate)
	}
	if p.allowInternetAccess {
		t.Error("allowInternetAccess = true, want false by default")
	}
	if p.insecureHTTP {
		t.Error("insecureHTTP = true, want false by default")
	}
	if p.timeoutSec != ucloudAgentSandboxDefaultTimeout {
		t.Errorf("timeoutSec = %d, want %d", p.timeoutSec, ucloudAgentSandboxDefaultTimeout)
	}
	if p.sandboxTimeoutSec != ucloudAgentSandboxDefaultLifetime {
		t.Errorf("sandboxTimeoutSec = %d, want %d", p.sandboxTimeoutSec, ucloudAgentSandboxDefaultLifetime)
	}
	if p.maxOutputBytes != ucloudAgentSandboxDefaultMaxOutputBytes {
		t.Errorf("maxOutputBytes = %d, want %d", p.maxOutputBytes, ucloudAgentSandboxDefaultMaxOutputBytes)
	}
	if p.maxArtifacts != ucloudAgentSandboxDefaultMaxArtifacts {
		t.Errorf("maxArtifacts = %d, want %d", p.maxArtifacts, ucloudAgentSandboxDefaultMaxArtifacts)
	}
	if p.maxArtifactBytes != ucloudAgentSandboxDefaultMaxArtifactSize {
		t.Errorf("maxArtifactBytes = %d, want %d", p.maxArtifactBytes, ucloudAgentSandboxDefaultMaxArtifactSize)
	}
}

func TestUCloudAgentSandboxProvider_ConfigOverrides(t *testing.T) {
	t.Parallel()
	p := newUCloudAgentSandboxProviderFromConfig(map[string]any{
		"api_key":               "ucloud-key",
		"region":                "us-ca",
		"domain":                "sandbox.example.com",
		"api_url":               "https://api.example.com",
		"template":              "custom-template",
		"allow_internet_access": true,
		"insecure_http":         true,
		"timeout":               float64(45),
		"sandbox_timeout":       float64(600),
		"max_output_bytes":      float64(2_000_000),
		"max_artifacts":         float64(50),
		"max_artifact_bytes":    float64(20_000_000),
	})

	if p.apiKey != "ucloud-key" {
		t.Errorf("apiKey = %q", p.apiKey)
	}
	if p.region != "us-ca" || p.domain != "sandbox.example.com" || p.apiURL != "https://api.example.com" {
		t.Errorf("endpoint config = region:%q domain:%q apiURL:%q", p.region, p.domain, p.apiURL)
	}
	if p.template != "custom-template" {
		t.Errorf("template = %q", p.template)
	}
	if !p.allowInternetAccess || !p.insecureHTTP {
		t.Errorf("boolean overrides not applied: internet=%v insecureHTTP=%v", p.allowInternetAccess, p.insecureHTTP)
	}
	if p.timeoutSec != 45 || p.sandboxTimeoutSec != 600 {
		t.Errorf("timeouts = execution:%d sandbox:%d", p.timeoutSec, p.sandboxTimeoutSec)
	}
	if p.maxOutputBytes != 2_000_000 || p.maxArtifacts != 50 || p.maxArtifactBytes != 20_000_000 {
		t.Errorf("limits = output:%d artifacts:%d artifactBytes:%d", p.maxOutputBytes, p.maxArtifacts, p.maxArtifactBytes)
	}
}

func TestUCloudAgentSandboxProvider_EnvOverrides(t *testing.T) {
	t.Setenv("UCLOUD_SANDBOX_API_KEY", "env-key")
	t.Setenv("UCLOUD_SANDBOX_REGION", "us-ca")
	t.Setenv("UCLOUD_SANDBOX_TEMPLATE", "env-template")
	t.Setenv("UCLOUD_SANDBOX_ALLOW_INTERNET_ACCESS", "true")
	t.Setenv("UCLOUD_SANDBOX_INSECURE_HTTP", "true")
	t.Setenv("UCLOUD_SANDBOX_EXECUTION_TIMEOUT", "60")
	t.Setenv("UCLOUD_SANDBOX_TIMEOUT", "900")
	t.Setenv("UCLOUD_SANDBOX_MAX_OUTPUT_BYTES", "2000")
	t.Setenv("UCLOUD_SANDBOX_MAX_ARTIFACTS", "4")
	t.Setenv("UCLOUD_SANDBOX_MAX_ARTIFACT_BYTES", "3000")

	p := newUCloudAgentSandboxProviderFromEnv()
	if p.apiKey != "env-key" || p.region != "us-ca" || p.template != "env-template" {
		t.Errorf("env string overrides not applied: key:%q region:%q template:%q", p.apiKey, p.region, p.template)
	}
	if !p.allowInternetAccess || !p.insecureHTTP {
		t.Errorf("env boolean overrides not applied: internet=%v insecureHTTP=%v", p.allowInternetAccess, p.insecureHTTP)
	}
	if p.timeoutSec != 60 || p.sandboxTimeoutSec != 900 {
		t.Errorf("env timeouts = execution:%d sandbox:%d", p.timeoutSec, p.sandboxTimeoutSec)
	}
	if p.maxOutputBytes != 2000 || p.maxArtifacts != 4 || p.maxArtifactBytes != 3000 {
		t.Errorf("env limits = output:%d artifacts:%d artifactBytes:%d", p.maxOutputBytes, p.maxArtifacts, p.maxArtifactBytes)
	}
}

func TestUCloudAgentSandboxProvider_Initialize(t *testing.T) {
	t.Parallel()

	t.Run("missing API key", func(t *testing.T) {
		p := newUCloudAgentSandboxProviderFromConfig(map[string]any{})
		err := p.Initialize(context.Background())
		if err == nil {
			t.Fatal("Initialize with no API key: got nil error, want one")
		}
		if !strings.Contains(err.Error(), "API key") {
			t.Errorf("err = %v, want to mention API key", err)
		}
	})

	t.Run("valid local configuration", func(t *testing.T) {
		p := newUCloudAgentSandboxProviderFromConfig(map[string]any{"api_key": "fake-key"})
		if err := p.Initialize(context.Background()); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		if !p.isInitialized() || p.client == nil {
			t.Error("provider did not retain an initialized SDK client")
		}
		if err := p.HealthCheck(context.Background()); err != nil {
			t.Errorf("HealthCheck after Initialize: %v", err)
		}
	})

	t.Run("invalid limits", func(t *testing.T) {
		p := newUCloudAgentSandboxProviderFromConfig(map[string]any{
			"api_key":       "fake-key",
			"max_artifacts": -1,
		})
		if err := p.Initialize(context.Background()); err == nil {
			t.Fatal("Initialize with invalid limits: got nil error, want one")
		}
	})
}

func TestUCloudAgentSandboxProvider_AllOpsBeforeInit(t *testing.T) {
	t.Parallel()
	p := newUCloudAgentSandboxProviderFromConfig(map[string]any{})
	inst := &SandboxInstance{InstanceID: "x", Provider: ProviderUCloudAgentSandbox}

	if _, err := p.CreateInstance(t.Context(), "python"); err == nil {
		t.Error("CreateInstance before init: got nil error, want one")
	}
	if _, err := p.ExecuteCode(t.Context(), inst, "x", "python", 5, nil); err == nil {
		t.Error("ExecuteCode before init: got nil error, want one")
	}
	if err := p.DestroyInstance(t.Context(), inst); err == nil {
		t.Error("DestroyInstance before init: got nil error, want one")
	}
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Error("HealthCheck before init: got nil error, want one")
	}
}

func TestUCloudAgentSandboxProvider_ExecuteCodeRejectsBadInputs(t *testing.T) {
	t.Parallel()
	p := newUCloudAgentSandboxProviderFromConfig(map[string]any{"api_key": "fake-key"})
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	p.instances["x"] = &ucloudAgentSandboxInstance{}

	cases := []struct {
		name string
		inst *SandboxInstance
		lang string
		time int
		want string
	}{
		{name: "nil instance", inst: nil, lang: "python", time: 5, want: "instance id"},
		{name: "empty instance id", inst: &SandboxInstance{}, lang: "python", time: 5, want: "instance id"},
		{name: "unknown instance", inst: &SandboxInstance{InstanceID: "missing"}, lang: "python", time: 5, want: "unknown instance"},
		{name: "unsupported language", inst: &SandboxInstance{InstanceID: "x"}, lang: "ruby", time: 5, want: "unsupported language"},
		{name: "timeout too small", inst: &SandboxInstance{InstanceID: "x"}, lang: "python", time: 0, want: "timeout"},
		{name: "timeout too large", inst: &SandboxInstance{InstanceID: "x"}, lang: "python", time: 1000, want: "timeout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := p.ExecuteCode(t.Context(), tc.inst, "x", tc.lang, tc.time, nil)
			if err == nil {
				t.Fatalf("got nil error, want one containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want to contain %q", err, tc.want)
			}
		})
	}
}

func TestUCloudAgentSandboxProvider_DestroyInstanceRejectsEmptyID(t *testing.T) {
	t.Parallel()
	p := newUCloudAgentSandboxProviderFromConfig(map[string]any{"api_key": "fake-key"})
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := p.DestroyInstance(t.Context(), nil); err == nil {
		t.Error("DestroyInstance(nil): got nil error, want one")
	}
	if err := p.DestroyInstance(t.Context(), &SandboxInstance{}); err == nil {
		t.Error("DestroyInstance(empty id): got nil error, want one")
	}
	if err := p.DestroyInstance(t.Context(), &SandboxInstance{InstanceID: "already-gone"}); err != nil {
		t.Errorf("DestroyInstance(unknown id): %v, want idempotent success", err)
	}
}

func TestUCloudAgentSandboxProvider_ProviderTypeStaysDistinct(t *testing.T) {
	t.Parallel()
	seen := map[ProviderType]bool{}
	for _, provider := range []SandboxProvider{
		newSelfManagedProviderFromEnv(),
		newAliyunProviderFromEnv(),
		newE2BProviderFromEnv(),
		newTenkiProviderFromEnv(),
		newUCloudAgentSandboxProviderFromConfig(map[string]any{}),
	} {
		if seen[provider.ProviderType()] {
			t.Errorf("provider type %q seen twice", provider.ProviderType())
		}
		seen[provider.ProviderType()] = true
	}
}
