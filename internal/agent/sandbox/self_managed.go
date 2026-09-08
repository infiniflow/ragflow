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
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"

	"github.com/google/uuid"
	"go.uber.org/zap"
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
	// apiToken is the shared secret authenticating this RAGFlow
	// instance towards the executor_manager's /run endpoint
	// (Authorization: Bearer). Empty means the executor_manager runs
	// without authentication (backwards compatibility).
	apiToken string
}

// newSelfManagedProviderFromEnv reads SANDBOX_EXECUTOR_MANAGER_URL
// (default: http://sandbox-executor-manager:9385) and
// SANDBOX_EXECUTOR_MANAGER_TIMEOUT (default 30s) and returns a
// provider ready for Initialize. The per-language base image
// overrides (SANDBOX_BASE_PYTHON_IMAGE / SANDBOX_BASE_NODEJS_IMAGE)
// are also read; empty values mean "use executor_manager's
// default image" — no override.
func newSelfManagedProviderFromEnv() *SelfManagedProvider {
	return newSelfManagedProviderFromConfig(map[string]any{})
}

// The provider resolves each field through one canonical chain: the
// lowercase admin-panel settings schema first (endpoint, timeout,
// max_retries, pool_size, api_token — the exact schema
// agent/sandbox/providers/self_managed.py persists and reads; pool_size
// additionally accepts its executor_manager_pool_size spelling, which the
// Python provider reads first), then the SANDBOX_* environment variable for
// fields absent from persisted settings, then the built-in default. Keeping
// a single resolution path on both runtimes means a settings row plus
// environment variables configures Python and Go identically.
func configStringEnv(cfg map[string]any, envName string, keys ...string) string {
	for _, key := range keys {
		// Blank-after-trim values count as absent so they fall through to
		// the environment, matching the Python provider's
		// `config.get(...) or env` resolution.
		if value := strings.TrimSpace(configString(cfg, key)); value != "" {
			return value
		}
	}
	return common.GetEnv(envName)
}

func configIntEnv(cfg map[string]any, fallback int, envName string, keys ...string) int {
	for _, key := range keys {
		if _, ok := cfg[key]; ok && cfg[key] != nil {
			return configInt(cfg, key, fallback)
		}
	}
	if envValue := common.GetEnv(envName); envValue != "" {
		if parsed, err := strconv.Atoi(envValue); err == nil {
			return parsed
		}
	}
	return fallback
}

func configDurationEnv(cfg map[string]any, fallback time.Duration, envName string, keys ...string) time.Duration {
	for _, key := range keys {
		if _, ok := cfg[key]; ok && cfg[key] != nil {
			return configDuration(cfg, key, fallback)
		}
	}
	if envValue := common.GetEnv(envName); envValue != "" {
		if parsed, err := time.ParseDuration(envValue); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

// newSelfManagedProviderFromConfig builds the provider from the persisted
// settings JSON (the `sandbox.self_managed` row written by the admin panel,
// lowercase keys: endpoint, timeout, max_retries, pool_size, api_token,
// optionally base_python_image / base_nodejs_image). Fields absent from the
// settings fall back to their SANDBOX_* environment variable before the
// built-in default, mirroring the Python provider's config-over-env
// resolution.
func newSelfManagedProviderFromConfig(cfg map[string]any) *SelfManagedProvider {
	endpoint := configStringEnv(cfg, common.EnvSandboxExecutorManagerURL, "endpoint")
	if endpoint == "" {
		endpoint = selfManagedDefaultEndpoint
	}
	endpoint = strings.TrimRight(endpoint, "/")

	timeout := configDurationEnv(cfg, 30*time.Second, common.EnvSandboxExecutorManagerTimeout, "timeout")
	poolSize := configIntEnv(cfg, 3, common.EnvSandboxExecutorManagerPoolSize, "executor_manager_pool_size", "pool_size")
	// max_retries maps to the HTTP client's total attempt count (first try
	// included), matching its default of 3 attempts.
	maxRetries := configIntEnv(cfg, 3, common.EnvSandboxExecutorManagerMaxRetries, "max_retries")

	// Per-language base image overrides. Empty = executor_manager
	// default; non-empty = the operator picked a custom base image
	// (typically a heavier Python image with torch/tensorflow
	// pre-installed, or a node image with native deps for native
	// addons).
	baseImages := map[string]string{
		"python": configStringEnv(cfg, common.EnvSandboxBasePythonImage, "base_python_image"),
		"nodejs": configStringEnv(cfg, common.EnvSandboxBaseNodeJSImage, "base_nodejs_image"),
	}

	apiToken := strings.TrimSpace(configStringEnv(cfg, common.EnvSandboxExecutorManagerAPIToken, "api_token"))

	return &SelfManagedProvider{
		endpoint:   endpoint,
		timeout:    timeout,
		poolSize:   poolSize,
		baseImages: baseImages,
		apiToken:   apiToken,
		helper: NewHTTPClient(HTTPConfig{
			Timeout:     timeout,
			MaxAttempts: maxRetries,
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
	// Shared-secret authentication towards the executor_manager; empty
	// token keeps the request unauthenticated (backwards compatibility).
	var reqHeaders map[string]string
	if p.apiToken != "" {
		reqHeaders = map[string]string{"Authorization": "Bearer " + p.apiToken}
	}
	resp, err := p.helper.Do(ctx, http.MethodPost, p.endpoint+"/run", string(body), "application/json", reqHeaders)
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
