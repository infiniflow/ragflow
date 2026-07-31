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

package io

import (
	"bytes"
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWritePDF_UsesGoLibrary(t *testing.T) {
	requirePDFLatinFont(t)
	out, err := WritePDF("Hello\n中文", PDFOptions{})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("PDF output missing magic header: %q", out[:min(len(out), 8)])
	}
}

func TestWritePDF_RendersVisibleText(t *testing.T) {
	requirePDFLatinFont(t)
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not available")
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available")
	}

	out, err := WritePDF("Visible PDF body", PDFOptions{
		HeaderText:     "Visible Header",
		FooterText:     "Visible Footer",
		AddPageNumbers: true,
		AddTimestamp:   false,
	})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "visible.pdf")
	if err := os.WriteFile(pdfPath, out, 0o600); err != nil {
		t.Fatalf("WriteFile pdf: %v", err)
	}
	prefix := filepath.Join(dir, "page")
	cmd := exec.Command("pdftoppm", "-png", "-singlefile", pdfPath, prefix)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pdftoppm: %v: %s", err, output)
	}
	pngFile, err := os.Open(prefix + ".png")
	if err != nil {
		t.Fatalf("Open png: %v", err)
	}
	defer pngFile.Close()
	img, _, err := image.Decode(pngFile)
	if err != nil {
		t.Fatalf("Decode png: %v", err)
	}
	bounds := img.Bounds()
	nonWhite := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r != 0xffff || g != 0xffff || b != 0xffff {
				nonWhite++
			}
		}
	}
	if nonWhite == 0 {
		t.Fatal("rendered PDF page is blank")
	}

	textOutput, err := exec.Command("pdftotext", pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v: %s", err, textOutput)
	}
	if !bytes.Contains(textOutput, []byte("Visible PDF body")) {
		t.Fatalf("pdftotext output = %q, want body text", textOutput)
	}
}

func TestResolvePDFLatinFontPathHonorsEnv(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "custom.ttf")
	if err := os.WriteFile(fontPath, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("WriteFile font: %v", err)
	}
	t.Setenv("RAGFLOW_PDF_LATIN_FONT_PATH", fontPath)
	if got := resolvePDFLatinFontPath(); got != fontPath {
		t.Fatalf("resolvePDFLatinFontPath() = %q, want %q", got, fontPath)
	}
}

func requirePDFLatinFont(t *testing.T) {
	t.Helper()
	if resolvePDFLatinFontPath() == "" {
		t.Skip("no local Latin PDF font available")
	}
}
