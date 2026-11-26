// Copied from github.com/mgilbir/gowemf, file decode_emf.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// MS-EMF 2.3. Fixed records permit trailing extension bytes (2.3). Every
// variable span is bounded within this record, never within the outer file.
func (c *cursor) emf(typ uint32) any {
	switch typ {
	case 1:
		c.take(80)
		return Empty{} // metadata is returned by Walk
	case EMRCreateColorSpace, EMRCreateColorSpaceW:
		return c.colorSpaceObject(typ == EMRCreateColorSpaceW)
	case EMRSetColorSpace, EMRDeleteColorSpace:
		return Value{c.dword()}
	case EMRSetICMProfileA, EMRSetICMProfileW, EMRColorMatchToTargetW:
		return c.colorProfile(typ)
	case EMRSetColorAdjustment:
		return c.colorAdjustment()
	case EMRColorCorrectPalette:
		v := Palette{Handle: c.dword(), Start: c.dword(), Count: c.dword()}
		c.dword()
		return v
	case 14:
		c.take(12)
		return Empty{}
	case 28, 33, 52, 59, 60, 61, 65, 66, 68:
		return Empty{}
	case 9, 10, 11, 12, 13, 26, 27, 54:
		return PointRecord{c.point(false)}
	case 15:
		return Pixel{c.point(false), c.dword()}
	case 16, 17, 18, 19, 20, 21, 22, 24, 25, 37, 40, 48, 57, 67, 98, 115:
		return Value{c.dword()}
	case 34:
		v := c.long()
		if v >= 0 {
			c.bad("EMF restore index must be negative")
		}
		return SignedValue{v}
	case 58:
		return FloatValue{c.float()}
	case 29, 30, 42, 43, 62, 63, 64:
		return RectRecord{c.rect()}
	case 31, 32:
		s := Scale{c.long(), c.long(), c.long(), c.long()}
		if s.XNum == 0 || s.YNum == 0 || s.XDenom == 0 || s.YDenom == 0 {
			c.bad("zero scale factor")
		}
		return s
	case 35:
		return Transform{Matrix: c.matrix()}
	case 36:
		start := c.pos
		raw := c.take(24)
		mode := c.dword()
		if mode < 1 || mode > 4 {
			c.bad("world transform mode")
		}
		if mode == 1 {
			return Transform{Matrix: Identity(), Mode: mode}
		}
		q := cursor{b: raw, base: c.base + start}
		m := q.matrix()
		if c.err == nil {
			c.err = q.err
		}
		return Transform{Matrix: m, Mode: mode}
	case 41:
		return AngleArc{c.point(false), c.dword(), c.float(), c.float()}
	case 44:
		return RoundRect{c.rect(), c.point(false)}
	case 45, 46, 47, 55:
		return Arc{c.rect(), c.point(false), c.point(false)}
	case 2, 3, 4, 5, 6, 7, 8, 56, 85, 86, 87, 88, 89, 90, 91, 92:
		return c.emfPoly(typ)
	case 38:
		p := Pen{Handle: c.dword(), Style: c.dword(), Width: float64(c.long())}
		c.long()
		p.Color = c.dword()
		return p
	case 39:
		return Brush{c.dword(), c.dword(), c.dword(), c.dword()}
	case 82:
		f := Font{Handle: c.dword(), Height: c.long(), Width: c.long(), Escapement: c.long(), Orientation: c.long(), Weight: c.long(), Unicode: true}
		c.fontFlags(&f)
		f.FaceName = c.take(64)
		start := c.pos
		f.Extensions = c.b[start:len(c.b):len(c.b)]
		if c.err == nil && len(c.b)-12 >= 320 {
			e := &FontExtension{FullName: c.take(128), Style: c.take(64)}
			f.Extended = e
			if len(c.b)-12 == 320 {
				c.dword()
				e.StyleSize = c.dword()
				c.take(16)
				e.Panose = c.take(10)
				c.take(2)
			} else {
				e.Script = c.take(64)
				if c.dword() != 0x08007664 {
					c.bad("font design-vector signature")
				}
				n := uint64(c.dword())
				if n > 16 {
					c.bad("font design-vector axes")
				}
				e.DesignAxes = c.ints(n, 4)
			}
		} else {
			c.take(uint64(len(c.b) - c.pos))
		}
		return f
	case 95:
		return c.emfPen()
	case 93, 94:
		v := PatternBrush{Handle: c.dword(), Usage: c.dword()}
		offInfo, nInfo, offBits, nBits := c.dword(), c.dword(), c.dword(), c.dword()
		v.Info, v.Bits = (dibSpans{offInfo, nInfo, offBits, nBits}).resolve(c, c.pos)
		return v
	case 49:
		v := Palette{Handle: c.dword()}
		if c.word() != 0x300 {
			c.bad("palette version")
		}
		v.Count = uint32(c.word())
		if v.Count == 0 {
			c.bad("empty palette")
		}
		v.Entries = c.elements(uint64(v.Count), 4)
		return v
	case 50:
		v := Palette{Handle: c.dword(), Start: c.dword(), Count: c.dword()}
		v.Entries = c.elements(uint64(v.Count), 4)
		return v
	case 51:
		v := Palette{Handle: c.dword(), Count: c.dword()}
		if v.Count == 0 || v.Count > 1024 {
			c.bad("palette resize count")
		}
		return v
	case 70:
		n := c.dword()
		data := c.take(uint64(n))
		v := Comment{Data: data}
		if len(data) >= 4 {
			v.Identifier = u32(data)
			v.Data = data[4:len(data):len(data)]
		}
		return v
	case 75:
		return c.emfRegion()
	case EMRFillRgn, EMRFrameRgn, EMRInvertRgn, EMRPaintRgn:
		v := EMFRegionPaint{Bounds: c.rect()}
		n := uint64(c.dword())
		if typ == EMRFillRgn || typ == EMRFrameRgn {
			v.HasBrush = true
			v.Brush = c.dword()
		}
		if typ == EMRFrameRgn {
			v.Frame = c.point(false)
		}
		v.Region = c.regionData(n)
		return v
	case EMRExtFloodFill:
		v := FloodFill{Point: c.point(false), Color: c.dword(), Mode: c.dword()}
		if v.Mode > 1 {
			c.bad("flood fill mode")
		}
		return v
	case EMRGradientFill:
		return c.emfGradient()
	case EMRSmallTextOut:
		return c.smallText()
	case 80, 81:
		return c.emfBitmap(typ)
	case 76, 77, 114, 116:
		return c.emfRaster(typ)
	case EMRMaskBlt, EMRPlgBlt:
		return c.maskBlt(typ)
	case 83, 84:
		return c.emfText(typ == 84)
	case EMRPolyTextOutA, EMRPolyTextOutW:
		return c.polyText(typ == EMRPolyTextOutW)
	case 120:
		return TextJustification{c.long(), c.long()}
	default:
		return c.unsupported()
	}
}

type FloatValue struct{ Value float64 }
type PatternBrush struct {
	Handle, Usage uint32
	Info, Bits    []byte
}
type Palette struct {
	Handle, Start, Count uint32
	Entries              []byte
}

// RasterTransfer is a BitBlt/StretchBlt/AlphaBlend/TransparentBlt body. Operation
// is respectively a ROP3, packed BLENDFUNCTION, or COLORREF. Record.Type selects
// its meaning; callers must not interpret a blend function as a raster opcode.
type RasterTransfer struct {
	BitmapTransfer
	Operation, SourceBackground uint32
	SourceTransform             Matrix
}

func (c *cursor) emfRaster(typ uint32) RasterTransfer {
	v := RasterTransfer{}
	v.Bounds = c.rect()
	v.Destination = c.point(false)
	v.DestinationSize = c.point(false)
	v.Operation = c.dword()
	v.Source = c.point(false)
	v.SourceTransform = c.matrix()
	v.SourceBackground = c.dword()
	v.Usage = c.dword()
	offInfo, nInfo, offBits, nBits := c.dword(), c.dword(), c.dword(), c.dword()
	if typ == 76 {
		v.SourceSize = v.DestinationSize
	} else {
		v.SourceSize = c.point(false)
	}
	v.Info, v.Bits = (dibSpans{offInfo, nInfo, offBits, nBits}).resolve(c, c.pos)
	if typ == 114 {
		if byte(v.Operation) != 0 || byte(v.Operation>>24) > 1 || v.DestinationSize.X <= 0 || v.DestinationSize.Y <= 0 || v.SourceSize.X <= 0 || v.SourceSize.Y <= 0 {
			c.bad("alpha blend parameters")
		}
	}
	return v
}

func (c *cursor) emfPoly(typ uint32) Poly {
	p := Poly{Bounds: c.rect()}
	multiple := typ == 7 || typ == 8 || typ == 90 || typ == 91
	var polygons uint64
	if multiple {
		polygons = uint64(c.dword())
	}
	n := uint64(c.dword())
	if multiple {
		p.Counts = c.ints(polygons, 4)
		var total uint64
		for i := 0; i < p.Counts.Len(); i++ {
			total += uint64(p.Counts.At(i))
			if total > n {
				c.bad("polygon point count sum")
				break
			}
		}
		if total != n {
			c.bad("polygon point count sum")
		}
	}
	encoding := uint8(32)
	if typ >= 85 {
		encoding = 16
	}
	p.Points = c.points(n, encoding)
	if typ == 56 || typ == 92 {
		p.Types = c.elements(n, 1)
	}
	// Bezier and BezierTo consume control-point triples; malformed groupings
	// cannot safely be handed to a path builder.
	if (typ == 2 || typ == 85) && (n < 4 || (n-1)%3 != 0) {
		c.bad("Bezier point count")
	}
	if (typ == 5 || typ == 88) && (n == 0 || n%3 != 0) {
		c.bad("BezierTo point count")
	}
	return p
}

func (c *cursor) emfPen() Pen {
	p := Pen{Handle: c.dword()}
	offInfo, nInfo, offBits, nBits := c.dword(), c.dword(), c.dword(), c.dword()
	p.Style = c.dword()
	p.Width = float64(c.dword())
	p.BrushStyle = c.dword()
	p.Color = c.dword()
	p.Hatch = c.dword()
	p.Dashes = c.ints(uint64(c.dword()), 4)
	p.BitmapInfo, p.BitmapBits = (dibSpans{offInfo, nInfo, offBits, nBits}).resolve(c, c.pos)
	return p
}

func (c *cursor) emfText(unicode bool) Text {
	c.rect() // bounds MUST be ignored
	t := Text{Unicode: unicode, GraphicsMode: c.dword()}
	if t.GraphicsMode == 2 {
		c.take(8) // exScale/eyScale MUST be ignored in GM_ADVANCED.
		t.ScaleX, t.ScaleY = 1, 1
	} else {
		if t.GraphicsMode != 1 {
			c.bad("text graphics mode")
		}
		t.ScaleX, t.ScaleY = c.float(), c.float()
	}
	t.Reference = c.point(false)
	count, offString := uint64(c.dword()), uint64(c.dword())
	t.Options = c.dword()
	// ETO_NO_RECT suppresses the rectangle entirely, unlike ETO_CLIPPED and
	// ETO_OPAQUE, which determine how a present rectangle is used.
	if t.Options&0x100 == 0 {
		t.Rectangle = c.rect()
		t.HasRectangle = true
	}
	offDx := uint64(c.dword())
	c.resolveText(&t, count, offString, offDx, c.pos)
	return t
}

func (c *cursor) resolveText(t *Text, count, offString, offDx uint64, minimum int) {
	width := 1
	if t.Unicode {
		width = 2
	}
	if count > c.limits.MaxElements {
		if c.err == nil {
			c.err = failure(c.base+c.pos, "text length", ErrLimit)
		}
		return
	}
	t.Bytes = c.span(offString, count*uint64(width), minimum, width)
	if offDx != 0 {
		n := count
		if t.Options&0x2000 != 0 {
			n *= 2
		}
		if n > c.limits.MaxElements {
			if c.err == nil {
				c.err = failure(c.base+c.pos, "text advance count", ErrLimit)
			}
			return
		}
		t.Advances = Integers{c.span(offDx, n*4, minimum, 4), 4}
	}
}

func (c *cursor) emfBitmap(typ uint32) BitmapTransfer {
	v := BitmapTransfer{Bounds: c.rect(), Destination: c.point(false), Source: c.point(false), SourceSize: c.point(false)}
	offInfo, nInfo, offBits, nBits := c.dword(), c.dword(), c.dword(), c.dword()
	v.Usage = c.dword()
	if typ == 80 {
		v.StartScan = c.dword()
		v.Scans = c.dword()
	} else {
		v.RasterOperation = c.dword()
		v.DestinationSize = c.point(false)
	}
	v.Info, v.Bits = (dibSpans{offInfo, nInfo, offBits, nBits}).resolve(c, c.pos)
	return v
}

func (c *cursor) emfRegion() Region {
	n, mode := uint64(c.dword()), c.dword()
	v := Region{Mode: mode}
	if mode < 1 || mode > 5 {
		c.bad("region combination mode")
	}
	if n == 0 {
		if mode != 5 {
			c.bad("empty non-copy region")
		}
		return v
	}
	v = c.regionData(n)
	v.Mode = mode
	return v
}

func (c *cursor) regionData(n uint64) Region {
	v := Region{}
	r := c.take(n)
	if c.err != nil {
		return v
	}
	q := cursor{b: r, base: c.base + c.pos - int(n), limits: c.limits}
	if q.dword() != 32 || q.dword() != 1 {
		q.bad("region header")
	}
	v.Count = q.dword()
	// ivy: nRgnSize is the size of the buffer of the rectangles, and the
	// RGNDATAHEADER of Windows lets it be 0 when the size is not known.
	// Some writers give a larger buffer, such as 168 for one rectangle.
	// The reader takes nCount rectangles, which must fit in the data.
	_ = q.dword()
	v.Bounds = q.rect()
	v.Rectangles = q.elements(uint64(v.Count), 16)
	if q.err != nil {
		c.err = q.err
	}
	return v
}
