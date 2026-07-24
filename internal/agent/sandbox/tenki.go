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
// Like the Go e2b provider, this port does not collect run artifacts
// (artifact collection lives only in the local/ssh providers).
//
// SDK: github.com/TenkiCloud/tenki-sdk-go/sandbox (MIT). The auth
// token and API URL are read from env by Initialize; project id,
// image and tunables come from the admin-panel config map.

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"sync"
	"time"

	tenkisdk "github.com/TenkiCloud/tenki-sdk-go/sandbox"
)

// tenkiDefaultSandboxTimeout bounds a single CodeExec call. A fresh
// sandbox is provisioned per CreateInstance and destroyed on
// DestroyInstance, so the lifetime need only outlast one execution.
const tenkiDefaultSandboxTimeout = 5 * time.Minute

// TenkiProvider is the Go port of the Python TenkiProvider.
type TenkiProvider struct {
	client         *tenkisdk.Client
	apiKey         string
	apiURL         string
	projectID      string
	image          string
	allowOutbound  bool
	sandboxTimeout time.Duration

	mu          sync.Mutex
	initialized bool
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
		"API_KEY":        common.GetEnv(common.EnvTenkiApiKey),
		"API_URL":        common.GetEnv(common.EnvTenkiAPIURL),
		"PROJECT_ID":     common.GetEnv(common.EnvTenkiProjectID),
		"IMAGE":          common.GetEnv(common.EnvTenkiImage),
		"TIMEOUT":        common.GetEnv(common.EnvTenkiTimeout),
		"ALLOW_OUTBOUND": common.GetEnv(common.EnvTenkiAllowOutbound),
	}
}

// newTenkiProviderFromConfig builds the provider from a JSON config
// map (admin-panel settings or the env-backed map above).
func newTenkiProviderFromConfig(cfg map[string]any) *TenkiProvider {
	p := &TenkiProvider{
		apiKey:    configString(cfg, "API_KEY"),
		apiURL:    configString(cfg, "API_URL"),
		projectID: configString(cfg, "PROJECT_ID"),
		image:     configString(cfg, "IMAGE"),
		// Outbound network is opt-in: sandboxed code has no egress
		// unless ALLOW_OUTBOUND is explicitly "true". This matches
		// the self_managed sandbox, which treats network access as an
		// unauthorized-access event by default.
		allowOutbound: configString(cfg, "ALLOW_OUTBOUND") == "true",
	}
	timeoutSec := configInt(cfg, "TIMEOUT", int(tenkiDefaultSandboxTimeout.Seconds()))
	if timeoutSec > 0 {
		p.sandboxTimeout = time.Duration(timeoutSec) * time.Second
	} else {
		p.sandboxTimeout = tenkiDefaultSandboxTimeout
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
		apiKey = common.GetEnv(common.EnvTenkiApiKey)
	}
	if apiKey == "" {
		return errors.New("tenki: API key is required (set it in Admin > Sandbox Settings or TENKI_API_KEY)")
	}
	apiURL := p.apiURL
	if apiURL == "" {
		apiURL = common.GetEnv(common.EnvTenkiAPIURL)
	}
	opts := []tenkisdk.Option{tenkisdk.WithAuthToken(apiKey)}
	if apiURL != "" {
		opts = append(opts, tenkisdk.WithBaseURL(apiURL))
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
	return []string{"python", "nodejs", "javascript"}
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
		tenkisdk.WithMaxDuration(p.sandboxTimeout),
		tenkisdk.WithWaitTimeout(p.sandboxTimeout),
	}
	if p.projectID != "" {
		opts = append(opts, tenkisdk.WithProjectID(p.projectID))
	}
	if p.image != "" {
		opts = append(opts, tenkisdk.WithImage(p.image))
	}
	sess, err := p.client.Create(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("tenki: Create: %w", err)
	}
	return &SandboxInstance{
		InstanceID: sess.ID,
		Provider:   ProviderTenki,
		Status:     "running",
		Metadata: map[string]any{
			"language":        lang,
			"image":           p.image,
			"sandbox_timeout": p.sandboxTimeout.String(),
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
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = int(p.sandboxTimeout.Seconds())
	}

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

	// Re-obtain a handle to the sandbox created in CreateInstance and
	// wait for its data plane to be ready before exec.
	sess, err := p.client.Session(inst.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("tenki: Session(%s): %w", inst.InstanceID, err)
	}
	if err := sess.WaitReady(ctx, p.sandboxTimeout); err != nil {
		return nil, fmt.Errorf("tenki: WaitReady(%s): %w", inst.InstanceID, err)
	}

	start := time.Now()
	res, err := sess.Exec(ctx, cmd,
		tenkisdk.WithArgs(runArgs...),
		tenkisdk.WithTimeout(time.Duration(timeout)*time.Second),
	)
	if err != nil {
		// A non-zero exit is reported as a *Result, not an error;
		// a nil result here means a transport, timeout or session
		// error that we cannot map to stdout/stderr. Do not include
		// runArgs — they embed the user's code and arguments.
		return nil, fmt.Errorf("tenki: exec %s: %w", cmd, err)
	}
	return buildTenkiExecutionResult(res, lang, start), nil
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
	sess, err := p.client.Session(inst.InstanceID)
	if err != nil {
		return fmt.Errorf("tenki: Session(%s): %w", inst.InstanceID, err)
	}
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
