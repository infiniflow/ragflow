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

// tenki.go is the Go port of
// `agent/sandbox/providers/tenki.py::TenkiProvider`. It runs each
// CodeExec call in a disposable Tenki microVM: CreateInstance
// provisions a fresh sandbox, ExecuteCode runs `python3 -c <wrapped>`
// or `node -e <wrapped>` inside it, and DestroyInstance terminates it.
// The provider uses only Tenki's create/exec/destroy operations; it
// does not use volumes or snapshots.
//
// The code-wrapping protocol is shared with SelfManaged, Aliyun and
// e2b, so the `__RAGFLOW_RESULT__:` marker extraction works uniformly.
// Artifacts are collected from a per-instance workspace using the same
// allowlist and result shape as the local/ssh providers.
//
// SDK: github.com/LuxorLabs/tenki-sdk-go/sandbox (MIT). The auth
// token and API URL are read from env by Initialize; image and
// tunables come from the admin-panel config map. Sandbox scope is
// inferred from the API key by the SDK.

package sandbox

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"path"
	"ragflow/internal/common"
	"slices"
	"strings"
	"sync"
	"time"

	tenkisdk "github.com/LuxorLabs/tenki-sdk-go/sandbox"
)

const (
	tenkiDefaultTimeout       = 30 * time.Second
	tenkiDefaultMaxLifetime   = time.Hour
	tenkiMaxArtifactDepth     = 16
	tenkiDefaultMaxOutput     = 1 << 20
	tenkiDefaultMaxArtifacts  = 20
	tenkiDefaultArtifactBytes = 10 << 20
)

type tenkiInstance struct {
	session       *tenkisdk.Session
	remoteWorkDir string
}

// TenkiProvider is the Go port of the Python TenkiProvider.
type TenkiProvider struct {
	client           *tenkisdk.Client
	apiKey           string
	baseURL          string
	image            string
	allowOutbound    bool
	timeout          time.Duration
	maxLifetime      time.Duration
	cpuCores         int
	memoryMB         int
	diskSizeGB       int
	maxOutputBytes   int
	maxArtifacts     int
	maxArtifactBytes int

	mu          sync.Mutex
	initialized bool
	instancesMu sync.Mutex
	instances   map[string]tenkiInstance
}

// newTenkiProviderFromEnv reads TENKI_* env vars and returns a
// provider ready for Initialize.
func newTenkiProviderFromEnv() *TenkiProvider {
	return newTenkiProviderFromConfig(tenkiConfigFromEnv())
}

// tenkiConfigFromEnv builds a config map from the TENKI_* env vars,
// mirroring the admin-panel settings JSON shape.
func tenkiConfigFromEnv() map[string]any {
	return map[string]any{
		"api_key":        common.GetEnv(common.EnvTenkiAPIKey),
		"base_url":       common.GetEnv(common.EnvTenkiAPIURL),
		"image":          common.GetEnv(common.EnvTenkiImage),
		"timeout":        common.GetEnv(common.EnvTenkiTimeout),
		"allow_outbound": common.GetEnv(common.EnvTenkiAllowOutbound),
	}
}

// newTenkiProviderFromConfig builds the provider from a JSON config
// map (admin-panel settings or the env-backed map above).
func newTenkiProviderFromConfig(cfg map[string]any) *TenkiProvider {
	p := &TenkiProvider{
		apiKey:  configString(cfg, "api_key"),
		baseURL: configString(cfg, "base_url"),
		image:   configString(cfg, "image"),
		// Outbound network is opt-in: sandboxed code has no egress
		// unless ALLOW_OUTBOUND is explicitly "true". This matches
		// the self_managed sandbox, which treats network access as an
		// unauthorized-access event by default.
		allowOutbound:    configString(cfg, "allow_outbound") == "true",
		timeout:          time.Duration(configInt(cfg, "timeout", int(tenkiDefaultTimeout/time.Second))) * time.Second,
		maxLifetime:      time.Duration(configInt(cfg, "max_lifetime", int(tenkiDefaultMaxLifetime/time.Second))) * time.Second,
		cpuCores:         configInt(cfg, "cpu_cores", 0),
		memoryMB:         configInt(cfg, "memory_mb", 0),
		diskSizeGB:       configInt(cfg, "disk_size_gb", 0),
		maxOutputBytes:   configInt(cfg, "max_output_bytes", tenkiDefaultMaxOutput),
		maxArtifacts:     configInt(cfg, "max_artifacts", tenkiDefaultMaxArtifacts),
		maxArtifactBytes: configInt(cfg, "max_artifact_bytes", tenkiDefaultArtifactBytes),
		instances:        make(map[string]tenkiInstance),
	}
	return p
}

// ProviderType returns ProviderTenki.
func (p *TenkiProvider) ProviderType() ProviderType { return ProviderTenki }

// Initialize builds the Tenki SDK client. The auth token comes from
// the admin config or TENKI_API_KEY; we check explicitly so the
// manager does not register a broken provider.
func (p *TenkiProvider) Initialize(ctx context.Context) error {
	apiKey := p.apiKey
	if apiKey == "" {
		apiKey = common.GetEnv(common.EnvTenkiAPIKey)
	}
	if apiKey == "" {
		return errors.New("tenki: API key is required (set it in Admin > Sandbox Settings or TENKI_API_KEY)")
	}
	baseURL := p.baseURL
	if baseURL == "" {
		baseURL = common.GetEnv(common.EnvTenkiAPIURL)
	}
	opts := []tenkisdk.Option{tenkisdk.WithAuthToken(apiKey)}
	if p.timeout > 0 {
		opts = append(opts, tenkisdk.WithHTTPTimeout(p.timeout))
	}
	if baseURL != "" {
		opts = append(opts, tenkisdk.WithBaseURL(baseURL))
	}
	c, err := tenkisdk.New(opts...)
	if err != nil {
		return fmt.Errorf("tenki: build client: %w", err)
	}
	p.client = c
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the languages the default Tenki image can
// run. The default image ships with python3 and node.
func (p *TenkiProvider) SupportedLanguages() []string {
	return []string{"python", "javascript"}
}

// CreateInstance provisions a fresh Tenki sandbox. As with the e2b
// provider, the template argument is treated as the language hint; the
// actual base image comes from the configured image (empty = Tenki's
// default image). InstanceID is the Tenki session id.
func (p *TenkiProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("tenki: provider not initialized")
	}
	lang := normalizeLanguage(template)
	if lang == "" {
		return nil, fmt.Errorf("tenki: unsupported language %q", template)
	}
	opts := []tenkisdk.CreateOption{
		tenkisdk.WithAllowOutbound(p.allowOutbound),
		tenkisdk.WithMaxDuration(p.maxLifetime),
		tenkisdk.WithWaitTimeout(p.timeout),
		tenkisdk.WithMetadata(map[string]string{"source": "ragflow"}),
	}
	if p.cpuCores > 0 {
		opts = append(opts, tenkisdk.WithCPUCores(int32(p.cpuCores)))
	}
	if p.memoryMB > 0 {
		opts = append(opts, tenkisdk.WithMemoryMB(int32(p.memoryMB)))
	}
	if p.diskSizeGB > 0 {
		opts = append(opts, tenkisdk.WithDiskSizeGB(p.diskSizeGB))
	}
	if p.image != "" {
		opts = append(opts, tenkisdk.WithImage(p.image))
	}
	sess, err := p.client.Create(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("tenki: Create: %w", err)
	}
	remoteWorkDir := tenkiWorkspacePath(sess.ID)
	if err := sess.Mkdir(ctx, path.Join(remoteWorkDir, "artifacts")); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		_ = sess.Close(cleanupCtx)
		cancel()
		return nil, fmt.Errorf("tenki: create workspace: %w", err)
	}
	instanceID := sess.ID
	p.instancesMu.Lock()
	p.instances[instanceID] = tenkiInstance{session: sess, remoteWorkDir: remoteWorkDir}
	p.instancesMu.Unlock()
	return &SandboxInstance{
		InstanceID: instanceID,
		Provider:   ProviderTenki,
		Status:     "running",
		Metadata: map[string]any{
			"language":        lang,
			"image":           p.image,
			"remote_work_dir": remoteWorkDir,
			"max_lifetime":    p.maxLifetime.String(),
			"timeout":         p.timeout.String(),
		},
	}, nil
}

// ExecuteCode runs the user's code inside the sandbox via
// `python3 -c <wrapped>` (Python) or `node -e <wrapped>` (JS). The
// wrapped code carries the `__RAGFLOW_RESULT__:` marker so the
// structured main() return value comes back as before.
func (p *TenkiProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("tenki: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, fmt.Errorf("tenki: instance id required")
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("tenki: unsupported language %q", language)
	}
	if timeoutSec == 0 {
		timeoutSec = int(p.timeout.Seconds())
	}
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	timeout = min(timeout, int(p.timeout.Seconds()))

	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var wrapped, cmd string
	var runArgs []string
	if lang == "python" {
		cmd = "python3"
		wrapped = BuildPythonWrapper(code, argsJSON)
		runArgs = []string{"-c", wrapped}
	} else {
		cmd = "node"
		wrapped = BuildJavaScriptWrapper(code, argsJSON)
		runArgs = []string{"-e", wrapped}
	}

	p.instancesMu.Lock()
	instance, ok := p.instances[inst.InstanceID]
	p.instancesMu.Unlock()
	if !ok || instance.session == nil {
		return nil, fmt.Errorf("tenki: unknown instance %q", inst.InstanceID)
	}
	sess := instance.session

	start := time.Now()
	res, err := sess.Exec(ctx, cmd,
		tenkisdk.WithArgs(runArgs...),
		tenkisdk.WithDir(instance.remoteWorkDir),
		tenkisdk.WithTimeout(time.Duration(timeout)*time.Second),
	)
	if err != nil {
		// A non-zero exit is reported as a *Result, not an error;
		// a nil result here means a transport, timeout or session
		// error that we cannot map to stdout/stderr. Do not include
		// runArgs — they embed the user's code and arguments.
		return nil, fmt.Errorf("tenki: exec %s: %w", cmd, err)
	}
	if outputErr := validateTenkiOutputSize(string(res.Stdout), string(res.Stderr), p.maxOutputBytes); outputErr != nil {
		return nil, outputErr
	}
	result := buildTenkiExecutionResult(res, lang, start)
	artifacts, artifactErr := p.collectArtifacts(ctx, sess, path.Join(instance.remoteWorkDir, "artifacts"), 0)
	if artifactErr != nil {
		return nil, fmt.Errorf("tenki: collect artifacts: %w", artifactErr)
	}
	result.Metadata["artifacts"] = artifacts
	result.Metadata["remote_work_dir"] = instance.remoteWorkDir
	result.Metadata["timeout"] = timeout
	return result, nil
}

// buildTenkiExecutionResult maps the Tenki Result to our
// sandbox.ExecutionResult. Stdout is scanned (untrimmed) for the
// `__RAGFLOW_RESULT__:` marker so the model gets the structured
// main() return value.
func buildTenkiExecutionResult(r *tenkisdk.Result, lang string, start time.Time) *ExecutionResult {
	stdout, structured := ExtractStructuredResult(string(r.Stdout))
	return &ExecutionResult{
		Stdout:        stdout,
		Stderr:        string(r.Stderr),
		ExitCode:      int(r.ExitCode),
		ExecutionTime: time.Since(start).Seconds(),
		Metadata: map[string]any{
			"language":          lang,
			"structured_result": structured,
			"tenki_status":      string(r.Status),
		},
	}
}

// DestroyInstance terminates the sandbox. A session that is already
// gone (not found, terminated, or expired past its max duration) is
// treated as success so the call is idempotent.
func (p *TenkiProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return fmt.Errorf("tenki: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return fmt.Errorf("tenki: instance id required")
	}
	p.instancesMu.Lock()
	instance, ok := p.instances[inst.InstanceID]
	if ok {
		delete(p.instances, inst.InstanceID)
	}
	p.instancesMu.Unlock()
	if !ok || instance.session == nil {
		return nil
	}
	sess := instance.session
	if err := sess.Close(ctx); err != nil {
		if errors.Is(err, tenkisdk.ErrSessionNotFound) ||
			errors.Is(err, tenkisdk.ErrSessionTerminated) ||
			errors.Is(err, tenkisdk.ErrSessionExpired) {
			return nil
		}
		return fmt.Errorf("tenki: Close(%s): %w", inst.InstanceID, err)
	}
	return nil
}

// HealthCheck probes the Tenki control plane. There is no dedicated
// ping endpoint, so a successful WhoAmI is our probe.
func (p *TenkiProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized() {
		return errors.New("tenki: provider not initialized")
	}
	if _, err := p.client.WhoAmI(ctx); err != nil {
		return fmt.Errorf("tenki: WhoAmI: %w", err)
	}
	return nil
}

func (p *TenkiProvider) isInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
}

func validateTenkiOutputSize(stdout, stderr string, maxBytes int) error {
	if maxBytes > 0 && len(stdout)+len(stderr) > maxBytes {
		return fmt.Errorf("tenki: output exceeds %d bytes", maxBytes)
	}
	return nil
}

func tenkiWorkspacePath(sessionID string) string {
	return path.Join("/home/tenki", "ragflow-codeexec-"+sessionID)
}

func (p *TenkiProvider) collectArtifacts(ctx context.Context, sess *tenkisdk.Session, currentDir string, depth int) ([]map[string]any, error) {
	return p.collectArtifactsRecursive(ctx, sess, currentDir, "", depth, nil)
}

func (p *TenkiProvider) collectArtifactsRecursive(ctx context.Context, sess *tenkisdk.Session, currentDir, relativeDir string, depth int, artifacts []map[string]any) ([]map[string]any, error) {
	if depth > tenkiMaxArtifactDepth {
		return nil, fmt.Errorf("artifact directory nesting exceeds %d levels: %s", tenkiMaxArtifactDepth, currentDir)
	}
	entries, err := sess.List(ctx, currentDir)
	if errors.Is(err, tenkisdk.ErrFileNotFound) {
		return artifacts, nil
	}
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b tenkisdk.FileInfo) int {
		return strings.Compare(a.Path, b.Path)
	})

	if artifacts == nil {
		artifacts = make([]map[string]any, 0, len(entries))
	}
	for _, entry := range entries {
		name := path.Base(entry.Path)
		if name == "." || name == ".." || entry.Path != name {
			return nil, fmt.Errorf("tenki: invalid artifact entry name: %s", entry.Path)
		}
		remotePath := path.Join(currentDir, name)
		relativePath := path.Join(relativeDir, name)
		// List does not populate IsSymlink in this SDK; Mode uses POSIX bits.
		if entry.IsSymlink || entry.Mode&0o170000 == 0o120000 {
			return nil, fmt.Errorf("artifact symlinks are not allowed: %s", relativePath)
		}
		info, statErr := sess.Stat(ctx, remotePath)
		if statErr != nil {
			return nil, statErr
		}
		if info.IsSymlink || info.Mode&0o170000 == 0o120000 {
			return nil, fmt.Errorf("artifact symlinks are not allowed: %s", relativePath)
		}
		if entry.IsDir {
			nested, nestedErr := p.collectArtifactsRecursive(ctx, sess, remotePath, relativePath, depth+1, artifacts)
			if nestedErr != nil {
				return nil, nestedErr
			}
			artifacts = nested
			continue
		}
		if len(artifacts) >= p.maxArtifacts {
			return nil, fmt.Errorf("tenki: execution produced more than %d artifacts", p.maxArtifacts)
		}
		if entry.Size > int64(p.maxArtifactBytes) {
			return nil, fmt.Errorf("tenki: artifact exceeds %d bytes: %s", p.maxArtifactBytes, relativePath)
		}
		ext := strings.ToLower(path.Ext(name))
		if _, ok := allowedArtifactExts[ext]; !ok {
			return nil, fmt.Errorf("tenki: unsupported artifact type: %s", relativePath)
		}
		content, readErr := sess.ReadFile(ctx, remotePath)
		if readErr != nil {
			return nil, readErr
		}
		if len(content) > p.maxArtifactBytes {
			return nil, fmt.Errorf("tenki: artifact exceeds %d bytes: %s", p.maxArtifactBytes, relativePath)
		}
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		artifacts = append(artifacts, map[string]any{
			"name":        relativePath,
			"content_b64": base64.StdEncoding.EncodeToString(content),
			"mime_type":   mimeType,
			"size":        int64(len(content)),
		})
	}
	return artifacts, nil
}
