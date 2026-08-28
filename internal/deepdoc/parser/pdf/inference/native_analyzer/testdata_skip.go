// This file is always compiled (no build constraint) and provides the package
// TestMain that guards the whole test binary behind the presence of the
// DeepDoc testdata directory. The native_analyzer tests read their fixtures
// from the sibling internal/deepdoc/native/testdata directory (via a relative
// path), so that is what we check here. When the external testdata has not been
// fetched (e.g. -tags fetch_testdata was not supplied, or the download failed),
// the tests are skipped rather than failed — keeping the default `go test ./...`
// and `go test -tags cgo` runs green without the assets.
package infnative

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataDir resolves the native package's testdata directory, which the
// native_analyzer tests reach via a relative path from this package directory
// (the consuming tests use filepath.Join("..","..","..","..","native","testdata",...),
// i.e. four ".." hops from internal/deepdoc/parser/pdf/inference/native_analyzer).
func testdataDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("..", "..", "..", "..", "native", "testdata")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "native", "testdata")
}

// TestMain skips the entire package when the testdata directory is missing.
func TestMain(m *testing.M) {
	if fi, err := os.Stat(testdataDir()); err != nil || !fi.IsDir() {
		fmt.Println("SKIP: DeepDoc native testdata not available; run with -tags fetch_testdata to fetch it")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
