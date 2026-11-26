// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/lut_transforms.rs.

package moxcms

// matrixStage is MatrixStage in moxcms src/conversions/lut_transforms.rs. It
// applies each matrix, in order, to each triplet.
type matrixStage struct {
	matrices []Matrix3f
}

// Transform is InPlaceStage::transform for MatrixStage in moxcms
// src/conversions/lut_transforms.rs.
func (s matrixStage) Transform(dst []float32) error {
	applyMatrices(s.matrices, dst)
	return nil
}

// lutSampling is LUT_SAMPLING in moxcms src/conversions/lut_transforms.rs.
// It is the largest sample of 8 bits.
const lutSampling = 255

// grid4x3 is Grid4x3 in moxcms src/conversions/lut_transforms.rs. It is a 4D
// grid of three values in each node. It holds the whole grid, or a function
// that fills one plane of the last input at a time.
//
// Port: upstream is an enum with data. The port makes it a struct. Upstream
// can map the planes into the layout of a transform, so that it never holds
// the grid twice. The port makes the whole grid in f32 first, because the
// grid of the subset is small.
type grid4x3 struct {
	whole  []float32
	planes int
	plane  int
	fill   func(k int, out []float32) error
}

// intoF32 is Grid4x3::into_f32 in moxcms src/conversions/lut_transforms.rs.
// It returns the whole grid.
func (g grid4x3) intoF32() ([]float32, error) {
	if g.fill == nil {
		return g.whole, nil
	}
	out := make([]float32, g.planes*g.plane*3)
	for k := range g.planes {
		if err := g.fill(k, out[k*g.plane*3:(k+1)*g.plane*3]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pcsLabV4ToV2 is pcs_lab_v4_to_v2 in moxcms
// src/conversions/lut_transforms.rs. It scales Lab of the encoding of
// version 4 to the one of version 2, when the B2A LUT of profile is a
// lut16Type.
func pcsLabV4ToV2(profile *ColorProfile, intent RenderingIntent, lut []float32) {
	if profile.Pcs == DataColorSpaceLab && profile.pcsToDeviceIsLut16(intent) && len(lut)%3 == 0 {
		s := float32(65280.0 / 65535.0)
		matrixStage{matrices: []Matrix3f{{V: [3][3]float32{{s, 0, 0}, {0, s, 0}, {0, 0, s}}}}}.apply(lut)
	}
}

// pcsLabV2ToV4 is pcs_lab_v2_to_v4 in moxcms
// src/conversions/lut_transforms.rs. It scales Lab of the encoding of
// version 2 to the one of version 4, when the A2B LUT of profile is a
// lut16Type.
func pcsLabV2ToV4(profile *ColorProfile, intent RenderingIntent, lut []float32) {
	if profile.Pcs == DataColorSpaceLab && profile.deviceToPcsIsLut16(intent) && len(lut)%3 == 0 {
		s := float32(65535.0 / 65280.0)
		matrixStage{matrices: []Matrix3f{{V: [3][3]float32{{s, 0, 0}, {0, s, 0}, {0, 0, s}}}}}.apply(lut)
	}
}

// apply applies the matrices of s to lut.
func (s matrixStage) apply(lut []float32) {
	applyMatrices(s.matrices, lut)
}

// sourcePcsToDestPcs is source_pcs_to_dest_pcs in moxcms
// src/conversions/lut_transforms.rs. It converts each triplet of lut from
// the PCS of source to the PCS of dest, with the compensation of the black
// point.
func sourcePcsToDestPcs(source, dest *ColorProfile, intent RenderingIntent, applyBpcScaling func([]float32), lut []float32) error {
	pcsLabV2ToV4(source, intent, lut)

	switch {
	case source.Pcs == DataColorSpaceXyz && dest.Pcs == DataColorSpaceLab:
		applyBpcScaling(lut)
		if err := (stageXyzToLab{}).Transform(lut); err != nil {
			return err
		}
	case source.Pcs == DataColorSpaceLab && dest.Pcs == DataColorSpaceXyz:
		if err := (stageLabToXyz{}).Transform(lut); err != nil {
			return err
		}
		applyBpcScaling(lut)
	default:
		applyBpcScaling(lut)
	}

	pcsLabV4ToV2(dest, intent, lut)
	return nil
}

// bpcScaling returns the compensation of the black point between source
// and dest, as the closure apply_bpc_scaling of make_lut_transform in moxcms
// src/conversions/lut_transforms.rs does.
func bpcScaling(source, dest *ColorProfile, intent RenderingIntent) func([]float32) {
	srcBp, srcOk := source.detectBlackPoint(intent, deviceToPcs)
	destBp, destOk := dest.detectBlackPoint(intent, pcsToDevice)
	return func(lut []float32) {
		if srcOk && destOk {
			compensateBpcInLut(lut, srcBp, destBp)
		}
	}
}

// grid4x3ByPlanes is grid_4x3_by_planes in moxcms
// src/conversions/lut_transforms.rs, for samples of 8 bits. It returns the
// 4D grid of makeLutTransform as planes of the last ink, for an A2B LUT of
// the type lut8Type or lut16Type into a matrix shaper or a B2A LUT of the
// type lut8Type or lut16Type. It reports false otherwise.
func grid4x3ByPlanes(gridSize int, source, dest *ColorProfile, options TransformOptions) (grid4x3, bool, error) {
	deviceToPcsLut, ok := source.getDeviceToPcs(options.RenderingIntent).(*LutDataType)
	if !ok {
		return grid4x3{}, false, nil
	}
	if deviceToPcsLut.NumInputChannels != 4 {
		return grid4x3{}, false, cmsError(UnsupportedProfileConnection)
	}
	lutStage, err := stageLut4x3(deviceToPcsLut, options, source.Pcs)
	if err != nil {
		return grid4x3{}, false, err
	}

	// last turns the PCS into the device values of dest.
	var last func(out, scratch []float32) error
	switch {
	case dest.Pcs == DataColorSpaceXyz && dest.IsMatrixShaper():
		stage, err := inverseLutRgbXyzStage(dest, options)
		if err != nil {
			return grid4x3{}, false, err
		}
		last = func(out, _ []float32) error { return stage.Transform(out) }
	case dest.Pcs == DataColorSpaceLab:
		pcsToDeviceLut := dest.getPcsToDevice(options.RenderingIntent)
		if pcsToDeviceLut == nil {
			return grid4x3{}, false, cmsError(UnsupportedProfileConnection)
		}
		lut, ok := pcsToDeviceLut.(*LutDataType)
		if !ok {
			return grid4x3{}, false, nil
		}
		if lut.NumInputChannels != 3 || lut.NumOutputChannels != 3 {
			return grid4x3{}, false, cmsError(UnsupportedProfileConnection)
		}
		stage, err := stageLut3x3(lut, options, dest.Pcs)
		if err != nil {
			return grid4x3{}, false, err
		}
		last = func(out, scratch []float32) error {
			copy(scratch, out)
			return stage.Transform(scratch, out)
		}
	default:
		return grid4x3{}, false, nil
	}
	applyBpcScaling := bpcScaling(source, dest, options.RenderingIntent)

	plane := gridSize * gridSize * gridSize
	recpeq := 1 / float32(gridSize-1)
	samples := make([]float32, plane*4)
	for i := range plane {
		c, m, y := i/(gridSize*gridSize), i/gridSize%gridSize, i%gridSize
		samples[4*i] = float32(c) * recpeq
		samples[4*i+1] = float32(m) * recpeq
		samples[4*i+2] = float32(y) * recpeq
		samples[4*i+3] = 0
	}
	scratch := make([]float32, plane*3)
	fill := func(k int, out []float32) error {
		kv := float32(k) * recpeq
		for i := 3; i < len(samples); i += 4 {
			samples[i] = kv
		}
		if err := lutStage.Transform(samples, out); err != nil {
			return err
		}
		if err := sourcePcsToDestPcs(source, dest, options.RenderingIntent, applyBpcScaling, out); err != nil {
			return err
		}
		return last(out, scratch)
	}
	return grid4x3{planes: gridSize, plane: plane, fill: fill}, true, nil
}

// makeLutTransform is make_lut_transform in moxcms
// src/conversions/lut_transforms.rs, for samples of 8 bits. It builds a
// transform through a LUT: from CMYK to a profile of three channels, or
// between two profiles of three channels.
//
// Port: the subset leaves out the transform from RGB or Lab to CMYK, and
// returns UnsupportedProfileConnection for it. Upstream builds the
// transforms of the feature "any_to_any" only with that feature, which hayro
// does not enable.
func makeLutTransform(srcLayout Layout, source *ColorProfile, dstLayout Layout, dest *ColorProfile, options TransformOptions) (TransformExecutor, error) {
	applyBpcScaling := bpcScaling(source, dest, options.RenderingIntent)

	switch {
	case (source.ColorSpace == DataColorSpaceCmyk || source.ColorSpace == DataColorSpaceColor4) &&
		(dest.ColorSpace == DataColorSpaceRgb || dest.ColorSpace == DataColorSpaceLab):
		if err := source.ColorSpace.CheckLayout(srcLayout); err != nil {
			return nil, err
		}
		if err := dest.ColorSpace.CheckLayout(dstLayout); err != nil {
			return nil, err
		}
		if source.Pcs != DataColorSpaceXyz && source.Pcs != DataColorSpaceLab {
			return nil, cmsError(UnsupportedProfileConnection)
		}
		if dest.Pcs != DataColorSpaceLab && dest.Pcs != DataColorSpaceXyz {
			return nil, cmsError(UnsupportedProfileConnection)
		}

		const gridSize = 17

		lut, ok, err := grid4x3ByPlanes(gridSize, source, dest, options)
		if err != nil {
			return nil, err
		}
		if !ok {
			whole, err := makeLut4Whole(gridSize, source, dest, options, applyBpcScaling)
			if err != nil {
				return nil, err
			}
			lut = grid4x3{whole: whole}
		}

		isDestLinearProfile := dest.ColorSpace == DataColorSpaceRgb && dest.IsMatrixShaper() && dest.isLinearMatrixShaper()
		return makeTransform4x3(dstLayout, gridSize, bitDepth8, lut, options, dest.ColorSpace, isDestLinearProfile)

	case (source.ColorSpace == DataColorSpaceRgb || source.ColorSpace == DataColorSpaceLab) &&
		(dest.ColorSpace == DataColorSpaceCmyk || dest.ColorSpace == DataColorSpaceColor4):
		// RGB or Lab to CMYK.
		return nil, cmsError(UnsupportedProfileConnection)

	case source.ColorSpace.isThreeChannels() && dest.ColorSpace.isThreeChannels():
		if err := source.ColorSpace.CheckLayout(srcLayout); err != nil {
			return nil, err
		}
		if err := dest.ColorSpace.CheckLayout(dstLayout); err != nil {
			return nil, err
		}

		const gridSize = 33

		var lut []float32
		switch {
		case source.hasDeviceToPcsLut():
			deviceToPcsLut := source.getDeviceToPcs(options.RenderingIntent)
			if deviceToPcsLut == nil {
				return nil, cmsError(UnsupportedProfileConnection)
			}
			lut = createLut3SamplesNorm(gridSize)
			switch l := deviceToPcsLut.(type) {
			case *LutDataType:
				var err error
				if lut, err = createLut3x3(l, lut, options, source.Pcs); err != nil {
					return nil, err
				}
			case *LutMultidimensionalType:
				if err := prepareMab3x3(l, lut, options, source.Pcs); err != nil {
					return nil, err
				}
			}
		case source.IsMatrixShaper():
			var err error
			if lut, err = createRgbLinLut(source, gridSize, options); err != nil {
				return nil, err
			}
		default:
			return nil, cmsError(UnsupportedProfileConnection)
		}

		if err := sourcePcsToDestPcs(source, dest, options.RenderingIntent, applyBpcScaling, lut); err != nil {
			return nil, err
		}

		switch {
		case dest.hasPcsToDeviceLut():
			pcsToDeviceLut := dest.getPcsToDevice(options.RenderingIntent)
			if pcsToDeviceLut == nil {
				return nil, cmsError(UnsupportedProfileConnection)
			}
			switch l := pcsToDeviceLut.(type) {
			case *LutDataType:
				var err error
				if lut, err = createLut3x3(l, lut, options, dest.Pcs); err != nil {
					return nil, err
				}
			case *LutMultidimensionalType:
				if err := prepareMba3x3(l, lut, options, dest.Pcs); err != nil {
					return nil, err
				}
			}
		case dest.IsMatrixShaper():
			if err := prepareInverseLutRgbXyz(dest, lut, options); err != nil {
				return nil, err
			}
		default:
			return nil, cmsError(UnsupportedProfileConnection)
		}

		isDestLinearProfile := dest.ColorSpace == DataColorSpaceRgb && dest.IsMatrixShaper() && dest.isLinearMatrixShaper()
		return makeTransform3x3(srcLayout, dstLayout, gridSize, bitDepth8, lut, options, dest.ColorSpace, isDestLinearProfile), nil

	default:
		return nil, cmsError(UnsupportedProfileConnection)
	}
}

// makeLut4Whole is the branch of make_lut_transform in moxcms
// src/conversions/lut_transforms.rs that makes the whole 4D grid, when
// grid_4x3_by_planes cannot make it by planes.
func makeLut4Whole(gridSize int, source, dest *ColorProfile, options TransformOptions, applyBpcScaling func([]float32)) ([]float32, error) {
	deviceToPcsLut := source.getDeviceToPcs(options.RenderingIntent)
	if deviceToPcsLut == nil {
		return nil, &CmsError{Kind: UnsupportedLutRenderingIntent, Intent: source.RenderingIntent}
	}
	var lut []float32
	var err error
	switch l := deviceToPcsLut.(type) {
	case *LutDataType:
		lut, err = createLut4(gridSize, l, options, source.Pcs)
	case *LutMultidimensionalType:
		samples := createLut4NormSamples(gridSize)
		lut, err = prepareMab4x3(l, samples, options, source.Pcs)
	}
	if err != nil {
		return nil, err
	}

	if err := sourcePcsToDestPcs(source, dest, options.RenderingIntent, applyBpcScaling, lut); err != nil {
		return nil, err
	}

	switch dest.Pcs {
	case DataColorSpaceXyz:
		if !dest.IsMatrixShaper() {
			return nil, cmsError(UnsupportedProfileConnection)
		}
		if err := prepareInverseLutRgbXyz(dest, lut, options); err != nil {
			return nil, err
		}
	case DataColorSpaceLab:
		pcsToDeviceLut := dest.getPcsToDevice(options.RenderingIntent)
		if pcsToDeviceLut == nil {
			return nil, cmsError(UnsupportedProfileConnection)
		}
		switch l := pcsToDeviceLut.(type) {
		case *LutDataType:
			if lut, err = createLut3x3(l, lut, options, dest.Pcs); err != nil {
				return nil, err
			}
		case *LutMultidimensionalType:
			if err := prepareMba3x3(l, lut, options, dest.Pcs); err != nil {
				return nil, err
			}
		}
	}
	return lut, nil
}
