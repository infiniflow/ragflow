//go:build fetch_testdata

// This package's TestMain skips everything unless the fixtures were fetched, so
// these tests only run under the same tag — which is how CI executes them.
package native

import (
	"os"
	"path/filepath"
	"testing"
)

// packageDirName must match the real package dir name: resolveTestdataDir
// derives the pre-seeded subtree from basename(pkgDir).
const packageDirName = "native"

// newPackageDir creates an empty <tmp>/native directory to stand in for the
// package directory.
func newPackageDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), packageDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	return dir
}

// seedTestdata creates <dir>/testdata with one file so it counts as populated.
func seedTestdata(t *testing.T, dir string) {
	t.Helper()
	target := filepath.Join(dir, "testdata")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", target, err)
	}
	if err := os.WriteFile(filepath.Join(target, "fixture.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// TestResolveTestdataDir_KeepsLocalCopy verifies that an inline testdata
// directory always wins: it is what a developer fetched locally, and it must
// keep working unchanged.
func TestResolveTestdataDir_KeepsLocalCopy(t *testing.T) {
	local := newPackageDir(t)
	seedTestdata(t, local)
	// A pre-seeded tree is configured too, but the local copy takes priority.
	t.Setenv("RAGFLOW_TESTDATA_DIR", t.TempDir())

	if got := resolveTestdataDir(local); got != "" {
		t.Fatalf("want empty (stay in the package dir), got %q", got)
	}
}

// TestResolveTestdataDir_FallsBackToPreseeded covers the read-only workspace
// case: the fixtures cannot be linked into the package dir, and the pre-seeded
// copy must be used from where it is instead of failing the package.
func TestResolveTestdataDir_FallsBackToPreseeded(t *testing.T) {
	local := newPackageDir(t) // no inline testdata
	root := t.TempDir()
	seedTestdata(t, filepath.Join(root, "deepdoc", packageDirName))
	t.Setenv("RAGFLOW_TESTDATA_DIR", root)

	want := filepath.Join(root, "deepdoc", packageDirName)
	if got := resolveTestdataDir(local); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestResolveTestdataDir_EmptyWhenNothingAvailable makes sure we do not chdir
// into a directory that has no fixtures: that would turn a clear "missing
// testdata" failure into a confusing file-not-found one.
func TestResolveTestdataDir_EmptyWhenNothingAvailable(t *testing.T) {
	local := newPackageDir(t)
	t.Setenv("RAGFLOW_TESTDATA_DIR", t.TempDir()) // pre-seeded tree is empty

	if got := resolveTestdataDir(local); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}
