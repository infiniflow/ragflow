package ingestion

import (
	"strings"
	"testing"
)

func TestValidateRequiresKvrocks(t *testing.T) {
	err := (&Deps{}).Validate()
	if err == nil || !strings.Contains(err.Error(), "kvrocks") {
		t.Fatalf("Validate error = %v, want missing kvrocks", err)
	}
}
