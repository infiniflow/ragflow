// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/conversions/xyz_lab.rs.

package moxcms

// stageLabToXyz is StageLabToXyz in moxcms src/conversions/xyz_lab.rs. It
// converts each triplet from the Lab encoding of the PCS to the XYZ
// encoding of the PCS.
type stageLabToXyz struct{}

// Transform is InPlaceStage::transform for StageLabToXyz.
func (stageLabToXyz) Transform(dst []float32) error {
	for i := 0; i+3 <= len(dst); i += 3 {
		xyz := NewLab(dst[i], dst[i+1], dst[i+2]).ToPcsXyz()
		dst[i], dst[i+1], dst[i+2] = xyz.X, xyz.Y, xyz.Z
	}
	return nil
}

// stageXyzToLab is StageXyzToLab in moxcms src/conversions/xyz_lab.rs. It
// converts each triplet from the XYZ encoding of the PCS to the Lab
// encoding of the PCS.
type stageXyzToLab struct{}

// Transform is InPlaceStage::transform for StageXyzToLab.
func (stageXyzToLab) Transform(dst []float32) error {
	for i := 0; i+3 <= len(dst); i += 3 {
		lab := FromPcsXyz(NewXyz(dst[i], dst[i+1], dst[i+2]))
		dst[i], dst[i+1], dst[i+2] = lab.L, lab.A, lab.B
	}
	return nil
}
