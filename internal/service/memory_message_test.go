package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	enginetypes "ragflow/internal/engine/types"
)

func TestIsMessageDocumentNotFound(t *testing.T) {
	if !isMessageDocumentNotFound(fmt.Errorf("wrapped: %w", enginetypes.ErrDocumentNotFound)) {
		t.Fatal("expected wrapped document-not-found error to be recognized")
	}

	if isMessageDocumentNotFound(errors.New("index does not exist")) {
		t.Fatal("expected unrelated backend error to remain a server error")
	}
}

func TestRequireMemoryAccessReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := ctx.Err()
	if _, gotErr := NewMemoryService().requireMemoryAccess(ctx, "user-1", "memory-1"); !errors.Is(gotErr, err) {
		t.Fatalf("requireMemoryAccess error = %v, want %v", gotErr, err)
	}
}
