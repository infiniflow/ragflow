// Copyright (c) Radzivon Bartoshyk. All rights reserved.
//
// Use of this source code is governed by the BSD-3-Clause or Apache-2.0
// license that can be found in the LICENSE.md and LICENSE-APACHE.md files.

// This file ports moxcms src/nd_array.rs.

package moxcms

// Port: upstream returns an empty Cube or Hypercube for an empty array or a
// grid with a size of zero, and it panics when it interpolates in one. It
// also accepts an array whose last node is in the array only in part. The
// port returns MalformedClut for both, so that an interpolation never reads
// past the array.

// lerpVec3 is lerp in moxcms src/nd_array.rs, for Vector3f. It returns
// a + (b - a)*t, as moxcms rounds it.
func lerpVec3(a, b, t Vector3f) Vector3f {
	return a.negMla(a, t).mla(b, t)
}

// Hypercube is Hypercube in moxcms src/nd_array.rs. It is a 4D CLUT.
type Hypercube struct {
	array    []float32
	xStride  uint32
	yStride  uint32
	zStride  uint32
	gridSize [4]uint8
}

// checkLastIndex returns MalformedClut when the node at lastIndex, with
// channels values, is not in an array of length n.
func checkLastIndex(lastIndex, channels uint64, n int) error {
	end, err := safeAdd(lastIndex, channels)
	if err != nil {
		return err
	}
	if lastIndex >= uint64(n) || end > uint64(n) {
		return &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: n, Expected: int(min(lastIndex, 1<<31))}}
	}
	return nil
}

// NewHypercube is Hypercube::new in moxcms src/nd_array.rs. It returns the
// 4D CLUT in array, with gridSize nodes in each dimension and channels
// values in each node.
func NewHypercube(array []float32, gridSize, channels int) (*Hypercube, error) {
	if len(array) == 0 || gridSize <= 0 || gridSize > 255 {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(array)}}
	}
	g := uint8(gridSize)
	return NewHypercubeGrid(array, [4]uint8{g, g, g, g}, channels)
}

// NewHypercubeGrid is Hypercube::new_hypercube in moxcms src/nd_array.rs. It
// returns the 4D CLUT in array, with the number of nodes in each dimension
// in gridSize.
func NewHypercubeGrid(array []float32, gridSize [4]uint8, channels int) (*Hypercube, error) {
	if len(array) == 0 || gridSize[0] == 0 || gridSize[1] == 0 || gridSize[2] == 0 || gridSize[3] == 0 {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(array)}}
	}
	// Port: upstream takes the strides from the sizes of the first three
	// dimensions, not the last three. The port keeps this.
	zStride := uint64(gridSize[2])
	yStride := zStride * uint64(gridSize[1])
	xStride := yStride * uint64(gridSize[0])
	lastIndex := (uint64(gridSize[0])-1)*xStride +
		(uint64(gridSize[1])-1)*yStride +
		(uint64(gridSize[2])-1)*zStride +
		uint64(gridSize[3]) - 1
	lastIndex *= uint64(channels)
	if err := checkLastIndex(lastIndex, uint64(channels), len(array)); err != nil {
		return nil, err
	}
	return &Hypercube{
		array:    array,
		xStride:  uint32(xStride),
		yStride:  uint32(yStride),
		zStride:  uint32(zStride),
		gridSize: gridSize,
	}, nil
}

// fetch3 is Fetch4Vec3::fetch in moxcms src/nd_array.rs.
func (h *Hypercube) fetch3(x, y, z, w int32) Vector3f {
	start := int(uint32(x)*h.xStride+uint32(y)*h.yStride+uint32(z)*h.zStride+uint32(w)) * 3
	k := h.array[start : start+3]
	return Vector3f{V: [3]float32{k[0], k[1], k[2]}}
}

// QuadlinearVec3 is Hypercube::quadlinear_vec3 in moxcms src/nd_array.rs. It
// interpolates the CLUT at a point with each coordinate from 0 to 1.
func (h *Hypercube) QuadlinearVec3(linX, linY, linZ, linW float32) Vector3f {
	linX = maxF32(linX, 0)
	linY = maxF32(linY, 0)
	linZ = maxF32(linZ, 0)
	linW = maxF32(linW, 0)

	scaleX := float32(int32(h.gridSize[0]) - 1)
	scaleY := float32(int32(h.gridSize[1]) - 1)
	scaleZ := float32(int32(h.gridSize[2]) - 1)
	scaleW := float32(int32(h.gridSize[3]) - 1)

	x := f32ToI32(minF32(floorF32(linX*scaleX), scaleX))
	y := f32ToI32(minF32(floorF32(linY*scaleY), scaleY))
	z := f32ToI32(minF32(floorF32(linZ*scaleZ), scaleZ))
	w := f32ToI32(minF32(floorF32(linW*scaleW), scaleW))

	xn := f32ToI32(minF32(ceilF32(linX*scaleX), scaleX))
	yn := f32ToI32(minF32(ceilF32(linY*scaleY), scaleY))
	zn := f32ToI32(minF32(ceilF32(linZ*scaleZ), scaleZ))
	wn := f32ToI32(minF32(ceilF32(linW*scaleW), scaleW))

	xd := vector3fFrom(float32(linX*scaleX) - float32(x))
	yd := vector3fFrom(float32(linY*scaleY) - float32(y))
	zd := vector3fFrom(float32(linZ*scaleZ) - float32(z))
	wd := vector3fFrom(float32(linW*scaleW) - float32(w))

	rx1 := lerpVec3(h.fetch3(x, y, z, w), h.fetch3(xn, y, z, w), xd)
	rx2 := lerpVec3(h.fetch3(x, yn, z, w), h.fetch3(xn, yn, z, w), xd)
	ry1 := lerpVec3(rx1, rx2, yd)
	rx3 := lerpVec3(h.fetch3(x, y, zn, w), h.fetch3(xn, y, zn, w), xd)
	rx4 := lerpVec3(h.fetch3(x, yn, zn, w), h.fetch3(xn, yn, zn, w), xd)
	ry2 := lerpVec3(rx3, rx4, yd)
	rz1 := lerpVec3(ry1, ry2, zd)

	rx1 = lerpVec3(h.fetch3(x, y, z, wn), h.fetch3(xn, y, z, wn), xd)
	rx2 = lerpVec3(h.fetch3(x, yn, z, wn), h.fetch3(xn, yn, z, wn), xd)
	ry1 = lerpVec3(rx1, rx2, yd)
	rx3 = lerpVec3(h.fetch3(x, y, zn, wn), h.fetch3(xn, y, zn, wn), xd)
	rx4 = lerpVec3(h.fetch3(x, yn, zn, wn), h.fetch3(xn, yn, zn, wn), xd)
	ry2 = lerpVec3(rx3, rx4, yd)
	rz2 := lerpVec3(ry1, ry2, zd)
	return lerpVec3(rz1, rz2, wd)
}

// Cube is Cube in moxcms src/nd_array.rs. It is a 3D CLUT.
type Cube struct {
	array    []float32
	xStride  uint32
	yStride  uint32
	gridSize [3]uint8
}

// NewCube is Cube::new in moxcms src/nd_array.rs. It returns the 3D CLUT in
// array, with gridSize nodes in each dimension and channels values in each
// node.
func NewCube(array []float32, gridSize, channels int) (*Cube, error) {
	if len(array) == 0 || gridSize <= 0 || gridSize > 255 {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(array)}}
	}
	g := uint8(gridSize)
	return NewCubeGrid(array, [3]uint8{g, g, g}, channels)
}

// NewCubeGrid is Cube::new_cube in moxcms src/nd_array.rs. It returns the 3D
// CLUT in array, with the number of nodes in each dimension in gridSize.
func NewCubeGrid(array []float32, gridSize [3]uint8, channels int) (*Cube, error) {
	if len(array) == 0 || gridSize[0] == 0 || gridSize[1] == 0 || gridSize[2] == 0 {
		return nil, &CmsError{Kind: MalformedClut, Size: MalformedSize{Size: len(array)}}
	}
	yStride := uint64(gridSize[2])
	xStride := yStride * uint64(gridSize[1])
	lastIndex := (uint64(gridSize[0])-1)*xStride +
		(uint64(gridSize[1])-1)*yStride +
		uint64(gridSize[2]) - 1
	lastIndex *= uint64(channels)
	if err := checkLastIndex(lastIndex, uint64(channels), len(array)); err != nil {
		return nil, err
	}
	return &Cube{
		array:    array,
		xStride:  uint32(xStride),
		yStride:  uint32(yStride),
		gridSize: gridSize,
	}, nil
}

// fetch3 is ArrayFetchVector3f::fetch in moxcms src/nd_array.rs.
func (c *Cube) fetch3(x, y, z int32) Vector3f {
	start := int(uint32(x)*c.xStride+uint32(y)*c.yStride+uint32(z)) * 3
	k := c.array[start : start+3]
	return Vector3f{V: [3]float32{k[0], k[1], k[2]}}
}

// TrilinearVec3 is Cube::trilinear_vec3 in moxcms src/nd_array.rs. It
// interpolates the CLUT at a point with each coordinate from 0 to 1.
func (c *Cube) TrilinearVec3(linX, linY, linZ float32) Vector3f {
	linX = maxF32(linX, 0)
	linY = maxF32(linY, 0)
	linZ = maxF32(linZ, 0)

	scaleX := float32(int32(c.gridSize[0]) - 1)
	scaleY := float32(int32(c.gridSize[1]) - 1)
	scaleZ := float32(int32(c.gridSize[2]) - 1)

	x := f32ToI32(minF32(floorF32(linX*scaleX), scaleX))
	y := f32ToI32(minF32(floorF32(linY*scaleY), scaleY))
	z := f32ToI32(minF32(floorF32(linZ*scaleZ), scaleZ))

	xn := f32ToI32(minF32(ceilF32(linX*scaleX), scaleX))
	yn := f32ToI32(minF32(ceilF32(linY*scaleY), scaleY))
	zn := f32ToI32(minF32(ceilF32(linZ*scaleZ), scaleZ))

	xd := vector3fFrom(float32(linX*scaleX) - float32(x))
	yd := vector3fFrom(float32(linY*scaleY) - float32(y))
	zd := vector3fFrom(float32(linZ*scaleZ) - float32(z))

	c000 := c.fetch3(x, y, z)
	c100 := c.fetch3(xn, y, z)
	c010 := c.fetch3(x, yn, z)
	c110 := c.fetch3(xn, yn, z)
	c001 := c.fetch3(x, y, zn)
	c101 := c.fetch3(xn, y, zn)
	c011 := c.fetch3(x, yn, zn)
	c111 := c.fetch3(xn, yn, zn)

	c00 := c000.negMla(c000, xd).mla(c100, xd)
	c10 := c010.negMla(c010, xd).mla(c110, xd)
	c01 := c001.negMla(c001, xd).mla(c101, xd)
	c11 := c011.negMla(c011, xd).mla(c111, xd)

	c0 := c00.negMla(c00, yd).mla(c10, yd)
	c1 := c01.negMla(c01, yd).mla(c11, yd)

	return c0.negMla(c0, zd).mla(c1, zd)
}
