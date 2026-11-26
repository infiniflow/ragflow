// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/prelude_lut_xyz_rgb.rs.

package moxcms

// applyMatrices applies each matrix of matrices, in order, to each triplet
// of dst. It is the loop that MatrixStage::transform and
// XyzToRgbStage::transform in moxcms share.
func applyMatrices(matrices []Matrix3f, dst []float32) {
	for _, m := range matrices {
		for i := 0; i+3 <= len(dst); i += 3 {
			x, y, z := dst[i], dst[i+1], dst[i+2]
			dst[i] = mlaf(mlaf(float32(x*m.V[0][0]), y, m.V[0][1]), z, m.V[0][2])
			dst[i+1] = mlaf(mlaf(float32(x*m.V[1][0]), y, m.V[1][1]), z, m.V[1][2])
			dst[i+2] = mlaf(mlaf(float32(x*m.V[2][0]), y, m.V[2][1]), z, m.V[2][2])
		}
	}
}

// xyzToRgbStage is XyzToRgbStage in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs. It converts XYZ to the RGB of a
// matrix shaper, with the values scaled to the range from 0 to 1.
type xyzToRgbStage struct {
	// gamma holds one table for each distinct curve. The channels with the
	// same curve share a table.
	gamma [][]uint8
	// channels holds the index into gamma of the table of each channel.
	channels [3]int
	matrices []Matrix3f
	bitDepth int
	gammaLut int
}

// Transform is InPlaceStage::transform for XyzToRgbStage in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs.
//
// Port: upstream makes an Rgb of the three values. The port reads them
// directly.
func (s *xyzToRgbStage) Transform(dst []float32) error {
	applyMatrices(s.matrices, dst)

	maxColors := (1 << s.bitDepth) - 1
	colorScale := 1 / float32(maxColors)
	lutCap := float32(s.gammaLut - 1)
	rGamma := s.gamma[s.channels[0]]
	gGamma := s.gamma[s.channels[1]]
	bGamma := s.gamma[s.channels[2]]
	index := func(v float32) uint16 {
		return f32ToU16(maxF32(minF32(mlaf(0.5, v, lutCap), lutCap), 0))
	}
	for i := 0; i+3 <= len(dst); i += 3 {
		r := index(dst[i])
		g := index(dst[i+1])
		b := index(dst[i+2])
		dst[i] = float32(rGamma[r]) * colorScale
		dst[i+1] = float32(gGamma[g]) * colorScale
		dst[i+2] = float32(bGamma[b]) * colorScale
	}
	return nil
}

// rgbLinearizationStage is RgbLinearizationStage in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs. It linearizes the integer nodes
// of a grid of samples nodes in each dimension.
type rgbLinearizationStage struct {
	rLin     []float32
	gLin     []float32
	bLin     []float32
	bitDepth int
	samples  int
}

// transform is RgbLinearizationStage::transform in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs.
func (s *rgbLinearizationStage) transform(src []uint8, dst []float32) error {
	if len(src)%3 != 0 || len(dst)%3 != 0 {
		return cmsError(LaneMultipleOfChannels)
	}
	scale := float32(int(1)<<s.bitDepth-1) / (float32(s.samples) - 1)
	cappedValue := float32(int(1)<<s.bitDepth - 1)
	index := func(v uint8) uint16 {
		return f32ToU16(minF32(maxF32(roundF32(float32(v)*scale), 0), cappedValue))
	}
	for i := 0; i+3 <= len(src) && i+3 <= len(dst); i += 3 {
		dst[i] = s.rLin[index(src[i])]
		dst[i+1] = s.gLin[index(src[i+1])]
		dst[i+2] = s.bLin[index(src[i+2])]
	}
	return nil
}

// createRgbLinLut is create_rgb_lin_lut in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs, for samples of 8 bits. It returns
// the XYZ of each node of a grid of gridSize nodes in each dimension, for
// the matrix shaper source.
func createRgbLinLut(source *ColorProfile, gridSize int, opts TransformOptions) ([]float32, error) {
	lutOrigins := createLut3Samples(gridSize)

	linR, err := source.buildRLinearizeTable(linearCap8, bitDepth8, opts.AllowUseCicpTransfer)
	if err != nil {
		return nil, err
	}
	linG, err := source.buildGLinearizeTable(linearCap8, bitDepth8, opts.AllowUseCicpTransfer)
	if err != nil {
		return nil, err
	}
	linB, err := source.buildBLinearizeTable(linearCap8, bitDepth8, opts.AllowUseCicpTransfer)
	if err != nil {
		return nil, err
	}

	linStage := rgbLinearizationStage{rLin: linR, gLin: linG, bLin: linB, bitDepth: bitDepth8, samples: gridSize}
	lut := make([]float32, len(lutOrigins))
	if err := linStage.transform(lutOrigins, lut); err != nil {
		return nil, err
	}

	xyzToRgb := source.RgbToXyzMatrix()
	scale := float32(32768.0 / 65535.0)
	matrices := []Matrix3f{
		xyzToRgb.ToF32(),
		{V: [3][3]float32{{scale, 0, 0}, {0, scale, 0}, {0, 0, scale}}},
	}
	stage := matrixStage{matrices: matrices}
	if err := stage.Transform(lut); err != nil {
		return nil, err
	}
	return lut, nil
}

// prepareInverseLutRgbXyz is prepare_inverse_lut_rgb_xyz in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs. It converts each XYZ triplet of
// lut to the RGB of the matrix shaper dest.
func prepareInverseLutRgbXyz(dest *ColorProfile, lut []float32, options TransformOptions) error {
	stage, err := inverseLutRgbXyzStage(dest, options)
	if err != nil {
		return err
	}
	return stage.Transform(lut)
}

// inverseLutRgbXyzStage is inverse_lut_rgb_xyz_stage in moxcms
// src/conversions/prelude_lut_xyz_rgb.rs, for samples of 8 bits. It returns
// the stage that prepareInverseLutRgbXyz applies, so that a caller can apply
// it to a LUT in parts.
func inverseLutRgbXyzStage(dest *ColorProfile, options TransformOptions) (*xyzToRgbStage, error) {
	// The channels with the same curve share its table, as the three of sRGB
	// do.
	curves := [3]*ToneReprCurve{dest.RedTrc, dest.GreenTrc, dest.BlueTrc}
	var gamma [][]uint8
	var channels [3]int
	for c, curve := range curves {
		same := -1
		for k := range c {
			if curves[k].equal(curve) {
				same = k
				break
			}
		}
		if same >= 0 {
			channels[c] = channels[same]
			continue
		}
		t, err := dest.buildGammaTable(curve, gammaCap8, bitDepth8, options.AllowUseCicpTransfer)
		if err != nil {
			return nil, err
		}
		gamma = append(gamma, t)
		channels[c] = len(gamma) - 1
	}

	xyzToRgb := dest.RgbToXyzMatrix().Inverse()
	scale := float32(65535.0 / 32768.0)
	matrices := []Matrix3f{
		{V: [3][3]float32{{scale, 0, 0}, {0, scale, 0}, {0, 0, scale}}},
		xyzToRgb.ToF32(),
	}
	return &xyzToRgbStage{
		gamma:    gamma,
		channels: channels,
		matrices: matrices,
		gammaLut: gammaCap8,
		bitDepth: bitDepth8,
	}, nil
}
