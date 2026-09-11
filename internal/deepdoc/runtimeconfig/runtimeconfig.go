// Package runtimeconfig coordinates process-wide DeepDoc CPU usage.
package runtimeconfig

import (
	"os"
	"runtime"
	"strconv"
)

const (
	maxDefaultORTThreads          = 4
	maxDefaultInferenceConcurrent = 8
)

// ORTThreads returns the intra-op thread count used by one ONNX inference.
func ORTThreads() int {
	cpus := max(1, runtime.GOMAXPROCS(0))
	if value, err := strconv.Atoi(os.Getenv("DEEPDOC_ORT_NUM_THREADS")); err == nil && value > 0 {
		return min(value, cpus)
	}
	return min(maxDefaultORTThreads, max(1, cpus/4))
}

// InferenceConcurrency returns the process-wide number of concurrent model calls.
func InferenceConcurrency() int {
	cpus := max(1, runtime.GOMAXPROCS(0))
	return min(maxDefaultInferenceConcurrent, max(1, cpus/ORTThreads()))
}
