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
	"encoding/xml"
	"image"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestWritePDF_WrapsAndPaginatesWithinMargins(t *testing.T) {
	requirePDFLatinFont(t)
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available")
	}

	content := "First marker.\n\nSecond marker. " +
		strings.Repeat("This paragraph must wrap inside the page margins. ", 8) + "\n" +
		strings.Repeat("Mixed English and 中文内容 must use measured fallback glyph widths. ", 8) + "\n" +
		strings.Repeat("UNBROKEN", 80) + "\n" +
		strings.Repeat("A final line forces pagination while preserving every word.\n", 80)
	out, err := WritePDF(content, PDFOptions{})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}

	pdfPath := filepath.Join(t.TempDir(), "wrapped.pdf")
	if err := os.WriteFile(pdfPath, out, 0o600); err != nil {
		t.Fatalf("WriteFile pdf: %v", err)
	}
	bbox, err := exec.Command("pdftotext", "-bbox-layout", pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v: %s", err, bbox)
	}

	var document struct {
		Pages []struct {
			Width  float64 `xml:"width,attr"`
			Height float64 `xml:"height,attr"`
			Words  []struct {
				Text string  `xml:",chardata"`
				XMax float64 `xml:"xMax,attr"`
				YMin float64 `xml:"yMin,attr"`
				YMax float64 `xml:"yMax,attr"`
			} `xml:"flow>block>line>word"`
		} `xml:"body>doc>page"`
	}
	if err := xml.Unmarshal(bbox, &document); err != nil {
		t.Fatalf("parse bbox output: %v", err)
	}
	if len(document.Pages) < 2 {
		t.Fatalf("pages = %d, want at least 2", len(document.Pages))
	}
	extracted, err := exec.Command("pdftotext", pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext text: %v: %s", err, extracted)
	}
	for _, marker := range []string{"First marker.", "Second marker.", "Mixed English and", "中文内容", "UNBROKEN"} {
		if !bytes.Contains(extracted, []byte(marker)) {
			t.Fatalf("extracted PDF text missing %q", marker)
		}
	}
	compact := strings.Join(strings.Fields(string(extracted)), "")
	if got := strings.Count(compact, "UNBROKEN"); got != 80 {
		t.Fatalf("unbroken token count = %d, want 80", got)
	}
	if got := strings.Count(string(extracted), "A final line forces pagination while preserving every word."); got != 80 {
		t.Fatalf("final line count = %d, want 80", got)
	}

	var firstY, secondY float64
	for _, page := range document.Pages {
		for _, word := range page.Words {
			if word.XMax > page.Width-36 || word.YMin < 0 || word.YMax > page.Height {
				t.Fatalf("word %q bounds exceed page: xMax=%.2f y=%.2f..%.2f page=%.2fx%.2f", word.Text, word.XMax, word.YMin, word.YMax, page.Width, page.Height)
			}
			switch word.Text {
			case "First":
				firstY = word.YMin
			case "Second":
				secondY = word.YMin
			}
		}
	}
	if firstY == 0 || secondY-firstY < 30 {
		t.Fatalf("blank line not preserved: first y=%.2f second y=%.2f", firstY, secondY)
	}
}

func TestWritePDF_LargeFontStartsInsidePage(t *testing.T) {
	requirePDFLatinFont(t)
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available")
	}

	out, err := WritePDF("BodyMarker 中文 Ẫ", PDFOptions{
		FontSize:       48,
		HeaderText:     "HeaderMarker 标题 Ẫ",
		FooterText:     "Large footer",
		AddPageNumbers: true,
	})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}
	pdfPath := filepath.Join(t.TempDir(), "large-font.pdf")
	if err := os.WriteFile(pdfPath, out, 0o600); err != nil {
		t.Fatalf("WriteFile pdf: %v", err)
	}
	bbox, err := exec.Command("pdftotext", "-bbox-layout", pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v: %s", err, bbox)
	}
	var document struct {
		Pages []struct {
			Height float64 `xml:"height,attr"`
			Words  []struct {
				Text string  `xml:",chardata"`
				YMin float64 `xml:"yMin,attr"`
				YMax float64 `xml:"yMax,attr"`
			} `xml:"flow>block>line>word"`
		} `xml:"body>doc>page"`
	}
	if err := xml.Unmarshal(bbox, &document); err != nil {
		t.Fatalf("parse bbox output: %v", err)
	}
	if len(document.Pages) != 1 || len(document.Pages[0].Words) == 0 {
		t.Fatalf("unexpected bbox output: %+v", document)
	}
	headerBottom := 0.0
	bodyTop := document.Pages[0].Height
	for _, word := range document.Pages[0].Words {
		if word.YMin < 0 || word.YMax > document.Pages[0].Height {
			t.Fatalf("word vertical bounds %.2f..%.2f exceed page height %.2f", word.YMin, word.YMax, document.Pages[0].Height)
		}
		switch word.Text {
		case "HeaderMarker":
			headerBottom = max(headerBottom, word.YMax)
		case "BodyMarker":
			bodyTop = min(bodyTop, word.YMin)
		}
	}
	if bodyTop <= headerBottom {
		t.Fatalf("body starts at %.2f before header ends at %.2f", bodyTop, headerBottom)
	}
}

func TestWritePDF_LongDecorationsFitPage(t *testing.T) {
	requirePDFLatinFont(t)
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available")
	}

	longText := strings.Repeat("Long decoration text ", 12)
	out, err := WritePDF("Body", PDFOptions{
		HeaderText:    longText,
		FooterText:    longText,
		WatermarkText: longText,
	})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}
	pdfPath := filepath.Join(t.TempDir(), "decorations.pdf")
	if err := os.WriteFile(pdfPath, out, 0o600); err != nil {
		t.Fatalf("WriteFile pdf: %v", err)
	}
	bbox, err := exec.Command("pdftotext", "-bbox-layout", pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v: %s", err, bbox)
	}
	var document struct {
		Pages []struct {
			Width float64 `xml:"width,attr"`
			Words []struct {
				Text string  `xml:",chardata"`
				XMax float64 `xml:"xMax,attr"`
			} `xml:"flow>block>line>word"`
		} `xml:"body>doc>page"`
	}
	if err := xml.Unmarshal(bbox, &document); err != nil {
		t.Fatalf("parse bbox output: %v", err)
	}
	for _, page := range document.Pages {
		for _, word := range page.Words {
			if word.XMax > page.Width-36 {
				t.Fatalf("word %q ends at %.2f, beyond right margin %.2f", word.Text, word.XMax, page.Width-36)
			}
		}
	}
}

func TestWritePDF_RejectsFontTooLargeForBody(t *testing.T) {
	requirePDFLatinFont(t)
	_, err := WritePDF("Body", PDFOptions{FontSize: 1_000_000_000, HeaderText: "Header"})
	if err == nil || !strings.Contains(err.Error(), "leaves no usable body area") {
		t.Fatalf("WritePDF error = %v, want unusable body area", err)
	}
}

func TestWritePDF_RendersMarkdownHeadings(t *testing.T) {
	requirePDFLatinFont(t)
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available")
	}

	out, err := WritePDF("# TitleMarker\n\n## SectionMarker\n\nBodyMarker paragraph.", PDFOptions{FontSize: 12})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}
	pdfPath := filepath.Join(t.TempDir(), "markdown.pdf")
	if err := os.WriteFile(pdfPath, out, 0o600); err != nil {
		t.Fatalf("WriteFile pdf: %v", err)
	}
	bbox, err := exec.Command("pdftotext", "-bbox-layout", pdfPath, "-").CombinedOutput()
	if err != nil {
		t.Fatalf("pdftotext: %v: %s", err, bbox)
	}
	var document struct {
		Pages []struct {
			Words []struct {
				Text string  `xml:",chardata"`
				YMin float64 `xml:"yMin,attr"`
				YMax float64 `xml:"yMax,attr"`
			} `xml:"flow>block>line>word"`
		} `xml:"body>doc>page"`
	}
	if err := xml.Unmarshal(bbox, &document); err != nil {
		t.Fatalf("parse bbox output: %v", err)
	}
	heights := map[string]float64{}
	for _, page := range document.Pages {
		for _, word := range page.Words {
			if word.Text == "#" || word.Text == "##" {
				t.Fatalf("rendered literal Markdown marker %q", word.Text)
			}
			heights[word.Text] = word.YMax - word.YMin
		}
	}
	if heights["TitleMarker"] <= heights["BodyMarker"] || heights["SectionMarker"] <= heights["BodyMarker"] {
		t.Fatalf("heading heights = title %.2f section %.2f body %.2f", heights["TitleMarker"], heights["SectionMarker"], heights["BodyMarker"])
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
