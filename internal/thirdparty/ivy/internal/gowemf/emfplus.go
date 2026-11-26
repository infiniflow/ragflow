// Copied from github.com/mgilbir/gowemf, file emfplus.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// MS-EMFPLUS 2.3.3.3 and 2.3.3.1. A comment contains whole records; object
// continuation across records is retained for a future semantic decoder.
func (p *parser) plus(b []byte, base, parent int) error {
	if len(b) == 0 {
		return malformed(base, "empty EMF+ comment")
	}
	for pos := 0; pos < len(b); {
		off := base + pos
		if p.plusEnded {
			return malformed(off, "EMF+ record after EOF")
		}
		if len(b)-pos < 12 {
			return malformed(off, "EMF+ record header")
		}
		r := b[pos:]
		typ, flags, n, dataSize := u16(r), u16(r[2:]), uint64(u32(r[4:])), uint64(u32(r[8:]))
		if err := p.record(off, n, len(r), 12, 4); err != nil {
			return err
		}
		if dataSize > n-12 || n-12-dataSize > 3 {
			return malformed(off+8, "EMF+ data size")
		}
		if p.header.EMFPlus == nil && typ != 0x4001 {
			return malformed(off, "missing EMF+ header")
		}
		switch typ {
		case 0x4001:
			if p.header.EMFPlus != nil {
				return malformed(off, "duplicate EMF+ header")
			}
			if n != 28 || dataSize != 16 {
				return malformed(off, "EMF+ header size")
			}
			if u32(r[12:])>>12 != 0xdbc01 {
				return malformed(off+12, "EMF+ graphics signature")
			}
			p.header.EMFPlus = &EMFPlusHeader{flags&1 != 0, u32(r[12:]), u32(r[16:]), u32(r[20:]), u32(r[24:])}
		case 0x4002:
			if n != 12 || dataSize != 0 {
				return malformed(off, "EMF+ EOF size")
			}
			p.plusEnded = true
		}
		if err := p.emit(EMFPlus, uint32(typ), flags, off, parent, r[:int(n)], r[12:12+int(dataSize)]); err != nil {
			return err
		}
		pos += int(n)
	}
	return nil
}
