// Copied from github.com/mgilbir/gowemf, file plus_effects.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "fmt"

// EffectGUID uses the MS-DTYP GUID fields. The first three fields are read
// little-endian from packets; Data4 is not byte-swapped.
type EffectGUID struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}

func (g EffectGUID) String() string {
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}", g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1], g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

type EffectKind uint8

const (
	EffectBlur EffectKind = iota + 1
	EffectBrightnessContrast
	EffectColorBalance
	EffectColorCurve
	EffectColorLookupTable
	EffectColorMatrix
	EffectHueSaturationLightness
	EffectLevels
	EffectRedEyeCorrection
	EffectSharpen
	EffectTint
)

var effectGUIDs = [...]EffectGUID{
	{},
	{0x633c80a4, 0x1843, 0x482b, [8]byte{0x9e, 0xf2, 0xbe, 0x28, 0x34, 0xc5, 0xfd, 0xd4}},
	{0xd3a1dbe1, 0x8ec4, 0x4c17, [8]byte{0x9f, 0x4c, 0xea, 0x97, 0xad, 0x1c, 0x34, 0x3d}},
	{0x537e597d, 0x251e, 0x48da, [8]byte{0x96, 0x64, 0x29, 0xca, 0x49, 0x6b, 0x70, 0xf8}},
	{0xdd6a0022, 0x58e4, 0x4a67, [8]byte{0x9d, 0x9b, 0xd4, 0x8e, 0xb8, 0x81, 0xa5, 0x3d}},
	{0xa7ce72a9, 0x0f7f, 0x40d7, [8]byte{0xb3, 0xcc, 0xd0, 0xc0, 0x2d, 0x5c, 0x32, 0x12}},
	{0x718f2615, 0x7933, 0x40e3, [8]byte{0xa5, 0x11, 0x5f, 0x68, 0xfe, 0x14, 0xdd, 0x74}},
	{0x8b2dd6c3, 0xeb07, 0x4d87, [8]byte{0xa5, 0xf0, 0x71, 0x08, 0xe2, 0x6a, 0x9c, 0x5f}},
	{0x99c354ec, 0x2a31, 0x4f3a, [8]byte{0x8c, 0x34, 0x17, 0xa8, 0x03, 0xb3, 0x3a, 0x25}},
	{0x74d29d05, 0x69a4, 0x4266, [8]byte{0x95, 0x49, 0x3c, 0xc5, 0x28, 0x36, 0xb6, 0x32}},
	{0x63cbf3ee, 0xc526, 0x402c, [8]byte{0x8f, 0x71, 0x62, 0xc5, 0x40, 0xbf, 0x51, 0x42}},
	{0x1077af00, 0x2848, 0x4441, [8]byte{0x94, 0x89, 0x44, 0xad, 0x4c, 0x2d, 0x7a, 0x2c}},
}

// GUID returns the specification identifier; an unknown kind returns the zero GUID.
func (k EffectKind) GUID() EffectGUID {
	if k == 0 || int(k) >= len(effectGUIDs) {
		return EffectGUID{}
	}
	return effectGUIDs[k]
}

// PlusEffect carries a decoded serialized parameter block. It describes an
// effect; it does not apply the effect to pixels. Lookup tables and rectangle
// arrays borrow the source record and must remain immutable.
type PlusEffect struct {
	GUID       EffectGUID
	Kind       EffectKind
	Parameters any
}
type BlurEffect struct {
	Radius     float64
	ExpandEdge bool
}
type BrightnessContrastEffect struct{ Brightness, Contrast int32 }
type ColorBalanceEffect struct{ CyanRed, MagentaGreen, YellowBlue int32 }
type ColorCurveEffect struct {
	Adjustment, Channel uint32
	Intensity           int32
}
type ColorLookupTableEffect struct{ Blue, Green, Red, Alpha []byte }

// ColorMatrixEffect retains the five consecutive wire rows. Rows[4][0:4] are
// translations; Rows[0:4][4] must be zero. In the specification's field names,
// Rows[row][column] corresponds to Matrix_column_row.
type ColorMatrixEffect struct{ Rows [5][5]float64 }
type HueSaturationLightnessEffect struct{ Hue, Saturation, Lightness int32 }
type LevelsEffect struct{ Highlight, Midtone, Shadow int32 }
type SharpenEffect struct{ Radius, Amount float64 }
type TintEffect struct{ Hue, Amount int32 }
type RedEyeCorrectionEffect struct{ Areas EffectRectangles }
type EffectRectangles struct{ data []byte }

func (r EffectRectangles) Len() int { return len(r.data) / 16 }
func (r EffectRectangles) At(i int) Rect {
	if i < 0 || i >= r.Len() {
		panic("gowemf: effect rectangle index out of range")
	}
	return rect(r.data[i*16:])
}

func (c *cursor) serializableEffect() PlusEffect {
	v := PlusEffect{}
	raw := c.take(16)
	if c.err != nil {
		return v
	}
	v.GUID = EffectGUID{Data1: u32(raw), Data2: u16(raw[4:]), Data3: u16(raw[6:])}
	copy(v.GUID.Data4[:], raw[8:])
	n := uint64(c.dword())
	if c.err != nil {
		return v
	}
	if n%4 != 0 || n != uint64(len(c.b)-c.pos) {
		c.bad("serialized effect buffer size")
		return v
	}
	for kind := EffectBlur; int(kind) < len(effectGUIDs); kind++ {
		if v.GUID == kind.GUID() {
			v.Kind = kind
			break
		}
	}
	switch v.Kind {
	case EffectBlur:
		e := BlurEffect{Radius: c.float()}
		expand := c.dword()
		e.ExpandEdge = expand != 0
		if e.Radius < 0 || e.Radius > 255 || expand > 1 {
			c.bad("blur effect parameters")
		}
		v.Parameters = e
	case EffectBrightnessContrast:
		v.Parameters = BrightnessContrastEffect{c.effectInt(-255, 255), c.effectInt(-100, 100)}
	case EffectColorBalance:
		v.Parameters = ColorBalanceEffect{c.effectInt(-100, 100), c.effectInt(-100, 100), c.effectInt(-100, 100)}
	case EffectColorCurve:
		e := ColorCurveEffect{Adjustment: c.dword(), Channel: c.dword()}
		if e.Adjustment > 7 || e.Channel > 3 {
			c.bad("color-curve selector")
		}
		lo, hi := int32(-100), int32(100)
		if e.Adjustment <= 1 {
			lo, hi = -255, 255
		} else if e.Adjustment >= 6 {
			lo, hi = 0, 255
		}
		e.Intensity = c.effectInt(lo, hi)
		v.Parameters = e
	case EffectColorLookupTable:
		v.Parameters = ColorLookupTableEffect{c.take(256), c.take(256), c.take(256), c.take(256)}
	case EffectColorMatrix:
		e := ColorMatrixEffect{}
		for row := range e.Rows {
			for col := range e.Rows[row] {
				e.Rows[row][col] = c.float()
			}
			if row < 4 && e.Rows[row][4] != 0 {
				c.bad("color-matrix affine column")
			}
		}
		v.Parameters = e
	case EffectHueSaturationLightness:
		v.Parameters = HueSaturationLightnessEffect{c.effectInt(-180, 180), c.effectInt(-100, 100), c.effectInt(-100, 100)}
	case EffectLevels:
		v.Parameters = LevelsEffect{c.effectInt(0, 100), c.effectInt(-100, 100), c.effectInt(0, 100)}
	case EffectRedEyeCorrection:
		count := c.long()
		if count < 0 {
			c.bad("negative red-eye rectangle count")
			return v
		}
		v.Parameters = RedEyeCorrectionEffect{EffectRectangles{c.elements(uint64(count), 16)}}
	case EffectSharpen:
		e := SharpenEffect{c.float(), c.float()}
		if e.Radius < 0 || e.Amount < 0 || e.Amount > 100 {
			c.bad("sharpen effect parameters")
		}
		v.Parameters = e
	case EffectTint:
		v.Parameters = TintEffect{c.effectInt(-180, 180), c.effectInt(-100, 100)}
	default:
		c.unsupported()
	}
	return v
}

func (c *cursor) effectInt(low, high int32) int32 {
	v := c.long()
	if v < low || v > high {
		c.bad("effect parameter range")
	}
	return v
}
