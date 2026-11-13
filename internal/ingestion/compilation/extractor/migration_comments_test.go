// Package extractor_test hosts the static hygiene check that flags comments still
// anchoring the Go implementation on the deleted earlier codebase.
package extractor_test

import (
	"testing"

	"ragflow/internal/ingestion/internal/migrationguard"
)

// TestNoMigrationHistoryComments fails if any non-test .go file under this
// package still anchors a comment on the deleted earlier codebase (the word
// "python", a .py file, or a rag.(flow|nlp|app|deepdoc) module path). See
// migrationguard for the exact patterns and rationale.
func TestNoMigrationHistoryComments(t *testing.T) {
	migrationguard.Check(t, ".")
}
