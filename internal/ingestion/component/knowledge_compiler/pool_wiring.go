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

package knowledge_compiler

import (
	"context"

	"ragflow/internal/ingestion/component/knowledge_compiler/mindmap"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
	"ragflow/internal/ingestion/component/knowledge_compiler/wiki"
	"ragflow/internal/ingestion/knowledge_compile"
)

// init wires every knowledge-compiler variant's batch fan-out to the single
// process-wide compiler pool (vCPU-sized, defined in the knowledge_compile
// package). This unifies the component-level MAP/extraction stages with the
// consumer-level KNN / LLM-merge / write stages under one shared concurrency
// bound, so docengine-bounded and LLM-bounded work never exceed the host's
// core count regardless of which stage is running.
func init() {
	submit := func(ctx context.Context, jobs []func() error) error {
		return knowledge_compile.SubmitCompilerJobs(ctx, jobs)
	}
	structure.SetBatchSubmitter(submit)
	mindmap.SetBatchSubmitter(submit)
	wiki.SetBatchSubmitter(submit)
}
