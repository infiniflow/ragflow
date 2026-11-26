// Copied from github.com/mgilbir/gowemf, file decode_color.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

type ColorSpaceObject struct {
	Handle, Flags uint32
	Space         ColorSpace
	Data          []byte
}
type ColorProfile struct {
	Action, Flags uint32
	Unicode       bool
	Name, Data    []byte
}
type ColorAdjustment struct {
	Flags, Illuminant, RedGamma, GreenGamma, BlueGamma, ReferenceBlack, ReferenceWhite uint16
	Contrast, Brightness, Colorfulness, RedGreenTint                                   int16
}

func (c *cursor) colorSpaceObject(unicode bool) ColorSpaceObject {
	v := ColorSpaceObject{Handle: c.dword()}
	start := c.pos
	signature, version, size := c.dword(), c.dword(), c.dword()
	want := uint32(328)
	nameBytes := uint64(260)
	if unicode {
		want = 588
		nameBytes = 520
	}
	if signature != 0x50534f43 || version != 0x400 || size != want {
		c.bad("logical color-space header")
	}
	v.Space = ColorSpace{Type: c.dword(), Intent: c.dword(), Unicode: unicode}
	for i := range v.Space.Endpoints {
		v.Space.Endpoints[i] = XYZ{float64(c.long()) / (1 << 30), float64(c.long()) / (1 << 30), float64(c.long()) / (1 << 30)}
	}
	for i := range v.Space.Gamma {
		v.Space.Gamma[i] = float64(c.dword()) / 65536
	}
	v.Space.Name = c.take(nameBytes)
	if v.Space.Type != ColorCalibratedRGB && !v.Space.IsSRGB() {
		c.unsupported()
	}
	if _, err := v.Space.ICCIntent(); err != nil || v.Space.Intent == 0 {
		c.bad("logical color-space intent")
	}
	if c.err == nil && c.pos-start != int(size) {
		c.bad("logical color-space size")
	}
	if unicode {
		v.Flags = c.dword()
		v.Data = c.take(uint64(c.dword()))
		if v.Flags & ^uint32(1) != 0 {
			c.bad("color-space profile flags")
		}
		if v.Flags&1 != 0 {
			v.Space.Profile = v.Data
		}
	}
	return v
}

func (c *cursor) colorProfile(typ uint32) ColorProfile {
	v := ColorProfile{Unicode: typ != EMRSetICMProfileA}
	if typ == EMRColorMatchToTargetW {
		v.Action = c.dword()
		if v.Action < 1 || v.Action > 3 {
			c.bad("color-match action")
		}
	}
	v.Flags = c.dword()
	name, data := uint64(c.dword()), uint64(c.dword())
	if v.Unicode && name%2 != 0 {
		c.bad("UTF-16 color-profile name length")
	}
	if typ == EMRColorMatchToTargetW && v.Flags > 1 {
		c.bad("color-match flags")
	}
	v.Name = c.take(name)
	v.Data = c.take(data)
	return v
}

func (c *cursor) colorAdjustment() ColorAdjustment {
	if c.word() != 24 {
		c.bad("color-adjustment size")
	}
	v := ColorAdjustment{c.word(), c.word(), c.word(), c.word(), c.word(), c.word(), c.word(), int16(c.word()), int16(c.word()), int16(c.word()), int16(c.word())}
	// Recommended gamma/reference/tone ranges are SHOULDs, not wire validity
	// bounds. Preserve them for the renderer instead of silently clamping them.
	if v.Flags & ^uint16(3) != 0 || v.Illuminant > 8 {
		c.bad("color-adjustment flags or illuminant")
	}
	return v
}
