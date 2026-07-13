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

// Package io provides a PDF writer backed by signintech/gopdf.
//
// WritePDF renders the supplied content with gopdf. The writer registers a
// Latin font and, when available, a separate CJK fallback font, then switches
// fonts per text segment. This avoids the blank-page failure where ASCII text
// is sent through a CJK fallback font that does not expose ASCII glyphs.
package io

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/signintech/gopdf"
)

// PDFOptions is the public contract for the PDF writer.
type PDFOptions struct {
	FontSize       int
	HeaderText     string
	FooterText     string
	WatermarkText  string
	AddPageNumbers bool
	AddTimestamp   bool
	FontFamily     string
}

var ErrPDFFontNotConfigured = errors.New("PDF font not configured: install a TTF such as DejaVu Sans or Noto Sans CJK SC")

const (
	pdfLatinFontFamily = "RAGFlowLatin"
	pdfCJKFontFamily   = "RAGFlowCJK"
)

var defaultPDFLatinFontPaths = []string{
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
	"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
}

var defaultPDFCJKFontPaths = []string{
	"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttf",
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
	"/usr/share/fonts/truetype/arphic/uming.ttc",
	"/usr/share/fonts/truetype/arphic/ukai.ttc",
}

type pdfFontSet struct {
	latinFamily string
	cjkFamily   string
	hasCJK      bool
}

// WritePDF renders the content to a PDF byte stream.
func WritePDF(content string, opts PDFOptions) ([]byte, error) {
	if opts.FontSize <= 0 {
		opts.FontSize = 12
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.AddPage()

	fonts, err := ensurePDFFonts(pdf, opts.FontSize)
	if err != nil {
		return nil, err
	}

	drawHeader(pdf, fonts, opts)

	bodyX := 36.0
	bodyY := 72.0
	lineHeight := float64(opts.FontSize) * 1.5
	pdf.SetX(bodyX)
	pdf.SetY(bodyY)

	for _, line := range splitLines(content) {
		if line == "" {
			bodyY += lineHeight
			if bodyY > 760 {
				drawFooter(pdf, fonts, opts)
				pdf.AddPage()
				drawHeader(pdf, fonts, opts)
				bodyY = 72.0
			}
			pdf.SetX(bodyX)
			pdf.SetY(bodyY)
			continue
		}
		if bodyY > 760 {
			drawFooter(pdf, fonts, opts)
			pdf.AddPage()
			drawHeader(pdf, fonts, opts)
			bodyY = 72.0
		}
		pdf.SetX(bodyX)
		pdf.SetY(bodyY)
		if err := drawPDFText(pdf, fonts, line, opts.FontSize); err != nil {
			return nil, fmt.Errorf("PDF: body text: %w", err)
		}
		bodyY += lineHeight
	}

	if opts.WatermarkText != "" {
		drawWatermark(pdf, fonts, opts)
	}
	drawFooter(pdf, fonts, opts)

	return writePDFToBytes(pdf)
}

func ensurePDFFonts(pdf *gopdf.GoPdf, size int) (pdfFontSet, error) {
	latinPath := resolvePDFLatinFontPath()
	if latinPath == "" {
		return pdfFontSet{}, ErrPDFFontNotConfigured
	}
	if err := pdf.AddTTFFont(pdfLatinFontFamily, latinPath); err != nil {
		return pdfFontSet{}, fmt.Errorf("PDF: add latin font from %s: %w", latinPath, err)
	}
	if err := pdf.SetFont(pdfLatinFontFamily, "", size); err != nil {
		return pdfFontSet{}, fmt.Errorf("PDF: set latin font: %w", err)
	}

	fonts := pdfFontSet{latinFamily: pdfLatinFontFamily, cjkFamily: pdfLatinFontFamily}
	cjkPath := resolvePDFCJKFontPath()
	if cjkPath == "" || cjkPath == latinPath {
		return fonts, nil
	}
	if err := pdf.AddTTFFont(pdfCJKFontFamily, cjkPath); err != nil {
		return fonts, nil
	}
	if err := pdf.SetFont(pdfCJKFontFamily, "", size); err != nil {
		return fonts, nil
	}
	fonts.cjkFamily = pdfCJKFontFamily
	fonts.hasCJK = true
	_ = pdf.SetFont(pdfLatinFontFamily, "", size)
	return fonts, nil
}

func resolvePDFLatinFontPath() string {
	return resolvePDFFontPath("RAGFLOW_PDF_LATIN_FONT_PATH", defaultPDFLatinFontPaths)
}

func resolvePDFCJKFontPath() string {
	if explicit := strings.TrimSpace(os.Getenv("RAGFLOW_PDF_FONT_PATH")); explicit != "" {
		if path := normalizeExistingFontPath(explicit); path != "" {
			return path
		}
	}
	return resolvePDFFontPath("RAGFLOW_PDF_CJK_FONT_PATH", defaultPDFCJKFontPaths)
}

func resolvePDFFontPath(envKey string, candidates []string) string {
	if explicit := strings.TrimSpace(os.Getenv(envKey)); explicit != "" {
		if path := normalizeExistingFontPath(explicit); path != "" {
			return path
		}
	}
	for _, candidate := range candidates {
		if path := normalizeExistingFontPath(candidate); path != "" {
			return path
		}
	}
	return ""
}

func normalizeExistingFontPath(candidate string) string {
	path := strings.TrimSpace(candidate)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		return path
	}
	return ""
}

func drawHeader(pdf *gopdf.GoPdf, fonts pdfFontSet, opts PDFOptions) {
	if opts.HeaderText == "" {
		return
	}
	size := overlayFontSize(opts)
	pdf.SetX(36)
	pdf.SetY(24)
	_ = drawPDFText(pdf, fonts, opts.HeaderText, size)
}

func drawFooter(pdf *gopdf.GoPdf, fonts pdfFontSet, opts PDFOptions) {
	if opts.FooterText == "" && !opts.AddTimestamp && !opts.AddPageNumbers {
		return
	}
	size := overlayFontSize(opts)
	pdf.SetX(36)
	pdf.SetY(800)
	parts := []string{}
	if opts.FooterText != "" {
		parts = append(parts, opts.FooterText)
	}
	if opts.AddTimestamp {
		parts = append(parts, time.Now().UTC().Format("2006-01-02 15:04"))
	}
	if opts.AddPageNumbers {
		parts = append(parts, "Page #")
	}
	_ = drawPDFText(pdf, fonts, strings.Join(parts, " | "), size)
}

func drawWatermark(pdf *gopdf.GoPdf, fonts pdfFontSet, opts PDFOptions) {
	if opts.WatermarkText == "" {
		return
	}
	pdf.SetTextColor(200, 200, 200)
	pdf.SetX(120)
	pdf.SetY(360)
	_ = drawPDFText(pdf, fonts, opts.WatermarkText, 48)
	pdf.SetTextColor(0, 0, 0)
}

func overlayFontSize(opts PDFOptions) int {
	size := opts.FontSize - 2
	if size < 1 {
		return 1
	}
	return size
}

func drawPDFText(pdf *gopdf.GoPdf, fonts pdfFontSet, text string, size int) error {
	if text == "" {
		return nil
	}
	for _, segment := range splitByPDFFont(text) {
		family := fonts.latinFamily
		if segment.cjk && fonts.hasCJK {
			family = fonts.cjkFamily
		}
		if err := pdf.SetFont(family, "", size); err != nil {
			return err
		}
		if err := pdf.Text(segment.text); err != nil {
			return err
		}
	}
	return nil
}

type pdfTextSegment struct {
	text string
	cjk  bool
}

func splitByPDFFont(text string) []pdfTextSegment {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	out := []pdfTextSegment{}
	start := 0
	current := needsCJKFont(runes[0])
	for i := 1; i < len(runes); i++ {
		next := needsCJKFont(runes[i])
		if next == current {
			continue
		}
		out = append(out, pdfTextSegment{text: string(runes[start:i]), cjk: current})
		start = i
		current = next
	}
	out = append(out, pdfTextSegment{text: string(runes[start:]), cjk: current})
	return out
}

func needsCJKFont(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) ||
		(r >= 0x3000 && r <= 0x303f) ||
		(r >= 0xff00 && r <= 0xffef)
}

func writePDFToBytes(pdf *gopdf.GoPdf) ([]byte, error) {
	tmp, err := os.CreateTemp("", "ragflow-pdf-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("PDF: tmpfile: %w", err)
	}
	tmpName := tmp.Name()
	if err := pdf.Write(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("PDF: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, fmt.Errorf("PDF: close: %w", err)
	}
	defer os.Remove(tmpName)
	return os.ReadFile(tmpName)
}

// splitLines is a conservative wrapper that splits on \n and
// preserves blank lines as empty strings.
func splitLines(content string) []string {
	if content == "" {
		return []string{""}
	}
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return lines
}
