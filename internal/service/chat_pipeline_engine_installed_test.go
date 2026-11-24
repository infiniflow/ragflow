//go:build !ee

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

package service

import (
	"testing"

	"ragflow/internal/agentic_rag"
)

// TestEngineInstalledFallsBackToThePipeline pins the community edition's
// degradation: a level-6 (plan-explorer) or level-7 (iterative) turn with
// knowledge bases selects the engine, but the engine is not installed in this
// build, so the dispatcher must keep the turn on the regular pipeline instead
// of failing inside a stub. The agentic engine always passes: it is the plain
// explorer the community edition ships.
//
// This lives in its own !ee file because the assertion is about the OSS build:
// the enterprise build installs both engines, so PlanEngineAvailable is true
// there. The EE counterpart is engine_installed_ee_test.go. Keep the two in
// sync — same property, opposite expectation.
func TestEngineInstalledFallsBackToThePipeline(t *testing.T) {
	for _, tc := range []struct {
		engine agenticEngine
		want   bool
	}{
		{enginePlanExplorer, agentic_rag.PlanEngineAvailable()},
		{engineIterative, agentic_rag.IterativeEngineAvailable()},
		{engineAgenticRAG, true},
		{"", true},
	} {
		if got := engineInstalled(tc.engine); got != tc.want {
			t.Errorf("engineInstalled(%q) = %v, want %v", tc.engine, got, tc.want)
		}
	}
	// The whole point of the fallback: in a build without the engines, a level-6
	// turn with KBs selects one and is then refused by engineInstalled in the
	// dispatcher, so the turn reaches the regular pipeline.
	if sel := selectAgenticEngine(reasoningLevelPlanExplorer, true); sel == enginePlanExplorer && agentic_rag.PlanEngineAvailable() {
		t.Fatal("this test asserts the community (stub) build; PlanEngineAvailable must be false here")
	}
}
