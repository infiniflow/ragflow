// Copied from github.com/mgilbir/gowemf, file tiff_compression.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"bytes"
	"compress/zlib"
	"io"
)

func decodeTIFFStrip(dst, src []byte, compression uint64, rowBytes int) error {
	switch compression {
	case 1:
		if len(src) != len(dst) {
			return malformed(0, "TIFF uncompressed strip size")
		}
		copy(dst, src)
		return nil
	case 5:
		return decodeTIFFLZW(dst, src)
	case 8, 32946:
		r, err := zlib.NewReader(bytes.NewReader(src))
		if err != nil {
			return malformed(0, "TIFF Deflate header")
		}
		defer r.Close()
		if _, err = io.ReadFull(r, dst); err != nil {
			return malformed(0, "TIFF Deflate output size")
		}
		var extra [1]byte
		n, err := r.Read(extra[:])
		if n != 0 || err != io.EOF {
			return malformed(0, "TIFF Deflate excess data/checksum")
		}
		return nil
	case 32773:
		in, out := 0, 0
		for out < len(dst) {
			if in >= len(src) {
				return malformed(in, "truncated TIFF PackBits")
			}
			n := int(int8(src[in]))
			in++
			if n == -128 {
				continue
			}
			count := n + 1
			if n < 0 {
				count = 1 - n
			}
			if count > rowBytes-out%rowBytes {
				return malformed(in, "PackBits crosses row")
			}
			if n >= 0 {
				if count > len(src)-in {
					return malformed(in, "PackBits literal")
				}
				copy(dst[out:out+count], src[in:in+count])
				in += count
			} else {
				if in >= len(src) {
					return malformed(in, "PackBits repeat")
				}
				for j := 0; j < count; j++ {
					dst[out+j] = src[in]
				}
				in++
			}
			out += count
		}
		for ; in < len(src); in++ {
			if src[in] != 128 {
				return malformed(in, "PackBits trailing data")
			}
		}
		return nil
	}
	return failure(0, "TIFF compression", ErrUnsupported)
}

// TIFF 6.0 section 13 uses MSB-first codes and an early width change: after
// storing entry 510, the next code has width 10 (then 1022/2046). The fixed
// dictionary and expansion stack never grow from input-controlled sizes.
func decodeTIFFLZW(dst, src []byte) error {
	var prefix [4096]uint16
	var suffix [4096]byte
	var stack [4096]byte
	width, next, old := uint(9), 256+2, -1
	var bit uint64
	out := 0
	firstCode := true
	for {
		if uint64(len(src))*8-bit < uint64(width) {
			return malformed(int(bit/8), "TIFF LZW missing EOI")
		}
		code := 0
		for i := uint(0); i < width; i++ {
			code = code<<1 | int(src[int(bit/8)]>>uint(7-bit%8)&1)
			bit++
		}
		if firstCode {
			firstCode = false
			if code != 256 {
				return malformed(0, "TIFF LZW missing clear code")
			}
		}
		if code == 256 {
			width, next, old = 9, 258, -1
			continue
		}
		if code == 257 {
			if out != len(dst) {
				return malformed(int(bit/8), "TIFF LZW short output")
			}
			return nil
		}
		if old < 0 {
			if code > 255 {
				return malformed(int(bit/8), "TIFF LZW first symbol")
			}
			if out == len(dst) {
				return malformed(int(bit/8), "TIFF LZW excess output")
			}
			dst[out] = byte(code)
			out++
			old = code
			continue
		}
		if code > next || code >= 4096 {
			return malformed(int(bit/8), "TIFF LZW dictionary code")
		}
		current := code
		special := code == next
		if special {
			current = old
		}
		count := 0
		for current >= 256 {
			if current >= next || count >= len(stack)-1 {
				return malformed(int(bit/8), "TIFF LZW dictionary chain")
			}
			stack[count] = suffix[current]
			count++
			current = int(prefix[current])
		}
		stack[count] = byte(current)
		count++
		first := byte(current)
		needed := count
		if special {
			needed++
		}
		if needed > len(dst)-out {
			return malformed(int(bit/8), "TIFF LZW output limit")
		}
		for i := count - 1; i >= 0; i-- {
			dst[out] = stack[i]
			out++
		}
		if special {
			dst[out] = first
			out++
		}
		if next < 4096 {
			prefix[next] = uint16(old)
			suffix[next] = first
			next++
			if width < 12 && next == (1<<width)-1 {
				width++
			}
		}
		old = code
	}
}
