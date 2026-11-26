// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/profile.rs.

package moxcms

import "encoding/binary"

// maxProfileSize is MAX_PROFILE_SIZE in moxcms src/profile.rs. It is the
// largest profile, in bytes, that ParsingOptions allows by default.
const maxProfileSize = 1024 * 1024 * 10

// profileSignature is ProfileSignature::Acsp in moxcms src/profile.rs. It is
// the signature that each profile holds at offset 36.
var profileSignature = sig("acsp")

// ProfileVersion is ProfileVersion in moxcms src/profile.rs. It is the
// version of the ICC specification of a profile.
type ProfileVersion uint32

// The versions of ProfileVersion in moxcms src/profile.rs.
const (
	ProfileVersionV20     ProfileVersion = 0x02000000
	ProfileVersionV21     ProfileVersion = 0x02100000
	ProfileVersionV22     ProfileVersion = 0x02200000
	ProfileVersionV23     ProfileVersion = 0x02300000
	ProfileVersionV24     ProfileVersion = 0x02400000
	ProfileVersionV40     ProfileVersion = 0x04000000
	ProfileVersionV41     ProfileVersion = 0x04100000
	ProfileVersionV42     ProfileVersion = 0x04200000
	ProfileVersionV43     ProfileVersion = 0x04300000
	ProfileVersionV44     ProfileVersion = 0x04400000
	ProfileVersionUnknown ProfileVersion = 0x04400001
)

// profileVersionFrom is impl TryFrom<u32> for ProfileVersion in moxcms
// src/profile.rs. It maps a version with a patch level to the version of
// its major and minor numbers. It rejects version 0.x, except the value 0,
// and versions 5 and later.
func profileVersionFrom(v uint32) (ProfileVersion, error) {
	switch v {
	case 0:
		// Some profiles in the field hold a version of zero.
		return ProfileVersionV20, nil
	case 0x02000000, 0x02100000, 0x02200000, 0x02300000, 0x02400000,
		0x04000000, 0x04100000, 0x04200000, 0x04300000, 0x04400000:
		return ProfileVersion(v), nil
	}
	major := (v >> 24) & 0xff
	minor := (v >> 20) & 0x0f
	switch major {
	case 2:
		switch minor {
		case 0:
			return ProfileVersionV20, nil
		case 1:
			return ProfileVersionV21, nil
		case 2:
			return ProfileVersionV22, nil
		case 3:
			return ProfileVersionV23, nil
		default:
			return ProfileVersionV24, nil
		}
	case 3:
		return ProfileVersionV24, nil
	case 4:
		switch minor {
		case 0:
			return ProfileVersionV40, nil
		case 1:
			return ProfileVersionV41, nil
		case 2:
			return ProfileVersionV42, nil
		case 3:
			return ProfileVersionV43, nil
		default:
			return ProfileVersionV44, nil
		}
	default:
		return 0, cmsError(InvalidProfile)
	}
}

// DataColorSpace is DataColorSpace in moxcms src/profile.rs. It is the color
// space of the data of a profile, or of its profile connection space.
type DataColorSpace uint32

// The color spaces of DataColorSpace in moxcms src/profile.rs.
const (
	DataColorSpaceXyz DataColorSpace = iota
	DataColorSpaceLab
	DataColorSpaceLuv
	DataColorSpaceYCbr
	DataColorSpaceYxy
	DataColorSpaceRgb
	DataColorSpaceGray
	DataColorSpaceHsv
	DataColorSpaceHls
	DataColorSpaceCmyk
	DataColorSpaceCmy
	DataColorSpaceColor2
	DataColorSpaceColor3
	DataColorSpaceColor4
	DataColorSpaceColor5
	DataColorSpaceColor6
	DataColorSpaceColor7
	DataColorSpaceColor8
	DataColorSpaceColor9
	DataColorSpaceColor10
	DataColorSpaceColor11
	DataColorSpaceColor12
	DataColorSpaceColor13
	DataColorSpaceColor14
	DataColorSpaceColor15
)

// dataColorSpaceSignatures holds the signature of each DataColorSpace, in
// the order of the constants.
var dataColorSpaceSignatures = [...]string{
	"XYZ ", "Lab ", "Luv ", "YCbr", "Yxy ", "RGB ", "GRAY", "HSV ", "HLS ",
	"CMYK", "CMY ", "2CLR", "3CLR", "4CLR", "5CLR", "6CLR", "7CLR", "8CLR",
	"9CLR", "ACLR", "BCLR", "CCLR", "DCLR", "ECLR", "FCLR",
}

// dataColorSpaceFrom is impl TryFrom<u32> for DataColorSpace in moxcms
// src/profile.rs.
func dataColorSpaceFrom(v uint32) (DataColorSpace, error) {
	for i, s := range dataColorSpaceSignatures {
		if v == sig(s) {
			return DataColorSpace(i), nil
		}
	}
	return 0, cmsError(InvalidProfile)
}

// CheckLayout is DataColorSpace::check_layout in moxcms src/profile.rs. It
// returns an error when layout cannot hold the data of the color space.
func (c DataColorSpace) CheckLayout(layout Layout) error {
	var unsupported bool
	switch c {
	case DataColorSpaceLab, DataColorSpaceRgb:
		unsupported = layout != LayoutRgb && layout != LayoutRgba
	case DataColorSpaceGray:
		unsupported = layout != LayoutGray && layout != LayoutGrayAlpha
	case DataColorSpaceCmyk, DataColorSpaceColor4:
		unsupported = layout != LayoutRgba
	case DataColorSpaceColor2:
		unsupported = layout != LayoutGrayAlpha
	case DataColorSpaceColor5:
		unsupported = layout != LayoutInks5
	case DataColorSpaceColor6:
		unsupported = layout != LayoutInks6
	case DataColorSpaceColor7:
		unsupported = layout != LayoutInks7
	case DataColorSpaceColor8:
		unsupported = layout != LayoutInks8
	case DataColorSpaceColor9:
		unsupported = layout != LayoutInks9
	case DataColorSpaceColor10:
		unsupported = layout != LayoutInks10
	case DataColorSpaceColor11:
		unsupported = layout != LayoutInks11
	case DataColorSpaceColor12:
		unsupported = layout != LayoutInks12
	case DataColorSpaceColor13:
		unsupported = layout != LayoutInks13
	case DataColorSpaceColor14:
		unsupported = layout != LayoutInks14
	case DataColorSpaceColor15:
		unsupported = layout != LayoutInks15
	default:
		// Xyz, Luv, YCbr, Yxy, Hsv, Hls, Cmy and Color3.
		unsupported = layout != LayoutRgb
	}
	if unsupported {
		return cmsError(InvalidLayout)
	}
	return nil
}

// isThreeChannels is DataColorSpace::is_three_channels in moxcms
// src/profile.rs.
func (c DataColorSpace) isThreeChannels() bool {
	switch c {
	case DataColorSpaceXyz, DataColorSpaceLab, DataColorSpaceLuv, DataColorSpaceYCbr,
		DataColorSpaceYxy, DataColorSpaceRgb, DataColorSpaceHsv, DataColorSpaceHls,
		DataColorSpaceCmy, DataColorSpaceColor3:
		return true
	}
	return false
}

// ProfileClass is ProfileClass in moxcms src/profile.rs. It is the class of
// a profile.
type ProfileClass uint32

// The classes of ProfileClass in moxcms src/profile.rs.
const (
	ProfileClassInputDevice ProfileClass = iota
	ProfileClassDisplayDevice
	ProfileClassOutputDevice
	ProfileClassDeviceLink
	ProfileClassColorSpace
	ProfileClassAbstract
	ProfileClassNamed
)

// profileClassFrom is impl TryFrom<u32> for ProfileClass in moxcms
// src/profile.rs.
func profileClassFrom(v uint32) (ProfileClass, error) {
	for i, s := range [...]string{"scnr", "mntr", "prtr", "link", "spac", "abst", "nmcl"} {
		if v == sig(s) {
			return ProfileClass(i), nil
		}
	}
	return 0, cmsError(InvalidProfile)
}

// LutStore is LutStore in moxcms src/profile.rs. It holds the entries of a
// table of a LUT. It is a LutStore8 or a LutStore16.
//
// Port: upstream is an enum with data. The port makes it an interface.
type LutStore interface {
	// ToClutF32 is LutStore::to_clut_f32 in moxcms src/helpers.rs. It
	// returns the entries scaled to the range from 0 to 1.
	ToClutF32() []float32
}

// LutStore8 is LutStore::Store8 in moxcms src/profile.rs.
type LutStore8 []uint8

// LutStore16 is LutStore::Store16 in moxcms src/profile.rs.
type LutStore16 []uint16

// LutType is LutType in moxcms src/profile.rs. It is the type of a LUT tag.
type LutType uint8

// The types of LutType in moxcms src/profile.rs.
const (
	LutTypeLut8 LutType = iota
	LutTypeLut16
	LutTypeLutMab
	LutTypeLutMba
)

// lutTypeFrom is impl TryFrom<u32> for LutType in moxcms src/profile.rs.
func lutTypeFrom(v uint32) (LutType, error) {
	for i, s := range [...]string{"mft1", "mft2", "mAB ", "mBA "} {
		if v == sig(s) {
			return LutType(i), nil
		}
	}
	return 0, cmsError(InvalidProfile)
}

// LutWarehouse is LutWarehouse in moxcms src/profile.rs. It is a
// *LutDataType or a *LutMultidimensionalType.
//
// Port: upstream is an enum with data. The port makes it an interface.
type LutWarehouse interface {
	isLutWarehouse()
}

// LutDataType is LutDataType in moxcms src/profile.rs. It holds a tag of the
// type lut8Type or lut16Type.
type LutDataType struct {
	NumInputChannels      uint8
	NumOutputChannels     uint8
	NumClutGridPoints     uint8
	Matrix                Matrix3d
	NumInputTableEntries  uint16
	NumOutputTableEntries uint16
	InputTable            LutStore
	ClutTable             LutStore
	OutputTable           LutStore
	LutType               LutType
}

func (*LutDataType) isLutWarehouse() {}

// LutMultidimensionalType is LutMultidimensionalType in moxcms
// src/profile.rs. It holds a tag of the type lutAToBType or lutBToAType.
type LutMultidimensionalType struct {
	NumInputChannels  uint8
	NumOutputChannels uint8
	GridPoints        [16]uint8
	// Clut is nil when the tag has no CLUT.
	Clut    LutStore
	ACurves []*ToneReprCurve
	BCurves []*ToneReprCurve
	MCurves []*ToneReprCurve
	Matrix  Matrix3d
	Bias    Vector3d
}

func (*LutMultidimensionalType) isLutWarehouse() {}

// RenderingIntent is RenderingIntent in moxcms src/profile.rs.
type RenderingIntent uint32

// The intents of RenderingIntent in moxcms src/profile.rs.
const (
	RenderingIntentPerceptual           RenderingIntent = 0
	RenderingIntentRelativeColorimetric RenderingIntent = 1
	RenderingIntentSaturation           RenderingIntent = 2
	RenderingIntentAbsoluteColorimetric RenderingIntent = 3
)

// renderingIntentFrom is impl TryFrom<u32> for RenderingIntent in moxcms
// src/profile.rs. It maps a value that is not valid to Perceptual, because
// some profiles in the field hold such values.
func renderingIntentFrom(v uint32) RenderingIntent {
	if v <= 3 {
		return RenderingIntent(v)
	}
	return RenderingIntentPerceptual
}

// profileHeaderSize is size_of::<ProfileHeader>() in moxcms src/profile.rs.
// It is the size of the header and of the count of tags.
const profileHeaderSize = 132

// profileHeader is ProfileHeader in moxcms src/profile.rs. It holds the
// fields of the header that the subset uses.
type profileHeader struct {
	version          ProfileVersion
	profileClass     ProfileClass
	dataColorSpace   DataColorSpace
	pcs              DataColorSpace
	creationDateTime ColorDateTime
	renderingIntent  RenderingIntent
	illuminant       Xyz
	tagCount         uint32
}

// newProfileHeaderFromSlice is ProfileHeader::new_from_slice in moxcms
// src/profile.rs.
func newProfileHeaderFromSlice(b []byte) (profileHeader, error) {
	var h profileHeader
	if len(b) < profileHeaderSize {
		return h, cmsError(InvalidProfile)
	}
	u32 := func(i int) uint32 { return binary.BigEndian.Uint32(b[i:]) }
	var err error
	if h.version, err = profileVersionFrom(u32(8)); err != nil {
		return h, err
	}
	if h.profileClass, err = profileClassFrom(u32(12)); err != nil {
		return h, err
	}
	if h.dataColorSpace, err = dataColorSpaceFrom(u32(16)); err != nil {
		return h, err
	}
	if h.pcs, err = dataColorSpaceFrom(u32(20)); err != nil {
		return h, err
	}
	if h.creationDateTime, err = newColorDateTimeFromSlice(b[24:36]); err != nil {
		return h, err
	}
	if u32(36) != profileSignature {
		return h, cmsError(InvalidProfile)
	}
	h.renderingIntent = renderingIntentFrom(u32(64))
	h.illuminant = Xyz{
		X: s15Fixed16NumberToFloat(int32(u32(68))),
		Y: s15Fixed16NumberToFloat(int32(u32(72))),
		Z: s15Fixed16NumberToFloat(int32(u32(76))),
	}
	h.tagCount = u32(128)
	return h, nil
}

// CicpProfile is CicpProfile in moxcms src/profile.rs. It holds the code
// points of a cicp tag.
type CicpProfile struct {
	ColorPrimaries          CicpColorPrimaries
	TransferCharacteristics TransferCharacteristics
	MatrixCoefficients      MatrixCoefficients
	FullRange               bool
}

// ColorProfile is ColorProfile in moxcms src/profile.rs. It holds an ICC
// profile. A nil pointer or a nil interface is the None of an Option of
// upstream.
//
// Port: the subset leaves out the fields of text, the measurement, the
// viewing conditions, the technology and the calibration date, because no
// transform uses them.
type ColorProfile struct {
	Pcs             DataColorSpace
	ColorSpace      DataColorSpace
	ProfileClass    ProfileClass
	RenderingIntent RenderingIntent
	RedColorant     Xyzd
	GreenColorant   Xyzd
	BlueColorant    Xyzd
	WhitePoint      Xyzd
	BlackPoint      *Xyzd
	// MediaWhitePoint is the chromatically adapted media white (wtpt). It is
	// the PCS D50 for display profiles.
	MediaWhitePoint *Xyzd
	Luminance       *Xyzd
	RedTrc          *ToneReprCurve
	GreenTrc        *ToneReprCurve
	BlueTrc         *ToneReprCurve
	GrayTrc         *ToneReprCurve
	Cicp            *CicpProfile
	// ChromaticAdaptation is the full transform from the native adopted
	// white to the PCS D50 (chad).
	ChromaticAdaptation *Matrix3d
	LutAToBPerceptual   LutWarehouse
	LutAToBColorimetric LutWarehouse
	LutAToBSaturation   LutWarehouse
	LutBToAPerceptual   LutWarehouse
	LutBToAColorimetric LutWarehouse
	LutBToASaturation   LutWarehouse
	Gamut               LutWarehouse
	CreationDateTime    ColorDateTime
	versionInternal     ProfileVersion
}

// defaultColorProfile is impl Default for ColorProfile in moxcms
// src/profile.rs.
//
// Port: the zero value of a Go struct is not the default of upstream in two
// fields, so the port sets them.
func defaultColorProfile() ColorProfile {
	return ColorProfile{
		ProfileClass:    ProfileClassDisplayDevice,
		versionInternal: ProfileVersionV44,
	}
}

// ParsingOptions is ParsingOptions in moxcms src/profile.rs. It holds the
// limits for the reading of a profile.
type ParsingOptions struct {
	// MaxProfileSize is the largest profile in bytes. A profile of this size
	// or larger is an error.
	MaxProfileSize int
	// MaxAllowedClutSize is the largest number of grid points of the CLUT of
	// a lut8Type or lut16Type tag.
	MaxAllowedClutSize int
	// MaxAllowedTrcSize is the largest number of entries of a curve.
	MaxAllowedTrcSize int
}

// DefaultParsingOptions is impl Default for ParsingOptions in moxcms
// src/profile.rs.
func DefaultParsingOptions() ParsingOptions {
	return ParsingOptions{
		MaxProfileSize:     maxProfileSize,
		MaxAllowedClutSize: 10_000_000,
		MaxAllowedTrcSize:  40_000,
	}
}

// Version is ColorProfile::version in moxcms src/profile.rs.
func (p *ColorProfile) Version() ProfileVersion {
	return p.versionInternal
}

// NewFromSlice is ColorProfile::new_from_slice in moxcms src/profile.rs. It
// reads the ICC profile in b, with the default ParsingOptions.
func NewFromSlice(b []byte) (*ColorProfile, error) {
	return NewFromSliceWithOptions(b, DefaultParsingOptions())
}

// NewFromSliceWithOptions is ColorProfile::new_from_slice_with_options in
// moxcms src/profile.rs. It reads the ICC profile in b within the limits of
// options. The profile keeps no reference to b.
func NewFromSliceWithOptions(b []byte, options ParsingOptions) (*ColorProfile, error) {
	header, err := newProfileHeaderFromSlice(b)
	if err != nil {
		return nil, err
	}
	if len(b) >= options.MaxProfileSize {
		return nil, cmsError(InvalidProfile)
	}
	tagsEnd, err := safeMul(uint64(header.tagCount), tagSize)
	if err == nil {
		tagsEnd, err = safeAdd(tagsEnd, profileHeaderSize)
	}
	if err != nil {
		return nil, err
	}
	if uint64(len(b)) < tagsEnd {
		return nil, cmsError(InvalidProfile)
	}
	p := defaultColorProfile()
	p.RenderingIntent = header.renderingIntent
	p.Pcs = header.pcs
	p.ProfileClass = header.profileClass
	p.ColorSpace = header.dataColorSpace
	p.WhitePoint = header.illuminant.ToXyzd()
	p.versionInternal = header.version
	p.CreationDateTime = header.creationDateTime
	if err := p.readTags(b, b[profileHeaderSize:tagsEnd], options); err != nil {
		return nil, err
	}
	return &p, nil
}

// readTags is the loop over the tag table of
// ColorProfile::new_from_slice_with_options in moxcms src/profile.rs. It
// ignores a tag that it does not know.
//
// Port: the subset ignores the tags desc, cprt, vued, dmnd, dmdd, targ,
// meas, view, tech and calt, because no transform uses them. Upstream reads
// them, so a profile with one of these tags malformed is an error in
// upstream and not in the port.
func (p *ColorProfile) readTags(b, tags []byte, options ParsingOptions) error {
	colorSpace := p.ColorSpace
	for i := 0; i+tagSize <= len(tags); i += tagSize {
		t := tag(binary.BigEndian.Uint32(tags[i:]))
		entry := uint64(binary.BigEndian.Uint32(tags[i+4:]))
		size := uint64(binary.BigEndian.Uint32(tags[i+8:]))
		var err error
		switch t {
		case tagRedXyz:
			if colorSpace == DataColorSpaceRgb {
				p.RedColorant, err = readXyzTag(b, entry, size)
			}
		case tagGreenXyz:
			if colorSpace == DataColorSpaceRgb {
				p.GreenColorant, err = readXyzTag(b, entry, size)
			}
		case tagBlueXyz:
			if colorSpace == DataColorSpaceRgb {
				p.BlueColorant, err = readXyzTag(b, entry, size)
			}
		case tagRedToneReproduction:
			if colorSpace == DataColorSpaceRgb {
				p.RedTrc, err = readTrcTagS(b, entry, size, options)
			}
		case tagGreenToneReproduction:
			if colorSpace == DataColorSpaceRgb {
				p.GreenTrc, err = readTrcTagS(b, entry, size, options)
			}
		case tagBlueToneReproduction:
			if colorSpace == DataColorSpaceRgb {
				p.BlueTrc, err = readTrcTagS(b, entry, size, options)
			}
		case tagGreyToneReproduction:
			if colorSpace == DataColorSpaceGray {
				p.GrayTrc, err = readTrcTagS(b, entry, size, options)
			}
		case tagMediaWhitePoint:
			p.MediaWhitePoint, err = readXyzTagPtr(b, entry, size)
		case tagLuminance:
			p.Luminance, err = readXyzTagPtr(b, entry, size)
		case tagBlackPoint:
			p.BlackPoint, err = readXyzTagPtr(b, entry, size)
		case tagCodeIndependentPoints:
			// The tag can be present when the data color space is RGB, YCbCr
			// or XYZ, and the class is Input or Display. It must not be
			// present for other color spaces or classes.
			if (p.ProfileClass == ProfileClassInputDevice || p.ProfileClass == ProfileClassDisplayDevice) &&
				(p.ColorSpace == DataColorSpaceRgb || p.ColorSpace == DataColorSpaceYCbr || p.ColorSpace == DataColorSpaceXyz) {
				p.Cicp, err = readCicpTag(b, entry, size)
			}
		case tagChromaticAdaptation:
			p.ChromaticAdaptation, err = readChadTag(b, entry, size)
		case tagDeviceToPcsLutPerceptual:
			p.LutAToBPerceptual, err = readLutTag(b, entry, size, options)
		case tagDeviceToPcsLutColorimetric:
			p.LutAToBColorimetric, err = readLutTag(b, entry, size, options)
		case tagDeviceToPcsLutSaturation:
			p.LutAToBSaturation, err = readLutTag(b, entry, size, options)
		case tagPcsToDeviceLutPerceptual:
			p.LutBToAPerceptual, err = readLutTag(b, entry, size, options)
		case tagPcsToDeviceLutColorimetric:
			p.LutBToAColorimetric, err = readLutTag(b, entry, size, options)
		case tagPcsToDeviceLutSaturation:
			p.LutBToASaturation, err = readLutTag(b, entry, size, options)
		case tagGamut:
			p.Gamut, err = readLutTag(b, entry, size, options)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// colorantMatrix is ColorProfile::colorant_matrix in moxcms src/profile.rs.
// It holds the colorants in its columns.
func (p *ColorProfile) colorantMatrix() Matrix3d {
	return Matrix3d{V: [3][3]float64{
		{p.RedColorant.X, p.GreenColorant.X, p.BlueColorant.X},
		{p.RedColorant.Y, p.GreenColorant.Y, p.BlueColorant.Y},
		{p.RedColorant.Z, p.GreenColorant.Z, p.BlueColorant.Z},
	}}
}

// colorantsMatrix is ColorProfile::colorants_matrix in moxcms
// src/profile.rs. It computes the colorants of the primaries for a white
// point, adapted to the PCS D50.
func colorantsMatrix(whitePoint XyY, primaries ColorPrimaries) Matrix3d {
	red := primaries.Red.ToXyzd()
	green := primaries.Green.ToXyzd()
	blue := primaries.Blue.ToXyzd()
	xyzMatrix := Matrix3d{V: [3][3]float64{
		{red.X, green.X, blue.X},
		{red.Y, green.Y, blue.Y},
		{red.Z, green.Z, blue.Z},
	}}
	colorants := rgbToXyzD(xyzMatrix, whitePoint.ToXyzd())
	return adaptToD50D(colorants, whitePoint)
}

// updateChromaticAdaptation is ColorProfile::update_chromatic_adaptation in
// moxcms src/profile.rs.
func (p *ColorProfile) updateChromaticAdaptation(whitePoint XyY) {
	p.WhitePoint = pcsD50
	mwp := pcsD50
	p.MediaWhitePoint = &mwp
	chad := adaptToD50D(Matrix3dIdentity, whitePoint)
	p.ChromaticAdaptation = &chad
}

// updateColorants is ColorProfile::update_colorants in moxcms
// src/profile.rs. It reads the colorants from the columns of colorants.
func (p *ColorProfile) updateColorants(colorants Matrix3d) {
	p.RedColorant = Xyzd{X: colorants.V[0][0], Y: colorants.V[1][0], Z: colorants.V[2][0]}
	p.GreenColorant = Xyzd{X: colorants.V[0][1], Y: colorants.V[1][1], Z: colorants.V[2][1]}
	p.BlueColorant = Xyzd{X: colorants.V[0][2], Y: colorants.V[1][2], Z: colorants.V[2][2]}
}

// updateRgbColorimetry is ColorProfile::update_rgb_colorimetry in moxcms
// src/profile.rs. It sets the colorants of the primaries for a white point,
// adapted to the PCS D50, and it removes the cicp tag.
func (p *ColorProfile) updateRgbColorimetry(whitePoint XyY, primaries ColorPrimaries) {
	p.Cicp = nil
	red := primaries.Red.ToXyzd()
	green := primaries.Green.ToXyzd()
	blue := primaries.Blue.ToXyzd()
	p.updateRgbColorimetryTriplet(whitePoint, red, green, blue)
}

// updateRgbColorimetryTriplet is ColorProfile::update_rgb_colorimetry_triplet
// in moxcms src/profile.rs. It sets the colorants of three colors in XYZ
// for a white point, adapted to the PCS D50, and it removes the cicp tag.
func (p *ColorProfile) updateRgbColorimetryTriplet(whitePoint XyY, red, green, blue Xyzd) {
	p.Cicp = nil
	p.updateChromaticAdaptation(whitePoint)
	xyzMatrix := Matrix3d{V: [3][3]float64{
		{red.X, green.X, blue.X},
		{red.Y, green.Y, blue.Y},
		{red.Z, green.Z, blue.Z},
	}}
	colorants := rgbToXyzD(xyzMatrix, whitePoint.ToXyzd())
	colorants = adaptToD50D(colorants, whitePoint)
	p.updateColorants(colorants)
}

// updateRgbColorimetryFromCicp is
// ColorProfile::update_rgb_colorimetry_from_cicp in moxcms src/profile.rs.
// It sets the colorants, the tone curves and the cicp tag of cicp. It leaves
// p as it is when cicp has no primaries or no transfer curve.
//
// Port: upstream returns false in each case, also after it set the
// profile. The port keeps this.
func (p *ColorProfile) updateRgbColorimetryFromCicp(cicp CicpProfile) bool {
	if !cicp.ColorPrimaries.hasChromaticity() || !cicp.TransferCharacteristics.hasTransferCurve() {
		return false
	}
	primaries, err := colorPrimariesFrom(cicp.ColorPrimaries)
	if err != nil {
		return false
	}
	whitePoint, err := cicp.ColorPrimaries.whitePoint()
	if err != nil {
		return false
	}
	p.updateRgbColorimetry(whitePoint.ToXyYb(), primaries)
	c := cicp
	p.Cicp = &c

	trc, err := toneReprCurveFrom(cicp.TransferCharacteristics)
	if err != nil {
		return false
	}
	green, blue := *trc, *trc
	p.GreenTrc = &green
	p.BlueTrc = &blue
	p.RedTrc = trc
	return false
}

// rgbToXyzD is ColorProfile::rgb_to_xyz_d in moxcms src/profile.rs. It
// scales the columns of xyzMatrix so that RGB white maps to wp.
func rgbToXyzD(xyzMatrix Matrix3d, wp Xyzd) Matrix3d {
	xyzInverse := xyzMatrix.Inverse()
	s := xyzInverse.MulVector(wp.ToVectorD())
	v := xyzMatrix.mulRowVector(0, s)
	v = v.mulRowVector(1, s)
	return v.mulRowVector(2, s)
}

// RgbToXyzMatrix is ColorProfile::rgb_to_xyz_matrix in moxcms
// src/profile.rs. It returns the matrix from linear RGB to XYZ. The
// colorants are its columns, as ICC.1:2022-05 section F.3 says.
func (p *ColorProfile) RgbToXyzMatrix() Matrix3d {
	return p.colorantMatrix()
}

// TransformMatrix is ColorProfile::transform_matrix in moxcms
// src/profile.rs. It returns the matrix from the linear RGB of p to the
// linear RGB of dest.
func (p *ColorProfile) TransformMatrix(dest *ColorProfile) Matrix3d {
	source := p.RgbToXyzMatrix()
	dst := dest.RgbToXyzMatrix()
	destInverse := dst.Inverse()
	return destInverse.MatMul(source)
}

// hasDeviceToPcsLut is ColorProfile::has_device_to_pcs_lut in moxcms
// src/profile.rs.
func (p *ColorProfile) hasDeviceToPcsLut() bool {
	return p.LutAToBPerceptual != nil || p.LutAToBSaturation != nil || p.LutAToBColorimetric != nil
}

// hasPcsToDeviceLut is ColorProfile::has_pcs_to_device_lut in moxcms
// src/profile.rs.
func (p *ColorProfile) hasPcsToDeviceLut() bool {
	return p.LutBToAPerceptual != nil || p.LutBToASaturation != nil || p.LutBToAColorimetric != nil
}
