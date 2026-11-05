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

package opensearch

import (
	"math"
	"strings"
	"testing"
)

func TestParseSemVer(t *testing.T) {
	tests := []struct {
		input       string
		major       int
		minor       int
		patch       int
		expectError bool
	}{
		{"2.19.0", 2, 19, 0, false},
		{"v2.19.1", 2, 19, 1, false},
		{"V2.19.7", 2, 19, 7, false},
		{"2.19.0-SNAPSHOT", 2, 19, 0, false},
		{"2.19.2+build.42", 2, 19, 2, false},
		{"2.18.1", 2, 18, 1, false},
		{"1.3.14", 1, 3, 14, false},
		{"3.0", 3, 0, 0, false},
		{"", 0, 0, 0, true},
		{"invalid", 0, 0, 0, true},
	}

	for _, tc := range tests {
		maj, min, patch, err := parseSemVer(tc.input)
		if tc.expectError {
			if err == nil {
				t.Errorf("parseSemVer(%q) expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseSemVer(%q) unexpected error: %v", tc.input, err)
			}
			if maj != tc.major || min != tc.minor || patch != tc.patch {
				t.Errorf("parseSemVer(%q) = (%d, %d, %d), expected (%d, %d, %d)",
					tc.input, maj, min, patch, tc.major, tc.minor, tc.patch)
			}
		}
	}
}

func TestNormalizeWeights(t *testing.T) {
	tests := []struct {
		name     string
		input    []float64
		expected []float64
	}{
		{
			name:     "already normalized",
			input:    []float64{0.3, 0.7},
			expected: []float64{0.3, 0.7},
		},
		{
			name:     "unnormalized integers",
			input:    []float64{1.0, 3.0},
			expected: []float64{0.25, 0.75},
		},
		{
			name:     "nil weights",
			input:    nil,
			expected: []float64{0.5, 0.5},
		},
		{
			name:     "single element",
			input:    []float64{1.0},
			expected: []float64{0.5, 0.5},
		},
		{
			name:     "three elements",
			input:    []float64{0.2, 0.3, 0.5},
			expected: []float64{0.5, 0.5},
		},
		{
			name:     "all zeros",
			input:    []float64{0.0, 0.0},
			expected: []float64{0.5, 0.5},
		},
		{
			name:     "one negative",
			input:    []float64{-0.5, 1.0},
			expected: []float64{0.0001, 0.9999},
		},
		{
			name:     "NaN element",
			input:    []float64{math.NaN(), 0.5},
			expected: []float64{0.5, 0.5},
		},
		{
			name:     "Inf element",
			input:    []float64{math.Inf(1), 0.5},
			expected: []float64{0.5, 0.5},
		},
		{
			name:     "overflowing elements",
			input:    []float64{1e308, 1e308},
			expected: []float64{0.5, 0.5},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeWeights(tc.input)
			if len(got) != 2 {
				t.Fatalf("NormalizeWeights returned length %d, expected 2", len(got))
			}
			if math.Abs(got[0]-tc.expected[0]) > 1e-6 || math.Abs(got[1]-tc.expected[1]) > 1e-6 {
				t.Errorf("NormalizeWeights(%v) = %v, expected %v", tc.input, got, tc.expected)
			}
			if math.Abs((got[0]+got[1])-1.0) > 1e-6 {
				t.Errorf("Weights do not sum to 1.0: %v (sum=%f)", got, got[0]+got[1])
			}
		})
	}
}

func TestBuildInlinePipeline(t *testing.T) {
	pipe := BuildInlinePipeline([]float64{0.3, 0.7})
	if pipe == nil {
		t.Fatalf("BuildInlinePipeline returned nil")
	}

	procs, ok := pipe["phase_results_processors"].([]interface{})
	if !ok || len(procs) != 1 {
		t.Fatalf("expected 1 phase_results_processor, got %#v", pipe["phase_results_processors"])
	}

	proc, ok := procs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected processor map, got %#v", procs[0])
	}

	normProc, ok := proc["normalization-processor"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected normalization-processor, got %#v", proc)
	}

	norm, ok := normProc["normalization"].(map[string]interface{})
	if !ok || norm["technique"] != "min_max" {
		t.Errorf("expected min_max normalization technique, got %#v", norm)
	}

	comb, ok := normProc["combination"].(map[string]interface{})
	if !ok || comb["technique"] != "arithmetic_mean" {
		t.Errorf("expected arithmetic_mean combination technique, got %#v", comb)
	}

	params, ok := comb["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected combination parameters, got %#v", comb)
	}

	weights, ok := params["weights"].([]float64)
	if !ok || len(weights) != 2 {
		t.Fatalf("expected []float64 weights with len 2, got %#v", params["weights"])
	}
	if math.Abs(weights[0]-0.3) > 1e-6 || math.Abs(weights[1]-0.7) > 1e-6 {
		t.Errorf("expected weights [0.3, 0.7], got %v", weights)
	}
}

func TestNewPipelineManager_VersionValidation(t *testing.T) {
	// 1. Accepted versions: 2.19.x
	accepted := []string{"2.19.0", "2.19.7", "v2.19.1", "2.19.0-SNAPSHOT", "2.19.12"}
	for _, ver := range accepted {
		pm, err := newPipelineManager(ver)
		if err != nil {
			t.Errorf("expected version %q to be accepted, got error: %v", ver, err)
			continue
		}
		if pm.Major() != 2 || pm.Minor() != 19 {
			t.Errorf("expected major=2, minor=19 for %q, got (%d, %d)", ver, pm.Major(), pm.Minor())
		}
		if pm.Version() != ver {
			t.Errorf("expected version string %q, got %q", ver, pm.Version())
		}
	}

	// 2. Compatibility mode 7.10.2 rejected with clear instructions
	pmCompat, errCompat := newPipelineManager("7.10.2")
	if errCompat == nil || pmCompat != nil {
		t.Errorf("expected 7.10.2 compatibility mode to be rejected, got pm=%v, err=nil", pmCompat)
	} else {
		errMsg := errCompat.Error()
		if !strings.Contains(errMsg, "7.x compatibility mode is not supported") {
			t.Errorf("expected error message to mention 7.x compatibility mode, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "compatibility.override_main_response_version") {
			t.Errorf("expected error message to mention compatibility.override_main_response_version, got: %s", errMsg)
		}
	}

	// 3. Rejected other versions: < 2.19, >= 3.x, 1.x
	rejected := []string{"2.18.0", "2.17.1", "2.10.0", "2.8.0", "1.3.14", "3.0.0"}
	for _, ver := range rejected {
		pm, err := newPipelineManager(ver)
		if err == nil || pm != nil {
			t.Errorf("expected version %q to be rejected, got pm=%v, err=nil", ver, pm)
			continue
		}
		if !strings.Contains(err.Error(), "only OpenSearch 2.19.x is supported") {
			t.Errorf("expected 'only OpenSearch 2.19.x is supported' error for %q, got: %v", ver, err)
		}
	}

	// 4. Unparseable versions
	unparseable := []string{"invalid-version", "", "not.a.version"}
	for _, ver := range unparseable {
		pm, err := newPipelineManager(ver)
		if err == nil || pm != nil {
			t.Errorf("expected unparseable version %q to be rejected, got pm=%v, err=nil", ver, pm)
		}
	}
}
