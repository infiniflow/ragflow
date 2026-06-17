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

// e2b.go is the Go port of
// `agent/sandbox/providers/e2b.py` (which is itself a stub on the
// Python side — see Python E2BProvider.execute_code raising
// "E2B provider is not yet fully implemented"). On the Go side we
// go further: we ship a real implementation that talks to e2b
// cloud sandboxes via the community Go SDK.
//
// The community SDK is at github.com/eric642/e2b-go-sdk
// (Apache-2.0; community-maintained port of the official
// e2b-code-interpreter Python SDK). It exposes the full sandbox
// lifecycle (Create / Kill / IsRunning / GetInfo) and a
// Commands.Run API that runs a shell command inside the sandbox.
// We use Commands.Run to invoke `python3 -c <wrapped>` and
// `node -e <wrapped>` — the same code-wrapping protocol as
// SelfManaged and Aliyun, so the `__RAGFLOW_RESULT__:` marker
// extraction works uniformly across all three providers.
//
// See §17.6.2 of docs/develop/agent-go-port-design.md for the SDK gap note and the
// risk register. The community SDK is acceptable for our use
// because it (a) mirrors the official Python SDK behavior,
// (b) is Apache-2.0, and (c) tracks upstream spec changes via
// buf-generated stubs. Re-evaluate the risk register quarterly.

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	e2bsdk "github.com/eric642/e2b-go-sdk"
)

// e2bDefaultTemplate is the e2b sandbox template the operator
// expects to be on PATH. The e2b "base" template ships with
// python3 and node pre-installed; the provider does not need a
// custom template for the common case. Operators can override
// with E2B_TEMPLATE.
const e2bDefaultTemplate = "base"

// e2bDefaultSandboxTimeout is the sandbox lifetime for a single
// CodeExec call. e2b bills by the second; we provision a fresh
// sandbox per CreateInstance call and destroy it on
// DestroyInstance, so the lifetime should be just slightly longer
// than the per-execution timeout. The 5-minute default matches
// the Python e2b-code-interpreter SDK's default.
const e2bDefaultSandboxTimeout = 5 * time.Minute

// E2BProvider is the Go port of
// `agent/sandbox/providers/e2b.py::E2BProvider`. On the Python
// side this class is a stub; on the Go side it is a real
// implementation that talks to e2b cloud sandboxes.
type E2BProvider struct {
	client         *e2bsdk.Client
	template       string
	sandboxTimeout time.Duration

	mu          sync.Mutex
	initialized bool
}

// newE2BProviderFromEnv reads E2B_* env vars and returns a
// provider ready for Initialize. The provider requires either
// E2B_API_KEY or E2B_ACCESS_TOKEN; Initialize surfaces the error
// if neither is set.
func newE2BProviderFromEnv() *E2BProvider {
	return newE2BProviderFromConfig(e2bConfigFromEnv())
}

// e2bConfigFromEnv builds a config map from the E2B_* env vars,
// mirroring the admin-panel settings JSON shape. Note: E2B_API_KEY
// and E2B_ACCESS_TOKEN are intentionally read directly by
// Initialize (the SDK requires env or Config{}), so they are NOT
// part of the JSON config map.
func e2bConfigFromEnv() map[string]any {
	return map[string]any{
		"TEMPLATE": os.Getenv("E2B_TEMPLATE"),
		"TIMEOUT":  os.Getenv("E2B_TIMEOUT"),
	}
}

// newE2BProviderFromConfig builds the provider from a JSON config
// map. API key / access token are read by Initialize directly
// from env (the e2b SDK requires it).
func newE2BProviderFromConfig(cfg map[string]any) *E2BProvider {
	p := &E2BProvider{
		template: configString(cfg, "TEMPLATE"),
	}
	if p.template == "" {
		p.template = e2bDefaultTemplate
	}
	timeoutSec := configInt(cfg, "TIMEOUT", int(e2bDefaultSandboxTimeout.Seconds()))
	if timeoutSec > 0 {
		p.sandboxTimeout = time.Duration(timeoutSec) * time.Second
	} else {
		p.sandboxTimeout = e2bDefaultSandboxTimeout
	}
	return p
}

// ProviderType returns ProviderE2B.
func (p *E2BProvider) ProviderType() ProviderType { return ProviderE2B }

// Initialize builds the e2b SDK client. The SDK reads E2B_API_KEY
// or E2B_ACCESS_TOKEN from the Config struct, which it also
// resolves from the same env vars. We return an error if neither
// is set so the manager does not register a broken provider.
func (p *E2BProvider) Initialize(ctx context.Context) error {
	apiKey := os.Getenv("E2B_API_KEY")
	accessToken := os.Getenv("E2B_ACCESS_TOKEN")
	if apiKey == "" && accessToken == "" {
		return errors.New(
			"e2b: E2B_API_KEY or E2B_ACCESS_TOKEN env var is required " +
				"(see §17.4 of docs/develop/agent-go-port-design.md for the risk register entry on the community e2b SDK)",
		)
	}
	cfg := e2bsdk.Config{
		APIKey:      apiKey,
		AccessToken: accessToken,
		Domain:      os.Getenv("E2B_DOMAIN"),
		APIURL:      os.Getenv("E2B_API_URL"),
	}
	c, err := e2bsdk.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("e2b: build client: %w", err)
	}
	p.client = c
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the languages the e2b "base" template
// can run. The base template ships with python3 and node.
func (p *E2BProvider) SupportedLanguages() []string {
	return []string{"python", "nodejs", "javascript"}
}

// CreateInstance provisions a fresh e2b sandbox. The sandbox's
// InstanceID is the e2b sandbox id (a UUID-shaped string).
func (p *E2BProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("e2b: provider not initialized")
	}
	// Use the per-call template if the caller passed one; otherwise
	// fall back to the configured default.
	tpl := template
	if tpl == "" {
		tpl = p.template
	}
	// Validate the language to fail fast on unsupported calls.
	lang := normalizeLanguage(template)
	if lang == "" {
		return nil, fmt.Errorf("e2b: unsupported language %q", template)
	}
	// CreateOptions.Template is the template id; e2b ignores the
	// language hint at create time and dispatches to the right
	// runtime based on the command at execute time.
	opts := e2bsdk.CreateOptions{
		Template: tpl,
		Timeout:  p.sandboxTimeout,
	}
	sb, err := p.client.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("e2b: Create: %w", err)
	}
	info, _ := sb.GetInfo(ctx)
	return &SandboxInstance{
		InstanceID: sb.ID,
		Provider:   ProviderE2B,
		Status:     "running",
		Metadata: map[string]any{
			"language":        lang,
			"template":        tpl,
			"sandbox_timeout": p.sandboxTimeout.String(),
			"e2b_alias":       infoAlias(info),
		},
	}, nil
}

// ExecuteCode runs the user's code inside the sandbox via
// `python3 -c <wrapped>` (Python) or `node -e <wrapped>` (JS).
// The wrapped code carries the `__RAGFLOW_RESULT__:` marker so
// the structured main() return value comes back as before.
func (p *E2BProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, fmt.Errorf("e2b: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, fmt.Errorf("e2b: instance id required")
	}
	lang := normalizeLanguage(language)
	if lang == "" {
		return nil, fmt.Errorf("e2b: unsupported language %q", language)
	}
	timeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if timeout == 0 {
		timeout = int(p.sandboxTimeout.Seconds())
	}

	// Wrap the code in the result-protocol driver.
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}
	var wrapped string
	var cmd string
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

	// Connect to the existing sandbox (created in CreateInstance).
	sb, err := p.client.Connect(ctx, inst.InstanceID, e2bsdk.ConnectOptions{})
	if err != nil {
		return nil, fmt.Errorf("e2b: Connect(%s): %w", inst.InstanceID, err)
	}

	start := time.Now()
	handle, err := sb.Commands.Run(ctx, cmd, e2bsdk.RunOptions{
		Args:      runArgs,
		TimeoutMs: timeout * 1000,
	})
	if err != nil {
		return nil, fmt.Errorf("e2b: %s %v: %w", cmd, runArgs, err)
	}
	result, err := handle.Wait(ctx)
	if err != nil {
		// CommandExitError is a typed error carrying the partial
		// result; we still want to surface stdout/stderr even on
		// non-zero exit. Result is a value field (not a pointer),
		// so we pass its address to keep the helper signature
		// consistent with the success path.
		var exitErr *e2bsdk.CommandExitError
		if errors.As(err, &exitErr) {
			return buildE2BExecutionResult(&exitErr.Result, lang, start), nil
		}
		return nil, fmt.Errorf("e2b: %s wait: %w", cmd, err)
	}
	return buildE2BExecutionResult(result, lang, start), nil
}

// buildE2BExecutionResult maps the e2b CommandResult to our
// sandbox.ExecutionResult. The exit code maps 1:1; stdout is
// scanned for the `__RAGFLOW_RESULT__:` marker so the model gets
// the structured main() return value (matching SelfManaged and
// Aliyun).
func buildE2BExecutionResult(r *e2bsdk.CommandResult, lang string, start time.Time) *ExecutionResult {
	stdout, structured := ExtractStructuredResult(r.Stdout)
	exitCode := int(r.ExitCode)
	return &ExecutionResult{
		Stdout:        stdout,
		Stderr:        r.Stderr,
		ExitCode:      exitCode,
		ExecutionTime: time.Since(start).Seconds(),
		Metadata: map[string]any{
			"language":          lang,
			"structured_result": structured,
			"e2b_error":         r.Error,
		},
	}
}

// DestroyInstance kills the sandbox. e2b bills by the second; the
// caller MUST call DestroyInstance after each CodeExec to release
// the sandbox. The e2b SDK's Kill is idempotent (returns false if
// the sandbox is already gone), so we don't error on "already
// gone" conditions.
func (p *E2BProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return fmt.Errorf("e2b: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return fmt.Errorf("e2b: instance id required")
	}
	// Use the Client-level Kill to avoid needing a fresh Connect.
	// If the sandbox is already gone, Kill returns (false, nil).
	_, err := p.client.Kill(ctx, inst.InstanceID)
	if err != nil {
		return fmt.Errorf("e2b: Kill(%s): %w", inst.InstanceID, err)
	}
	return nil
}

// HealthCheck lists running sandboxes for the account. The e2b
// SDK does not expose a dedicated "is the control plane up?"
// endpoint, so a successful list is our probe. The call is cheap
// when the account has zero sandboxes.
func (p *E2BProvider) HealthCheck(ctx context.Context) error {
	if !p.isInitialized() {
		return errors.New("e2b: provider not initialized")
	}
	_, err := p.client.ListAll(ctx, e2bsdk.SandboxListOptions{})
	if err != nil {
		return fmt.Errorf("e2b: ListAll: %w", err)
	}
	return nil
}

func (p *E2BProvider) isInitialized() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized
}

// infoAlias extracts the "alias" field from e2b's SandboxInfo if
// present, returning "" otherwise. Used only for the Metadata
// surfacing — does not affect dispatch.
func infoAlias(info *e2bsdk.SandboxInfo) string {
	if info == nil {
		return ""
	}
	return info.Alias
}
