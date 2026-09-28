package infnative

// testdataFetchAttempted is set to true by testdata_fetch.go (compiled only with
// -tags fetch_testdata) once the external DeepDoc testdata has been fetched and
// linked into the sibling internal/deepdoc/native/testdata directory. It is
// declared in this always-compiled (non-_test.go) file so it is also defined when
// this package is built as a dependency of another package's test binary. In the
// default build it stays false, so TestMain skips the package.
var testdataFetchAttempted bool

// testdataFetchFailed is set to true by testdata_fetch.go (compiled only with
// -tags fetch_testdata) when the external testdata download fails. Declared here
// (always-compiled) so TestMain can read it. Under the fetch_testdata build tag a
// failed fetch must FAIL the package rather than silently skip — a missing
// fixture must never produce a green CI run. In the default build this stays
// false, so the absence of fixtures is a benign skip.
var testdataFetchFailed bool
