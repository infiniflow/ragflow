// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/gray2rgb.rs.

package moxcms

// transformGray2RgbFusedExecutor is TransformGray2RgbFusedExecutor in
// moxcms src/conversions/gray2rgb.rs. Its table maps a gray sample to the
// sample of each output channel.
type transformGray2RgbFusedExecutor struct {
	fusedGamma []uint8
	srcLayout  Layout
	dstLayout  Layout
	bitDepth   int
}

// fuseGrayGamma builds the table of transformGray2RgbFusedExecutor, as
// make_gray_to_x and make_gray_to_gray_in_place in moxcms
// src/conversions/gray2rgb.rs do.
func fuseGrayGamma(grayLinear []float32, grayGamma []uint8, gammaLut int) []uint8 {
	fusedGamma := make([]uint8, gammaTableSize)
	maxLutSize := float32(gammaLut - 1)
	for i, src := range grayLinear[:min(len(grayLinear), len(fusedGamma))] {
		possibleValue := uint16(min(f32ToU32(roundF32(src*maxLutSize)), f32ToU32(maxLutSize)))
		fusedGamma[i] = grayGamma[possibleValue]
	}
	return fusedGamma
}

// isGrayLayout reports whether l is LayoutGray or LayoutGrayAlpha.
func isGrayLayout(l Layout) bool {
	return l == LayoutGray || l == LayoutGrayAlpha
}

// makeGrayToX is make_gray_to_x in moxcms src/conversions/gray2rgb.rs. It
// returns a transform from gray to gray or to RGB through one table.
func makeGrayToX(srcLayout, dstLayout Layout, grayLinear []float32, grayGamma []uint8, bitDepth, gammaLut int) (TransformExecutor, error) {
	if !isGrayLayout(srcLayout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	fusedGamma := fuseGrayGamma(grayLinear, grayGamma, gammaLut)
	switch dstLayout {
	case LayoutRgb, LayoutRgba, LayoutGray, LayoutGrayAlpha:
		return &transformGray2RgbFusedExecutor{
			fusedGamma: fusedGamma,
			srcLayout:  srcLayout,
			dstLayout:  dstLayout,
			bitDepth:   bitDepth,
		}, nil
	default:
		return nil, cmsError(UnsupportedProfileConnection)
	}
}

// makeGrayToGrayInPlace is make_gray_to_gray_in_place in moxcms
// src/conversions/gray2rgb.rs. It returns an in-place transform from gray
// to gray through one table.
func makeGrayToGrayInPlace(layout Layout, grayLinear []float32, grayGamma []uint8, bitDepth, gammaLut int) (InPlaceTransformExecutor, error) {
	if !isGrayLayout(layout) {
		return nil, cmsError(UnsupportedProfileConnection)
	}
	t := &transformGray2RgbFusedExecutor{
		fusedGamma: fuseGrayGamma(grayLinear, grayGamma, gammaLut),
		srcLayout:  layout,
		dstLayout:  layout,
		bitDepth:   bitDepth,
	}
	return inPlace{t: t, channels: layout.Channels()}, nil
}

// Transform is TransformExecutor::transform for
// TransformGray2RgbFusedExecutor in moxcms src/conversions/gray2rgb.rs.
func (t *transformGray2RgbFusedExecutor) Transform(src, dst []uint8) error {
	srcCn, dstCn := t.srcLayout, t.dstLayout
	srcChannels, dstChannels := srcCn.Channels(), dstCn.Channels()
	if err := checkLanes(len(src), srcChannels, len(dst), dstChannels); err != nil {
		return err
	}
	isGrayAlpha := srcCn == LayoutGrayAlpha
	maxValue := uint8(uint32(1)<<t.bitDepth - 1)
	for i, j := 0, 0; i+srcChannels <= len(src) && j+dstChannels <= len(dst); i, j = i+srcChannels, j+dstChannels {
		s, d := src[i:i+srcChannels], dst[j:j+dstChannels]
		g := t.fusedGamma[s[0]]
		a := maxValue
		if isGrayAlpha {
			a = s[1]
		}
		d[0] = g
		switch dstCn {
		case LayoutGrayAlpha:
			d[1] = a
		case LayoutRgb:
			d[1] = g
			d[2] = g
		case LayoutRgba:
			d[1] = g
			d[2] = g
			d[3] = a
		}
	}
	return nil
}
