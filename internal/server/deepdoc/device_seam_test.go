//go:build cgo

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
	"testing"

	native "ragflow/internal/deepdoc/native"
)

// CPU-acceptance and canonical rendering are edition-neutral contracts and run
// in every build. The GPU-rejection / invalid-value / no-op-hook assertions are
// default-build only and live in device_seam_default_test.go.

// TestParseDeviceAcceptsCPU pins the accepted CPU spellings.
func TestParseDeviceAcceptsCPU(t *testing.T) {
	for _, spec := range []string{"", "cpu", " CPU ", "Cpu", "CPU"} {
		cfg, err := native.ParseDevice(spec)
		if err != nil {
			t.Errorf("ParseDevice(%q) error = %v, want nil", spec, err)
			continue
		}
		if cfg.UseGPU {
			t.Errorf("ParseDevice(%q) UseGPU = true, want false", spec)
		}
	}
}

// TestDeviceConfigString pins the canonical cpu|gpu[:id] rendering.
func TestDeviceConfigString(t *testing.T) {
	cases := []struct {
		cfg  native.DeviceConfig
		want string
	}{
		{native.DeviceConfig{}, "cpu"},
		{native.DeviceConfig{DeviceID: 3}, "cpu"}, // id ignored when GPU off
		{native.DeviceConfig{UseGPU: true}, "gpu:0"},
		{native.DeviceConfig{UseGPU: true, DeviceID: 3}, "gpu:3"},
	}
	for _, c := range cases {
		if got := c.cfg.String(); got != c.want {
			t.Errorf("DeviceConfig%+v.String() = %q, want %q", c.cfg, got, c.want)
		}
	}
}
