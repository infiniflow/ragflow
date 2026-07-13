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

package chunker

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
)

// concatImg vertically concatenates two images represented as raw bytes.
// Mirrors Python's rag/nlp.concat_img:
//
//   - img1 is img2 (same slice pointer) → returns img1 unchanged.
//   - one side is nil → returns the non-nil side.
//   - both nil → returns nil.
//   - both non-nil → decodes both, creates a new RGB image of
//     max(width1, width2) × (height1 + height2), pastes img1 at the top
//     and img2 below it, then re-encodes as PNG.
//
// Unlike Python's version this function works with raw image bytes rather
// than PIL Image / LazyImage objects. The LazyImage blob-list merge
// (LazyImage.merge) is not needed here because Go chunker items carry
// decoded image bytes rather than deferred-load blobs.
func concatImg(img1, img2 []byte) []byte {
	// Same-reference guard (Python: img1 is img2).
	if len(img1) > 0 && len(img2) > 0 && &img1[0] == &img2[0] {
		return img1
	}
	// Nil / empty guard (Python: img1 and not img2 → img1, etc.).
	if len(img1) == 0 && len(img2) == 0 {
		return nil
	}
	if len(img1) == 0 {
		return img2
	}
	if len(img2) == 0 {
		return img1
	}

	// Decode both images.
	dec1, _, err1 := image.Decode(bytes.NewReader(img1))
	dec2, _, err2 := image.Decode(bytes.NewReader(img2))
	if err1 != nil {
		if err2 != nil {
			return nil
		}
		return img2
	}
	if err2 != nil {
		return img1
	}

	// Pixel-data equality guard (Python: img1.tobytes() == img2.tobytes()).
	if img1data, img2data := imgBytes(dec1), imgBytes(dec2); img1data != nil && img2data != nil && bytes.Equal(img1data, img2data) {
		return img1
	}

	// Compute dimensions.
	bounds1 := dec1.Bounds()
	bounds2 := dec2.Bounds()
	w1, h1 := bounds1.Dx(), bounds1.Dy()
	w2, h2 := bounds2.Dx(), bounds2.Dy()

	newW := w1
	if w2 > newW {
		newW = w2
	}
	newH := h1 + h2

	// Create new RGBA image and paste img1 at top, img2 below.
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(dst, image.Rect(0, 0, w1, h1), dec1, bounds1.Min, draw.Over)
	draw.Draw(dst, image.Rect(0, h1, w2, h1+h2), dec2, bounds2.Min, draw.Over)

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil
	}
	return buf.Bytes()
}

// imgBytes returns the raw RGBA pixel data of img as a []byte. Returns nil
// when img is not convertible (should not happen for formats decoded by
// the Go image library, which always produce RGBA or NRGBA).
func imgBytes(img image.Image) []byte {
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	return rgba.Pix
}
