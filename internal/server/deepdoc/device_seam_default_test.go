//go:build cgo && !ee

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

package deepdoc

import (
	"strings"
	"testing"

	native "ragflow/internal/deepdoc/native"
)

// These assertions pin the CPU-only build contract: well-formed GPU requests
// are rejected with an enterprise hint, malformed values are rejected as
// invalid, and the session/EP hooks are no-ops. An accelerator-capable build
// has the opposite contract (GPU requests succeed, hooks install an EP), so
// these tests are gated to the default build only.

// TestParseDeviceGPURequestsReportEnterpriseEdition pins the distinguishing
// condition for a well-formed GPU request: rejected with a clear hint.
func TestParseDeviceGPURequestsReportEnterpriseEdition(t *testing.T) {
	for _, spec := range []string{"gpu", "gpu:0", "gpu:3", "GPU:1"} {
		_, err := native.ParseDevice(spec)
		if err == nil {
			t.Errorf("ParseDevice(%q) error = nil, want enterprise-edition rejection", spec)
			continue
		}
		if !strings.Contains(err.Error(), "enterprise edition") {
			t.Errorf("ParseDevice(%q) error = %q, want 'enterprise edition' hint", spec, err.Error())
		}
	}
}

// TestParseDeviceInvalidValueReportsInvalid is the adversarial counterpart:
// malformed / unknown specs must be classified as invalid values and must NOT
// falsely claim the value is a GPU request needing the enterprise edition.
func TestParseDeviceInvalidValueReportsInvalid(t *testing.T) {
	for _, spec := range []string{"tpu", "cuda", "cpuu", "npu:0", "gpu:abc", "gpu:-1", "0"} {
		_, err := native.ParseDevice(spec)
		if err == nil {
			t.Errorf("ParseDevice(%q) error = nil, want invalid-value rejection", spec)
			continue
		}
		if !strings.Contains(err.Error(), "invalid device") {
			t.Errorf("ParseDevice(%q) error = %q, want 'invalid device' hint", spec, err.Error())
		}
		if strings.Contains(err.Error(), "enterprise edition") {
			t.Errorf("ParseDevice(%q) error = %q, must NOT mention 'enterprise edition' for a malformed value", spec, err.Error())
		}
	}
}

// TestDefaultBuildHooksAreNoOps guards that the default edition's session/EP
// hooks stay inert: RegisterExecutionProvider must not error in a CPU-only build.
func TestDefaultBuildHooksAreNoOps(t *testing.T) {
	if err := native.RegisterExecutionProvider(); err != nil {
		t.Errorf("RegisterExecutionProvider() = %v, want nil", err)
	}
}
