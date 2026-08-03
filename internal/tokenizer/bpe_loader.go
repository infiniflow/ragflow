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
// tiktoken-go's stock loader fetches the encoding table over HTTP and caches it
// under TIKTOKEN_CACHE_DIR, falling back to os.TempDir()/data-gym-cache. Neither
// works for RAGFlow:
//
//   - The variable is exported only inside the Python process, by
//     common/token_utils.py. docker/entrypoint.sh starts bin/ragflow_server from
//     a shell, so the Go process never inherits it.
//   - The Dockerfile does ship the table — it lands in the working directory
//     under its sha1 name — but nothing tells the Go side to look there.
//   - Reaching openaipublic.blob.core.windows.net at runtime is not an option
//     for air-gapped installs, and is unreliable in regions where that host is
//     blocked.
//
// The consequence was silent rather than loud. NumTokensFromString returns 0
// when the encoder fails to build, and the failure is memoised by a sync.Once,
// so a single miss at startup makes every token count zero for the lifetime of
// the process. Chunk merging then never crosses its token budget and an entire
// document collapses into one chunk.
//
// Python does not have this failure mode: its encoder is built at import time,
// so a missing table aborts startup instead of degrading silently.
//
// This loader closes the gap by resolving the table from disk only. It never
// performs I/O over the network, and when nothing is found it reports every
// path it tried.

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
			continue
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

	for _, root := range searchRoots() {
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
