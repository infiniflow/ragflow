// Package raster decodes raster images within the limits of a configuration,
// and turns a raster image into an ivy.Page.
//
// It registers the PNG, JPEG and GIF decoders of the standard library with
// the package image. A format that needs another decoder, such as TIFF or
// WebP, imports that decoder itself, so that only it links the decoder (D6).
package raster

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"  // the GIF decoder, for image.Decode
	_ "image/jpeg" // the JPEG decoder, for image.Decode
	_ "image/png"  // the PNG decoder, for image.Decode
	"io"
	"sync"

	"github.com/xo/okage"
	"ragflow/internal/thirdparty/ivy"
)

// ReadAll reads r to its end. It returns an error that wraps ivy.ErrLimit if
// r holds more than limit bytes.
func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	buf, err := io.ReadAll(io.LimitReader(r, limit+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("reading: %w", err)
	case int64(len(buf)) > limit:
		return nil, fmt.Errorf("reading more than %d bytes: %w", limit, ivy.ErrLimit)
	}
	return buf, nil
}

// Config returns the size of the encoded image in buf, without a decode of
// its pixels.
func Config(buf []byte) (image.Point, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return image.Point{}, fmt.Errorf("reading image header: %w: %w", ivy.ErrFormat, err)
	}
	return image.Pt(cfg.Width, cfg.Height), nil
}

// Resolution returns the resolution of the JPEG or PNG file in buf, in dots
// per inch, from the density of its JFIF header or from its pHYs chunk. It
// returns false if buf gives no resolution, or a resolution outside the
// range from 1 to 10000 dots per inch.
func Resolution(buf []byte) (float64, float64, bool) {
	x, y, ok := resolution(buf)
	if !ok || !(x >= 1 && x <= 10000) || !(y >= 1 && y <= 10000) {
		return 0, 0, false
	}
	return x, y, true
}

// resolution reads the resolution of buf without a check of its range.
func resolution(buf []byte) (float64, float64, bool) {
	switch {
	case len(buf) >= 18 && buf[0] == 0xff && buf[1] == 0xd8 && buf[2] == 0xff && buf[3] == 0xe0 &&
		string(buf[6:11]) == "JFIF\x00":
		x, y := float64(binary.BigEndian.Uint16(buf[14:])), float64(binary.BigEndian.Uint16(buf[16:]))
		switch buf[13] {
		case 1:
			return x, y, true
		case 2:
			// dots per centimeter
			return x * 2.54, y * 2.54, true
		}
	case len(buf) >= 8 && string(buf[:8]) == "\x89PNG\r\n\x1a\n":
		// the chunks that come before the first IDAT chunk
		for i := 8; i+12 <= len(buf); {
			n := int(binary.BigEndian.Uint32(buf[i:]))
			typ := string(buf[i+4 : i+8])
			if n < 0 || n > len(buf)-i-12 || typ == "IDAT" {
				break
			}
			if typ == "pHYs" && n >= 9 && buf[i+16] == 1 {
				// pixels per meter
				x, y := float64(binary.BigEndian.Uint32(buf[i+8:])), float64(binary.BigEndian.Uint32(buf[i+12:]))
				return x * 0.0254, y * 0.0254, true
			}
			i += n + 12
		}
	}
	return 0, 0, false
}

// Decode decodes the encoded image in buf. It checks the size of the image
// against lim.MaxPixels before it decodes the pixels.
func Decode(ctx context.Context, buf []byte, lim ivy.Limits) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	size, err := Config(buf)
	if err != nil {
		return nil, err
	}
	if err := CheckPixels(size, lim); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("decoding image: %w: %w", ivy.ErrFormat, err)
	}
	return img, nil
}

// CheckPixels returns an error that wraps ivy.ErrLimit if an image of size
// has more pixels than lim.MaxPixels allows, and ivy.ErrFormat if the size
// is not positive.
func CheckPixels(size image.Point, lim ivy.Limits) error {
	lim = lim.WithDefaults()
	switch {
	case size.X <= 0 || size.Y <= 0:
		return fmt.Errorf("image of %dx%d: %w", size.X, size.Y, ivy.ErrFormat)
	case int64(size.X)*int64(size.Y) > lim.MaxPixels:
		return fmt.Errorf("image of %dx%d: %w", size.X, size.Y, ivy.ErrLimit)
	}
	return nil
}

// Page is an ivy.Page for a raster image. It has one point for each pixel.
// It decodes the image on its first render, and keeps it.
type Page struct {
	size image.Point
	load func(context.Context) (image.Image, error)

	mu  sync.Mutex
	pix *okage.Pixmap
}

// NewPage returns a page of the given size in pixels. load decodes the image.
func NewPage(size image.Point, load func(context.Context) (image.Image, error)) *Page {
	return &Page{size: size, load: load}
}

// Size satisfies the ivy.Page interface.
func (p *Page) Size() (float32, float32) {
	return float32(p.size.X), float32(p.size.Y)
}

// Render satisfies the ivy.Page interface. It draws the image with bilinear
// filtering.
func (p *Page) Render(ctx context.Context, dst *okage.Pixmap, t okage.Transform) error {
	pix, err := p.pixmap(ctx)
	if err != nil {
		return err
	}
	paint := okage.DefaultPixmapPaint()
	paint.Quality = okage.FilterQualityBilinear
	dst.DrawPixmap(0, 0, pix.AsRef(), &paint, t, nil)
	return nil
}

// Image returns the image of the page, with its premultiplied pixels from
// the origin, which decodes on the first call. The PDF writer of pdfenc
// draws it into the file at its own pixels (D58). raster cannot satisfy
// display.Drawer, because the display list imports raster.
func (p *Page) Image(ctx context.Context) (*image.RGBA, error) {
	pix, err := p.pixmap(ctx)
	if err != nil {
		return nil, err
	}
	w, h := int(pix.Width()), int(pix.Height())
	return &image.RGBA{Pix: pix.Data(), Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}, nil
}

// pixmap decodes the image on the first call, and converts it to a pixmap.
// A failed decode is not kept, so that a call with a new context can try
// again.
func (p *Page) pixmap(ctx context.Context) (*okage.Pixmap, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pix != nil {
		return p.pix, nil
	}
	img, err := p.load(ctx)
	if err != nil {
		return nil, err
	}
	pix, err := Pixmap(img)
	if err != nil {
		return nil, err
	}
	p.pix = pix
	return pix, nil
}

// Pixmap converts img to a pixmap, with its premultiplied pixels.
func Pixmap(img image.Image) (*okage.Pixmap, error) {
	b := img.Bounds()
	if b.Empty() {
		return nil, fmt.Errorf("empty image: %w", ivy.ErrFormat)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok || rgba.Stride != 4*b.Dx() || b.Min != (image.Point{}) {
		rgba = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	}
	size, ok := okage.IntSizeFromWH(uint32(b.Dx()), uint32(b.Dy()))
	if !ok {
		return nil, fmt.Errorf("image of %v: %w", b.Size(), ivy.ErrLimit)
	}
	pix, ok := okage.PixmapFromVec(rgba.Pix, size)
	if !ok {
		return nil, errors.New("converting image to pixmap")
	}
	return pix, nil
}
