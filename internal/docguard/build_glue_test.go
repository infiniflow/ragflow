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

package docguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// runHelper sources a tools/scripts helper and calls fn with args in a clean
// environment, returning stdout and whether the call succeeded.
func runHelper(t *testing.T, script, fn string, env []string, args ...string) (string, bool) {
	t.Helper()
	path := filepath.Join(repoRoot(t), "tools", "scripts", script)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s missing: %v", script, err)
	}
	cmd := exec.Command("bash", append([]string{"-c", `source "$0" && ` + fn + ` "$@"`, path}, args...)...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err == nil
}

func TestNativeLibsDirResolution(t *testing.T) {
	cases := []struct {
		env  []string
		want string
	}{
		{[]string{"HOME=/home/alice"}, "/home/alice/ragflow-native-libs"},
		{[]string{"HOME=/home/alice", "RAGFLOW_NATIVE_LIBS_DIR=/mnt/cache"}, "/mnt/cache"},
		{[]string{"HOME=/home/alice", "RAGFLOW_NATIVE_LIBS_DIR="}, "/home/alice/ragflow-native-libs"},
	}
	for _, c := range cases {
		got, ok := runHelper(t, "native_lib_version.sh", "ragflow_native_libs_dir", c.env)
		if !ok || got != c.want {
			t.Errorf("env %v: got %q (ok=%v); want %q", c.env, got, ok, c.want)
		}
	}
}

func TestDeepDocORTVersionShellHelper(t *testing.T) {
	fixture := t.TempDir()
	env := filepath.Join(fixture, "internal", "common", "environments.go")
	if err := os.MkdirAll(filepath.Dir(env), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("package common\n\nconst DeepDocORTVersion = \"9.9.9\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := runHelper(t, "native_lib_version.sh", "deepdoc_ort_version", nil, fixture); !ok || got != "9.9.9" {
		t.Errorf("fixture: got %q (ok=%v)", got, ok)
	}
	if got, ok := runHelper(t, "native_lib_version.sh", "deepdoc_ort_version", nil, repoRoot(t)); !ok || !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(got) {
		t.Errorf("repo: got %q (ok=%v)", got, ok)
	}
	if got, ok := runHelper(t, "native_lib_version.sh", "deepdoc_ort_version", nil, t.TempDir()); ok || got != "" {
		t.Errorf("missing environments.go must fail silently: got %q (ok=%v)", got, ok)
	}
}

func writeFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("!<arch>\nort"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestORTStampTarget pins ragflow_ort_stamp_target: the stamp symlink is
// relative, so it resolves under any cache mount, and an existing correct link
// needs no write, so a read-only CI mount is a no-op.
func TestORTStampTarget(t *testing.T) {
	if exec.Command("realpath", "-m", "--relative-to=/", "/").Run() != nil {
		t.Skip("ort_stamp.sh needs GNU realpath (-m --relative-to)")
	}
	root := t.TempDir()
	versioned := filepath.Join(root, "static_lib", "v1.29.0-abc123")
	archive := filepath.Join(root, "static_lib", "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28", "lib", "libonnxruntime.a")
	writeFixture(t, archive)
	if err := os.MkdirAll(versioned, 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := runHelper(t, "ort_stamp.sh", "ragflow_ort_stamp_target", nil, versioned, archive)
	needs, target, _ := strings.Cut(got, "|")
	if !ok || needs != "yes" || filepath.IsAbs(target) {
		t.Fatalf("missing link: got %q (ok=%v); want yes|<relative>", got, ok)
	}
	if resolved := filepath.Join(versioned, target); filepath.Clean(resolved) != archive {
		t.Errorf("target %q resolves to %q; want %q", target, resolved, archive)
	}

	if err := os.Symlink(target, filepath.Join(versioned, "libonnxruntime.a")); err != nil {
		t.Fatal(err)
	}
	got, ok = runHelper(t, "ort_stamp.sh", "ragflow_ort_stamp_target", nil, versioned, archive)
	if !ok || got != "no|"+target {
		t.Errorf("correct link: got %q (ok=%v); want no|%s", got, ok, target)
	}
}

// TestORTPresentIsVersionAndPlatformAware pins ragflow_ort_present, which
// decides whether build.sh runs the downloader: only a base libonnxruntime
// archive under the target platform and version directory counts.
func TestORTPresentIsVersionAndPlatformAware(t *testing.T) {
	const prefix = "onnxruntime-linux-x64-static_lib-"
	cases := []struct {
		name string
		file string // relative to the static_lib root; empty seeds nothing
		want bool
	}{
		{"matching version and platform", "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28/lib/libonnxruntime.a", true},
		{"old version", "onnxruntime-linux-x64-static_lib-1.23.2-glibc2_28/lib/libonnxruntime.a", false},
		{"other platform", "onnxruntime-linux-aarch64-static_lib-1.29.0-glibc2_28/lib/libonnxruntime.a", false},
		{"empty cache", "", false},
		{"stray top-level archive", "libonnxruntime.a", false},
		{"provider archive only", "onnxruntime-linux-x64-static_lib-1.29.0-glibc2_28/lib/libonnxruntime_providers_cuda.a", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			staticLib := filepath.Join(t.TempDir(), "static_lib")
			if err := os.MkdirAll(staticLib, 0o755); err != nil {
				t.Fatal(err)
			}
			if c.file != "" {
				writeFixture(t, filepath.Join(staticLib, filepath.FromSlash(c.file)))
			}
			if _, ok := runHelper(t, "ort_present.sh", "ragflow_ort_present", nil, staticLib, prefix, "1.29.0"); ok != c.want {
				t.Errorf("present = %v; want %v", ok, c.want)
			}
		})
	}
}

// TestSepTestsMountsNativeLibsCache guards the CI wiring: every builder
// container mounts the native static-library cache at RAGFLOW_NATIVE_LIBS_DIR
// and fails fast when the mount is missing.
func TestSepTestsMountsNativeLibsCache(t *testing.T) {
	text := readFile(t, filepath.Join(repoRoot(t), ".github", "workflows", "sep-tests.yml"))
	if n := strings.Count(text, "${RAGFLOW_NATIVE_LIBS_DIR}:${RAGFLOW_NATIVE_LIBS_DIR}"); n < 4 {
		t.Errorf("native-lib cache mounted into %d builder containers; want at least 4", n)
	}
	if !strings.Contains(text, "native-lib cache not mounted") {
		t.Error("sep-tests.yml lost the missing-mount guard")
	}
}

func arm64Section(t *testing.T) string {
	t.Helper()
	doc := readFile(t, filepath.Join(repoRoot(t), "internal", "development.md"))
	start := strings.Index(doc, "ARM64 (aarch64) build hosts")
	if start < 0 {
		t.Fatal("development.md has no ARM64 build-hosts section")
	}
	if end := strings.Index(doc[start:], "\n### "); end >= 0 {
		return doc[start : start+end]
	}
	return doc[start:]
}

// TestDevelopmentDocKeepsARM64HostRequirements pins the empirically found
// ARM64 build-host minimums and their failure signatures, including lld-18
// producing a binary that links but hangs at startup.
func TestDevelopmentDocKeepsARM64HostRequirements(t *testing.T) {
	section := arm64Section(t)
	for _, token := range []string{"glibc", "2.38", "__isoc23_strtoll", "GCC", "13", "libsimde-dev", "simde/x86/sse4.1.h", "cmake", "4.0", "lld", "20", "inittrace", "lld-18"} {
		if !strings.Contains(section, token) {
			t.Errorf("ARM64 section lost %q", token)
		}
	}
	lower := strings.ToLower(section)
	if !strings.Contains(lower, "hang") && !strings.Contains(lower, "spin") {
		t.Error("ARM64 section no longer warns that lld-18 binaries hang at startup")
	}
}

func TestDevelopmentDocUsesVersionedNativeLayout(t *testing.T) {
	doc := readFile(t, filepath.Join(repoRoot(t), "internal", "development.md"))
	for _, want := range []string{"office_oxide/v", "pdfium-static/v", "pdf_oxide/v"} {
		if !strings.Contains(doc, want) {
			t.Errorf("development.md lost the versioned %s<version> layout", want)
		}
	}
	for _, flat := range []string{"office_oxide/lib/liboffice_oxide.a", "pdfium-static/lib/libpdfium.a"} {
		if strings.Contains(doc, flat) {
			t.Errorf("development.md references the flat layout %s", flat)
		}
	}
}
