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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestProviderManager_SetGet(t *testing.T) {
	t.Parallel()
	m := &ProviderManager{}
	if m.IsConfigured() {
		t.Errorf("fresh manager reports configured")
	}
	if m.Provider() != nil {
		t.Errorf("fresh manager has non-nil provider")
	}

	m.SetProvider(newSelfManagedProviderFromEnv())
	if !m.IsConfigured() {
		t.Errorf("manager not configured after SetProvider")
	}
	if m.Provider() == nil {
		t.Errorf("Provider() is nil after SetProvider")
	}
}

func TestProviderManager_Reset(t *testing.T) {
	t.Parallel()
	m := &ProviderManager{}
	m.SetProvider(newSelfManagedProviderFromEnv())
	m.Reset()
	if m.IsConfigured() {
		t.Errorf("manager reports configured after Reset")
	}
}

// stubProvider is a SandboxProvider used by manager tests.
type stubProvider struct {
	ptype     ProviderType
	supported []string
}

func (s *stubProvider) ProviderType() ProviderType         { return s.ptype }
func (s *stubProvider) Initialize(_ context.Context) error { return nil }
func (s *stubProvider) CreateInstance(_ context.Context, _ string) (*SandboxInstance, error) {
	return &SandboxInstance{InstanceID: "x", Provider: s.ptype, Status: "ok"}, nil
}
func (s *stubProvider) ExecuteCode(_ context.Context, _ *SandboxInstance, _, lang string, _ int, _ map[string]any) (*ExecutionResult, error) {
	return &ExecutionResult{Stdout: "ok", ExitCode: 0, Metadata: map[string]any{"lang": lang}}, nil
}
func (s *stubProvider) DestroyInstance(_ context.Context, _ *SandboxInstance) error { return nil }
func (s *stubProvider) HealthCheck(_ context.Context) error                         { return nil }
func (s *stubProvider) SupportedLanguages() []string                                { return s.supported }

func TestProviderManager_BuildProvider_KnownTypes(t *testing.T) {
	t.Parallel()

	for _, ptype := range []ProviderType{ProviderSelfManaged, ProviderAliyun, ProviderE2B, ProviderLocal, ProviderSSH} {
		t.Run(string(ptype), func(t *testing.T) {
			p, err := buildProvider(ptype)
			if err != nil {
				t.Fatalf("buildProvider(%q): %v", ptype, err)
			}
			if p.ProviderType() != ptype {
				t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ptype)
			}
		})
	}
}

func TestProviderManager_BuildProvider_UnknownType(t *testing.T) {
	t.Parallel()
	if _, err := buildProvider("not-a-real-provider"); err == nil {
		t.Errorf("buildProvider on unknown: got nil error, want one")
	}
}

func TestE2BProvider_AllOps_LoudFail(t *testing.T) {
	// v3: e2b is now a real implementation. The "all ops loud-fail"
	// expectation is obsolete. The new "no creds → error" path is
	// covered by TestE2BProvider_Initialize_MissingCreds in
	// e2b_test.go. This stub is kept as a no-op marker to make the
	// migration trace explicit; remove once the test has been
	// confirmed obsolete in a later cleanup pass.
	t.Skip("removed in v3 — see e2b_test.go for the new behavior")
}

func TestAliyun_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newAliyunProviderFromEnv()
	if p.ProviderType() != ProviderAliyun {
		t.Errorf("ProviderType = %q, want %q", p.ProviderType(), ProviderAliyun)
	}
	langs := p.SupportedLanguages()
	if len(langs) == 0 {
		t.Errorf("SupportedLanguages is empty")
	}
}

func TestAliyun_Initialize_MissingCreds(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv.
	// Save and clear AGENTRUN_* env vars to simulate an unconfigured
	// operator.
	for _, k := range []string{"AGENTRUN_ACCESS_KEY_ID", "AGENTRUN_ACCESS_KEY_SECRET", "AGENTRUN_ACCOUNT_ID"} {
		t.Setenv(k, "")
	}
	p := newAliyunProviderFromEnv()
	if err := p.Initialize(context.Background()); err == nil {
		t.Errorf("Initialize with missing creds: got nil error, want one")
	}
}

// TestSelfManaged_EndToEnd_FullLoop exercises the full self-managed
// flow against a mock executor_manager: Initialize → CreateInstance
// → ExecuteCode → DestroyInstance. Regression test for the
// self_managed provider end-to-end flow.
func TestSelfManaged_EndToEnd_FullLoop(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/run":
			handleRun(t, w, r, "result-stdout", "result-stderr")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.Provider != ProviderSelfManaged {
		t.Errorf("provider = %q, want %q", inst.Provider, ProviderSelfManaged)
	}
	result, err := p.ExecuteCode(context.Background(), inst, "def main(): return 1", "python", 5, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if result.Stdout != "result-stdout" {
		t.Errorf("stdout = %q, want 'result-stdout'", result.Stdout)
	}
	if result.Stderr != "result-stderr" {
		t.Errorf("stderr = %q, want 'result-stderr'", result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", result.ExitCode)
	}
	if err := p.DestroyInstance(context.Background(), inst); err != nil {
		t.Errorf("DestroyInstance: %v", err)
	}
}

// TestNewSelfManagedProviderFromConfig_MinimalConfig pins the
// settings-driven init path: a minimal JSON config yields the
// expected defaults (default endpoint, 30s timeout, pool size 3,
// no per-language base image override).
func TestNewSelfManagedProviderFromConfig_MinimalConfig(t *testing.T) {
	t.Parallel()
	p := newSelfManagedProviderFromConfig(map[string]any{})
	if p.endpoint != selfManagedDefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", p.endpoint, selfManagedDefaultEndpoint)
	}
	if p.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", p.timeout)
	}
	if p.poolSize != 3 {
		t.Errorf("poolSize = %d, want 3", p.poolSize)
	}
	if p.baseImages["python"] != "" || p.baseImages["nodejs"] != "" {
		t.Errorf("baseImages should be empty, got %+v", p.baseImages)
	}
}

// TestNewSelfManagedProviderFromConfig_FullConfig verifies that
// every config key propagates: a custom endpoint, a custom timeout
// in seconds-as-float, a non-default pool size, and per-language
// base images.
func TestNewSelfManagedProviderFromConfig_FullConfig(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{
		"EXECUTOR_MANAGER_URL":       "https://custom.example:9999/",
		"EXECUTOR_MANAGER_TIMEOUT":   float64(45), // JSON-decoded number
		"EXECUTOR_MANAGER_POOL_SIZE": float64(10),
		"BASE_PYTHON_IMAGE":          "registry.example.com/py:latest",
		"BASE_NODEJS_IMAGE":          "registry.example.com/node:20",
	}
	p := newSelfManagedProviderFromConfig(cfg)
	if p.endpoint != "https://custom.example:9999" {
		t.Errorf("endpoint = %q, want trailing slash stripped", p.endpoint)
	}
	if p.timeout != 45*time.Second {
		t.Errorf("timeout = %v, want 45s", p.timeout)
	}
	if p.poolSize != 10 {
		t.Errorf("poolSize = %d, want 10", p.poolSize)
	}
	if p.baseImages["python"] != "registry.example.com/py:latest" {
		t.Errorf("python baseImage = %q", p.baseImages["python"])
	}
	if p.baseImages["nodejs"] != "registry.example.com/node:20" {
		t.Errorf("nodejs baseImage = %q", p.baseImages["nodejs"])
	}
}

// TestNewSelfManagedProviderFromConfig_TimeoutAsString covers the
// duration-string code path: "1m30s" must parse correctly.
func TestNewSelfManagedProviderFromConfig_TimeoutAsString(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{
		"EXECUTOR_MANAGER_TIMEOUT": "1m30s",
	}
	p := newSelfManagedProviderFromConfig(cfg)
	if p.timeout != 90*time.Second {
		t.Errorf("timeout = %v, want 1m30s", p.timeout)
	}
}

// TestNewAliyunProviderFromConfig_Minimal pins the aliyun
// settings-driven init. The 30s hard cap from the env path
// must apply here too.
func TestNewAliyunProviderFromConfig_Minimal(t *testing.T) {
	t.Parallel()
	p := newAliyunProviderFromConfig(map[string]any{})
	if p.region != aliyunDefaultRegion {
		t.Errorf("region = %q, want %q (default)", p.region, aliyunDefaultRegion)
	}
	if p.timeout != 30 {
		t.Errorf("timeout = %d, want 30 (default + cap)", p.timeout)
	}
}

// TestNewAliyunProviderFromConfig_TimeoutCap: timeout above 30
// must clamp to 30.
func TestNewAliyunProviderFromConfig_TimeoutCap(t *testing.T) {
	t.Parallel()
	p := newAliyunProviderFromConfig(map[string]any{
		"ACCESS_KEY_ID":     "k",
		"ACCESS_KEY_SECRET": "s",
		"ACCOUNT_ID":        "a",
		"REGION":            "cn-shanghai",
		"TIMEOUT":           float64(120),
	})
	if p.timeout != 30 {
		t.Errorf("timeout = %d, want 30 (hard cap)", p.timeout)
	}
}

// TestNewLocalProviderFromConfig_Defaults pins the local provider's
// settings-driven defaults.
func TestNewLocalProviderFromConfig_Defaults(t *testing.T) {
	t.Parallel()
	p := newLocalProviderFromConfig(map[string]any{})
	if p.pythonBin != localDefaultPythonBin {
		t.Errorf("pythonBin = %q, want default", p.pythonBin)
	}
	if p.nodeBin != localDefaultNodeBin {
		t.Errorf("nodeBin = %q, want default", p.nodeBin)
	}
	if p.workDir != localDefaultWorkDir {
		t.Errorf("workDir = %q, want default", p.workDir)
	}
	if p.timeout != localDefaultTimeout {
		t.Errorf("timeout = %d, want default", p.timeout)
	}
}

// TestNewLocalProviderFromConfig_FullConfig verifies that config
// keys override every field.
func TestNewLocalProviderFromConfig_FullConfig(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{
		"PYTHON_BIN":         "python3.12",
		"NODE_BIN":           "node22",
		"WORK_DIR":           "/var/sandbox",
		"TIMEOUT":            float64(60),
		"MAX_OUTPUT_BYTES":   float64(2_000_000),
		"MAX_ARTIFACTS":      float64(50),
		"MAX_ARTIFACT_BYTES": float64(20_000_000),
	}
	p := newLocalProviderFromConfig(cfg)
	if p.pythonBin != "python3.12" {
		t.Errorf("pythonBin = %q", p.pythonBin)
	}
	if p.nodeBin != "node22" {
		t.Errorf("nodeBin = %q", p.nodeBin)
	}
	if p.workDir != "/var/sandbox" {
		t.Errorf("workDir = %q", p.workDir)
	}
	if p.timeout != 60 {
		t.Errorf("timeout = %d", p.timeout)
	}
	if p.maxOutputBytes != 2_000_000 {
		t.Errorf("maxOutputBytes = %d", p.maxOutputBytes)
	}
	if p.maxArtifacts != 50 {
		t.Errorf("maxArtifacts = %d", p.maxArtifacts)
	}
	if p.maxArtifactBytes != 20_000_000 {
		t.Errorf("maxArtifactBytes = %d", p.maxArtifactBytes)
	}
}

// TestNewE2BProviderFromConfig_Default pins the e2b settings-driven
// defaults (template + 60s timeout).
func TestNewE2BProviderFromConfig_Default(t *testing.T) {
	t.Parallel()
	p := newE2BProviderFromConfig(map[string]any{})
	if p.template != e2bDefaultTemplate {
		t.Errorf("template = %q, want %q", p.template, e2bDefaultTemplate)
	}
	if p.sandboxTimeout != e2bDefaultSandboxTimeout {
		t.Errorf("sandboxTimeout = %v, want %v", p.sandboxTimeout, e2bDefaultSandboxTimeout)
	}
}

// TestNewSSHProviderFromConfig_Defaults pins the ssh settings-driven
// defaults (port + python/node bins + work dir).
func TestNewSSHProviderFromConfig_Defaults(t *testing.T) {
	t.Parallel()
	p := newSSHProviderFromConfig(map[string]any{})
	if p.port != sshDefaultPort {
		t.Errorf("port = %d, want %d", p.port, sshDefaultPort)
	}
	if p.pythonBin != sshDefaultPythonBin {
		t.Errorf("pythonBin = %q", p.pythonBin)
	}
	if p.workDir != sshDefaultWorkDir {
		t.Errorf("workDir = %q", p.workDir)
	}
}

// TestBuildProviderFromConfig_UnknownType covers the switch
// default branch in buildProviderFromConfig.
func TestBuildProviderFromConfig_UnknownType(t *testing.T) {
	t.Parallel()
	_, err := buildProviderFromConfig(ProviderType("nonexistent"), map[string]any{})
	if err == nil {
		t.Errorf("buildProviderFromConfig(nonexistent) = nil error, want one")
	}
}

// TestBuildProviderFromConfig_SelfManaged_HappyPath verifies the
// settings-driven switch dispatch returns a working SelfManaged
// provider (the contract is "constructs without panicking"; the
// healthz probe is checked at Initialize time, not at construct).
func TestBuildProviderFromConfig_SelfManaged_HappyPath(t *testing.T) {
	t.Parallel()
	p, err := buildProviderFromConfig(ProviderSelfManaged, map[string]any{
		"EXECUTOR_MANAGER_URL": "http://example.invalid:9999",
	})
	if err != nil {
		t.Fatalf("buildProviderFromConfig: %v", err)
	}
	if p == nil {
		t.Fatal("provider is nil")
	}
	if p.ProviderType() != ProviderSelfManaged {
		t.Errorf("provider type = %q, want self_managed", p.ProviderType())
	}
}

// fakeSettingsReader is the test double for SettingsReader. It
// serves a hard-coded map keyed by setting name and returns
// fakeErr when non-nil (so the tests can exercise the
// "DAO returns an error → fall back to env" branch).
type fakeSettingsReader struct {
	rows    map[string][]entity.SystemSettings
	fakeErr error
}

func (f *fakeSettingsReader) GetByName(name string) ([]entity.SystemSettings, error) {
	if f.fakeErr != nil {
		return nil, f.fakeErr
	}
	return f.rows[name], nil
}

// TestLoadFromSettingsWithReader_HappyPath pins the settings-driven
// init: a fake reader returns a self_managed row + a full JSON
// config; the manager builds a SelfManagedProvider with the
// config-derived endpoint, timeout, and base images. The provider's
// Initialize runs against a mock executor_manager so the full path
// succeeds and the manager flips IsConfigured.
func TestLoadFromSettingsWithReader_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Drive the mock server by setting the SANDBOX_EXECUTOR_MANAGER_URL
	// env var; then have the settings config return a matching
	// override. The env path is what the manager falls back to when
	// the settings row's URL is invalid; here we set both so the
	// happy path uses the settings URL.
	r := &fakeSettingsReader{
		rows: map[string][]entity.SystemSettings{
			"sandbox.provider_type": {{Name: "sandbox.provider_type", Value: "self_managed"}},
			"sandbox.self_managed": {{Name: "sandbox.self_managed", Value: `{
				"EXECUTOR_MANAGER_URL": "` + srv.URL + `",
				"EXECUTOR_MANAGER_TIMEOUT": "5s",
				"EXECUTOR_MANAGER_POOL_SIZE": 7,
				"BASE_PYTHON_IMAGE": "reg.example.com/py:1"
			}`}},
		},
	}
	m := &ProviderManager{}
	if err := m.LoadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("LoadFromSettingsWithReader: %v", err)
	}
	if !m.IsConfigured() {
		t.Errorf("manager not configured after settings load")
	}
	sm, ok := m.Provider().(*SelfManagedProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *SelfManagedProvider", m.Provider())
	}
	if sm.endpoint != srv.URL {
		t.Errorf("endpoint = %q, want %q (from settings)", sm.endpoint, srv.URL)
	}
	if sm.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s (from settings)", sm.timeout)
	}
	if sm.poolSize != 7 {
		t.Errorf("poolSize = %d, want 7 (from settings)", sm.poolSize)
	}
	if sm.baseImages["python"] != "reg.example.com/py:1" {
		t.Errorf("python baseImage = %q (from settings)", sm.baseImages["python"])
	}
}

// TestLoadFromSettingsWithReader_EmptyFallback: when the reader
// returns no rows, the manager falls back to env-driven init.
// We clear the SANDBOX_PROVIDER_TYPE env so the fallback picks
// the default (self_managed), then point the endpoint at a
// working httptest server to let Initialize succeed.
func TestLoadFromSettingsWithReader_EmptyFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("SANDBOX_PROVIDER_TYPE", "")
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_URL", srv.URL)
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_TIMEOUT", "5s")

	r := &fakeSettingsReader{rows: map[string][]entity.SystemSettings{}}
	m := &ProviderManager{}
	if err := m.LoadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("LoadFromSettingsWithReader: %v", err)
	}
	if !m.IsConfigured() {
		t.Errorf("manager not configured after env fallback")
	}
	if got := m.Provider().ProviderType(); got != ProviderSelfManaged {
		t.Errorf("provider type = %q, want self_managed (env default)", got)
	}
}

// TestLoadFromSettingsWithReader_DAOErrorFallback: when the
// reader returns an error, the manager falls back to env-driven
// init (same path as the empty-rows case).
func TestLoadFromSettingsWithReader_DAOErrorFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("SANDBOX_PROVIDER_TYPE", "")
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_URL", srv.URL)
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_TIMEOUT", "5s")

	r := &fakeSettingsReader{fakeErr: errors.New("db is down")}
	m := &ProviderManager{}
	if err := m.LoadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("LoadFromSettingsWithReader (DAO error fallback): %v", err)
	}
	if got := m.Provider().ProviderType(); got != ProviderSelfManaged {
		t.Errorf("provider type = %q, want self_managed (env fallback after DAO error)", got)
	}
}

// TestLoadFromSettingsWithReader_MalformedJSONFallback: when the
// settings row exists but contains invalid JSON, the manager
// uses an empty config and falls through to env defaults.
func TestLoadFromSettingsWithReader_MalformedJSONFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("SANDBOX_PROVIDER_TYPE", "")
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_URL", srv.URL)
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_TIMEOUT", "5s")

	r := &fakeSettingsReader{
		rows: map[string][]entity.SystemSettings{
			"sandbox.provider_type": {{Name: "sandbox.provider_type", Value: "self_managed"}},
			"sandbox.self_managed":  {{Name: "sandbox.self_managed", Value: `{not valid json`}},
		},
	}
	m := &ProviderManager{}
	if err := m.LoadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("LoadFromSettingsWithReader (malformed JSON fallback): %v", err)
	}
	sm, ok := m.Provider().(*SelfManagedProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *SelfManagedProvider", m.Provider())
	}
	// Empty config → default endpoint (the test server URL was
	// set via env, so the manager's InitFromEnv would have used
	// it; the settings path fell through to env defaults when
	// JSON was malformed).
	if sm.endpoint == "" {
		t.Errorf("endpoint should be the env default, got empty")
	}
}

// TestLoadFromSettingsWithReader_UnknownProviderType covers the
// "settings row says 'foo' which we don't know" branch. The
// manager falls back to env.
func TestLoadFromSettingsWithReader_UnknownProviderType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("SANDBOX_PROVIDER_TYPE", "")
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_URL", srv.URL)
	t.Setenv("SANDBOX_EXECUTOR_MANAGER_TIMEOUT", "5s")

	r := &fakeSettingsReader{
		rows: map[string][]entity.SystemSettings{
			"sandbox.provider_type": {{Name: "sandbox.provider_type", Value: "mystery_provider"}},
		},
	}
	m := &ProviderManager{}
	if err := m.LoadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("LoadFromSettingsWithReader (unknown type fallback): %v", err)
	}
	// Falls back to env-driven self_managed, NOT the unknown type.
	if got := m.Provider().ProviderType(); got != ProviderSelfManaged {
		t.Errorf("provider type = %q, want self_managed (env fallback)", got)
	}
}

// TestLoadFromSettingsWithReader_AlreadyLoaded_NoOp: once a
// provider is loaded, subsequent LoadFromSettings calls are
// no-ops. The reader is intentionally rigged to return a
// different provider type — if LoadFromSettings honored that,
// the manager would re-init; the test asserts it doesn't.
func TestLoadFromSettingsWithReader_AlreadyLoaded_NoOp(t *testing.T) {
	m := &ProviderManager{}
	m.SetProvider(newSelfManagedProviderFromEnv())
	original := m.Provider()

	r := &fakeSettingsReader{
		rows: map[string][]entity.SystemSettings{
			"sandbox.provider_type": {{Name: "sandbox.provider_type", Value: "local"}},
		},
	}
	if err := m.LoadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("LoadFromSettingsWithReader: %v", err)
	}
	if m.Provider() != original {
		t.Errorf("provider was replaced after load; expected no-op")
	}
}

// TestReloadFromSettingsWithReader pins the reload path: after a
// successful load, ReloadFromSettings resets the manager and
// re-reads settings. The fake reader returns self_managed + a
// working executor_manager URL, so the reload path builds and
// initializes a fresh provider.
func TestReloadFromSettingsWithReader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := &fakeSettingsReader{
		rows: map[string][]entity.SystemSettings{
			"sandbox.provider_type": {{Name: "sandbox.provider_type", Value: "self_managed"}},
			"sandbox.self_managed": {{Name: "sandbox.self_managed", Value: `{
				"EXECUTOR_MANAGER_URL": "` + srv.URL + `",
				"EXECUTOR_MANAGER_TIMEOUT": "5s"
			}`}},
		},
	}
	m := &ProviderManager{}
	if err := m.ReloadFromSettingsWithReader(context.Background(), r); err != nil {
		t.Fatalf("ReloadFromSettingsWithReader: %v", err)
	}
	if got := m.Provider().ProviderType(); got != ProviderSelfManaged {
		t.Errorf("provider type after reload = %q, want self_managed", got)
	}
	// Confirm the manager was actually reset + reloaded, not a
	// no-op: a freshly-built SelfManagedProvider's endpoint
	// comes from the settings row, not the env default.
	sm := m.Provider().(*SelfManagedProvider)
	if sm.endpoint != srv.URL {
		t.Errorf("endpoint = %q, want %q (from settings after reload)", sm.endpoint, srv.URL)
	}
}
