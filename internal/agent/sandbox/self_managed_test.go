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
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newSelfManagedForTest builds a provider pointing at the given
// endpoint. env-driven factory not used because we want to inject
// the test server's URL.
func newSelfManagedForTest(endpoint string) *SelfManagedProvider {
	return &SelfManagedProvider{
		endpoint:     endpoint,
		timeout:      5 * time.Second,
		poolSize:     3,
		helper:       NewHTTPClient(HTTPConfig{}),
		healthHelper: NewHTTPClient(HTTPConfig{Timeout: 2 * time.Second}),
	}
}

func TestSelfManaged_HealthCheck_OK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	p := newSelfManagedForTest(srv.URL)
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestSelfManaged_HealthCheck_Fail(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	if err := p.HealthCheck(context.Background()); err == nil {
		t.Errorf("HealthCheck on 500: got nil error, want one")
	}
}

func TestSelfManaged_Initialize(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Healthz OK; /run returns success.
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/run" {
			handleRun(t, w, r, "ok", "")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	if err := p.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !p.isInitialized() {
		t.Errorf("provider not flagged initialized after successful probe")
	}
}

func TestSelfManaged_Initialize_HealthFails(t *testing.T) {
	t.Parallel()
	// Server that is reachable but returns 500 for /healthz.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	if err := p.Initialize(context.Background()); err == nil {
		t.Errorf("Initialize on 500 healthz: got nil error, want one")
	}
}

func TestSelfManaged_CreateInstance(t *testing.T) {
	t.Parallel()
	p := newSelfManagedForTest("http://example.invalid:9999")
	p.initialized = true // bypass probe for unit testing
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if inst.Provider != ProviderSelfManaged {
		t.Errorf("provider = %q, want %q", inst.Provider, ProviderSelfManaged)
	}
	if inst.Status != "running" {
		t.Errorf("status = %q, want %q", inst.Status, "running")
	}
	if inst.InstanceID == "" {
		t.Errorf("instance id is empty")
	}
}

func TestSelfManaged_CreateInstance_UnsupportedLanguage(t *testing.T) {
	t.Parallel()
	p := newSelfManagedForTest("http://example.invalid:9999")
	p.initialized = true
	if _, err := p.CreateInstance(context.Background(), "ruby"); err == nil {
		t.Errorf("CreateInstance(ruby): got nil error, want one")
	}
}

func TestSelfManaged_ExecuteCode(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		handleRunWithResult(t, w, r, "hello", "world", map[string]any{
			"present": true,
			"value":   2,
			"type":    "json",
		})
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	p.initialized = true
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	result, err := p.ExecuteCode(context.Background(), inst, "def main(): return 1+1", "python", 10, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	if capturedPath != "/run" {
		t.Errorf("captured path = %q, want /run", capturedPath)
	}

	// Verify request body shape
	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("decode body: %v (raw=%s)", err, capturedBody)
	}
	if payload["language"] != "python" {
		t.Errorf("language = %v, want python", payload["language"])
	}
	codeB64, _ := payload["code_b64"].(string)
	decoded, _ := base64.StdEncoding.DecodeString(codeB64)
	if !strings.Contains(string(decoded), "def main(): return 1+1") {
		t.Errorf("decoded code does not contain user script: %q", string(decoded))
	}
	if strings.Contains(string(decoded), resultMarkerPrefix) {
		t.Errorf("decoded code should be raw user script, got wrapped payload: %q", string(decoded))
	}
	if strings.Contains(string(decoded), `main(**{})`) {
		t.Errorf("decoded code should not contain client-side main(**args) wrapper: %q", string(decoded))
	}

	// Verify response parsing
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("stdout = %q, want to contain 'hello'", result.Stdout)
	}
	if !strings.Contains(result.Stderr, "world") {
		t.Errorf("stderr = %q, want to contain 'world'", result.Stderr)
	}
	if got, ok := result.Metadata["structured_result"].(map[string]any); !ok || got["value"] != json.Number("2") {
		t.Errorf("structured_result = %#v, want value 2 from HTTP result field", result.Metadata["structured_result"])
	}
}

func TestSelfManaged_ExecuteCode_JSWrapped(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		handleRun(t, w, r, "ok", "")
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	p.initialized = true
	inst, err := p.CreateInstance(context.Background(), "nodejs")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	_, err = p.ExecuteCode(context.Background(), inst, "async function main() {}", "javascript", 5, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["language"] != "nodejs" {
		t.Errorf("language = %v, want nodejs", payload["language"])
	}
	codeB64, _ := payload["code_b64"].(string)
	decoded, _ := base64.StdEncoding.DecodeString(codeB64)
	// The Go wrapper binds the args and looks for `main` either
	// globally or via `module.exports.main`. The literal
	// "module.exports = { main }" is added server-side by
	// executor_manager (see handlers.py), not by our wrapper —
	// so we look for the bits the wrapper actually emits.
	if strings.Contains(string(decoded), "const __ragflowArgs = {};") {
		t.Errorf("decoded JS should be raw user script, got wrapped payload: %q", string(decoded))
	}
	if strings.Contains(string(decoded), "module.exports && module.exports.main") {
		t.Errorf("decoded JS should not contain client-side wrapper logic: %q", string(decoded))
	}
}

func TestSelfManaged_ExecuteCode_PrefersHTTPResultField(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status":"SUCCESS",
			"stdout":"",
			"stderr":"",
			"exit_code":0,
			"artifacts":[],
			"result":{"present":true,"value":16,"type":"json"}
		}`))
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	p.initialized = true
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	result, err := p.ExecuteCode(context.Background(), inst, "def main(): return 16", "python", 10, nil)
	if err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	structured, ok := result.Metadata["structured_result"].(map[string]any)
	if !ok {
		t.Fatalf("structured_result type = %T, want map[string]any", result.Metadata["structured_result"])
	}
	if structured["present"] != true {
		t.Fatalf("structured_result.present = %#v, want true", structured["present"])
	}
	if structured["value"] != json.Number("16") {
		t.Fatalf("structured_result.value = %#v, want 16", structured["value"])
	}
}

func TestSelfManaged_ExecuteCode_Non200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad code"))
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	p.initialized = true
	inst, _ := p.CreateInstance(context.Background(), "python")
	_, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
	if err == nil {
		t.Errorf("ExecuteCode on 400: got nil error, want one")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v, want to mention 400", err)
	}
}

func TestSelfManaged_ExecuteCode_NotInitialized(t *testing.T) {
	t.Parallel()
	p := newSelfManagedForTest("http://example.invalid:9999")
	// do NOT set initialized
	inst := &SandboxInstance{InstanceID: "x"}
	_, err := p.ExecuteCode(context.Background(), inst, "x", "python", 5, nil)
	if err == nil {
		t.Errorf("ExecuteCode on uninitialized: got nil error, want one")
	}
}

func TestSelfManaged_ExecuteCode_UnsupportedLanguage(t *testing.T) {
	t.Parallel()
	p := newSelfManagedForTest("http://example.invalid:9999")
	p.initialized = true
	inst, _ := p.CreateInstance(context.Background(), "python")
	_, err := p.ExecuteCode(context.Background(), inst, "x", "ruby", 5, nil)
	if err == nil {
		t.Errorf("ExecuteCode(ruby): got nil error, want one")
	}
}

func TestSelfManaged_DestroyInstance_Noop(t *testing.T) {
	t.Parallel()
	p := newSelfManagedForTest("http://example.invalid:9999")
	p.initialized = true
	if err := p.DestroyInstance(context.Background(), &SandboxInstance{InstanceID: "x"}); err != nil {
		t.Errorf("DestroyInstance: %v", err)
	}
}

func TestSelfManaged_ProviderTypeAndLanguages(t *testing.T) {
	t.Parallel()
	p := newSelfManagedForTest("http://x")
	if got := p.ProviderType(); got != ProviderSelfManaged {
		t.Errorf("ProviderType = %q, want %q", got, ProviderSelfManaged)
	}
	langs := p.SupportedLanguages()
	if len(langs) == 0 {
		t.Errorf("SupportedLanguages is empty")
	}
}

// TestNewSelfManagedProviderFromEnv_BaseImages pins the operator-facing
// per-language base image override path. When SANDBOX_BASE_PYTHON_IMAGE
// / SANDBOX_BASE_NODEJS_IMAGE are set, the provider must surface
// them in the baseImages map (used as the `base_image` field on
// POST /run payloads). When unset, the entries must be empty
// strings (the server treats empty as "use my default image").
func TestNewSelfManagedProviderFromEnv_BaseImages(t *testing.T) {
	// Case 1: both env vars set.
	t.Setenv("SANDBOX_BASE_PYTHON_IMAGE", "registry.example.com/custom-python:1.2")
	t.Setenv("SANDBOX_BASE_NODEJS_IMAGE", "registry.example.com/custom-node:20")
	p1 := newSelfManagedProviderFromEnv()
	if got := p1.baseImages["python"]; got != "registry.example.com/custom-python:1.2" {
		t.Errorf("python baseImage = %q, want registry.example.com/custom-python:1.2", got)
	}
	if got := p1.baseImages["nodejs"]; got != "registry.example.com/custom-node:20" {
		t.Errorf("nodejs baseImage = %q, want registry.example.com/custom-node:20", got)
	}

	// Case 2: env vars unset. Empty string is the documented
	// "no override; use executor_manager's default" sentinel.
	t.Setenv("SANDBOX_BASE_PYTHON_IMAGE", "")
	t.Setenv("SANDBOX_BASE_NODEJS_IMAGE", "")
	p2 := newSelfManagedProviderFromEnv()
	if got, ok := p2.baseImages["python"]; !ok || got != "" {
		t.Errorf("python baseImage = (%q, %v); want (\"\", true)", got, ok)
	}
	if got, ok := p2.baseImages["nodejs"]; !ok || got != "" {
		t.Errorf("nodejs baseImage = (%q, %v); want (\"\", true)", got, ok)
	}

	// Case 3: only python set. nodejs slot must be empty.
	t.Setenv("SANDBOX_BASE_PYTHON_IMAGE", "only-python:latest")
	t.Setenv("SANDBOX_BASE_NODEJS_IMAGE", "")
	p3 := newSelfManagedProviderFromEnv()
	if got := p3.baseImages["python"]; got != "only-python:latest" {
		t.Errorf("python baseImage = %q, want only-python:latest", got)
	}
	if got := p3.baseImages["nodejs"]; got != "" {
		t.Errorf("nodejs baseImage = %q, want empty (only python was set)", got)
	}
}

// TestSelfManaged_ExecuteCode_PassesBaseImage verifies the
// `base_image` field flows from the provider's baseImages map into
// the POST /run payload when the operator has configured an override.
func TestSelfManaged_ExecuteCode_PassesBaseImage(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		handleRun(t, w, r, "ok", "")
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	p.initialized = true
	p.baseImages = map[string]string{
		"python": "custom-python:v1",
		"nodejs": "",
	}
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if _, err := p.ExecuteCode(context.Background(), inst, "def main(): return 1", "python", 10, nil); err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("decode: %v (raw=%s)", err, capturedBody)
	}
	if got := payload["base_image"]; got != "custom-python:v1" {
		t.Errorf("base_image = %v, want custom-python:v1", got)
	}
}

// TestSelfManaged_ExecuteCode_OmitsEmptyBaseImage verifies that
// an empty string override is NOT sent on the wire (we want to
// avoid `base_image: ""` confusing the executor_manager). The
// provider plumbs the field only when the slot is non-empty.
func TestSelfManaged_ExecuteCode_OmitsEmptyBaseImage(t *testing.T) {
	t.Parallel()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capturedBody = body
		handleRun(t, w, r, "ok", "")
	}))
	defer srv.Close()

	p := newSelfManagedForTest(srv.URL)
	p.initialized = true
	p.baseImages = map[string]string{
		"python": "", // operator did not override
		"nodejs": "",
	}
	inst, err := p.CreateInstance(context.Background(), "python")
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	if _, err := p.ExecuteCode(context.Background(), inst, "def main(): return 1", "python", 10, nil); err != nil {
		t.Fatalf("ExecuteCode: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(capturedBody, &payload); err != nil {
		t.Fatalf("decode: %v (raw=%s)", err, capturedBody)
	}
	if _, present := payload["base_image"]; present {
		t.Errorf("base_image should be absent when no override is set; got %v", payload["base_image"])
	}
}

// handleRun is a small helper that responds with a fake
// executor_manager /run result.
func handleRun(t *testing.T, w http.ResponseWriter, _ *http.Request, stdout, stderr string) {
	t.Helper()
	handleRunWithResult(t, w, nil, stdout, stderr, map[string]any{
		"present": false,
		"value":   nil,
		"type":    "json",
	})
}

func handleRunWithResult(t *testing.T, w http.ResponseWriter, _ *http.Request, stdout, stderr string, result map[string]any) {
	t.Helper()
	resp := map[string]any{
		"status":    "ok",
		"stdout":    stdout,
		"stderr":    stderr,
		"exit_code": 0,
		"detail":    "",
		"artifacts": []any{},
		"result":    result,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
