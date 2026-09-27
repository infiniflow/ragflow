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

// MODEL_ASSETS_DIR is how a deployment points at a mounted model-asset tree instead of
// relying on the working directory. The test runs in a child process with the working
// directory moved to a temporary tree, because a counter that a test in this process
// already loaded would otherwise hide the effect of the environment variable.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
)

const assetDirChildEnv = "RAGFLOW_TOKENIZER_ASSET_DIR_CHILD"

// realSPMAsset is the asset shipped by ragflow_deps/download_go_deps.py, relative to the
// repository root.
var realSPMAsset = filepath.Join("ragflow_deps", "huggingface.co", "BAAI", "bge-m3", "sentencepiece.bpe.model")

func TestModelAssetsDirIsHonoured(t *testing.T) {
	if os.Getenv(assetDirChildEnv) != "" {
		t.Skip("child process")
	}
	source := filepath.Join("..", "..", realSPMAsset)
	if _, err := os.Stat(source); err != nil {
		t.Skipf("the SPM asset is not present here (%v); run ragflow_deps/download_go_deps.py", err)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, filepath.FromSlash("huggingface.co/BAAI/bge-m3/sentencepiece.bpe.model"))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	copyAssetFile(t, source, target)

	cmd := exec.Command(os.Args[0], "-test.run=^TestModelAssetsDirIsHonouredChild$", "-test.v")
	cmd.Dir = dir // nothing to find by walking up from here
	cmd.Env = append(os.Environ(),
		assetDirChildEnv+"=1",
		common.EnvModelAssetsDir+"="+dir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "asset dir child: ok") {
		t.Fatalf("child did not confirm the override:\n%s", out)
	}
}

// TestModelAssetsDirIsHonoured is the child half: with MODEL_ASSETS_DIR pointing at the
// temporary tree and an unrelated working directory, the counter must still load, and
// report where it loaded from.
func TestModelAssetsDirIsHonouredChild(t *testing.T) {
	if os.Getenv(assetDirChildEnv) == "" {
		t.Skip("parent process")
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if !CounterExact(CounterXLMRSentence) {
		t.Fatalf("%s=%s was not honoured: the xlmr-spm counter is unavailable", common.EnvModelAssetsDir, dir)
	}
	counter, ok := CounterByID(CounterXLMRSentence)
	if !ok {
		t.Fatal("xlmr-spm missing after a successful load check")
	}
	pathAware, ok := counter.(interface{ SourcePath() string })
	if !ok {
		t.Fatal("the counter does not report its source path")
	}
	source := pathAware.SourcePath()
	if !strings.HasPrefix(source, dir) {
		t.Fatalf("counter loaded from %q, expected it under %q", source, dir)
	}
	if !strings.Contains(source, filepath.Join("huggingface.co", "BAAI", "bge-m3")) {
		t.Fatalf("unexpected asset layout in %q", source)
	}
	for _, status := range CounterStatuses() {
		if status.ID == CounterXLMRSentence {
			if !status.Available || status.Source != source {
				t.Fatalf("status for xlmr-spm = %+v, want available from %q", status, source)
			}
			t.Logf("asset dir child: ok (loaded %s)", source)
			return
		}
	}
	t.Fatal("xlmr-spm missing from CounterStatuses")
}

func TestCounterStatusesCoverEveryCounter(t *testing.T) {
	statuses := CounterStatuses()
	want := []string{CounterCL100K, CounterXLMRSentence, CounterBERTWordPiece, CounterQwenBPE, CounterLlamaBPE}
	seen := make(map[string]CounterStatus, len(statuses))
	for _, status := range statuses {
		seen[status.ID] = status
	}
	for _, id := range want {
		status, ok := seen[id]
		if !ok {
			t.Errorf("CounterStatuses is missing %s", id)
			continue
		}
		// The report is what the startup log prints, so it must agree with the
		// question the ingest path asks.
		if status.Available != CounterExact(id) {
			t.Errorf("%s: status.Available=%v but CounterExact=%v", id, status.Available, CounterExact(id))
		}
		if status.Available && status.Source == "" {
			t.Logf("note: %s is available but reports no source path", id)
		}
	}
}

func copyAssetFile(t *testing.T, source, target string) {
	t.Helper()
	in, err := os.Open(source)
	if err != nil {
		t.Fatalf("open %s: %v", source, err)
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		t.Fatalf("create %s: %v", target, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy %s: %v", source, err)
	}
}
