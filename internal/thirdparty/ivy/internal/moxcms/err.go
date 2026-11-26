// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/err.rs.

package moxcms

import (
	"fmt"

	"ragflow/internal/thirdparty/ivy"
)

// MalformedSize is MalformedSize in moxcms src/err.rs. It holds the size of
// a table and the size that the table must have.
type MalformedSize struct {
	Size     int
	Expected int
}

// CmsErrorKind is the variant of CmsError in moxcms src/err.rs.
type CmsErrorKind uint8

// The variants of CmsError in moxcms src/err.rs.
const (
	LaneSizeMismatch CmsErrorKind = iota
	LaneMultipleOfChannels
	InvalidProfile
	InvalidTrcCurve
	InvalidCicp
	CurveLutIsTooLarge
	ParametricCurveZeroDivision
	InvalidRenderingIntent
	DivisionByZero
	UnsupportedColorPrimaries
	UnsupportedTrc
	InvalidLayout
	UnsupportedProfileConnection
	BuildTransferFunction
	UnsupportedChannelConfiguration
	UnknownTag
	UnknownTagTypeDefinition
	UnsupportedLutRenderingIntent
	InvalidAtoBLut
	OverflowingError
	LUTTablesInvalidKind
	MalformedClut
	MalformedCurveLutTable
	InvalidInksCountForProfile
	MalformedTrcCurve
	OutOfMemory
	IncorrectlyFormedLut
)

// CmsError is CmsError in moxcms src/err.rs. Kind is the variant. The other
// fields hold the data of the variants that have data.
//
// Port: upstream is an enum with data. Go has no such enum, so CmsError is a
// struct. A CmsError wraps a sentinel error of ivy, so that a caller can test
// it with errors.Is.
type CmsError struct {
	// Kind is the variant.
	Kind CmsErrorKind
	// Size is the data of MalformedClut and MalformedCurveLutTable.
	Size MalformedSize
	// Value is the data of UnsupportedColorPrimaries, UnsupportedTrc,
	// UnknownTag, UnknownTagTypeDefinition and OutOfMemory.
	Value uint64
	// Intent is the data of UnsupportedLutRenderingIntent.
	Intent RenderingIntent
	// Message is the data of MalformedTrcCurve and IncorrectlyFormedLut.
	Message string
}

// cmsError returns a CmsError of a variant without data.
func cmsError(kind CmsErrorKind) error {
	return &CmsError{Kind: kind}
}

// Error is impl Display for CmsError in moxcms src/err.rs.
//
// Port: the messages are plain English in lower case, as Go writes them.
func (e *CmsError) Error() string {
	var s string
	switch e.Kind {
	case LaneSizeMismatch:
		s = "the source and the destination hold different numbers of pixels"
	case LaneMultipleOfChannels:
		s = "the length of a buffer is not a multiple of its channel count"
	case InvalidProfile:
		s = "invalid ICC profile"
	case InvalidCicp:
		s = "invalid CICP tag in the ICC profile"
	case InvalidTrcCurve:
		s = "invalid tone curve"
	case CurveLutIsTooLarge:
		s = "the tone curve is too large"
	case ParametricCurveZeroDivision:
		s = "the parametric curve divides by zero"
	case InvalidRenderingIntent:
		s = "invalid rendering intent"
	case DivisionByZero:
		s = "division by zero"
	case UnsupportedColorPrimaries:
		s = fmt.Sprintf("unsupported color primaries %d", e.Value)
	case UnsupportedTrc:
		s = fmt.Sprintf("unsupported tone curve %d", e.Value)
	case InvalidLayout:
		s = "invalid layout"
	case UnsupportedProfileConnection:
		s = "unsupported profile connection"
	case BuildTransferFunction:
		s = "cannot build the transfer function"
	case UnsupportedChannelConfiguration:
		s = "unsupported channel configuration"
	case UnknownTag:
		s = fmt.Sprintf("unknown tag %d", e.Value)
	case UnknownTagTypeDefinition:
		s = fmt.Sprintf("unknown tag type %d", e.Value)
	case UnsupportedLutRenderingIntent:
		s = fmt.Sprintf("no LUT for the rendering intent %d", e.Intent)
	case InvalidAtoBLut:
		s = "invalid A to B LUT"
	case OverflowingError:
		s = "an integer overflowed"
	case LUTTablesInvalidKind:
		s = "the LUT tables have different kinds"
	case MalformedClut:
		s = fmt.Sprintf("invalid CLUT size %d, want %d", e.Size.Size, e.Size.Expected)
	case MalformedCurveLutTable:
		s = fmt.Sprintf("invalid curve LUT size %d, want %d", e.Size.Size, e.Size.Expected)
	case InvalidInksCountForProfile:
		s = "invalid ink count for the profile"
	case MalformedTrcCurve, IncorrectlyFormedLut:
		s = e.Message
	case OutOfMemory:
		s = fmt.Sprintf("not enough memory for %d bytes", e.Value)
	default:
		s = fmt.Sprintf("unknown error %d", e.Kind)
	}
	return "moxcms: " + s
}

// Unwrap returns the sentinel error of ivy that e wraps. A profile that
// breaks the ICC format gives ivy.ErrFormat. A size past a limit gives
// ivy.ErrLimit. A transform that the subset cannot build, or a buffer that
// does not fit a transform, gives ivy.ErrUnsupported.
//
// Port: upstream has no such method.
func (e *CmsError) Unwrap() error {
	switch e.Kind {
	case LaneSizeMismatch, LaneMultipleOfChannels, InvalidLayout,
		UnsupportedColorPrimaries, UnsupportedTrc, UnsupportedProfileConnection,
		UnsupportedChannelConfiguration, UnsupportedLutRenderingIntent,
		InvalidRenderingIntent, InvalidInksCountForProfile:
		return ivy.ErrUnsupported
	case CurveLutIsTooLarge, OverflowingError, OutOfMemory:
		return ivy.ErrLimit
	default:
		return ivy.ErrFormat
	}
}
