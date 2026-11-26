// Copied from github.com/mgilbir/gowemf, file decode_emf_extra.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

type EMFRegionPaint struct {
	Bounds   Rect
	Region   Region
	Brush    uint32
	HasBrush bool
	Frame    Point
}

type PolyText struct {
	Bounds  Rect
	Strings []Text
}

func (c *cursor) polyText(unicode bool) PolyText {
	v := PolyText{Bounds: c.rect()}
	mode := c.dword()
	var sx, sy float64
	if mode == 2 {
		c.take(8)
		sx, sy = 1, 1
	} else {
		if mode != 1 {
			c.bad("poly-text graphics mode")
		}
		sx, sy = c.float(), c.float()
	}
	n := uint64(c.dword())
	if c.err != nil {
		return v
	}
	if n > c.limits.MaxElements {
		c.err = failure(c.base+c.pos, "poly-text string count", ErrLimit)
		return v
	}
	if n > uint64(len(c.b)-c.pos)/24 {
		c.bad("poly-text descriptors")
		return v
	}
	// Includes Text values and temporary offset bindings on 32/64-bit hosts.
	if !c.allocation(n, 192) {
		return v
	}
	type binding struct{ count, stringOffset, advanceOffset uint64 }
	spans := make([]binding, int(n))
	v.Strings = make([]Text, int(n))
	for i := range v.Strings {
		t := &v.Strings[i]
		t.Unicode, t.GraphicsMode, t.ScaleX, t.ScaleY = unicode, mode, sx, sy
		t.Reference = c.point(false)
		spans[i].count, spans[i].stringOffset = uint64(c.dword()), uint64(c.dword())
		t.Options = c.dword()
		if t.Options&0x100 == 0 {
			t.HasRectangle = true
			t.Rectangle = c.rect()
		}
		spans[i].advanceOffset = uint64(c.dword())
		if c.err != nil {
			return v
		}
	}
	minimum := c.pos // strings/advances cannot overlap ANY descriptor in the array
	for i, s := range spans {
		c.resolveText(&v.Strings[i], s.count, s.stringOffset, s.advanceOffset, minimum)
		if c.err != nil {
			return v
		}
	}
	return v
}

// RectangleAt follows slice indexing semantics. Region.Count gives the number
// of rectangles for a decoded region; caller-constructed values are also checked
// against the actual byte buffer before indexing.
func (r Region) RectangleAt(i int) Rect {
	if i < 0 || uint64(i) >= uint64(r.Count) || i >= len(r.Rectangles)/16 {
		panic("gowemf: region rectangle index out of range")
	}
	return rect(r.Rectangles[i*16:])
}

type GradientVertex struct {
	Point            Point
	Red, Green, Blue uint16
	IgnoredAlpha     uint16
}
type GradientVertices struct{ data []byte }

func (v GradientVertices) Len() int { return len(v.data) / 16 }
func (v GradientVertices) At(i int) GradientVertex {
	if i < 0 || i >= v.Len() {
		panic("gowemf: gradient vertex index out of range")
	}
	b := v.data[i*16:]
	return GradientVertex{Point{float64(int32(u32(b))), float64(int32(u32(b[4:])))}, u16(b[8:]), u16(b[10:]), u16(b[12:]), u16(b[14:])}
}

// Gradient uses pairs of vertex indexes for rectangle modes 0/1, triples for
// triangle mode 2. EMR_GRADIENTFILL MUST ignore the source vertex alpha values.
type Gradient struct {
	Bounds   Rect
	Mode     uint32
	Vertices GradientVertices
	Indexes  Integers
}

func (c *cursor) emfGradient() Gradient {
	v := Gradient{Bounds: c.rect()}
	vertices, meshes := uint64(c.dword()), uint64(c.dword())
	v.Mode = c.dword()
	if v.Mode > 2 {
		c.bad("gradient fill mode")
	}
	v.Vertices = GradientVertices{c.elements(vertices, 16)}
	stride := uint64(2)
	if v.Mode == 2 {
		stride = 3
	}
	v.Indexes = c.ints(meshes*stride, 4)
	if c.err == nil {
		for i := 0; i < v.Indexes.Len(); i++ {
			if uint64(v.Indexes.At(i)) >= vertices {
				c.bad("gradient vertex index")
				break
			}
		}
	}
	// Rectangle-mode vertex padding is unused tail data. MS-EMF 2.3 permits
	// truncation of unused tail fields, so neither its value nor presence matters.
	return v
}

func (c *cursor) smallText() Text {
	v := Text{Reference: c.point(false), Unicode: true}
	count := uint64(c.dword())
	v.Options = c.dword()
	v.GraphicsMode = c.dword()
	// Unlike ExtTextOut, SmallTextOut defines its scale fields without an
	// ignored-GM_ADVANCED qualification (MS-EMF 2.3.5.37).
	v.ScaleX, v.ScaleY = c.float(), c.float()
	if v.GraphicsMode != 1 && v.GraphicsMode != 2 {
		c.bad("small-text graphics mode")
	}
	if v.Options&0x100 == 0 {
		v.HasRectangle = true
		v.Rectangle = c.rect()
	}
	width := 2
	if v.Options&0x200 != 0 {
		v.SmallChars = true
		width = 1
	}
	v.Bytes = c.elements(count, width)
	return v
}

// MaskedBitmapTransfer holds source and optional monochrome mask spans. ROP4 is
// meaningful for MaskBlt; DestinationPoints describes PlgBlt's parallelogram.
// A renderer must still validate/decode the DIBs and apply their mask/ROP rules.
type MaskedBitmapTransfer struct {
	Bounds                                                       Rect
	Destination, DestinationSize, Source, SourceSize, MaskOrigin Point
	DestinationPoints                                            Points
	SourceTransform                                              Matrix
	ROP4, SourceBackground, SourceUsage, MaskUsage               uint32
	SourceInfo, SourceBits, MaskInfo, MaskBits                   []byte
}
type dibSpans struct{ infoOffset, infoBytes, bitsOffset, bitsBytes uint32 }

func (c *cursor) dibSpans() dibSpans { return dibSpans{c.dword(), c.dword(), c.dword(), c.dword()} }
func (s dibSpans) resolve(c *cursor, minimum int) ([]byte, []byte) {
	// EMF aligns records, not arbitrary byte buffers addressed by offsets.
	return c.span(uint64(s.infoOffset), uint64(s.infoBytes), minimum, 1), c.span(uint64(s.bitsOffset), uint64(s.bitsBytes), minimum, 1)
}
func (c *cursor) maskBlt(typ uint32) MaskedBitmapTransfer {
	v := MaskedBitmapTransfer{Bounds: c.rect()}
	if typ == EMRMaskBlt {
		v.Destination = c.point(false)
		v.DestinationSize = c.point(false)
		v.ROP4 = c.dword()
		v.Source = c.point(false)
		v.SourceSize = v.DestinationSize
	} else {
		v.DestinationPoints = c.points(3, 32)
		v.Source = c.point(false)
		v.SourceSize = c.point(false)
	}
	v.SourceTransform = c.matrix()
	v.SourceBackground = c.dword()
	v.SourceUsage = c.dword()
	source := c.dibSpans()
	v.MaskOrigin = c.point(false)
	v.MaskUsage = c.dword()
	mask := c.dibSpans()
	v.SourceInfo, v.SourceBits = source.resolve(c, c.pos)
	v.MaskInfo, v.MaskBits = mask.resolve(c, c.pos)
	return v
}
