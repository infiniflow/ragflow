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

//go:build cgo

package native

// dla_preprocess.go — DLA preprocessing.
//
// Decodes via the shared Go image decoder and resizes with the pure-Go
// bilinearResize. This is geometrically faithful to deepdoc's cv2 pipeline but
// NOT bit-exact: the Go decoder and bilinear sampler yield slightly different
// pixels, which propagate into box coordinates (the "pure-Go floor", ~3px on
// the expanded fixtures). This is the only DLA build.

// dlaPreprocess letterboxes the image into the 1024 canvas and returns the CHW
// blob plus the scale factor. See dlaLetterbox / dlaScaleFactor for details.
func dlaPreprocess(img *Image) (blob []float32, scaleFactor [4]float32) {
	newW, newH, dw, dh := dlaGeom(img)
	bgr := img.ToBGR()
	resized := bilinearResize(bgr, img.W, img.H, newW, newH)
	return dlaLetterbox(resized, newW, newH, dw, dh), dlaScaleFactor(img, newW, newH, dw, dh)
}
