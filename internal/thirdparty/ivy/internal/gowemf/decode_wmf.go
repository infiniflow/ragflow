// Copied from github.com/mgilbir/gowemf, file decode_wmf.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// MS-WMF 2.3: dispatch uses the low byte of RecordFunction, as required by
// individual record definitions. The original full value remains in Record.
func (c *cursor) wmf(typ uint32) any {
	switch typ & 255 {
	case 0:
		if typ != 0 || len(c.b) != 6 {
			c.bad("WMF EOF")
		}
		return Empty{}
	case 0x05, 0x1e, 0x35: // SetRelAbs (undefined/ignored), SaveDC, RealizePalette
		return Empty{}
	case 0x02, 0x03, 0x04, 0x06, 0x07, 0x2e, 0x2d, 0x2c, 0x2a, 0x2b, 0x34, 0xf0:
		return Value{uint32(c.word())}
	case 0x08, 0x27:
		return SignedValue{c.short()}
	case 0x49:
		v := Value{uint32(c.word())}
		c.word()
		return v
	case 0x01, 0x09, 0x31:
		return Value{c.dword()}
	case 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x11, 0x13, 0x14, 0x20:
		return PointRecord{c.wmfPoint()}
	case 0x10, 0x12:
		yd, yn, xd, xn := c.short(), c.short(), c.short(), c.short()
		if xd == 0 || yd == 0 {
			c.bad("zero scale denominator")
		}
		return Scale{xn, xd, yn, yd}
	case 0x15, 0x16, 0x18, 0x1b:
		return RectRecord{c.wmfRect()}
	case 0x1c:
		corner := c.wmfPoint()
		return RoundRect{c.wmfRect(), corner}
	case 0x17, 0x1a, 0x30:
		end, start := c.wmfPoint(), c.wmfPoint()
		return Arc{c.wmfRect(), start, end}
	case 0x1f:
		color := c.dword()
		return Pixel{c.wmfPoint(), color}
	case 0x24, 0x25:
		n := c.short()
		if n < 0 || (typ&255 == 0x24 && n < 2) {
			c.bad("WMF poly point count")
			return nil
		}
		return Poly{Points: c.points(uint64(n), 16)}
	case 0x38:
		counts := c.ints(uint64(c.word()), 2)
		var total uint64
		for i := 0; i < counts.Len(); i++ {
			total += uint64(counts.At(i))
		}
		return Poly{Counts: counts, Points: c.points(total, 16)}
	case 0xfa:
		p := Pen{Style: uint32(c.word())}
		p.Width = float64(c.short())
		c.short()
		p.Color = c.dword()
		return p
	case 0xfc:
		return Brush{Style: uint32(c.word()), Color: c.dword(), Hatch: uint32(c.word())}
	case 0xfb:
		f := Font{Height: c.short(), Width: c.short(), Escapement: c.short(), Orientation: c.short(), Weight: c.short()}
		c.fontFlags(&f)
		// ivy: MS-WMF 2.2.1.2 limits the face name to 32 bytes with its
		// NUL, and writers such as MathType store fewer bytes. Windows
		// reads the record as it is.
		f.FaceName = c.take(min(32, uint64(len(c.b)-c.pos)))
		return f
	case 0x21:
		n := c.short()
		if n < 0 {
			c.bad("negative text length")
			return nil
		}
		t := Text{Bytes: c.elements(uint64(n), 1)}
		if n&1 != 0 {
			c.take(1)
		}
		t.Reference = c.wmfPoint()
		return t
	case 0x32:
		t := Text{Reference: c.wmfPoint()}
		n := c.short()
		t.Options = uint32(c.word())
		if n < 0 {
			c.bad("negative text length")
			return nil
		}
		if t.Options&6 != 0 {
			t.HasRectangle = true
			t.Rectangle = Rect{c.short(), c.short(), c.short(), c.short()}
		}
		t.Bytes = c.elements(uint64(n), 1)
		if n&1 != 0 {
			c.take(1)
		}
		// ivy: a record too short for all the advances has none, and the
		// text takes the advances of its font. Writers such as Aspose
		// leave out the advance of the last character.
		if c.pos < len(c.b) && uint64(len(c.b)-c.pos) >= 2*uint64(n) {
			t.Advances = c.ints(uint64(n), 2)
		}
		return t
	case 0x0a:
		count, extra := c.word(), c.word()
		return TextJustification{int32(extra), int32(count)}
	case MetaStretchDIB & 255:
		v := PackedDIBTransfer{RasterOperation: c.dword(), Usage: uint32(c.word())}
		v.SourceSize = c.wmfPoint()
		v.Source = c.wmfPoint()
		v.DestinationSize = c.wmfPoint()
		v.Destination = c.wmfPoint()
		v.DIB = c.take(uint64(len(c.b) - c.pos))
		return v
	case MetaDIBBitBlt & 255, MetaDIBStretchBlt & 255:
		return c.wmfDIBBlt(typ)
	case 0x33:
		v := PackedDIBTransfer{Usage: uint32(c.word()), Scans: uint32(c.word()), StartScan: uint32(c.word())}
		v.Source = c.wmfUnsignedPoint()
		v.SourceSize = c.wmfUnsignedPoint()
		v.DestinationSize = v.SourceSize
		v.Destination = c.wmfUnsignedPoint()
		v.DIB = c.take(uint64(len(c.b) - c.pos))
		return v
	case 0x1d:
		v := PackedDIBTransfer{RasterOperation: c.dword()}
		v.DestinationSize = c.wmfPoint()
		v.Destination = c.wmfPoint()
		return v
	case 0x42:
		v := PackedPatternBrush{Style: uint32(c.word()), Usage: uint32(c.word())}
		if v.Style == 3 {
			v.Usage = 0
		} else {
			v.Style = 6
		}
		v.Data = c.take(uint64(len(c.b) - c.pos))
		return v
	case 0x26:
		return c.wmfEscape()
	case 0x19, 0x48:
		v := FloodFill{}
		if typ&255 == 0x48 {
			v.Mode = uint32(c.word())
		}
		v.Color = c.dword()
		v.Point = c.wmfPoint()
		if v.Mode > 1 {
			c.bad("flood fill mode")
		}
		return v
	case 0x28, 0x29:
		v := RegionPaint{Region: uint32(c.word()), Brush: uint32(c.word())}
		if typ&255 == 0x29 {
			v.Frame = c.wmfPoint()
		}
		return v
	case 0xf7, 0x36, 0x37:
		v := Palette{Start: uint32(c.word()), Count: uint32(c.word())}
		if typ&255 == 0xf7 {
			if v.Start != 0x300 {
				c.bad("WMF palette version")
			}
			v.Start = 0
		}
		v.Entries = c.elements(uint64(v.Count), 4)
		return v
	case 0x39:
		return Palette{Count: uint32(c.word())}
	case 0xff:
		return c.wmfRegion()
	case 0xf9:
		return BitmapPatternBrush{Bitmap: c.bitmap16(true)}
	case 0x22, 0x23:
		normalized := typ&0xff00 | 0x40
		if typ&255 == 0x23 {
			normalized = typ&0xff00 | 0x41
		}
		geometry := c.wmfDIBBlt(normalized)
		v := Bitmap16Transfer{RasterOperation: geometry.RasterOperation, Source: geometry.Source, SourceSize: geometry.SourceSize, Destination: geometry.Destination, DestinationSize: geometry.DestinationSize, DeviceSource: geometry.DeviceSource}
		if !v.DeviceSource && c.err == nil {
			q := cursor{b: geometry.DIB, base: c.base + c.pos - len(geometry.DIB), limits: c.limits}
			bitmap := q.bitmap16(false)
			v.Bitmap = &bitmap
			c.err = q.err
		}
		return v
	default:
		return c.unsupported()
	}
}

type TextJustification struct{ Extra, Count int32 }

// PackedDIBTransfer carries a WMF packed DIB. ParsePackedDIB splits and validates
// its header/palette/pixel layout before allocating an image.
type PackedDIBTransfer struct {
	// DeviceSource distinguishes the no-bitmap DibBlt forms from a source DIB.
	DeviceSource                                     bool
	StartScan, Scans                                 uint32
	RasterOperation, Usage                           uint32
	Source, SourceSize, Destination, DestinationSize Point
	DIB                                              []byte
}

// PackedPatternBrush retains a WMF DIB/Bitmap16 pattern. Style is normalized as
// required by MS-WMF 2.3.4.8; bitmap pixel interpretation is a separate operation.
type PackedPatternBrush struct {
	Style, Usage uint32
	Data         []byte
}

func (c *cursor) wmfDIBBlt(typ uint32) PackedDIBTransfer {
	v := PackedDIBTransfer{RasterOperation: c.dword(), DeviceSource: uint32(len(c.b)/2) == (typ>>8)+3}
	stretch := typ&255 == 0x41
	parameters := uint32(9)
	if stretch {
		parameters = 11
	}
	if typ>>8 != parameters {
		c.bad("WMF bitmap function parameter count")
	}
	if stretch {
		v.SourceSize = c.wmfPoint()
	}
	v.Source = c.wmfPoint()
	if v.DeviceSource {
		c.word()
	} // Reserved exists only in the no-bitmap form.
	v.DestinationSize = c.wmfPoint()
	if !stretch {
		v.SourceSize = v.DestinationSize
	}
	v.Destination = c.wmfPoint()
	if !v.DeviceSource {
		v.DIB = c.take(uint64(len(c.b) - c.pos))
		if len(v.DIB) < 12 {
			c.bad("missing WMF source DIB")
		}
	}
	return v
}

func (c *cursor) wmfUnsignedPoint() Point {
	y, x := c.word(), c.word()
	return Point{float64(x), float64(y)}
}

// WMFEnhancedMetafile is a typed envelope for one WMFC escape fragment. It is
// metadata, not a drawing command; the enclosing WMF records remain its fallback.
// Decode does not concatenate or checksum the entire embedded EMF stream.
type WMFEnhancedMetafile struct {
	Version                        uint32
	Checksum                       uint16
	Records, Remaining, TotalBytes uint32
	Data                           []byte
}

// WMFEscape is any other META_ESCAPE record (MS-WMF 2.3.6.1): a printer-driver
// function from the MetafileEscapes enumeration (2.1.1.17) with its EscapeData,
// or an MFCOMMENT (function 15) private comment. Escapes do not draw on a
// display device, so playback ignores them.
type WMFEscape struct {
	Function uint16
	Data     []byte
}

// wmfEscapeFunctions holds the MetafileEscapes enumeration (MS-WMF 2.1.1.17).
var wmfEscapeFunctions = map[uint16]bool{
	0x0001: true, 0x0002: true, 0x0003: true, 0x0004: true, 0x0005: true, 0x0006: true,
	0x0007: true, 0x0008: true, 0x0009: true, 0x000a: true, 0x000b: true, 0x000c: true,
	0x000d: true, 0x000e: true, 0x000f: true, 0x0010: true, 0x0011: true, 0x0012: true,
	0x0013: true, 0x0014: true, 0x0015: true, 0x0016: true, 0x0017: true, 0x0018: true,
	0x0019: true, 0x001a: true, 0x001b: true, 0x001c: true, 0x001d: true, 0x001e: true,
	0x001f: true, 0x0020: true, 0x0021: true, 0x0022: true, 0x0023: true, 0x0025: true,
	0x0026: true, 0x002a: true, 0x0100: true, 0x0102: true, 0x0200: true, 0x0201: true,
	0x0202: true, 0x0801: true, 0x0c01: true, 0x1000: true, 0x1001: true, 0x1002: true,
	0x100e: true, 0x100f: true, 0x1010: true, 0x1013: true, 0x1014: true, 0x1015: true,
	0x1016: true, 0x1017: true, 0x1018: true, 0x1019: true, 0x101a: true, 0x11d8: true,
}

// isWMFCFragment reports whether escape data is a META_ESCAPE_ENHANCED_METAFILE
// fragment: an MFCOMMENT whose CommentIdentifier is WMFC and CommentType is 1
// (MS-WMF 2.3.6.25). Every other MFCOMMENT is a private comment.
func isWMFCFragment(function uint16, data []byte) bool {
	return function == 15 && len(data) >= 8 && u32(data) == 0x43464d57 && u32(data[4:]) == 1
}

func (c *cursor) wmfEscape() any {
	function, count := c.word(), uint64(c.word())
	if !wmfEscapeFunctions[function] {
		return c.unsupported()
	}
	raw := c.take(count)
	if c.err != nil {
		return nil
	}
	if !isWMFCFragment(function, raw) {
		return WMFEscape{Function: function, Data: raw}
	}
	q := cursor{b: raw, base: c.base + c.pos - len(raw), limits: c.limits}
	q.dword()
	q.dword()
	f := WMFEnhancedMetafile{Version: q.dword(), Checksum: q.word()}
	if q.dword() != 0 {
		q.bad("WMFC flags")
	}
	f.Records = q.dword()
	current := uint64(q.dword())
	f.Remaining = q.dword()
	f.TotalBytes = q.dword()
	if f.Records == 0 || current == 0 || current > 8192 || current+uint64(f.Remaining) > uint64(f.TotalBytes) || count != 34+current {
		q.bad("WMFC fragment lengths")
	}
	if uint64(f.TotalBytes) > c.limits.MaxObjectBytes && q.err == nil {
		q.err = failure(q.base, "embedded EMF bytes", ErrLimit)
	}
	f.Data = q.take(current)
	if q.err != nil {
		c.err = q.err
	}
	return f
}

func (c *cursor) wmfPoint() Point { y, x := c.short(), c.short(); return Point{float64(x), float64(y)} }
func (c *cursor) wmfRect() Rect {
	b, r, t, l := c.short(), c.short(), c.short(), c.short()
	return Rect{l, t, r, b}
}
func (c *cursor) fontFlags(f *Font) {
	b := c.take(8)
	if b == nil {
		return
	}
	f.Italic, f.Underline, f.StrikeOut, f.CharSet = b[0], b[1], b[2], b[3]
	f.OutPrecision, f.ClipPrecision, f.Quality, f.PitchAndFamily = b[4], b[5], b[6], b[7]
}
