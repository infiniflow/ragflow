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
	"strings"
	"testing"
	"time"
)

// TestWriteHTML_Minimal: empty options produce a valid HTML5
// document with default font / size.
func TestWriteHTML_Minimal(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{}))
	if !strings.HasPrefix(out, "<!DOCTYPE html>") {
		t.Errorf("missing DOCTYPE: %q", out)
	}
	if !strings.Contains(out, "<main>\nbody\n</main>") {
		t.Errorf("missing main block: %q", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "</html>") {
		t.Errorf("missing closing </html>: %q", out)
	}
	// No header / footer / watermark → no <header> / <footer> /
	// .watermark CSS.
	if strings.Contains(out, "<header>") {
		t.Errorf("unexpected <header> block in minimal output: %q", out)
	}
	if strings.Contains(out, "<footer>") {
		t.Errorf("unexpected <footer> block in minimal output: %q", out)
	}
	if strings.Contains(out, "watermark") {
		t.Errorf("unexpected watermark CSS in minimal output: %q", out)
	}
}

// TestWriteHTML_HeaderInTitle: header text populates the <title>
// and a <header> block.
func TestWriteHTML_HeaderInTitle(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{HeaderText: "My Doc"}))
	if !strings.Contains(out, "<title>My Doc</title>") {
		t.Errorf("missing title: %q", out)
	}
	if !strings.Contains(out, "<header>My Doc</header>") {
		t.Errorf("missing <header> block: %q", out)
	}
}

// TestWriteHTML_FooterBlock: footer text populates a <footer>
// block at the end of the body.
func TestWriteHTML_FooterBlock(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{FooterText: "page 1"}))
	if !strings.Contains(out, "<footer>page 1</footer>") {
		t.Errorf("missing <footer> block: %q", out)
	}
}

// TestWriteHTML_Watermark: watermark text triggers the
// .watermark CSS and a <div class="watermark"> block.
func TestWriteHTML_Watermark(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{WatermarkText: "DRAFT"}))
	if !strings.Contains(out, ".watermark {") {
		t.Errorf("missing .watermark CSS: %q", out)
	}
	if !strings.Contains(out, `<div class="watermark">DRAFT</div>`) {
		t.Errorf("missing watermark <div>: %q", out)
	}
}

// TestWriteHTML_FontSizeFamily: fontSize + fontFamily propagate
// to the body CSS rule.
func TestWriteHTML_FontSizeFamily(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{
		FontSize:   14,
		FontFamily: "Georgia",
	}))
	wantStyle := `body { font-family: "Georgia"; font-size: 14pt; line-height: 1.5; }`
	if !strings.Contains(out, wantStyle) {
		t.Errorf("style block missing/wrong:\n got  %q\n want substring %q", out, wantStyle)
	}
}

// TestWriteHTML_DefaultsApplied: zero FontSize / empty FontFamily
// fall back to 11pt / sans-serif.
func TestWriteHTML_DefaultsApplied(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{}))
	if !strings.Contains(out, `font-family: "sans-serif"`) {
		t.Errorf("default font-family not applied: %q", out)
	}
	if !strings.Contains(out, "font-size: 11pt") {
		t.Errorf("default font-size not applied: %q", out)
	}
}

// TestWriteHTML_Timestamp: AddTimestamp=true inserts a "Generated:"
// line at the end of the body, using the supplied Now value.
func TestWriteHTML_Timestamp(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	out := string(WriteHTML("body", HTMLOptions{AddTimestamp: true, Now: fixed}))
	want := "<p><small>Generated: 2026-06-15T12:00:00Z</small></p>"
	if !strings.Contains(out, want) {
		t.Errorf("missing timestamp line: %q", out)
	}
}

// TestWriteHTML_HTMLStructure: the document ends with the closing
// tags in the right order. Catches accidental re-ordering.
func TestWriteHTML_HTMLStructure(t *testing.T) {
	out := string(WriteHTML("body", HTMLOptions{
		HeaderText: "H", FooterText: "F",
	}))
	// Order matters: <header> must come before <main> which must
	// come before <footer>.
	idxH := strings.Index(out, "<header>H</header>")
	idxM := strings.Index(out, "<main>")
	idxF := strings.Index(out, "<footer>F</footer>")
	if idxH < 0 || idxM < 0 || idxF < 0 || !(idxH < idxM && idxM < idxF) {
		t.Errorf("expected order header < main < footer, got idxH=%d idxM=%d idxF=%d",
			idxH, idxM, idxF)
	}
}

// TestWriteHTML_ContentNotEscaped: the writer does NOT HTML-escape
// the body — callers are expected to pass already-clean content.
// This test pins the no-escape contract.
func TestWriteHTML_ContentNotEscaped(t *testing.T) {
	out := string(WriteHTML("<b>bold</b>", HTMLOptions{}))
	if !strings.Contains(out, "<b>bold</b>") {
		t.Errorf("content should pass through unescaped, got %q", out)
	}
}
