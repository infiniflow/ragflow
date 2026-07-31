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

// TestAuthHeaderForDownload covers the host-scoping rule for the TCADP
// result-URL bearer token. The token should only travel to a URL whose
// host matches the configured API server's host.
func TestAuthHeaderForDownload(t *testing.T) {
	cases := []struct {
		name       string
		apiBase    string
		download   string
		key        string
		wantHeader string // "" means no Authorization header should be set
	}{
		{
			name:       "matching host sends bearer",
			apiBase:    "https://api.tcadp.example.com",
			download:   "https://api.tcadp.example.com/results/abc.zip",
			key:        "tcadp-secret",
			wantHeader: "Bearer tcadp-secret",
		},
		{
			name:       "matching host with trailing slash on base still matches",
			apiBase:    "https://api.tcadp.example.com/",
			download:   "https://api.tcadp.example.com/results/abc.zip",
			key:        "tcadp-secret",
			wantHeader: "Bearer tcadp-secret",
		},
		{
			name:       "matching host with port",
			apiBase:    "https://api.tcadp.example.com:8443",
			download:   "https://api.tcadp.example.com:8443/results/abc.zip",
			key:        "tcadp-secret",
			wantHeader: "Bearer tcadp-secret",
		},
		{
			name:       "different host — presigned S3 URL — no auth",
			apiBase:    "https://api.tcadp.example.com",
			download:   "https://bucket.s3.amazonaws.com/results/abc.zip?X-Amz-Signature=...",
			key:        "tcadp-secret",
			wantHeader: "",
		},
		{
			name:       "different host — CDN URL — no auth",
			apiBase:    "https://api.tcadp.example.com",
			download:   "https://cdn.example.com/results/abc.zip",
			key:        "tcadp-secret",
			wantHeader: "",
		},
		{
			name:       "subdomain mismatch — no auth",
			apiBase:    "https://api.tcadp.example.com",
			download:   "https://malicious.tcadp.example.com/results/abc.zip",
			key:        "tcadp-secret",
			wantHeader: "",
		},
		{
			name:       "unparseable download URL — safe default, no auth",
			apiBase:    "https://api.tcadp.example.com",
			download:   "ht!tp://not a url",
			key:        "tcadp-secret",
			wantHeader: "",
		},
		{
			name:       "unparseable API URL — safe default, no auth",
			apiBase:    "ht!tp://not a url",
			download:   "https://api.tcadp.example.com/results/abc.zip",
			key:        "tcadp-secret",
			wantHeader: "",
		},
		{
			name:       "empty API key — bearer returns empty, no header",
			apiBase:    "https://api.tcadp.example.com",
			download:   "https://api.tcadp.example.com/results/abc.zip",
			key:        "",
			wantHeader: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authHeaderForDownload(tc.apiBase, tc.download, tc.key)
			if got != tc.wantHeader {
				t.Errorf("authHeaderForDownload(%q, %q, %q) = %q, want %q",
					tc.apiBase, tc.download, tc.key, got, tc.wantHeader)
			}
		})
	}
}
