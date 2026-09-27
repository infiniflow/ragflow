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

package tree

import "context"

// batchSubmitter fans out independent Tree extraction jobs on the process-wide
// knowledge-compilation pool. It is injected by the knowledge_compiler wiring
// so Tree shares the same concurrency bound as the other compiler variants;
// when nil, batches run sequentially for direct package use and tests.
var batchSubmitter func(ctx context.Context, jobs []func() error) error

// SetBatchSubmitter installs the shared-pool fan-out used by Tree extraction.
// Pass nil to revert to serial execution.
func SetBatchSubmitter(submit func(ctx context.Context, jobs []func() error) error) {
	batchSubmitter = submit
}

// runBatches mirrors the other compiler variants: concurrent under the wired
// global compiler pool, or serial when no submitter is set. The first error is
// returned after all jobs settle; the global pool is never StopWait'd.
func runBatches(ctx context.Context, jobs []func() error) error {
	if len(jobs) == 0 {
		return nil
	}
	if batchSubmitter != nil {
		return batchSubmitter(ctx, jobs)
	}
	for _, job := range jobs {
		if err := job(); err != nil {
			return err
		}
	}
	return nil
}
