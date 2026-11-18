//go:build fetch_testdata

package native

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testRepoRoot walks up from this test file to the directory containing go.mod.
func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root (go.mod) not found")
		}
		dir = parent
	}
}

func TestScriptPath(t *testing.T) {
	if got := scriptPath("/repo"); got != "/repo/tools/scripts/fetch_deepdoc_testdata.sh" {
		t.Fatalf("scriptPath = %q, want /repo/tools/scripts/fetch_deepdoc_testdata.sh", got)
	}
}

// TestFetchScriptResolvedToToolsScripts guards the move: the fetch script must
// live at tools/scripts/ (not the old scripts/) so the repo root derivation in
// the script itself stays correct.
func TestFetchScriptResolvedToToolsScripts(t *testing.T) {
	root := testRepoRoot(t)
	got := scriptPath(root)
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("expected fetch script at %s: %v", got, err)
	}
	old := filepath.Join(root, "scripts", "fetch_deepdoc_testdata.sh")
	if _, err := os.Stat(old); err == nil {
		t.Fatalf("script still present at old location %s; it should have been moved to tools/scripts/", old)
	}
}

// TestFetchMissingScriptReturnsError covers the error path: a missing script
// must surface a descriptive error rather than a confusing exec failure.
func TestFetchMissingScriptReturnsError(t *testing.T) {
	root := testRepoRoot(t)
	fetchScriptOverride = filepath.Join(root, "does-not-exist.sh")
	defer func() { fetchScriptOverride = "" }()
	if err := FetchTestdata("native"); err == nil {
		t.Fatal("FetchTestdata returned nil error for missing script, want error")
	} else if !strings.Contains(err.Error(), "fetch script not found") {
		t.Fatalf("FetchTestdata error = %q, want it to mention 'fetch script not found'", err.Error())
	}
}

// TestFetchHappyPathWithFakeScript exercises the real exec path deterministically
// (no network clone) using a fake script that exits 0.
func TestFetchHappyPathWithFakeScript(t *testing.T) {
	tmp := t.TempDir()
	fake := filepath.Join(tmp, "fake-fetch.sh")
	if err := os.WriteFile(fake, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fetchScriptOverride = fake
	defer func() { fetchScriptOverride = "" }()
	if err := FetchTestdata("native"); err != nil {
		t.Fatalf("FetchTestdata with fake success script = %v, want nil", err)
	}
}
