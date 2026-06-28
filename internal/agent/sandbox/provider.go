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

// Package sandbox implements RAGFlow's Go-side CodeExec sandbox subsystem.
//
// The package is the Go port of `agent/sandbox/` in the Python repo. It
// exposes a single active provider (self_managed or aliyun_codeinterpreter
// today; e2b is a loud stub) through a ProviderManager. The
// ManagerClient returned by NewManagerClient implements the
// tool.SandboxClient interface in the parent package, so the CodeExec
// tool can dispatch code execution through this manager without
// knowing which provider is active.
//
// See §17 of docs/develop/agent-go-port-design.md for the decision
// record and the SDK gap that forced an HTTP fallback for the
// aliyun execute path.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ProviderType is the canonical identifier for a sandbox backend. The
// values match the Python-side `sandbox.provider_type` system setting
// and the keys in `agent/sandbox/providers/__init__.py`.
type ProviderType string

const (
	// ProviderSelfManaged uses the executor_manager HTTP API (port 9385)
	// and is the default. Matches Python's SelfManagedProvider.
	ProviderSelfManaged ProviderType = "self_managed"

	// ProviderAliyun uses Alibaba Cloud's Code Interpreter service
	// (agentrun SDK). Matches Python's AliyunCodeInterpreterProvider.
	ProviderAliyun ProviderType = "aliyun_codeinterpreter"

	// ProviderE2B uses e2b cloud sandboxes (community Go SDK).
	// Matches Python's E2BProvider (which is a stub on the
	// Python side; the Go side goes further with a real
	// implementation).
	ProviderE2B ProviderType = "e2b"

	// ProviderLocal runs the user's code on the Go host itself
	// via os/exec. Matches Python's LocalProvider. There is no
	// sandboxing — operators that need isolation should
	// configure SelfManaged or Aliyun / e2b.
	ProviderLocal ProviderType = "local"

	// ProviderSSH runs the user's code on a remote host via
	// SSH. Matches Python's SSHProvider.
	ProviderSSH ProviderType = "ssh"
)

// ErrE2BProviderNotImplemented is returned when an operator configures
// SANDBOX_PROVIDER_TYPE=e2b. The Go port does not yet ship an e2b
// provider; route such workflows to the Python canvas until a Go SDK
// or wire protocol is established. This is a loud-fail sentinel in the
// same family as ErrRetrievalServiceMissing and
// ErrTTSEngineNotConfigured.
var ErrE2BProviderNotImplemented = errors.New(
	"sandbox: e2b provider is not implemented in the Go port — " +
		"route CodeExec nodes that need e2b to the Python canvas " +
		"(see §17.4 of docs/develop/agent-go-port-design.md for the deferral rationale — " +
		"the e2b provider IS implemented in the Go port via the community SDK, " +
		"so this sentinel is no longer surfaced; kept for forward-compat with the " +
		"ErrCodeExecSandboxMissing pattern in tool.SandboxClient)",
)

// ExecutionResult is the wire shape returned by every provider. The
// fields mirror the Python `agent/sandbox/providers/base.py`
// ExecutionResult dataclass so downstream consumers (the CodeExec
// tool, the Message component, rich-content rendering) can pattern
// match unchanged across the two ports.
type ExecutionResult struct {
	// Stdout is the captured standard output from the sandbox.
	Stdout string
	// Stderr is the captured standard error.
	Stderr string
	// ExitCode is the process exit code. 0 = success, non-zero = error.
	ExitCode int
	// ExecutionTime is wall-clock duration in seconds.
	ExecutionTime float64
	// Metadata carries provider-specific extras: structured result
	// value, artifact list (self_managed), context id (aliyun), etc.
	Metadata map[string]any
}

// SandboxInstance is the logical handle returned by CreateInstance.
// Self-managed treats this as a UUID tracking label (executor_manager
// pools internally); aliyun returns the agentrun CodeInterpreterId.
type SandboxInstance struct {
	// InstanceID is the opaque provider-specific handle. Pass it back
	// to ExecuteCode / DestroyInstance.
	InstanceID string
	// Provider is the ProviderType that produced this instance. Set
	// for telemetry and log scoping.
	Provider ProviderType
	// Status is the provider's last known lifecycle state — running,
	// READY, error, etc. Self-managed always reports "running" because
	// the container is owned by the pool, not by this instance.
	Status string
	// Metadata is provider-specific.
	Metadata map[string]any
}

// SandboxProvider is the contract every backend must satisfy. The
// shape mirrors `agent/sandbox/providers/base.py` SandboxProvider ABC
// so the Go port's providers are semantically interchangeable with
// the Python ones — same lifecycle, same argument shape, same result
// shape.
type SandboxProvider interface {
	// Initialize configures the provider. Returns an error if the
	// provider is misconfigured or the upstream is unreachable. The
	// manager will not register a provider whose Initialize fails.
	Initialize(ctx context.Context) error

	// ProviderType returns the canonical ProviderType identifier.
	ProviderType() ProviderType

	// CreateInstance provisions a logical instance handle. For
	// self-managed this is a UUID label; for aliyun this triggers a
	// CreateCodeInterpreter API call.
	CreateInstance(ctx context.Context, template string) (*SandboxInstance, error)

	// ExecuteCode runs the user's code in the instance. Implementations
	// wrap the code in build_python_wrapper / build_javascript_wrapper
	// (see result_protocol.go) so the structured result extraction
	// works uniformly across providers.
	ExecuteCode(ctx context.Context, inst *SandboxInstance, code, language string, timeoutSec int, args map[string]any) (*ExecutionResult, error)

	// DestroyInstance releases the instance. Self-managed returns true
	// unconditionally because the container is owned by the pool;
	// aliyun calls DeleteCodeInterpreter and surfaces its error.
	DestroyInstance(ctx context.Context, inst *SandboxInstance) error

	// HealthCheck returns nil if the upstream is reachable. Used by
	// the manager's lazy first-use probe and the health endpoint.
	HealthCheck(ctx context.Context) error

	// SupportedLanguages lists language identifiers the provider
	// accepts ("python", "nodejs"). Mirrors Python's
	// get_supported_languages().
	SupportedLanguages() []string
}

// normalizeLanguage maps "python3"/"javascript"/"js"/"node" to the
// canonical "python" / "nodejs" form used by the upstream sandboxes.
// Returns "" for unsupported languages so the caller can reject them.
// Matching the Python side's _normalize_language behavior
// (lowercased before comparison), this is case-insensitive.
func normalizeLanguage(in string) string {
	v := strings.ToLower(strings.TrimSpace(in))
	switch v {
	case "python", "python3":
		return "python"
	case "javascript", "js", "nodejs", "node":
		return "nodejs"
	}
	return ""
}

// validateTimeout clamps a requested timeout to the [1, 600] range.
// The Python side has the same hard 30s cap for aliyun specifically;
// 600s matches the Python self_managed range (1..600). Callers
// (CodeExec tool) pick a timeout based on user input; this is a
// defense-in-depth clamp.
func validateTimeout(t int) (int, error) {
	if t < 1 {
		return 0, fmt.Errorf("sandbox: timeout %d < 1s", t)
	}
	if t > 600 {
		return 0, fmt.Errorf("sandbox: timeout %d > 600s", t)
	}
	return t, nil
}
