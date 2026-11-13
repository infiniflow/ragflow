package parser

import (
	"errors"
	"testing"
)

// TestParseResultProduced pins the success/failure boundary that replaced the
// old produced != "" check. A result is "produced" only when it carries a
// structured payload (JSON items or enriched File metadata) and has no error.
func TestParseResultProduced(t *testing.T) {
	someErr := errors.New("boom")

	// Error set => never produced, even when a payload is present.
	if (ParseResult{Err: someErr}).Produced() {
		t.Error("result with Err must not be Produced")
	}
	if (ParseResult{Err: someErr, JSON: []DocItem{NewTextJSONItem("x")}}).Produced() {
		t.Error("result with Err and JSON must not be Produced")
	}

	// Success with JSON items.
	if !(ParseResult{JSON: []DocItem{NewTextJSONItem("x")}}).Produced() {
		t.Error("success with JSON must be Produced")
	}

	// Success with enriched File but no JSON (e.g. emptyPDFResult / metadata-only).
	if !(ParseResult{File: map[string]any{"name": "x"}}).Produced() {
		t.Error("success with File must be Produced")
	}

	// Success with neither JSON nor File => raw-text fallback path.
	if (ParseResult{}).Produced() {
		t.Error("empty success must not be Produced (raw fallback expected)")
	}
}
