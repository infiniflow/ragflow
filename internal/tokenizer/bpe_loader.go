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

// Offline BPE table loading for tiktoken.
//
// RAGFlow ships the cl100k_base table on disk (Dockerfile drops it into the
// working directory under its sha1 name; download_deps.py writes it to
// ragflow_deps/). tiktoken-go's stock loader instead downloads it over HTTP and
// relies on TIKTOKEN_CACHE_DIR, which the Go server never inherits, so a
// missing table degrades every token count to 0. This loader resolves the
// table from disk only: it performs no network I/O, and when nothing is found
// it reports every path it tried.

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"github.com/pkoukk/tiktoken-go"
)

func init() {
	tiktoken.SetBpeLoader(localBpeLoader{})
}

// expectedBpeHashes maps a tiktoken table URL to the SHA-1 of its canonical
// on-disk content. We only ship cl100k_base today; entries here let the loader
// reject a corrupt or tampered file instead of trusting it. Unknown URLs are
// loaded without a digest check (defense-in-depth, not a hard gate).
//
// NOTE: this is the digest of the file *contents*, not the tiktoken cache
// filename. tiktoken-go names its cached file by sha1(bpeURL)
// (223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7 for
// cl100k_base); that value identifies the path, while the value below verifies
// the bytes we actually load. Compute it from the table shipped by
// ragflow_deps/download_deps.py: `sha1sum cl100k_base.tiktoken`.
var expectedBpeHashes = map[string]string{
	"https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken": "6494e42d5aad2bbb441ea9793af9e7db335c8d9c",
}

// localBpeLoader resolves tiktoken BPE tables from the local filesystem.
type localBpeLoader struct{}

// LoadTiktokenBpe implements tiktoken.BpeLoader.
//
// bpeURL is the upstream table URL that tiktoken-go would otherwise download;
// here it serves only to derive the file names to look for.
func (localBpeLoader) LoadTiktokenBpe(bpeURL string) (map[string]int, error) {
	candidates := bpeCandidatePaths(bpeURL)
	for _, candidate := range candidates {
		contents, err := os.ReadFile(candidate)
		if err != nil {
			// Only a missing candidate is skippable; a permission or I/O
			// failure must not be masked as "not found".
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading BPE table %s: %w", candidate, err)
		}
		// Integrity check: for tables we ship, a digest mismatch means the
		// file is corrupt or tampered with. Refuse to load it rather than
		// skipping to the next candidate — a different candidate holds the
		// same (wrong) content, and masking the failure would defeat the
		// check. This mirrors the malformed-table path just below.
		if want, ok := expectedBpeHashes[bpeURL]; ok {
			if got := fmt.Sprintf("%x", sha1.Sum(contents)); got != want {
				return nil, fmt.Errorf("BPE table %s digest mismatch (got %s, want %s); refusing to load a corrupt or tampered file", candidate, got, want)
			}
		}

		ranks, err := parseBpeTable(contents)
		if err != nil {
			// A file that exists but does not parse is a corrupt download or a
			// name collision. Continuing to the next candidate would mask it.
			return nil, fmt.Errorf("BPE table %s is malformed: %w", candidate, err)
		}
		return ranks, nil
	}

	err := fmt.Errorf(
		"no local BPE table for %s; run `uv run ragflow_deps/download_deps.py` or set TIKTOKEN_CACHE_DIR to the directory holding the table; tried: %s",
		bpeURL, strings.Join(candidates, ", "))
	// Logged as well as returned: tiktoken-go propagates this to GetEncoding,
	// whose error NumTokensFromString discards to keep returning 0.
	common.Error("cl100k BPE table not found; every token count will be 0", err)
	return nil, err
}

// bpeCandidatePaths lists, in priority order, every local path that may hold
// the table for bpeURL.
//
// Explicit configuration wins, then the directories RAGFlow actually ships the
// table in. Both the working directory and the executable's directory are
// walked upwards: the server runs with the working directory set to the
// installation root, while `go test` runs from a package subdirectory.
func bpeCandidatePaths(bpeURL string) []string {
	cacheName := fmt.Sprintf("%x", sha1.Sum([]byte(bpeURL)))
	// download_deps.py stores the table under the URL's own basename.
	bundledName := path.Base(bpeURL)

	var paths []string
	seen := make(map[string]struct{})
	add := func(p string) {
		if _, dup := seen[p]; dup {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	// Honour both variables tiktoken-go itself reads, so an operator who has
	// already configured one keeps working.
	for _, env := range []string{"TIKTOKEN_CACHE_DIR", "DATA_GYM_CACHE_DIR"} {
		if dir := strings.TrimSpace(os.Getenv(env)); dir != "" {
			add(filepath.Join(dir, cacheName))
		}
	}

	for _, root := range bpeSearchRoots() {
		// Same layout the Dockerfile creates: the table sits in the
		// installation root under its sha1 name.
		add(filepath.Join(root, cacheName))
		// download_deps.py writes the table into ragflow_deps/ under its
		// download name; a developer checkout that has run it but never
		// started the Python side only has this copy.
		add(filepath.Join(root, "ragflow_deps", bundledName))
	}

	return paths
}

// searchRoots returns the working directory and the executable's directory
// together with all of their ancestors.
func searchRoots() []string {
	var roots []string
	seen := make(map[string]struct{})
	for _, start := range startingDirs() {
		for dir := start; ; {
			if _, dup := seen[dir]; !dup {
				seen[dir] = struct{}{}
				roots = append(roots, dir)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return roots
}

// bpeSearchRoots is the directory set the loader walks for the table. It
// defaults to searchRoots (working + executable dirs and their ancestors) so
// the production image finds the table dropped into the install root. Tests
// override it with SetBpeSearchRootsForTest to isolate from the host
// filesystem — without that, a tiktoken cache leaked into /tmp (an ancestor of
// any temp working dir) would be picked up and make "missing table" tests
// non-deterministic.
var bpeSearchRoots = searchRoots

// SetBpeSearchRootsForTest replaces the directory set the loader searches.
// Pass nil to restore the default. Test-only; it mutates package state.
func SetBpeSearchRootsForTest(roots []string) {
	if roots == nil {
		bpeSearchRoots = searchRoots
		return
	}
	bpeSearchRoots = func() []string { return roots }
}

func startingDirs() []string {
	var dirs []string
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		dirs = append(dirs, filepath.Dir(exe))
	}
	return dirs
}

// parseBpeTable decodes tiktoken's on-disk format: one
// "<base64 token> <rank>" pair per line.
func parseBpeTable(contents []byte) (map[string]int, error) {
	ranks := make(map[string]int)
	for i, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		token, rank, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("line %d: expected \"<token> <rank>\"", i+1)
		}
		decoded, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		value, err := strconv.Atoi(rank)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		ranks[string(decoded)] = value
	}
	if len(ranks) == 0 {
		return nil, fmt.Errorf("table is empty")
	}
	return ranks, nil
}
