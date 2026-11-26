// Copied from github.com/mgilbir/gowemf, file wmf_objects.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

type FloodFill struct {
	Point       Point
	Color, Mode uint32
}
type RegionPaint struct {
	Region, Brush uint32
	Frame         Point
}
type WMFRegion struct {
	DeclaredBytes, MaxScan int32
	Bounds                 Rect
	Scans                  []WMFScan
}

// WMF scan endpoints are unsigned WORDs in MS-WMF 2.2.2.21, unlike the signed
// bounding rectangle. Endpoints contains left/right coordinate pairs.
type WMFScan struct {
	Top, Bottom uint16
	Endpoints   Integers
}

func (c *cursor) wmfRegion() WMFRegion {
	start := c.pos
	c.word()
	if c.word() != 6 {
		c.bad("WMF region object type")
	}
	c.dword()
	v := WMFRegion{DeclaredBytes: c.short()}
	count := c.short()
	v.MaxScan = c.short()
	v.Bounds = Rect{c.short(), c.short(), c.short(), c.short()}
	if count < 0 || v.DeclaredBytes < 22 || v.MaxScan < 0 {
		c.bad("WMF region size/count")
	}
	if c.err != nil {
		return v
	}
	n := uint64(count)
	if n > c.limits.MaxElements {
		c.err = failure(c.base+c.pos, "WMF scan count", ErrLimit)
		return v
	}
	if n > uint64(len(c.b)-c.pos)/8 {
		c.bad("WMF region scans")
		return v
	}
	if !c.allocation(n, 64) {
		return v
	}
	v.Scans = make([]WMFScan, int(n))
	var max uint16
	for i := range v.Scans {
		count := c.word()
		if count&1 != 0 {
			c.bad("unpaired WMF scan endpoints")
		}
		top, bottom := c.word(), c.word()
		v.Scans[i] = WMFScan{top, bottom, c.ints(uint64(count), 2)}
		if c.word() != count {
			c.bad("WMF scan count suffix")
		}
		if count > max {
			max = count
		}
		if c.err != nil {
			return v
		}
	}
	if int32(max) != v.MaxScan || c.pos-start != int(v.DeclaredBytes) {
		c.bad("WMF region size or maximum scan")
	}
	return v
}

// Bitmap16 is a checked device-dependent bitmap view. Pixel colors depend on
// the original device/selected palette; this type does not guess that mapping.
type Bitmap16 struct {
	Type, Width, Height, WidthBytes int32
	Planes, BitCount                byte
	Bits                            []byte
}
type BitmapPatternBrush struct{ Bitmap Bitmap16 }
type Bitmap16Transfer struct {
	RasterOperation                                  uint32
	Source, SourceSize, Destination, DestinationSize Point
	DeviceSource                                     bool
	Bitmap                                           *Bitmap16
}

func (c *cursor) bitmap16(pattern bool) Bitmap16 {
	v := Bitmap16{Type: c.short(), Width: c.short(), Height: c.short(), WidthBytes: c.short()}
	format := c.take(2)
	if format == nil {
		return v
	}
	v.Planes, v.BitCount = format[0], format[1]
	if v.Width <= 0 || v.Height <= 0 || v.WidthBytes <= 0 || v.Planes != 1 || v.BitCount == 0 {
		c.bad("Bitmap16 dimensions/format")
		return v
	}
	stride := ((uint64(v.Width)*uint64(v.BitCount) + 15) / 16) * 2
	if stride != uint64(v.WidthBytes) {
		c.bad("Bitmap16 row size")
		return v
	}
	if pattern {
		c.take(22)
	} // ignored pointer (4) and reserved bytes (18)
	v.Bits = c.take(stride * uint64(v.Height))
	return v
}
