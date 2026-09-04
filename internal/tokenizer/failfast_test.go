//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package tokenizer

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitCL100KEncoder_FailFast pins the contract that InitCL100KEncoder fails
// fast on a missing cl100k_base table, and — the part the regression actually
// guards — that a PRESENT table yields a working encoder (NumTokensFromString > 0)
// rather than a silent 0.
//
// Background: NumTokensFromString swallows the loader error and returns 0, so a
// Go image that forgot to bake cl100k_base.tiktoken silently zeroed every token
// count while content_ltks (a separate C++ tokenizer) kept working. The fix is
// to fail fast at startup via InitCL100KEncoder; this test must exercise BOTH
// branches, otherwise it only re-asserts "missing → error" and leaves the
// "present → counts" behaviour — the thing that regressed — uncovered.
//
// Both branches reset the encoder cache and scope bpeSearchRoots to a temp dir,
// so neither depends on the host (no leaked tiktoken cache in an ancestor, no
// success cached by a sibling test, no order dependence). "absent" runs first so
// that the table "present" later copies under /tmp is not discoverable by it via
// searchRoots() walking up to the shared /tmp ancestor.
func TestInitCL100KEncoder_FailFast(t *testing.T) {
	// fail-fast: an empty scoped dir must surface a hard error, not a silent 0.
	t.Run("absent", func(t *testing.T) {
		resetCL100KEncoderForTest()
		dir := t.TempDir()
		t.Setenv("TIKTOKEN_CACHE_DIR", "")
		t.Setenv("DATA_GYM_CACHE_DIR", "")
		SetBpeSearchRootsForTest([]string{dir})
		t.Cleanup(func() { SetBpeSearchRootsForTest(nil) })

		err := InitCL100KEncoder()
		if err == nil {
			t.Fatalf("InitCL100KEncoder returned nil with no table present; expected a fail-fast error")
		}
		if !strings.Contains(err.Error(), "cl100k") {
			t.Fatalf("InitCL100KEncoder error does not mention cl100k: %v", err)
		}
	})

	// present: copy a real table into an isolated dir (via TIKTOKEN_CACHE_DIR so
	// the loader finds it without walking ancestors) and require that startup
	// succeeds AND the encoder actually counts tokens (not just "loads").
	t.Run("present", func(t *testing.T) {
		src := findRealBpeTable(t)
		if src == "" {
			t.Skip("no cl100k_base.tiktoken on disk to exercise the present branch")
		}
		resetCL100KEncoderForTest()
		dir := t.TempDir()
		t.Setenv("TIKTOKEN_CACHE_DIR", dir)
		t.Setenv("DATA_GYM_CACHE_DIR", "")
		dst := filepath.Join(dir, cacheFileName(testBpeURL))
		copyFile(t, src, dst)
		SetBpeSearchRootsForTest([]string{dir})
		t.Cleanup(func() { SetBpeSearchRootsForTest(nil) })

		if err := InitCL100KEncoder(); err != nil {
			t.Fatalf("InitCL100KEncoder with table present: unexpected error: %v", err)
		}
		if got := NumTokensFromString("hello world"); got <= 0 {
			t.Fatalf("InitCL100KEncoder succeeded but NumTokensFromString(%q) = %d, want > 0 (silent-zero regression)", "hello world", got)
		}
	})
}

// findRealBpeTable returns a path to a cl100k_base.tiktoken on disk, or "" if
// none is available. The loader's integrity gate already knows this table's
// digest (expectedBpeHashes), so copying it into a scoped dir is enough.
func findRealBpeTable(t *testing.T) string {
	t.Helper()
	// 1. Explicit cache dirs (tiktoken-go / data-gym layouts).
	for _, env := range []string{"TIKTOKEN_CACHE_DIR", "DATA_GYM_CACHE_DIR"} {
		if dir := strings.TrimSpace(os.Getenv(env)); dir != "" {
			for _, name := range []string{cacheFileName(testBpeURL), "cl100k_base.tiktoken"} {
				if p := filepath.Join(dir, name); fileExists(p) {
					return p
				}
			}
		}
	}
	// 2. repo-local ragflow_deps/cl100k_base.tiktoken (provisioned by download
	//    scripts / CI), walking up from the package dir.
	pkgDir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := pkgDir; ; {
		p := filepath.Join(dir, "ragflow_deps", "cl100k_base.tiktoken")
		if fileExists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}
