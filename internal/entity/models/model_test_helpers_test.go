package models

import (
	"strings"
	"testing"

	"ragflow/internal/utility"
)

// withSSRFBypass enables the test-only SSRF bypass for the duration of a test
// and restores the previous value in t.Cleanup. Driver tests exercise request
// and response logic against httptest servers bound to 127.0.0.1, which the
// strict SSRF guard (NewDriverHTTPClient(false)) rejects. SSRF enforcement has
// its own dedicated coverage in internal/utility/ssrf_test.go.
func withSSRFBypass(t *testing.T) {
	t.Helper()
	prev := utility.AllowAnyHostForTest
	utility.AllowAnyHostForTest = true
	t.Cleanup(func() { utility.AllowAnyHostForTest = prev })
}

func requireNoSuchMethod(t *testing.T, name string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected no such method error, got nil", name)
	}
	if !strings.Contains(err.Error(), "no such method") {
		t.Fatalf("%s: expected no such method error, got %v", name, err)
	}
}
