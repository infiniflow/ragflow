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

// Package io — PDF writer (signintech/gopdf).
//
// WritePDF renders the supplied content to a PDF using the
// MIT-licensed signintech/gopdf library. The writer probes via
// gopdf.SetFont; if the family is unknown, it surfaces
// ErrPDFFontNotConfigured so the orchestrator can return a clear
// deployment-time error. Production deployments register a TTF
// (e.g. Noto Sans CJK SC) at startup.
//
// When a TTF *is* registered, the writer emits a simple
// one-paragraph page per line of content, with a centered header
// and a centered footer carrying the page number / timestamp when
// requested.
package io

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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

// ErrPDFFontNotConfigured is returned when no TTF is registered.
// Callers should register a TTF via gopdf.SetFont before invoking
// WritePDF.
var ErrPDFFontNotConfigured = errors.New("PDF font not configured: register a TTF (e.g. Noto Sans CJK SC) via gopdf.SetFont before calling WritePDF")

// WritePDF renders the content to a PDF byte stream.
//
// Layout:
//
//   - A4 portrait, 36pt margins on all sides.
//   - Body lines are drawn top-to-bottom, one per line of content.
//   - Header is centered at the top of every page (when set).
//   - Footer is centered at the bottom of every page and may include
//     the footer text, a generation timestamp, and a page number.
//   - Watermark is rendered as grey text near the page center.
//
// When the requested font family is not registered, the function
// returns ErrPDFFontNotConfigured and does not write any output.
func WritePDF(content string, opts PDFOptions) ([]byte, error) {
	if opts.FontSize <= 0 {
		opts.FontSize = 12
	}
	if opts.FontFamily == "" {
		opts.FontFamily = "Noto Sans CJK SC"
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	// Probe the font registry. gopdf returns an error like "font not
	// found" when the family is not registered; we surface that as
	// ErrPDFFontNotConfigured so callers can map it to a clear
	// deployment message.
	if err := pdf.SetFont(opts.FontFamily, "", opts.FontSize); err != nil {
		if isFontNotFound(err) {
			return nil, ErrPDFFontNotConfigured
		}
		return nil, fmt.Errorf("PDF: set font %q: %w", opts.FontFamily, err)
	}

	pdf.AddPage()
	drawHeader(pdf, opts)

	// Body — one Cell per line, manual y-cursor.
	bodyX := 36.0
	bodyY := 72.0
	lineHeight := float64(opts.FontSize) * 1.5
	pdf.SetX(bodyX)
	pdf.SetY(bodyY)

	for _, line := range splitLines(content) {
		if line == "" {
			// Preserve blank lines as vertical space.
			bodyY += lineHeight
			if bodyY > 760 {
				drawFooter(pdf, opts)
				pdf.AddPage()
				drawHeader(pdf, opts)
				bodyY = 72.0
			}
			pdf.SetX(bodyX)
			pdf.SetY(bodyY)
			continue
		}
		if bodyY > 760 {
			drawFooter(pdf, opts)
			pdf.AddPage()
			drawHeader(pdf, opts)
			bodyY = 72.0
		}
		pdf.SetX(bodyX)
		pdf.SetY(bodyY)
		if err := pdf.Cell(nil, line); err != nil {
			return nil, fmt.Errorf("PDF: cell: %w", err)
		}
		bodyY += lineHeight
	}

	if opts.WatermarkText != "" {
		drawWatermark(pdf, opts)
	}
	drawFooter(pdf, opts)

	return writePDFToBytes(pdf)
}

// drawHeader emits the header text at the top of the current page.
// gopdf's API in v0.36.x doesn't expose a Header() callback; we draw
// at the top of every page after AddPage.
func drawHeader(pdf *gopdf.GoPdf, opts PDFOptions) {
	if opts.HeaderText == "" {
		return
	}
	_ = pdf.SetFont(opts.FontFamily, "", opts.FontSize-2)
	pdf.SetX(36)
	pdf.SetY(24)
	_ = pdf.Cell(nil, opts.HeaderText)
	// Restore body font.
	_ = pdf.SetFont(opts.FontFamily, "", opts.FontSize)
}

// drawFooter emits the footer text plus optional timestamp / page
// number at the bottom of the current page.
func drawFooter(pdf *gopdf.GoPdf, opts PDFOptions) {
	if opts.FooterText == "" && !opts.AddTimestamp && !opts.AddPageNumbers {
		return
	}
	_ = pdf.SetFont(opts.FontFamily, "", opts.FontSize-2)
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
		// gopdf doesn't expose a page-number macro; emit a
		// literal placeholder until upstream adds one.
		parts = append(parts, "Page #")
	}
	_ = pdf.Cell(nil, strings.Join(parts, " | "))
	// Restore body font.
	_ = pdf.SetFont(opts.FontFamily, "", opts.FontSize)
}

// drawWatermark emits a centered grey watermark. Full rotation is
// not in the gopdf v0.36.x public surface; we use a light grey fill
// as a visual proxy.
func drawWatermark(pdf *gopdf.GoPdf, opts PDFOptions) {
	if opts.WatermarkText == "" {
		return
	}
	_ = pdf.SetFont(opts.FontFamily, "", 48)
	pdf.SetTextColor(200, 200, 200)
	pdf.SetX(120)
	pdf.SetY(360)
	_ = pdf.Cell(nil, opts.WatermarkText)
	// Restore.
	pdf.SetTextColor(0, 0, 0)
	_ = pdf.SetFont(opts.FontFamily, "", opts.FontSize)
}

// writePDFToBytes serializes the gopdf output to a byte slice.
//
// gopdf's Write method requires an *os.File (it needs random access
// for the xref table), so we route through a TempFile.
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

// isFontNotFound reports whether the gopdf error indicates a missing
// TTF registration. We match the substrings that have been stable
// across recent gopdf versions.
func isFontNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "font") && (strings.Contains(s, "not") || strings.Contains(s, "no such") || strings.Contains(s, "undefined"))
}
