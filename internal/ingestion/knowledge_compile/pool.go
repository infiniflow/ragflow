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

package knowledge_compile

import (
	"context"
	"os"
	"runtime"
	"strconv"

	"ragflow/internal/utility"
)

// CompilerJob is one unit of knowledge-compilation work (an I/O- or
// LLM-bounded task) executed on the shared global pool. It is an exported type
// alias for func() error so callers (including lower-level packages such as the
// knowledge_compiler wiring) can pass plain []func() error slices without a cast.
type CompilerJob = func() error

// compilerPool is the process-wide bounded worker pool that drives cross-doc
// concurrency for every knowledge-compilation stage: the DocEngine KNN pass in
// processBatch, the LLM merge-decision batches inside DecideBatch, and the
// merged-product writes/deletes. It mirrors internal/ingestion/component/
// extractor.go's extractorPool: held globally so every Consumer invocation
// shares one rate limiter instead of spinning up a pool per batch. The pool
// only bounds concurrency (it is never StopWait'd), so concurrent processBatch
// calls do not disturb each other — each call tracks completion with its own
// WaitGroup + first-error collection.
//
// The fixed size is the host vCPU count: the stages are docengine-bounded
// (KNN / write / delete) or LLM-bounded (merge decisions) rather than
// CPU-bounded, so the degree of useful parallelism is capped by the number of
// available cores rather than by a hand-tuned constant.
var compilerPool = utility.NewWorkerPool[CompilerJob, struct{}](
	compilerConcurrency(),
	compilerConcurrency()*4,
	func(_ context.Context, j CompilerJob) (struct{}, error) { return struct{}{}, j() },
)

// compilerConcurrency resolves the global pool size. It defaults to the host
// vCPU count, overridable via KC_COMPILE_CONCURRENCY (mirroring the extractor
// pool's MAX_CONCURRENT_CHATS tuning knob).
func compilerConcurrency() int {
	if v := os.Getenv("KC_COMPILE_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	n := runtime.NumCPU()
	if n <= 0 {
		return 1
	}
	return n
}

// SetCompilerConcurrency overrides the global pool size at runtime (e.g. from
// service init or tests). Mirrors SetExtractorConcurrency.
func SetCompilerConcurrency(n int) {
	if n > 0 {
		compilerPool.Resize(n)
	}
}

// runCompilerJobs submits every job to the global pool and waits for all to
// finish, returning the first non-nil error (if any). ctx cancellation aborts
// outstanding jobs.
//
// No per-job goroutines are spun up: Submit is non-blocking until the pool's
// input buffer fills (vCPU*4 deep), so we first collect one future per job and
// then Wait on each in a second pass on the calling goroutine. This keeps the
// fan-out bounded by the shared pool's worker count while avoiding len(jobs)
// short-lived goroutines.
func runCompilerJobs(ctx context.Context, jobs []CompilerJob) error {
	if len(jobs) == 0 {
		return nil
	}
	futures := make([]utility.WorkerPoolFuture[CompilerJob, struct{}], 0, len(jobs))
	var firstErr error
	for _, j := range jobs {
		f, err := compilerPool.Submit(ctx, j)
		if err != nil {
			// Pool stopped / ctx done before we could enqueue the rest:
			// remember it and stop submitting; we still await what is queued.
			if firstErr == nil {
				firstErr = err
			}
			break
		}
		futures = append(futures, f)
	}
	for _, f := range futures {
		res, werr := f.Wait(ctx)
		if werr != nil {
			// Wait returns the context error (not a result error) when ctx wins
			// the select; surface it so callers don't see a clean nil while jobs
			// are incomplete.
			if firstErr == nil {
				firstErr = werr
			}
			continue
		}
		if res.Err != nil && firstErr == nil {
			firstErr = res.Err
		}
	}
	return firstErr
}

// SubmitCompilerJob runs a single job on the global pool and waits for it,
// returning its error. Used to inject bounded parallelism into lower-level
// packages (e.g. structure.LLMMergeDecider) without creating an import cycle.
func SubmitCompilerJob(ctx context.Context, fn CompilerJob) error {
	f, err := compilerPool.Submit(ctx, fn)
	if err != nil {
		return err
	}
	res, werr := f.Wait(ctx)
	if werr != nil {
		return werr
	}
	return res.Err
}

// CompilerBatchSubmitter is the fan-out contract injected into lower-level
// knowledge_compiler variant packages (structure/mindmap) so every stage shares
// the one process-wide compiler pool. Implementations must submit every job to
// the shared pool, wait for all to finish, and return the first non-nil error
// (without StopWait-ing the global pool).
type CompilerBatchSubmitter func(ctx context.Context, jobs []CompilerJob) error

// SubmitCompilerJobs fans out a batch of jobs on the global pool and returns the
// first error. This is the CompilerBatchSubmitter handed to variant packages.
func SubmitCompilerJobs(ctx context.Context, jobs []CompilerJob) error {
	return runCompilerJobs(ctx, jobs)
}
