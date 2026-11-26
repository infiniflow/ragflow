// Copied from github.com/mgilbir/gowemf, file plus_caps.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// PlusCustomLineCap represents either a path-based cap (Type 0) or an adjustable
// arrow (Type 1). It can appear as object type 9 or inline in a pen. Dimensions
// and scales are retained for the renderer; no geometry is rasterized here.
type PlusCustomLineCap struct {
	Type    uint32
	Default *PlusCustomCapData
	Arrow   *PlusArrowCap
}

type PlusCustomCapData struct {
	Flags, BaseCap                           uint32
	BaseInset                                float64
	StrokeStartCap, StrokeEndCap, StrokeJoin uint32
	StrokeMiterLimit, WidthScale             float64
	FillPath, StrokePath                     *PlusPath
}

type PlusArrowCap struct {
	Width, Height, MiddleInset         float64
	Filled                             bool
	LineStartCap, LineEndCap, LineJoin uint32
	LineMiterLimit, WidthScale         float64
}

func validPlusLineCap(v uint32) bool {
	return v <= 3 || (v >= 0x10 && v <= 0x14) || v == 0xf0 || v == 0xff
}

func (c *cursor) customLineCap() PlusCustomLineCap {
	v := PlusCustomLineCap{Type: c.dword()}
	switch v.Type {
	case 0:
		d := &PlusCustomCapData{Flags: c.dword(), BaseCap: c.dword(), BaseInset: c.float(), StrokeStartCap: c.dword(), StrokeEndCap: c.dword(), StrokeJoin: c.dword(), StrokeMiterLimit: c.float(), WidthScale: c.float()}
		v.Default = d
		if d.Flags & ^uint32(3) != 0 || !validPlusLineCap(d.BaseCap) || !validPlusLineCap(d.StrokeStartCap) || !validPlusLineCap(d.StrokeEndCap) || d.StrokeJoin > 3 {
			c.bad("custom line-cap flags/styles")
		}
		c.capHotSpots()
		if d.Flags&1 != 0 {
			d.FillPath = c.capPath()
		}
		if d.Flags&2 != 0 {
			d.StrokePath = c.capPath()
		}
	case 1:
		a := &PlusArrowCap{Width: c.float(), Height: c.float(), MiddleInset: c.float(), Filled: c.dword() != 0, LineStartCap: c.dword(), LineEndCap: c.dword(), LineJoin: c.dword(), LineMiterLimit: c.float(), WidthScale: c.float()}
		v.Arrow = a
		if !validPlusLineCap(a.LineStartCap) || !validPlusLineCap(a.LineEndCap) || a.LineJoin > 3 {
			c.bad("arrow-cap styles")
		}
		c.capHotSpots()
	default:
		c.unsupported()
	}
	return v
}

func (c *cursor) capHotSpots() {
	for i := 0; i < 4; i++ {
		if c.float() != 0 {
			c.bad("custom-cap hotspot must be zero")
		}
	}
}

func (c *cursor) capPath() *PlusPath {
	n := c.long()
	if n < 0 {
		c.bad("negative custom-cap path size")
		return nil
	}
	v := c.childObject(3, uint64(n), c.objectDepth+1)
	if v == nil {
		return nil
	}
	p := v.(PlusPath)
	return &p
}
