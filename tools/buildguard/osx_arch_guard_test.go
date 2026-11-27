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

package buildguard

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// cmakeOSXArchForGo extracts the Darwin arch-normalization block from
// build.sh and runs it with a fake `go env GOARCH`, returning the value that
// the script would pass to -DCMAKE_OSX_ARCHITECTURES (empty means no flag).
// Testing the script's own lines keeps the guard in sync with the build.
func cmakeOSXArchForGo(t *testing.T, goarch string) string {
	t.Helper()
	src := readFile(t, filepath.Join(repoRoot(t), "build.sh"))
	re := regexp.MustCompile(`(?s)(    if \[ "\$\(uname -s\)" = "Darwin" \]; then\n        local goarch\n.*?\n    fi)`)
	block := re.FindStringSubmatch(src)
	if block == nil {
		t.Fatal("cannot find the Darwin arch block in build.sh")
	}
	archBlock := block[1]
	// Run the extracted block inside a function (it uses `local`) with fake
	// `go` and `uname`. Use a positive array index: macOS ships bash 3.2,
	// which has no negative-index syntax.
	script := `
go() { printf '%s\n' "$FAKE_GOARCH"; }
uname() { printf 'Darwin\n'; }
probe() {
    local cmake_args=(-DCMAKE_BUILD_TYPE=Release)
` + archBlock + `
    printf '%s\n' "${cmake_args[1]-}"
}
probe
`
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(cmd.Environ(), "FAKE_GOARCH="+goarch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Darwin arch block (GOARCH=%q) failed: %v (%s)", goarch, err, out)
	}
	line := strings.TrimSpace(string(out))
	const prefix = "-DCMAKE_OSX_ARCHITECTURES="
	if line == "" {
		return ""
	}
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("unexpected cmake arg %q for GOARCH=%q", line, goarch)
	}
	return strings.TrimPrefix(line, prefix)
}

// TestCMakeOSXArchMatchesLipoNames guards the Intel-Mac path: CMake's
// CMAKE_OSX_ARCHITECTURES (and lipo -archs, which internal/binding/cpp's
// PCRE2 check compares against) names the architecture x86_64, not Go's
// amd64. Passing amd64 through makes `bash build.sh --all` fail to configure.
func TestCMakeOSXArchMatchesLipoNames(t *testing.T) {
	cases := map[string]string{
		"amd64": "x86_64",
		"arm64": "arm64",
		"":      "",
	}
	for goarch, want := range cases {
		if got := cmakeOSXArchForGo(t, goarch); got != want {
			t.Errorf("GOARCH=%q -> CMAKE_OSX_ARCHITECTURES=%q, want %q", goarch, got, want)
		}
	}
}
