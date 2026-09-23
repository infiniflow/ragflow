//go:build cgo

package native

import (
	"fmt"
	"sync/atomic"
)

// intraOpThreads is the intra-op thread count every ONNX session is opened
// with. It is process-global policy registered once at startup (see
// SetIntraOpThreads) and read on every session creation.
//
// ONNX Runtime gives each session its own intra-op thread pool (the C API never
// switches a session onto a shared/global pool), so the threads a single DeepDoc
// inference Run occupies equal intraOpThreads. The process-wide inference budget
// (see inference_limit.go) bounds how many Runs may be in flight at once, so the
// total cores inference may occupy is intraOpThreads × concurrency.
//
// Historically this was pinned to 1 (each Run cost exactly one core, making the
// concurrency budget a plain core count). It is now configurable: the process
// owner computes intraOpThreads = max(1, N/K) from the configured CPU-core
// budget N and inference concurrency K, and registers it once at startup before
// any session is created.
var intraOpThreads int32 = 1

// SetIntraOpThreads sets the intra-op thread count every subsequently created
// session opens with. It is meant to be called exactly once at startup, before
// any model session is created. Non-positive values are clamped to 1: ORT needs
// at least one thread per session, and a zero/negative request is a caller bug.
func SetIntraOpThreads(n int) {
	if n < 1 {
		n = 1
	}
	atomic.StoreInt32(&intraOpThreads, int32(n))
}

// intraOpThreadCount returns the currently configured intra-op thread count.
// Sessions call it at creation time; after startup registration it is stable.
func intraOpThreadCount() int {
	return int(atomic.LoadInt32(&intraOpThreads))
}

// ValidateInferenceConfig computes the per-inference core budget from the
// requested configuration and the machine's available core count, failing fast
// on any invalid combination.
//
//   - totalCores is the process's available CPU budget (callers pass
//     runtime.GOMAXPROCS(0), which is cgroup-quota aware in Go 1.25+, rather than
//     runtime.NumCPU(), which ignores a container's CPU limit). It bounds both the
//     "all cores" resolution of rawCPUCores == 0 and every validation ceiling below.
//   - rawCPUCores is the requested CPU-core budget N. A value of 0 means "use
//     all cores" and is resolved to totalCores.
//   - concurrency is the requested inference concurrency K (max in-flight Runs).
//
// It returns coresPerInference (max(1, N/K), the intra-op thread count per Run)
// and totalCPUCores (the resolved N). The following are rejected:
//   - concurrency < 1 (not a valid integer / below the minimum),
//   - concurrency > totalCores (cannot run more parallel Runs than cores),
//   - rawCPUCores > totalCores (cannot allocate more cores than the machine has),
//   - concurrency > resolvedN (the CPU-core budget N is a hard ceiling: when
//     K > N, max(1, N/K) floors at 1 so total occupancy would be K, oversubscribing
//     the box beyond the budget N).
//
// totalCPUCores is returned as the resolved N so callers can log the effective
// budget; it equals totalCores when rawCPUCores == 0.
func ValidateInferenceConfig(totalCores, rawCPUCores, concurrency int) (coresPerInference, totalCPUCores int, err error) {
	if concurrency < 1 {
		return 0, 0, fmt.Errorf("deepdoc inference_concurrency %d invalid: must be a positive integer", concurrency)
	}
	if concurrency > totalCores {
		return 0, 0, fmt.Errorf("deepdoc inference_concurrency %d exceeds available CPU cores %d", concurrency, totalCores)
	}
	resolved := rawCPUCores
	if resolved == 0 {
		resolved = totalCores
	}
	if resolved < 0 {
		return 0, 0, fmt.Errorf("deepdoc inference_cpu_cores %d invalid: must be >= 0 (0 means all cores)", resolved)
	}
	if resolved > totalCores {
		return 0, 0, fmt.Errorf("deepdoc inference_cpu_cores %d exceeds available CPU cores %d", resolved, totalCores)
	}
	if concurrency > resolved {
		return 0, 0, fmt.Errorf("deepdoc inference_concurrency %d exceeds cpu-core budget %d (inference_cpu_cores)", concurrency, resolved)
	}
	return max(1, resolved/concurrency), resolved, nil
}
