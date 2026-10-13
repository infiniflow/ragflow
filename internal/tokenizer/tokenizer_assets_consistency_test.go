// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tokenizer

// This test is the Go port of the deleted ragflow_deps/test_tokenizer_assets.py.
// It guards the three-way agreement that keeps the embedding token counters
// honest:
//
//  1. ragflow_deps/tokenizer_assets.txt - the single manifest the runtime
//     downloader (download_deps.py) and this test read; lists each
//     (repo, file, kind).
//  2. Dockerfile / Dockerfile_base        - copy the "runtime" assets into the
//     image at the path the counters search.
//  3. internal/tokenizer/*.go             - pin the SHA-1 of each "runtime"
//     asset so a swapped file is detected at startup.
//
// When they drift, the layer that can see it fails loudly: the runtime image
// does not build without a runtime asset, and an embedder whose model declares
// a counter that cannot load refuses to ingest rather than substituting the
// calibrated cl100k count (an under-count is what makes a provider answer 400).
// See internal/tokenizer/embedding_token_limits.md.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this source file to the directory holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test file")
		}
		dir = parent
	}
}

// tokenizerAsset is one entry of ragflow_deps/tokenizer_assets.txt.
type tokenizerAsset struct {
	repo string
	file string
	kind string // "runtime" | "oracle"
}

// loadTokenizerAssets reads the manifest. The "runtime" assets are the ones the
// counters load in production (must be shipped + pinned); the "oracle" assets are
// only needed by scripts/gen_tokenizer_oracle.py to regenerate test fixtures.
func loadTokenizerAssets(t *testing.T, root string) []tokenizerAsset {
	t.Helper()
	path := filepath.Join(root, "ragflow_deps", "tokenizer_assets.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v (run ragflow_deps/download_deps.py or recreate the manifest)", path, err)
	}
	defer f.Close()

	var assets []tokenizerAsset
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) != 3 {
			t.Fatalf("%s:%d: malformed entry %q (want 'repo file kind')", path, lineNo, raw)
		}
		repo, file, kind := fields[0], fields[1], fields[2]
		if kind != "runtime" && kind != "oracle" {
			t.Fatalf("%s:%d: unknown kind %q for %s/%s", path, lineNo, kind, repo, file)
		}
		if !strings.Contains(repo, "/") || file == "" {
			t.Fatalf("%s:%d: malformed repo/file %s/%s", path, lineNo, repo, file)
		}
		assets = append(assets, tokenizerAsset{repo: repo, file: file, kind: kind})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(assets) == 0 {
		t.Fatalf("%s is empty", path)
	}
	return assets
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// pinRegex matches a tokenizer asset SHA-1 pin in the Go loaders, e.g.
// "BAAI/bge-m3/sentencepiece.bpe.model": "7e88c49faff6c6c136fdf4a3402d0cb534c6ab10".
var pinRegex = regexp.MustCompile(`"([^"]+)":\s*"([0-9a-f]{40})"`)

func TestTokenizerAssetsAreWellFormed(t *testing.T) {
	root := repoRoot(t)
	assets := loadTokenizerAssets(t, root)
	if len(assets) == 0 {
		t.Fatal("no tokenizer assets listed")
	}
	runtimeCount := 0
	for _, a := range assets {
		if a.kind == "runtime" {
			runtimeCount++
		}
	}
	if runtimeCount == 0 {
		t.Fatal("no runtime asset: at least one counter would always be unavailable")
	}
}

func TestEveryRuntimeAssetIsCopiedIntoEveryRuntimeImage(t *testing.T) {
	root := repoRoot(t)
	assets := loadTokenizerAssets(t, root)
	runtimeImages := []string{"Dockerfile", "Dockerfile_base"}
	for _, image := range runtimeImages {
		text := readFile(t, filepath.Join(root, image))
		for _, a := range assets {
			if a.kind != "runtime" {
				continue
			}
			key := a.repo + "/" + a.file
			if !strings.Contains(text, key) {
				t.Errorf("%s does not copy %s; the matching counter would be unavailable in that image and ingest would silently fall back to the calibrated count", image, key)
			}
		}
	}
}

func TestEveryRuntimeAssetIsPinnedByTheLoader(t *testing.T) {
	root := repoRoot(t)
	assets := loadTokenizerAssets(t, root)

	loaderFiles := []string{"spm.go", "wordpiece.go", "bpe.go"}
	var loaderText strings.Builder
	for _, name := range loaderFiles {
		loaderText.WriteString(readFile(t, filepath.Join(root, "internal", "tokenizer", name)))
	}

	pins := map[string]string{}
	for _, m := range pinRegex.FindAllStringSubmatch(loaderText.String(), -1) {
		pins[m[1]] = m[2]
	}

	for _, a := range assets {
		if a.kind != "runtime" {
			continue
		}
		key := a.repo + "/" + a.file
		digest, ok := pins[key]
		if !ok {
			t.Errorf("%s has no 40-hex SHA-1 pin in internal/tokenizer/*.go", key)
			continue
		}
		if isLowEntropyHex(digest) {
			t.Errorf("%s pin %q does not look like a digest", key, digest)
		}
	}
}

func TestOracleOnlyAssetsAreNotShipped(t *testing.T) {
	root := repoRoot(t)
	assets := loadTokenizerAssets(t, root)
	runtimeImages := []string{"Dockerfile", "Dockerfile_base"}
	for _, image := range runtimeImages {
		text := readFile(t, filepath.Join(root, image))
		for _, a := range assets {
			if a.kind != "oracle" {
				continue
			}
			key := a.repo + "/" + a.file
			if strings.Contains(text, key) {
				t.Errorf("%s ships %s, which only the fixture generator needs", image, key)
			}
		}
	}
}

// isLowEntropyHex reports true when a 40-hex digest looks hand-written rather
// than a real SHA-1 (too few distinct characters).
func isLowEntropyHex(digest string) bool {
	if len(digest) != 40 {
		return true
	}
	seen := map[rune]struct{}{}
	for _, r := range digest {
		seen[r] = struct{}{}
	}
	return len(seen) <= 4
}
