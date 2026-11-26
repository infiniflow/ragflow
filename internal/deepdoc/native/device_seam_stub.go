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

//go:build cgo && !ee

package native

import (
	"fmt"
	"strconv"
	"strings"

	ort "github.com/infiniflow/onnxruntime_go"
)

// Default (accelerator-free) device-seam implementation. An accelerator-capable
// edition provides the same symbols under the `ee` build tag; keeping the
// implementations in mutually exclusive files means an edition that forgets
// one of them fails to compile rather than silently falling back.

func gpuAvailable() bool { return false }

// normalizeDevice in this build discards any non-CPU policy: the backend always
// runs on CPU. Negative values are clamped defensively even though they cannot
// originate from this build's parser.
func normalizeDevice(cfg DeviceConfig) DeviceConfig {
	if !gpuAvailable() {
		return DeviceConfig{}
	}
	if cfg.MemLimitMB < 0 {
		cfg.MemLimitMB = 0
	}
	if cfg.DeviceID < 0 {
		cfg.DeviceID = 0
	}
	return cfg
}

// parseDeviceHook accepts only cpu (or an empty spec). Well-formed GPU requests
// get a clear "enterprise edition" hint; malformed values get an invalid-value
// hint so a typo is not mistaken for an unsupported-but-valid device.
func parseDeviceHook(spec string) (DeviceConfig, error) {
	s := strings.ToLower(strings.TrimSpace(spec))
	if s == "" || s == "cpu" {
		return DeviceConfig{}, nil
	}
	if isGPURequest(s) {
		return DeviceConfig{}, fmt.Errorf("deepdoc device %q is not supported in this build: GPU requires the enterprise edition (use cpu)", spec)
	}
	return DeviceConfig{}, fmt.Errorf("invalid device %q: expected cpu or gpu[:id]", spec)
}

// isGPURequest reports whether spec is a well-formed gpu or gpu:N request.
// "gpu:abc" and "gpu:-1" are malformed and therefore NOT GPU requests.
func isGPURequest(s string) bool {
	if s == "gpu" {
		return true
	}
	if !strings.HasPrefix(s, "gpu:") {
		return false
	}
	id := strings.TrimPrefix(s, "gpu:")
	if id == "" {
		return false
	}
	n, err := strconv.Atoi(id)
	return err == nil && n >= 0
}

func applyDeviceHook(_ *ort.SessionOptions) error { return nil }

func registerEPHook() error { return nil }
