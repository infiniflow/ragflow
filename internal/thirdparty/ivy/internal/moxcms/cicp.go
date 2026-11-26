// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/cicp.rs.

package moxcms

// CicpColorPrimaries is CicpColorPrimaries in moxcms src/cicp.rs. It is the
// code point of the color primaries in Rec. ITU-T H.273 table 2.
type CicpColorPrimaries uint8

// The color primaries of CicpColorPrimaries in moxcms src/cicp.rs. The
// values 0, 3, 13 to 21, and 23 to 255 are reserved, and they all map to
// CicpColorPrimariesReserved.
const (
	CicpColorPrimariesReserved    CicpColorPrimaries = 0
	CicpColorPrimariesBt709       CicpColorPrimaries = 1
	CicpColorPrimariesUnspecified CicpColorPrimaries = 2
	CicpColorPrimariesBt470M      CicpColorPrimaries = 4
	CicpColorPrimariesBt470Bg     CicpColorPrimaries = 5
	CicpColorPrimariesBt601       CicpColorPrimaries = 6
	CicpColorPrimariesSmpte240    CicpColorPrimaries = 7
	CicpColorPrimariesGenericFilm CicpColorPrimaries = 8
	CicpColorPrimariesBt2020      CicpColorPrimaries = 9
	CicpColorPrimariesXyz         CicpColorPrimaries = 10
	CicpColorPrimariesSmpte431    CicpColorPrimaries = 11
	CicpColorPrimariesSmpte432    CicpColorPrimaries = 12
	CicpColorPrimariesEbu3213     CicpColorPrimaries = 22
)

// CicpColorPrimariesFrom is impl TryFrom<u8> for CicpColorPrimaries in
// moxcms src/cicp.rs. Each value maps to a variant, so it never fails.
func CicpColorPrimariesFrom(v uint8) CicpColorPrimaries {
	switch {
	case v == 0 || v == 3 || (v >= 13 && v <= 21) || v >= 23:
		return CicpColorPrimariesReserved
	default:
		return CicpColorPrimaries(v)
	}
}

// ColorPrimaries is ColorPrimaries in moxcms src/cicp.rs. It holds the
// chromaticities of the red, green and blue primaries.
type ColorPrimaries struct {
	Red, Green, Blue Chromaticity
}

// The primaries of ColorPrimaries in moxcms src/cicp.rs. The subset holds
// the primaries of the CICP code points only.
var (
	// ColorPrimariesBt709 is ColorPrimaries::BT_709.
	ColorPrimariesBt709 = ColorPrimaries{
		Red:   Chromaticity{X: 0.64, Y: 0.33},
		Green: Chromaticity{X: 0.30, Y: 0.60},
		Blue:  Chromaticity{X: 0.15, Y: 0.06},
	}
	// ColorPrimariesBt470M is ColorPrimaries::BT_470M.
	ColorPrimariesBt470M = ColorPrimaries{
		Red:   Chromaticity{X: 0.67, Y: 0.33},
		Green: Chromaticity{X: 0.21, Y: 0.71},
		Blue:  Chromaticity{X: 0.14, Y: 0.08},
	}
	// ColorPrimariesBt470Bg is ColorPrimaries::BT_470BG.
	ColorPrimariesBt470Bg = ColorPrimaries{
		Red:   Chromaticity{X: 0.64, Y: 0.33},
		Green: Chromaticity{X: 0.29, Y: 0.60},
		Blue:  Chromaticity{X: 0.15, Y: 0.06},
	}
	// ColorPrimariesBt601 is ColorPrimaries::BT_601, which is also
	// ColorPrimaries::SMPTE_240.
	ColorPrimariesBt601 = ColorPrimaries{
		Red:   Chromaticity{X: 0.630, Y: 0.340},
		Green: Chromaticity{X: 0.310, Y: 0.595},
		Blue:  Chromaticity{X: 0.155, Y: 0.070},
	}
	// ColorPrimariesGenericFilm is ColorPrimaries::GENERIC_FILM.
	ColorPrimariesGenericFilm = ColorPrimaries{
		Red:   Chromaticity{X: 0.681, Y: 0.319},
		Green: Chromaticity{X: 0.243, Y: 0.692},
		Blue:  Chromaticity{X: 0.145, Y: 0.049},
	}
	// ColorPrimariesBt2020 is ColorPrimaries::BT_2020.
	ColorPrimariesBt2020 = ColorPrimaries{
		Red:   Chromaticity{X: 0.708, Y: 0.292},
		Green: Chromaticity{X: 0.170, Y: 0.797},
		Blue:  Chromaticity{X: 0.131, Y: 0.046},
	}
	// ColorPrimariesXyz is ColorPrimaries::XYZ.
	ColorPrimariesXyz = ColorPrimaries{
		Red:   Chromaticity{X: 1.0, Y: 0.0},
		Green: Chromaticity{X: 0.0, Y: 1.0},
		Blue:  Chromaticity{X: 0.0, Y: 0.0},
	}
	// ColorPrimariesDciP3 is ColorPrimaries::DCI_P3, which is also
	// ColorPrimaries::DISPLAY_P3, SMPTE_431 and SMPTE_432.
	ColorPrimariesDciP3 = ColorPrimaries{
		Red:   Chromaticity{X: 0.680, Y: 0.320},
		Green: Chromaticity{X: 0.265, Y: 0.690},
		Blue:  Chromaticity{X: 0.150, Y: 0.060},
	}
	// ColorPrimariesEbu3213 is ColorPrimaries::EBU_3213.
	ColorPrimariesEbu3213 = ColorPrimaries{
		Red:   Chromaticity{X: 0.630, Y: 0.340},
		Green: Chromaticity{X: 0.295, Y: 0.605},
		Blue:  Chromaticity{X: 0.155, Y: 0.077},
	}
)

// hasChromaticity is CicpColorPrimaries::has_chromaticity in moxcms
// src/cicp.rs.
func (c CicpColorPrimaries) hasChromaticity() bool {
	return c != CicpColorPrimariesReserved && c != CicpColorPrimariesUnspecified
}

// whitePoint is CicpColorPrimaries::white_point in moxcms src/cicp.rs. It
// returns the white point of the primaries.
func (c CicpColorPrimaries) whitePoint() (Chromaticity, error) {
	switch c {
	case CicpColorPrimariesBt709, CicpColorPrimariesBt470Bg, CicpColorPrimariesBt601,
		CicpColorPrimariesSmpte240, CicpColorPrimariesBt2020, CicpColorPrimariesSmpte432,
		CicpColorPrimariesEbu3213:
		return ChromaticityD65, nil
	case CicpColorPrimariesBt470M, CicpColorPrimariesGenericFilm:
		return Chromaticity{X: 0.310, Y: 0.316}, nil
	case CicpColorPrimariesXyz:
		return Chromaticity{X: 1.0 / 3.0, Y: 1.0 / 3.0}, nil
	case CicpColorPrimariesSmpte431:
		return Chromaticity{X: 0.314, Y: 0.351}, nil
	default:
		// Reserved and Unspecified.
		return Chromaticity{}, &CmsError{Kind: UnsupportedColorPrimaries, Value: uint64(c)}
	}
}

// colorPrimariesFrom is impl TryFrom<CicpColorPrimaries> for ColorPrimaries
// in moxcms src/cicp.rs.
func colorPrimariesFrom(c CicpColorPrimaries) (ColorPrimaries, error) {
	switch c {
	case CicpColorPrimariesBt709:
		return ColorPrimariesBt709, nil
	case CicpColorPrimariesBt470M:
		return ColorPrimariesBt470M, nil
	case CicpColorPrimariesBt470Bg:
		return ColorPrimariesBt470Bg, nil
	case CicpColorPrimariesBt601, CicpColorPrimariesSmpte240:
		return ColorPrimariesBt601, nil
	case CicpColorPrimariesGenericFilm:
		return ColorPrimariesGenericFilm, nil
	case CicpColorPrimariesBt2020:
		return ColorPrimariesBt2020, nil
	case CicpColorPrimariesXyz:
		return ColorPrimariesXyz, nil
	case CicpColorPrimariesSmpte431, CicpColorPrimariesSmpte432:
		// The two share the primaries, and have different white points.
		return ColorPrimariesDciP3, nil
	case CicpColorPrimariesEbu3213:
		return ColorPrimariesEbu3213, nil
	default:
		// Reserved and Unspecified.
		return ColorPrimaries{}, &CmsError{Kind: UnsupportedColorPrimaries, Value: uint64(c)}
	}
}

// TransferCharacteristics is TransferCharacteristics in moxcms src/cicp.rs.
// It is the code point of the transfer characteristics in Rec. ITU-T H.273
// table 3.
type TransferCharacteristics uint8

// The transfer characteristics of TransferCharacteristics in moxcms
// src/cicp.rs. The values 0, 3, and 19 to 255 are reserved, and they all
// map to TransferCharacteristicsReserved.
const (
	TransferCharacteristicsReserved     TransferCharacteristics = 0
	TransferCharacteristicsBt709        TransferCharacteristics = 1
	TransferCharacteristicsUnspecified  TransferCharacteristics = 2
	TransferCharacteristicsBt470M       TransferCharacteristics = 4
	TransferCharacteristicsBt470Bg      TransferCharacteristics = 5
	TransferCharacteristicsBt601        TransferCharacteristics = 6
	TransferCharacteristicsSmpte240     TransferCharacteristics = 7
	TransferCharacteristicsLinear       TransferCharacteristics = 8
	TransferCharacteristicsLog100       TransferCharacteristics = 9
	TransferCharacteristicsLog100sqrt10 TransferCharacteristics = 10
	TransferCharacteristicsIec61966     TransferCharacteristics = 11
	TransferCharacteristicsBt1361       TransferCharacteristics = 12
	TransferCharacteristicsSrgb         TransferCharacteristics = 13
	TransferCharacteristicsBt202010bit  TransferCharacteristics = 14
	TransferCharacteristicsBt202012bit  TransferCharacteristics = 15
	TransferCharacteristicsSmpte2084    TransferCharacteristics = 16
	TransferCharacteristicsSmpte428     TransferCharacteristics = 17
	TransferCharacteristicsHlg          TransferCharacteristics = 18
)

// TransferCharacteristicsFrom is impl TryFrom<u8> for
// TransferCharacteristics in moxcms src/cicp.rs. Each value maps to a
// variant, so it never fails.
func TransferCharacteristicsFrom(v uint8) TransferCharacteristics {
	if v == 0 || v == 3 || v >= 19 {
		return TransferCharacteristicsReserved
	}
	return TransferCharacteristics(v)
}

// hasTransferCurve is TransferCharacteristics::has_transfer_curve in moxcms
// src/cicp.rs.
func (t TransferCharacteristics) hasTransferCurve() bool {
	return t != TransferCharacteristicsReserved && t != TransferCharacteristicsUnspecified
}

// createRec709Parametric is create_rec709_parametric in moxcms src/cicp.rs.
// It returns the parameters g, a, b, c and d of the parametric curve of
// Rec. 709.
func createRec709Parametric() [5]float32 {
	// Port: upstream computes b, d and a in f64 and rounds them to f32. The
	// variables keep Go from an exact computation with constants.
	alpha, alpha1, beta := 1.09929682680944, 0.09929682680944, 0.018053968510807
	powExp := float32(0.45)
	g := 1 / powExp
	b := float32(alpha1 / alpha)
	c := float32(1) / float32(4.5)
	d := float32(4.5 * beta)
	a := float32(1 / alpha)
	return [5]float32{g, a, b, c, d}
}

// numTrcTableEntries is NUM_TRC_TABLE_ENTRIES of impl
// TryFrom<TransferCharacteristics> for ToneReprCurve in moxcms src/cicp.rs.
const numTrcTableEntries = 1024

// toneReprCurveFrom is impl TryFrom<TransferCharacteristics> for
// ToneReprCurve in moxcms src/cicp.rs. It returns the tone curve of the
// transfer characteristics, as ICC.1:2010 and Rec. ITU-R BT.2100-2 give it.
func toneReprCurveFrom(t TransferCharacteristics) (*ToneReprCurve, error) {
	switch t {
	case TransferCharacteristicsBt709, TransferCharacteristicsBt601,
		TransferCharacteristicsBt202010bit, TransferCharacteristicsBt202012bit:
		// The inverse of the OETF of Rec. ITU-T H.273 table 3, row 1, as a
		// parametric curve of ICC.1:2010 table 5. createRec709Parametric
		// holds the derivation.
		rec709 := createRec709Parametric()
		return &ToneReprCurve{Kind: CurveParametric, Parametric: rec709[:]}, nil
	case TransferCharacteristicsBt470M:
		return curveFromGamma(2.2), nil
	case TransferCharacteristicsBt470Bg:
		return curveFromGamma(2.8), nil
	case TransferCharacteristicsSmpte240:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, smpte240ToLinear)}, nil
	case TransferCharacteristicsLinear:
		return curveFromGamma(1), nil
	case TransferCharacteristicsLog100:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, log100ToLinear)}, nil
	case TransferCharacteristicsLog100sqrt10:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, log100Sqrt10ToLinear)}, nil
	case TransferCharacteristicsIec61966:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, iec61966ToLinear)}, nil
	case TransferCharacteristicsBt1361:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, bt1361ToLinear)}, nil
	case TransferCharacteristicsSrgb:
		return &ToneReprCurve{Kind: CurveParametric, Parametric: srgbParametric()}, nil
	case TransferCharacteristicsSmpte2084:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, pqToLinear)}, nil
	case TransferCharacteristicsSmpte428:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, smpte428ToLinear)}, nil
	case TransferCharacteristicsHlg:
		return &ToneReprCurve{Lut: buildTrcTable(numTrcTableEntries, hlgToLinear)}, nil
	default:
		// Reserved and Unspecified.
		return nil, &CmsError{Kind: UnsupportedTrc, Value: uint64(t)}
	}
}

// MatrixCoefficients is MatrixCoefficients in moxcms src/cicp.rs. It is the
// code point of the matrix coefficients in ISO/IEC 23091-4.
type MatrixCoefficients uint8

// The matrix coefficients of MatrixCoefficients in moxcms src/cicp.rs.
const (
	MatrixCoefficientsIdentity MatrixCoefficients = iota
	MatrixCoefficientsBt709
	MatrixCoefficientsUnspecified
	MatrixCoefficientsReserved
	MatrixCoefficientsFcc
	MatrixCoefficientsBt470Bg
	MatrixCoefficientsSmpte170m
	MatrixCoefficientsSmpte240m
	MatrixCoefficientsYCgCo
	MatrixCoefficientsBt2020Ncl
	MatrixCoefficientsBt2020Cl
	MatrixCoefficientsSmpte2085
	MatrixCoefficientsChromaticityDerivedNCL
	MatrixCoefficientsChromaticityDerivedCL
	MatrixCoefficientsICtCp
	MatrixCoefficientsIPtC2
	MatrixCoefficientsYCgCoRe
	MatrixCoefficientsYCgCoRo
)

// MatrixCoefficientsFrom is impl TryFrom<u8> for MatrixCoefficients in
// moxcms src/cicp.rs. It fails for a value larger than 17.
func MatrixCoefficientsFrom(v uint8) (MatrixCoefficients, error) {
	if v > uint8(MatrixCoefficientsYCgCoRo) {
		return 0, cmsError(InvalidCicp)
	}
	return MatrixCoefficients(v), nil
}
