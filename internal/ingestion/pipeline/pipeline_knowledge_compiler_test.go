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

package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	// Blank-import the KnowledgeCompiler component so its runtime.MustRegister
	// init hook fires and the component name referenced by the template is
	// resolvable in the default runtime factory.
	"ragflow/internal/agent/runtime"
	// Blank-import the parent component package so its init registers the
	// File/Parser/TokenChunker components this test's template references.
	_ "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	_ "ragflow/internal/ingestion/component/knowledge_compiler"
)

// TestKnowledgeCompilerTemplate_RegisteredAndDecodable verifies the
// knowledge_compiler ingestion template is loaded by the built-in registry,
// carries the expected title, references runtime-registered components, and
// decodes into a pipeline via NewPipelineFromDSL. A full Run is intentionally
// not exercised here: it needs real LLM/embedder/ES wiring (covered by the
// knowledge_compiler component E2E tests) and is skipped in the headless
// chunks-smoke test.
func TestKnowledgeCompilerTemplate_RegisteredAndDecodable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	tpl, ok := r.Get("knowledge_compiler")
	if !ok {
		t.Fatal("knowledge_compiler template not registered in built-in registry")
	}
	if tpl.Title != "Knowledge Compiler" {
		t.Fatalf("title = %q, want 'Knowledge Compiler'", tpl.Title)
	}

	// Ensure the default runtime factory is installed, then confirm every
	// component_name the template references is actually registered.
	runtime.InstallDefaultRegistryFactory()
	if runtime.DefaultFactory() == nil {
		t.Fatal("default runtime factory not installed")
	}
	for _, name := range []string{"File", "Parser", "TokenChunker", "Compiler"} {
		if _, _, _, ok := runtime.DefaultRegistry.Lookup(name); !ok {
			t.Errorf("component %q referenced by template is not registered in the runtime factory", name)
		}
	}

	path := filepath.Join(repoRootFromPipelineTest(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_knowledge_compiler.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if _, err := NewPipelineFromDSL(raw, "kc-validation"); err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
}
