//go:build fetch_testdata

// This file is compiled only with -tags fetch_testdata. Its init() downloads
// and symlinks the external DeepDoc testdata for this package (pkg = "native")
// so the consuming tests can run. A fetch failure is fatal under this build tag:
// it sets testdataFetchFailed so TestMain fails the package instead of silently
// skipping — a missing fixture must never be a green CI run.
package native

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func init() {
	if err := fetchTestdata(); err != nil {
		fmt.Fprintf(os.Stderr, "fetch_deepdoc_testdata: not fetched (%v)\n", err)
		testdataFetchFailed = true
		return
	}
	testdataFetchAttempted = true
}

// fetchTestdata resolves the repo root, then invokes scripts/fetch_deepdoc_testdata.sh
// with this package's directory name as <pkg>.
func fetchTestdata() error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot resolve caller path")
	}
	// thisFile = <root>/internal/deepdoc/native/testdata_fetch.go
	dir := filepath.Dir(thisFile)
	root := dir
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return fmt.Errorf("repo root (go.mod) not found from %s", dir)
		}
		root = parent
	}
	pkg := filepath.Base(dir)
	script := filepath.Join(root, "scripts", "fetch_deepdoc_testdata.sh")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("fetch script not found at %s", script)
	}
	cmd := exec.Command("bash", script, pkg)
	cmd.Dir = root
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
