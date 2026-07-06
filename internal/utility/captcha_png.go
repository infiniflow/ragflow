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

// Stdlib-only PNG captcha renderer.
//
// PR #15290 review (Hz-186): the previous SVG renderer embedded the
// captcha text in <text> nodes, so a scripted client could base64-
// decode the response and read the answer with a regex — defeating
// the captcha entirely. The reviewer asked for either a raster
// captcha or something that doesn't put the answer in machine-
// readable response content. We have no image-captcha library
// vendored in go.mod and no network access during build, so this
// renders a real PNG using only stdlib `image`, `image/color`,
// `image/draw`, and `image/png`, with a hand-rolled 5x7 bitmap font
// for [A-Z0-9].
//
// The output bytes contain only the raster — the captcha text is
// nowhere in the response stream — so the previous regex-the-answer
// attack is closed. An OCR-capable attacker can still solve it, but
// that's the standard limit of any non-trivial captcha; the bar set
// by the reviewer was specifically "not machine-readable in the
// response content."
package utility

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"strings"
	"time"
)

// captchaPNGScale is the per-glyph pixel multiplier. The font is 5x7,
// so a scale of 4 produces 20x28 glyphs, which are ~16x16 px after
// padding — comfortably readable for humans at typical browser zoom.
const (
	captchaPNGScale    = 4
	captchaGlyphW      = 5
	captchaGlyphH      = 7
	captchaCharSpacing = 4 // px between glyphs (after scaling)
	captchaSidePadding = 8
	captchaTopPadding  = 6
	captchaNoiseDots   = 60
	captchaNoiseLines  = 4
)

// font5x7 maps a single character to its 7-row bitmap. Each row is a
// 5-character string where '#' is a foreground pixel and any other
// character is background. Covers the captcha alphabet ([A-Z0-9])
// plus '?' as a fallback glyph for anything unexpected.
//
// These are hand-drawn — apologies for the eye-strain.
var font5x7 = map[byte][7]string{
	'A': {".###.", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'B': {"####.", "#...#", "#...#", "####.", "#...#", "#...#", "####."},
	'C': {".####", "#....", "#....", "#....", "#....", "#....", ".####"},
	'D': {"####.", "#...#", "#...#", "#...#", "#...#", "#...#", "####."},
	'E': {"#####", "#....", "#....", "####.", "#....", "#....", "#####"},
	'F': {"#####", "#....", "#....", "####.", "#....", "#....", "#...."},
	'G': {".####", "#....", "#....", "#..##", "#...#", "#...#", ".####"},
	'H': {"#...#", "#...#", "#...#", "#####", "#...#", "#...#", "#...#"},
	'I': {"#####", "..#..", "..#..", "..#..", "..#..", "..#..", "#####"},
	'J': {"#####", "...#.", "...#.", "...#.", "...#.", "#..#.", ".##.."},
	'K': {"#...#", "#..#.", "#.#..", "##...", "#.#..", "#..#.", "#...#"},
	'L': {"#....", "#....", "#....", "#....", "#....", "#....", "#####"},
	'M': {"#...#", "##.##", "#.#.#", "#...#", "#...#", "#...#", "#...#"},
	'N': {"#...#", "##..#", "#.#.#", "#.#.#", "#..##", "#...#", "#...#"},
	'O': {".###.", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'P': {"####.", "#...#", "#...#", "####.", "#....", "#....", "#...."},
	'Q': {".###.", "#...#", "#...#", "#...#", "#.#.#", "#..#.", ".##.#"},
	'R': {"####.", "#...#", "#...#", "####.", "#.#..", "#..#.", "#...#"},
	'S': {".####", "#....", "#....", ".###.", "....#", "....#", "####."},
	'T': {"#####", "..#..", "..#..", "..#..", "..#..", "..#..", "..#.."},
	'U': {"#...#", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'V': {"#...#", "#...#", "#...#", "#...#", "#...#", ".#.#.", "..#.."},
	'W': {"#...#", "#...#", "#...#", "#...#", "#.#.#", "##.##", "#...#"},
	'X': {"#...#", "#...#", ".#.#.", "..#..", ".#.#.", "#...#", "#...#"},
	'Y': {"#...#", "#...#", ".#.#.", "..#..", "..#..", "..#..", "..#.."},
	'Z': {"#####", "....#", "...#.", "..#..", ".#...", "#....", "#####"},
	'0': {".###.", "#...#", "#..##", "#.#.#", "##..#", "#...#", ".###."},
	'1': {"..#..", ".##..", "..#..", "..#..", "..#..", "..#..", ".###."},
	'2': {".###.", "#...#", "....#", "...#.", "..#..", ".#...", "#####"},
	'3': {"####.", "....#", "....#", ".###.", "....#", "....#", "####."},
	'4': {"...#.", "..##.", ".#.#.", "#..#.", "#####", "...#.", "...#."},
	'5': {"#####", "#....", "####.", "....#", "....#", "....#", "####."},
	'6': {".###.", "#....", "#....", "####.", "#...#", "#...#", ".###."},
	'7': {"#####", "....#", "....#", "...#.", "..#..", ".#...", "#...."},
	'8': {".###.", "#...#", "#...#", ".###.", "#...#", "#...#", ".###."},
	'9': {".###.", "#...#", "#...#", ".####", "....#", "....#", ".###."},
	'?': {".###.", "#...#", "....#", "...#.", "..#..", ".....", "..#.."},
}

// RenderCaptchaPNG renders the captcha text as a PNG and returns the
// raw bytes. The image has per-character jitter, random distractor
// lines, and dot noise applied — enough to defeat the trivial-regex
// attack from the previous SVG implementation. OCR-capable attackers
// remain a possibility (standard captcha limit).
//
// The output never references the original text — the answer is
// painted as raster pixels only.
func RenderCaptchaPNG(text string) []byte {
	if text == "" {
		text = " "
	}
	upper := strings.ToUpper(text)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	glyphW := captchaGlyphW * captchaPNGScale
	glyphH := captchaGlyphH * captchaPNGScale
	width := captchaSidePadding*2 + len(upper)*glyphW + (len(upper)-1)*captchaCharSpacing
	if width < 40 {
		width = 40
	}
	height := captchaTopPadding*2 + glyphH + 8 // a bit of headroom for jitter

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Background — light, slightly cool grey.
	bg := color.RGBA{R: 0xf5, G: 0xf5, B: 0xf7, A: 0xff}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Distractor lines drawn under the glyphs.
	for i := 0; i < captchaNoiseLines; i++ {
		drawLine(
			img,
			rng.Intn(width), rng.Intn(height),
			rng.Intn(width), rng.Intn(height),
			pickStrokeRGBA(rng),
		)
	}

	// Glyphs, each with x/y jitter and a per-glyph foreground colour.
	x := captchaSidePadding
	for i := 0; i < len(upper); i++ {
		ch := upper[i]
		bitmap, ok := font5x7[ch]
		if !ok {
			bitmap = font5x7['?']
		}
		dx := rng.Intn(5) - 2
		dy := rng.Intn(7) - 3
		fg := pickFillRGBA(rng)
		drawGlyph(img, x+dx, captchaTopPadding+dy, bitmap, fg)
		x += glyphW + captchaCharSpacing
		_ = i // explicit to silence any future lint pass
	}

	// Foreground dot noise on top.
	for i := 0; i < captchaNoiseDots; i++ {
		img.Set(rng.Intn(width), rng.Intn(height), pickStrokeRGBA(rng))
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// RenderCaptchaPNGDataURL base64-wraps the PNG so the handler can
// return a single JSON string the FE drops into <img src="...">.
func RenderCaptchaPNGDataURL(text string) string {
	pngBytes := RenderCaptchaPNG(text)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
}

// drawGlyph blits a 5x7 bitmap at (x, y) using captchaPNGScale x
// captchaPNGScale pixel blocks. Each '#' in the bitmap becomes a
// scale*scale block of `fg`.
func drawGlyph(img *image.RGBA, x, y int, bitmap [7]string, fg color.RGBA) {
	for row := 0; row < captchaGlyphH; row++ {
		line := bitmap[row]
		for col := 0; col < captchaGlyphW && col < len(line); col++ {
			if line[col] != '#' {
				continue
			}
			for dy := 0; dy < captchaPNGScale; dy++ {
				for dx := 0; dx < captchaPNGScale; dx++ {
					img.Set(x+col*captchaPNGScale+dx, y+row*captchaPNGScale+dy, fg)
				}
			}
		}
	}
}

// drawLine paints a 1px line using Bresenham's algorithm. Out-of-bounds
// pixels are clipped by image.RGBA.Set silently, so no bounds check
// is needed here.
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := 1
	if x0 >= x1 {
		sx = -1
	}
	sy := 1
	if y0 >= y1 {
		sy = -1
	}
	err := dx + dy
	for {
		img.Set(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func pickFillRGBA(rng *rand.Rand) color.RGBA {
	palette := []color.RGBA{
		{R: 0x1f, G: 0x29, B: 0x37, A: 0xff},
		{R: 0x1d, G: 0x4e, B: 0xd8, A: 0xff},
		{R: 0x7c, G: 0x2d, B: 0x12, A: 0xff},
		{R: 0x06, G: 0x5f, B: 0x46, A: 0xff},
		{R: 0x7e, G: 0x22, B: 0xce, A: 0xff},
	}
	return palette[rng.Intn(len(palette))]
}

func pickStrokeRGBA(rng *rand.Rand) color.RGBA {
	palette := []color.RGBA{
		{R: 0x9c, G: 0xa3, B: 0xaf, A: 0xff},
		{R: 0x6b, G: 0x72, B: 0x80, A: 0xff},
		{R: 0xa1, G: 0x62, B: 0x07, A: 0xff},
		{R: 0x0e, G: 0x74, B: 0x90, A: 0xff},
		{R: 0xbe, G: 0x18, B: 0x5d, A: 0xff},
	}
	return palette[rng.Intn(len(palette))]
}
