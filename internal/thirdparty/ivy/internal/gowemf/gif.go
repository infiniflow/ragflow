// Copied from github.com/mgilbir/gowemf, file gif.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
)

type gifFrameInfo struct {
	canvas, frame image.Rectangle
	background    color.NRGBA
}

// firstGIFFrame checks dimensions and structural spans before a codec can
// allocate pixels. Only the first image is decoded: later animation frames and
// their potentially unbounded aggregate pixel count are never loaded.
func firstGIFFrame(data []byte, l ImageLimits) (gifFrameInfo, error) {
	var result gifFrameInfo
	if len(data) < 13 {
		return result, malformed(0, "GIF logical screen")
	}
	w, h := uint64(u16(data[6:])), uint64(u16(data[8:]))
	if w == 0 || h == 0 {
		return result, malformed(6, "GIF dimensions")
	}
	if w > l.MaxPixels/h || w > uint64(int(^uint(0)>>1))/4/h {
		return result, failure(6, "GIF canvas pixels", ErrLimit)
	}
	result.canvas = image.Rect(0, 0, int(w), int(h))
	c := cursor{b: data, pos: 13}
	globalCount := 0
	if data[10]&128 != 0 {
		globalCount = 2 << uint(data[10]&7)
		palette := c.take(uint64(globalCount * 3))
		if c.err != nil {
			return result, c.err
		}
		index := int(data[11])
		if index >= globalCount {
			return result, malformed(11, "GIF background index")
		}
		col := palette[index*3:]
		result.background = color.NRGBA{col[0], col[1], col[2], 255}
	}
	transparent := false
	var transparentIndex byte
	for c.err == nil {
		marker := c.take(1)
		if marker == nil {
			break
		}
		switch marker[0] {
		case 0x21:
			label := c.take(1)
			if label == nil {
				break
			}
			switch label[0] {
			case 0xf9:
				control := c.take(6)
				if control == nil {
					break
				}
				if control[0] != 4 || control[5] != 0 {
					c.bad("GIF graphic control block")
					break
				}
				transparent = control[1]&1 != 0
				transparentIndex = control[4]
			case 0xfe, 0xff:
				for c.err == nil {
					size := c.take(1)
					if size == nil || size[0] == 0 {
						break
					}
					c.take(uint64(size[0]))
				}
			default:
				return result, failure(c.pos-1, "GIF rendering/control extension", ErrUnsupported)
			}
		case 0x2c:
			desc := c.take(9)
			if desc == nil {
				break
			}
			x, y, fw, fh := uint64(u16(desc)), uint64(u16(desc[2:])), uint64(u16(desc[4:])), uint64(u16(desc[6:]))
			if fw == 0 || fh == 0 || x+fw > w || y+fh > h {
				return result, malformed(c.pos-9, "GIF frame outside logical screen")
			}
			activeCount := globalCount
			if desc[8]&128 != 0 {
				activeCount = 2 << uint(desc[8]&7)
				c.take(uint64(activeCount * 3))
			}
			if c.err != nil {
				return result, c.err
			}
			if activeCount == 0 {
				return result, malformed(c.pos, "GIF missing color table")
			}
			if transparent && int(transparentIndex) >= activeCount {
				return result, malformed(c.pos, "GIF transparency index")
			}
			result.frame = image.Rect(int(x), int(y), int(x+fw), int(y+fh))
			// Embedded image transparency is preserved. Otherwise uncovered
			// canvas pixels use the specified global background, when present.
			if transparent {
				result.background = color.NRGBA{}
			}
			return result, nil
		case 0x3b:
			return result, malformed(c.pos-1, "GIF has no image")
		default:
			c.bad("GIF block marker")
		}
	}
	return result, c.err
}

func decodeGIFImage(data []byte, l ImageLimits) (image.Image, error) {
	info, err := firstGIFFrame(data, l)
	if err != nil {
		return nil, err
	}
	frame, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if frame.Bounds() != info.frame {
		return nil, malformed(0, "GIF frame bounds changed")
	}
	out := image.NewNRGBA(info.canvas)
	if info.background.A != 0 {
		draw.Draw(out, out.Bounds(), image.NewUniform(info.background), image.Point{}, draw.Src)
	}
	draw.Draw(out, info.frame, frame, info.frame.Min, draw.Over)
	return out, nil
}
