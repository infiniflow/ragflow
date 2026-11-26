// Copied from github.com/mgilbir/gowemf, file plus_objects.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "math"

type PlusBrush struct {
	Texture                       *PlusTextureBrush
	PathGradient                  *PlusPathGradient
	Type, Color, Hatch, BackColor uint32
	LinearGradient                *PlusLinearGradient
}
type PlusFont struct {
	EmSize      float64
	Unit, Style uint32
	FamilyUTF16 []byte
}
type PlusPath struct {
	Flags  uint32
	Points Points
	Types  []byte
}
type PlusImage struct {
	// AlignedPlaceable identifies the observed 24-byte GDI+ serialization of a
	// 22-byte placeable header. Use MetafileBytes to obtain standard WMF framing.
	AlignedPlaceable                            bool
	Type, MetafileType, PixelFormat, BitmapType uint32
	Width, Height, Stride                       int32
	Data                                        []byte
}
type PlusImageAttributes struct{ WrapMode, ClampColor, ObjectClamp uint32 }
type PlusPen struct {
	CustomStartCap, CustomEndCap                          *PlusCustomLineCap
	Flags, Unit                                           uint32
	Width                                                 float64
	Transform                                             Matrix
	StartCap, EndCap, Join, LineStyle, DashCap, Alignment uint32
	MiterLimit, DashOffset                                float64
	Dashes, Compound                                      []float64
	Brush                                                 PlusBrush
}
type PlusStringFormat struct {
	Flags, Language, Alignment, LineAlignment, DigitSubstitution, DigitLanguage uint32
	FirstTabOffset                                                              float64
	HotkeyPrefix                                                                uint32
	LeadingMargin, TrailingMargin, Tracking                                     float64
	Trimming                                                                    uint32
	TabStops                                                                    []float64
	Ranges                                                                      Integers // pairs of signed first-character index and length
}

// DecodePlusObject decodes a complete EMF+ object, using the ObjectType value
// from its record flags. Fragment bytes must first be assembled. Embedded
// metafiles are exposed, not recursively parsed; the caller owns nesting policy.
func DecodePlusObject(typ uint8, data []byte, limits DecodeLimits) (any, error) {
	l := limits.defaults()
	v, _, err := decodePlusObject(typ, data, l, 0, 1, 0)
	return v, err
}

func decodePlusObject(typ uint8, data []byte, l DecodeLimits, base int, depth uint32, allocated uint64) (any, uint64, error) {
	if depth > l.MaxNesting {
		return nil, allocated, failure(base, "EMF+ object nesting", ErrLimit)
	}
	if uint64(len(data)) > l.MaxObjectBytes {
		return nil, allocated, failure(base, "EMF+ object bytes", ErrLimit)
	}
	c := cursor{b: data, limits: l, base: base, objectDepth: depth, allocated: allocated}
	version := c.dword()
	if version>>12 != 0xdbc01 {
		c.bad("EMF+ object graphics signature")
	}
	var v any
	switch typ {
	case 1:
		v = c.plusBrush()
	case 2:
		v = c.plusPen()
	case 3:
		n := uint64(c.dword())
		flags := c.dword()
		p := PlusPath{Flags: flags}
		// MS-EMFPLUS 2.2.1.6 defines only R (0x0800) and C (0x4000). Writers
		// set other bits (0x2000 is common) without changing the layout, so
		// they are retained in Flags but do not select an encoding.
		flags &= 0x4800
		p.Points = c.plusPoints(n, uint16(flags))
		if flags&0x800 == 0 {
			p.Types = c.elements(n, 1)
		} else if c.err == nil {
			if !c.allocation(n, 1) {
				return nil, c.allocated, c.err
			}
			p.Types = make([]byte, int(n))
			used := 0
			for used < len(p.Types) && c.err == nil {
				run := c.take(2)
				if run == nil {
					break
				}
				count := int(run[0] & 63)
				if run[0]&64 == 0 || count == 0 || count > len(p.Types)-used {
					c.bad("EMF+ path type run")
					break
				}
				kind := run[1] & 15
				if (kind == 3 && run[0]&128 == 0) || (kind == 1 && run[0]&128 != 0) {
					c.bad("inconsistent path type run Bezier flag")
					break
				}
				for i := 0; i < count; i++ {
					p.Types[used+i] = run[1]
				}
				used += count
			}
		}
		if c.err == nil {
			if err := p.Validate(); err != nil {
				c.bad(err.(*ParseError).Field)
			}
		}
		v = p
	case 5:
		v = c.plusImage()
	case 4:
		v = c.plusRegion()
	case 6:
		f := PlusFont{EmSize: c.float(), Unit: c.dword(), Style: c.dword()}
		c.dword()
		f.FamilyUTF16 = c.elements(uint64(c.dword()), 2)
		v = f
	case 7:
		f := PlusStringFormat{Flags: c.dword(), Language: c.dword(), Alignment: c.dword(), LineAlignment: c.dword(), DigitSubstitution: c.dword(), DigitLanguage: c.dword(), FirstTabOffset: c.float(), HotkeyPrefix: c.dword(), LeadingMargin: c.float(), TrailingMargin: c.float(), Tracking: c.float(), Trimming: c.dword()}
		tabs, ranges := c.long(), c.long()
		if tabs < 0 || ranges < 0 {
			c.bad("negative EMF+ string-format count")
		} else {
			f.TabStops = c.floats(uint64(tabs))
			f.Ranges = c.ints(uint64(ranges)*2, 4)
		}
		v = f
	case 8:
		c.dword()
		v = PlusImageAttributes{c.dword(), c.dword(), c.dword()}
		c.dword()
	case 9:
		v = c.customLineCap()
	default:
		v = c.unsupported()
	}
	if c.err == nil && len(c.b)-c.pos > 3 {
		c.bad("extra EMF+ object data")
	}
	if c.err != nil {
		return nil, c.allocated, c.err
	}
	return v, c.allocated, nil
}

// childObject shares the enclosing object's expanded-allocation budget and
// keeps errors relative to the outer input. Taking the span and checking depth
// happens before decoding; no nested decoder gets a fresh allocation allowance.
func (c *cursor) childObject(typ uint8, n uint64, depth uint32) any {
	start := c.pos
	raw := c.take(n)
	if c.err != nil {
		return nil
	}
	v, allocated, err := decodePlusObject(typ, raw, c.limits, c.base+start, depth, c.allocated)
	if err != nil {
		c.err = err
		return nil
	}
	c.allocated = allocated
	return v
}

func (c *cursor) plusBrush() PlusBrush {
	b := PlusBrush{Type: c.dword()}
	switch b.Type {
	case 0:
		b.Color = c.dword()
	case 1:
		b.Hatch = c.dword()
		b.Color = c.dword()
		b.BackColor = c.dword()
	case 4:
		b.LinearGradient = c.linearGradient()
	case 2:
		b.Texture = c.textureBrush()
	case 3:
		b.PathGradient = c.pathGradient()
	default:
		c.unsupported()
	}
	return b
}

func (c *cursor) plusPen() PlusPen {
	if c.dword() != 0 {
		c.bad("EMF+ pen type")
	}
	p := PlusPen{Flags: c.dword(), Unit: c.dword(), Width: c.float(), Transform: Identity()}
	if p.Flags & ^uint32(0x1fff) != 0 {
		c.unsupported()
		return p
	}
	if p.Flags&1 != 0 {
		p.Transform = c.matrix()
	}
	if p.Flags&2 != 0 {
		p.StartCap = c.dword()
	}
	if p.Flags&4 != 0 {
		p.EndCap = c.dword()
	}
	if p.Flags&8 != 0 {
		p.Join = c.dword()
	}
	if p.Flags&16 != 0 {
		p.MiterLimit = c.float()
	}
	if p.Flags&32 != 0 {
		p.LineStyle = c.dword()
	}
	if p.Flags&64 != 0 {
		p.DashCap = c.dword()
	}
	if p.Flags&128 != 0 {
		p.DashOffset = c.float()
	}
	if p.Flags&256 != 0 {
		p.Dashes = c.floats(uint64(c.dword()))
	}
	if p.Flags&512 != 0 {
		p.Alignment = c.dword()
	}
	if p.Flags&1024 != 0 {
		p.Compound = c.floats(uint64(c.dword()))
	}
	if p.Flags&0x800 != 0 {
		if v := c.childObject(9, uint64(c.dword()), c.objectDepth+1); v != nil {
			cap := v.(PlusCustomLineCap)
			p.CustomStartCap = &cap
		}
	}
	if p.Flags&0x1000 != 0 {
		if v := c.childObject(9, uint64(c.dword()), c.objectDepth+1); v != nil {
			cap := v.(PlusCustomLineCap)
			p.CustomEndCap = &cap
		}
	}
	if v := c.childObject(1, uint64(len(c.b)-c.pos), c.objectDepth+1); v != nil {
		p.Brush = v.(PlusBrush)
	}
	return p
}

func (c *cursor) floats(n uint64) []float64 {
	b := c.elements(n, 4)
	if c.err != nil {
		return nil
	}
	if !c.allocation(n, 8) {
		return nil
	}
	v := make([]float64, int(n))
	for i := range v {
		v[i] = float64(math.Float32frombits(u32(b[i*4:])))
		if !finite(v[i]) {
			c.bad("non-finite array value")
			return nil
		}
	}
	return v
}

func (c *cursor) plusImage() PlusImage {
	p := PlusImage{Type: c.dword()}
	switch p.Type {
	case 1:
		p.Width, p.Height, p.Stride = c.long(), c.long(), c.long()
		p.PixelFormat, p.BitmapType = c.dword(), c.dword()
		if p.BitmapType > 1 {
			c.bad("EMF+ bitmap encoding")
		}
		p.Data = c.take(uint64(len(c.b) - c.pos))
		if p.BitmapType == 0 {
			if c.err == nil {
				if _, err := p.bitmapLayout(c.limits.MaxElements); err != nil {
					if pe, ok := err.(*ParseError); ok {
						c.err = &ParseError{c.base + c.pos - len(p.Data) + pe.Offset, pe.Field, pe.Err}
					} else {
						c.err = err
					}
				}
			}
		}
	case 2:
		p.MetafileType = c.dword()
		if p.MetafileType < 1 || p.MetafileType > 5 {
			c.bad("EMF+ embedded metafile type")
		}
		n := uint64(c.dword())
		if p.MetafileType == 2 && c.err == nil {
			r := c.b[c.pos:]
			// Observed in pinned POI nested_wmf.emf. Require both the WMF
			// header and its own word count to corroborate the interpretation;
			// never treat arbitrary trailing object bytes as an embedded file.
			if len(r) >= 42 && u32(r) == 0x9ac6cdd7 && u16(r[24:]) >= 1 && u16(r[24:]) <= 2 && u16(r[26:]) == 9 && uint64(u32(r[30:]))*2 == n && n+24 <= uint64(len(r)) && uint64(len(r))-n-24 <= 3 {
				n += 24
				p.AlignedPlaceable = true
			}
		}
		p.Data = c.take(n)
	default:
		c.unsupported()
	}
	return p
}

// MetafileBytes returns a complete embedded metafile without parsing it. Normal
// encodings borrow Data; the aligned-placeable variant is normalized into a new
// buffer. The byte limit is enforced even for caller-constructed PlusImage values.
// Consumers must bound recursive nesting separately.
func (p PlusImage) MetafileBytes(limits Limits) ([]byte, error) {
	if p.Type != 2 {
		return nil, failure(0, "not an embedded metafile", ErrFormat)
	}
	if uint64(len(p.Data)) > limits.defaults().MaxBytes {
		return nil, failure(0, "embedded metafile bytes", ErrLimit)
	}
	if !p.AlignedPlaceable {
		return p.Data[:len(p.Data):len(p.Data)], nil
	}
	if len(p.Data) < 42 || u32(p.Data) != 0x9ac6cdd7 {
		return nil, malformed(0, "aligned placeable header")
	}
	out := make([]byte, len(p.Data)-2)
	copy(out, p.Data[:22])
	copy(out[22:], p.Data[24:])
	return out, nil
}

// PlusAssembler assembles one continued object at a time. It accepts intervening
// non-object records outside Add; the next object fragment must have the same
// ID/type. Bytes are bounded cumulatively, not allocated from TotalSize. The zero
// value uses default limits. An error resets pending state so no partial object
// can subsequently be mistaken for a completed one. It is not concurrency-safe.
type PlusAssembler struct {
	Limits  DecodeLimits
	pending []byte
	id, typ uint8
	total   uint32
	active  bool
}

// Add returns nil while continuation is pending. An object completes when its
// payload reaches TotalSize, even if that fragment is still marked Continued.
// A completed object owns an
// independent byte slice when assembled; single-record objects borrow f.Data.
func (a *PlusAssembler) Add(f PlusObjectFragment) (data []byte, err error) {
	defer func() {
		if err != nil {
			a.Reset()
		}
	}()
	l := a.Limits.defaults()
	if f.ID > 63 || f.Type < 1 || f.Type > 9 {
		return nil, malformed(0, "EMF+ fragment identity")
	}
	if !a.active && !f.Continued {
		if uint64(len(f.Data)) > l.MaxObjectBytes {
			return nil, failure(0, "EMF+ object bytes", ErrLimit)
		}
		if len(f.Data) == 0 {
			return nil, malformed(0, "empty EMF+ object")
		}
		return f.Data, nil
	}
	if !a.active {
		if f.TotalSize == 0 {
			return nil, malformed(0, "EMF+ total object size")
		}
		a.active = true
		a.id = f.ID
		a.typ = f.Type
		a.total = f.TotalSize
	}
	if a.id != f.ID || a.typ != f.Type || (f.Continued && a.total != f.TotalSize) {
		return nil, malformed(0, "inconsistent EMF+ continuation")
	}
	n := uint64(len(a.pending)) + uint64(len(f.Data))
	if uint64(a.total) > l.MaxObjectBytes || uint64(a.total) > uint64(int(^uint(0)>>1)) {
		return nil, failure(0, "EMF+ object bytes", ErrLimit)
	}
	// The object ends once TotalObjectSize bytes have been read (MS-EMFPLUS
	// 2.3.5.1), whether or not the final record clears C, as the spec says
	// it should: writers set C on every record. Only a record without C may
	// end short of the total, and the final record can add DWORD padding.
	complete := n >= uint64(a.total)
	if (!complete && !f.Continued) || (complete && n-uint64(a.total) > 3) {
		return nil, malformed(0, "EMF+ continuation length")
	}
	if complete {
		n = uint64(a.total)
	}
	if n > uint64(cap(a.pending)) {
		capacity := uint64(cap(a.pending)) * 2
		if capacity < n {
			capacity = n
		}
		if capacity > uint64(a.total) {
			capacity = uint64(a.total)
		}
		if capacity > uint64(int(^uint(0)>>1)) {
			capacity = uint64(int(^uint(0) >> 1))
		}
		grown := make([]byte, len(a.pending), int(capacity))
		copy(grown, a.pending)
		a.pending = grown
	}
	a.pending = append(a.pending, f.Data[:int(n)-len(a.pending)]...)
	if !complete {
		return nil, nil
	}
	data = a.pending
	a.Reset()
	return data, nil
}
func (a *PlusAssembler) Finish() error {
	if a.active {
		return malformed(0, "unfinished EMF+ object")
	}
	return nil
}
func (a *PlusAssembler) Reset() { a.pending = nil; a.active = false; a.total = 0; a.id = 0; a.typ = 0 }
