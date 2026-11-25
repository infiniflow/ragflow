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

//go:build !windows

package sandbox

import "testing"

// TestNewLocalProviderFromConfig_Defaults pins the local provider's
// settings-driven defaults.
func TestNewLocalProviderFromConfig_Defaults(t *testing.T) {
	t.Parallel()
	p := newLocalProviderFromConfig(map[string]any{})
	if p.pythonBin != localDefaultPythonBin {
		t.Errorf("pythonBin = %q, want default", p.pythonBin)
	}
	if p.nodeBin != localDefaultNodeBin {
		t.Errorf("nodeBin = %q, want default", p.nodeBin)
	}
	if p.workDir != localDefaultWorkDir {
		t.Errorf("workDir = %q, want default", p.workDir)
	}
	if p.timeout != localDefaultTimeout {
		t.Errorf("timeout = %d, want default", p.timeout)
	}
}

// TestNewLocalProviderFromConfig_FullConfig verifies that config
// keys override every field.
func TestNewLocalProviderFromConfig_FullConfig(t *testing.T) {
	t.Parallel()
	cfg := map[string]any{
		"python_bin":         "python3.12",
		"node_bin":           "node22",
		"work_dir":           "/var/sandbox",
		"timeout":            float64(60),
		"max_output_bytes":   float64(2_000_000),
		"max_artifacts":      float64(50),
		"max_artifact_bytes": float64(20_000_000),
	}
	p := newLocalProviderFromConfig(cfg)
	if p.pythonBin != "python3.12" {
		t.Errorf("pythonBin = %q", p.pythonBin)
	}
	if p.nodeBin != "node22" {
		t.Errorf("nodeBin = %q", p.nodeBin)
	}
	if p.workDir != "/var/sandbox" {
		t.Errorf("workDir = %q", p.workDir)
	}
	if p.timeout != 60 {
		t.Errorf("timeout = %d", p.timeout)
	}
	if p.maxOutputBytes != 2_000_000 {
		t.Errorf("maxOutputBytes = %d", p.maxOutputBytes)
	}
	if p.maxArtifacts != 50 {
		t.Errorf("maxArtifacts = %d", p.maxArtifacts)
	}
	if p.maxArtifactBytes != 20_000_000 {
		t.Errorf("maxArtifactBytes = %d", p.maxArtifactBytes)
	}
}
