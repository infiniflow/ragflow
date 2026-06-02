package service

import (
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
