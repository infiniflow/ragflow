package component

import (
	"testing"

	"ragflow/internal/utility"
)

// withSSRFBypass enables the test-only SSRF bypass for the duration of a test
// and restores the previous value in t.Cleanup. Component tests exercise
// parsing/dispatch logic against httptest servers bound to 127.0.0.1, which
// the strict SSRF guard rejects. SSRF enforcement has its own dedicated
// coverage in internal/utility/ssrf_test.go.
func withSSRFBypass(t *testing.T) {
	t.Helper()
	prev := utility.AllowAnyHostForTest
	utility.AllowAnyHostForTest = true
	t.Cleanup(func() { utility.AllowAnyHostForTest = prev })
}
