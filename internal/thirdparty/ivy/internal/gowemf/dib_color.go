// Copied from github.com/mgilbir/gowemf, file dib_color.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

func dibColorSpace(info []byte, header uint64) (ColorSpace, uint64, uint64, error) {
	s := ColorSpace{Type: ColorSRGB}
	if header < 108 {
		return s, 0, 0, nil
	}
	s.Type = u32(info[56:])
	for i := range s.Endpoints {
		b := info[60+i*12:]
		s.Endpoints[i] = XYZ{float64(int32(u32(b))) / (1 << 30), float64(int32(u32(b[4:]))) / (1 << 30), float64(int32(u32(b[8:]))) / (1 << 30)}
	}
	for i := range s.Gamma {
		s.Gamma[i] = float64(u32(info[96+i*4:])) / 65536
	}
	if header == 124 {
		s.Intent = u32(info[108:])
	}
	switch s.Type {
	case ColorSRGB, ColorWindows, ColorCalibratedRGB:
		return s, 0, 0, nil
	case ColorProfileEmbedded, ColorProfileLinked:
		if header != 124 {
			return s, 0, 0, malformed(56, "profile requires BITMAPV5HEADER")
		}
	default:
		return s, 0, 0, failure(56, "DIB color space", ErrUnsupported)
	}
	off, n := uint64(u32(info[112:])), uint64(u32(info[116:]))
	if n == 0 || off < header || off > uint64(len(info)) || n > uint64(len(info))-off {
		return s, 0, 0, malformed(112, "DIB profile span")
	}
	profile := info[int(off):int(off+n):int(off+n)]
	if s.Type == ColorProfileEmbedded {
		s.Profile = profile
	} else {
		s.Name = profile
		if profile[len(profile)-1] != 0 {
			return s, 0, 0, malformed(int(off), "unterminated DIB profile name")
		}
	}
	return s, off, off + n, nil
}

func profileOverlapsPixels(start, end uint64, pixelStart, pixelBytes uint64) bool {
	return end != 0 && start < pixelStart+pixelBytes && end > pixelStart
}
