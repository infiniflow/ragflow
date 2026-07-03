package graph

import (
	"os"
	"testing"

	_ "ragflow/internal/harness/graph/pregel" // injects full Pregel engine
)

// TestMain provides package-level test filtering.
func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
