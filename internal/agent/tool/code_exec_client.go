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

// CodeExec dispatches through the Go provider manager in internal/agent/sandbox.
// Self-managed execution uses the external Sandbox executor-manager HTTP API.
package tool

import (
	"context"
	"sync"
)

// SandboxClient dispatches CodeExec requests to the configured provider.
// Production installs a ManagerClient; the default stub reports missing wiring.
type SandboxClient interface {
	ExecuteCode(ctx context.Context, req SandboxRequest) (*SandboxResponse, error)
}

// SandboxRequest carries code, arguments, and timeout to the provider adapter.
type SandboxRequest struct {
	Lang      string         // "python" | "javascript"
	Script    string         // the user's code
	Arguments map[string]any // optional, passed to main(**args)
	Timeout   int            // seconds, 0 = use provider default (30s)
}

// SandboxResponse is what the sandbox returns. Stdout / Stderr are
// captured streams; Returned is the legacy alias for the
// structured main() return value; StructuredResult carries the
// full extracted payload; Metadata holds provider-specific extras.
type SandboxResponse struct {
	Stdout           string
	Stderr           string
	Returned         string
	ExitCode         int
	StructuredResult map[string]any
	Metadata         map[string]any
}

// ErrSandboxNotWired is the sentinel returned by the default stub
// client. Kept as an alias of ErrCodeExecSandboxMissing for backward
// compatibility with existing code_exec_test.go callers — the tool's
// public error surface is ErrCodeExecSandboxMissing; the client
// surface here is the same condition, named for code-path clarity.
var ErrSandboxNotWired = ErrCodeExecSandboxMissing

var (
	sandboxClientMu   sync.RWMutex
	sandboxClientImpl SandboxClient = stubSandboxClient{}
)

func SetSandboxClient(c SandboxClient) {
	sandboxClientMu.Lock()
	defer sandboxClientMu.Unlock()
	if c == nil {
		sandboxClientImpl = stubSandboxClient{}
		return
	}
	sandboxClientImpl = c
}

func GetSandboxClient() SandboxClient {
	sandboxClientMu.RLock()
	defer sandboxClientMu.RUnlock()
	return sandboxClientImpl
}

type stubSandboxClient struct{}

func (stubSandboxClient) ExecuteCode(_ context.Context, _ SandboxRequest) (*SandboxResponse, error) {
	return nil, ErrSandboxNotWired
}

func (stubSandboxClient) IsStubSandboxClient() bool { return true }
