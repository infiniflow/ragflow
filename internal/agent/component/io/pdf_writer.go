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
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
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
	// Use the language-specific Noto CJK Variable TTF shipped in the runtime image.
	// gopdf uses its default variation instance; it does not expose variable
	// font axes, so this is used as the regular CJK face.
	"/usr/local/share/fonts/truetype/noto/NotoSansCJKsc-VF.ttf",
	"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
}

type pdfFontSet struct {
	latinFamily string
	cjkFamily   string
	hasCJK      bool
}

type pdfBlock struct {
	text        string
	fontSize    int
	indent      float64
	spaceBefore float64
	spaceAfter  float64
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

	bodyX := 36.0
	baseLineHeight := float64(opts.FontSize) * 1.5
	bodyTop := max(72.0, 36+baseLineHeight)
	bodyBottom := 760.0
	if bodyTop+baseLineHeight > bodyBottom {
		return nil, fmt.Errorf("PDF: font size %d leaves no usable body area", opts.FontSize)
	}
	if err := drawHeader(pdf, fonts, opts); err != nil {
		return nil, err
	}
	bodyY := bodyTop
	bodyWidth := gopdf.PageSizeA4.W - 2*bodyX
	pdf.SetX(bodyX)
	pdf.SetY(bodyY)
	pageNumber := 1
	closePage := func() error {
		if err := drawWatermark(pdf, fonts, opts); err != nil {
			return err
		}
		return drawFooter(pdf, fonts, opts, pageNumber)
	}
	newPage := func() error {
		if err := closePage(); err != nil {
			return err
		}
		pdf.AddPage()
		if err := drawHeader(pdf, fonts, opts); err != nil {
			return err
		}
		bodyY = bodyTop
		pageNumber++
		return nil
	}

	for _, block := range markdownPDFBlocks(content, opts.FontSize) {
		lineHeight := float64(block.fontSize) * 1.5
		if bodyTop+lineHeight > bodyBottom {
			return nil, fmt.Errorf("PDF: font size %d leaves no usable body area", block.fontSize)
		}
		if bodyY > bodyTop {
			bodyY += block.spaceBefore
		}
		for _, line := range splitLines(block.text) {
			if line == "" {
				bodyY += lineHeight
				continue
			}
			width := bodyWidth - block.indent
			wrapped, err := wrapPDFText(pdf, fonts, line, block.fontSize, width)
			if err != nil {
				return nil, fmt.Errorf("PDF: wrap body text: %w", err)
			}
			for _, visualLine := range wrapped {
				if bodyY > bodyTop && bodyY+lineHeight > bodyBottom {
					if err := newPage(); err != nil {
						return nil, err
					}
				}
				pdf.SetX(bodyX + block.indent)
				pdf.SetY(bodyY)
				if err := drawPDFText(pdf, fonts, visualLine, block.fontSize); err != nil {
					return nil, fmt.Errorf("PDF: body text: %w", err)
				}
				bodyY += lineHeight
			}
		}
		bodyY += block.spaceAfter
	}

	if err := closePage(); err != nil {
		return nil, err
	}

	return writePDFToBytes(pdf)
}

func markdownPDFBlocks(content string, baseSize int) []pdfBlock {
	source := []byte(content)
	document := goldmark.DefaultParser().Parse(gmtext.NewReader(source))
	blocks := make([]pdfBlock, 0, document.ChildCount())
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		switch n := node.(type) {
		case *ast.Heading:
			sizes := [...]int{8, 6, 4, 2, 1, 0}
			blocks = append(blocks, pdfBlock{
				text: inlinePDFText(n, source), fontSize: baseSize + sizes[n.Level-1],
				spaceBefore: float64(baseSize) * 0.6, spaceAfter: float64(baseSize) * 0.5,
			})
		case *ast.Paragraph:
			blocks = append(blocks, pdfBlock{text: inlinePDFText(n, source), fontSize: baseSize, spaceAfter: float64(baseSize)})
		case *ast.List:
			index := n.Start
			for item := n.FirstChild(); item != nil; item = item.NextSibling() {
				prefix := "• "
				if n.IsOrdered() {
					prefix = fmt.Sprintf("%d. ", index)
					index++
				}
				blocks = append(blocks, pdfBlock{text: prefix + inlinePDFText(item, source), fontSize: baseSize, indent: 12, spaceAfter: float64(baseSize) * 0.35})
			}
		case *ast.Blockquote:
			blocks = append(blocks, pdfBlock{text: "| " + inlinePDFText(n, source), fontSize: baseSize, indent: 12, spaceAfter: float64(baseSize)})
		case *ast.CodeBlock:
			blocks = append(blocks, pdfBlock{text: rawPDFBlockText(n, source), fontSize: baseSize, indent: 12, spaceAfter: float64(baseSize)})
		case *ast.FencedCodeBlock:
			blocks = append(blocks, pdfBlock{text: rawPDFBlockText(n, source), fontSize: baseSize, indent: 12, spaceAfter: float64(baseSize)})
		case *ast.ThematicBreak:
			blocks = append(blocks, pdfBlock{fontSize: baseSize, spaceAfter: float64(baseSize)})
		default:
			if text := inlinePDFText(n, source); text != "" {
				blocks = append(blocks, pdfBlock{text: text, fontSize: baseSize, spaceAfter: float64(baseSize)})
			}
		}
	}
	return blocks
}

func inlinePDFText(node ast.Node, source []byte) string {
	var text strings.Builder
	_ = ast.Walk(node, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Text:
			text.Write(n.Text(source))
			if n.SoftLineBreak() || n.HardLineBreak() {
				text.WriteByte('\n')
			}
		case *ast.String:
			text.Write(n.Text(source))
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(text.String())
}

func rawPDFBlockText(node ast.Node, source []byte) string {
	var text strings.Builder
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		line := lines.At(i)
		text.Write(line.Value(source))
	}
	return strings.TrimRight(text.String(), "\r\n")
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

func drawHeader(pdf *gopdf.GoPdf, fonts pdfFontSet, opts PDFOptions) error {
	if opts.HeaderText == "" {
		return nil
	}
	return drawPDFTextFitted(pdf, fonts, opts.HeaderText, min(overlayFontSize(opts), 20), 36, 24, gopdf.PageSizeA4.W-72)
}

func drawFooter(pdf *gopdf.GoPdf, fonts pdfFontSet, opts PDFOptions, pageNumber int) error {
	if opts.FooterText == "" && !opts.AddTimestamp && !opts.AddPageNumbers {
		return nil
	}
	size := overlayFontSize(opts)
	parts := []string{}
	if opts.FooterText != "" {
		parts = append(parts, opts.FooterText)
	}
	if opts.AddTimestamp {
		parts = append(parts, time.Now().UTC().Format("2006-01-02 15:04"))
	}
	if opts.AddPageNumbers {
		parts = append(parts, fmt.Sprintf("Pages %d", pageNumber))
	}
	return drawPDFTextFitted(pdf, fonts, strings.Join(parts, " | "), min(size, 42), 36, 800, gopdf.PageSizeA4.W-72)
}

func drawWatermark(pdf *gopdf.GoPdf, fonts pdfFontSet, opts PDFOptions) error {
	if opts.WatermarkText == "" {
		return nil
	}
	pdf.SetTextColor(200, 200, 200)
	err := drawPDFTextFitted(pdf, fonts, opts.WatermarkText, 48, 120, 360, gopdf.PageSizeA4.W-156)
	pdf.SetTextColor(0, 0, 0)
	return err
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

func drawPDFTextFitted(pdf *gopdf.GoPdf, fonts pdfFontSet, text string, size int, x, y, width float64) error {
	for ; size > 0; size-- {
		measured := 0.0
		for _, segment := range splitByPDFFont(text) {
			family := fonts.latinFamily
			if segment.cjk && fonts.hasCJK {
				family = fonts.cjkFamily
			}
			if err := pdf.SetFont(family, "", size); err != nil {
				return err
			}
			segmentWidth, err := pdf.MeasureTextWidth(segment.text)
			if err != nil {
				return err
			}
			measured += segmentWidth
		}
		if measured <= width {
			pdf.SetX(x)
			pdf.SetY(y)
			return drawPDFText(pdf, fonts, text, size)
		}
	}
	return fmt.Errorf("PDF: decoration text is too long for the page")
}

func wrapPDFText(pdf *gopdf.GoPdf, fonts pdfFontSet, text string, size int, width float64) ([]string, error) {
	lines := []string{}
	current := ""
	currentWidth := 0.0
	for _, segment := range splitByPDFFont(text) {
		family := fonts.latinFamily
		if segment.cjk && fonts.hasCJK {
			family = fonts.cjkFamily
		}
		if err := pdf.SetFont(family, "", size); err != nil {
			return nil, err
		}
		segmentWidth, err := pdf.MeasureTextWidth(segment.text)
		if err != nil {
			return nil, err
		}
		if currentWidth+segmentWidth <= width {
			current += segment.text
			currentWidth += segmentWidth
			continue
		}
		if current != "" {
			lines = append(lines, current)
			current = ""
			currentWidth = 0
		}
		if segmentWidth <= width {
			current = segment.text
			currentWidth = segmentWidth
			continue
		}
		for _, r := range segment.text {
			runeWidth, err := pdf.MeasureTextWidth(string(r))
			if err != nil {
				return nil, err
			}
			if runeWidth > width {
				return nil, fmt.Errorf("font size %d is too large for the PDF body width", size)
			}
		}
		wrapped, err := pdf.SplitTextWithWordWrap(segment.text, width)
		if err != nil {
			return nil, err
		}
		lines = append(lines, wrapped[:len(wrapped)-1]...)
		current = wrapped[len(wrapped)-1]
		currentWidth, err = pdf.MeasureTextWidth(current)
		if err != nil {
			return nil, err
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines, nil
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
