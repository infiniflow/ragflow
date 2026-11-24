//go:build !ee

package common

import (
	"errors"
	"testing"
)

func TestUnsupportedTimelineKind(t *testing.T) {
	if _, err := KindToVariant("timeline"); !errors.Is(err, ErrUnknownVariant) {
		t.Fatalf("KindToVariant(timeline) error = %v", err)
	}
	if _, err := KindToTaskType("timeline"); !errors.Is(err, ErrUnknownVariant) {
		t.Fatalf("KindToTaskType(timeline) error = %v", err)
	}
}
