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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cl100kURL is hosted on Azure blob storage, not GitHub, so the GitHub proxy
// does not apply.
const cl100kURL = "https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"

const deepdocRepo = "InfiniFlow/deepdoc"

// deepdocModelFiles mirrors common.DeepDocModelFiles, the only weights the
// in-process DeepDoc backend loads. TestDeepDocModelFilesMatchCommon keeps the
// two lists equal; this command cannot import internal/common because that
// package needs cgo.
var deepdocModelFiles = []string{"det.ort", "layout.ort", "tsr.ort", "rec.ort", "ocr.res"}

// tokenizerAsset is one file the embedding token counters in
// internal/tokenizer load, fetched individually because the repositories also
// carry multi-GB weights. Runtime assets ship in the images; oracle assets only
// feed scripts/gen_tokenizer_oracle.py and are not copied into images.
type tokenizerAsset struct {
	repo, file string
	runtime    bool
}

var tokenizerAssets = []tokenizerAsset{
	{"BAAI/bge-m3", "sentencepiece.bpe.model", true},
	{"BAAI/bge-large-en-v1.5", "vocab.txt", true},
	{"Qwen/Qwen3-Embedding-0.6B", "tokenizer.json", true},
	{"intfloat/e5-mistral-7b-instruct", "tokenizer.json", true},
	{"BAAI/bge-m3", "tokenizer.json", false},
	{"BAAI/bge-large-en-v1.5", "tokenizer.json", false},
}

// Noto Sans CJK SC is embedded into PDFs by
// internal/agent/component/io/pdf_writer.go. When notoFontSHA256 is empty, the
// digest recorded in deps/NotoSansCJKsc-VF.ttf.sha256 is the baseline: the
// first download records it and later runs reject drift.
const (
	notoFontFile   = "NotoSansCJKsc-VF.ttf"
	notoFontSHA256 = ""
)

func (d *downloader) notoFontURL() string {
	return d.ghProxy + "https://github.com/notofonts/noto-cjk/raw/main/Sans/Variable/TTF/" + notoFontFile
}

func (d *downloader) hfURL(repo, file string) string {
	return d.hfEndpoint + "/" + repo + "/resolve/main/" + file
}

// provisionRuntime fetches the resources the server loads at run time. The
// CJK font is only needed by the dependency image.
func (d *downloader) provisionRuntime(image bool) error {
	fmt.Fprintln(d.out, "== cl100k_base.tiktoken ==")
	if err := d.provisionCL100K(); err != nil {
		return err
	}
	if image {
		fmt.Fprintln(d.out, "== "+notoFontFile+" ==")
		if err := d.provisionNotoFont(); err != nil {
			return err
		}
	}
	fmt.Fprintln(d.out, "== DeepDoc models ==")
	if err := d.provisionDeepDocModels(); err != nil {
		return err
	}
	fmt.Fprintln(d.out, "== tokenizer assets ==")
	return d.provisionTokenizerAssets()
}

func (d *downloader) provisionCL100K() error {
	target := filepath.Join(d.depsDir, "cl100k_base.tiktoken")
	if nonEmptyFile(target) {
		fmt.Fprintln(d.out, "  ✓ cl100k_base.tiktoken already present")
		return nil
	}
	fmt.Fprintf(d.out, "Downloading cl100k_base.tiktoken from %s...\n", cl100kURL)
	return d.download(cl100kURL, target)
}

// provisionDeepDocModels lands the DeepDoc weights in internal/rag/res/deepdoc,
// which the server discovers without MODEL_DIR, then mirrors them into
// deps/huggingface.co/InfiniFlow/deepdoc for the dependency image build
// context. A tree pre-baked at MODEL_DIR is reused before Hugging Face.
func (d *downloader) provisionDeepDocModels() error {
	targetDir := filepath.Join(d.repoRoot, "internal", "rag", "res", "deepdoc")
	var failures []string
	for _, name := range deepdocModelFiles {
		dest := filepath.Join(targetDir, name)
		if nonEmptyFile(dest) {
			fmt.Fprintf(d.out, "  ✓ %s already present\n", name)
			continue
		}
		if d.modelDir != "" {
			if src := filepath.Join(d.modelDir, name); nonEmptyFile(src) {
				if err := copyFile(src, dest); err != nil {
					return err
				}
				fmt.Fprintf(d.out, "  ✓ %s reused from MODEL_DIR (%s)\n", name, d.modelDir)
				continue
			}
		}
		fmt.Fprintf(d.out, "Downloading DeepDoc model %s...\n", name)
		if err := d.download(d.hfURL(deepdocRepo, name), dest); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("could not fetch DeepDoc models from %s into %s:\n  %s\n"+
			"The in-process DeepDoc backend cannot serve without them. Re-run this command, "+
			"use --china-mirrors behind the GFW, or copy the files into that directory by hand",
			deepdocRepo, targetDir, strings.Join(failures, "\n  "))
	}

	imageDir := filepath.Join(d.depsDir, "huggingface.co", deepdocRepo)
	for _, name := range deepdocModelFiles {
		if err := copyFile(filepath.Join(targetDir, name), filepath.Join(imageDir, name)); err != nil {
			return err
		}
	}
	fmt.Fprintf(d.out, "  ✓ DeepDoc models ready under %s\n", targetDir)
	return nil
}

// provisionTokenizerAssets lands each asset at
// deps/huggingface.co/<repo>/<file>, the path internal/tokenizer searches and
// deps/Dockerfile ships. A missing asset is fatal: models declaring that
// tokenizer refuse ingestion, and the runtime images refuse to build.
func (d *downloader) provisionTokenizerAssets() error {
	base := filepath.Join(d.depsDir, "huggingface.co")
	var failures []string
	for _, a := range tokenizerAssets {
		dest := filepath.Join(base, filepath.FromSlash(a.repo), a.file)
		if nonEmptyFile(dest) {
			fmt.Fprintf(d.out, "  ✓ %s/%s already present\n", a.repo, a.file)
			continue
		}
		fmt.Fprintf(d.out, "Downloading tokenizer asset %s/%s...\n", a.repo, a.file)
		if err := d.download(d.hfURL(a.repo, a.file), dest); err != nil {
			failures = append(failures, fmt.Sprintf("%s/%s: %v", a.repo, a.file, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("could not fetch embedding tokenizer assets into %s:\n  %s\n"+
			"Models that declare these tokenizers cannot be ingested. Re-run this command, "+
			"use --china-mirrors behind the GFW, or point MODEL_ASSETS_DIR at a directory with the same layout",
			base, strings.Join(failures, "\n  "))
	}
	fmt.Fprintln(d.out, "  ✓ embedding tokenizer assets ready")
	return nil
}

// validFont reports whether path is a TrueType font with the 00 01 00 00 sfnt
// signature, the only one gopdf's AddTTFFont accepts. OpenType, collections,
// WOFF, and an HTML error page all fail this check.
func validFont(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := f.Read(head); err != nil {
		return false
	}
	return bytes.Equal(head, []byte{0, 1, 0, 0})
}

func (d *downloader) provisionNotoFont() error {
	font := filepath.Join(d.depsDir, notoFontFile)
	if nonEmptyFile(font) {
		if validFont(font) {
			if err := d.verifyFontChecksum(font); err != nil {
				return err
			}
			fmt.Fprintf(d.out, "  ✓ %s ready\n", notoFontFile)
			return nil
		}
		fmt.Fprintf(d.out, "  WARNING: existing %s is not a valid font; re-downloading\n", notoFontFile)
		if err := os.Remove(font); err != nil {
			return err
		}
	}
	fmt.Fprintf(d.out, "Downloading %s from %s...\n", notoFontFile, d.notoFontURL())
	if err := d.download(d.notoFontURL(), font); err != nil {
		return err
	}
	if !validFont(font) {
		os.Remove(font)
		return fmt.Errorf("downloaded %s is not a TrueType font", notoFontFile)
	}
	if err := d.verifyFontChecksum(font); err != nil {
		return err
	}
	fmt.Fprintf(d.out, "  ✓ %s ready\n", notoFontFile)
	return nil
}

// verifyFontChecksum checks the font against notoFontSHA256 when pinned,
// otherwise against the recorded sidecar, recording it on first use. A
// mismatching font is removed.
func (d *downloader) verifyFontChecksum(font string) error {
	actual, err := fileSHA256(font)
	if err != nil {
		return err
	}
	expected := notoFontSHA256
	sidecar := filepath.Join(d.depsDir, notoFontFile+".sha256")
	if expected == "" {
		data, err := os.ReadFile(sidecar)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if fields := strings.Fields(string(data)); len(fields) > 0 {
			expected = fields[0]
		}
	}
	if expected == "" {
		fmt.Fprintf(d.out, "  recorded %s sha256 %s in %s\n", notoFontFile, actual, sidecar)
		return os.WriteFile(sidecar, []byte(actual+"\n"), 0o644)
	}
	if actual != expected {
		os.Remove(font)
		return fmt.Errorf("%s checksum mismatch (got %s, expected %s)", notoFontFile, actual, expected)
	}
	return nil
}
