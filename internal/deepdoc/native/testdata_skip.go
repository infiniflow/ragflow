// This file is always compiled (no build constraint) and provides the package
// TestMain that guards the whole test binary behind the presence of the
// package-level testdata directory. When the external testdata has not been
// fetched (e.g. -tags fetch_testdata was not supplied, or the download failed),
// the tests are skipped rather than failed — keeping the default `go test ./...`
// and `go test -tags cgo` runs green without the assets.
package native

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir resolves the package-level testdata directory. It is resolved via
// the source file location rather than the working directory so it works whether
// the data is present inline (pre-migration) or via the symlink set up by the
// fetch script.
func testdataDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "testdata"
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata")
}

// TestMain skips the entire package when the testdata directory is missing.
func TestMain(m *testing.M) {
	if fi, err := os.Stat(testdataDir()); err != nil || !fi.IsDir() {
		fmt.Println("SKIP: DeepDoc native testdata not available; run with -tags fetch_testdata to fetch it")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
