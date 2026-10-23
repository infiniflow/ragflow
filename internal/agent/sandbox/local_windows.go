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

//go:build windows

package sandbox

import (
	"context"
	"errors"
)

// errLocalUnsupported is returned by the local provider on Windows, where the
// Unix process-group control used by the implementation in local.go is not
// available. buildProvider still compiles and reports a clear error instead of
// the package failing to build.
var errLocalUnsupported = errors.New("local sandbox provider is not supported on Windows")

// newLocalProviderFromEnv mirrors the Unix constructor so buildProvider links
// on Windows.
func newLocalProviderFromEnv() SandboxProvider {
	return unsupportedLocalProvider{}
}

// newLocalProviderFromConfig mirrors the Unix constructor so
// buildProviderFromConfig links on Windows.
func newLocalProviderFromConfig(cfg map[string]any) SandboxProvider {
	return unsupportedLocalProvider{}
}

// unsupportedLocalProvider satisfies SandboxProvider on Windows; every
// operation fails with errLocalUnsupported.
type unsupportedLocalProvider struct{}

func (unsupportedLocalProvider) Initialize(context.Context) error { return errLocalUnsupported }

func (unsupportedLocalProvider) ProviderType() ProviderType { return ProviderLocal }

func (unsupportedLocalProvider) CreateInstance(context.Context, string) (*SandboxInstance, error) {
	return nil, errLocalUnsupported
}

func (unsupportedLocalProvider) ExecuteCode(context.Context, *SandboxInstance, string, string, int, map[string]any) (*ExecutionResult, error) {
	return nil, errLocalUnsupported
}

func (unsupportedLocalProvider) DestroyInstance(context.Context, *SandboxInstance) error {
	return errLocalUnsupported
}

func (unsupportedLocalProvider) HealthCheck(context.Context) error { return errLocalUnsupported }

func (unsupportedLocalProvider) SupportedLanguages() []string { return nil }
