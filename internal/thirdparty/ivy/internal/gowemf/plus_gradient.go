// Copied from github.com/mgilbir/gowemf, file plus_gradient.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

type PlusLinearGradient struct {
	Flags, WrapMode      uint32
	Rect                 Box
	StartColor, EndColor uint32
	Transform            Matrix
	PresetPositions      []float64
	PresetColors         Integers
	Horizontal, Vertical PlusBlendFactors
}
type PlusBlendFactors struct{ Positions, Factors []float64 }

func (c *cursor) linearGradient() *PlusLinearGradient {
	g := &PlusLinearGradient{Flags: c.dword(), WrapMode: c.dword(), Rect: c.plusBox(false), StartColor: c.dword(), EndColor: c.dword(), Transform: Identity()}
	c.dword()
	c.dword()
	if g.Flags & ^uint32(0x9e) != 0 {
		c.unsupported()
		return g
	}
	if g.WrapMode > 4 {
		c.bad("gradient wrap mode")
	}
	if g.Flags&4 != 0 && g.Flags&0x18 != 0 {
		c.bad("conflicting gradient blend patterns")
	}
	if g.Flags&2 != 0 {
		g.Transform = c.matrix()
	}
	if g.Flags&4 != 0 {
		n := uint64(c.dword())
		g.PresetPositions = c.floats(n)
		g.PresetColors = c.ints(n, 4)
		c.proportions(g.PresetPositions)
	}
	// MS-EMFPLUS 2.2.2.25 specifies vertical before horizontal when both
	// independent blend-factor arrays are present.
	if g.Flags&0x10 != 0 {
		g.Vertical = c.blendFactors()
	}
	if g.Flags&8 != 0 {
		g.Horizontal = c.blendFactors()
	}
	return g
}
func (c *cursor) blendFactors() PlusBlendFactors {
	n := uint64(c.dword())
	b := PlusBlendFactors{c.floats(n), c.floats(n)}
	c.proportions(b.Positions)
	c.proportions(b.Factors)
	if c.err == nil && (n < 2 || b.Positions[0] != 0 || b.Positions[len(b.Positions)-1] != 1) {
		c.bad("gradient factor endpoints")
	}
	return b
}
func (c *cursor) proportions(values []float64) {
	for _, v := range values {
		if v < 0 || v > 1 {
			c.bad("gradient proportion")
			return
		}
	}
}
