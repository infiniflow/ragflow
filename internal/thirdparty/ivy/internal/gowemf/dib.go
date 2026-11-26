// Copied from github.com/mgilbir/gowemf, file dib.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/bits"
)

// ImageLimits bound encoded input and pixel output before allocation. Decoder
// temporary storage is additional to the returned image. Defaults: 16 MiB
// encoded input, 16 million pixels (64 MiB for NRGBA), and 128 MiB decoded-byte
// storage for TIFF inflation and effect working buffers.
type ImageLimits struct{ MaxBytes, MaxPixels, MaxDecodedBytes uint64 }

func (l ImageLimits) defaults() ImageLimits {
	if l.MaxDecodedBytes == 0 {
		l.MaxDecodedBytes = 128 << 20
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = 16 << 20
	}
	if l.MaxPixels == 0 {
		l.MaxPixels = 16_000_000
	}
	return l
}

// DIB is a validated bitmap layout with borrowed bytes. Its fields are private
// so callers cannot bypass the layout checks before raster decoding.
type DIB struct {
	width, height int
	topDown       bool
	bpp           uint16
	compression   uint32
	stride        int
	palette       []color.NRGBA
	masks         [4]uint32
	pixels        []byte
	limits        ImageLimits
	colorSpace    ColorSpace
}

func (d *DIB) Width() int          { return d.width }
func (d *DIB) Height() int         { return d.height }
func (d *DIB) BitCount() uint16    { return d.bpp }
func (d *DIB) Compression() uint32 { return d.compression }

// ParseDIB checks separate bitmap-info and pixel buffers, as found in EMF.
// usage is DIB_RGB_COLORS (0), DIB_PAL_COLORS (1), or DIB_PAL_INDICES (2).
// logical supplies the current logical palette for the latter two modes.
// Unsupported header/color-space/compression variants return ErrUnsupported.
func ParseDIB(info, pixels []byte, usage uint32, logical []color.NRGBA, limits ImageLimits) (*DIB, error) {
	d, _, err := parseDIB(info, pixels, false, usage, logical, limits.defaults())
	return d, err
}

// ParsePackedDIB splits a packed WMF DIB, checking its palette and pixel layout.
func ParsePackedDIB(data []byte, usage uint32, logical []color.NRGBA, limits ImageLimits) (*DIB, error) {
	d, _, err := parseDIB(data, nil, true, usage, logical, limits.defaults())
	return d, err
}

func parseDIB(info, pixels []byte, packed bool, usage uint32, logical []color.NRGBA, l ImageLimits) (*DIB, int, error) {
	bad := func(field string) (*DIB, int, error) { return nil, 0, malformed(0, "DIB "+field) }
	if uint64(len(info))+uint64(len(pixels)) > l.MaxBytes {
		return nil, 0, failure(0, "DIB bytes", ErrLimit)
	}
	if len(info) < 4 {
		return bad("header")
	}
	hs := uint64(u32(info))
	if hs != 12 && hs != 40 && hs != 108 && hs != 124 {
		return nil, 0, failure(0, "DIB header version", ErrUnsupported)
	}
	if hs > uint64(len(info)) {
		return bad("header size")
	}
	d := &DIB{limits: l}
	var w, h int64
	var planes uint16
	var used, imageSize uint32
	paletteWidth := 4
	if hs == 12 {
		w, h = int64(u16(info[4:])), int64(u16(info[6:]))
		planes = u16(info[8:])
		d.bpp = u16(info[10:])
		paletteWidth = 3
	} else {
		w, h = int64(int32(u32(info[4:]))), int64(int32(u32(info[8:])))
		planes = u16(info[12:])
		d.bpp = u16(info[14:])
		d.compression = u32(info[16:])
		imageSize = u32(info[20:])
		used = u32(info[32:])
	}
	if w <= 0 || h == 0 || planes != 1 {
		return bad("dimensions or planes")
	}
	d.topDown = h < 0
	if h < 0 {
		h = -h
	}
	if uint64(w)*uint64(h) > l.MaxPixels || uint64(w)*uint64(h) > uint64(int(^uint(0)>>1))/4 {
		return nil, 0, failure(0, "DIB pixel count", ErrLimit)
	}
	d.width, d.height = int(w), int(h)
	if usage > 2 {
		return bad("color usage")
	}
	if d.topDown && d.compression != 0 && d.compression != 3 {
		return bad("top-down compression")
	}
	space, profileStart, profileEnd, err := dibColorSpace(info, hs)
	if err != nil {
		return nil, 0, err
	}
	d.colorSpace = space
	if d.compression == 4 || d.compression == 5 {
		// PNG can produce 16-bit/channel images (8 bytes/pixel). Enforce our
		// native-index bound before entering a codec, independent of its own
		// dimension-overflow checks and even when MaxPixels is explicitly raised.
		if uint64(w)*uint64(h) > uint64(int(^uint(0)>>1))/8 {
			return nil, 0, failure(0, "compressed DIB pixel storage", ErrLimit)
		}
		if d.bpp != 0 || usage != 0 || used != 0 {
			return bad("compressed image format")
		}
		if packed {
			pixels = info[int(hs):]
		}
		if imageSize == 0 || uint64(imageSize) > uint64(len(pixels)) {
			return bad("compressed image size")
		}
		if packed && profileOverlapsPixels(profileStart, profileEnd, hs, uint64(imageSize)) {
			return bad("profile overlaps encoded pixels")
		}
		d.pixels = pixels[:int(imageSize):int(imageSize)]
		cfg, err := d.compressedConfig()
		if err != nil {
			return nil, 0, err
		}
		if cfg.Width != d.width || cfg.Height != d.height {
			return bad("compressed image dimensions")
		}
		return d, int(hs), nil
	}
	if d.compression > 3 {
		return nil, 0, failure(16, "DIB compression", ErrUnsupported)
	}
	switch d.bpp {
	case 1, 4, 8, 16, 24, 32:
	default:
		return bad("bit count")
	}
	if d.compression == 3 && (d.bpp != 16 && d.bpp != 32) {
		return bad("bitfield depth")
	}
	if (d.compression == 1 && d.bpp != 8) || (d.compression == 2 && d.bpp != 4) {
		return bad("RLE bit count")
	}
	pos := int(hs)
	if d.compression == 3 {
		mpos := 40
		if hs == 40 {
			if len(info) < 52 {
				return bad("bitfields")
			}
			pos = 52
		}
		for i := 0; i < 3; i++ {
			d.masks[i] = u32(info[mpos+i*4:])
		}
		if hs >= 108 {
			d.masks[3] = u32(info[52:])
		}
	} else if d.bpp == 16 {
		d.masks = [4]uint32{0x7c00, 0x3e0, 0x1f, 0}
	} else if d.bpp == 32 {
		d.masks = [4]uint32{0xff0000, 0xff00, 0xff, 0}
		if hs >= 108 {
			d.masks[3] = u32(info[52:])
		}
	}
	if d.bpp == 16 || d.bpp == 32 {
		var union uint32
		for i, m := range d.masks {
			if m == 0 {
				if i < 3 {
					return bad("empty color mask")
				}
				continue
			}
			shifted := m >> bits.TrailingZeros32(m)
			if shifted&(shifted+1) != 0 || m&union != 0 || (d.bpp == 16 && m > 0xffff) {
				return bad("overlapping or non-contiguous masks")
			}
			union |= m
		}
	}
	count := uint64(used)
	if d.bpp <= 8 {
		max := uint64(1) << d.bpp
		if count == 0 || count > max {
			count = max
		}
	}
	if usage == 1 {
		paletteWidth = 2
	}
	if usage == 2 {
		count = 0
	}
	// ivy: a packed DIB of 8 bits or less whose pixels fill the record
	// without its color table, as PowerPoint writes in the WMF thumbnail of
	// a presentation, takes the colors of the selected logical palette.
	noTable := false
	if packed && usage == 0 && d.bpp <= 8 && used == 0 && d.compression == 0 && len(logical) > 0 {
		stride := ((uint64(w)*uint64(d.bpp) + 31) / 32) * 4
		if uint64(len(info)-pos) == stride*uint64(h) {
			noTable, count = true, 0
		}
	}
	if count > uint64(len(info)-pos)/uint64(paletteWidth) {
		return bad("palette length")
	}
	if profileEnd != 0 && profileStart < uint64(pos)+count*uint64(paletteWidth) {
		return bad("profile overlaps color table")
	}
	if d.bpp <= 8 {
		if noTable {
			// ivy: an index past the logical palette is black
			d.palette = make([]color.NRGBA, 1<<d.bpp)
			for i := range d.palette {
				d.palette[i] = color.NRGBA{A: 255}
			}
			copy(d.palette, logical)
		} else if usage == 2 {
			if len(logical) == 0 || len(logical) > 256 {
				return bad("logical palette")
			}
			d.palette = append([]color.NRGBA(nil), logical...)
		} else {
			d.palette = make([]color.NRGBA, int(count))
			for i := range d.palette {
				b := info[pos+i*paletteWidth:]
				if usage == 1 {
					index := int(u16(b))
					if index >= len(logical) {
						return bad("logical palette index")
					}
					d.palette[i] = logical[index]
				} else {
					d.palette[i] = color.NRGBA{R: b[2], G: b[1], B: b[0], A: 255}
				}
			}
		}
	}
	pos += int(count) * paletteWidth
	if packed {
		pixels = info[pos:]
	}
	if d.compression == 1 || d.compression == 2 {
		if imageSize == 0 || uint64(imageSize) > uint64(len(pixels)) {
			return bad("RLE image size")
		}
		if packed && profileOverlapsPixels(profileStart, profileEnd, uint64(pos), uint64(imageSize)) {
			return bad("profile overlaps RLE pixels")
		}
		d.pixels = pixels[:int(imageSize):int(imageSize)]
		if err := d.rle(nil); err != nil {
			return nil, 0, err
		}
		return d, pos, nil
	}
	stride := ((uint64(w)*uint64(d.bpp) + 31) / 32) * 4
	needed := stride * uint64(h)
	if needed > uint64(len(pixels)) {
		return bad("pixel buffer")
	}
	if packed && profileOverlapsPixels(profileStart, profileEnd, uint64(pos), needed) {
		return bad("profile overlaps pixels")
	}
	d.stride = int(stride)
	d.pixels = pixels[:int(needed):int(needed)]
	return d, pos, nil
}

func (d *DIB) compressedConfig() (image.Config, error) {
	if d.compression == 4 {
		return jpeg.DecodeConfig(bytes.NewReader(d.pixels))
	}
	return png.DecodeConfig(bytes.NewReader(d.pixels))
}

// Image allocates and decodes the checked DIB. RGB32's unused high byte is
// opaque, not alpha. Bitfield alpha is straight (non-premultiplied). A zero DIB
// is invalid. Borrowed input bytes must remain unchanged after parsing.
func (d *DIB) Image() (image.Image, error) {
	return d.ImageWithColorTransform(nil)
}

func (d *DIB) rawImage() (image.Image, error) {
	if d == nil || d.width <= 0 || d.height <= 0 {
		return nil, malformed(0, "uninitialized DIB")
	}
	if d.compression == 4 || d.compression == 5 {
		// Recheck dimensions before invoking decoders, including if a caller has
		// violated the immutable-input contract between ParseDIB and Image.
		cfg, err := d.compressedConfig()
		if err != nil {
			return nil, err
		}
		if cfg.Width != d.width || cfg.Height != d.height {
			return nil, malformed(0, "changed DIB dimensions")
		}
		if d.compression == 4 {
			return jpeg.Decode(bytes.NewReader(d.pixels))
		}
		return png.Decode(bytes.NewReader(d.pixels))
	}
	out := image.NewNRGBA(image.Rect(0, 0, d.width, d.height))
	if d.compression == 1 || d.compression == 2 {
		zero := d.palette[0]
		for i := 0; i < len(out.Pix); i += 4 {
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = zero.R, zero.G, zero.B, zero.A
		}
		if err := d.rle(out); err != nil {
			return nil, err
		}
		return out, nil
	}
	for y := 0; y < d.height; y++ {
		sy := y
		if !d.topDown {
			sy = d.height - 1 - y
		}
		row := d.pixels[sy*d.stride : (sy+1)*d.stride]
		for x := 0; x < d.width; x++ {
			var col color.NRGBA
			switch d.bpp {
			case 1, 4, 8:
				var index byte
				if d.bpp == 1 {
					index = row[x/8] >> uint(7-x%8) & 1
				} else if d.bpp == 4 {
					index = row[x/2] >> uint(4*(1-x%2)) & 15
				} else {
					index = row[x]
				}
				if int(index) >= len(d.palette) {
					return nil, malformed(sy*d.stride, "DIB pixel palette index")
				}
				col = d.palette[index]
			case 24:
				p := row[x*3:]
				col = color.NRGBA{R: p[2], G: p[1], B: p[0], A: 255}
			case 16, 32:
				var v uint32
				if d.bpp == 16 {
					v = uint32(u16(row[x*2:]))
				} else {
					v = u32(row[x*4:])
				}
				col = color.NRGBA{maskChannel(v, d.masks[0]), maskChannel(v, d.masks[1]), maskChannel(v, d.masks[2]), 255}
				if d.masks[3] != 0 {
					col.A = maskChannel(v, d.masks[3])
				}
			}
			i := y*out.Stride + x*4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = col.R, col.G, col.B, col.A
		}
	}
	return out, nil
}
func maskChannel(v, m uint32) byte {
	shift := bits.TrailingZeros32(m)
	max := uint64(m >> shift)
	return byte((uint64((v&m)>>shift)*255 + max/2) / max)
}

// monoBits returns the pixel indexes of an uncompressed 1-bit DIB in top-down
// order, for monochrome pattern brushes whose colors come from the playback
// device context rather than the DIB color table.
func (d *DIB) monoBits() ([]bool, error) {
	if d == nil || d.bpp != 1 || d.compression != 0 {
		return nil, failure(0, "monochrome pattern bitmap format", ErrUnsupported)
	}
	out := make([]bool, d.width*d.height)
	for y := 0; y < d.height; y++ {
		sy := y
		if !d.topDown {
			sy = d.height - 1 - y
		}
		row := d.pixels[sy*d.stride : (sy+1)*d.stride]
		for x := 0; x < d.width; x++ {
			out[y*d.width+x] = row[x/8]>>uint(7-x%8)&1 != 0
		}
	}
	return out, nil
}
