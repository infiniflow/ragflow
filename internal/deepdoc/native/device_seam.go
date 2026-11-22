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

//go:build cgo

package native

import (
	"strconv"

	ort "github.com/infiniflow/onnxruntime_go"
)

// DeviceConfig is the runtime device policy for DeepDoc inference. The default
// build always uses the zero value (CPU). An edition that supports accelerators
// fills it from RAGFLOW_DEEPDOC_DEVICE / --deepdoc-device via its own device_seam file.
type DeviceConfig struct {
	UseGPU        bool
	DeviceID      int
	MemLimitMB    int
	EPLibraryPath string
}

// String renders the device in canonical cpu|gpu[:id] form (shared, no CUDA).
func (d DeviceConfig) String() string {
	if !d.UseGPU {
		return "cpu"
	}
	if d.DeviceID == 0 {
		return "gpu:0"
	}
	return "gpu:" + strconv.Itoa(d.DeviceID)
}

// deviceConfig holds the registered policy (atomic-safe immutable value).
var deviceConfigHolder atomicDeviceConfig

// getDevice returns the current device policy.
func getDevice() DeviceConfig { return deviceConfigHolder.load() }

// SetDevice registers the process-wide device policy. Edition-specific
// normalization is supplied by normalizeDevice in the edition seam file; an
// edition that omits it fails to link, so a GPU-capable build cannot silently
// fall back to the default policy.
func SetDevice(cfg DeviceConfig) {
	cfg = normalizeDevice(cfg)
	deviceConfigHolder.store(cfg)
}

// The four functions below are implemented exactly once by the edition-specific
// seam file: device_seam_stub.go in this build, device_seam_ee.go in an
// accelerator-capable edition. They are normalizeDevice, parseDeviceHook,
// applyDeviceHook, and registerEPHook. Declaring no bodies here means a missing
// edition implementation is a link error rather than a silent no-op.

// ParseDevice parses a device spec. The default build accepts cpu only (gpu ->
// error); accelerator-capable builds accept cpu|gpu[:id].
func ParseDevice(spec string) (DeviceConfig, error) { return parseDeviceHook(spec) }

// ApplyDeviceOptions is called by newSessionOptions to attach the selected EP.
func ApplyDeviceOptions(opts *ort.SessionOptions) error { return applyDeviceHook(opts) }

// RegisterExecutionProvider loads any runtime EP plugin. Called once at
// startup after InitORT; no-op on the default build.
func RegisterExecutionProvider() error { return registerEPHook() }
