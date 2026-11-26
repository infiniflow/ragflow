// Copied from github.com/mgilbir/gowemf, file image.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
)

// AlphaImage interprets a 32-bit RGB or standard BGR-bitfield DIB as premultiplied BGRA, as required
// by EMR_ALPHABLEND with AC_SRC_ALPHA. Image instead treats BI_RGB's fourth byte
// as unused/opaque. SourceConstantAlpha and destination compositing are left to
// the renderer. Channels greater than alpha are rejected as non-premultiplied.
func (d *DIB) AlphaImage() (*image.RGBA, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	if d != nil && !d.colorSpace.IsSRGB() {
		return nil, failure(0, "alpha DIB color conversion", ErrUnsupported)
	}
	return d.rawAlphaImage()
}

func (d *DIB) rawAlphaImage() (*image.RGBA, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	standardFields := d.compression == 3 && d.masks[0] == 0xff0000 && d.masks[1] == 0xff00 && d.masks[2] == 0xff && (d.masks[3] == 0 || d.masks[3] == 0xff000000)
	if d.bpp != 32 || (d.compression != 0 && !standardFields) {
		return nil, failure(0, "alpha DIB layout", ErrUnsupported)
	}
	out := image.NewRGBA(image.Rect(0, 0, d.width, d.height))
	for y := 0; y < d.height; y++ {
		sy := y
		if !d.topDown {
			sy = d.height - 1 - y
		}
		row := d.pixels[sy*d.stride : (sy+1)*d.stride]
		for x := 0; x < d.width; x++ {
			p := row[x*4:]
			if p[0] > p[3] || p[1] > p[3] || p[2] > p[3] {
				return nil, malformed(sy*d.stride+x*4, "non-premultiplied alpha DIB")
			}
			i := y*out.Stride + x*4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = p[2], p[1], p[0], p[3]
		}
	}
	return out, nil
}

// Image decodes EMF+ PNG/JPEG/GIF and all defined raw pixel formats. GIF returns
// the first image on its logical canvas; subsequent animation is not decoded. Extended
// channels retain 16-bit precision in Gray16, NRGBA64 or RGBA64 outputs. Indexed
// colors retain palette alpha; premultiplied formats return RGBA/RGBA64.
// All public fields are revalidated, including caller-constructed values.
func (p PlusImage) Image(limits ImageLimits) (image.Image, error) {
	l := limits.defaults()
	if uint64(len(p.Data)) > l.MaxBytes {
		return nil, failure(0, "EMF+ image bytes", ErrLimit)
	}
	if p.Type != 1 {
		return nil, failure(0, "EMF+ non-bitmap image", ErrUnsupported)
	}
	if p.BitmapType == 1 {
		return decodeEncodedImage(p.Data, l)
	}
	if p.BitmapType != 0 {
		return nil, malformed(0, "EMF+ bitmap data type")
	}
	return p.rawBitmapImage(l)
}

// ImageWithColorTransform adds explicit ICC handling for TIFF payloads. Other
// encodings retain Image's behavior; no metafile-provided profile paths are opened.
func (p PlusImage) ImageWithColorTransform(limits ImageLimits, factory ColorTransformFactory) (image.Image, error) {
	if p.Type == 1 && p.BitmapType == 1 && len(p.Data) >= 4 && (string(p.Data[:2]) == "II" || string(p.Data[:2]) == "MM") {
		t, err := ParseTIFF(p.Data, limits)
		if err != nil {
			return nil, err
		}
		return t.ImageWithColorTransform(factory)
	}
	return p.Image(limits)
}

func decodeEncodedImage(data []byte, l ImageLimits) (image.Image, error) {
	if len(data) >= 4 && (string(data[:2]) == "II" || string(data[:2]) == "MM") {
		t, err := ParseTIFF(data, l)
		if err != nil {
			return nil, err
		}
		return t.Image()
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return decodeGIFImage(data, l)
	}
	var cfg image.Config
	var err error
	isPNG := len(data) >= 8 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10})
	isJPEG := len(data) >= 2 && data[0] == 255 && data[1] == 216
	if isPNG {
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	} else if isJPEG {
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	} else {
		return nil, failure(0, "compressed EMF+ image encoding", ErrUnsupported)
	}
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || uint64(cfg.Width) > l.MaxPixels/uint64(cfg.Height) || uint64(cfg.Width) > uint64(int(^uint(0)>>1))/8/uint64(cfg.Height) {
		return nil, failure(0, "compressed EMF+ image pixels", ErrLimit)
	}
	if isPNG {
		return png.Decode(bytes.NewReader(data))
	}
	return jpeg.Decode(bytes.NewReader(data))
}
