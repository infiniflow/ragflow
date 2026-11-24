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

package main

import (
	"testing"

	"github.com/spf13/viper"
	native "ragflow/internal/deepdoc/native"
	server "ragflow/internal/server/config"
)

func TestValidateInferenceConfigKNExceedsBudget(t *testing.T) {
	if _, _, err := native.ValidateInferenceConfig(8, 2, 8); err == nil {
		t.Fatal("expected error when concurrency K=8 exceeds cpu-core budget N=2")
	}
	if c, total, err := native.ValidateInferenceConfig(8, 4, 2); err != nil {
		t.Fatalf("unexpected error for K<=N: %v", err)
	} else if c != 2 || total != 4 {
		t.Fatalf("coresPerInference=%d totalCPUCores=%d, want 2/4", c, total)
	}
}

// TestRegisterNativeDeepDocDefaultBootsOnSmallMachine pins the startup contract
// that the shipped default configuration (both deepdoc keys unset, so K defaults
// to 1 and N is derived from K) still boots on a host with fewer than the
// nominal core count: the derived N equals K, ValidateInferenceConfig accepts
// them, and registerNativeDeepDoc must not call common.Fatal. This mirrors the
// resolution order in registerNativeDeepDoc and guards against a regression
// where an explicit default larger than the host core count would fatally reject
// startup.
func TestRegisterNativeDeepDocDefaultBootsOnSmallMachine(t *testing.T) {
	const totalCores = 2
	cfg := &server.Config{}
	if err := cfg.ParseIngestorConfig(viper.New()); err != nil {
		t.Fatalf("ParseIngestorConfig: %v", err)
	}
	// No CLI override; both config keys unset.
	K, explicitK, errK := cfg.GetDeepDocInferenceConcurrency(nil)
	if errK != nil {
		t.Fatalf("GetDeepDocInferenceConcurrency: %v", errK)
	}
	rawN, explicitN, errN := cfg.GetDeepDocInferenceCPUCores(nil)
	if errN != nil {
		t.Fatalf("GetDeepDocInferenceCPUCores: %v", errN)
	}
	if explicitK || explicitN {
		t.Fatal("default config must report both K and N as not-explicit")
	}
	// Unset N is derived from K, preserving single-threaded-per-run semantics.
	resolvedN := rawN
	if !explicitN {
		resolvedN = K
	}
	if K != 1 {
		t.Fatalf("default K = %d, want 1", K)
	}
	if resolvedN != K {
		t.Fatalf("derived N = %d, want K=%d", resolvedN, K)
	}
	if _, _, err := native.ValidateInferenceConfig(totalCores, resolvedN, K); err != nil {
		t.Fatalf("default config on %d-core host must validate, got: %v", totalCores, err)
	}
}
