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

// Package io — HTML writer.
//
// Minimal HTML5 wrapper around the body. Header and footer are
// placed in <header> and <footer> elements; an optional watermark
// becomes a fixed-position translucent overlay. The output is a
// self-contained document — no external CSS, no JS.

package io

import (
	"bytes"
	"fmt"
	"time"
)

// HTMLOptions is the public contract for the HTML writer.
type HTMLOptions struct {
	HeaderText    string
	FooterText    string
	WatermarkText string
	AddTimestamp  bool
	FontSize      int
	FontFamily    string
	// Now overrides time.Now() for deterministic tests.
	Now time.Time
}

// WriteHTML renders content as an HTML5 document. The fontSize is
// in points; the fontFamily string is emitted as the CSS
// font-family value (a CSS-escape contract lives with the caller —
// this writer does not sanitise CSS identifiers).
func WriteHTML(content string, opts HTMLOptions) []byte {
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if opts.FontFamily == "" {
		opts.FontFamily = "sans-serif"
	}
	if opts.FontSize <= 0 {
		opts.FontSize = 11
	}
	var b bytes.Buffer
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<title>")
	if opts.HeaderText != "" {
		b.WriteString(opts.HeaderText)
	} else {
		b.WriteString("Document")
	}
	b.WriteString("</title>\n")
	b.WriteString("<style>\n")
	fmt.Fprintf(&b, "body { font-family: %q; font-size: %dpt; line-height: 1.5; }\n", opts.FontFamily, opts.FontSize)
	if opts.WatermarkText != "" {
		b.WriteString(".watermark { position: fixed; top: 50%; left: 50%; transform: translate(-50%, -50%) rotate(-30deg); font-size: 96pt; color: rgba(0,0,0,0.06); pointer-events: none; z-index: -1; }\n")
	}
	b.WriteString("</style>\n</head>\n<body>\n")
	if opts.HeaderText != "" {
		b.WriteString("<header>")
		b.WriteString(opts.HeaderText)
		b.WriteString("</header>\n")
	}
	if opts.WatermarkText != "" {
		b.WriteString("<div class=\"watermark\">")
		b.WriteString(opts.WatermarkText)
		b.WriteString("</div>\n")
	}
	b.WriteString("<main>\n")
	b.WriteString(content)
	b.WriteString("\n</main>\n")
	if opts.FooterText != "" {
		b.WriteString("<footer>")
		b.WriteString(opts.FooterText)
		b.WriteString("</footer>\n")
	}
	if opts.AddTimestamp {
		fmt.Fprintf(&b, "<p><small>Generated: %s</small></p>\n", now.Format(time.RFC3339))
	}
	b.WriteString("</body>\n</html>\n")
	return b.Bytes()
}
