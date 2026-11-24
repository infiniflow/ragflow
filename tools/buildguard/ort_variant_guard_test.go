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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildShORTVersion returns the pinned ORT version from the single source of
// truth (internal/common/environments.go: DeepDocORTVersion). Tests must read
// this instead of hardcoding a version, so a version bump cannot create a
// false-green fixture that no longer matches the production archive naming.
func buildShORTVersion(t *testing.T) string {
	t.Helper()
	src := readFile(t, filepath.Join(repoRoot(t), "internal", "common", "environments.go"))
	re := regexp.MustCompile(`const\s+DeepDocORTVersion\s*=\s*"([^"]+)"`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("cannot parse DeepDocORTVersion from internal/common/environments.go")
	}
	return m[1]
}

// ortVariantScriptPath is the helper that maps DEEPDOC_ORT_VARIANT to an ORT
// archive-directory glob suffix. build.sh sources it; the tests below source
// it directly so the variant-selection behaviour is falsifiable in isolation.
func ortVariantScriptPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), "tools", "scripts", "ort_variant.sh")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("tools/scripts/ort_variant.sh must exist: %v", err)
	}
	return p
}

// runVariantSuffix sources the helper with the given DEEPDOC_ORT_VARIANT
// (empty means unset) and returns ort_variant_suffix's stdout.
func runVariantSuffix(t *testing.T, variant string) string {
	t.Helper()
	// Helper path is $2 (quoted) so checkouts with spaces work.
	script := "unset DEEPDOC_ORT_VARIANT; [ -n \"$1\" ] && export DEEPDOC_ORT_VARIANT=\"$1\"; source \"$2\" && ort_variant_suffix"
	cmd := exec.Command("bash", "-c", script, "bash", variant, ortVariantScriptPath(t))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ort_variant_suffix (variant=%q) failed: %v (%s)", variant, err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestOrtVariantSuffixDefaultKeepsLegacyGlob guards the additive/OSS-safe
// requirement: with no variant set (or variant=cpu), the suffix must be the
// legacy wildcard so the existing "match any version/platform dir" behaviour
// is byte-for-byte preserved.
func TestOrtVariantSuffixDefaultKeepsLegacyGlob(t *testing.T) {
	for _, variant := range []string{"", "cpu"} {
		if got := runVariantSuffix(t, variant); got != "*" {
			t.Errorf("ort_variant_suffix (variant=%q) = %q, want '*' (legacy glob)", variant, got)
		}
	}
}

// TestOrtVariantSuffixGpuSelectsFull is the distinguishing-condition guard:
// gpu/full must narrow the suffix to the full archive dir, NOT the wildcard.
// A wildcard implementation would still pass the default test but fail here.
func TestOrtVariantSuffixGpuSelectsFull(t *testing.T) {
	for _, variant := range []string{"gpu", "full"} {
		got := runVariantSuffix(t, variant)
		if got != "-full" {
			t.Errorf("ort_variant_suffix (variant=%q) = %q, want '-full' (wildcard would re-introduce minimal/full ambiguity)", variant, got)
		}
	}
}

// makeFakeOrtCache lays down one suffixed ORT archive under a version dir and
// returns the cache root. Callers add a second suffix (via addOrtArchive) to
// build the adversarial "minimal + full coexist" fixture.
func makeFakeOrtCache(t *testing.T, suffix string) string {
	t.Helper()
	tmp := t.TempDir()
	addOrtArchive(t, tmp, suffix)
	return tmp
}

// addOrtArchive adds another suffixed archive dir into an existing cache.
func addOrtArchive(t *testing.T, cache, suffix string) string {
	t.Helper()
	libDir := filepath.Join(cache, "onnxruntime-linux-x64-static_lib-"+buildShORTVersion(t)+suffix, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "libonnxruntime.a"), []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	return libDir
}

// runBuildShOrtSelection sources the REAL build.sh check_onnxruntime_deps
// against a fake cache and returns its ORT_A_FILES result. This exercises the
// production case-pattern (not a test-reconstructed glob), so deleting the
// variant suffix from build.sh makes the test fail — a real adversarial guard.
func runBuildShOrtSelection(t *testing.T, cache, variant string) (string, error) {
	t.Helper()
	script := `
set -e
source "$1" >/dev/null 2>&1
export ONNXRUNTIME_STATIC_PREFIX="$2"
# Empty variant string means "leave unset" (default/OSS path); a non-empty
# value is exported explicitly. ${VAR:-} detection in ort_variant.sh treats
# truly-unset and explicitly-empty differently, so unset must really unset.
if [ -n "$3" ]; then export DEEPDOC_ORT_VARIANT="$3"; else unset DEEPDOC_ORT_VARIANT; fi
check_onnxruntime_deps >/dev/null 2>&1 || { echo "CHECK_FAILED"; exit 2; }
printf '%s' "$ORT_A_FILES"
`
	cmd := exec.Command("bash", "-c", script, "bash",
		filepath.Join(repoRoot(t), "build.sh"), cache, variant)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestOrtVariantGlobExcludesMinimal calls the REAL build.sh function with both
// a minimal and full archive present. A gpu variant must select ONLY the full
// archive — the exact bug that made GPU builds link minimal nondeterministically.
// Deleting ${ort_variant_suffix_value} from build.sh's case pattern makes this
// fail (both archives would be collected).
func TestOrtVariantGlobExcludesMinimal(t *testing.T) {
	cache := makeFakeOrtCache(t, "-glibc2_28") // minimal present
	addOrtArchive(t, cache, "-full")           // full also present

	out, err := runBuildShOrtSelection(t, cache, "gpu")
	if err != nil {
		t.Fatalf("build.sh check_onnxruntime_deps (gpu) failed: %v (%s)", err, out)
	}
	if strings.Contains(out, "CHECK_FAILED") || strings.TrimSpace(out) == "" {
		t.Fatalf("build.sh selected no ORT archive for gpu variant: %s", out)
	}
	if strings.Contains(out, "-glibc2_28") {
		t.Errorf("build.sh gpu variant selected the minimal archive (ORT_A_FILES=%s); must pick -full only", out)
	}
	if !strings.Contains(out, "-full") {
		t.Errorf("build.sh gpu variant did not select the full archive (ORT_A_FILES=%s)", out)
	}
	if len(strings.Fields(strings.TrimSpace(out))) != 1 {
		t.Errorf("build.sh gpu variant selected %d archives, want exactly 1: %s", len(strings.Fields(strings.TrimSpace(out))), out)
	}
}

// TestOrtVariantMinimalOnlyFailsFast is the adversarial missing-full case: with
// variant=gpu and only a minimal archive present, the real build.sh function
// must FAIL (no full archive) rather than silently selecting the minimal one.
func TestOrtVariantMinimalOnlyFailsFast(t *testing.T) {
	cache := makeFakeOrtCache(t, "-glibc2_28") // ONLY minimal, no full
	out, err := runBuildShOrtSelection(t, cache, "gpu")
	if err == nil && !strings.Contains(out, "CHECK_FAILED") {
		t.Fatalf("build.sh gpu variant succeeded with only a minimal archive (ORT_A_FILES=%s); must fail-fast", out)
	}
}

// TestOrtVariantDefaultStillSelectsArchive guards backward compatibility at
// the production-function level: with no variant (OSS/CPU), a cache holding a
// minimal archive must still select it (byte-for-byte legacy behaviour).
func TestOrtVariantDefaultStillSelectsArchive(t *testing.T) {
	cache := makeFakeOrtCache(t, "-glibc2_28")
	out, err := runBuildShOrtSelection(t, cache, "")
	if err != nil {
		t.Fatalf("build.sh default-variant selection failed: %v (%s)", err, out)
	}
	if !strings.Contains(out, "-glibc2_28") {
		t.Errorf("build.sh default variant did not select the minimal archive (ORT_A_FILES=%s)", out)
	}
}

// TestOrtPresentVariantAware is the fail-fast guard for the "presence gate"
// helper: with variant=-full and only a minimal archive present, the cache must
// be reported MISSING (so the downloader/fail-fast kicks in) rather than
// falsely treated as present. With the full archive present it must be found.
func TestOrtPresentVariantAware(t *testing.T) {
	present := filepath.Join(repoRoot(t), "tools", "scripts", "ort_present.sh")
	if _, err := os.Stat(present); err != nil {
		t.Fatalf("ort_present.sh must exist: %v", err)
	}

	minimalOnly := makeFakeOrtCache(t, "-glibc2_28")
	prefix := "onnxruntime-linux-x64-static_lib-"
	version := buildShORTVersion(t)
	// ragflow_ort_present <cache> <dir_prefix> <version> [variant_suffix]
	// The helper path is $1 (quoted) so checkouts with spaces work.
	// Assert it exits exactly 1 (missing), not a source/bash error (other codes).
	cmd := exec.Command("bash", "-c",
		"source \"$1\" && ragflow_ort_present \"$2\" \"$3\" \"$4\" \"$5\"",
		"bash", present, minimalOnly, prefix, version, "-full")
	if err := cmd.Run(); err == nil {
		t.Error("ragflow_ort_present reported full variant PRESENT when only minimal archive exists; should exit 1 (missing)")
	} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() != 1 {
		t.Errorf("ragflow_ort_present exited %d, want exactly 1 (missing)", ee.ExitCode())
	}

	addOrtArchive(t, minimalOnly, "-full")
	cmd = exec.Command("bash", "-c",
		"source \"$1\" && ragflow_ort_present \"$2\" \"$3\" \"$4\" \"$5\"",
		"bash", present, minimalOnly, prefix, version, "-full")
	if err := cmd.Run(); err != nil {
		t.Errorf("ragflow_ort_present reported full variant MISSING after full archive added: %v", err)
	}
}

// TestOrtPresentBackwardCompatible guards the coupled-field edge case: when
// the 4th variant_suffix arg is omitted, ragflow_ort_present must keep the old
// 3-arg behaviour and find the wildcard archive (OSS/CPU builds call it this way).
func TestOrtPresentBackwardCompatible(t *testing.T) {
	present := filepath.Join(repoRoot(t), "tools", "scripts", "ort_present.sh")
	cache := makeFakeOrtCache(t, "-glibc2_28")
	prefix := "onnxruntime-linux-x64-static_lib-"
	cmd := exec.Command("bash", "-c",
		"source \"$1\" && ragflow_ort_present \"$2\" \"$3\" \"$4\"",
		"bash", present, cache, prefix, buildShORTVersion(t))
	if err := cmd.Run(); err != nil {
		t.Errorf("3-arg ragflow_ort_present broke backward compatibility: %v", err)
	}
}

// TestOrtVariantSuffixRejectsUnknown is the adversarial configuration guard:
// a misspelled/unknown variant must NOT silently fall back to the wildcard,
// which would reintroduce the exact minimal/full selection bug this change
// fixes. It must fail loudly so a bad config surfaces at build time.
func TestOrtVariantSuffixRejectsUnknown(t *testing.T) {
	script := "unset DEEPDOC_ORT_VARIANT; export DEEPDOC_ORT_VARIANT=\"$1\"; source \"$2\" && ort_variant_suffix"
	cmd := exec.Command("bash", "-c", script, "bash", "gpou", ortVariantScriptPath(t)) // typo of "gpu"
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("ort_variant_suffix accepted unknown variant 'gpou' and returned %q; want non-zero exit", strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "unknown") {
		t.Errorf("unknown-variant error should say 'unknown', got: %s", string(out))
	}
}

// TestOrtVariantEmptyValueIsRejected guards the explicit-empty edge case: an
// explicitly set but empty DEEPDOC_ORT_VARIANT is a misconfiguration and must
// be rejected, not silently treated as "cpu". ${VAR:-cpu} cannot distinguish
// unset from set-to-empty, so the helper must use ${VAR+set} detection.
func TestOrtVariantEmptyValueIsRejected(t *testing.T) {
	script := "export DEEPDOC_ORT_VARIANT=\"\"; source \"$1\" && ort_variant_suffix"
	cmd := exec.Command("bash", "-c", script, "bash", ortVariantScriptPath(t))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("ort_variant_suffix accepted an explicitly empty variant and returned %q; want rejection", strings.TrimSpace(string(out)))
	}
}

// TestInvalidVariantDoesNotTriggerDownload guards the "wasted download" edge:
// an invalid variant makes _native_libs_present return 1, which the download
// gate can mistake for "libs missing" and run the full downloader before
// eventually failing. The variant config must be validated BEFORE the gate so
// a misconfiguration fails immediately without a pointless network download.
func TestInvalidVariantDoesNotTriggerDownload(t *testing.T) {
	script := `
source "$1" >/dev/null 2>&1 || exit 90
export DEEPDOC_ORT_VARIANT="gpou"
_ensure_native_libs_installed
`
	cmd := exec.Command("bash", "-c", script, "bash",
		filepath.Join(repoRoot(t), "build.sh"))
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "running downloader") {
		t.Errorf("invalid variant triggered a full native-lib download before failing: %s", out)
	}
}

// TestFullVariantMissingHasActionableHint (Finding A) is the adversarial guard:
// a gpu/full build with only a minimal archive must produce an actionable hint
// naming the -full archive and that it must be pre-staged manually — because the
// bundled downloader only produces minimal. A generic "still missing" message
// is not enough. The hint is emitted by a dedicated function called from BOTH
// the early presence gate and the post-download failure branch.
func TestFullVariantMissingHasActionableHint(t *testing.T) {
	script := `
set +e
source "$1" >/dev/null 2>&1 || exit 90
export DEEPDOC_ORT_VARIANT=gpu
if declare -F ort_full_archive_hint >/dev/null 2>&1; then
  ort_full_archive_hint 2>&1
else
  echo "NO_HINT_FUNCTION"
fi
`
	cmd := exec.Command("bash", "-c", script, "bash", filepath.Join(repoRoot(t), "build.sh"))
	out, _ := cmd.CombinedOutput()
	msg := strings.ToLower(string(out))
	if strings.Contains(msg, "no_hint_function") {
		t.Fatal("build.sh has no ort_full_archive_hint helper; gpu/full missing-full has no actionable diagnostic")
	}
	if !strings.Contains(msg, "-full") || !strings.Contains(msg, "manually") {
		t.Errorf("ort_full_archive_hint lacks actionable pre-stage wording (want '-full' + 'manually'): %s", out)
	}
}

// TestFullVariantHintNotCalledForCpu guards the "no false hint" edge: the
// actionable full-archive hint must NOT fire for a cpu/default build (which
// legitimately uses the minimal archive). A hint that always printed would be
// noise and could mislead CPU users.
func TestFullVariantHintNotCalledForCpu(t *testing.T) {
	script := `
set +e
source "$1" >/dev/null 2>&1 || exit 90
export DEEPDOC_ORT_VARIANT=cpu
if declare -F ort_full_archive_hint >/dev/null 2>&1; then
  ort_full_archive_hint 2>&1
fi
`
	cmd := exec.Command("bash", "-c", script, "bash", filepath.Join(repoRoot(t), "build.sh"))
	out, _ := cmd.CombinedOutput()
	// Guard against a vacuous pass: if the helper is absent, the conditional
	// never runs and the test would be green while proving nothing.
	probe := exec.Command("bash", "-c",
		"source \"$1\" >/dev/null 2>&1 && declare -F ort_full_archive_hint >/dev/null 2>&1",
		"bash", filepath.Join(repoRoot(t), "build.sh"))
	if probe.Run() != nil {
		t.Fatal("ort_full_archive_hint helper missing; cpu-silence test would be vacuous")
	}
	if strings.Contains(strings.ToLower(string(out)), "-full") {
		t.Errorf("full-archive hint fired for cpu variant; should be silent: %s", out)
	}
}

// TestBuildShPropagatesVariantFailure guards round-2 finding: bash command
// substitution failures are swallowed in `local x="$(f)"` and `cmd "$(f)"`
// contexts. If build.sh calls ort_variant_suffix without checking its exit
// status, an unknown variant's non-zero exit is lost, the suffix becomes empty,
// and the glob silently falls back to matching every dir — reintroducing the
// very ambiguity the fail-fast is meant to prevent. Both call sites must
// propagate the failure (`... || return 1`).
func TestBuildShPropagatesVariantFailure(t *testing.T) {
	script := readFile(t, filepath.Join(repoRoot(t), "build.sh"))
	// Every `$(ort_variant_suffix)` call must be immediately guarded. Count
	// unguarded occurrences: the command substitution not followed by `||`.
	for _, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "$(ort_variant_suffix)") {
			continue
		}
		if !strings.Contains(line, "||") {
			t.Errorf("build.sh call site does not propagate ort_variant_suffix failure (missing `||`): %s", strings.TrimSpace(line))
		}
	}
}
