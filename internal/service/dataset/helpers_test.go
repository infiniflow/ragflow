package dataset

import (
	"strings"
	"testing"
)

// TestValidateParserID_AcceptsRegistryRefs verifies that every
// canonical builtin pipeline id passes validation.
func TestValidateParserID_AcceptsRegistryRefs(t *testing.T) {
	for _, id := range []string{"general", "book", "audio", "qa", "table", "tag"} {
		if err := validateParserID(id); err != nil {
			t.Errorf("validateParserID(%q) = %v, want nil", id, err)
		}
	}
}

// TestValidateParserID_AcceptsNaiveAlias verifies the legacy
// parser_id "naive" still validates (alias for general).
func TestValidateParserID_AcceptsNaiveAlias(t *testing.T) {
	if err := validateParserID("naive"); err != nil {
		t.Errorf("validateParserID(naive) = %v, want nil (alias for general)", err)
	}
}

// TestValidateParserID_RejectsUnknown verifies unknown/empty
// values are rejected with an error that lists the valid options.
func TestValidateParserID_RejectsUnknown(t *testing.T) {
	for _, id := range []string{"", "unknown", "NAIVE"} {
		err := validateParserID(id)
		if err == nil {
			t.Errorf("validateParserID(%q) = nil, want error", id)
		}
	}

	err := validateParserID("unknown")
	if err == nil {
		t.Fatal("expected error for unknown parser id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "general") {
		t.Errorf("error message %q should mention general", msg)
	}
}
