package native

// testdataFetchAttempted is set to true by testdata_fetch.go (compiled only with
// -tags fetch_testdata) once the external DeepDoc testdata has been fetched and
// linked into ./testdata. It is declared in this always-compiled (non-_test.go)
// file — rather than in a _test.go — so it is also defined when this package is
// compiled as a dependency of another package's test binary (e.g. native_analyzer
// imports native, and _test.go files are excluded from dependency builds). In the
// default build it stays false, so TestMain skips the package.
var testdataFetchAttempted bool
