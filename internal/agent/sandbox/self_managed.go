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

// self_managed.go is the Go port of
// `agent/sandbox/providers/self_managed.py`.
//
// The provider wraps the existing executor_manager HTTP API
// (default `http://sandbox-executor-manager:9385`) which manages a
// pool of Docker containers with gVisor for secure code execution.
// We do NOT port the container pool itself — that's the Python
// FastAPI service and stays in Python. We only port the *client*
// surface that the Go CodeExec tool needs to dispatch a code
// execution request.
//
// Wire format (matches the Python `requests.post` call in
// self_managed.py::execute_code):
//
//   POST {endpoint}/run
//   Content-Type: application/json
//   {
//     "code_b64":   "<base64-encoded code>",
//     "language":   "python" | "nodejs",
//     "arguments":  { ... }    // optional, defaults to {}
//   }
//
// Response (executor_manager's CodeExecutionResult, see
// agent/sandbox/executor_manager/models/schemas.py):
//
//   {
//     "status":          "ok" | "PROGRAM_RUNNER_ERROR" | ...,
//     "stdout":          "...",
//     "stderr":          "...",
//     "exit_code":       0,
//     "detail":          "...",
//     "time_used_ms":    1234,
//     "memory_used_kb":  5678,
//     "artifacts":       [ {name, mime_type, size, content_b64}, ... ],
//     "result":          { "present": bool, "value": ..., "type": "json" }
//   }
//
// Health check: GET {endpoint}/healthz → 200 {"status":"ok"}.

package sandbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// selfManagedDefaultEndpoint is the canonical executor_manager
// endpoint baked into the Python side. Operators override via
// SANDBOX_EXECUTOR_MANAGER_URL.
const selfManagedDefaultEndpoint = "http://sandbox-executor-manager:9385"

// SelfManagedProvider is the Go port of
// `agent/sandbox/providers/self_managed.py::SelfManagedProvider`.
type SelfManagedProvider struct {
	endpoint     string
	timeout      time.Duration
	poolSize     int
	maxRetries   int
	helper       *HTTPClient
	healthHelper *HTTPClient
	mu           sync.Mutex
	initialized  bool
	// baseImages is the per-language base image override. Keys are
	// canonical language names ("python" / "nodejs"); values are
	// fully-qualified Docker image references. Empty string means
	// "use the executor_manager's default" — no override.
	//
	// Mirrors the Python side's
	// `SANDBOX_BASE_PYTHON_IMAGE` / `SANDBOX_BASE_NODEJS_IMAGE`
	// env vars (default: `infiniflow/sandbox-base-python:latest`
	// and `infiniflow/sandbox-base-nodejs:latest`). The Go port
	// reads the same env vars; operators who customize one
	// language's image get a per-language override path that the
	// executor_manager can then route at container-create time.
	baseImages map[string]string
}

// newSelfManagedProviderFromEnv reads SANDBOX_EXECUTOR_MANAGER_URL
// (default: http://sandbox-executor-manager:9385) and
// SANDBOX_EXECUTOR_MANAGER_TIMEOUT (default 30s) and returns a
// provider ready for Initialize. The per-language base image
// overrides (SANDBOX_BASE_PYTHON_IMAGE / SANDBOX_BASE_NODEJS_IMAGE)
// are also read; empty values mean "use executor_manager's
// default image" — no override.
func newSelfManagedProviderFromEnv() *SelfManagedProvider {
	return newSelfManagedProviderFromConfig(selfManagedConfigFromEnv())
}

// selfManagedConfigFromEnv builds a config map from the SANDBOX_*
// env vars, mirroring the admin-panel settings JSON shape.
func selfManagedConfigFromEnv() map[string]any {
	return map[string]any{
		"EXECUTOR_MANAGER_URL":         os.Getenv("SANDBOX_EXECUTOR_MANAGER_URL"),
		"EXECUTOR_MANAGER_TIMEOUT":     os.Getenv("SANDBOX_EXECUTOR_MANAGER_TIMEOUT"),
		"EXECUTOR_MANAGER_POOL_SIZE":   os.Getenv("SANDBOX_EXECUTOR_MANAGER_POOL_SIZE"),
		"EXECUTOR_MANAGER_MAX_RETRIES": os.Getenv("SANDBOX_EXECUTOR_MANAGER_MAX_RETRIES"),
		"BASE_PYTHON_IMAGE":            os.Getenv("SANDBOX_BASE_PYTHON_IMAGE"),
		"BASE_NODEJS_IMAGE":            os.Getenv("SANDBOX_BASE_NODEJS_IMAGE"),
	}
}

// newSelfManagedProviderFromConfig builds the provider from a JSON
// config map (as stored in the system_settings table for the
// self_managed provider). The config keys mirror the env-var names
// without the SANDBOX_ prefix; see selfManagedConfigFromEnv for the
// shape. Missing / unparseable values fall back to the same defaults
// the env-driven path uses.
func newSelfManagedProviderFromConfig(cfg map[string]any) *SelfManagedProvider {
	endpoint := configString(cfg, "EXECUTOR_MANAGER_URL")
	if endpoint == "" {
		endpoint = selfManagedDefaultEndpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")

	timeout := configDuration(cfg, "EXECUTOR_MANAGER_TIMEOUT", 30*time.Second)
	poolSize := configInt(cfg, "EXECUTOR_MANAGER_POOL_SIZE", 3)
	maxRetries := configInt(cfg, "EXECUTOR_MANAGER_MAX_RETRIES", 3)
	_ = maxRetries // retained for future use; retry is in HTTPClient

	// Per-language base image overrides. Empty = executor_manager
	// default; non-empty = the operator picked a custom base image
	// (typically a heavier Python image with torch/tensorflow
	// pre-installed, or a node image with native deps for native
	// addons).
	baseImages := map[string]string{
		"python": configString(cfg, "BASE_PYTHON_IMAGE"),
		"nodejs": configString(cfg, "BASE_NODEJS_IMAGE"),
	}

	return &SelfManagedProvider{
		endpoint:   endpoint,
		timeout:    timeout,
		poolSize:   poolSize,
		baseImages: baseImages,
		helper: NewHTTPClient(HTTPConfig{
			Timeout: timeout,
		}),
		healthHelper: NewHTTPClient(HTTPConfig{
			Timeout: 5 * time.Second,
		}),
	}
}

// ProviderType returns ProviderSelfManaged.
func (p *SelfManagedProvider) ProviderType() ProviderType {
	return ProviderSelfManaged
}

// Initialize probes the upstream via /healthz. If unreachable, returns
// an error so the manager does not register a broken provider.
func (p *SelfManagedProvider) Initialize(ctx context.Context) error {
	if err := p.HealthCheck(ctx); err != nil {
		return fmt.Errorf("self_managed: %w", err)
	}
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the languages the executor_manager
// accepts.
func (p *SelfManagedProvider) SupportedLanguages() []string {
	return []string{"python", "nodejs", "javascript"}
}

// CreateInstance returns a logical instance handle. Self-managed's
// instance lifetime is owned by the executor_manager's container
// pool; this method only generates a tracking UUID.
func (p *SelfManagedProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("self_managed: provider not initialized")
	}
	lang := normalizeLanguage(template)
	if lang == "" {
		return nil, fmt.Errorf("self_managed: unsupported language %q", template)
	}
	return &SandboxInstance{
		InstanceID: uuid.NewString(),
		Provider:   ProviderSelfManaged,
		Status:     "running",
		Metadata: map[string]any{
			"language":  lang,
			"endpoint":  p.endpoint,
			"pool_size": p.poolSize,
		},
	}, nil
}

// ExecuteCode POSTs to {endpoint}/run with base64-encoded code.
// The result is parsed and the structured `__RAGFLOW_RESULT__` marker
// (if any) is extracted from stdout via ExtractStructuredResult.
func (p *SelfManagedProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("self_managed: provider not initialized")
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("self_managed: unsupported language %q", language)
	}

	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = int(p.timeout.Seconds())
	}

	payload := map[string]any{
		// executor_manager wraps the raw user code on the server side.
		// Do not pre-wrap here or we risk double-execution semantics.
		"code_b64":  base64.StdEncoding.EncodeToString([]byte(code)),
		"language":  lang,
		"arguments": args,
	}
	// Per-language base image override. Empty string (operator
	// did not set SANDBOX_BASE_<LANG>_IMAGE) means "let the
	// executor_manager pick its default image". We omit the
	// field entirely when there's no override — sending
	// `base_image: ""` would force the server into a
	// "pull the empty-named image" branch we don't want. The
	// server treats absent == "use my default image".
	if img := p.baseImages[lang]; img != "" {
		payload["base_image"] = img
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("self_managed: marshal request: %w", err)
	}

	start := time.Now()
	resp, err := p.helper.Do(ctx, http.MethodPost, p.endpoint+"/run", string(body), "application/json", nil)
	if err != nil {
		return nil, fmt.Errorf("self_managed: POST /run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain for connection reuse and return a typed error.
		var b strings.Builder
		_, _ = io.Copy(&b, resp.Body)
		return nil, fmt.Errorf("self_managed: POST /run returned %d: %s", resp.StatusCode, b.String())
	}

	var raw struct {
		Status        string           `json:"status"`
		Stdout        string           `json:"stdout"`
		Stderr        string           `json:"stderr"`
		ExitCode      int              `json:"exit_code"`
		Detail        string           `json:"detail"`
		TimeUsedMs    float64          `json:"time_used_ms"`
		MemoryUsedKb  float64          `json:"memory_used_kb"`
		Artifacts     []map[string]any `json:"artifacts"`
		Result        map[string]any   `json:"result"`
		ResourceLimit string           `json:"resource_limit_type"`
		UnauthType    string           `json:"unauthorized_access_type"`
		RuntimeErr    string           `json:"runtime_error_type"`
		Other         map[string]any   `json:"-"`
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("self_managed: decode response: %w", err)
	}

	// executor_manager already extracts the structured result on
	// its end (it has the same `__RAGFLOW_RESULT__` parser); we
	// still run ExtractStructuredResult on stdout as a defense in
	// depth — if the server-side parser is ever bypassed (direct
	// container exec), the Go side still gets the value.
	stdout, structured := ExtractStructuredResult(raw.Stdout)

	// Prefer the server-side result whenever it is present in the
	// HTTP payload. executor_manager already parsed the canonical
	// result marker; this is the most reliable source of truth.
	if len(raw.Result) > 0 {
		structured = raw.Result
	}

	metadata := map[string]any{
		"status":              raw.Status,
		"time_used_ms":        raw.TimeUsedMs,
		"memory_used_kb":      raw.MemoryUsedKb,
		"detail":              raw.Detail,
		"instance_id":         instanceIDOrEmpty(inst),
		"artifacts":           raw.Artifacts,
		"resource_limit_type": raw.ResourceLimit,
		"unauthorized_access": raw.UnauthType,
		"runtime_error_type":  raw.RuntimeErr,
		"structured_result":   structured,
	}
	common.Debug("CodeExec self_managed",
		zap.Any("http_result", raw.Result),
		zap.Any("structured_result", structured),
		zap.String("stdout", stdout),
		zap.String("stderr", raw.Stderr),
		zap.Int("exit_code", raw.ExitCode))

	return &ExecutionResult{
		Stdout:        stdout,
		Stderr:        raw.Stderr,
		ExitCode:      raw.ExitCode,
		ExecutionTime: time.Since(start).Seconds(),
		Metadata:      metadata,
	}, nil
}

// DestroyInstance is a no-op for self-managed. The executor_manager
// returns the container to its pool after each /run call. We return
// nil unconditionally, matching the Python implementation.
func (p *SelfManagedProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return fmt.Errorf("self_managed: provider not initialized")
	}
	return nil
}

// HealthCheck GETs {endpoint}/healthz.
func (p *SelfManagedProvider) HealthCheck(ctx context.Context) error {
	resp, err := p.healthHelper.Do(ctx, http.MethodGet, p.endpoint+"/healthz", "", "", nil)
	if err != nil {
		return fmt.Errorf("self_managed: healthz: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("self_managed: healthz returned %d", resp.StatusCode)
	}
	return nil
}

func (p *SelfManagedProvider) isInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
}

func instanceIDOrEmpty(inst *SandboxInstance) string {
	if inst == nil {
		return ""
	}
	return inst.InstanceID
}
