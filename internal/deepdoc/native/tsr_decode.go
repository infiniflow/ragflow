//go:build cgo

package native

// tsr_decode.go — TSR preprocessing.
//
// TSR decodes through the shared Go image decoder (image.go's Decode) and
// resizes with the pure-Go bilinearResize.
//
// Why: deepdoc's production TSR adapter (deepdoc/server/adapters/tsr_adapter.py)
// decodes the crop with PIL (Image.open(...).convert("RGB")), not cv2. Go's
// image decoder lands closer to PIL than OpenCV's JPEG decoder does, so the
// pure-Go decode matches the production TSR baseline within the ~3px parity
// tolerance on real-table fixtures. The cv2 decode used by the DLA/DET/OCR
// paths instead diverges from PIL by ~1px and, on boundary boxes, can flip a
// structural box by tens of pixels.
//
// The CHW assembly is shared via tsrBlob; ref_tsr.py feeds the model BGR, which
// is exactly what img.ToBGR() yields from the Go-decoded RGB pixels.

func tsrPreprocess(img *Image) (blob []float32, scaleFactor [2]float32) {
	bgr := img.ToBGR()
	resized := bilinearResize(bgr, img.W, img.H, tsrInputSize, tsrInputSize)
	out := tsrBlob(resized)
	return out, tsrScaleFactor(img)
}
