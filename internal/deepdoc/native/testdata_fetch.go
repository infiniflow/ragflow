//go:build fetch_testdata

// This file is compiled only with -tags fetch_testdata. Its init() downloads
// and symlinks the external DeepDoc testdata for this package (pkg = "native")
// so the consuming tests can run. A fetch failure is fatal under this build tag:
// it sets testdataFetchFailed so TestMain fails the package instead of silently
// skipping — a missing fixture must never be a green CI run.
//
// FetchTestdata is exported so the native_analyzer package — whose tests read
// native's testdata directory via a relative path — can reuse the same logic
// instead of duplicating it.
package native

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// fetchScriptOverride is a test-only seam; empty means use the real script
// location. It lets tests exercise the error and happy paths deterministically.
var fetchScriptOverride string

// scriptPath returns the on-disk location of fetch_deepdoc_testdata.sh relative
// to the repository root.
func scriptPath(root string) string {
	return filepath.Join(root, "tools", "scripts", "fetch_deepdoc_testdata.sh")
}

// resolveScript returns the script path to invoke, honoring the test override.
func resolveScript(root string) string {
	if fetchScriptOverride != "" {
		return fetchScriptOverride
	}
	return scriptPath(root)
}

// FetchTestdata resolves the repository root (via go.mod) and invokes
// tools/scripts/fetch_deepdoc_testdata.sh with the given <pkg> (the package
// directory that owns internal/deepdoc/<pkg>/testdata). The script resolves its
// own repo root, so cmd.Dir here is advisory.
func FetchTestdata(pkg string) error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("cannot resolve caller path")
	}
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
	script := resolveScript(root)
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("fetch script not found at %s", script)
	}
	cmd := exec.Command("bash", script, pkg)
	cmd.Dir = root
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func init() {
	if err := FetchTestdata("native"); err != nil {
		fmt.Fprintf(os.Stderr, "fetch_deepdoc_testdata: not fetched (%v)\n", err)
		testdataFetchFailed = true
		return
	}
	testdataFetchAttempted = true
}
