// Copied from github.com/mgilbir/gowemf, file plus_pixels.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"encoding/binary"
	"image"
)

// Pixel formats from MS-EMFPLUS 2.1.1.24. Channel order in raw data is BGR(A),
// with little-endian WORD channels for extended formats. Indexed pixels are
// most-significant-bit/nibble first; palette entries are little-endian ARGB.
const (
	PixelFormatUndefined      uint32 = 0
	PixelFormat1bppIndexed    uint32 = 0x30101
	PixelFormat4bppIndexed    uint32 = 0x30402
	PixelFormat8bppIndexed    uint32 = 0x30803
	PixelFormat16bppGrayScale uint32 = 0x101004
	PixelFormat16bppRGB555    uint32 = 0x21005
	PixelFormat16bppRGB565    uint32 = 0x21006
	PixelFormat16bppARGB1555  uint32 = 0x61007
	PixelFormat24bppRGB       uint32 = 0x21808
	PixelFormat32bppRGB       uint32 = 0x22009
	PixelFormat32bppARGB      uint32 = 0x26200a
	PixelFormat32bppPARGB     uint32 = 0xe200b
	PixelFormat48bppRGB       uint32 = 0x10300c
	PixelFormat64bppARGB      uint32 = 0x34400d
	PixelFormat64bppPARGB     uint32 = 0x1a400e
)

type plusPixelInfo struct {
	bits, outputBytes            uint64
	indexed, premult, wide, gray bool
}

func pixelInfo(format uint32) (plusPixelInfo, error) {
	f := plusPixelInfo{bits: uint64(format >> 8 & 255), outputBytes: 4}
	switch format {
	case PixelFormat1bppIndexed, PixelFormat4bppIndexed, PixelFormat8bppIndexed:
		f.indexed = true
	case PixelFormat16bppRGB555, PixelFormat16bppRGB565, PixelFormat16bppARGB1555, PixelFormat24bppRGB, PixelFormat32bppRGB, PixelFormat32bppARGB:
	case PixelFormat32bppPARGB:
		f.premult = true
	case PixelFormat16bppGrayScale:
		f.gray = true
		f.outputBytes = 2
	case PixelFormat48bppRGB, PixelFormat64bppARGB:
		f.wide = true
		f.outputBytes = 8
	case PixelFormat64bppPARGB:
		f.wide = true
		f.premult = true
		f.outputBytes = 8
	default:
		return f, failure(0, "EMF+ pixel format", ErrUnsupported)
	}
	return f, nil
}

type plusBitmapLayout struct {
	format          plusPixelInfo
	stride          int
	palette, pixels []byte
}

// Palette bytes must never count as pixel storage. This shared validator runs
// both when decoding an image object and for caller-constructed PlusImage values.
func (p PlusImage) bitmapLayout(maxPaletteEntries uint64) (plusBitmapLayout, error) {
	f, err := pixelInfo(p.PixelFormat)
	if err != nil {
		return plusBitmapLayout{}, err
	}
	l := plusBitmapLayout{format: f}
	if p.Width <= 0 || p.Height <= 0 || p.Stride == 0 || p.Stride%4 != 0 {
		return l, malformed(0, "EMF+ bitmap dimensions/stride")
	}
	stride := int64(p.Stride)
	if stride < 0 {
		stride = -stride
	}
	if (uint64(p.Width)*f.bits+7)/8 > uint64(stride) {
		return l, malformed(0, "EMF+ bitmap row")
	}
	data := p.Data
	if f.indexed {
		if len(data) < 8 {
			return l, malformed(0, "EMF+ palette header")
		}
		flags, count := u32(data), uint64(u32(data[4:]))
		if flags & ^uint32(7) != 0 || count == 0 {
			return l, malformed(0, "EMF+ palette flags/count")
		}
		if count > maxPaletteEntries {
			return l, failure(4, "EMF+ palette entries", ErrLimit)
		}
		if count > uint64(len(data)-8)/4 {
			return l, malformed(4, "EMF+ palette data")
		}
		end := 8 + int(count)*4
		l.palette = data[8:end:end]
		data = data[end:]
		if flags&2 != 0 {
			for off := 0; off < len(l.palette); off += 4 {
				v := l.palette[off:]
				if v[0] != v[1] || v[1] != v[2] {
					return l, malformed(8+off, "non-grayscale palette entry")
				}
			}
		}
	}
	needed := uint64(stride) * uint64(p.Height)
	if needed > uint64(len(data)) {
		return l, malformed(0, "EMF+ bitmap pixels")
	}
	// A positive height plus the byte-span check also bounds stride on 32-bit.
	l.stride = int(stride)
	l.pixels = data[:int(needed):int(needed)]
	return l, nil
}

func (p PlusImage) rawBitmapImage(l ImageLimits) (image.Image, error) {
	f, err := pixelInfo(p.PixelFormat)
	if err != nil {
		return nil, err
	}
	if p.Width <= 0 || p.Height <= 0 {
		return nil, malformed(0, "EMF+ image dimensions")
	}
	w, h := uint64(p.Width), uint64(p.Height)
	if w > l.MaxPixels/h || w > uint64(int(^uint(0)>>1))/f.outputBytes/h {
		return nil, failure(0, "EMF+ image pixels", ErrLimit)
	}
	layout, err := p.bitmapLayout(l.MaxBytes / 4)
	if err != nil {
		return nil, err
	}
	rect := image.Rect(0, 0, int(w), int(h))
	var result image.Image
	var dst []byte
	var stride int
	switch {
	case f.gray:
		im := image.NewGray16(rect)
		result, dst, stride = im, im.Pix, im.Stride
	case f.wide && f.premult:
		im := image.NewRGBA64(rect)
		result, dst, stride = im, im.Pix, im.Stride
	case f.wide:
		im := image.NewNRGBA64(rect)
		result, dst, stride = im, im.Pix, im.Stride
	case f.premult:
		im := image.NewRGBA(rect)
		result, dst, stride = im, im.Pix, im.Stride
	default:
		im := image.NewNRGBA(rect)
		result, dst, stride = im, im.Pix, im.Stride
	}
	for y := 0; y < int(h); y++ {
		sy := y
		if p.Stride < 0 {
			sy = int(h) - 1 - y
		}
		row := layout.pixels[sy*layout.stride : (sy+1)*layout.stride]
		out := dst[y*stride : (y+1)*stride]
		for x := 0; x < int(w); x++ {
			if f.gray {
				binary.BigEndian.PutUint16(out[x*2:], u16(row[x*2:]))
				continue
			}
			if f.wide {
				in := row[x*int(f.bits/8):]
				b, g, r, a := u16(in), u16(in[2:]), u16(in[4:]), uint16(65535)
				if f.bits == 64 {
					a = u16(in[6:])
				}
				if f.premult && (r > a || g > a || b > a) {
					return nil, malformed(sy*layout.stride, "non-premultiplied EMF+ 64-bit pixel")
				}
				pixel := out[x*8:]
				binary.BigEndian.PutUint16(pixel, r)
				binary.BigEndian.PutUint16(pixel[2:], g)
				binary.BigEndian.PutUint16(pixel[4:], b)
				binary.BigEndian.PutUint16(pixel[6:], a)
				continue
			}
			var r, g, b byte
			a := byte(255)
			switch {
			case f.indexed:
				var index byte
				if f.bits == 1 {
					index = row[x/8] >> uint(7-x%8) & 1
				} else if f.bits == 4 {
					index = row[x/2] >> uint(4*(1-x%2)) & 15
				} else {
					index = row[x]
				}
				if int(index) >= len(layout.palette)/4 {
					return nil, malformed(sy*layout.stride, "EMF+ pixel palette index")
				}
				color := layout.palette[int(index)*4:]
				b, g, r, a = color[0], color[1], color[2], color[3]
			case f.bits == 16:
				v := uint32(u16(row[x*2:]))
				red, green := uint32(0x7c00), uint32(0x3e0)
				if p.PixelFormat == PixelFormat16bppRGB565 {
					red, green = 0xf800, 0x7e0
				}
				r, g, b = maskChannel(v, red), maskChannel(v, green), maskChannel(v, 0x1f)
				if p.PixelFormat == PixelFormat16bppARGB1555 {
					a = byte(v>>15) * 255
				}
			default:
				in := row[x*int(f.bits/8):]
				b, g, r = in[0], in[1], in[2]
				if p.PixelFormat == PixelFormat32bppARGB || f.premult {
					a = in[3]
				}
			}
			if f.premult && (r > a || g > a || b > a) {
				return nil, malformed(sy*layout.stride+x*4, "non-premultiplied EMF+ bitmap")
			}
			i := x * 4
			out[i], out[i+1], out[i+2], out[i+3] = r, g, b, a
		}
	}
	return result, nil
}
