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
	want := min(2, runtime.GOMAXPROCS(0))
	if got := runtimeconfig.ORTThreads(); got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestInferenceConcurrency(t *testing.T) {
	got := runtimeconfig.InferenceConcurrency()
	if got < 1 || got > 8 {
		t.Fatalf("got %d, want between 1 and 8", got)
	}
}
