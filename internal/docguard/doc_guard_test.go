// Package docguard holds regression guards for repository documentation and
// build glue that the native-lib versioned-cache change (PR #22099) must keep
// correct. These are plain unit tests (no build tag) so they run in the default
// `bash build.sh --test` tier and in CI.
//
// The guards exist because the IDE/`go build` CGO snippet in
// internal/development.md is the one piece of build glue that is NOT executed by
// CI: a broken snippet only fails for the developer who copy-pastes it. These
// tests make the P1 review findings (review 5821944375) falsifiable — an
// un-expanded `v*` include glob, a glob+`head -1` version resolution that pins
// the oldest cache version, and stale `/opt` pre-bake prose — fail a test rather
// than relying on prose review alone.
package docguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this source file to the directory containing go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate repo root (go.mod)")
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestDevelopmentDocResolvesNativeLibVersions guards the P1 finding from review
// 5821944375: the IDE/`go build` CGO snippet must resolve each native lib from
// its single source of truth (the version-pinned resolvers) instead of a glob +
// `head -1`, and the include path must be derived from the resolved archive
// rather than a literal `v*` glob that the shell never expands inside quotes.
func TestDevelopmentDocResolvesNativeLibVersions(t *testing.T) {
	doc := readFile(t, filepath.Join(repoRoot(t), "internal", "development.md"))

	// The include path must no longer be a literal `v*` glob inside the
	// double-quoted CGO_CFLAGS assignment — that glob is never expanded by the
	// shell and breaks the first cgo compile.
	if strings.Contains(doc, "office_oxide/v*/include/office_oxide_c") {
		t.Error("development.md still has the un-expanded `v*` include glob in CGO_CFLAGS; derive the include from the resolved archive instead")
	}

	// The `.a` archives must no longer be resolved by `ls …/v*/… | head -1`,
	// which silently pins the lexicographically-oldest version in a cache that
	// holds several versions at once.
	if strings.Contains(doc, "v*/lib/liboffice_oxide.a | head -1") {
		t.Error("development.md still resolves office_oxide via `ls …/v*… | head -1`; use the version-pinned resolver")
	}
	if strings.Contains(doc, "v*/lib/libpdfium.a | head -1") {
		t.Error("development.md still resolves pdfium via `ls …/v*… | head -1`; use the version-pinned resolver")
	}

	// Positive guard: the snippet must resolve versions from the single source
	// of truth and derive the include from the resolved archive variable.
	if !strings.Contains(doc, "native_lib_version.sh") {
		t.Error("development.md should source tools/scripts/native_lib_version.sh to resolve versions from the single source of truth")
	}
	if !strings.Contains(doc, "OFFICE_OXIDE_A") {
		t.Error("development.md should resolve the office_oxide archive into OFFICE_OXIDE_A and derive CGO_CFLAGS from it")
	}
}

// normalizeDoc strips markdown blockquote prefixes ('>') and collapses
// newlines so multi-line blockquote sentences can be matched as a single
// string. The native-lib notes in development.md are long blockquotes that wrap
// across lines, so a raw substring search misses the stale phrases.
func normalizeDoc(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, ">", " ")
	return strings.Join(strings.Fields(s), " ")
}

// TestDevelopmentDocNoStaleOptBake guards against the superseded /opt pre-bake
// description that the PR deleted from build.sh but that the doc still asserts.
func TestDevelopmentDocNoStaleOptBake(t *testing.T) {
	doc := normalizeDoc(readFile(t, filepath.Join(repoRoot(t), "internal", "development.md")))
	if strings.Contains(doc, "seeds ORT from") {
		t.Error("development.md still claims CI seeds ORT from /opt; the /opt pre-bake was removed")
	}
	if strings.Contains(doc, "bake is authoritative") {
		t.Error("development.md still references the /opt pre-bake as authoritative; the pre-bake was removed")
	}
}

// TestBuildShNoStaleOptBake guards the stale comment inside
// check_onnxruntime_deps that contradicts the new "There is no /opt pre-bake"
// comment a few lines below it.
func TestBuildShNoStaleOptBake(t *testing.T) {
	script := readFile(t, filepath.Join(repoRoot(t), "build.sh"))
	if strings.Contains(script, "seeds from /opt") {
		t.Error("build.sh still claims CI seeds ORT from /opt inside check_onnxruntime_deps; the /opt pre-bake was removed")
	}
	if strings.Contains(script, "bake is authoritative") {
		t.Error("build.sh still references the /opt pre-bake as authoritative; the /opt pre-bake was removed")
	}
}

// ortVersionNote extracts the "pinned in a few places" note block from
// development.md so the ORT pin-list guards can assert on that note alone.
func ortVersionNote(t *testing.T) string {
	t.Helper()
	doc := readFile(t, filepath.Join(repoRoot(t), "internal", "development.md"))
	start := strings.Index(doc, "ONNX Runtime native version is pinned")
	if start < 0 {
		t.Fatal("development.md has no ORT version-pinning note")
	}
	endMarker := "build.sh` runs this consistency check"
	end := strings.Index(doc[start:], endMarker)
	if end < 0 {
		t.Fatal("development.md ORT version note has no recognizable end")
	}
	return normalizeDoc(doc[start : start+end])
}

// TestDevelopmentDocOrtPinsAreNotMisleading guards P1 sub-finding 3a: the note
// used to list tools/scripts/download_dev_deps.py's ORT_VERSION as a THIRD pin
// to edit when bumping ORT. That constant is derived at runtime from
// DeepDocORTVersion (environments.go) — check_ort_version_consistency compares
// only environments.go and the Dockerfile — so telling developers to edit the
// downloader sends them to the wrong place. The note must (a) not present the
// downloader as a pin bullet, and (b) still list the two real pins.
func TestDevelopmentDocOrtPinsAreNotMisleading(t *testing.T) {
	note := ortVersionNote(t)
	// (a) No bullet that presents the downloader's ORT_VERSION as a pin to edit.
	if strings.Contains(note, "- `tools/scripts/download_dev_deps.py`") {
		t.Error("development.md still lists download_dev_deps.py ORT_VERSION as a pin to edit; it is derived from DeepDocORTVersion, not a pin")
	}
	// (a) The downloader derives its version from the Go const — no second copy.
	if !strings.Contains(note, "derive") || !strings.Contains(note, "DeepDocORTVersion") {
		t.Error("development.md should explain that build.sh and download_dev_deps.py derive ORT_VERSION from DeepDocORTVersion, so there is no third pin to edit")
	}
	// (b) The two real pin locations must remain documented.
	if !strings.Contains(note, "internal/common/environments.go") || !strings.Contains(note, "Dockerfile") {
		t.Error("development.md must still list the two real ORT pins (environments.go:DeepDocORTVersion and Dockerfile:ARG ORT_VERSION)")
	}
}

// TestNativeLibVersionPinnedNotGlob is the adversarial behavioural guard for the
// config-pinning requirement: with two versions present in the cache, the
// resolver must return the version pinned in go.mod (pinned != latest), whereas
// the naive `ls …/v*… | head -1` would silently pick the lexicographically
// oldest. This is exactly the failure mode the doc snippet used to have.
func TestNativeLibVersionPinnedNotGlob(t *testing.T) {
	root := repoRoot(t)
	versionSh := filepath.Join(root, "tools", "scripts", "native_lib_version.sh")
	if _, err := os.Stat(versionSh); err != nil {
		t.Fatalf("native_lib_version.sh missing: %v", err)
	}

	// Build a fake repo root with go.mod pinning office_oxide v0.1.16 and a
	// cache that also carries an OLDER v0.1.12 — the adversarial case where a
	// glob+head -1 would wrongly select the stale version.
	tmp := t.TempDir()
	goMod := "module example.com/ragflow\n\nrequire github.com/yfedoseev/office_oxide/go v0.1.16\n"
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(tmp, "ragflow-native-libs", "office_oxide")
	for _, v := range []string{"v0.1.12", "v0.1.16"} {
		dir := filepath.Join(cache, v, "lib")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "liboffice_oxide.a"), []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Resolve the pinned version through the shared resolver.
	out, err := exec.Command("bash", "-c",
		sourceCmd(versionSh)+" && go_module_version \"github.com/yfedoseev/office_oxide/go\" "+tmp,
	).Output()
	if err != nil {
		t.Fatalf("go_module_version failed: %v (%s)", err, out)
	}
	got := strings.TrimSpace(string(out))
	if got != "0.1.16" {
		t.Fatalf("go_module_version returned %q, want pinned 0.1.16", got)
	}

	// Demonstrate the bug the fix prevents: the naive glob+head -1 picks the
	// lexicographically-oldest version, not the pinned one.
	naive, err := exec.Command("bash", "-c",
		"ls "+filepath.Join(cache, "v*", "lib", "liboffice_oxide.a")+" | head -1",
	).Output()
	if err != nil {
		t.Fatalf("naive glob failed: %v (%s)", err, naive)
	}
	if !strings.Contains(string(naive), "v0.1.12") {
		t.Fatalf("naive glob+head -1 did not select the oldest version; got %q", naive)
	}
}

// TestDocumentedCgoCflagsResolvesPinnedInclude is the end-to-end adversarial
// guard for the blocking P1 finding: it extracts the ACTUAL `export CGO_CFLAGS=`
// line from development.md, executes it against a fake cache holding two
// versions, and asserts the resulting include path (a) resolves to a real header
// under the pinned v0.1.16, not the oldest v0.1.12, and (b) is not a literal
// un-expanded `v*` glob. This catches both the "glob never expands in quotes"
// bug and the "include must follow the pinned version" requirement.
func TestDocumentedCgoCflagsResolvesPinnedInclude(t *testing.T) {
	root := repoRoot(t)
	doc := readFile(t, filepath.Join(root, "internal", "development.md"))

	// Extract the documented CGO_CFLAGS export line from the fenced bash block,
	// stripping the markdown blockquote prefix ("> "). Executing the real line
	// (rather than a copy) means a future edit that reintroduces a bug fails here.
	var cflagsLine string
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ">"))
		if strings.HasPrefix(trimmed, "export CGO_CFLAGS=") {
			cflagsLine = trimmed
			break
		}
	}
	if cflagsLine == "" {
		t.Fatal("development.md has no `export CGO_CFLAGS=` line to verify")
	}

	// Fake cache: two versions, each with a header. The pinned version
	// (v0.1.16) is NOT the lexicographically-oldest (v0.1.12), so a glob+head -1
	// or a literal `v*` would both be wrong.
	tmp := t.TempDir()
	cache := filepath.Join(tmp, "office_oxide")
	for _, v := range []string{"v0.1.12", "v0.1.16"} {
		inc := filepath.Join(cache, v, "include", "office_oxide_c")
		lib := filepath.Join(cache, v, "lib")
		if err := os.MkdirAll(inc, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(lib, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(inc, "office_oxide.h"), []byte("header"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(lib, "liboffice_oxide.a"), []byte("archive"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pinnedArchive := filepath.Join(cache, "v0.1.16", "lib", "liboffice_oxide.a")

	// Execute the documented line with OFFICE_OXIDE_A bound to the pinned
	// archive, then emit the resulting CGO_CFLAGS value.
	cmd := exec.Command("bash", "-c", cflagsLine+` ; printf '%s' "$CGO_CFLAGS"`)
	cmd.Env = append(os.Environ(), "OFFICE_OXIDE_A="+pinnedArchive)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("documented CGO_CFLAGS line failed to evaluate: %v (%s)", err, out)
	}
	cflags := strings.TrimSpace(string(out))
	if !strings.HasPrefix(cflags, "-I") {
		t.Fatalf("CGO_CFLAGS is not an -I include flag: %q", cflags)
	}
	includeDir := strings.TrimPrefix(cflags, "-I")

	// (a) It must be derived from the PINNED archive, not the oldest version.
	if !strings.Contains(includeDir, "v0.1.16") || strings.Contains(includeDir, "v0.1.12") {
		t.Errorf("include %q not derived from pinned v0.1.16 archive", includeDir)
	}
	// (a) It must resolve to a real header — the whole point of the fix.
	if _, err := os.Stat(filepath.Join(includeDir, "office_oxide.h")); err != nil {
		t.Errorf("derived include %q has no office_oxide.h: %v", includeDir, err)
	}
	// (b) It must NOT be an un-expanded literal `v*` glob (the original bug:
	// the shell does not expand globs inside double-quoted CGO_CFLAGS).
	if strings.Contains(includeDir, "v*/") || strings.Contains(includeDir, "v*") {
		t.Errorf("include %q still contains an un-expanded v* glob", includeDir)
	}

	// Adversarial proof of the original defect: a literal `v*` inside a quoted
	// path never matches a real file, so the old line could not find the header.
	literalGlob := filepath.Join(cache, "v*", "include", "office_oxide_c", "office_oxide.h")
	if _, err := os.Stat(literalGlob); err == nil {
		t.Errorf("a literal v* path unexpectedly resolved to a header; the un-expanded-glob guard is vacuous")
	}
}

func sourceCmd(path string) string {
	return "set -e; source " + path
}
