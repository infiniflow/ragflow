package font

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"

	gofont "github.com/xo/okage/typesetting/font"
	"github.com/xo/okage/typesetting/font/opentype"
)

// maxBitmapPixels is the number of pixels of the bitmap of a glyph that
// Bitmap decodes at most. The largest strikes of the emoji fonts have 160
// by 160 pixels.
const maxBitmapPixels = 1 << 20

// Bitmap is the bitmap of a glyph, from the table sbix, CBDT, EBDT or
// bdat of its font, such as the color glyph of an emoji.
type Bitmap struct {
	// Image is the bitmap. For a bitmap of one bit for each pixel, it is an
	// *image.Alpha, a mask that the color of the text fills.
	Image image.Image
	// Mask is set when Image is a mask.
	Mask bool
	// X and Y are the corner of the bitmap from the origin of the glyph on
	// the baseline, and W and H its size, in ems, with y down.
	X, Y, W, H float32
}

// Bitmap returns the bitmap of a glyph, and false when the font has none
// or the bitmap cannot be decoded. It decodes PNG, JPEG and the bitmaps of
// one bit for each pixel, from the largest strike of the font. The bitmaps
// are kept, and a caller must not change one.
func (f *Face) Bitmap(gid GID) (*Bitmap, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.bitmaps[gid]; ok {
		return b, b != nil
	}
	b := f.bitmap(gid)
	if f.bitmaps == nil {
		f.bitmaps = make(map[GID]*Bitmap)
	}
	f.bitmaps[gid] = b
	return b, b != nil
}

// bitmap decodes the bitmap of a glyph, or returns nil.
func (f *Face) bitmap(gid GID) (b *Bitmap) {
	// go-text can panic on a malformed font, and the decoders on a
	// malformed image (D5).
	defer func() {
		if recover() != nil {
			b = nil
		}
	}()
	data, ok := f.face.GlyphDataBitmap(opentype.GID(gid))
	if !ok || data.Width <= 0 || data.Height <= 0 || data.Width*data.Height > maxBitmapPixels {
		return nil
	}
	ext, ok := f.face.GlyphExtents(opentype.GID(gid))
	if !ok || ext.Width <= 0 || ext.Height >= 0 {
		return nil
	}
	b = &Bitmap{
		X: ext.XBearing / f.upem, Y: -ext.YBearing / f.upem,
		W: ext.Width / f.upem, H: -ext.Height / f.upem,
	}
	switch data.Format {
	case gofont.PNG, gofont.JPG:
		decode, config := png.Decode, png.DecodeConfig
		if data.Format == gofont.JPG {
			decode, config = jpeg.Decode, jpeg.DecodeConfig
		}
		c, err := config(bytes.NewReader(data.Data))
		if err != nil || c.Width <= 0 || c.Height <= 0 || c.Width*c.Height > maxBitmapPixels {
			return nil
		}
		img, err := decode(bytes.NewReader(data.Data))
		if err != nil {
			return nil
		}
		b.Image = img
	case gofont.BlackAndWhite, gofont.BlackAndWhiteByteAligned:
		stride := data.Width
		if data.Format == gofont.BlackAndWhiteByteAligned {
			stride = (data.Width + 7) / 8 * 8
		}
		if len(data.Data)*8 < stride*(data.Height-1)+data.Width {
			return nil
		}
		mask := image.NewAlpha(image.Rect(0, 0, data.Width, data.Height))
		for y := range data.Height {
			for x := range data.Width {
				bit := y*stride + x
				if data.Data[bit/8]&(0x80>>(bit%8)) != 0 {
					mask.Pix[y*mask.Stride+x] = 0xff
				}
			}
		}
		b.Image, b.Mask = mask, true
	default:
		// TIFF, which the Go library does not decode.
		return nil
	}
	return b
}
