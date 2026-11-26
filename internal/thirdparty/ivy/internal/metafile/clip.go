package metafile

import (
	"github.com/xo/okage"

	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/gowemf"
)

// clipped calls draw with the clip of the device set to clip. The clip
// stays on the device after draw, and the next call with the same clip
// draws at once: a clip of okage is a mask of the size of the device, and
// a picture with many records under one clip, such as the WMF pictures of
// poi/xls/landscape-a4-drawing.xls, spends most of its time making the
// same mask again. endClip removes the clip.
func (b *backend) clipped(clip gowemf.Clip, draw func()) {
	if b.clipOpen && sameClip(b.clip, clip) {
		if !b.clipEmpty {
			draw()
		}
		return
	}
	b.endClip()
	if len(clip) == 0 {
		draw()
		return
	}
	var steps []*gowemf.ClipRegion
	for _, r := range clip {
		var ok bool
		if steps, ok = clipSteps(r, steps); !ok {
			b.clippedMasks(clip, draw)
			return
		}
	}
	b.d.Save()
	b.clipOpen, b.clip, b.clipEmpty = true, clip, false
	for _, s := range steps {
		p := b.path(s.Area)
		if p == nil {
			// an empty area clips everything
			b.clipEmpty = true
			return
		}
		b.d.ClipPath(p, rule(s.Rule))
	}
	draw()
}

// endClip removes the clip that clipped left on the device.
func (b *backend) endClip() {
	if b.clipOpen {
		b.d.Restore()
		b.clipOpen, b.clip, b.clipEmpty = false, nil, false
	}
}

// sameClip reports whether the clips a and b hold the same regions.
func sameClip(a, b gowemf.Clip) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// clippedMasks calls draw with the clip set to clip, whose regions that
// are not intersections of areas become masks of groups.
func (b *backend) clippedMasks(clip gowemf.Clip, draw func()) {
	b.d.Save()
	defer b.d.Restore()
	var masks []*gowemf.ClipRegion
	for _, r := range clip {
		steps, ok := clipSteps(r, nil)
		if !ok {
			masks = append(masks, r)
			continue
		}
		for _, s := range steps {
			p := b.path(s.Area)
			if p == nil {
				// an empty area clips everything
				return
			}
			b.d.ClipPath(p, rule(s.Rule))
		}
	}
	for _, r := range masks {
		b.d.BeginGroup(display.Group{Opacity: 1, Mask: &display.Mask{
			Type:    okage.MaskTypeAlpha,
			Content: func(d display.Device) { b.region(d, r) },
			Key:     r,
		}})
	}
	draw()
	for range masks {
		b.d.EndGroup()
	}
}

// clipSteps appends to steps the regions whose intersection is r, and
// returns false if r is not such an intersection. A nil region is the whole
// surface, which needs no step.
func clipSteps(r *gowemf.ClipRegion, steps []*gowemf.ClipRegion) ([]*gowemf.ClipRegion, bool) {
	if r == nil {
		return steps, true
	}
	switch r.Op {
	case gowemf.ClipReplace:
		if r.Operand != nil {
			return clipSteps(r.Operand, steps)
		}
		return append(steps, r), true
	case gowemf.ClipIntersect:
		steps, ok := clipSteps(r.Base, steps)
		if !ok {
			return nil, false
		}
		if r.Operand != nil {
			return clipSteps(r.Operand, steps)
		}
		return append(steps, r), true
	}
	return nil, false
}

// region draws the region r onto d in opaque black, for a soft mask. Each
// operator of a region becomes the blend mode of a group, which combines
// the operand with the base that d holds.
func (b *backend) region(d display.Device, r *gowemf.ClipRegion) {
	if r == nil {
		d.FillPath(okage.PathFromRect(b.surface), okage.FillRuleWinding, okage.SolidColor(okage.ColorBlack))
		return
	}
	var blend okage.BlendMode
	switch r.Op {
	case gowemf.ClipReplace:
		b.operand(d, r)
		return
	case gowemf.ClipOffset:
		d.Save()
		d.Concat(okage.TransformFromTranslate(float32(r.Offset.X), float32(r.Offset.Y)))
		b.region(d, r.Base)
		d.Restore()
		return
	case gowemf.ClipIntersect:
		blend = okage.BlendModeDestinationIn
	case gowemf.ClipUnion:
		blend = okage.BlendModeSourceOver
	case gowemf.ClipXor:
		blend = okage.BlendModeXor
	case gowemf.ClipDifference:
		blend = okage.BlendModeDestinationOut
	case gowemf.ClipComplement:
		blend = okage.BlendModeSourceOut
	default:
		return
	}
	b.region(d, r.Base)
	d.BeginGroup(display.Group{Opacity: 1, Blend: blend, Isolated: true})
	b.operand(d, r)
	d.EndGroup()
}

// operand draws the operand of r: a region, or an area.
func (b *backend) operand(d display.Device, r *gowemf.ClipRegion) {
	if r.Operand != nil {
		b.region(d, r.Operand)
		return
	}
	if p := b.path(r.Area); p != nil {
		d.FillPath(p, rule(r.Rule), okage.SolidColor(okage.ColorBlack))
	}
}
