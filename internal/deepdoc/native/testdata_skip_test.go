// This file is always compiled (no build constraint) and provides the package
// TestMain that guards the whole test binary behind the presence of the
// package-level testdata directory. When the external testdata has not been
// fetched (e.g. -tags fetch_testdata was not supplied), the tests are skipped
// rather than failed — keeping the default `go test ./...` and `go test -tags cgo`
// runs green without the assets. A fetch ATTEMPTED but FAILED (under
// -tags fetch_testdata) is fatal: a missing fixture must never be a green run.
package native

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// packageDir returns this package's directory (the one holding testdata).
func packageDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(file)
}

// dirHasFiles reports whether dir exists and holds at least one entry.
func dirHasFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// resolveTestdataDir decides which directory the fixtures should be read from.
//
// Tests reach fixtures through the relative path "testdata/...", so the process
// must either hold them inline or run from a directory that does. CI runners
// pre-seed the assets at RAGFLOW_TESTDATA_DIR and mount the workspace
// read-only, so linking the fixtures into the package dir — what the fetch
// script used to do — fails there even though the data is present and readable.
// Running from the pre-seeded directory instead keeps the workspace untouched.
//
// It returns "" when the package's own testdata should be used as-is.
func resolveTestdataDir(pkgDir string) string {
	if pkgDir == "" || dirHasFiles(filepath.Join(pkgDir, "testdata")) {
		return ""
	}
	root := os.Getenv("RAGFLOW_TESTDATA_DIR")
	if root == "" {
		return ""
	}
	preseeded := filepath.Join(root, "deepdoc", filepath.Base(pkgDir))
	if dirHasFiles(filepath.Join(preseeded, "testdata")) {
		return preseeded
	}
	return ""
}

// TestMain skips the entire package unless the testdata was fetched via the
// fetch_testdata build tag. In the default build (and any build without that
// tag) it stays false, so the tests are skipped rather than failing on absent
// assets — which also makes a stale/partial leftover testdata directory on disk
// harmless: we skip regardless of what (if anything) is present.
func TestMain(m *testing.M) {
	if testdataFetchFailed {
		fmt.Fprintln(os.Stderr, "FAIL: DeepDoc native testdata fetch failed (see the fetch_deepdoc_testdata error above). CI must not pass with missing fixtures; run with -tags fetch_testdata and fix the download.")
		os.Exit(1)
	}
	if !testdataFetchAttempted {
		fmt.Println("SKIP: DeepDoc native testdata not fetched; run with -tags fetch_testdata to fetch and run it")
		os.Exit(0)
	}
	// The fetch only guarantees the fixtures exist somewhere; with a pre-seeded
	// runner they live outside the package, so run from their directory.
	if dir := resolveTestdataDir(packageDir()); dir != "" {
		if err := os.Chdir(dir); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: cannot chdir to pre-seeded testdata %s: %v\n", dir, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
