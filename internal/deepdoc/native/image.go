//go:build cgo

package native

// image.go — format-agnostic image decoding shared by all recognizers.
//
// DeepDoc's models all consume BGR-ordered pixels (DLA/TSR reach it via a
// cv2.cvtColor(rgb, BGR2RGB) swap on a PIL RGB image; the OCR models are fed
// cv2-decoded BGR directly). Decoding therefore yields RGB and callers ask for
// BGR via ToBGR() — one consistent path, no per-task decode duplication.
//
// Decode uses image.Decode (format auto-detection) rather than a hard-coded
// jpeg.Decode so the comparison tool mirrors the Python service, which
// decodes whatever bytes it receives (PIL for DLA/TSR, cv2.imdecode for OCR)
// and is format-agnostic. The comparison tool must be able to load the same
// formats production sends — chiefly PNG, since the Go inference client
// transport now encodes pages/crops as lossless PNG.

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// Decode bounds guard against decompression bombs. The comparison tool only
// decodes fixed fixtures today, but if it ever ingests untrusted input the
// image/* decoders perform no size check of their own, so we cap dimensions
// and total pixels before materializing the raster.
const (
	maxImageDim    = 16384
	maxImagePixels = 100_000_000 // 100 MP
)

// Image is a decoded raster in R,G,B byte order, row-major (len H*W*3).
type Image struct {
	W, H int
	Pix  []byte
}

// checkImageBounds rejects empty, oversized, or decompression-bomb rasters.
func checkImageBounds(b image.Rectangle) error {
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return fmt.Errorf("native: empty image")
	}
	if b.Dx() > maxImageDim || b.Dy() > maxImageDim {
		return fmt.Errorf("native: image dimension %dx%d exceeds cap %d",
			b.Dx(), b.Dy(), maxImageDim)
	}
	if px := int64(b.Dx()) * int64(b.Dy()); px > maxImagePixels {
		return fmt.Errorf("native: image has %d pixels, exceeds cap %d", px, maxImagePixels)
	}
	return nil
}

// Decode reads an image file (any format Go's image package can decode,
// e.g. PNG/JPEG) and returns it as RGB pixels. Format is auto-detected,
// matching the Python service's format-agnostic decode.
func Decode(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	if err := checkImageBounds(img.Bounds()); err != nil {
		return nil, err
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	pix := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := rgba.PixOffset(x, y)
			d := (y*w + x) * 3
			pix[d] = rgba.Pix[o]     // R
			pix[d+1] = rgba.Pix[o+1] // G
			pix[d+2] = rgba.Pix[o+2] // B
		}
	}
	return &Image{W: w, H: h, Pix: pix}, nil
}

// FromImage converts a standard library image.Image into the RGB row-major
// raster the recognizers consume. It is the in-process analogue of Decode
// (which reads from a file): the Go DeepDoc client and the PDF parser already
// hold an image.Image, so decoding to disk and back is unnecessary.
func FromImage(img image.Image) (*Image, error) {
	b := img.Bounds()
	if err := checkImageBounds(b); err != nil {
		return nil, err
	}
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, img, b.Min, draw.Src)
	w, h := b.Dx(), b.Dy()
	pix := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := rgba.PixOffset(x, y)
			d := (y*w + x) * 3
			pix[d] = rgba.Pix[o]     // R
			pix[d+1] = rgba.Pix[o+1] // G
			pix[d+2] = rgba.Pix[o+2] // B
		}
	}
	return &Image{W: w, H: h, Pix: pix}, nil
}

// FromImages converts a slice of standard library images into the RGB
// row-major rasters the recognizers consume, in order. It is the batched
// analogue of FromImage used by RunOCRRecBatchReal so a page's crops are
// converted once before the single forward pass. A bounds failure on any
// element aborts the whole batch (the recognizer cannot resize a degenerate
// raster), matching FromImage's all-or-nothing contract.
func FromImages(imgs []image.Image) ([]*Image, error) {
	out := make([]*Image, len(imgs))
	for i, img := range imgs {
		ni, err := FromImage(img)
		if err != nil {
			return nil, err
		}
		out[i] = ni
	}
	return out, nil
}

// ToBGR returns a copy of the pixels with R and B channels swapped (B,G,R
// order), which is the channel order every DeepDoc ONNX expects.
func (im *Image) ToBGR() []byte {
	bgr := make([]byte, len(im.Pix))
	for i := 0; i < len(im.Pix); i += 3 {
		bgr[i] = im.Pix[i+2]
		bgr[i+1] = im.Pix[i+1]
		bgr[i+2] = im.Pix[i]
	}
	return bgr
}
