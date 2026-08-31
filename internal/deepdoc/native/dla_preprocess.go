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
