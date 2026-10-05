//go:build !integration
// +build !integration

package service

import "testing"

// TestMaskAPIToken_NeverLeaksFullToken pins the masking format for the
// GET /api/v1/system/api_keys fix (#20555). Mirrors the cycle-96
// TestMaskAPIKey_* contract so every masked credential in this service
// uses the same shape.
func TestMaskAPIToken_NeverLeaksFullToken(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "single char is too short, never echoed", in: "x", want: "***"},
		{name: "seven chars is too short, never echoed", in: "abcdefg", want: "***"},
		{name: "eight chars exposes only prefix+suffix", in: "abcdefgh", want: "abcd***efgh"},
		{name: "ragflow token keeps the ragf prefix and tail", in: "ragflow-abcdefghijklmnopqrstuvwxyz123456", want: "ragf***3456"},
		{name: "long realistic token keeps start and end", in: "ragflow-abc123XYZ-def456GHI-jkl789MNO", want: "ragf***LMNO"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := maskAPIToken(c.in)
			if got != c.want {
				t.Fatalf("maskAPIToken(%q) = %q, want %q", c.in, got, c.want)
			}
			if c.in != "" && got == c.in {
				t.Fatalf("maskAPIToken(%q) returned the input verbatim", c.in)
			}
			if c.in != "" && len(c.in) > 7 && contains(got, c.in) {
				t.Fatalf("maskAPIToken(%q) = %q: masked value still contains the input", c.in, got)
			}
		})
	}
}

// contains is a tiny helper that avoids importing strings just for one
// substring check. strings.Contains is exported elsewhere; we keep the test
// self-contained.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestMaskAPIToken_DoesNotEchoBodyOfLongToken guards against a future naive
// implementation (raw return, strings.Repeat, or helper removal at the call
// site). The masked form must preserve prefix/suffix and must not equal /
// / be contained in the input.
func TestMaskAPIToken_DoesNotEchoBodyOfLongToken(t *testing.T) {
	in := "ragflow-aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ABCDefgh"
	got := maskAPIToken(in)
	if got == in {
		t.Fatalf("maskAPIToken returned the input verbatim: %q", got)
	}
	if contains(got, in) {
		t.Fatalf("maskAPIToken output %q contains the input %q", got, in)
	}
	if !startsWith(got, in[:4]) || !endsWith(got, in[len(in)-4:]) {
		t.Fatalf("maskAPIToken output %q lost the prefix or suffix of %q", got, in)
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
