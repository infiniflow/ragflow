package runtimeconfig_test

import (
	"runtime"
	"testing"

	"ragflow/internal/deepdoc/runtimeconfig"
)

func TestORTThreads(t *testing.T) {
	t.Setenv("DEEPDOC_ORT_NUM_THREADS", "")
	got := runtimeconfig.ORTThreads()
	if got < 1 || got > runtime.GOMAXPROCS(0) {
		t.Fatalf("got %d threads for GOMAXPROCS=%d", got, runtime.GOMAXPROCS(0))
	}

	t.Setenv("DEEPDOC_ORT_NUM_THREADS", "2")
	if got := runtimeconfig.ORTThreads(); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestInferenceConcurrency(t *testing.T) {
	got := runtimeconfig.InferenceConcurrency()
	if got < 1 || got > 8 {
		t.Fatalf("got %d, want between 1 and 8", got)
	}
}
