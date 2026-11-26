// Copied from github.com/mgilbir/gowemf, file tiff.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"encoding/binary"
	"image"
	"image/color"
	"math/bits"
)

// TIFF is a bounded view of the first classic-TIFF IFD. Subsequent pages,
// SubIFDs and private pointer tags are not traversed. Profiles remain borrowed
// metadata; Image refuses to silently discard an embedded ICC profile.
type TIFF struct {
	data                                                                                               []byte
	order                                                                                              binary.ByteOrder
	limits                                                                                             ImageLimits
	width, height, samples, depth, photo, compression, rows, planar, predictor, orientation, fillOrder uint64
	alpha                                                                                              int
	associated                                                                                         bool
	offsets, counts, palette                                                                           tiffField
	profile                                                                                            []byte
	rowBytes, planeBytes, totalBytes                                                                   uint64
}
type tiffField struct {
	data  []byte
	count uint64
	kind  uint16
	order binary.ByteOrder
}

func (f tiffField) at(i uint64) uint64 {
	switch f.kind {
	case 1:
		return uint64(f.data[int(i)])
	case 3:
		return uint64(f.order.Uint16(f.data[int(i)*2:]))
	case 4:
		return uint64(f.order.Uint32(f.data[int(i)*4:]))
	}
	return 0
}
func (t *TIFF) Width() int {
	if t.orientation >= 5 {
		return int(t.height)
	}
	return int(t.width)
}
func (t *TIFF) Height() int {
	if t.orientation >= 5 {
		return int(t.width)
	}
	return int(t.height)
}
func (t *TIFF) ColorSpace() ColorSpace {
	if len(t.profile) == 0 {
		return ColorSpace{Type: ColorSRGB}
	}
	return ColorSpace{Type: ColorProfileEmbedded, Profile: t.profile}
}

// ParseTIFF supports classic II/MM TIFF with stripped unsigned gray, palette or
// RGB pixels, contiguous/separate samples, 1/4/8/16-bit uniform samples,
// orientations 1–8, and uncompressed, PackBits, LZW or Deflate strips. BigTIFF,
// tiled/CCITT/JPEG/YCbCr layouts return ErrUnsupported. Metadata is capped at
// 4096 IFD entries and samples at 16; expansion is bounded before allocation.
func ParseTIFF(data []byte, limits ImageLimits) (*TIFF, error) {
	l := limits.defaults()
	if uint64(len(data)) > l.MaxBytes {
		return nil, failure(0, "TIFF bytes", ErrLimit)
	}
	if len(data) < 8 {
		return nil, malformed(0, "TIFF header")
	}
	t := &TIFF{data: data, limits: l, alpha: -1}
	switch string(data[:2]) {
	case "II":
		t.order = binary.LittleEndian
	case "MM":
		t.order = binary.BigEndian
	default:
		return nil, failure(0, "TIFF byte order", ErrFormat)
	}
	if t.order.Uint16(data[2:]) != 42 {
		return nil, failure(2, "TIFF version", ErrUnsupported)
	}
	off := uint64(t.order.Uint32(data[4:]))
	if off < 8 || off%2 != 0 || off > uint64(len(data))-2 {
		return nil, malformed(4, "TIFF first IFD")
	}
	n := uint64(t.order.Uint16(data[int(off):]))
	if n == 0 {
		return nil, malformed(int(off), "empty TIFF IFD")
	}
	if n > 4096 {
		return nil, failure(int(off), "TIFF IFD entries", ErrLimit)
	}
	if n*12+6 > uint64(len(data))-off {
		return nil, malformed(int(off), "TIFF IFD span")
	}
	fields := make(map[uint16]tiffField)
	for i := uint64(0); i < n; i++ {
		entry := data[int(off+2+i*12):]
		tag, kind, count := t.order.Uint16(entry), t.order.Uint16(entry[2:]), uint64(t.order.Uint32(entry[4:]))
		switch tag {
		case 256, 257, 258, 259, 262, 266, 273, 274, 277, 278, 279, 284, 317, 320, 322, 323, 324, 325, 338, 339, 34675:
		default:
			continue
		}
		if _, exists := fields[tag]; exists {
			return nil, malformed(int(off+2+i*12), "duplicate TIFF field")
		}
		var size uint64
		switch kind {
		case 1, 7:
			size = 1
		case 3:
			size = 2
		case 4:
			size = 4
		default:
			return nil, failure(int(off+2+i*12), "TIFF field type", ErrUnsupported)
		}
		bytes := count * size
		start := off + 2 + i*12 + 8
		if bytes > 4 {
			start = uint64(t.order.Uint32(entry[8:]))
			if start%2 != 0 {
				return nil, malformed(int(off+2+i*12), "TIFF field alignment")
			}
		}
		if start > uint64(len(data)) || bytes > uint64(len(data))-start {
			return nil, malformed(int(off+2+i*12), "TIFF field span")
		}
		fields[tag] = tiffField{data[int(start):int(start+bytes):int(start+bytes)], count, kind, t.order}
	}
	var err error
	value := func(tag uint16, def uint64) uint64 {
		f, ok := fields[tag]
		if !ok {
			return def
		}
		if f.count != 1 || (f.kind != 1 && f.kind != 3 && f.kind != 4) {
			if err == nil {
				err = malformed(int(off), "TIFF scalar field")
			}
			return 0
		}
		return f.at(0)
	}
	t.width, t.height = value(256, 0), value(257, 0)
	t.samples = value(277, 1)
	t.compression = value(259, 1)
	t.photo = value(262, ^uint64(0))
	t.rows = value(278, ^uint64(0)>>32)
	t.planar = value(284, 1)
	t.predictor = value(317, 1)
	t.orientation = value(274, 1)
	t.fillOrder = value(266, 1)
	if err != nil {
		return nil, err
	}
	if t.width == 0 || t.height == 0 || t.rows == 0 || t.samples == 0 {
		return nil, malformed(int(off), "TIFF dimensions/sample count")
	}
	if t.width > l.MaxPixels/t.height || t.width > uint64(int(^uint(0)>>1))/8/t.height || t.samples > 16 {
		return nil, failure(int(off), "TIFF pixel/sample budget", ErrLimit)
	}
	if _, ok := fields[324]; ok {
		return nil, failure(int(off), "tiled TIFF", ErrUnsupported)
	}
	if t.photo > 3 || t.planar < 1 || t.planar > 2 || t.predictor < 1 || t.predictor > 2 || t.fillOrder < 1 || t.fillOrder > 2 || t.orientation < 1 || t.orientation > 8 {
		return nil, failure(int(off), "TIFF pixel layout", ErrUnsupported)
	}
	switch t.compression {
	case 1, 5, 8, 32946, 32773:
	default:
		return nil, failure(int(off), "TIFF compression", ErrUnsupported)
	}
	base := uint64(1)
	if t.photo == 2 {
		base = 3
	}
	if t.samples < base {
		return nil, malformed(int(off), "TIFF color samples")
	}
	t.depth = 1
	if f, ok := fields[258]; ok {
		if f.count != t.samples || f.kind != 3 {
			return nil, malformed(int(off), "TIFF BitsPerSample")
		}
		t.depth = f.at(0)
		for i := uint64(1); i < f.count; i++ {
			if f.at(i) != t.depth {
				return nil, failure(int(off), "mixed TIFF sample widths", ErrUnsupported)
			}
		}
	}
	if t.depth != 1 && t.depth != 4 && t.depth != 8 && t.depth != 16 {
		return nil, failure(int(off), "TIFF sample width", ErrUnsupported)
	}
	if f, ok := fields[339]; ok {
		if f.count != t.samples || f.kind != 3 {
			return nil, malformed(int(off), "TIFF SampleFormat")
		}
		for i := uint64(0); i < f.count; i++ {
			if f.at(i) != 1 {
				return nil, failure(int(off), "non-unsigned TIFF samples", ErrUnsupported)
			}
		}
	}
	if f, ok := fields[338]; ok {
		if f.kind != 3 || f.count != t.samples-base {
			return nil, malformed(int(off), "TIFF extra samples")
		}
		for i := uint64(0); i < f.count; i++ {
			v := f.at(i)
			if v > 2 {
				return nil, failure(int(off), "TIFF extra-sample type", ErrUnsupported)
			}
			if v != 0 {
				if t.alpha >= 0 {
					return nil, malformed(int(off), "multiple TIFF alpha channels")
				}
				t.alpha = int(base + i)
				t.associated = v == 1
			}
		}
	}
	if t.photo == 3 {
		t.palette = fields[320]
		if t.palette.kind != 3 || t.palette.count != 3*(uint64(1)<<t.depth) {
			return nil, malformed(int(off), "TIFF color map")
		}
	}
	t.offsets, t.counts = fields[273], fields[279]
	if (t.offsets.kind != 1 && t.offsets.kind != 3 && t.offsets.kind != 4) || (t.counts.kind != 1 && t.counts.kind != 3 && t.counts.kind != 4) {
		return nil, malformed(int(off), "TIFF strip arrays")
	}
	strips := (t.height-1)/t.rows + 1
	planes := uint64(1)
	components := t.samples
	if t.planar == 2 {
		planes = t.samples
		components = 1
	}
	if t.offsets.count != strips*planes || t.counts.count != t.offsets.count {
		return nil, malformed(int(off), "TIFF strip count")
	}
	t.rowBytes = (t.width*components*t.depth + 7) / 8
	if t.rowBytes > l.MaxDecodedBytes/t.height/planes || t.rowBytes > uint64(int(^uint(0)>>1))/t.height/planes {
		return nil, failure(int(off), "TIFF decoded-byte budget", ErrLimit)
	}
	t.planeBytes = t.rowBytes * t.height
	t.totalBytes = t.planeBytes * planes
	for i := uint64(0); i < t.offsets.count; i++ {
		offset, size := t.offsets.at(i), t.counts.at(i)
		if offset < 8 || offset > uint64(len(data)) || size > uint64(len(data))-offset || size == 0 {
			return nil, malformed(int(off), "TIFF strip span")
		}
	}
	if f, ok := fields[34675]; ok {
		if f.kind != 7 && f.kind != 1 {
			return nil, malformed(int(off), "TIFF ICC field")
		}
		t.profile = f.data
	}
	return t, nil
}

func (t *TIFF) Image() (image.Image, error) { return t.ImageWithColorTransform(nil) }
func (t *TIFF) ImageWithColorTransform(factory ColorTransformFactory) (image.Image, error) {
	if t == nil || t.width == 0 || t.height == 0 {
		return nil, malformed(0, "uninitialized TIFF")
	}
	if len(t.profile) != 0 && factory == nil {
		return nil, failure(0, "TIFF ICC conversion", ErrUnsupported)
	}
	im, err := t.rawImage()
	if err != nil {
		return nil, err
	}
	if len(t.profile) == 0 {
		return im, nil
	}
	return ConvertToSRGB(im, t.ColorSpace(), factory, t.limits)
}

func (t *TIFF) rawImage() (image.Image, error) {
	raw := make([]byte, int(t.totalBytes))
	strips := (t.height-1)/t.rows + 1
	for i := uint64(0); i < t.offsets.count; i++ {
		plane, strip := i/strips, i%strips
		y := strip * t.rows
		rows := t.rows
		if rows > t.height-y {
			rows = t.height - y
		}
		n := rows * t.rowBytes
		start := plane*t.planeBytes + y*t.rowBytes
		dst := raw[int(start):int(start+n)]
		off, size := t.offsets.at(i), t.counts.at(i)
		if err := decodeTIFFStrip(dst, t.data[int(off):int(off+size)], t.compression, int(t.rowBytes)); err != nil {
			return nil, err
		}
		if t.fillOrder == 2 && t.depth < 8 {
			for j := range dst {
				dst[j] = bits.Reverse8(dst[j])
			}
		}
		if t.predictor == 2 {
			components := t.samples
			if t.planar == 2 {
				components = 1
			}
			for row := uint64(0); row < rows; row++ {
				line := dst[int(row*t.rowBytes):int((row+1)*t.rowBytes)]
				for s := components; s < t.width*components; s++ {
					v := (t.sample(line, s) + t.sample(line, s-components)) & ((1 << t.depth) - 1)
					t.putSample(line, s, v)
				}
			}
		}
	}
	wide := t.depth == 16 || t.photo == 3
	var out image.Image
	var low *image.NRGBA
	var high *image.NRGBA64
	if wide {
		high = image.NewNRGBA64(image.Rect(0, 0, t.Width(), t.Height()))
		out = high
	} else {
		low = image.NewNRGBA(image.Rect(0, 0, t.Width(), t.Height()))
		out = low
	}
	max := uint64(1)<<t.depth - 1
	for y := uint64(0); y < t.height; y++ {
		for x := uint64(0); x < t.width; x++ {
			get := func(component uint64) uint64 {
				if t.planar == 1 {
					row := raw[int(y*t.rowBytes):int((y+1)*t.rowBytes)]
					return t.sample(row, x*t.samples+component)
				}
				start := component*t.planeBytes + y*t.rowBytes
				return t.sample(raw[int(start):int(start+t.rowBytes)], x)
			}
			var r, g, b uint64
			a := max
			if t.alpha >= 0 {
				a = get(uint64(t.alpha))
			}
			switch t.photo {
			case 0, 1:
				r = get(0)
				if t.photo == 0 {
					r = max - r
				}
				g, b = r, r
			case 2:
				r, g, b = get(0), get(1), get(2)
			case 3:
				index := get(0)
				n := uint64(1) << t.depth
				r, g, b = t.palette.at(index), t.palette.at(n+index), t.palette.at(2*n+index)
			}
			if t.associated {
				if t.photo == 3 {
					return nil, failure(0, "associated-alpha palette TIFF", ErrUnsupported)
				}
				if r > a || g > a || b > a {
					return nil, malformed(0, "non-premultiplied TIFF sample")
				}
				if a != 0 {
					r, g, b = r*max/a, g*max/a, b*max/a
				}
			}
			if t.photo != 3 {
				r, g, b = r*65535/max, g*65535/max, b*65535/max
			}
			a = a * 65535 / max
			px, py := t.orient(int(x), int(y))
			if wide {
				high.SetNRGBA64(px, py, color.NRGBA64{uint16(r), uint16(g), uint16(b), uint16(a)})
			} else {
				low.SetNRGBA(px, py, color.NRGBA{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
			}
		}
	}
	return out, nil
}
func (t *TIFF) sample(row []byte, i uint64) uint64 {
	switch t.depth {
	case 16:
		return uint64(t.order.Uint16(row[int(i*2):]))
	case 8:
		return uint64(row[int(i)])
	default:
		bit := i * t.depth
		return uint64(row[int(bit/8)]>>uint(8-t.depth-bit%8)) & ((1 << t.depth) - 1)
	}
}
func (t *TIFF) putSample(row []byte, i, v uint64) {
	switch t.depth {
	case 16:
		t.order.PutUint16(row[int(i*2):], uint16(v))
	case 8:
		row[int(i)] = byte(v)
	default:
		bit := i * t.depth
		shift := uint(8 - t.depth - bit%8)
		mask := byte((1<<t.depth)-1) << shift
		row[int(bit/8)] = row[int(bit/8)]&^mask | byte(v)<<shift
	}
}
func (t *TIFF) orient(x, y int) (int, int) {
	w, h := int(t.width), int(t.height)
	switch t.orientation {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return h - 1 - y, x
	case 7:
		return h - 1 - y, w - 1 - x
	case 8:
		return y, w - 1 - x
	}
	return x, y
}
