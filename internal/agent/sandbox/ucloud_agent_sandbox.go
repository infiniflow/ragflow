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
	"errors"
	"fmt"
	"mime"
	"path"
	"ragflow/internal/common"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	ucloudsdk "github.com/ucloud/ucloud-sandbox-sdk-go"
)

const (
	ucloudAgentSandboxDefaultRegion          = "cn-wlcb"
	ucloudAgentSandboxDomainSuffix           = "sandbox.ucloudai.com"
	ucloudAgentSandboxDefaultTemplate        = "base"
	ucloudAgentSandboxHome                   = "/home/user"
	ucloudAgentSandboxMaxArtifactDepth       = 16
	ucloudAgentSandboxDefaultTimeout         = 30
	ucloudAgentSandboxDefaultLifetime        = 300
	ucloudAgentSandboxDefaultMaxOutputBytes  = 1024 * 1024
	ucloudAgentSandboxDefaultMaxArtifacts    = 20
	ucloudAgentSandboxDefaultMaxArtifactSize = 10 * 1024 * 1024
)

type ucloudAgentSandboxInstance struct {
	sandbox       *ucloudsdk.Sandbox
	remoteWorkDir string
}

// UCloudAgentSandboxProvider executes code in disposable UCloud Agent Sandboxes.
type UCloudAgentSandboxProvider struct {
	client              *ucloudsdk.Client
	apiKey              string
	region              string
	domain              string
	apiURL              string
	template            string
	allowInternetAccess bool
	insecureHTTP        bool
	timeoutSec          int
	sandboxTimeoutSec   int
	maxOutputBytes      int
	maxArtifacts        int
	maxArtifactBytes    int

	mu          sync.RWMutex
	initialized bool
	instances   map[string]*ucloudAgentSandboxInstance
}

func newUCloudAgentSandboxProviderFromEnv() *UCloudAgentSandboxProvider {
	return newUCloudAgentSandboxProviderFromConfig(ucloudAgentSandboxConfigFromEnv())
}

func ucloudAgentSandboxConfigFromEnv() map[string]any {
	return map[string]any{
		"api_key":               common.GetEnv(common.EnvUCloudSandboxAPIKey),
		"region":                common.GetEnv(common.EnvUCloudSandboxRegion),
		"domain":                common.GetEnv(common.EnvUCloudSandboxDomain),
		"api_url":               common.GetEnv(common.EnvUCloudSandboxAPIURL),
		"template":              common.GetEnv(common.EnvUCloudSandboxTemplate),
		"allow_internet_access": common.GetEnv(common.EnvUCloudSandboxAllowInternetAccess),
		"insecure_http":         common.GetEnv(common.EnvUCloudSandboxInsecureHTTP),
		"timeout":               common.GetEnv(common.EnvUCloudSandboxExecutionTimeout),
		"sandbox_timeout":       common.GetEnv(common.EnvUCloudSandboxTimeout),
		"max_output_bytes":      common.GetEnv(common.EnvUCloudSandboxMaxOutputBytes),
		"max_artifacts":         common.GetEnv(common.EnvUCloudSandboxMaxArtifacts),
		"max_artifact_bytes":    common.GetEnv(common.EnvUCloudSandboxMaxArtifactBytes),
	}
}

func newUCloudAgentSandboxProviderFromConfig(cfg map[string]any) *UCloudAgentSandboxProvider {
	region := strings.TrimSpace(configString(cfg, "region"))
	if region == "" {
		region = ucloudAgentSandboxDefaultRegion
	}
	template := strings.TrimSpace(configString(cfg, "template"))
	if template == "" {
		template = ucloudAgentSandboxDefaultTemplate
	}
	return &UCloudAgentSandboxProvider{
		apiKey:              strings.TrimSpace(configString(cfg, "api_key")),
		region:              region,
		domain:              strings.TrimSpace(configString(cfg, "domain")),
		apiURL:              strings.TrimSpace(configString(cfg, "api_url")),
		template:            template,
		allowInternetAccess: strings.EqualFold(configString(cfg, "allow_internet_access"), "true"),
		insecureHTTP:        strings.EqualFold(configString(cfg, "insecure_http"), "true"),
		timeoutSec:          configInt(cfg, "timeout", ucloudAgentSandboxDefaultTimeout),
		sandboxTimeoutSec:   configInt(cfg, "sandbox_timeout", ucloudAgentSandboxDefaultLifetime),
		maxOutputBytes:      configInt(cfg, "max_output_bytes", ucloudAgentSandboxDefaultMaxOutputBytes),
		maxArtifacts:        configInt(cfg, "max_artifacts", ucloudAgentSandboxDefaultMaxArtifacts),
		maxArtifactBytes:    configInt(cfg, "max_artifact_bytes", ucloudAgentSandboxDefaultMaxArtifactSize),
		instances:           make(map[string]*ucloudAgentSandboxInstance),
	}
}

// ProviderType returns ProviderUCloudAgentSandbox.
func (p *UCloudAgentSandboxProvider) ProviderType() ProviderType {
	return ProviderUCloudAgentSandbox
}

// Initialize creates the UCloud SDK client. Connectivity is verified by the
// admin connection test, which creates and executes in a temporary sandbox.
func (p *UCloudAgentSandboxProvider) Initialize(context.Context) error {
	if p.apiKey == "" {
		return errors.New("ucloud agent sandbox: API key is required")
	}
	if p.timeoutSec <= 0 || p.sandboxTimeoutSec <= 0 || p.maxOutputBytes <= 0 || p.maxArtifactBytes <= 0 || p.maxArtifacts < 0 {
		return errors.New("ucloud agent sandbox: timeout and size limits must be positive, and max_artifacts must be non-negative")
	}
	domain := p.domain
	if domain == "" {
		domain = p.region + "." + ucloudAgentSandboxDomainSuffix
	}
	options := []ucloudsdk.ClientOption{
		ucloudsdk.WithRequestTimeout(time.Duration(p.timeoutSec) * time.Second),
		ucloudsdk.WithInsecureHTTP(p.insecureHTTP),
		// The SDK currently defaults to skipping TLS verification. RAGFlow
		// always opts into certificate verification.
		ucloudsdk.WithInsecureSkipTLS(false),
	}
	if p.apiURL != "" {
		options = append(options, ucloudsdk.WithAPIURL(p.apiURL))
	}
	p.client = ucloudsdk.NewClient(domain, p.apiKey, options...)
	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()
	return nil
}

// SupportedLanguages returns the runtimes included in UCloud's base template.
func (p *UCloudAgentSandboxProvider) SupportedLanguages() []string {
	return []string{"python", "javascript"}
}

// CreateInstance provisions a fresh UCloud Agent Sandbox.
func (p *UCloudAgentSandboxProvider) CreateInstance(ctx context.Context, template string) (*SandboxInstance, error) {
	if !p.isInitialized() {
		return nil, errors.New("ucloud agent sandbox: provider not initialized")
	}
	language := normalizeLanguage(template)
	if language == "" {
		return nil, fmt.Errorf("ucloud agent sandbox: unsupported language %q", template)
	}
	sbx, err := p.client.CreateSandbox(ctx,
		ucloudsdk.WithTemplate(p.template),
		ucloudsdk.WithTimeout(p.sandboxTimeoutSec),
		ucloudsdk.WithMetadata(map[string]string{"source": "ragflow"}),
		ucloudsdk.WithManageBy("ragflow"),
		ucloudsdk.WithSecure(true),
		ucloudsdk.WithAllowInternetAccess(p.allowInternetAccess),
	)
	if err != nil {
		return nil, fmt.Errorf("ucloud agent sandbox: create sandbox: %w", err)
	}
	remoteWorkDir := path.Join(ucloudAgentSandboxHome, "ragflow-codeexec-"+uuid.NewString())
	if _, err := sbx.Commands.Run(ctx, "mkdir -p "+shq(path.Join(remoteWorkDir, "artifacts")), ucloudsdk.WithCommandTimeout(minTimeout(p.timeoutSec, 10))); err != nil {
		_, _ = sbx.Kill(ctx)
		return nil, fmt.Errorf("ucloud agent sandbox: create workspace: %w", err)
	}
	p.mu.Lock()
	p.instances[sbx.ID] = &ucloudAgentSandboxInstance{sandbox: sbx, remoteWorkDir: remoteWorkDir}
	p.mu.Unlock()
	return &SandboxInstance{
		InstanceID: sbx.ID,
		Provider:   ProviderUCloudAgentSandbox,
		Status:     "running",
		Metadata: map[string]any{
			"language":        language,
			"remote_work_dir": remoteWorkDir,
			"sandbox_id":      sbx.ID,
			"template":        p.template,
		},
	}, nil
}

// ExecuteCode writes the shared RAGFlow wrapper into the sandbox and runs it.
func (p *UCloudAgentSandboxProvider) ExecuteCode(
	ctx context.Context,
	inst *SandboxInstance,
	code, language string,
	timeoutSec int,
	args map[string]any,
) (*ExecutionResult, error) {
	if !p.isInitialized() {
		return nil, errors.New("ucloud agent sandbox: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return nil, errors.New("ucloud agent sandbox: instance id required")
	}
	p.mu.RLock()
	instance := p.instances[inst.InstanceID]
	p.mu.RUnlock()
	if instance == nil {
		return nil, fmt.Errorf("ucloud agent sandbox: unknown instance %q", inst.InstanceID)
	}
	normalizedLanguage := normalizeLanguage(language)
	if normalizedLanguage == "" {
		return nil, fmt.Errorf("ucloud agent sandbox: unsupported language %q", language)
	}
	executionTimeout, err := validateTimeout(timeoutSec)
	if err != nil {
		return nil, err
	}
	if executionTimeout == 0 || executionTimeout > p.timeoutSec {
		executionTimeout = p.timeoutSec
	}
	argsJSON, err := argsToJSON(args)
	if err != nil {
		return nil, err
	}

	var scriptName, scriptContent, executable string
	if normalizedLanguage == "python" {
		scriptName = "main.py"
		scriptContent = BuildPythonWrapper(code, argsJSON)
		executable = "python3"
	} else {
		scriptName = "main.js"
		scriptContent = BuildJavaScriptWrapper(code, argsJSON)
		executable = "node"
	}
	scriptPath := path.Join(instance.remoteWorkDir, scriptName)
	if _, err := instance.sandbox.Files.Write(ctx, scriptPath, scriptContent); err != nil {
		return nil, fmt.Errorf("ucloud agent sandbox: write script: %w", err)
	}
	if err := instance.sandbox.SetTimeout(ctx, max(p.sandboxTimeoutSec, executionTimeout+30)); err != nil {
		return nil, fmt.Errorf("ucloud agent sandbox: extend sandbox timeout: %w", err)
	}

	started := time.Now()
	commandResult, runErr := instance.sandbox.Commands.Run(
		ctx,
		executable+" "+shq(scriptPath),
		ucloudsdk.WithCwd(instance.remoteWorkDir),
		ucloudsdk.WithCommandTimeout(executionTimeout),
	)
	executionTime := time.Since(started).Seconds()
	if runErr != nil {
		var exitErr *ucloudsdk.CommandExitError
		if errors.As(runErr, &exitErr) {
			commandResult = &ucloudsdk.CommandResult{Stdout: exitErr.Stdout, Stderr: exitErr.Stderr, ExitCode: exitErr.ExitCode, Error: exitErr.Message}
		} else {
			var timeoutErr *ucloudsdk.TimeoutError
			if errors.As(runErr, &timeoutErr) || errors.Is(runErr, context.DeadlineExceeded) {
				return nil, fmt.Errorf("ucloud agent sandbox: execution timed out after %d seconds: %w", executionTimeout, runErr)
			}
			return nil, fmt.Errorf("ucloud agent sandbox: execute code: %w", runErr)
		}
	}
	if commandResult == nil {
		return nil, errors.New("ucloud agent sandbox: command returned no result")
	}
	if len(commandResult.Stdout)+len(commandResult.Stderr) > p.maxOutputBytes {
		return nil, fmt.Errorf("ucloud agent sandbox: execution output exceeded %d bytes", p.maxOutputBytes)
	}
	stdout, structured := ExtractStructuredResult(commandResult.Stdout)
	artifacts := make([]map[string]any, 0)
	if err := p.collectArtifacts(ctx, instance.sandbox, path.Join(instance.remoteWorkDir, "artifacts"), "", &artifacts, 0); err != nil {
		return nil, err
	}
	return &ExecutionResult{
		Stdout:        stdout,
		Stderr:        commandResult.Stderr,
		ExitCode:      commandResult.ExitCode,
		ExecutionTime: executionTime,
		Metadata: map[string]any{
			"instance_id":     inst.InstanceID,
			"sandbox_id":      instance.sandbox.ID,
			"language":        normalizedLanguage,
			"script_path":     scriptPath,
			"remote_work_dir": instance.remoteWorkDir,
			"status":          map[bool]string{true: "ok", false: "error"}[commandResult.ExitCode == 0],
			"timeout":         executionTimeout,
			"artifacts":       artifacts,
			"result_present":  structured["present"],
			"result_value":    structured["value"],
			"result_type":     structured["type"],
		},
	}, nil
}

// DestroyInstance terminates the sandbox. A missing instance is already clean.
func (p *UCloudAgentSandboxProvider) DestroyInstance(ctx context.Context, inst *SandboxInstance) error {
	if !p.isInitialized() {
		return errors.New("ucloud agent sandbox: provider not initialized")
	}
	if inst == nil || inst.InstanceID == "" {
		return errors.New("ucloud agent sandbox: instance id required")
	}
	p.mu.Lock()
	instance := p.instances[inst.InstanceID]
	delete(p.instances, inst.InstanceID)
	p.mu.Unlock()
	if instance == nil {
		return nil
	}
	if _, err := instance.sandbox.Kill(ctx); err != nil {
		return fmt.Errorf("ucloud agent sandbox: kill sandbox: %w", err)
	}
	return nil
}

// HealthCheck reports whether the provider has a configured client.
func (p *UCloudAgentSandboxProvider) HealthCheck(context.Context) error {
	if !p.isInitialized() || p.client == nil {
		return errors.New("ucloud agent sandbox: provider not initialized")
	}
	return nil
}

func (p *UCloudAgentSandboxProvider) collectArtifacts(
	ctx context.Context,
	sbx *ucloudsdk.Sandbox,
	currentDir, relativeDir string,
	artifacts *[]map[string]any,
	depth int,
) error {
	if depth > ucloudAgentSandboxMaxArtifactDepth {
		return fmt.Errorf("ucloud agent sandbox: artifact directory nesting exceeds %d levels: %s", ucloudAgentSandboxMaxArtifactDepth, relativeDir)
	}
	entries, err := sbx.Files.List(ctx, currentDir, ucloudsdk.WithDepth(1))
	if err != nil {
		if errors.Is(err, ucloudsdk.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("ucloud agent sandbox: list artifacts: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	for _, entry := range entries {
		name := path.Base(entry.Path)
		relativePath := path.Join(relativeDir, name)
		if entry.SymlinkTarget != nil {
			return fmt.Errorf("ucloud agent sandbox: artifact symlinks are not allowed: %s", relativePath)
		}
		if entry.Type == ucloudsdk.EntryTypeDir {
			if err := p.collectArtifacts(ctx, sbx, entry.Path, relativePath, artifacts, depth+1); err != nil {
				return err
			}
			continue
		}
		if len(*artifacts) >= p.maxArtifacts {
			return fmt.Errorf("ucloud agent sandbox: execution produced more than %d artifacts", p.maxArtifacts)
		}
		if entry.Size > int64(p.maxArtifactBytes) {
			return fmt.Errorf("ucloud agent sandbox: artifact exceeds %d bytes: %s", p.maxArtifactBytes, relativePath)
		}
		extension := strings.ToLower(path.Ext(name))
		if _, allowed := allowedArtifactExts[extension]; !allowed {
			return fmt.Errorf("ucloud agent sandbox: unsupported artifact type: %s", relativePath)
		}
		content, err := sbx.Files.ReadBytes(ctx, entry.Path)
		if err != nil {
			return fmt.Errorf("ucloud agent sandbox: read artifact %s: %w", relativePath, err)
		}
		mimeType := mime.TypeByExtension(extension)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		*artifacts = append(*artifacts, map[string]any{
			"name":        relativePath,
			"content_b64": base64.StdEncoding.EncodeToString(content),
			"mime_type":   mimeType,
			"size":        entry.Size,
		})
	}
	return nil
}

func (p *UCloudAgentSandboxProvider) isInitialized() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.initialized
}
