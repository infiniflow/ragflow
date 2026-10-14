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

import "fmt"

// Knowledge compilation scopes: "dataset" keeps the merged dataset-level rows,
// "doc" keeps the per-document rows, "both" (the default) keeps either.
const (
	knowledgeCompilationScopeDataset = "dataset"
	knowledgeCompilationScopeDoc     = "doc"
	knowledgeCompilationScopeBoth    = "both"
)

// ValidateKnowledgeCompilationScope rejects a knowledge_compilation_scope other
// than dataset, doc or both. An empty scope means both.
func ValidateKnowledgeCompilationScope(scope string) error {
	switch scope {
	case "", knowledgeCompilationScopeBoth, knowledgeCompilationScopeDataset, knowledgeCompilationScopeDoc:
		return nil
	}
	return fmt.Errorf("knowledge_compilation_scope must be one of dataset, doc, both")
}

// CompiledChunksFilter returns the retrieval filter for knowledge-compilation
// rows, or nil when every row stays. include=false drops every compiled row
// (compile_kwd set) and wins over scope; a dataset or doc scope drops the
// compiled rows whose scope_kwd differs. Source chunks never carry compile_kwd,
// so they are kept either way.
func CompiledChunksFilter(include *bool, scope string) map[string]interface{} {
	if include != nil && !*include {
		return map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}}
	}
	if scope != knowledgeCompilationScopeDataset && scope != knowledgeCompilationScopeDoc {
		return nil
	}
	return map[string]interface{}{"must_not": map[string]interface{}{
		"exists":   "compile_kwd",
		"must_not": map[string]interface{}{"scope_kwd": scope},
	}}
}
