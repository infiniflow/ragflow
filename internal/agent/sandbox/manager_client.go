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
	"fmt"
	"time"

	agenttool "ragflow/internal/agent/tool"
)

// ManagerClient adapts the active sandbox provider manager to the CodeExec
// tool's SandboxClient interface.
type ManagerClient struct {
	manager *ProviderManager
}

func NewManagerClient() *ManagerClient {
	return &ManagerClient{manager: DefaultManager()}
}

func (c *ManagerClient) ExecuteCode(ctx context.Context, req agenttool.SandboxRequest) (*agenttool.SandboxResponse, error) {
	if c == nil || c.manager == nil {
		return nil, fmt.Errorf("sandbox: provider manager unavailable")
	}
	if err := c.manager.LoadFromSettings(ctx); err != nil {
		return nil, err
	}
	provider := c.manager.Provider()
	if provider == nil {
		return nil, fmt.Errorf("sandbox: no active provider configured")
	}

	inst, err := provider.CreateInstance(ctx, req.Lang)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = provider.DestroyInstance(cleanupCtx, inst)
	}()

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30
	}
	result, err := provider.ExecuteCode(ctx, inst, req.Script, req.Lang, timeout, req.Arguments)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return &agenttool.SandboxResponse{}, nil
	}

	resp := &agenttool.SandboxResponse{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Metadata: result.Metadata,
	}
	if result.Metadata != nil {
		if structured, ok := result.Metadata["structured_result"].(map[string]any); ok {
			resp.StructuredResult = structured
		} else if structured, ok = result.Metadata["result"].(map[string]any); ok {
			resp.StructuredResult = structured
		}
	}
	if resp.StructuredResult != nil {
		if present, _ := resp.StructuredResult["present"].(bool); present {
			resp.Returned = fmt.Sprint(resp.StructuredResult["value"])
		}
	}
	return resp, nil
}
