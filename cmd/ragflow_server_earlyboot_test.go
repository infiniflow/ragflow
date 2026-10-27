// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"testing"

	"ragflow/internal/common"
)

// TestIsStandaloneDeepDocServer pins the contract used by main() to decide
// whether a server type is a standalone DeepDoc inference server that must boot
// BEFORE the heavy RAGFlow infrastructure (Redis/DB/engine/cache/storage/MQ) is
// initialized. The standalone DeepDoc types are exactly the three DeepDoc
// variants; every other server type (including the unknown sentinel) must NOT be
// treated as standalone.
//
// The adversarial cases (non-DeepDoc types returning false, and DeepDoc types
// returning true) make the distinguishing condition falsifiable: an
// implementation that returns true for everything, or that forgets one of the
// three DeepDoc variants, fails here.
func TestIsStandaloneDeepDocServer(t *testing.T) {
	standalone := []common.ServerType{
		common.ServerTypeDeepDocCPU,
		common.ServerTypeDeepDocGPU,
		common.ServerTypeDeepDocAscend,
	}
	for _, st := range standalone {
		if !isStandaloneDeepDocServer(st) {
			t.Errorf("isStandaloneDeepDocServer(%q) = false, want true", st)
		}
	}

	notStandalone := []common.ServerType{
		common.ServerTypeAPI,
		common.ServerTypeAdmin,
		common.ServerTypeIngestion,
		common.ServerTypeFileSyncer,
		common.ServerTypeMigrate,
		common.ServerTypeUnknown,
		common.ServerType("some-future-type"),
	}
	for _, st := range notStandalone {
		if isStandaloneDeepDocServer(st) {
			t.Errorf("isStandaloneDeepDocServer(%q) = true, want false", st)
		}
	}
}
