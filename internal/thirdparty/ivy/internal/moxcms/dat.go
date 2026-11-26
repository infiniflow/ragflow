// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/dat.rs.

package moxcms

import "encoding/binary"

// ColorDateTime is ColorDateTime in moxcms src/dat.rs. It holds a
// dateTimeNumber of ICC.1.
type ColorDateTime struct {
	Year          uint16
	Month         uint16
	DayOfTheMonth uint16
	Hours         uint16
	Minutes       uint16
	Seconds       uint16
}

// newColorDateTimeFromSlice is ColorDateTime::new_from_slice in moxcms
// src/dat.rs. It reads the 12 bytes of a dateTimeNumber.
func newColorDateTimeFromSlice(b []byte) (ColorDateTime, error) {
	if len(b) != 12 {
		return ColorDateTime{}, cmsError(InvalidProfile)
	}
	return ColorDateTime{
		Year:          binary.BigEndian.Uint16(b[0:]),
		Month:         binary.BigEndian.Uint16(b[2:]),
		DayOfTheMonth: binary.BigEndian.Uint16(b[4:]),
		Hours:         binary.BigEndian.Uint16(b[6:]),
		Minutes:       binary.BigEndian.Uint16(b[8:]),
		Seconds:       binary.BigEndian.Uint16(b[10:]),
	}, nil
}
