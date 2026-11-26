// Copied from github.com/mgilbir/gowemf, file dib_rle.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "image"

// MS-WMF 3.1.6.1–2. Coordinates advance monotonically through the bottom-up
// image. Run and delta bounds are checked before writing. With out=nil this
// validates the compressed stream without allocating an expanded image.
func (d *DIB) rle(out *image.NRGBA) error {
	b := d.pixels
	x, y, pos := 0, 0, 0
	write := func(index byte) error {
		if int(index) >= len(d.palette) {
			return malformed(pos, "RLE palette index")
		}
		if out != nil {
			col := d.palette[index]
			i := (d.height-1-y)*out.Stride + x*4
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = col.R, col.G, col.B, col.A
		}
		x++
		return nil
	}
	for pos < len(b) {
		if len(b)-pos < 2 {
			return malformed(pos, "RLE command")
		}
		count, value := int(b[pos]), b[pos+1]
		pos += 2
		if count == 0 {
			switch value {
			case 0:
				if y >= d.height {
					return malformed(pos, "RLE row overflow")
				}
				x = 0
				y++
				continue
			case 1:
				return nil
			case 2:
				if len(b)-pos < 2 {
					return malformed(pos, "RLE delta")
				}
				dx, dy := int(b[pos]), int(b[pos+1])
				pos += 2
				if dx > d.width-x || dy >= d.height-y {
					return malformed(pos, "RLE delta outside image")
				}
				x += dx
				y += dy
				continue
			}
			count = int(value)
			if y >= d.height || count > d.width-x {
				return malformed(pos, "RLE absolute run outside row")
			}
			n := count
			if d.bpp == 4 {
				n = (count + 1) / 2
			}
			padded := (n + 1) &^ 1
			if padded > len(b)-pos {
				return malformed(pos, "RLE absolute bytes")
			}
			for i := 0; i < count; i++ {
				var index byte
				if d.bpp == 4 {
					index = b[pos+i/2] >> uint(4*(1-i%2)) & 15
				} else {
					index = b[pos+i]
				}
				if err := write(index); err != nil {
					return err
				}
			}
			pos += padded
		} else {
			if y >= d.height || count > d.width-x {
				return malformed(pos, "RLE encoded run outside row")
			}
			for i := 0; i < count; i++ {
				index := value
				if d.bpp == 4 {
					index = value >> uint(4*(1-i%2)) & 15
				}
				if err := write(index); err != nil {
					return err
				}
			}
		}
	}
	return malformed(pos, "missing RLE end-of-bitmap")
}
