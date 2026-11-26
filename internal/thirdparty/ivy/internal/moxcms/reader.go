// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/reader.rs.

package moxcms

import (
	"encoding/binary"
	"fmt"
)

// Port: upstream reads the offset and the size of a tag as usize. The port
// keeps them in uint64, and it compares them with the length of the data
// before it converts them to int. So the reader never panics on a 32-bit
// system.

// s15Fixed16NumberToFloat is s15_fixed16_number_to_float in moxcms
// src/reader.rs.
func s15Fixed16NumberToFloat(a int32) float32 {
	return float32(a) / 65536
}

// s15Fixed16NumberToDouble is s15_fixed16_number_to_double in moxcms
// src/reader.rs.
func s15Fixed16NumberToDouble(a int32) float64 {
	return float64(a) / 65536
}

// uint16NumberToFloatFast is uint16_number_to_float_fast in moxcms
// src/reader.rs.
func uint16NumberToFloatFast(a uint32) float32 {
	return float32(a) * (1.0 / 65535.0)
}

// uint8NumberToFloatFast is uint8_number_to_float_fast in moxcms
// src/reader.rs.
func uint8NumberToFloatFast(a uint8) float32 {
	return float32(a) * (1.0 / 255.0)
}

// tagSlice returns b[entry:entry+size]. It reports false when the slice is
// not in b, or when entry+size overflows.
func tagSlice(b []byte, entry, size uint64) ([]byte, bool, error) {
	last, err := safeAdd(size, entry)
	if err != nil {
		return nil, false, err
	}
	if last > uint64(len(b)) {
		return nil, false, nil
	}
	return b[entry:last], true, nil
}

// readLutType is ColorProfile::read_lut_type in moxcms src/reader.rs. It
// returns the type of the LUT tag at entry.
func readLutType(b []byte, entry, size uint64) (LutType, error) {
	if size == 0 {
		size = tagSize
	}
	t, ok, err := tagSlice(b, entry, size)
	if err != nil {
		return 0, err
	}
	if !ok || len(t) < 48 {
		return 0, cmsError(InvalidProfile)
	}
	return lutTypeFrom(binary.BigEndian.Uint32(t))
}

// readLutTableF32 is ColorProfile::read_lut_table_f32 in moxcms
// src/reader.rs. It reads a table of entries of 8 or 16 bits.
func readLutTableF32(table []byte, lutType LutType) LutStore {
	if lutType == LutTypeLut16 {
		clut := make(LutStore16, len(table)/2)
		for i := range clut {
			clut[i] = binary.BigEndian.Uint16(table[2*i:])
		}
		return clut
	}
	return LutStore8(append([]byte(nil), table...))
}

// readNestedToneCurves is ColorProfile::read_nested_tone_curves in moxcms
// src/reader.rs. It reads length curves that follow each other from offset,
// each one aligned to 4 bytes.
func readNestedToneCurves(b []byte, offset uint64, length int, options ParsingOptions) ([]*ToneReprCurve, error) {
	curveOffset := offset
	var curves []*ToneReprCurve
	for range length {
		end, err := safeAdd(curveOffset, 12)
		if err != nil {
			return nil, err
		}
		if uint64(len(b)) < end {
			return nil, cmsError(InvalidProfile)
		}
		var size uint64
		curve, err := readTrcTag(b, curveOffset, 0, &size, options)
		if err != nil {
			return nil, err
		}
		if curve == nil {
			return nil, cmsError(InvalidProfile)
		}
		curves = append(curves, curve)
		curveOffset += size
		// Each curve starts on a multiple of 4 bytes.
		if curveOffset%4 != 0 {
			curveOffset += 4 - curveOffset%4
		}
	}
	return curves, nil
}

// readLutAbmType is ColorProfile::read_lut_abm_type in moxcms src/reader.rs.
// It reads a tag of the type lutAToBType or lutBToAType. toPcs is true for
// lutAToBType.
func readLutAbmType(b []byte, entry, size uint64, toPcs bool, options ParsingOptions) (LutWarehouse, error) {
	if size < 48 {
		return nil, nil
	}
	t, ok, err := tagSlice(b, entry, size)
	if err != nil {
		return nil, err
	}
	if !ok || len(t) < 48 {
		return nil, cmsError(InvalidProfile)
	}
	tagType := tagTypeDefinition(binary.BigEndian.Uint32(t))
	if tagType != tagTypeMabLut && tagType != tagTypeMbaLut {
		return nil, nil
	}
	inChannels := t[8]
	outChannels := t[9]
	if inChannels > 4 && outChannels > 4 {
		return nil, nil
	}
	u32 := func(i int) uint64 { return uint64(binary.BigEndian.Uint32(t[i:])) }
	aCurveOffset := u32(28)
	clutOffset := u32(24)
	mCurveOffset := u32(20)
	matrixOffset := u32(16)
	bCurveOffset := u32(12)

	transform := Matrix3dIdentity
	var bias Vector3d
	if matrixOffset != 0 {
		matrixEnd, err := safeAdd(matrixOffset, 12*4)
		if err != nil {
			return nil, err
		}
		if uint64(len(t)) < matrixEnd {
			return nil, cmsError(InvalidProfile)
		}
		m := t[matrixOffset:matrixEnd]
		if bias, err = readVector3d(m[36:48]); err != nil {
			return nil, err
		}
		if transform, err = readMatrix3d(m); err != nil {
			return nil, err
		}
	}

	var gridPoints [16]uint8
	var clutTable LutStore
	if clutOffset != 0 {
		end, err := safeAdd(clutOffset, 20)
		if err != nil {
			return nil, err
		}
		if end > uint64(len(t)) {
			return nil, cmsError(InvalidProfile)
		}
		copy(gridPoints[:], t[clutOffset:clutOffset+16])

		clutSize := uint32(1)
		for _, g := range gridPoints[:min(int(inChannels), len(gridPoints))] {
			if clutSize, err = safeMulU32(clutSize, uint32(g)); err != nil {
				return nil, err
			}
		}
		if clutSize, err = safeMulU32(clutSize, uint32(outChannels)); err != nil {
			return nil, err
		}
		if clutSize == 0 {
			return nil, incorrectlyFormedLut("the size of the clut is zero")
		}
		if clutSize > 10_000_000 {
			return nil, incorrectlyFormedLut("the clut has more than 10000000 points")
		}

		// Check the dimensions of the grid.
		gridStride := uint64(1)
		lastIndex := uint64(0)
		dims := gridPoints[:min(int(inChannels), len(gridPoints))]
		for i := len(dims) - 1; i >= 0; i-- {
			dim := uint64(dims[i])
			if dim < 2 {
				return nil, incorrectlyFormedLut("a dimension of the grid is less than 2")
			}
			l, err := safeMul(dim-1, gridStride)
			if err == nil {
				l, err = safeAdd(l, lastIndex)
			}
			if err != nil {
				return nil, incorrectlyFormedLut("the dimensions of the lut overflow")
			}
			lastIndex = l
			if gridStride, err = safeMul(gridStride, dim); err != nil {
				return nil, incorrectlyFormedLut("the dimensions of the grid overflow")
			}
		}
		if lastIndex, err = safeMul(lastIndex, uint64(outChannels)); err != nil {
			return nil, incorrectlyFormedLut("the dimensions of the grid overflow")
		}
		if lastIndex >= uint64(clutSize) {
			return nil, incorrectlyFormedLut(fmt.Sprintf("the clut must hold at least %d entries", lastIndex))
		}

		entrySize := t[clutOffset+16]
		if entrySize != 1 && entrySize != 2 {
			return nil, cmsError(InvalidProfile)
		}
		n, err := safeMulU32(clutSize, uint32(entrySize))
		if err != nil {
			return nil, err
		}
		clutEnd, err := safeAdd(end, uint64(n))
		if err != nil {
			return nil, err
		}
		if uint64(len(t)) < clutEnd {
			return nil, cmsError(InvalidProfile)
		}
		lutType := LutTypeLut16
		if entrySize == 1 {
			lutType = LutTypeLut8
		}
		clutTable = readLutTableF32(t[end:clutEnd], lutType)
	}

	inCount, outCount := int(outChannels), int(inChannels)
	if toPcs {
		inCount, outCount = int(inChannels), int(outChannels)
	}
	var aCurves, mCurves, bCurves []*ToneReprCurve
	if aCurveOffset != 0 {
		if aCurves, err = readNestedToneCurves(t, aCurveOffset, inCount, options); err != nil {
			return nil, err
		}
	}
	if mCurveOffset != 0 {
		if mCurves, err = readNestedToneCurves(t, mCurveOffset, outCount, options); err != nil {
			return nil, err
		}
	}
	if bCurveOffset != 0 {
		if bCurves, err = readNestedToneCurves(t, bCurveOffset, outCount, options); err != nil {
			return nil, err
		}
	}
	return &LutMultidimensionalType{
		NumInputChannels:  inChannels,
		NumOutputChannels: outChannels,
		Matrix:            transform,
		Clut:              clutTable,
		ACurves:           aCurves,
		BCurves:           bCurves,
		MCurves:           mCurves,
		GridPoints:        gridPoints,
		Bias:              bias,
	}, nil
}

// incorrectlyFormedLut returns the error IncorrectlyFormedLut with a
// message.
func incorrectlyFormedLut(msg string) error {
	return &CmsError{Kind: IncorrectlyFormedLut, Message: msg}
}

// readLutAToBType is ColorProfile::read_lut_a_to_b_type in moxcms
// src/reader.rs. It reads a tag of the type lut8Type or lut16Type.
func readLutAToBType(b []byte, entry, size uint64, options ParsingOptions) (LutWarehouse, error) {
	if size < 48 {
		return nil, nil
	}
	t, ok, err := tagSlice(b, entry, size)
	if err != nil {
		return nil, err
	}
	if !ok || len(t) < 48 {
		return nil, cmsError(InvalidProfile)
	}
	lutType, err := lutTypeFrom(binary.BigEndian.Uint32(t))
	if err != nil {
		return nil, err
	}
	if lutType != LutTypeLut8 && lutType != LutTypeLut16 {
		// Port: upstream asserts this, and its callers make sure of it.
		return nil, cmsError(InvalidProfile)
	}
	if lutType == LutTypeLut16 && len(t) < 52 {
		return nil, cmsError(InvalidProfile)
	}

	numInputTableEntries := uint16(256)
	numOutputTableEntries := uint16(256)
	inputOffset := 48
	entrySize := 1
	if lutType == LutTypeLut16 {
		numInputTableEntries = binary.BigEndian.Uint16(t[48:])
		numOutputTableEntries = binary.BigEndian.Uint16(t[50:])
		inputOffset = 52
		entrySize = 2
	}
	if numInputTableEntries < 2 || numInputTableEntries > 4096 ||
		numOutputTableEntries < 2 || numOutputTableEntries > 4096 {
		return nil, cmsError(InvalidProfile)
	}

	inChan := t[8]
	outChan := t[9]
	isPcs := inChan == 3 || outChan == 3
	if !isPcs || inChan < 1 || inChan > 4 || outChan < 1 || outChan > 4 {
		return nil, cmsError(InvalidProfile)
	}
	gridPoints := t[10]
	if gridPoints < 2 {
		return nil, cmsError(InvalidProfile)
	}
	clutSize32, err := safePowiU32(uint32(gridPoints), uint32(inChan))
	if err != nil {
		return nil, err
	}
	clutSize := uint64(clutSize32)
	if clutSize < 1 || clutSize > uint64(options.MaxAllowedClutSize) {
		return nil, cmsError(InvalidProfile)
	}

	transform, err := readMatrix3d(t[12:48])
	if err != nil {
		return nil, err
	}

	lutInputSize, err := safeMulU16(numInputTableEntries, uint16(inChan))
	if err != nil {
		return nil, err
	}
	linearizationTableEnd := uint64(lutInputSize)*uint64(entrySize) + uint64(inputOffset)
	if uint64(len(t)) < linearizationTableEnd {
		return nil, cmsError(InvalidProfile)
	}
	linearizationTable := readLutTableF32(t[inputOffset:linearizationTableEnd], lutType)

	clutOffset := linearizationTableEnd
	clutDataSize := clutSize * uint64(outChan) * uint64(entrySize)
	if uint64(len(t)) < clutOffset+clutDataSize {
		return nil, cmsError(InvalidProfile)
	}
	clutTable := readLutTableF32(t[clutOffset:clutOffset+clutDataSize], lutType)

	outputOffset := clutOffset + clutDataSize
	outputSize := uint64(numOutputTableEntries) * uint64(outChan)
	shapedOutput := outputOffset + outputSize*uint64(entrySize)
	if uint64(len(t)) < shapedOutput {
		return nil, cmsError(InvalidProfile)
	}
	gammaTable := readLutTableF32(t[outputOffset:shapedOutput], lutType)

	return &LutDataType{
		NumInputTableEntries:  numInputTableEntries,
		NumOutputTableEntries: numOutputTableEntries,
		NumInputChannels:      inChan,
		NumOutputChannels:     outChan,
		NumClutGridPoints:     gridPoints,
		Matrix:                transform,
		InputTable:            linearizationTable,
		ClutTable:             clutTable,
		OutputTable:           gammaTable,
		LutType:               lutType,
	}, nil
}

// readLutTag is ColorProfile::read_lut_tag in moxcms src/reader.rs. It reads
// a LUT tag of any of the four LUT types. It returns nil for a tag that it
// ignores.
func readLutTag(b []byte, entry, size uint64, options ParsingOptions) (LutWarehouse, error) {
	lutType, err := readLutType(b, entry, size)
	if err != nil {
		return nil, err
	}
	switch lutType {
	case LutTypeLut8, LutTypeLut16:
		return readLutAToBType(b, entry, size, options)
	default:
		return readLutAbmType(b, entry, size, lutType == LutTypeLutMab, options)
	}
}

// readTrcTagS is ColorProfile::read_trc_tag_s in moxcms src/reader.rs.
func readTrcTagS(b []byte, entry, size uint64, options ParsingOptions) (*ToneReprCurve, error) {
	var readSize uint64
	return readTrcTag(b, entry, size, &readSize, options)
}

// readTrcTag is ColorProfile::read_trc_tag in moxcms src/reader.rs. It reads
// a tone curve of the type curveType or parametricCurveType. A size of zero
// reads the curve up to the end of b. It stores the size of the curve in
// readSize.
func readTrcTag(b []byte, entry, size uint64, readSize *uint64, options ParsingOptions) (*ToneReprCurve, error) {
	end, err := safeAdd(entry, 4)
	if err != nil {
		return nil, err
	}
	if uint64(len(b)) < end {
		return nil, nil
	}
	// The type of each tone curve must be known.
	curveType := tagTypeDefinition(binary.BigEndian.Uint32(b[entry:]))
	if size != 0 && size < tagSize {
		return nil, nil
	}
	last := uint64(len(b))
	if size != 0 {
		if last, err = safeAdd(size, entry); err != nil {
			return nil, err
		}
	}
	if last > uint64(len(b)) {
		return nil, malformedTrcCurve("the data of the tone curve ends too early")
	}
	t := b[entry:last]
	if len(t) < tagSize {
		return nil, malformedTrcCurve("the data of the tone curve ends too early")
	}
	switch curveType {
	case tagTypeLutToneCurve:
		entryCount := uint64(binary.BigEndian.Uint32(t[8:]))
		if entryCount == 0 {
			// The size of the type, the reserved field and the count, so that
			// a curve after this one in an mAB or mBA tag is read from its own
			// offset.
			*readSize = 12
			return &ToneReprCurve{}, nil
		}
		if entryCount > uint64(options.MaxAllowedTrcSize) {
			return nil, cmsError(CurveLutIsTooLarge)
		}
		curveEnd := entryCount*2 + 12
		if uint64(len(t)) < curveEnd {
			return nil, malformedTrcCurve("the tone curve ends too early")
		}
		values := make([]uint16, entryCount)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(t[12+2*i:])
		}
		*readSize = curveEnd
		return &ToneReprCurve{Lut: values}, nil
	case tagTypeParametricToneCurve:
		entryCount := int(binary.BigEndian.Uint16(t[8:]))
		if entryCount > 4 {
			return nil, malformedTrcCurve("the parametric curve has an unknown function type")
		}
		countToLength := [5]int{1, 3, 4, 5, 7}
		n := countToLength[entryCount]
		if len(t) < 12+n*4 {
			return nil, malformedTrcCurve("the parametric curve ends too early")
		}
		params := make([]float32, n)
		for i := range params {
			params[i] = s15Fixed16NumberToFloat(int32(binary.BigEndian.Uint32(t[12+4*i:])))
		}
		if entryCount == 1 || entryCount == 2 {
			// The functions of type 1 and 2 divide by a.
			if params[1] == 0 {
				return nil, cmsError(ParametricCurveZeroDivision)
			}
		}
		*readSize = uint64(12 + n*4)
		return &ToneReprCurve{Kind: CurveParametric, Parametric: params}, nil
	default:
		return nil, malformedTrcCurve("unknown type of tone curve")
	}
}

// malformedTrcCurve returns the error MalformedTrcCurve with a message.
func malformedTrcCurve(msg string) error {
	return &CmsError{Kind: MalformedTrcCurve, Message: msg}
}

// readChadTag is ColorProfile::read_chad_tag in moxcms src/reader.rs. It
// reads the matrix of a chad tag.
func readChadTag(b []byte, entry, size uint64) (*Matrix3d, error) {
	t, ok, err := tagSlice(b, entry, size)
	if err != nil {
		return nil, err
	}
	if !ok || uint64(len(b))-entry < 8 {
		return nil, cmsError(InvalidProfile)
	}
	if size < 8 {
		return nil, nil
	}
	if (size-8)/4 != 9 {
		return nil, nil
	}
	if tagTypeDefinition(binary.BigEndian.Uint32(b[entry:])) != tagTypeS15Fixed16Array {
		return nil, cmsError(InvalidProfile)
	}
	if len(b) < 9*4+8 {
		return nil, cmsError(InvalidProfile)
	}
	if len(t)-8 != 36 {
		return nil, cmsError(InvalidProfile)
	}
	m, err := readMatrix3d(t[8:])
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// readXyzTag is ColorProfile::read_xyz_tag in moxcms src/reader.rs. It
// returns the zero Xyzd for a tag that is too small or of another type.
func readXyzTag(b []byte, entry, size uint64) (Xyzd, error) {
	if size < tagSize {
		return Xyzd{}, nil
	}
	t, ok, err := tagSlice(b, entry, size)
	if err != nil {
		return Xyzd{}, err
	}
	if !ok {
		return Xyzd{}, cmsError(InvalidProfile)
	}
	if tagTypeDefinition(binary.BigEndian.Uint32(t)) != tagTypeXyz {
		return Xyzd{}, nil
	}
	if len(t) < 20 {
		return Xyzd{}, cmsError(InvalidProfile)
	}
	return Xyzd{
		X: s15Fixed16NumberToDouble(int32(binary.BigEndian.Uint32(t[8:]))),
		Y: s15Fixed16NumberToDouble(int32(binary.BigEndian.Uint32(t[12:]))),
		Z: s15Fixed16NumberToDouble(int32(binary.BigEndian.Uint32(t[16:]))),
	}, nil
}

// readXyzTagPtr is readXyzTag for an optional field of ColorProfile.
func readXyzTagPtr(b []byte, entry, size uint64) (*Xyzd, error) {
	v, err := readXyzTag(b, entry, size)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// readCicpTag is ColorProfile::read_cicp_tag in moxcms src/reader.rs.
func readCicpTag(b []byte, entry, size uint64) (*CicpProfile, error) {
	if size < tagSize {
		return nil, nil
	}
	t, ok, err := tagSlice(b, entry, size)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, cmsError(InvalidProfile)
	}
	if tagTypeDefinition(binary.BigEndian.Uint32(t)) != tagTypeCicp {
		return nil, nil
	}
	matrixCoefficients, err := MatrixCoefficientsFrom(t[10])
	if err != nil {
		return nil, err
	}
	return &CicpProfile{
		ColorPrimaries:          CicpColorPrimariesFrom(t[8]),
		TransferCharacteristics: TransferCharacteristicsFrom(t[9]),
		MatrixCoefficients:      matrixCoefficients,
		FullRange:               t[11] == 1,
	}, nil
}
