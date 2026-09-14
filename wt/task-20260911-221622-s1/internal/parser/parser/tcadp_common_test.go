package parser

import (
	"os"
	"testing"
)

// TestParseWithTCADPReadsLegacyTCADPAPISERVERURLEnvVar guards against
// the silent breaking change where the shared parseWithTCADP core
// dropped the legacy TCADP_APISERVER_URL env var that the old
// parsePDFWithTCADP wrapper honoured. Existing PDF deployments set
// this env var; we must keep reading it for backward compat.
func TestParseWithTCADPReadsLegacyTCADPAPISERVERURLEnvVar(t *testing.T) {
	t.Setenv("TCADP_APISERVER_URL", "https://legacy.tcadp.example.com")
	t.Setenv("TCADP_APISERVER", "")

	if got := tcadpAPIBaseURL(""); got != "https://legacy.tcadp.example.com" {
		t.Fatalf("expected tcadpAPIBaseURL to honor TCADP_APISERVER_URL fallback, got %q", got)
	}
}

// TestParseWithTCADPPrefersExplicitArgumentOverEnvVar pins the order:
// an explicit tcadpAPIServer argument wins over both env vars.
func TestParseWithTCADPPrefersExplicitArgumentOverEnvVar(t *testing.T) {
	t.Setenv("TCADP_APISERVER", "https://env.tcadp.example.com")
	t.Setenv("TCADP_APISERVER_URL", "https://legacy.tcadp.example.com")

	if got := tcadpAPIBaseURL("https://explicit.tcadp.example.com"); got != "https://explicit.tcadp.example.com" {
		t.Fatalf("explicit arg must win over env vars, got %q", got)
	}
}

// TestParseWithTCADPFallsBackToCanonicalTCADPAPISERVEREnvVar verifies
// the canonical env var works when the legacy one is unset.
func TestParseWithTCADPFallsBackToCanonicalTCADPAPISERVEREnvVar(t *testing.T) {
	t.Setenv("TCADP_APISERVER", "https://canonical.tcadp.example.com")
	t.Setenv("TCADP_APISERVER_URL", "")

	if got := tcadpAPIBaseURL(""); got != "https://canonical.tcadp.example.com" {
		t.Fatalf("expected tcadpAPIBaseURL to honor TCADP_APISERVER fallback, got %q", got)
	}
}

// TestParseWithTCADPReturnsEmptyWhenNoEnvVarOrArgSet guards against a
// regression where the fallback ordering ever leaks the literal string
// "" into the parse flow (which would surface as a malformed-URL error
// later instead of the helpful "configure tcadp_apiserver" error).
func TestParseWithTCADPReturnsEmptyWhenNoEnvVarOrArgSet(t *testing.T) {
	os.Unsetenv("TCADP_APISERVER_URL")
	os.Unsetenv("TCADP_APISERVER")

	if got := tcadpAPIBaseURL(""); got != "" {
		t.Fatalf("expected empty result when no env var and no arg, got %q", got)
	}
}
