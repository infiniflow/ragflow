package common

import (
	"os"
	"strings"
	"testing"
)

// TestNoHandWrittenTruncateInJSONChat guards the removal of the unused local
// truncate helper in jsonchat.go. It was dead code (no production or test
// caller); the shared utility.TruncateRunes is the single truncation path.
func TestNoHandWrittenTruncateInJSONChat(t *testing.T) {
	src, err := os.ReadFile("jsonchat.go")
	if err != nil {
		t.Fatalf("read jsonchat.go: %v", err)
	}
	if strings.Contains(string(src), "func truncate(") {
		t.Fatal("dead hand-written truncate still present in jsonchat.go")
	}
}
