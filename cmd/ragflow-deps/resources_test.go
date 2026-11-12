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

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func remoteRuntime() *fakeRemote {
	remote := newFakeRemote()
	remote.files["/encodings/cl100k_base.tiktoken"] = []byte("cl100k")
	for _, name := range deepdocModelFiles {
		remote.files["/"+deepdocRepo+"/resolve/main/"+name] = []byte("model " + name)
	}
	for _, a := range tokenizerAssets {
		remote.files["/"+a.repo+"/resolve/main/"+a.file] = []byte("asset " + a.repo + "/" + a.file)
	}
	return remote
}

func modelPath(d *downloader, name string) string {
	return filepath.Join(d.repoRoot, "internal", "rag", "res", "deepdoc", name)
}

func TestProvisionRuntimeDownloadsThenSkips(t *testing.T) {
	remote := remoteRuntime()
	d := testDownloader(t, remote)
	if err := d.provisionRuntime(false); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, filepath.Join(d.depsDir, "cl100k_base.tiktoken"))); got != "cl100k" {
		t.Errorf("cl100k = %q", got)
	}
	for _, name := range deepdocModelFiles {
		want := "model " + name
		if got := string(mustRead(t, modelPath(d, name))); got != want {
			t.Errorf("%s = %q", name, got)
		}
		if got := string(mustRead(t, filepath.Join(d.depsDir, "huggingface.co", deepdocRepo, name))); got != want {
			t.Errorf("image copy of %s = %q", name, got)
		}
	}
	for _, a := range tokenizerAssets {
		path := filepath.Join(d.depsDir, "huggingface.co", filepath.FromSlash(a.repo), a.file)
		if !nonEmptyFile(path) {
			t.Errorf("%s/%s missing", a.repo, a.file)
		}
	}
	if _, err := os.Stat(filepath.Join(d.depsDir, notoFontFile)); !os.IsNotExist(err) {
		t.Error("development mode fetched the image-only font")
	}

	remote.resetHits()
	if err := d.provisionRuntime(false); err != nil {
		t.Fatal(err)
	}
	if len(remote.hits) != 0 {
		t.Errorf("present resources downloaded again: %v", remote.hits)
	}
}

func TestDeepDocModelsReuseModelDir(t *testing.T) {
	remote := remoteRuntime()
	d := testDownloader(t, remote)
	d.modelDir = t.TempDir()
	mustWrite(t, filepath.Join(d.modelDir, "det.ort"), []byte("baked det"))
	mustWrite(t, filepath.Join(d.modelDir, "layout.ort"), nil) // empty: fall back to Hugging Face

	if err := d.provisionDeepDocModels(); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, modelPath(d, "det.ort"))); got != "baked det" {
		t.Errorf("det.ort = %q; want the MODEL_DIR copy", got)
	}
	if remote.hitCount("/"+deepdocRepo+"/resolve/main/det.ort") != 0 {
		t.Error("det.ort downloaded despite MODEL_DIR")
	}
	if got := string(mustRead(t, modelPath(d, "layout.ort"))); got != "model layout.ort" {
		t.Errorf("layout.ort = %q; want the Hugging Face copy", got)
	}
}

func TestDeepDocModelsFailLoud(t *testing.T) {
	remote := remoteRuntime()
	remote.status["/"+deepdocRepo+"/resolve/main/rec.ort"] = http.StatusNotFound
	d := testDownloader(t, remote)
	err := d.provisionDeepDocModels()
	if err == nil || !strings.Contains(err.Error(), "rec.ort") || !strings.Contains(err.Error(), "--china-mirrors") {
		t.Fatalf("err = %v", err)
	}
	if !nonEmptyFile(modelPath(d, "tsr.ort")) {
		t.Error("a failure aborted the remaining downloads")
	}
}

func TestTokenizerAssetsFailLoud(t *testing.T) {
	remote := remoteRuntime()
	remote.status["/BAAI/bge-m3/resolve/main/sentencepiece.bpe.model"] = http.StatusNotFound
	d := testDownloader(t, remote)
	err := d.provisionTokenizerAssets()
	if err == nil || !strings.Contains(err.Error(), "BAAI/bge-m3/sentencepiece.bpe.model") {
		t.Fatalf("err = %v", err)
	}
}

func TestChinaMirrorRoutesHuggingFace(t *testing.T) {
	d := &downloader{hfEndpoint: chinaHFEndpoint}
	if got := d.hfURL("BAAI/bge-m3", "vocab.txt"); got != "https://hf-mirror.com/BAAI/bge-m3/resolve/main/vocab.txt" {
		t.Errorf("hfURL = %q", got)
	}
}

// TestDockerfilesShipRuntimeTokenizerAssets keeps the images' copy loops in
// step with tokenizerAssets: every runtime asset ships, and the oracle-only
// files do not.
func TestDockerfilesShipRuntimeTokenizerAssets(t *testing.T) {
	root, err := findRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Dockerfile", "Dockerfile_base"} {
		text := string(mustRead(t, filepath.Join(root, name)))
		for _, a := range tokenizerAssets {
			entry := a.repo + "/" + a.file
			if shipped := strings.Contains(text, entry); shipped != a.runtime {
				t.Errorf("%s ships %s = %v; want %v", name, entry, shipped, a.runtime)
			}
		}
	}
}

const fontBody = "\x00\x01\x00\x00font-tables"

func TestNotoFontRecordsThenEnforcesChecksum(t *testing.T) {
	remote := newFakeRemote()
	fontPath := "/notofonts/noto-cjk/raw/main/Sans/Variable/TTF/" + notoFontFile
	remote.files[fontPath] = []byte(fontBody)
	d := testDownloader(t, remote)
	font := filepath.Join(d.depsDir, notoFontFile)
	sidecar := font + ".sha256"

	if err := d.provisionNotoFont(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(mustRead(t, sidecar))); got != sha256Hex([]byte(fontBody)) {
		t.Errorf("recorded sha256 = %q", got)
	}

	remote.resetHits()
	if err := d.provisionNotoFont(); err != nil {
		t.Fatal(err)
	}
	if remote.hitCount(fontPath) != 0 {
		t.Error("verified font downloaded again")
	}

	mustWrite(t, font, []byte("\x00\x01\x00\x00tampered"))
	if err := d.provisionNotoFont(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("err = %v; want checksum mismatch", err)
	}
	if nonEmptyFile(font) {
		t.Error("mismatching font kept")
	}
}

func TestNotoFontRejectsNonTrueType(t *testing.T) {
	remote := newFakeRemote()
	remote.files["/notofonts/noto-cjk/raw/main/Sans/Variable/TTF/"+notoFontFile] = []byte("<html>not found</html>")
	d := testDownloader(t, remote)
	if err := d.provisionNotoFont(); err == nil || !strings.Contains(err.Error(), "not a TrueType font") {
		t.Errorf("err = %v", err)
	}
	if nonEmptyFile(filepath.Join(d.depsDir, notoFontFile)) {
		t.Error("non-font kept")
	}
	for _, sig := range []string{"OTTO", "ttcf", "wOFF"} {
		path := filepath.Join(t.TempDir(), "f")
		mustWrite(t, path, []byte(sig+"rest"))
		if validFont(path) {
			t.Errorf("%s signature accepted", sig)
		}
	}
}
