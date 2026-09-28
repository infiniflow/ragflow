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

package task

import (
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestKnowledgeCompilerRegisteredByWiring locks the composition-root contract:
// the task package imports the knowledge_compiler root package (via blank
// import in knowledge_compiler_wiring.go) so its init() registers the
// knowledge-compilation component under the unified name "Compiler" in the
// production runtime registry. Without this blank import the component is never
// registered and an ingestor consuming a canvas with a Compiler node fails with
// "unknown component".
func TestKnowledgeCompilerRegisteredByWiring(t *testing.T) {
	factory, category, _, ok := runtime.DefaultRegistry.Lookup("Compiler")
	if !ok {
		t.Fatal("knowledge-compiler component \"Compiler\" is not registered; the task package blank-import must be present for its init() to run")
	}
	if category != runtime.CategoryIngestion {
		t.Fatalf("component \"Compiler\" category = %q, want %q", category, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Fatal("component \"Compiler\" registered with a nil factory")
	}
}
