// Copied from github.com/mgilbir/gowemf, file decode.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"errors"
	"math"
)

// ErrUnsupported distinguishes a record or encoding that has no typed decoder
// from corrupt input. The raw record is still available from Walk.
var ErrUnsupported = errors.New("unsupported metafile record or encoding")

// DecodeLimits bounds typed array processing and assembled object sizes.
// Zero fields select finite defaults. These limits supplement framing Limits.
type DecodeLimits struct {
	MaxNesting     uint32 // default 256 nested object/region-tree levels
	MaxElements    uint64 // default 1,000,000 per array
	MaxObjectBytes uint64 // default 16 MiB
}

func (l DecodeLimits) defaults() DecodeLimits {
	if l.MaxNesting == 0 {
		l.MaxNesting = 256
	}
	if l.MaxElements == 0 {
		l.MaxElements = 1_000_000
	}
	if l.MaxObjectBytes == 0 {
		l.MaxObjectBytes = 16 << 20
	}
	return l
}

// Point represents either integer GDI coordinates or floating-point GDI+ ones.
type Point struct{ X, Y float64 }
type Box struct{ X, Y, Width, Height float64 }

// Points is a checked, zero-copy coordinate array. At follows Go slice indexing
// semantics and panics for an index outside [0, Len()). The zero value is empty.
type Points struct {
	data     []byte
	encoding uint8
	relative []Point
}

func (p Points) Len() int {
	if p.relative != nil {
		return len(p.relative)
	}
	if p.encoding == 0 {
		return 0
	}
	if p.encoding == 16 {
		return len(p.data) / 4
	}
	return len(p.data) / 8
}
func (p Points) At(i int) Point {
	if i < 0 || i >= p.Len() {
		panic("gowemf: point index out of range")
	}
	if p.relative != nil {
		return p.relative[i]
	}
	if p.encoding == 16 {
		b := p.data[i*4:]
		return Point{float64(int16(u16(b))), float64(int16(u16(b[2:])))}
	}
	b := p.data[i*8:]
	if p.encoding == 32 {
		return Point{float64(int32(u32(b))), float64(int32(u32(b[4:])))}
	}
	return Point{float64(math.Float32frombits(u32(b))), float64(math.Float32frombits(u32(b[4:])))}
}

// Integers is a checked, zero-copy WORD or DWORD array, such as polygon counts
// or text advances. SignedAt sign-extends the wire width; At is unsigned.
type Integers struct {
	data  []byte
	width int
}

func (a Integers) Len() int {
	if a.width == 0 {
		return 0
	}
	return len(a.data) / a.width
}
func (a Integers) At(i int) uint32 {
	if i < 0 || i >= a.Len() {
		panic("gowemf: integer index out of range")
	}
	if a.width == 2 {
		return uint32(u16(a.data[i*2:]))
	}
	return u32(a.data[i*4:])
}
func (a Integers) SignedAt(i int) int32 {
	n := a.At(i)
	if a.width == 2 {
		return int32(int16(n))
	}
	return int32(n)
}

// Typed bodies retain format-specific modes and flags rather than inventing
// rendering defaults. Consult Record.Type to distinguish operations sharing a
// body shape. All byte/array views have the same ownership as Record.Raw.
type Empty struct{}
type Value struct{ Value uint32 }
type SignedValue struct{ Value int32 }
type PointRecord struct{ Point Point }
type RectRecord struct{ Rect Rect }
type RoundRect struct {
	Rect   Rect
	Corner Point
}
type Arc struct {
	Rect       Rect
	Start, End Point
}
type AngleArc struct {
	Center                 Point
	Radius                 uint32
	StartAngle, SweepAngle float64
}
type Scale struct{ XNum, XDenom, YNum, YDenom int32 }
type Poly struct {
	Bounds Rect
	Counts Integers
	Points Points
	Types  []byte
}
type Transform struct {
	Matrix Matrix
	Mode   uint32
}
type Pixel struct {
	Point Point
	Color uint32
}

// Matrix is the row-vector affine transform used by GDI/GDI+:
// x'=x*M11+y*M21+Dx, y'=x*M12+y*M22+Dy.
type Matrix struct{ M11, M12, M21, M22, Dx, Dy float64 }

func Identity() Matrix { return Matrix{M11: 1, M22: 1} }
func (m Matrix) Apply(p Point) Point {
	return Point{p.X*m.M11 + p.Y*m.M21 + m.Dx, p.X*m.M12 + p.Y*m.M22 + m.Dy}
}

// Then composes m followed by n. Callers must check Finite after composition.
func (m Matrix) Then(n Matrix) Matrix {
	return Matrix{m.M11*n.M11 + m.M12*n.M21, m.M11*n.M12 + m.M12*n.M22, m.M21*n.M11 + m.M22*n.M21, m.M21*n.M12 + m.M22*n.M22, m.Dx*n.M11 + m.Dy*n.M21 + n.Dx, m.Dx*n.M12 + m.Dy*n.M22 + n.Dy}
}
func (m Matrix) Finite() bool {
	return finite(m.M11) && finite(m.M12) && finite(m.M21) && finite(m.M22) && finite(m.Dx) && finite(m.Dy)
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

type Pen struct {
	Handle, Style            uint32
	Width                    float64
	Color, BrushStyle, Hatch uint32
	Dashes                   Integers
	BitmapInfo, BitmapBits   []byte
}
type Brush struct{ Handle, Style, Color, Hatch uint32 }
type Font struct {
	Extended                                                                                    *FontExtension
	Handle                                                                                      uint32
	Height, Width, Escapement, Orientation, Weight                                              int32
	Italic, Underline, StrikeOut, CharSet, OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
	// FaceName is raw ANSI (WMF) or UTF-16LE (EMF), including possible terminators.
	FaceName []byte
	Unicode  bool
	// Extensions retain PANOSE/design-vector fields without interpreting them.
	Extensions []byte
}

type FontExtension struct {
	FullName, Style, Script []byte // fixed-capacity UTF-16LE fields
	StyleSize               uint32
	Panose                  []byte
	DesignAxes              Integers
}
type Text struct {
	// SmallChars packs the low byte of Unicode code units (EMR_SMALLTEXTOUT),
	// not an ANSI code page. Unicode remains true for this encoding.
	SmallChars            bool
	Reference             Point
	Options, GraphicsMode uint32
	ScaleX, ScaleY        float64
	Rectangle             Rect
	HasRectangle, Unicode bool
	// Bytes contains code-page bytes, UTF-16LE code units, or glyph indexes when
	// ETO_GLYPH_INDEX is set. No locale-dependent conversion is performed.
	Bytes    []byte
	Advances Integers
}
type BitmapTransfer struct {
	Bounds                                           Rect
	Destination, Source, SourceSize, DestinationSize Point
	Usage, RasterOperation, StartScan, Scans         uint32
	Info, Bits                                       []byte // bounded DIB spans; decoded separately by DecodeDIB
}
type Region struct {
	Mode       uint32
	Bounds     Rect
	Rectangles []byte
	Count      uint32
}
type Comment struct {
	Identifier uint32
	Data       []byte
}

// Decode returns a typed record body after checking its layout, array lengths,
// offsets and finite floats. It does not perform playback or validate object
// references against a stream. The result's concrete types are documented in
// COVERAGE.md. Unsupported encodings return ErrUnsupported, never a success
// containing an opaque drawing body. Raw is authoritative; Data is not trusted.
func Decode(r Record, limits DecodeLimits) (any, error) {
	l := limits.defaults()
	if r.Offset < 0 || r.Offset > int(^uint(0)>>1)-len(r.Raw) {
		return nil, malformed(0, "record offset")
	}
	if uint64(len(r.Raw)) > l.MaxObjectBytes {
		return nil, failure(r.Offset, "typed record bytes", ErrLimit)
	}
	h := 8
	switch r.Format {
	case WMF:
		h = 6
	case EMF:
	case EMFPlus:
		h = 12
	default:
		return nil, failure(r.Offset, "record format", ErrFormat)
	}
	if len(r.Raw) < h {
		return nil, malformed(r.Offset, "typed record header")
	}
	b := r.Raw
	var n uint64
	var typ uint32
	if r.Format == WMF {
		n = uint64(u32(b)) * 2
		typ = uint32(u16(b[4:]))
	} else if r.Format == EMF {
		n = uint64(u32(b[4:]))
		typ = u32(b)
	} else {
		n = uint64(u32(b[4:]))
		typ = uint32(u16(b))
	}
	if n != uint64(len(b)) || typ != r.Type {
		return nil, malformed(r.Offset, "typed record framing")
	}
	if (r.Format == WMF && n%2 != 0) || (r.Format != WMF && n%4 != 0) {
		return nil, malformed(r.Offset, "typed record alignment")
	}
	if r.Format == EMFPlus {
		dn := uint64(u32(b[8:]))
		if dn > n-12 || n-12-dn > 3 || u16(b[2:]) != r.Flags {
			return nil, malformed(r.Offset, "typed EMF+ framing")
		}
		b = b[:12+int(dn)]
	}
	c := cursor{b: b, pos: h, base: r.Offset, limits: l}
	var v any
	switch r.Format {
	case WMF:
		v = c.wmf(r.Type)
	case EMF:
		v = c.emf(r.Type)
	case EMFPlus:
		v = c.emfplus(r.Type, r.Flags)
	}
	if c.err != nil {
		return nil, c.err
	}
	return v, nil
}

type cursor struct {
	b           []byte
	pos, base   int
	limits      DecodeLimits
	err         error
	allocated   uint64
	objectDepth uint32
}

func (c *cursor) allocation(n, width uint64) bool {
	if c.err != nil {
		return false
	}
	if n > c.limits.MaxObjectBytes/width || n > uint64(int(^uint(0)>>1))/width || n*width > c.limits.MaxObjectBytes-c.allocated {
		c.err = failure(c.base+c.pos, "decoded allocation budget", ErrLimit)
		return false
	}
	c.allocated += n * width
	return true
}

func (c *cursor) bad(field string) {
	if c.err == nil {
		c.err = malformed(c.base+c.pos, field)
	}
}
func (c *cursor) unsupported() any {
	if c.err == nil {
		c.err = failure(c.base, "typed record", ErrUnsupported)
	}
	return nil
}
func (c *cursor) take(n uint64) []byte {
	if c.err != nil {
		return nil
	}
	if n > uint64(len(c.b)-c.pos) {
		c.bad("truncated record body")
		return nil
	}
	s := c.pos
	c.pos += int(n)
	return c.b[s:c.pos:c.pos]
}
func (c *cursor) word() uint16 {
	b := c.take(2)
	if b == nil {
		return 0
	}
	return u16(b)
}
func (c *cursor) dword() uint32 {
	b := c.take(4)
	if b == nil {
		return 0
	}
	return u32(b)
}
func (c *cursor) short() int32 { return int32(int16(c.word())) }
func (c *cursor) long() int32  { return int32(c.dword()) }
func (c *cursor) float() float64 {
	v := float64(math.Float32frombits(c.dword()))
	if !finite(v) {
		c.bad("non-finite coordinate")
	}
	return v
}
func (c *cursor) point(short bool) Point {
	if short {
		return Point{float64(c.short()), float64(c.short())}
	}
	return Point{float64(c.long()), float64(c.long())}
}
func (c *cursor) rect() Rect { return Rect{c.long(), c.long(), c.long(), c.long()} }
func (c *cursor) matrix() Matrix {
	return Matrix{c.float(), c.float(), c.float(), c.float(), c.float(), c.float()}
}
func (c *cursor) elements(n uint64, width int) []byte {
	if n > c.limits.MaxElements {
		if c.err == nil {
			c.err = failure(c.base+c.pos, "array element count", ErrLimit)
		}
		return nil
	}
	return c.take(n * uint64(width)) // count comes from a DWORD or a bounded sum
}
func (c *cursor) ints(n uint64, width int) Integers { return Integers{c.elements(n, width), width} }
func (c *cursor) points(n uint64, encoding uint8) Points {
	w := 8
	if encoding == 16 {
		w = 4
	}
	p := Points{data: c.elements(n, w), encoding: encoding}
	if encoding == 1 && c.err == nil {
		for i := 0; i < p.Len(); i++ {
			q := p.At(i)
			if !finite(q.X) || !finite(q.Y) {
				c.bad("non-finite point")
				break
			}
		}
	}
	return p
}
func (c *cursor) span(off, n uint64, minimum, alignment int) []byte {
	if c.err != nil || n == 0 {
		return nil
	}
	if off < uint64(minimum) || off%uint64(alignment) != 0 || off > uint64(len(c.b)) || n > uint64(len(c.b))-off {
		c.bad("record-relative span")
		return nil
	}
	end := int(off + n)
	return c.b[int(off):end:end]
}
