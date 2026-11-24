//go:build ee

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

// TestEngineInstalledKeepsTheEngine is the enterprise counterpart of
// TestEngineInstalledFallsBackToThePipeline (!ee). The property is the same —
// engineInstalled must agree with the build's own availability seam — but the
// expectation is inverted: this build ships both engines, so a level-6 or
// level-7 turn with knowledge bases keeps its engine instead of being cleared
// back to the regular pipeline.
func TestEngineInstalledKeepsTheEngine(t *testing.T) {
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
	if !agentic_rag.PlanEngineAvailable() {
		t.Fatal("this test asserts the enterprise build; PlanEngineAvailable must be true here")
	}
	if sel := selectAgenticEngine(reasoningLevelPlanExplorer, true); sel != enginePlanExplorer {
		t.Fatalf("a plan-engine turn must keep the engine when it is installed, selected %q", sel)
	}
}
