//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildPythonWrapper_ContainsMainAndArgs(t *testing.T) {
	t.Parallel()
	wrapped := BuildPythonWrapper("def main(x): return x * 2", `{"x": 21}`)
	if !strings.Contains(wrapped, "def main(x): return x * 2") {
		t.Errorf("wrapper missing user code; got: %s", wrapped)
	}
	// argsJSON is base64-encoded in the wrapper, so the call site
	// must use json.loads(base64.b64decode(...)).
	if !strings.Contains(wrapped, "main(**json.loads(base64.b64decode(") {
		t.Errorf("wrapper missing main(**json.loads(base64.b64decode(...))) call; got: %s", wrapped)
	}
	if !strings.Contains(wrapped, resultMarkerPrefix) {
		t.Errorf("wrapper missing result marker; got: %s", wrapped)
	}
	// The marker line carries a base64-encoded JSON payload. We do
	// not decode it here — the round-trip is exercised in
	// TestExtractStructuredResult_RoundTrip.
}

func TestBuildJavaScriptWrapper_ContainsMainAndArgs(t *testing.T) {
	t.Parallel()
	wrapped := BuildJavaScriptWrapper("async function main(args) { return args.x * 2 }", `{"x": 21}`)
	if !strings.Contains(wrapped, "async function main(args)") {
		t.Errorf("wrapper missing user code; got: %s", wrapped)
	}
	if !strings.Contains(wrapped, "const __ragflowArgsB64 = ") {
		t.Errorf("wrapper missing base64 args literal; got: %s", wrapped)
	}
	if !strings.Contains(wrapped, "JSON.parse(Buffer.from(__ragflowArgsB64") {
		t.Errorf("wrapper missing args decoding; got: %s", wrapped)
	}
	if !strings.Contains(wrapped, resultMarkerPrefix) {
		t.Errorf("wrapper missing result marker; got: %s", wrapped)
	}
}

func TestExtractStructuredResult_RoundTrip(t *testing.T) {
	t.Parallel()

	// Simulate what the wrapper prints: stdout with a marker
	// carrying a base64 JSON payload, then any trailing output.
	payload := map[string]any{"present": true, "value": 42, "type": "json"}
	raw, _ := json.Marshal(payload)
	markerLine := resultMarkerPrefix + base64.StdEncoding.EncodeToString(raw)
	stdout := "Hello, world!\n" + markerLine + "\nTrailing line\n"

	cleaned, structured := ExtractStructuredResult(stdout)
	if structured == nil {
		t.Fatalf("structured result is nil; cleaned=%q", cleaned)
	}
	if v, ok := structured["value"].(float64); !ok || v != 42 {
		t.Errorf("structured[value] = %v, want 42", structured["value"])
	}
	if strings.Contains(cleaned, resultMarkerPrefix) {
		t.Errorf("cleaned stdout still contains marker: %q", cleaned)
	}
	if !strings.Contains(cleaned, "Hello, world!") || !strings.Contains(cleaned, "Trailing line") {
		t.Errorf("cleaned stdout lost user lines: %q", cleaned)
	}
}

func TestExtractStructuredResult_NoMarker(t *testing.T) {
	t.Parallel()
	stdout := "Plain output\nNothing fancy"
	cleaned, structured := ExtractStructuredResult(stdout)
	if cleaned != stdout {
		t.Errorf("cleaned = %q, want %q", cleaned, stdout)
	}
	if len(structured) != 0 {
		t.Errorf("structured = %v, want empty map", structured)
	}
}

func TestExtractStructuredResult_Empty(t *testing.T) {
	t.Parallel()
	cleaned, structured := ExtractStructuredResult("")
	if cleaned != "" {
		t.Errorf("cleaned = %q, want empty", cleaned)
	}
	if structured == nil {
		t.Errorf("structured is nil, want empty map (not nil)")
	}
	if len(structured) != 0 {
		t.Errorf("structured = %v, want empty", structured)
	}
}

func TestExtractStructuredResult_UndecodableMarker(t *testing.T) {
	t.Parallel()
	stdout := "good line\n" + resultMarkerPrefix + "!!!not-base64!!!\nmore output"
	cleaned, structured := ExtractStructuredResult(stdout)
	// Undecodable marker lines are kept in cleaned stdout (Python
	// behavior: except branch appends the line back). The user can
	// see the raw base64 and debug.
	if !strings.Contains(cleaned, "!!!not-base64!!!") {
		t.Errorf("cleaned dropped the undecodable marker line: %q", cleaned)
	}
	if len(structured) != 0 {
		t.Errorf("structured = %v, want empty", structured)
	}
}

func TestExtractStructuredResult_MultipleMarkers_LastWins(t *testing.T) {
	t.Parallel()
	// Python implementation overwrites structured_result on each
	// marker line, so the last marker wins.
	payload1 := map[string]any{"present": true, "value": "first", "type": "json"}
	payload2 := map[string]any{"present": true, "value": "second", "type": "json"}
	raw1, _ := json.Marshal(payload1)
	raw2, _ := json.Marshal(payload2)
	stdout := resultMarkerPrefix + base64.StdEncoding.EncodeToString(raw1) + "\n" +
		resultMarkerPrefix + base64.StdEncoding.EncodeToString(raw2) + "\n"
	_, structured := ExtractStructuredResult(stdout)
	if v, _ := structured["value"].(string); v != "second" {
		t.Errorf("structured[value] = %v, want 'second'", structured["value"])
	}
}

func TestExtractStructuredResult_PreservesTrailingNewline(t *testing.T) {
	t.Parallel()
	payload := map[string]any{"present": true, "value": 1, "type": "json"}
	raw, _ := json.Marshal(payload)
	stdout := "line\n" + resultMarkerPrefix + base64.StdEncoding.EncodeToString(raw) + "\n"
	cleaned, _ := ExtractStructuredResult(stdout)
	if !strings.HasSuffix(cleaned, "\n") {
		t.Errorf("cleaned lost trailing newline: %q", cleaned)
	}
}

func TestArgsToJSON(t *testing.T) {
	t.Parallel()

	t.Run("nil map", func(t *testing.T) {
		s, err := argsToJSON(nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if s != "{}" {
			t.Errorf("got %q, want %q", s, "{}")
		}
	})

	t.Run("empty map", func(t *testing.T) {
		s, err := argsToJSON(map[string]any{})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if s != "{}" {
			t.Errorf("got %q, want %q", s, "{}")
		}
	})

	t.Run("populated map", func(t *testing.T) {
		s, err := argsToJSON(map[string]any{"x": 1, "y": "hi"})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		// The exact JSON key order is non-deterministic; check both
		// fields independently.
		var got map[string]any
		if err := json.Unmarshal([]byte(s), &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got["x"].(float64) != 1 {
			t.Errorf("x = %v, want 1", got["x"])
		}
		if got["y"].(string) != "hi" {
			t.Errorf("y = %v, want 'hi'", got["y"])
		}
	})
}

func TestNormalizeLanguage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"python", "python"},
		{"python3", "python"},
		{"Python", "python"},
		{"javascript", "nodejs"},
		{"js", "nodejs"},
		{"nodejs", "nodejs"},
		{"node", "nodejs"},
		{"", ""},
		{"ruby", ""},
	}
	for _, tc := range cases {
		if got := normalizeLanguage(tc.in); got != tc.want {
			t.Errorf("normalizeLanguage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateTimeout(t *testing.T) {
	t.Parallel()

	if _, err := validateTimeout(0); err == nil {
		t.Errorf("validateTimeout(0) returned no error, want one")
	}
	if _, err := validateTimeout(-1); err == nil {
		t.Errorf("validateTimeout(-1) returned no error, want one")
	}
	if _, err := validateTimeout(700); err == nil {
		t.Errorf("validateTimeout(700) returned no error, want one (max is 600)")
	}
	if got, err := validateTimeout(30); err != nil || got != 30 {
		t.Errorf("validateTimeout(30) = %d, %v; want 30, nil", got, err)
	}
	if got, err := validateTimeout(600); err != nil || got != 600 {
		t.Errorf("validateTimeout(600) = %d, %v; want 600, nil", got, err)
	}
}
