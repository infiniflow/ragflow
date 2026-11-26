// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/trc.rs.

package moxcms

import (
	"math"
	"slices"
)

// Port: upstream makes the tables of this file generic over the type of a
// sample. The subset has samples of 8 bits, and the curves of LUTs that
// upstream reads as samples of 13 bits. Both are integer types, so the port
// keeps only the branches of upstream for integer samples. A table of gamma
// values holds u8, because only the transforms of 8 bits use one.

// ToneReprCurveKind is the variant of ToneReprCurve in moxcms src/trc.rs.
type ToneReprCurveKind uint8

// The variants of ToneReprCurve in moxcms src/trc.rs.
const (
	// CurveLut is ToneReprCurve::Lut. The curve is a table.
	CurveLut ToneReprCurveKind = iota
	// CurveParametric is ToneReprCurve::Parametric. The curve is a function
	// of ICC.1 with parameters.
	CurveParametric
)

// ToneReprCurve is ToneReprCurve in moxcms src/trc.rs. It is a tone curve.
// Lut holds the table of a CurveLut. An empty table is the identity, and a
// table of one entry is a gamma in u8Fixed8Number. Parametric holds the
// parameters of a CurveParametric.
//
// Port: upstream is an enum with data. The port makes it a struct with a
// kind.
type ToneReprCurve struct {
	Kind       ToneReprCurveKind
	Lut        []uint16
	Parametric []float32
}

// equal is impl PartialEq for ToneReprCurve in moxcms src/trc.rs. It
// reports whether c and o are the same curve. Two nil curves are equal.
func (c *ToneReprCurve) equal(o *ToneReprCurve) bool {
	if c == nil || o == nil {
		return c == o
	}
	if c.Kind != o.Kind {
		return false
	}
	if c.Kind == CurveLut {
		return slices.Equal(c.Lut, o.Lut)
	}
	return slices.Equal(c.Parametric, o.Parametric)
}

// curveFromGamma is curve_from_gamma in moxcms src/trc.rs. It returns the
// curve of a pure gamma.
func curveFromGamma(gamma float32) *ToneReprCurve {
	return &ToneReprCurve{Lut: []uint16{toU8Fixed8(gamma)}}
}

// ParametricCurve is ParametricCurve in moxcms src/trc.rs. It holds the
// seven parameters of the function of type 4 of ICC.1, to which each
// function of ICC.1 converts.
type ParametricCurve struct {
	G, A, B, C, D, E, F float32
}

// newParametricCurve is ParametricCurve::new in moxcms src/trc.rs. It
// reports false when params has a number of entries that no function has.
//
// Port: upstream panics on an empty params. The port reports false.
func newParametricCurve(params []float32) (ParametricCurve, bool) {
	if len(params) == 0 {
		return ParametricCurve{}, false
	}
	g := params[0]
	p := params[1:]
	switch len(p) {
	case 0:
		return ParametricCurve{G: g, A: 1}, true
	case 2:
		a, b := p[0], p[1]
		return ParametricCurve{G: g, A: a, B: b, D: -b / a}, true
	case 3:
		a, b, c := p[0], p[1], p[2]
		return ParametricCurve{G: g, A: a, B: b, D: -b / a, E: c, F: c}, true
	case 4:
		return ParametricCurve{G: g, A: p[0], B: p[1], C: p[2], D: p[3]}, true
	case 6:
		return ParametricCurve{G: g, A: p[0], B: p[1], C: p[2], D: p[3], E: p[4], F: p[5]}, true
	default:
		return ParametricCurve{}, false
	}
}

// isLinear is ParametricCurve::is_linear in moxcms src/trc.rs.
func (c ParametricCurve) isLinear() bool {
	return absF32(c.G-1) < 1e-5 && absF32(c.A-1) < 1e-5 && absF32(c.B) < 1e-5 && absF32(c.C) < 1e-5
}

// Eval is ParametricCurve::eval in moxcms src/trc.rs.
func (c ParametricCurve) Eval(x float32) float32 {
	if x < c.D {
		return float32(c.C*x) + c.F
	}
	return powF32(float32(c.A*x)+c.B, c.G) + c.E
}

// invert is ParametricCurve::invert in moxcms src/trc.rs. It reports false
// when the function is not continuous at d, or when a parameter of the
// inverse is not finite.
func (c ParametricCurve) invert() (ParametricCurve, bool) {
	// The function must be continuous at the cross-over point d.
	d1 := powF32(float32(c.A*c.D)+c.B, c.G) + c.E
	d2 := float32(c.C*c.D) + c.F
	if absF32(d1-d2) > 0.1 {
		return ParametricCurve{}, false
	}
	d := d1

	// y = (a*x + b)^g + e, so x = ((1/a^g)*y - e/a^g)^(1/g) - b/a.
	a := 1 / powF32(c.A, c.G)
	b := -c.E / powF32(c.A, c.G)
	g := 1 / c.G
	e := -c.B / c.A

	// y = c*x + f, so x = y/c - f/c.
	var cc, f float32
	if d <= 0 {
		cc = 1
		f = 0
	} else {
		cc = 1 / c.C
		f = -c.F / c.C
	}

	// When d > 0 and c is 0, as is likely with the functions of type 1 and
	// 2, cc and f are not finite.
	for _, v := range [...]float32{g, a, b, cc, d, e, f} {
		if math.IsInf(float64(v), 0) || math.IsNaN(float64(v)) {
			return ParametricCurve{}, false
		}
	}
	return ParametricCurve{G: g, A: a, B: b, C: cc, D: d, E: e, F: f}, true
}

// u8Fixed8NumberToFloat is u8_fixed_8number_to_float in moxcms src/trc.rs.
func u8Fixed8NumberToFloat(x uint16) float32 {
	return float32(float64(x) / 256.0)
}

// passthroughTable is passthrough_table in moxcms src/trc.rs. It returns a
// table of n entries that is the identity for samples of bitDepth bits.
func passthroughTable(n, bitDepth int) []float32 {
	maxValue := (1 << bitDepth) - 1
	capValues := 1 << bitDepth
	table := make([]float32, n)
	scaleValue := 1 / float64(maxValue)
	for i := range table[:min(capValues, n)] {
		table[i] = float32(float64(i) * scaleValue)
	}
	return table
}

// linearForwardTable is linear_forward_table in moxcms src/trc.rs. It
// returns a table of n entries for the pure gamma of a curve with one entry.
func linearForwardTable(n, bitDepth int, gamma uint16) []float32 {
	maxValue := (1 << bitDepth) - 1
	capValues := 1 << bitDepth
	table := make([]float32, n)
	scaleValue := 1 / float64(maxValue)
	g := float64(u8Fixed8NumberToFloat(gamma))
	for i := range table[:min(capValues, n)] {
		table[i] = float32(math.Pow(float64(i)*scaleValue, g))
	}
	return table
}

// lutInterpLinearFloat is lut_interp_linear_float in moxcms src/trc.rs. It
// interpolates table at x, which it clamps to the range from 0 to 1.
//
// Port: upstream panics on an empty table. The port returns x. The subset
// never calls it with an empty table.
func lutInterpLinearFloat(x float32, table []float32) float32 {
	if len(table) == 0 {
		return x
	}
	value := maxF32(minF32(x, 1), 0) * float32(len(table)-1)

	upper := f32ToI32(ceilF32(value))
	lower := f32ToI32(floorF32(value))

	diff := float32(upper) - value
	tu := table[upper]
	return mlaf(negMlaf(tu, tu, diff), table[lower], diff)
}

// lutInterpLinear is lut_interp_linear in moxcms src/trc.rs. It
// interpolates table at inputValue, and scales the result to the range from
// 0 to 1.
func lutInterpLinear(inputValue float64, table []uint16) float32 {
	if len(table) == 0 {
		return float32(inputValue)
	}
	inputValue *= float64(len(table) - 1)

	upper := f64ToI32(math.Ceil(inputValue))
	lower := f64ToI32(math.Floor(inputValue))
	// Port: upstream casts a negative index to usize, which makes it huge,
	// and then clamps it to the last entry.
	index := func(i int32) int {
		if i < 0 || int(i) > len(table)-1 {
			return len(table) - 1
		}
		return int(i)
	}
	w0 := float64(table[index(upper)])
	w1 := 1 - (float64(upper) - inputValue)
	w2 := float64(table[index(lower)])
	w3 := float64(upper) - inputValue
	value := float32(mlafF64(float64(w2*w3), w0, w1))
	return value * (1.0 / 65535.0)
}

// linearLutInterpolate is linear_lut_interpolate in moxcms src/trc.rs. It
// returns a table of n entries that interpolates the curve table.
func linearLutInterpolate(n, bitDepth int, table []uint16) []float32 {
	maxValue := (1 << bitDepth) - 1
	capValues := 1 << bitDepth
	out := make([]float32, n)
	scaleValue := 1 / float64(maxValue)
	for i := range out[:min(capValues, n)] {
		out[i] = lutInterpLinear(float64(i)*scaleValue, table)
	}
	return out
}

// linearCurveParametric is linear_curve_parametric in moxcms src/trc.rs. It
// returns a table of n entries for a parametric curve. It reports false for
// parameters that make no curve.
func linearCurveParametric(n, bitDepth int, params []float32) ([]float32, bool) {
	c, ok := newParametricCurve(params)
	if !ok {
		return nil, false
	}
	maxValue := (1 << bitDepth) - 1
	capValue := 1 << bitDepth
	table := make([]float32, n)
	scaleValue := 1 / float32(maxValue)
	for i := range table[:min(capValue, n)] {
		table[i] = clampF32(c.Eval(float32(i)*scaleValue), 0, 1)
	}
	return table, true
}

// linearCurveParametricS is linear_curve_parametric_s in moxcms src/trc.rs.
// It returns a table of n entries for a parametric curve over the range
// from 0 to 1.
func linearCurveParametricS(n int, params []float32) ([]float32, bool) {
	c, ok := newParametricCurve(params)
	if !ok {
		return nil, false
	}
	table := make([]float32, n)
	scaleValue := 1 / float32(n-1)
	for i := range table {
		table[i] = clampF32(c.Eval(float32(i)*scaleValue), 0, 1)
	}
	return table, true
}

// makeGammaLinearTable is make_gamma_linear_table in moxcms src/trc.rs, for
// samples of 8 bits. It returns the identity gamma table of n bins.
func makeGammaLinearTable(n, bitDepth int) []uint8 {
	maxRange := float32(1 / (float64(n-1) / float64(int(1)<<bitDepth)))
	table := make([]uint8, gammaTableSize)
	for v := range table[:n] {
		table[v] = f32ToU8(roundF32(float32(v) * maxRange))
	}
	return table
}

// lutInterpLinearGammaImpl is lut_interp_linear_gamma_impl in moxcms
// src/trc.rs, for samples of 8 bits. It interpolates the curve table at
// inputValue, which is in the range from 0 to n-1, and scales the result to
// the range of the samples.
func lutInterpLinearGammaImpl(n, bitDepth int, inputValue uint32, table []uint16) uint8 {
	// Scale inputValue to the range from 0 to (n-1)*(len(table)-1).
	value := inputValue * uint32(len(table)-1)
	capValue := uint32(n - 1)
	upper := (value + capValue - 1) / capValue
	lower := value / capValue
	// interp is the distance from lower to value, from 0 to n-1.
	interp := value % capValue
	lwValue := uint32(table[lower])
	hwValue := uint32(table[upper])
	// The values of table are from 0 to 65535.
	value = hwValue*interp + lwValue*(capValue-interp)

	// Round and scale to the range of the samples.
	maxColors := uint32(1)<<bitDepth - 1
	value += capValue * 65535 / maxColors / 2
	value /= capValue * 65535 / maxColors
	return uint8(value)
}

// makeGammaLut is make_gamma_lut in moxcms src/trc.rs, for samples of 8
// bits. It returns a gamma table of n bins from the inverse curve table.
func makeGammaLut(n, bitDepth int, table []uint16) []uint8 {
	out := make([]uint8, gammaTableSize)
	for v := range out[:n] {
		out[v] = lutInterpLinearGammaImpl(n, bitDepth, uint32(v), table)
	}
	return out
}

// lutInterpLinear16 is lut_interp_linear16 in moxcms src/trc.rs. It
// interpolates table at inputValue, in the range from 0 to 65535.
func lutInterpLinear16(inputValue uint16, table []uint16) uint16 {
	// Scale inputValue to the range from 0 to 65535*(len(table)-1).
	value := uint32(inputValue) * uint32(len(table)-1)
	upper := uint16((value + 65534) / 65535)
	lower := uint16(value / 65535)
	// interp is the distance from lower to value, from 0 to 65534.
	interp := value % 65535
	value = (uint32(table[upper])*interp + uint32(table[lower])*(65535-interp)) / 65535
	return uint16(value)
}

// makeGammaPowTable is make_gamma_pow_table in moxcms src/trc.rs, for
// samples of 8 bits. It returns a gamma table of n bins for a pure gamma.
func makeGammaPowTable(n int, gamma float32, bitDepth int) []uint8 {
	table := make([]uint8, gammaTableSize)
	capV := float32(int(1)<<bitDepth - 1)
	scale := 1 / float32(n-1)
	for v := range table[:n] {
		table[v] = f32ToU8(roundF32(capV * powF32(float32(v)*scale, gamma)))
	}
	return table
}

// makeGammaParametricTable is make_gamma_parametric_table in moxcms
// src/trc.rs, for samples of 8 bits. It returns a gamma table of n bins for
// the inverse of a parametric curve.
func makeGammaParametricTable(n, bitDepth int, curve ParametricCurve) []uint8 {
	capV := float32(int(1)<<bitDepth - 1)
	scale := 1 / float32(n-1)
	table := make([]uint8, gammaTableSize)
	for v := range table[:n] {
		table[v] = f32ToU8(roundF32(capV * curve.Eval(float32(v)*scale)))
	}
	return table
}

// compareParametric is compare_parametric in moxcms src/trc.rs.
func compareParametric(src, dst []float32) bool {
	for i := range min(len(src), len(dst)) {
		if absF32(src[i]-dst[i]) > 1e-4 {
			return false
		}
	}
	return true
}

// lutInverseInterp16 is lut_inverse_interp16 in moxcms src/trc.rs. It finds
// the input of lutTable whose output is value.
func lutInverseInterp16(value uint16, lutTable []uint16) uint16 {
	l := int32(1) // Space for negative values.
	r := int32(0x10000)
	x := int32(0)
	length := int32(len(lutTable))

	numZeroes := int32(0)
	for _, item := range lutTable {
		if item != 0 {
			break
		}
		numZeroes++
	}
	if numZeroes == 0 && value == 0 {
		return 0
	}
	numOfPolys := int32(0)
	for i := len(lutTable) - 1; i >= 0; i-- {
		if lutTable[i] != 0xffff {
			break
		}
		numOfPolys++
	}
	// Does the curve belong to this case?
	if numZeroes > 1 || numOfPolys > 1 {
		// Find whether the value falls in the zone of 0 or of 0xffff.
		if value == 0 {
			return 0
		}
		// Restrict the search to the valid zone.
		if numZeroes > 1 {
			a0 := (numZeroes - 1) * 0xffff / (length - 1)
			l = a0 - 1
		}
		if numOfPolys > 1 {
			b0 := (length - 1 - numOfPolys) * 0xffff / (length - 1)
			r = b0 + 1
		}
	}
	if r <= l {
		// The table has no inverse.
		return 0
	}

	for r > l {
		x = (l + r) / 2
		res := int32(lutInterpLinear16(uint16(x-1), lutTable))
		if res == int32(value) {
			// The match is exact.
			return uint16(x - 1)
		}
		if res > int32(value) {
			r = x - 1
		} else {
			l = x + 1
		}
	}

	// Interpolate between the nodes around x.
	val2 := float64(length-1) * (float64(x-1) / 65535.0)
	cell0 := f64ToI32(math.Floor(val2))
	cell1 := f64ToI32(math.Ceil(val2))
	if cell0 == cell1 {
		return uint16(x)
	}
	// Port: upstream panics when x is 0, because cell0 is then negative. The
	// port clamps the cells to the table.
	cell0 = max(min(cell0, length-1), 0)
	cell1 = max(min(cell1, length-1), 0)

	y0 := float64(lutTable[cell0])
	x0 := 65535.0 * float64(cell0) / float64(length-1)
	y1 := float64(lutTable[cell1])
	x1 := 65535.0 * float64(cell1) / float64(length-1)
	a := (y1 - y0) / (x1 - x0)
	b := mlafF64(y0, -a, x0)
	if math.Abs(a) < 0.01 {
		return uint16(x)
	}
	f := (float64(value) - b) / a
	if f < 0 {
		return 0
	}
	if f >= 65535 {
		return 0xffff
	}
	return f64ToU16(math.Floor(f + 0.5))
}

// invertLut is invert_lut in moxcms src/trc.rs. It returns the inverse of
// the curve table, in outLength entries.
func invertLut(table []uint16, outLength int) []uint16 {
	output := make([]uint16, outLength)
	scaleValue := 65535 / float64(outLength-1)
	for i := range output {
		x := float64(i) * scaleValue
		input := f64ToU16(math.Floor(x + 0.5))
		output[i] = lutInverseInterp16(input, table)
	}
	return output
}

// buildLinearizeTable is ToneReprCurve::build_linearize_table in moxcms
// src/trc.rs. It returns a table of n entries from samples of bitDepth bits
// to linear values. It reports false for a curve that has no table.
func (c *ToneReprCurve) buildLinearizeTable(n, bitDepth int) ([]float32, bool) {
	if c.Kind == CurveParametric {
		return linearCurveParametric(n, bitDepth, c.Parametric)
	}
	switch len(c.Lut) {
	case 0:
		return passthroughTable(n, bitDepth), true
	case 1:
		return linearForwardTable(n, bitDepth, c.Lut[0]), true
	default:
		return linearLutInterpolate(n, bitDepth, c.Lut), true
	}
}

// buildTrcTable is build_trc_table in moxcms src/trc.rs. It returns a
// table of numEntries samples of the function eotf, from 0 to 1, as
// numbers of 16 bits.
func buildTrcTable(numEntries int, eotf func(float64) float64) []uint16 {
	table := make([]uint16, numEntries)
	for i := range table {
		x := float64(i) / float64(numEntries-1)
		output := eotf(x)*65535.0 + 0.5
		// Port: Rust saturates a NaN to 0 in the cast. The comparisons
		// below keep a NaN, so the port sets it to 0 first.
		if math.IsNaN(output) {
			output = 0
		}
		output = math.Min(math.Max(output, 0), 65535)
		table[i] = uint16(math.Floor(output))
	}
	return table
}

// srgbParametric returns the parameters of the parametric curve of sRGB.
func srgbParametric() []float32 {
	return []float32{2.4, 1 / float32(1.055), float32(0.055) / float32(1.055), 1 / float32(12.92), 0.04045}
}

// buildGammaTable is ToneReprCurve::build_gamma_table in moxcms src/trc.rs,
// for samples of 8 bits. It returns a gamma table of n bins from linear
// values to samples of bitDepth bits. It reports false for a curve that has
// no table.
func (c *ToneReprCurve) buildGammaTable(n, bitDepth int) ([]uint8, bool) {
	if c.Kind == CurveLut {
		switch len(c.Lut) {
		case 0:
			return makeGammaLinearTable(n, bitDepth), true
		case 1:
			return makeGammaPowTable(n, 1/u8Fixed8NumberToFloat(c.Lut[0]), bitDepth), true
		default:
			invertedSize := max(len(c.Lut), 256)
			inverted := invertLut(c.Lut, invertedSize)
			return makeGammaLut(n, bitDepth, inverted), true
		}
	}

	params := c.Parametric
	if len(params) == 5 {
		rec709 := createRec709Parametric()
		if compareParametric(params, srgbParametric()) {
			return TransferCharacteristicsSrgb.makeGammaTable(n, bitDepth), true
		}
		if compareParametric(params, rec709[:]) {
			return TransferCharacteristicsBt709.makeGammaTable(n, bitDepth), true
		}
	}

	curve, ok := newParametricCurve(params)
	if !ok {
		return nil, false
	}
	if inverse, ok := curve.invert(); ok {
		return makeGammaParametricTable(n, bitDepth, inverse), true
	}

	gammaTable, ok := linearCurveParametricS(n, params)
	if !ok {
		return nil, false
	}
	gammaTableUint := make([]uint16, n)
	for i, v := range gammaTable {
		gammaTableUint[i] = f32ToU16(v * 65535)
	}
	inverted := invertLut(gammaTableUint, n)
	return makeGammaLut(n, bitDepth, inverted), true
}

// buildGrayLinearizeTable is ColorProfile::build_gray_linearize_table in
// moxcms src/trc.rs.
func (p *ColorProfile) buildGrayLinearizeTable(n, bitDepth int) ([]float32, error) {
	if p.GrayTrc != nil {
		if t, ok := p.GrayTrc.buildLinearizeTable(n, bitDepth); ok {
			return t, nil
		}
	}
	return nil, cmsError(BuildTransferFunction)
}

// buildLinearizeTable is the body of the functions
// ColorProfile::build_r_linearize_table, build_g_linearize_table and
// build_b_linearize_table in moxcms src/trc.rs. It takes the transfer
// characteristics of the cicp tag first, when useCicp is true.
func (p *ColorProfile) buildLinearizeTable(trc *ToneReprCurve, n, bitDepth int, useCicp bool) ([]float32, error) {
	if useCicp && p.Cicp != nil && p.Cicp.TransferCharacteristics.hasTransferCurve() {
		return p.Cicp.TransferCharacteristics.makeLinearTable(n, bitDepth), nil
	}
	if trc != nil {
		if t, ok := trc.buildLinearizeTable(n, bitDepth); ok {
			return t, nil
		}
	}
	return nil, cmsError(BuildTransferFunction)
}

// buildRLinearizeTable is ColorProfile::build_r_linearize_table in moxcms
// src/trc.rs.
func (p *ColorProfile) buildRLinearizeTable(n, bitDepth int, useCicp bool) ([]float32, error) {
	return p.buildLinearizeTable(p.RedTrc, n, bitDepth, useCicp)
}

// buildGLinearizeTable is ColorProfile::build_g_linearize_table in moxcms
// src/trc.rs.
func (p *ColorProfile) buildGLinearizeTable(n, bitDepth int, useCicp bool) ([]float32, error) {
	return p.buildLinearizeTable(p.GreenTrc, n, bitDepth, useCicp)
}

// buildBLinearizeTable is ColorProfile::build_b_linearize_table in moxcms
// src/trc.rs.
func (p *ColorProfile) buildBLinearizeTable(n, bitDepth int, useCicp bool) ([]float32, error) {
	return p.buildLinearizeTable(p.BlueTrc, n, bitDepth, useCicp)
}

// buildGammaTable is ColorProfile::build_gamma_table in moxcms src/trc.rs,
// for samples of 8 bits. It takes the transfer characteristics of the cicp
// tag first, when useCicp is true.
func (p *ColorProfile) buildGammaTable(trc *ToneReprCurve, n, bitDepth int, useCicp bool) ([]uint8, error) {
	if useCicp && p.Cicp != nil && p.Cicp.TransferCharacteristics.hasTransferCurve() {
		return p.Cicp.TransferCharacteristics.makeGammaTable(n, bitDepth), nil
	}
	if trc != nil {
		if t, ok := trc.buildGammaTable(n, bitDepth); ok {
			return t, nil
		}
	}
	return nil, cmsError(BuildTransferFunction)
}

// areAllTrcTheSame is ColorProfile::are_all_trc_the_same in moxcms
// src/trc.rs.
//
// Port: upstream reports true when the red curve equals the green one, or
// the green curve equals the blue one. The port keeps this.
func (p *ColorProfile) areAllTrcTheSame() bool {
	if p.ColorSpace == DataColorSpaceGray {
		return true
	}
	r, g, b := p.RedTrc, p.GreenTrc, p.BlueTrc
	if r == nil || g == nil || b == nil {
		return false
	}
	if r.Kind != g.Kind || g.Kind != b.Kind {
		return false
	}
	return r.equal(g) || g.equal(b)
}

// isLinearMatrixShaper is ColorProfile::is_linear_matrix_shaper in moxcms
// src/trc.rs. It reports whether p is a matrix shaper whose curves are all
// the same and linear.
func (p *ColorProfile) isLinearMatrixShaper() bool {
	if !p.IsMatrixShaper() {
		return false
	}
	if !p.areAllTrcTheSame() {
		return false
	}
	if p.RedTrc == nil {
		return false
	}
	if p.RedTrc.Kind == CurveLut {
		if len(p.RedTrc.Lut) == 0 {
			return true
		}
		return isCurveLinear16(p.RedTrc.Lut)
	}
	if c, ok := newParametricCurve(p.RedTrc.Parametric); ok {
		return c.isLinear()
	}
	return false
}
