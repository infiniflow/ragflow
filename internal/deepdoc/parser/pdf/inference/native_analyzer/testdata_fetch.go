//go:build fetch_testdata

// This file is compiled only with -tags fetch_testdata. Its init() downloads
// and symlinks the external DeepDoc testdata. The native_analyzer tests read
// their fixtures from the sibling internal/deepdoc/native/testdata directory
// (via a relative path), so we fetch with pkg = "native" by reusing native's
// FetchTestdata. A fetch failure is fatal under this build tag: it sets
// testdataFetchFailed so TestMain fails the package instead of silently
// skipping — a missing fixture must never be a green CI run.
package infnative

import (
	"fmt"
	"os"

	"ragflow/internal/deepdoc/native"
)

func init() {
	if err := native.FetchTestdata("native"); err != nil {
		fmt.Fprintf(os.Stderr, "fetch_deepdoc_testdata: not fetched (%v)\n", err)
		testdataFetchFailed = true
		return
	}
	testdataFetchAttempted = true
}
