// Copied from github.com/mgilbir/gowemf, file plus_terminal.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// PlusTSGraphics is a terminal-server graphics-state snapshot. Palette entries
// are ARGB DWORDs; reserved record flag bits have no effect on decoded fields.
type PlusTSGraphics struct {
	AntiAliasMode, TextRenderHint, CompositingMode, CompositingQuality byte
	Origin                                                             Point
	TextContrast                                                       uint16
	FilterType, PixelOffset                                            byte
	WorldToDevice                                                      Matrix
	VGAOnly, HasPalette                                                bool
	PaletteFlags                                                       uint32
	Palette                                                            Integers
}

// PlusTSClip holds expanded rectangle differences. C=1 uses signed seven-bit
// coordinates; C=0 uses signed fifteen-bit coordinates. Bottom is relative to
// the current top, while left/top/right are relative to the previous rectangle.
type PlusTSClip struct{ Rectangles []Rect }

func (c *cursor) tsGraphics(flags uint16) PlusTSGraphics {
	v := PlusTSGraphics{VGAOnly: flags&2 != 0, HasPalette: flags&1 != 0}
	modes := c.take(4)
	if modes == nil {
		return v
	}
	v.AntiAliasMode, v.TextRenderHint, v.CompositingMode, v.CompositingQuality = modes[0], modes[1], modes[2], modes[3]
	v.Origin = c.point(true)
	v.TextContrast = c.word()
	quality := c.take(2)
	if quality == nil {
		return v
	}
	v.FilterType, v.PixelOffset = quality[0], quality[1]
	v.WorldToDevice = c.matrix()
	if v.AntiAliasMode > 5 || v.TextRenderHint > 5 || v.CompositingMode > 1 || v.CompositingQuality < 1 || v.CompositingQuality > 5 || v.TextContrast > 12 || v.FilterType > 7 || v.FilterType == 5 || v.PixelOffset > 4 {
		c.bad("terminal graphics mode")
	}
	if v.HasPalette {
		v.PaletteFlags = c.dword()
		if v.PaletteFlags & ^uint32(7) != 0 {
			c.bad("terminal palette flags")
		}
		v.Palette = c.ints(uint64(c.dword()), 4)
	}
	return v
}

func (c *cursor) tsClip(flags uint16) PlusTSClip {
	v := PlusTSClip{}
	n := uint64(flags & 0x7fff)
	small := flags&0x8000 != 0
	width := uint64(8)
	if small {
		width = 4
	}
	if n > c.limits.MaxElements {
		c.err = failure(c.base+c.pos, "terminal rectangle count", ErrLimit)
		return v
	}
	if n*width != uint64(len(c.b)-c.pos) {
		c.bad("terminal rectangle data size")
		return v
	}
	if !c.allocation(n, 16) {
		return v
	}
	v.Rectangles = make([]Rect, int(n))
	var previous Rect
	coordinate := func() int32 {
		b := c.take(1)
		if b == nil {
			return 0
		}
		if small {
			if b[0]&128 == 0 {
				c.bad("terminal seven-bit marker")
			}
			return int32(int8(b[0]<<1) >> 1)
		}
		if b[0]&128 != 0 {
			c.bad("terminal fifteen-bit marker")
			return 0
		}
		low := c.take(1)
		if low == nil {
			return 0
		}
		return int32(int16((uint16(b[0])<<8|uint16(low[0]))<<1) >> 1)
	}
	for i := range v.Rectangles {
		// At most 32767 rectangles and signed 15-bit differences keep all
		// cumulative coordinates within int32, including current-top + height.
		r := Rect{Left: previous.Left + coordinate(), Top: previous.Top + coordinate(), Right: previous.Right + coordinate()}
		r.Bottom = r.Top + coordinate()
		if c.err != nil {
			return v
		}
		v.Rectangles[i] = r
		previous = r
	}
	return v
}
