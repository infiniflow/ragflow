package common

import (
	"errors"
	"testing"
)

func TestBestEffort_NoPanicOnSuccess(t *testing.T) {
	// Must not panic when f returns nil.
	BestEffort("test-success", func() error { return nil })
}

func TestBestEffort_NoPanicOnError(t *testing.T) {
	// Must not panic when f returns an error — it should be logged, not
	// propagated.
	BestEffort("test-fail", func() error { return errors.New("boom") })
}

func TestBestEffort_NoPanicOnNilFunc(t *testing.T) {
	// Must not panic when f is nil.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("BestEffort with nil func panicked: %v", r)
		}
	}()
	BestEffort("test-nil", nil)
}
