//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
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
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testBpeURL = "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"

// writeBpeTable writes a minimal well-formed BPE table. The rank values are
// arbitrary markers so a test can tell which file the loader actually read.
func writeBpeTable(t *testing.T, path string, marker int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	line := fmt.Sprintf("%s %d\n", base64.StdEncoding.EncodeToString([]byte("hello")), marker)
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	// Register this synthetic table's digest so the SHA-1 integrity gate in
	// bpe_loader.go accepts it. This also exercises the verification path
	// instead of disabling it. Restore the prior value afterwards so the
	// mutation does not leak into sibling tests (which would otherwise read a
	// stale digest for the same URL).
	prev := expectedBpeHashes[testBpeURL]
	expectedBpeHashes[testBpeURL] = fmt.Sprintf("%x", sha1.Sum([]byte(line)))
	t.Cleanup(func() { expectedBpeHashes[testBpeURL] = prev })
}

func cacheFileName(url string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(url)))
}

// isolate moves the process into an empty directory and clears both cache
// environment variables, so a test sees only the files it creates itself.
// Without this the real repository checkout — which does ship the table — would
// satisfy every lookup and hide ordering bugs. It also scopes the loader's
// search roots to this directory so a tiktoken cache leaked into an ancestor
// (e.g. /tmp) can't be picked up and make "missing table" tests flaky.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", "")
	t.Setenv("DATA_GYM_CACHE_DIR", "")
	t.Chdir(dir)
	SetBpeSearchRootsForTest([]string{dir})
	t.Cleanup(func() { SetBpeSearchRootsForTest(nil) })
	return dir
}

func TestLocalBpeLoader_ReadsTiktokenCacheDir(t *testing.T) {
	isolate(t)
	cache := t.TempDir()
	writeBpeTable(t, filepath.Join(cache, cacheFileName(testBpeURL)), 7)
	t.Setenv("TIKTOKEN_CACHE_DIR", cache)

	ranks, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ranks["hello"]; got != 7 {
		t.Errorf("rank from TIKTOKEN_CACHE_DIR = %d, want 7", got)
	}
}

func TestLocalBpeLoader_ReadsDataGymCacheDir(t *testing.T) {
	isolate(t)
	cache := t.TempDir()
	writeBpeTable(t, filepath.Join(cache, cacheFileName(testBpeURL)), 8)
	t.Setenv("DATA_GYM_CACHE_DIR", cache)

	ranks, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ranks["hello"]; got != 8 {
		t.Errorf("rank from DATA_GYM_CACHE_DIR = %d, want 8", got)
	}
}

// The production image has no TIKTOKEN_CACHE_DIR set: Dockerfile drops the
// table straight into the working directory under its sha1 name, and
// entrypoint.sh starts the Go binary from a shell, so nothing exports the
// variable that common/token_utils.py sets inside the Python process.
func TestLocalBpeLoader_ReadsSha1FileFromWorkingDirectory(t *testing.T) {
	dir := isolate(t)
	writeBpeTable(t, filepath.Join(dir, cacheFileName(testBpeURL)), 9)

	ranks, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ranks["hello"]; got != 9 {
		t.Errorf("rank from working directory = %d, want 9", got)
	}
}

// A developer checkout that has run download_deps.py but never started the
// Python side only has the file under its download name.
func TestLocalBpeLoader_ReadsBundledVocabFromAncestor(t *testing.T) {
	dir := isolate(t)
	writeBpeTable(t, filepath.Join(dir, "ragflow_deps", "cl100k_base.tiktoken"), 10)
	nested := filepath.Join(dir, "internal", "tokenizer")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Chdir(nested)

	ranks, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ranks["hello"]; got != 10 {
		t.Errorf("rank from bundled vocab = %d, want 10", got)
	}
}

func TestLocalBpeLoader_CacheDirWinsOverBundledVocab(t *testing.T) {
	dir := isolate(t)
	writeBpeTable(t, filepath.Join(dir, "ragflow_deps", "cl100k_base.tiktoken"), 10)
	cache := t.TempDir()
	writeBpeTable(t, filepath.Join(cache, cacheFileName(testBpeURL)), 7)
	t.Setenv("TIKTOKEN_CACHE_DIR", cache)

	ranks, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := ranks["hello"]; got != 7 {
		t.Errorf("rank = %d, want 7 (an explicit cache dir must win)", got)
	}
}

// The whole point of this loader is that a missing table is an error rather
// than an HTTP request. Reporting every path tried is what turns an opaque
// "all token counts are zero" deployment into a one-look diagnosis.
func TestLocalBpeLoader_MissingTableReportsCandidatesInsteadOfDownloading(t *testing.T) {
	isolate(t)

	_, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err == nil {
		t.Fatal("expected an error when no local table exists, got nil")
	}
	for _, want := range []string{cacheFileName(testBpeURL), "cl100k_base.tiktoken", "TIKTOKEN_CACHE_DIR"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q, so it cannot be acted on:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "http") && !strings.Contains(err.Error(), testBpeURL) {
		t.Errorf("error hints at a network attempt: %v", err)
	}
}

func TestLocalBpeLoader_RejectsMalformedTable(t *testing.T) {
	dir := isolate(t)
	path := filepath.Join(dir, cacheFileName(testBpeURL))
	if err := os.WriteFile(path, []byte("not base64 at all\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err == nil {
		t.Fatal("expected an error for a malformed table, got nil")
	}
}

// A candidate that exists but cannot be read as a file must surface as a read
// error rather than being skipped as "missing". LoadTiktokenBpe only continues
// past os.IsNotExist; a directory at the candidate path fails ReadFile with a
// distinct error, which this test pins to the read-error path.
func TestLocalBpeLoader_ReadErrorIsReported(t *testing.T) {
	dir := isolate(t)
	// A directory at the sha1-named candidate path exists but is not a
	// regular file, so os.ReadFile fails with a non-IsNotExist error.
	candidate := filepath.Join(dir, cacheFileName(testBpeURL))
	if err := os.Mkdir(candidate, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := localBpeLoader{}.LoadTiktokenBpe(testBpeURL)
	if err == nil {
		t.Fatal("expected a read error for a non-file candidate, got nil")
	}
	if strings.Contains(err.Error(), "no local BPE table") {
		t.Errorf("read error was masked as not-found: %v", err)
	}
}
