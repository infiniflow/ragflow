// Copied from github.com/mgilbir/gowemf, file codepage_dbcs.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"encoding/base64"
	"sync"
)

// dbcsCodePage is a double-byte Windows code page from the generated tables.
// Its trail mappings decode on first use.
type dbcsCodePage struct {
	defaultChar uint16
	leads       [][2]byte
	single      [256]uint16
	pairs       string

	once sync.Once
	rows [256]*[256]uint16 // by lead byte; 0 is an unmapped trail
}

func (c *dbcsCodePage) isLead(b byte) bool {
	for _, r := range c.leads {
		if b >= r[0] && b <= r[1] {
			return true
		}
	}
	return false
}

// load decodes pairs, which the generator wrote and tests verify; malformed
// data is a build defect and panics.
func (c *dbcsCodePage) load() {
	c.once.Do(func() {
		data, err := base64.StdEncoding.DecodeString(c.pairs)
		if err != nil {
			panic("gowemf: corrupt double-byte code page table")
		}
		next := func() uint64 {
			var v uint64
			for shift := uint(0); ; shift += 7 {
				if len(data) == 0 || shift > 56 {
					panic("gowemf: corrupt double-byte code page table")
				}
				b := data[0]
				data = data[1:]
				v |= uint64(b&0x7f) << shift
				if b < 0x80 {
					return v
				}
			}
		}
		var slots []*uint16
		for lead := 0; lead < 256; lead++ {
			if c.isLead(byte(lead)) {
				row := new([256]uint16)
				c.rows[lead] = row
				for t := range row {
					slots = append(slots, &row[t])
				}
			}
		}
		prev, i := int64(0), 0
		for len(data) > 0 {
			switch u := next(); u {
			case 0:
				n := next()
				if n > uint64(len(slots)-i) {
					panic("gowemf: corrupt double-byte code page table")
				}
				i += int(n)
			default:
				z := u - 1
				delta := int64(z >> 1)
				if z&1 != 0 {
					delta = -delta - 1
				}
				prev += delta
				if i >= len(slots) || prev <= 0 || prev > 0xffff {
					panic("gowemf: corrupt double-byte code page table")
				}
				*slots[i] = uint16(prev)
				i++
			}
		}
		if i != len(slots) {
			panic("gowemf: corrupt double-byte code page table")
		}
	})
}

// decode converts bytes to UTF-16 as MS-UCODEREF 3.1.5.1.1 specifies for a
// double-byte code page: a byte that is not a lead byte maps on its own; a
// lead byte and a mapped trail byte form one character; otherwise the
// default character replaces the lead byte and the byte after it, unless
// that byte is NUL, which Windows' MultiByteToWideChar keeps (ORACLES.md).
// spans gives the number of bytes each output unit came from.
func (c *dbcsCodePage) decode(b []byte) (out []uint16, spans []int) {
	c.load()
	out, spans = make([]uint16, 0, len(b)), make([]int, 0, len(b))
	for i := 0; i < len(b); i++ {
		if !c.isLead(b[i]) {
			out, spans = append(out, c.single[b[i]]), append(spans, 1)
			continue
		}
		if i+1 < len(b) {
			if u := c.rows[b[i]][b[i+1]]; u != 0 {
				out, spans = append(out, u), append(spans, 2)
				i++
				continue
			}
		}
		n := min(2, len(b)-i)
		if n == 2 && b[i+1] == 0 {
			n = 1
		}
		out, spans = append(out, c.defaultChar), append(spans, n)
		i += n - 1
	}
	return out, spans
}

// decodeANSI converts bytes in a Windows ANSI code page to UTF-16. spans is
// nil when every byte became one unit; ok is false for a code page without
// tables.
func decodeANSI(cp uint16, b []byte) (out []uint16, spans []int, ok bool) {
	if d := dbcsCodePages[cp]; d != nil {
		out, spans = d.decode(b)
		return out, spans, true
	}
	table := codePageHigh[cp]
	if table == nil {
		return nil, nil, false
	}
	out = make([]uint16, len(b))
	for i, c := range b {
		if c < 0x80 {
			out[i] = uint16(c)
		} else {
			out[i] = table[c-0x80]
		}
	}
	return out, nil, true
}
