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

// kosa8 runs each CodeExec call in a disposable kosa8 microVM sandbox:
// CreateInstance starts a sandbox from the template for the language
// (`python` or `node` by default), ExecuteCode writes the shared
// RAGFlow wrapper into the sandbox's /workspace and runs it with
// `python3` or `node`, and DestroyInstance deletes the sandbox.
//
// It speaks the kosa8 REST API (https://api.kosa8.com, or a
// self-hosted deployment) over plain HTTPS with the standard library
// net/http client, so it adds no module dependency. Only /workspace is
// shared between commands of a sandbox, so the per-instance working
// directory and its artifacts directory live there. Artifacts are
// collected with the files API using the same allowlist and result
// shape as the local, ssh, tenki and ucloud providers.

package sandbox

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"ragflow/internal/common"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	kosa8DefaultBaseURL        = "https://api.kosa8.com"
	kosa8DefaultPythonTemplate = "python"
	kosa8DefaultNodeTemplate   = "node"
	kosa8Workspace             = "/workspace"
	kosa8DefaultTimeout        = 30
	kosa8DefaultMaxLifetime    = 3600
	kosa8DefaultMaxOutput      = 1 << 20
	kosa8DefaultMaxArtifacts   = 20
	kosa8DefaultArtifactBytes  = 10 << 20
	kosa8MaxArtifactDepth      = 16
	// kosa8BufferedExecLimit is the longest command kosa8 answers
	// buffered; longer timeouts use the streamed (NDJSON) exec.
	kosa8BufferedExecLimit = 90
	// kosa8ExecWriteChunk keeps one exec argument under the API's 8192
	// character limit once base64-encoded (6000 bytes -> 8000 chars).
	kosa8ExecWriteChunk = 6000
	kosa8StartPoll      = time.Second
	kosa8StartTimeout   = 5 * time.Minute
	// A DELETE that fails with a network error, 429 or 5xx is tried
	// kosa8DeleteAttempts times in all, with jittered exponential backoff.
	kosa8DeleteAttempts   = 3
	kosa8DeleteBackoff    = 500 * time.Millisecond
	kosa8DeleteMaxBackoff = 3 * time.Second
	// kosa8ExecEnvelope is the room a buffered exec answer gets beyond
	// its escaped stdout and stderr: the exit status, keys and quotes.
	kosa8ExecEnvelope = 64 << 10
)

// kosa8InWorkdir enters the working directory first: the exec API has
// no working-directory field.
var kosa8InWorkdir = []string{"sh", "-c", `cd -- "$1" || exit 1; shift; exec "$@"`, "ragflow"}

// kosa8Timed runs a command under the sandbox's own `timeout` (GNU or
// busybox), for hosts that refuse timeout_s on a streamed exec. It runs
// as a child of the shell, not in its place: the shell can be the
// container's PID 1, and a PID 1 without a handler ignores SIGTERM. A
// template without `timeout` gets kosa8NoTimeoutExit and
// kosa8NoTimeoutMarker on stderr instead of an untimed run.
var kosa8Timed = []string{"sh", "-c", `t=$1; shift; if ! command -v timeout >/dev/null 2>&1; then echo "` + kosa8NoTimeoutMarker + `" >&2; exit ` + strconv.Itoa(kosa8NoTimeoutExit) + `; fi; timeout "$t" "$@"; exit $?`, "ragflow"}

const (
	kosa8NoTimeoutMarker = "ragflow-kosa8: no timeout command in this sandbox"
	kosa8NoTimeoutExit   = 127
)

// kosa8StreamGrace is how long past its timeout a streamed command is
// waited for before the client gives up on it.
const kosa8StreamGrace = 30 * time.Second

type kosa8Instance struct {
	sandboxID     string
	remoteWorkDir string
}

// Kosa8Provider executes code in disposable kosa8 sandboxes.
type Kosa8Provider struct {
	client           *http.Client
	apiKey           string
	baseURL          string
	pythonTemplate   string
	nodeTemplate     string
	allowInternet    bool
	timeoutSec       int
	maxLifetimeSec   int
	maxOutputBytes   int
	maxArtifacts     int
	maxArtifactBytes int

	// startPoll is how often a cold-booting sandbox is polled.
	startPoll time.Duration
	// deleteBackoff is the first wait before a failed DELETE is retried.
	deleteBackoff time.Duration

	mu          sync.RWMutex
	initialized bool
	filesAPI    bool
	// noStreamTimeout is set once a host refuses timeout_s on a
	// streamed exec (HTTP 501); commands are then timed in the sandbox.
	noStreamTimeout bool
	instances       map[string]kosa8Instance
}

// kosa8APIError is an error answered by the kosa8 API. Code is the
// API's stable error code, such as "payment_required" or "exec_timeout".
type kosa8APIError struct {
	Status  int
	Code    string
	Message string
	Body    map[string]any
}

func (e *kosa8APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("kosa8: HTTP %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("kosa8: HTTP %d: %s", e.Status, e.Message)
}

func newKosa8ProviderFromEnv() *Kosa8Provider {
	return newKosa8ProviderFromConfig(kosa8ConfigFromEnv())
}

// kosa8ConfigFromEnv builds a config map from the KOSA8_* env vars,
// mirroring the admin-panel settings JSON shape.
func kosa8ConfigFromEnv() map[string]any {
	return map[string]any{
		"api_key":         common.GetEnv(common.EnvKosa8APIKey),
		"base_url":        common.GetEnv(common.EnvKosa8APIURL),
		"python_template": common.GetEnv(common.EnvKosa8PythonTemplate),
		"node_template":   common.GetEnv(common.EnvKosa8NodeTemplate),
		"timeout":         common.GetEnv(common.EnvKosa8Timeout),
		"allow_internet":  common.GetEnv(common.EnvKosa8AllowInternet),
	}
}

func newKosa8ProviderFromConfig(cfg map[string]any) *Kosa8Provider {
	pythonTemplate := strings.TrimSpace(configString(cfg, "python_template"))
	if pythonTemplate == "" {
		pythonTemplate = kosa8DefaultPythonTemplate
	}
	nodeTemplate := strings.TrimSpace(configString(cfg, "node_template"))
	if nodeTemplate == "" {
		nodeTemplate = kosa8DefaultNodeTemplate
	}
	return &Kosa8Provider{
		apiKey:         strings.TrimSpace(configString(cfg, "api_key")),
		baseURL:        strings.TrimRight(strings.TrimSpace(configString(cfg, "base_url")), "/"),
		pythonTemplate: pythonTemplate,
		nodeTemplate:   nodeTemplate,
		// Outbound network is opt-in, as for tenki and ucloud: sandboxed
		// code reaches nothing unless allow_internet is "true".
		allowInternet:    strings.EqualFold(configString(cfg, "allow_internet"), "true"),
		timeoutSec:       configInt(cfg, "timeout", kosa8DefaultTimeout),
		maxLifetimeSec:   configInt(cfg, "max_lifetime", kosa8DefaultMaxLifetime),
		maxOutputBytes:   configInt(cfg, "max_output_bytes", kosa8DefaultMaxOutput),
		maxArtifacts:     configInt(cfg, "max_artifacts", kosa8DefaultMaxArtifacts),
		maxArtifactBytes: configInt(cfg, "max_artifact_bytes", kosa8DefaultArtifactBytes),
		startPoll:        kosa8StartPoll,
		deleteBackoff:    kosa8DeleteBackoff,
		filesAPI:         true,
		instances:        make(map[string]kosa8Instance),
	}
}

// ProviderType returns ProviderKosa8.
func (p *Kosa8Provider) ProviderType() ProviderType { return ProviderKosa8 }

// Initialize checks the configuration and builds the HTTP client. The
// API key comes from the admin config or KOSA8_API_KEY.
func (p *Kosa8Provider) Initialize(context.Context) error {
	if p.apiKey == "" {
		p.apiKey = strings.TrimSpace(common.GetEnv(common.EnvKosa8APIKey))
	}
	if p.apiKey == "" {
		return errors.New("kosa8: API key is required (set it in Admin > Sandbox Settings or KOSA8_API_KEY)")
	}
	if p.baseURL == "" {
		p.baseURL = strings.TrimRight(common.GetEnv(common.EnvKosa8APIURL), "/")
	}
	if p.baseURL == "" {
		p.baseURL = kosa8DefaultBaseURL
	}
	u, err := url.Parse(p.baseURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("kosa8: invalid API URL %q", p.baseURL)
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	// The API key travels in every request.
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return fmt.Errorf("kosa8: API URL must use https: %s", p.baseURL)
	}
	if p.timeoutSec <= 0 || p.maxLifetimeSec < 60 || p.maxOutputBytes <= 0 || p.maxArtifactBytes <= 0 || p.maxArtifacts < 0 {
		return errors.New("kosa8: timeout and size limits must be positive, max_lifetime at least 60 seconds, and max_artifacts non-negative")
	}
	// Per-call deadlines come from the context; this bounds the longest
	// call, a streamed exec of the maximum timeout. The package's
	// retrying HTTPClient is not used: it turns a 5xx answer into a bare
	// error, and this provider acts on the API's codes (501 for a host
	// without the files API, 504 exec_timeout).
	p.client = &http.Client{Timeout: 11 * time.Minute}
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the languages of the python and node templates.
func (p *Kosa8Provider) SupportedLanguages() []string {
	return []string{"python", "javascript"}
}

// CreateInstance starts a kosa8 sandbox from the template for the
// language hint, applies its egress policy and makes its working
// directory. A sandbox that fails to get ready is deleted.
func (p *Kosa8Provider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, errors.New("kosa8: provider not initialized")
	}
	language := normalizeLanguage(template)
	if language == "" {
		return nil, fmt.Errorf("kosa8: unsupported language %q", template)
	}
	kosa8Template := p.pythonTemplate
	if language == "nodejs" {
		kosa8Template = p.nodeTemplate
	}
	var created struct {
		Sandbox map[string]any `json:"sandbox"`
	}
	err := p.call(ctx, http.MethodPost, "/v1/sandboxes", map[string]any{
		"template": kosa8Template,
		// The server-side backstop that reclaims a sandbox whose
		// DestroyInstance never ran.
		"ttl_s": p.maxLifetimeSec,
	}, &created)
	if err != nil {
		return nil, fmt.Errorf("kosa8: create sandbox: %w", err)
	}
	sandboxID, _ := created.Sandbox["id"].(string)
	if sandboxID == "" {
		return nil, errors.New("kosa8: create sandbox: answer has no sandbox id")
	}
	remoteWorkDir := path.Join(kosa8Workspace, "ragflow-codeexec-"+uuid.NewString())
	if err := p.prepare(ctx, sandboxID, created.Sandbox, remoteWorkDir); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		_ = p.deleteSandbox(cleanupCtx, sandboxID)
		cancel()
		return nil, err
	}
	p.mu.Lock()
	p.instances[sandboxID] = kosa8Instance{sandboxID: sandboxID, remoteWorkDir: remoteWorkDir}
	p.mu.Unlock()
	return &SandboxInstance{
		InstanceID: sandboxID,
		Provider:   ProviderKosa8,
		Status:     "running",
		Metadata: map[string]any{
			"language":        language,
			"template":        kosa8Template,
			"remote_work_dir": remoteWorkDir,
			"sandbox_id":      sandboxID,
			"max_lifetime":    p.maxLifetimeSec,
		},
	}, nil
}

// prepare waits for a cold-booting sandbox, sets its labels and
// egress policy, and makes the per-instance artifacts directory.
func (p *Kosa8Provider) prepare(ctx context.Context, sandboxID string, info map[string]any, remoteWorkDir string) error {
	deadline := time.Now().Add(kosa8StartTimeout)
	for {
		state, _ := info["state"].(string)
		if state == "running" {
			break
		}
		if state != "creating" && state != "" {
			reason, _ := info["last_error"].(string)
			if reason == "" {
				reason = "it is " + state
			}
			return fmt.Errorf("kosa8: sandbox %s did not start: %s", sandboxID, reason)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("kosa8: sandbox %s still starting after %s", sandboxID, kosa8StartTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.startPoll):
		}
		var got struct {
			Sandbox map[string]any `json:"sandbox"`
		}
		if err := p.call(ctx, http.MethodGet, p.sandboxPath(sandboxID), nil, &got); err != nil {
			return fmt.Errorf("kosa8: wait for sandbox: %w", err)
		}
		info = got.Sandbox
	}
	if err := p.call(ctx, http.MethodPut, p.sandboxPath(sandboxID)+"/labels",
		map[string]any{"labels": map[string]string{"created-by": "ragflow"}}, nil); err != nil {
		return fmt.Errorf("kosa8: label sandbox: %w", err)
	}
	if err := p.call(ctx, http.MethodPut, p.sandboxPath(sandboxID)+"/egress",
		map[string]any{"internet": p.allowInternet}, nil); err != nil {
		return fmt.Errorf("kosa8: set egress policy: %w", err)
	}
	res, err := p.exec(ctx, sandboxID, []string{"mkdir", "-p", path.Join(remoteWorkDir, "artifacts")}, minTimeout(p.timeoutSec, 30))
	if err != nil {
		return fmt.Errorf("kosa8: create workspace: %w", err)
	}
	if res.exit != 0 {
		return fmt.Errorf("kosa8: create workspace: exit %d: %s", res.exit, strings.TrimSpace(res.stderr))
	}
	return nil
}

// ExecuteCode writes the shared RAGFlow wrapper into the sandbox and
// runs it in the instance's working directory.
func (p *Kosa8Provider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, errors.New("kosa8: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, errors.New("kosa8: instance id required")
	}
	p.mu.RLock()
	instance, ok := p.instances[inst.InstanceID]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("kosa8: unknown instance %q", inst.InstanceID)
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("kosa8: unsupported language %q", language)
	}
	if timeoutSec == 0 {
		timeoutSec = p.timeoutSec
	}
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	timeout = min(timeout, p.timeoutSec)
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var scriptName, script, executable string
	if lang == "python" {
		scriptName, script, executable = "main.py", BuildPythonWrapper(code, argsJSON), "python3"
	} else {
		scriptName, script, executable = "main.js", BuildJavaScriptWrapper(code, argsJSON), "node"
	}
	scriptPath := path.Join(instance.remoteWorkDir, scriptName)
	if err := p.writeFile(ctx, instance.sandboxID, scriptPath, []byte(script)); err != nil {
		return nil, fmt.Errorf("kosa8: write script: %w", err)
	}

	argv := append(slices.Clone(kosa8InWorkdir), instance.remoteWorkDir, executable, scriptName)
	started := time.Now()
	res, err := p.exec(ctx, instance.sandboxID, argv, timeout)
	if err != nil {
		var apiErr *kosa8APIError
		if errors.As(err, &apiErr) && apiErr.Code == "exec_timeout" {
			return nil, fmt.Errorf("kosa8: execution timed out after %d seconds", timeout)
		}
		// Do not include argv: it names the user's script only, but
		// the error may echo the request.
		return nil, fmt.Errorf("kosa8: execute code: %w", err)
	}
	if len(res.stdout)+len(res.stderr) > p.maxOutputBytes {
		return nil, fmt.Errorf("kosa8: execution output exceeded %d bytes", p.maxOutputBytes)
	}
	stdout, structured := ExtractStructuredResult(res.stdout)
	result := &ExecutionResult{
		Stdout:        stdout,
		Stderr:        res.stderr,
		ExitCode:      res.exit,
		ExecutionTime: time.Since(started).Seconds(),
		Metadata: map[string]any{
			"instance_id":       inst.InstanceID,
			"sandbox_id":        instance.sandboxID,
			"language":          lang,
			"script_path":       scriptPath,
			"remote_work_dir":   instance.remoteWorkDir,
			"timeout":           timeout,
			"structured_result": structured,
		},
	}
	artifacts := make([]map[string]any, 0)
	if p.usesFilesAPI() {
		if err := p.collectArtifacts(ctx, instance.sandboxID, path.Join(instance.remoteWorkDir, "artifacts"), "", &artifacts, 0); err != nil {
			var apiErr *kosa8APIError
			if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotImplemented {
				return nil, err
			}
			p.noFilesAPI()
		}
	}
	if !p.usesFilesAPI() {
		result.Metadata["artifacts_error"] = "kosa8: this sandbox's host has no files API; artifacts are not collected"
	}
	result.Metadata["artifacts"] = artifacts
	return result, nil
}

// DestroyInstance deletes the sandbox. A sandbox that is already gone
// counts as deleted, so the call is idempotent. The instance is
// forgotten only once the sandbox is deleted: after a failed DELETE a
// later DestroyInstance tries again.
func (p *Kosa8Provider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return errors.New("kosa8: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return errors.New("kosa8: instance id required")
	}
	p.mu.RLock()
	instance, ok := p.instances[inst.InstanceID]
	p.mu.RUnlock()
	if !ok {
		return nil
	}
	if err := p.deleteSandbox(ctx, instance.sandboxID); err != nil {
		return fmt.Errorf("kosa8: delete sandbox %s: %w", instance.sandboxID, err)
	}
	p.mu.Lock()
	delete(p.instances, inst.InstanceID)
	p.mu.Unlock()
	return nil
}

// HealthCheck probes the kosa8 API with the configured key.
func (p *Kosa8Provider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized() {
		return errors.New("kosa8: provider not initialized")
	}
	if err := p.call(ctx, http.MethodGet, "/v1/me", nil, nil); err != nil {
		return fmt.Errorf("kosa8: health check: %w", err)
	}
	return nil
}

// deleteSandbox deletes a sandbox, retrying a network error, 429 or 5xx.
// A sandbox that is already gone (404 or 410) counts as deleted.
func (p *Kosa8Provider) deleteSandbox(ctx context.Context, sandboxID string) error {
	var err error
	for attempt := 1; ; attempt++ {
		err = p.call(ctx, http.MethodDelete, p.sandboxPath(sandboxID), nil, nil)
		var apiErr *kosa8APIError
		if errors.As(err, &apiErr) && (apiErr.Status == http.StatusNotFound || apiErr.Status == http.StatusGone) {
			return nil
		}
		if err == nil || attempt >= kosa8DeleteAttempts || !kosa8Transient(err) || ctx.Err() != nil {
			return err
		}
		sleepCtx(ctx, backoff(p.deleteBackoff, kosa8DeleteMaxBackoff, attempt))
	}
}

// kosa8Transient reports whether a failed request may succeed if sent
// again: a network error, HTTP 429 or a 5xx answer.
func kosa8Transient(err error) bool {
	var apiErr *kosa8APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == http.StatusTooManyRequests || apiErr.Status >= 500
	}
	return isRetryableNetError(err)
}

// writeFile writes through the files API, or through exec on a host
// without it.
func (p *Kosa8Provider) writeFile(ctx context.Context, sandboxID, filePath string, content []byte) error {
	if p.usesFilesAPI() {
		err := p.do(ctx, http.MethodPut, p.sandboxPath(sandboxID)+"/files?path="+url.QueryEscape(filePath),
			string(content), "application/octet-stream", nil)
		var apiErr *kosa8APIError
		if err == nil || !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotImplemented {
			return err
		}
		p.noFilesAPI()
	}
	for offset := 0; offset == 0 || offset < len(content); offset += kosa8ExecWriteChunk {
		end := min(offset+kosa8ExecWriteChunk, len(content))
		redirect := ">>"
		if offset == 0 {
			redirect = ">"
		}
		script := `printf %s "$2" | base64 -d ` + redirect + ` "$1"`
		res, err := p.exec(ctx, sandboxID, []string{"sh", "-c", script, "ragflow", filePath,
			base64.StdEncoding.EncodeToString(content[offset:end])}, minTimeout(p.timeoutSec, 30))
		if err != nil {
			return err
		}
		if res.exit != 0 {
			return fmt.Errorf("exit %d: %s", res.exit, strings.TrimSpace(res.stderr))
		}
	}
	return nil
}

type kosa8ExecResult struct {
	stdout string
	stderr string
	exit   int
}

// exec runs argv with the API's own timeout. Up to the buffered limit
// the answer is one JSON object; longer timeouts are streamed.
func (p *Kosa8Provider) exec(ctx context.Context, sandboxID string, argv []string, timeoutSec int) (*kosa8ExecResult, error) {
	body := map[string]any{"cmd": argv, "timeout_s": timeoutSec}
	if timeoutSec > kosa8BufferedExecLimit {
		return p.execLong(ctx, sandboxID, argv, timeoutSec)
	}
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec+30)*time.Second)
	defer cancel()
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := p.send(callCtx, http.MethodPost, p.sandboxPath(sandboxID)+"/exec", string(payload), "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, kosa8ErrorFrom(resp)
	}
	// The answer is bounded before it is decoded. JSON escapes one byte
	// of output into at most six (\u00XX), so the cap allows six bytes
	// per byte of max_output_bytes plus the envelope; ExecuteCode then
	// checks the decoded size against max_output_bytes itself.
	limit := p.bufferedExecLimit()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("kosa8: read exec answer: %w", err)
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("kosa8: execution output exceeded %d bytes", p.maxOutputBytes)
	}
	var answer struct {
		Exit   *int   `json:"exit"`
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, fmt.Errorf("kosa8: decode exec answer: %w", err)
	}
	if answer.Exit == nil {
		return nil, errors.New("kosa8: command ended without an exit status")
	}
	return &kosa8ExecResult{stdout: answer.Stdout, stderr: answer.Stderr, exit: *answer.Exit}, nil
}

// execLong runs a command past the buffered limit as a streamed exec.
// A host that refuses timeout_s there (HTTP 501) gets the command
// without it, wrapped in kosa8Timed, and the client stops waiting
// kosa8StreamGrace after the deadline.
func (p *Kosa8Provider) execLong(ctx context.Context, sandboxID string, argv []string, timeoutSec int) (*kosa8ExecResult, error) {
	p.mu.RLock()
	timed := !p.noStreamTimeout
	p.mu.RUnlock()
	if timed {
		res, err := p.execStreamed(ctx, sandboxID, map[string]any{"cmd": argv, "timeout_s": timeoutSec})
		var apiErr *kosa8APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotImplemented || !strings.Contains(apiErr.Message, "timeout_s") {
			return res, err
		}
		p.mu.Lock()
		p.noStreamTimeout = true
		p.mu.Unlock()
	}
	cmd := append(slices.Clone(kosa8Timed), strconv.Itoa(timeoutSec))
	cmd = append(cmd, argv...)
	deadline := time.Duration(timeoutSec) * time.Second
	callCtx, cancel := context.WithTimeout(ctx, deadline+kosa8StreamGrace)
	defer cancel()
	started := time.Now()
	res, err := p.execStreamed(callCtx, sandboxID, map[string]any{"cmd": cmd})
	if err != nil {
		if ctx.Err() == nil && callCtx.Err() != nil {
			return nil, &kosa8APIError{Status: http.StatusGatewayTimeout, Code: "exec_timeout", Message: "command did not end after its timeout"}
		}
		return nil, err
	}
	if res.exit == kosa8NoTimeoutExit && strings.Contains(res.stderr, kosa8NoTimeoutMarker) {
		return nil, fmt.Errorf("kosa8: the sandbox template has no `timeout` command, which this kosa8 host needs to time a command longer than %d seconds; use a template that provides `timeout` (coreutils or busybox)", kosa8BufferedExecLimit)
	}
	// GNU timeout exits 124; busybox's ends the command with SIGTERM (143).
	if (res.exit == 124 || res.exit == 143) && time.Since(started) >= deadline {
		return nil, &kosa8APIError{Status: http.StatusGatewayTimeout, Code: "exec_timeout", Message: "command timed out"}
	}
	return res, nil
}

// execStreamed reads NDJSON frames: {stream, data: base64} while the
// command prints (empty ones are heartbeats), then {done: true, exit}
// or {done: true, error, code}.
func (p *Kosa8Provider) execStreamed(ctx context.Context, sandboxID string, body map[string]any) (*kosa8ExecResult, error) {
	body["stream"] = true
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := p.send(ctx, http.MethodPost, p.sandboxPath(sandboxID)+"/exec", string(payload), "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, kosa8ErrorFrom(resp)
	}
	var stdout, stderr strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var frame struct {
			Stream string `json:"stream"`
			Data   string `json:"data"`
			Done   bool   `json:"done"`
			Exit   *int   `json:"exit"`
			Error  string `json:"error"`
			Code   string `json:"code"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			return nil, fmt.Errorf("kosa8: bad exec frame: %w", err)
		}
		if frame.Done {
			if frame.Code == "exec_timeout" {
				return nil, &kosa8APIError{Status: http.StatusGatewayTimeout, Code: "exec_timeout", Message: frame.Error}
			}
			if frame.Error != "" {
				return nil, fmt.Errorf("kosa8: %s", frame.Error)
			}
			if frame.Exit == nil {
				return nil, errors.New("kosa8: command ended without an exit status")
			}
			return &kosa8ExecResult{stdout: stdout.String(), stderr: stderr.String(), exit: *frame.Exit}, nil
		}
		if frame.Data == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			return nil, fmt.Errorf("kosa8: bad exec frame data: %w", err)
		}
		if frame.Stream == "stderr" {
			stderr.Write(data)
		} else {
			stdout.Write(data)
		}
		if p.maxOutputBytes > 0 && stdout.Len()+stderr.Len() > p.maxOutputBytes {
			return nil, fmt.Errorf("kosa8: execution output exceeded %d bytes", p.maxOutputBytes)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("kosa8: read exec stream: %w", err)
	}
	return nil, errors.New("kosa8: exec stream ended without a result")
}

type kosa8FileInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

func (p *Kosa8Provider) collectArtifacts(ctx context.Context, sandboxID, currentDir, relativeDir string, artifacts *[]map[string]any, depth int) error {
	if depth > kosa8MaxArtifactDepth {
		return fmt.Errorf("kosa8: artifact directory nesting exceeds %d levels: %s", kosa8MaxArtifactDepth, relativeDir)
	}
	var listing struct {
		Files []kosa8FileInfo `json:"files"`
	}
	err := p.call(ctx, http.MethodGet, p.sandboxPath(sandboxID)+"/files/list?path="+url.QueryEscape(currentDir), nil, &listing)
	var apiErr *kosa8APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	entries := listing.Files
	slices.SortFunc(entries, func(a, b kosa8FileInfo) int { return strings.Compare(a.Name, b.Name) })
	for _, entry := range entries {
		name := path.Base(entry.Name)
		if name == "." || name == ".." || name != entry.Name {
			return fmt.Errorf("kosa8: invalid artifact entry name: %s", entry.Name)
		}
		remotePath := path.Join(currentDir, name)
		relativePath := path.Join(relativeDir, name)
		switch entry.Type {
		case "dir":
			if err := p.collectArtifacts(ctx, sandboxID, remotePath, relativePath, artifacts, depth+1); err != nil {
				return err
			}
			continue
		case "file":
		default:
			return fmt.Errorf("kosa8: artifact symlinks and special files are not allowed: %s", relativePath)
		}
		if len(*artifacts) >= p.maxArtifacts {
			return fmt.Errorf("kosa8: execution produced more than %d artifacts", p.maxArtifacts)
		}
		if entry.Size > int64(p.maxArtifactBytes) {
			return fmt.Errorf("kosa8: artifact exceeds %d bytes: %s", p.maxArtifactBytes, relativePath)
		}
		ext := strings.ToLower(path.Ext(name))
		if _, ok := allowedArtifactExts[ext]; !ok {
			return fmt.Errorf("kosa8: unsupported artifact type: %s", relativePath)
		}
		content, err := p.readFile(ctx, sandboxID, remotePath)
		if err != nil {
			return fmt.Errorf("kosa8: read artifact %s: %w", relativePath, err)
		}
		if len(content) > p.maxArtifactBytes {
			return fmt.Errorf("kosa8: artifact exceeds %d bytes: %s", p.maxArtifactBytes, relativePath)
		}
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		*artifacts = append(*artifacts, map[string]any{
			"name":        relativePath,
			"content_b64": base64.StdEncoding.EncodeToString(content),
			"mime_type":   mimeType,
			"size":        int64(len(content)),
		})
	}
	return nil
}

func (p *Kosa8Provider) readFile(ctx context.Context, sandboxID, filePath string) ([]byte, error) {
	resp, err := p.send(ctx, http.MethodGet, p.sandboxPath(sandboxID)+"/files?path="+url.QueryEscape(filePath), "", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, kosa8ErrorFrom(resp)
	}
	return io.ReadAll(io.LimitReader(resp.Body, int64(p.maxArtifactBytes)+1))
}

// call sends body as JSON and decodes a JSON answer into out (when
// non-nil). An HTTP error becomes a *kosa8APIError.
func (p *Kosa8Provider) call(ctx context.Context, method, apiPath string, body any, out any) error {
	payload := ""
	contentType := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload, contentType = string(b), "application/json"
	}
	return p.do(ctx, method, apiPath, payload, contentType, out)
}

func (p *Kosa8Provider) do(ctx context.Context, method, apiPath, body, contentType string, out any) error {
	resp, err := p.send(ctx, method, apiPath, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return kosa8ErrorFrom(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("kosa8: decode %s %s answer: %w", method, apiPath, err)
	}
	return nil
}

// send makes one authenticated request; the caller closes the body.
func (p *Kosa8Provider) send(ctx context.Context, method, apiPath, body, contentType string) (*http.Response, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+apiPath, reader)
	if err != nil {
		return nil, fmt.Errorf("kosa8: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ragflow-kosa8")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return p.client.Do(req)
}

func (p *Kosa8Provider) sandboxPath(sandboxID string) string {
	return "/v1/sandboxes/" + url.PathEscape(sandboxID)
}

// kosa8ErrorFrom reads the API's {error: code, message} answer.
func kosa8ErrorFrom(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	apiErr := &kosa8APIError{Status: resp.StatusCode}
	var body map[string]any
	if json.Unmarshal(raw, &body) == nil {
		apiErr.Body = body
		apiErr.Code, _ = body["error"].(string)
		apiErr.Message, _ = body["message"].(string)
	}
	if apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(raw))
		if len(apiErr.Message) > 500 {
			apiErr.Message = apiErr.Message[:500]
		}
	}
	return apiErr
}

// bufferedExecLimit is the most bytes a buffered exec answer may have:
// stdout and stderr of max_output_bytes in all, each byte escaped to at
// most six, plus the envelope.
func (p *Kosa8Provider) bufferedExecLimit() int64 {
	return 6*int64(max(p.maxOutputBytes, 0)) + kosa8ExecEnvelope
}

func (p *Kosa8Provider) usesFilesAPI() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.filesAPI
}

func (p *Kosa8Provider) noFilesAPI() {
	p.mu.Lock()
	p.filesAPI = false
	p.mu.Unlock()
}

func (p *Kosa8Provider) isInitialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized
}
