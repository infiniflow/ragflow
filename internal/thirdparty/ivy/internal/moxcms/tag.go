// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/tag.rs.

package moxcms

// tagSize is TAG_SIZE in moxcms src/tag.rs. It is the size of an entry of
// the tag table, and the size of the smallest tag.
const tagSize = 12

// sig returns the big-endian value of a signature of four bytes.
//
// Port: upstream writes u32::from_ne_bytes(*b"....").to_be().
func sig(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

// tag is Tag in moxcms src/tag.rs. It is the signature of a tag.
//
// Port: the subset reads only the tags that its transforms use. It skips
// the tags of text, the measurement, the viewing conditions, the technology
// and the calibration date, which upstream reads. See readTags.
type tag uint32

// The tags of the subset, from Tag in moxcms src/tag.rs.
var (
	tagRedXyz                     = tag(sig("rXYZ"))
	tagGreenXyz                   = tag(sig("gXYZ"))
	tagBlueXyz                    = tag(sig("bXYZ"))
	tagRedToneReproduction        = tag(sig("rTRC"))
	tagGreenToneReproduction      = tag(sig("gTRC"))
	tagBlueToneReproduction       = tag(sig("bTRC"))
	tagGreyToneReproduction       = tag(sig("kTRC"))
	tagMediaWhitePoint            = tag(sig("wtpt"))
	tagCodeIndependentPoints      = tag(sig("cicp"))
	tagChromaticAdaptation        = tag(sig("chad"))
	tagBlackPoint                 = tag(sig("bkpt"))
	tagDeviceToPcsLutPerceptual   = tag(sig("A2B0"))
	tagDeviceToPcsLutColorimetric = tag(sig("A2B1"))
	tagDeviceToPcsLutSaturation   = tag(sig("A2B2"))
	tagPcsToDeviceLutPerceptual   = tag(sig("B2A0"))
	tagPcsToDeviceLutColorimetric = tag(sig("B2A1"))
	tagPcsToDeviceLutSaturation   = tag(sig("B2A2"))
	tagGamut                      = tag(sig("gamt"))
	tagLuminance                  = tag(sig("lumi"))
)

// tagTypeDefinition is TagTypeDefinition in moxcms src/tag.rs. It is the
// signature of the type of a tag.
type tagTypeDefinition uint32

// The tag types of the subset, from TagTypeDefinition in moxcms src/tag.rs.
var (
	tagTypeMabLut              = tagTypeDefinition(sig("mAB "))
	tagTypeMbaLut              = tagTypeDefinition(sig("mBA "))
	tagTypeParametricToneCurve = tagTypeDefinition(sig("para"))
	tagTypeLutToneCurve        = tagTypeDefinition(sig("curv"))
	tagTypeXyz                 = tagTypeDefinition(sig("XYZ "))
	tagTypeCicp                = tagTypeDefinition(sig("cicp"))
	tagTypeS15Fixed16Array     = tagTypeDefinition(sig("sf32"))
)
