// Copied from github.com/mgilbir/gowemf, file decode_plus.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// Boxes is a checked zero-copy array of EMF+ rectangles (origin and size,
// unlike a GDI Rect's left/top/right/bottom coordinates).
type Boxes struct {
	data       []byte
	compressed bool
}

func (b Boxes) Len() int {
	if b.compressed {
		return len(b.data) / 8
	}
	return len(b.data) / 16
}
func (b Boxes) At(i int) Box {
	if i < 0 || i >= b.Len() {
		panic("gowemf: rectangle index out of range")
	}
	w := 16
	if b.compressed {
		w = 8
	}
	c := cursor{b: b.data[i*w : (i+1)*w]}
	return c.plusBox(b.compressed)
}

type PlusRects struct {
	ObjectID   uint8
	BrushID    uint32
	Solid      bool
	Rectangles Boxes
}
type PlusPoly struct {
	ObjectID      uint8
	BrushID       uint32
	Solid, Closed bool
	Points        Points
}
type PlusEllipse struct {
	ObjectID               uint8
	BrushID                uint32
	Solid                  bool
	Rect                   Box
	StartAngle, SweepAngle float64
}
type PlusPathDraw struct {
	ObjectID uint8
	PaintID  uint32
	Solid    bool
}
type PlusText struct {
	FontID            uint8
	BrushID, FormatID uint32
	Solid             bool
	Layout            Box
	UTF16             []byte
}
type PlusImageDraw struct {
	ImageID             uint8
	AttributesID, Unit  uint32
	Source, Destination Box
	Points              Points
	Effect              bool
}
type PlusClip struct {
	ObjectID uint8
	Mode     uint32
	Rect     Box
}
type PlusPageTransform struct {
	Unit  uint8
	Scale float64
}

// PlusObjectFragment is the typed envelope of an object record, not a decoded
// object. Assemble fragments before passing the bytes to DecodePlusObject.
type PlusObjectFragment struct {
	ID, Type  uint8
	Continued bool
	TotalSize uint32
	Data      []byte
}

type PlusCurve struct {
	ObjectID               uint8
	BrushID                uint32
	Solid, Winding, Closed bool
	Tension                float64
	Offset, Segments       uint32
	Points                 Points
}
type PlusContainer struct {
	Unit                uint8
	Destination, Source Box
	StackIndex          uint32
}

// Driver-string Glyphs are Unicode code units only when Options&1 is nonzero;
// otherwise they are glyph indexes. Realized-advance records expose one meaningful
// position; StoredPositions reports whether the wire carried one or all positions.
type PlusDriverString struct {
	FontID                            uint8
	BrushID, Options, StoredPositions uint32
	Solid, HasMatrix                  bool
	Glyphs                            Integers
	Positions                         Points
	Matrix                            Matrix
}

func (c *cursor) emfplus(typ uint32, flags uint16) any {
	id := uint8(flags)
	compressed, solid := flags&0x4000 != 0, flags&0x8000 != 0
	var v any
	switch typ {
	case 0x4001:
		v = EMFPlusHeader{flags&1 != 0, c.dword(), c.dword(), c.dword(), c.dword()}
	case 0x4002, 0x4004, 0x402b, 0x4031:
		v = Empty{}
	case PlusMultiFormatStartRecord, PlusMultiFormatSectionRecord, PlusMultiFormatEndRecord:
		c.bad("reserved EMF+ record type")
		return nil
	case 0x4003:
		v = Comment{Data: c.take(uint64(c.dword()))}
	case 0x4008:
		f := PlusObjectFragment{ID: id, Type: uint8(flags>>8) & 127, Continued: solid}
		if id > 63 || f.Type < 1 || f.Type > 9 {
			c.bad("EMF+ object identity")
		}
		if f.Continued {
			f.TotalSize = c.dword()
			if uint64(f.TotalSize) > c.limits.MaxObjectBytes {
				c.err = failure(c.base+c.pos, "EMF+ object bytes", ErrLimit)
			}
		}
		f.Data = c.take(uint64(len(c.b) - c.pos))
		v = f
	case 0x4009:
		v = Value{c.dword()} // Clear ARGB
	case 0x400a, 0x400b:
		x := PlusRects{ObjectID: id, Solid: typ == 0x400a && solid}
		if typ == 0x400a {
			x.ObjectID = 0
			x.BrushID = c.dword()
			c.brushID(x.BrushID, solid)
		} else {
			c.plusID(id)
		}
		n := uint64(c.dword())
		if n < 1 {
			c.bad("empty EMF+ rectangle array")
		}
		w := 16
		if compressed {
			w = 8
		}
		x.Rectangles = Boxes{c.elements(n, w), compressed}
		if !compressed && c.err == nil {
			for i := 0; i < x.Rectangles.Len(); i++ {
				b := x.Rectangles.At(i)
				if !finite(b.X) || !finite(b.Y) || !finite(b.Width) || !finite(b.Height) {
					c.bad("non-finite rectangle")
					break
				}
			}
		}
		v = x
	case 0x400c, 0x400d, 0x4019:
		x := PlusPoly{ObjectID: id, Solid: typ == 0x400c && solid, Closed: typ == 0x400d && flags&0x2000 != 0}
		if typ == 0x400c {
			x.ObjectID = 0
			x.BrushID = c.dword()
			c.brushID(x.BrushID, solid)
			x.Closed = true
		} else {
			c.plusID(id)
		}
		n := uint64(c.dword())
		min := uint64(2)
		if typ == 0x400c {
			min = 3
		}
		if typ == 0x4019 {
			min = 4
			if n%3 != 1 {
				c.bad("EMF+ Bezier point count")
			}
		}
		if n < min {
			c.bad("EMF+ point count")
		}
		x.Points = c.plusPoints(n, flags)
		v = x
	case 0x400e, 0x400f, 0x4010, 0x4011, 0x4012:
		x := PlusEllipse{ObjectID: id, Solid: (typ == 0x400e || typ == 0x4010) && solid}
		if typ == 0x400e || typ == 0x4010 {
			x.ObjectID = 0
			x.BrushID = c.dword()
			c.brushID(x.BrushID, solid)
		} else {
			c.plusID(id)
		}
		if typ >= 0x4010 {
			x.StartAngle = c.float()
			x.SweepAngle = c.float()
		}
		x.Rect = c.plusBox(compressed)
		v = x
	case 0x4013, 0x4014, 0x4015:
		c.plusID(id)
		x := PlusPathDraw{ObjectID: id, PaintID: c.dword(), Solid: typ != 0x4015 && solid}
		if typ == 0x4015 {
			if x.PaintID > 63 {
				c.bad("EMF+ pen ID")
			}
		} else {
			c.brushID(x.PaintID, solid)
		}
		v = x
	case 0x4016, 0x4017, 0x4018:
		x := PlusCurve{ObjectID: id, Solid: typ == 0x4016 && solid, Winding: typ == 0x4016 && flags&0x2000 != 0, Closed: typ != 0x4018}
		if typ == 0x4016 {
			x.ObjectID = 0
			x.BrushID = c.dword()
			c.brushID(x.BrushID, solid)
		} else {
			c.plusID(id)
		}
		x.Tension = c.float()
		if typ == 0x4018 {
			x.Offset = c.dword()
			x.Segments = c.dword()
		}
		n := uint64(c.dword())
		minimum := uint64(3)
		if typ == 0x4018 {
			minimum = 2
			if uint64(x.Offset)+uint64(x.Segments) >= n {
				c.bad("cardinal curve segment range")
			}
		}
		if n < minimum {
			c.bad("cardinal curve point count")
		}
		pointFlags := flags
		if typ == 0x4018 {
			pointFlags &= 0x4000
		} // DrawCurve has no relative-coordinate flag.
		x.Points = c.plusPoints(n, pointFlags)
		v = x
	case 0x401a, 0x401b:
		c.plusID(id)
		x := PlusImageDraw{ImageID: id, AttributesID: c.dword(), Unit: c.dword(), Source: c.plusBox(false), Effect: typ == PlusDrawImagePointsRecord && flags&0x2000 != 0}
		if x.Unit != 2 {
			c.bad("EMF+ source unit")
		}
		if typ == 0x401a {
			x.Destination = c.plusBox(compressed)
		} else {
			n := uint64(c.dword())
			if n != 3 {
				c.bad("EMF+ image parallelogram")
			}
			x.Points = c.plusPoints(n, flags)
		}
		v = x
	case 0x401c:
		c.plusID(id)
		x := PlusText{FontID: id, BrushID: c.dword(), FormatID: c.dword(), Solid: solid}
		c.brushID(x.BrushID, solid)
		n := uint64(c.dword())
		x.Layout = c.plusBox(false)
		x.UTF16 = c.elements(n, 2)
		v = x
	case 0x401d:
		v = PointRecord{c.point(false)}
	case 0x401e, 0x401f, 0x4020, 0x4021, 0x4022, 0x4023, 0x4024:
		v = Value{uint32(flags)} // Property-specific flags retained, not normalized.
	case 0x4025, 0x4026, 0x4028, 0x4029:
		v = Value{c.dword()}
	case 0x4027:
		if flags&255 != 0 {
			c.bad("BeginContainer reserved flags")
		}
		v = PlusContainer{uint8(flags >> 8), c.plusBox(false), c.plusBox(false), c.dword()}
	case 0x402a, 0x402c:
		mode := uint32(0)
		if typ == 0x402c {
			mode = uint32(flags & 0x2000)
		}
		v = Transform{Matrix: c.matrix(), Mode: mode}
	case 0x402d, 0x402e, 0x4035:
		v = PointRecord{Point{c.float(), c.float()}}
	case 0x402f:
		v = FloatValue{c.float()}
	case 0x4030:
		v = PlusPageTransform{id, c.float()}
	case 0x4032:
		v = PlusClip{Mode: uint32(flags>>8) & 15, Rect: c.plusBox(false)}
	case 0x4033, 0x4034:
		c.plusID(id)
		v = PlusClip{ObjectID: id, Mode: uint32(flags>>8) & 15}
	case 0x4036:
		v = c.driverString(flags)
	case PlusSerializableObjectRecord:
		v = c.serializableEffect()
	case PlusSetTSGraphicsRecord:
		v = c.tsGraphics(flags)
	case PlusSetTSClipRecord:
		v = c.tsClip(flags)
	default:
		return c.unsupported()
	}
	// Only variable-length point/string/comment data can carry padding inside
	// DataSize. Fixed records have exact data lengths, even if flags are ignored.
	padding := 0
	if typ == 0x4003 || typ == 0x400c || typ == 0x400d || typ == 0x4016 || typ == 0x4017 || typ == 0x4019 || typ == 0x401b || typ == 0x401c || typ == 0x4036 {
		padding = 3
	}
	if c.err == nil && len(c.b)-c.pos > padding {
		c.bad("extra EMF+ record data")
	}
	return v
}

func (c *cursor) driverString(flags uint16) PlusDriverString {
	v := PlusDriverString{FontID: uint8(flags), BrushID: c.dword(), Options: c.dword(), Solid: flags&0x8000 != 0, Matrix: Identity()}
	c.plusID(v.FontID)
	c.brushID(v.BrushID, v.Solid)
	present := c.dword()
	n := uint64(c.dword())
	if present > 1 || v.Options & ^uint32(15) != 0 {
		c.bad("driver-string flags")
	}
	v.HasMatrix = present == 1
	v.Glyphs = c.ints(n, 2)
	v.StoredPositions = uint32(n)
	if v.Options&4 != 0 && n > 0 && c.err == nil {
		// The field description permits first-position-only, while the record
		// size formula describes a full array. Both unambiguous bounded layouts
		// are accepted; unused full-array positions are deliberately ignored.
		matrixBytes := uint64(0)
		if v.HasMatrix {
			matrixBytes = 24
		}
		remaining := uint64(len(c.b) - c.pos)
		full := n*8 + matrixBytes
		if remaining >= full && remaining-full <= 3 {
			v.StoredPositions = uint32(n)
		} else {
			v.StoredPositions = 1
		}
		v.Positions = c.points(1, 1)
		c.take((uint64(v.StoredPositions) - 1) * 8)
	} else {
		v.Positions = c.points(n, 1)
	}
	if v.HasMatrix {
		v.Matrix = c.matrix()
	}
	return v
}

func (c *cursor) plusID(id uint8) {
	if id > 63 {
		c.bad("EMF+ object ID")
	}
}
func (c *cursor) brushID(id uint32, solid bool) {
	if !solid && id > 63 {
		c.bad("EMF+ brush ID")
	}
}
func (c *cursor) plusBox(compressed bool) Box {
	if compressed {
		return Box{float64(c.short()), float64(c.short()), float64(c.short()), float64(c.short())}
	}
	return Box{c.float(), c.float(), c.float(), c.float()}
}

func (c *cursor) plusPoints(n uint64, flags uint16) Points {
	if flags&0x800 == 0 {
		enc := uint8(1)
		if flags&0x4000 != 0 {
			enc = 16
		}
		return c.points(n, enc)
	}
	// Relative coordinates require cumulative decoding. Validate a minimum wire
	// length and the allocation budget BEFORE allocating, even on 32-bit hosts.
	if c.err != nil {
		return Points{}
	}
	if n > c.limits.MaxElements {
		c.err = failure(c.base+c.pos, "relative point budget", ErrLimit)
		return Points{}
	}
	if n > uint64(len(c.b)-c.pos)/2 {
		c.bad("relative point count")
		return Points{}
	}
	if !c.allocation(n, 16) {
		return Points{}
	}
	points := make([]Point, int(n))
	var previous Point
	for i := range points {
		previous.X += float64(c.relative())
		previous.Y += float64(c.relative())
		points[i] = previous
		if c.err != nil {
			return Points{}
		}
	}
	return Points{relative: points}
}
func (c *cursor) relative() int32 {
	b := c.take(1)
	if b == nil {
		return 0
	}
	first := b[0]
	if first&128 == 0 {
		return int32(int8(first<<1) >> 1)
	}
	b = c.take(1)
	if b == nil {
		return 0
	}
	return int32(int16((uint16(first&127)<<8|uint16(b[0]))<<1) >> 1)
}
