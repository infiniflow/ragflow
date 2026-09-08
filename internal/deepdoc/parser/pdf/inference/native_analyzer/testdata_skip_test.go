// This file is always compiled (no build constraint) and provides the package
// TestMain that guards the whole test binary behind the presence of the
// DeepDoc testdata directory. The native_analyzer tests read their fixtures
// from the sibling internal/deepdoc/native/testdata directory (via a relative
// path), so that is what we check here. When the external testdata has not been
// fetched (e.g. -tags fetch_testdata was not supplied), the tests are skipped
// rather than failed — keeping the default `go test ./...` and `go test -tags cgo`
// runs green without the assets. A fetch ATTEMPTED but FAILED (under
// -tags fetch_testdata) is fatal: a missing fixture must never be a green run.
package infnative

import (
	"fmt"
	"os"
	"testing"
)

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
	os.Exit(m.Run())
}
