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

// Minimal SVG-based captcha renderer.
//
// We don't have an image-captcha library vendored in go.mod and there
// is no network in build, so we render a small SVG document with the
// captcha text plus visual distortion (per-character rotation, fill
// jitter, distractor lines, dot noise). SVG is the Content-Type
// `image/svg+xml`, so it satisfies the "captcha image" contract the
// reviewer requested in PR #15290 and is renderable in any browser via
// a standard `<img>` tag.
//
// Threat model caveat: SVG is XML and the rendered text is still in
// the document, so a determined scraper can grep it. This is not as
// strong as a true raster captcha. It's a pragmatic step up from "no
// challenge at all" until a real Go captcha library is added; the
// FE renders the SVG, the human reads it, and bots that do not parse
// SVG see a noisy image. Documented as a follow-up swap when a real
// captcha library lands.
package utility

import (
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// captchaSVGWidth and captchaSVGHeight pick a viewport that fits up to
// ~6 characters comfortably while staying a tidy `<img>` size.
const (
	captchaSVGWidth  = 160
	captchaSVGHeight = 60
)

// RenderCaptchaSVG returns SVG markup that displays the captcha text
// with mild visual distortion. Output is a self-contained `<svg>`
// element suitable for embedding via a data URL or returning with a
// `Content-Type: image/svg+xml` header.
//
// rand.Rand is seeded with the current time so the same call site
// produces different noise on every invocation.
func RenderCaptchaSVG(text string) string {
	if text == "" {
		text = " "
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var b strings.Builder
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`,
		captchaSVGWidth, captchaSVGHeight, captchaSVGWidth, captchaSVGHeight,
	)
	// Background.
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#f5f5f5"/>`)

	// Distractor lines drawn behind the text.
	for i := 0; i < 4; i++ {
		x1 := rng.Intn(captchaSVGWidth)
		y1 := rng.Intn(captchaSVGHeight)
		x2 := rng.Intn(captchaSVGWidth)
		y2 := rng.Intn(captchaSVGHeight)
		stroke := pickStroke(rng)
		fmt.Fprintf(&b,
			`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="1" opacity="0.55"/>`,
			x1, y1, x2, y2, stroke,
		)
	}

	// Characters, each in its own group, rotated and jittered.
	step := captchaSVGWidth / (len(text) + 1)
	for i, r := range text {
		cx := step * (i + 1)
		cy := captchaSVGHeight/2 + rng.Intn(8) - 4
		angle := rng.Intn(40) - 20 // -20..+19 degrees
		fill := pickFill(rng)
		fmt.Fprintf(&b,
			`<text x="%d" y="%d" font-family="monospace" font-size="32" font-weight="bold" fill="%s" `+
				`text-anchor="middle" transform="rotate(%d %d %d)">%s</text>`,
			cx, cy+10, fill, angle, cx, cy+10, escapeXMLChar(r),
		)
	}

	// Dot noise drawn on top.
	for i := 0; i < 25; i++ {
		dx := rng.Intn(captchaSVGWidth)
		dy := rng.Intn(captchaSVGHeight)
		fmt.Fprintf(&b,
			`<circle cx="%d" cy="%d" r="1" fill="%s" opacity="0.5"/>`,
			dx, dy, pickStroke(rng),
		)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// RenderCaptchaSVGDataURL wraps RenderCaptchaSVG in a base64 data URL so
// the handler can return it as a single JSON string the FE drops into
// `<img src="...">`.
func RenderCaptchaSVGDataURL(text string) string {
	svg := RenderCaptchaSVG(text)
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func pickFill(rng *rand.Rand) string {
	palette := []string{"#1f2937", "#1d4ed8", "#7c2d12", "#065f46", "#7e22ce"}
	return palette[rng.Intn(len(palette))]
}

func pickStroke(rng *rand.Rand) string {
	palette := []string{"#9ca3af", "#6b7280", "#a16207", "#0e7490", "#be185d"}
	return palette[rng.Intn(len(palette))]
}

// escapeXMLChar XML-escapes a single rune. The captcha alphabet is
// [A-Z0-9] today so this is a defensive no-op for normal input, but
// guarding here means a future alphabet change cannot inject markup.
func escapeXMLChar(r rune) string {
	switch r {
	case '&':
		return "&amp;"
	case '<':
		return "&lt;"
	case '>':
		return "&gt;"
	case '"':
		return "&quot;"
	case '\'':
		return "&apos;"
	default:
		return string(r)
	}
}
