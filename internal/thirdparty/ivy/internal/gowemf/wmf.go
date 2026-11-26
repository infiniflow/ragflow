// Copied from github.com/mgilbir/gowemf, file wmf.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// MS-WMF 2.3.2.2 (META_HEADER), 2.3.2.3 (META_PLACEABLE).
func (p *parser) wmf(placeable bool) error {
	b, start := p.data, 0
	p.header.Format = WMF
	if placeable {
		if len(b) < 22 {
			return malformed(0, "placeable header")
		}
		var checksum uint16
		for i := 0; i < 20; i += 2 {
			checksum ^= u16(b[i:])
		}
		if checksum != u16(b[20:]) {
			return malformed(20, "placeable checksum")
		}
		if u32(b[16:]) != 0 {
			return malformed(16, "placeable reserved")
		}
		if u16(b[14:]) == 0 {
			return malformed(14, "placeable units per inch")
		}
		p.header.Placeable = &PlaceableHeader{
			Bounds:       Rect{int32(int16(u16(b[6:]))), int32(int16(u16(b[8:]))), int32(int16(u16(b[10:]))), int32(int16(u16(b[12:])))},
			UnitsPerInch: u16(b[14:]),
		}
		start = 22
	}
	if len(b)-start < 18 {
		return malformed(start, "WMF header")
	}
	h := b[start:]
	if (u16(h) != 1 && u16(h) != 2) || u16(h[2:]) != 9 {
		return malformed(start, "WMF header type or size")
	}
	if u16(h[4:]) != 0x100 && u16(h[4:]) != 0x300 {
		return malformed(start+4, "WMF version")
	}
	if placeable && u16(h) == 2 && u16(b[4:]) != 0 {
		return malformed(4, "disk metafile handle")
	}
	w := &WMFHeader{Type: u16(h), Version: u16(h[4:]), SizeWords: u32(h[6:]), Objects: u16(h[10:]), MaxRecordWords: u32(h[12:])}
	p.header.WMF = w
	// ivy: Windows plays a file with bytes after its last record, so a size
	// smaller than the data is not an error. A larger size means that the
	// file is cut short, except for 22 bytes more in a file without a
	// placeable header: a WMF picture of the store of Office Drawing, such
	// as those of poi/xls/landscape-a4-drawing.xls, counts the placeable
	// header that the store removed.
	if size := uint64(w.SizeWords) * 2; size > uint64(len(h)) && (placeable || size != uint64(len(h))+22) {
		return malformed(start+6, "WMF file size")
	}
	for off := start + 18; off < len(b); {
		if len(b)-off < 6 {
			return malformed(off, "WMF record header")
		}
		words, typ := u32(b[off:]), u16(b[off+4:])
		n := uint64(words) * 2
		if err := p.record(off, n, len(b)-off, 6, 2); err != nil {
			return err
		}
		end := off + int(n)
		// ivy: the bytes after the EOF record are not read, and the
		// maximum record size of the header is not checked, as Windows
		// does not use them.
		if typ == 0 && n != 6 {
			return malformed(off, "WMF EOF")
		}
		if err := p.emit(WMF, uint32(typ), 0, off, -1, b[off:end], b[off+6:end]); err != nil {
			return err
		}
		if typ == 0 {
			return nil
		}
		off = end
	}
	// ivy: Windows plays a file without an EOF record when its last record
	// ends at the end of the file.
	return nil
}
