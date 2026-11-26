// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/gamma.rs.

package moxcms

import "math"

// Port: upstream computes the powers, exponents and logarithms with the
// crate pxfm. The port uses the package math. The last bit of a result can
// differ.

// srgbToLinear is srgb_to_linear in moxcms src/gamma.rs.
func srgbToLinear(gamma float64) float64 {
	switch {
	case gamma < 0:
		return 0
	case gamma < 12.92*0.0030412825601275209:
		return gamma * (1 / 12.92)
	case gamma < 1:
		return math.Pow((gamma+0.0550107189475866)/1.0550107189475866, 2.4)
	default:
		return 1
	}
}

// srgbFromLinear is srgb_from_linear in moxcms src/gamma.rs.
func srgbFromLinear(linear float64) float64 {
	switch {
	case linear < 0:
		return 0
	case linear < 0.0030412825601275209:
		return linear * 12.92
	case linear < 1:
		return fmlaF64(1.0550107189475866, math.Pow(linear, 1.0/2.4), -0.0550107189475866)
	default:
		return 1
	}
}

// rec709ToLinear is rec709_to_linear in moxcms src/gamma.rs.
func rec709ToLinear(gamma float64) float64 {
	switch {
	case gamma < 0:
		return 0
	case gamma < 4.5*0.018053968510807:
		return gamma * (1 / 4.5)
	case gamma < 1:
		return math.Pow((gamma+0.09929682680944)/1.09929682680944, 1.0/0.45)
	default:
		return 1
	}
}

// rec709FromLinear is rec709_from_linear in moxcms src/gamma.rs.
func rec709FromLinear(linear float64) float64 {
	switch {
	case linear < 0:
		return 0
	case linear < 0.018053968510807:
		return linear * 4.5
	case linear < 1:
		return fmlaF64(1.09929682680944, math.Pow(linear, 0.45), -0.09929682680944)
	default:
		return 1
	}
}

// smpte428ToLinear is smpte428_to_linear in moxcms src/gamma.rs.
func smpte428ToLinear(gamma float64) float64 {
	const scale = 1.0 / 0.91655527974030934
	return math.Pow(minF64(maxF64(gamma, 0), 1), 2.6) * scale
}

// smpte428FromLinear is smpte428_from_linear in moxcms src/gamma.rs.
func smpte428FromLinear(linear float64) float64 {
	const powerValue = 1.0 / 2.6
	return math.Pow(0.91655527974030934*maxF64(linear, 0), powerValue)
}

// smpte240ToLinear is smpte240_to_linear in moxcms src/gamma.rs.
func smpte240ToLinear(gamma float64) float64 {
	switch {
	case gamma < 0:
		return 0
	case gamma < 4.0*0.022821585529445:
		return gamma / 4.0
	case gamma < 1:
		return math.Pow((gamma+0.111572195921731)/1.111572195921731, 1.0/0.45)
	default:
		return 1
	}
}

// smpte240FromLinear is smpte240_from_linear in moxcms src/gamma.rs.
func smpte240FromLinear(linear float64) float64 {
	switch {
	case linear < 0:
		return 0
	case linear < 0.022821585529445:
		return linear * 4.0
	case linear < 1:
		return fmlaF64(1.111572195921731, math.Pow(linear, 0.45), -0.111572195921731)
	default:
		return 1
	}
}

// log100FromLinear is log100_from_linear in moxcms src/gamma.rs.
func log100FromLinear(linear float64) float64 {
	if linear <= 0.01 {
		return 0
	}
	return 1 + math.Log10(minF64(linear, 1))/2.0
}

// log100ToLinear is log100_to_linear in moxcms src/gamma.rs.
func log100ToLinear(gamma float64) float64 {
	// The function is not bijective, so it takes the middle of the range.
	const midInterval = 0.01 / 2.0
	if gamma <= 0 {
		return midInterval
	}
	return math.Pow(10, 2.0*(minF64(gamma, 1)-1))
}

// log100Sqrt10ToLinear is log100_sqrt10_to_linear in moxcms src/gamma.rs.
func log100Sqrt10ToLinear(gamma float64) float64 {
	// The function is not bijective, so it takes the middle of the range.
	const midInterval = 0.00316227766 / 2.0
	if gamma <= 0 {
		return midInterval
	}
	return math.Pow(10, 2.5*(minF64(gamma, 1)-1))
}

// log100Sqrt10FromLinear is log100_sqrt10_from_linear in moxcms
// src/gamma.rs.
func log100Sqrt10FromLinear(linear float64) float64 {
	if linear <= 0.00316227766 {
		return 0
	}
	return 1 + math.Log10(minF64(linear, 1))/2.5
}

// bt1361FromLinear is bt1361_from_linear in moxcms src/gamma.rs.
func bt1361FromLinear(linear float64) float64 {
	switch {
	case linear < -0.25:
		return -0.25
	case linear < 0:
		return fmlaF64(-0.27482420670236, math.Pow(-4.0*linear, 0.45), 0.02482420670236)
	case linear < 0.018053968510807:
		return linear * 4.5
	case linear < 1:
		return fmlaF64(1.09929682680944, math.Pow(linear, 0.45), -0.09929682680944)
	default:
		return 1
	}
}

// bt1361ToLinear is bt1361_to_linear in moxcms src/gamma.rs.
func bt1361ToLinear(gamma float64) float64 {
	switch {
	case gamma < -0.25:
		return -0.25
	case gamma < 0:
		return math.Pow((gamma-0.02482420670236)/-0.27482420670236, 1.0/0.45) / -4.0
	case gamma < 4.5*0.018053968510807:
		return gamma / 4.5
	case gamma < 1:
		return math.Pow((gamma+0.09929682680944)/1.09929682680944, 1.0/0.45)
	default:
		return 1
	}
}

// pureGammaFunction is pure_gamma_function in moxcms src/gamma.rs.
func pureGammaFunction(x, gamma float64) float64 {
	switch {
	case x <= 0:
		return 0
	case x >= 1:
		return 1
	default:
		return math.Pow(x, gamma)
	}
}

// iec61966ToLinear is iec61966_to_linear in moxcms src/gamma.rs.
func iec61966ToLinear(gamma float64) float64 {
	switch {
	case gamma < -4.5*0.018053968510807:
		return math.Pow((-gamma+0.09929682680944)/-1.09929682680944, 1.0/0.45)
	case gamma < 4.5*0.018053968510807:
		return gamma / 4.5
	default:
		return math.Pow((gamma+0.09929682680944)/1.09929682680944, 1.0/0.45)
	}
}

// iec61966FromLinear is iec61966_from_linear in moxcms src/gamma.rs.
func iec61966FromLinear(v float64) float64 {
	switch {
	case v < -0.018053968510807:
		return fmlaF64(-1.09929682680944, math.Pow(-v, 0.45), 0.09929682680944)
	case v < 0.018053968510807:
		return v * 4.5
	default:
		return fmlaF64(1.09929682680944, math.Pow(v, 0.45), -0.09929682680944)
	}
}

// gamma2p2FromLinear is gamma2p2_from_linear in moxcms src/gamma.rs.
func gamma2p2FromLinear(linear float64) float64 {
	return pureGammaFunction(linear, 1/2.2)
}

// gamma2p2ToLinear is gamma2p2_to_linear in moxcms src/gamma.rs.
func gamma2p2ToLinear(gamma float64) float64 {
	return pureGammaFunction(gamma, 2.2)
}

// gamma2p8FromLinear is gamma2p8_from_linear in moxcms src/gamma.rs.
func gamma2p8FromLinear(linear float64) float64 {
	return pureGammaFunction(linear, 1/2.8)
}

// gamma2p8ToLinear is gamma2p8_to_linear in moxcms src/gamma.rs.
func gamma2p8ToLinear(gamma float64) float64 {
	return pureGammaFunction(gamma, 2.8)
}

// pqToLinear is pq_to_linear in moxcms src/gamma.rs.
func pqToLinear(gamma float64) float64 {
	if gamma <= 0 {
		return 0
	}
	powGamma := math.Pow(gamma, 1.0/78.84375)
	num := maxF64(powGamma-0.8359375, 0)
	den := maxF64(mlafF64(18.8515625, -18.6875, powGamma), -math.MaxFloat64)
	return math.Pow(num/den, 1.0/0.1593017578125)
}

// pqFromLinear is pq_from_linear in moxcms src/gamma.rs.
func pqFromLinear(linear float64) float64 {
	if linear <= 0 {
		return 0
	}
	linear = math.Min(math.Max(linear, 0), 1)
	powLinear := math.Pow(linear, 0.1593017578125)
	num := fmlaF64(0.1640625, powLinear, -0.1640625)
	den := mlafF64(1.0, 18.6875, powLinear)
	return math.Pow(1.0+num/den, 78.84375)
}

// hlgToLinear is hlg_to_linear in moxcms src/gamma.rs.
func hlgToLinear(gamma float64) float64 {
	if gamma < 0 {
		return 0
	}
	if gamma <= 0.5 {
		return math.Pow((gamma*gamma)*(1.0/3.0), 1.2)
	}
	return math.Pow((math.Exp((gamma-0.55991073)/0.17883277)+0.28466892)/12.0, 1.2)
}

// hlgFromLinear is hlg_from_linear in moxcms src/gamma.rs.
func hlgFromLinear(linear float64) float64 {
	// Scale from the extended SDR range to the range from 0 to 1.
	linear = math.Min(math.Max(linear, 0), 1)
	// The inverse OOTF, then the OETF, as ITU-R BT.2100-2 table 5 and note
	// 5i say.
	linear = math.Pow(linear, 1.0/1.2)
	switch {
	case linear < 0:
		return 0
	case linear <= 1.0/12.0:
		return math.Sqrt(3.0 * linear)
	default:
		return fmlaF64(0.17883277, math.Log(fmlaF64(12.0, linear, -0.28466892)), 0.55991073)
	}
}

// trcLinear is trc_linear in moxcms src/gamma.rs.
func trcLinear(v float64) float64 {
	return maxF64(minF64(v, 1), 0)
}

// makeLinearTableImpl is make_linear_table_impl in moxcms src/gamma.rs.
func makeLinearTableImpl(output []float32, capValues, maxValue int, linearize func(float64) float64) {
	scaleValue := 1 / float64(maxValue)
	for i := range output[:min(capValues, len(output))] {
		output[i] = float32(linearize(float64(i) * scaleValue))
	}
}

// makeGammaTableImpl is make_gamma_table_impl in moxcms src/gamma.rs, for
// an integer table, which rounds each entry.
func makeGammaTableImpl(output []float32, maxValue float64, gamma func(float64) float64) {
	maxRange := 1 / float64(len(output)-1)
	for v := range output {
		output[v] = roundF32(float32(gamma(float64(v)*maxRange) * maxValue))
	}
}

// Linearize is TransferCharacteristics::linearize in moxcms src/gamma.rs. It
// returns the linear value of the encoded value v.
func (t TransferCharacteristics) Linearize(v float64) float64 {
	switch t {
	case TransferCharacteristicsBt709, TransferCharacteristicsBt601,
		TransferCharacteristicsBt202010bit, TransferCharacteristicsBt202012bit:
		return rec709ToLinear(v)
	case TransferCharacteristicsBt470M:
		return gamma2p2ToLinear(v)
	case TransferCharacteristicsBt470Bg:
		return gamma2p8ToLinear(v)
	case TransferCharacteristicsSmpte240:
		return smpte240ToLinear(v)
	case TransferCharacteristicsLinear:
		return trcLinear(v)
	case TransferCharacteristicsLog100:
		return log100ToLinear(v)
	case TransferCharacteristicsLog100sqrt10:
		return log100Sqrt10ToLinear(v)
	case TransferCharacteristicsIec61966:
		return iec61966ToLinear(v)
	case TransferCharacteristicsBt1361:
		return bt1361ToLinear(v)
	case TransferCharacteristicsSrgb:
		return srgbToLinear(v)
	case TransferCharacteristicsSmpte2084:
		return pqToLinear(v)
	case TransferCharacteristicsSmpte428:
		return smpte428ToLinear(v)
	case TransferCharacteristicsHlg:
		return hlgToLinear(v)
	default:
		// Reserved and Unspecified.
		return 0
	}
}

// Gamma is TransferCharacteristics::gamma in moxcms src/gamma.rs. It returns
// the encoded value of the linear value v.
func (t TransferCharacteristics) Gamma(v float64) float64 {
	switch t {
	case TransferCharacteristicsBt709, TransferCharacteristicsBt601,
		TransferCharacteristicsBt202010bit, TransferCharacteristicsBt202012bit:
		return rec709FromLinear(v)
	case TransferCharacteristicsBt470M:
		return gamma2p2FromLinear(v)
	case TransferCharacteristicsBt470Bg:
		return gamma2p8FromLinear(v)
	case TransferCharacteristicsSmpte240:
		return smpte240FromLinear(v)
	case TransferCharacteristicsLinear:
		return trcLinear(v)
	case TransferCharacteristicsLog100:
		return log100FromLinear(v)
	case TransferCharacteristicsLog100sqrt10:
		return log100Sqrt10FromLinear(v)
	case TransferCharacteristicsIec61966:
		return iec61966FromLinear(v)
	case TransferCharacteristicsBt1361:
		return bt1361FromLinear(v)
	case TransferCharacteristicsSrgb:
		return srgbFromLinear(v)
	case TransferCharacteristicsSmpte2084:
		return pqFromLinear(v)
	case TransferCharacteristicsSmpte428:
		return smpte428FromLinear(v)
	case TransferCharacteristicsHlg:
		return hlgFromLinear(v)
	default:
		// Reserved and Unspecified.
		return 0
	}
}

// makeLinearTable is TransferCharacteristics::make_linear_table in moxcms
// src/gamma.rs, for integer samples of bitDepth bits. It returns a table of
// n entries.
//
// Port: the subset has integer samples only, so the branch of upstream for
// the samples of floating point is not ported.
func (t TransferCharacteristics) makeLinearTable(n, bitDepth int) []float32 {
	maxValue := (1 << bitDepth) - 1
	capValues := 1 << bitDepth
	table := make([]float32, n)
	makeLinearTableImpl(table, capValues, maxValue, t.Linearize)
	return table
}

// makeGammaTable is TransferCharacteristics::make_gamma_table in moxcms
// src/gamma.rs, for samples of 8 bits. It returns a table of 65536 entries,
// of which the first n hold values.
func (t TransferCharacteristics) makeGammaTable(n, bitDepth int) []uint8 {
	scratch := make([]float32, n)
	maxValue := float64(int(1)<<bitDepth - 1)
	makeGammaTableImpl(scratch, maxValue, t.Gamma)
	table := make([]uint8, gammaTableSize)
	for i, v := range scratch {
		table[i] = f32ToU8(v)
	}
	return table
}
