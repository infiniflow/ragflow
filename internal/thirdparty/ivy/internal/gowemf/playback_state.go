// Copied from github.com/mgilbir/gowemf, file playback_state.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"image"
	"math"
)

// deviceContext is the saved/restored playback state of MS-EMF 3.1.1.2 and
// MS-WMF 3.1.5. Clip nodes and object references are immutable or shared, so
// a value copy is a complete SaveDC snapshot.
type deviceContext struct {
	mapMode                                        uint32
	windowOrg, windowExt, viewportOrg, viewportExt Point
	world                                          Matrix
	pen, brush, font                               objectRef
	bkMode, polyFill, rop2, stretchMode, arcDir    uint32
	layout, bkColor, textColor, textAlign          uint32
	charExtra, breakExtra, breakCount              int32
	miterLimit                                     float64
	brushOrg, position                             Point
	clip                                           *ClipRegion
	meta                                           []*ClipRegion
	palette                                        paletteRef
}

func defaultDeviceContext() deviceContext {
	return deviceContext{
		mapMode: 1, windowExt: Point{1, 1}, viewportExt: Point{1, 1}, world: Identity(),
		pen: stockRef(stockPen(0x00000000, false)), brush: stockRef(stockBrush(0, 0x00ffffff)), font: stockRef(stockFonts[systemFont]),
		bkMode: 2, polyFill: 1, rop2: 13, stretchMode: 1, arcDir: 1, bkColor: 0x00ffffff, miterLimit: 10,
	}
}

// objectRef is a selection. It holds the object itself, as a GDI device
// context does: a saved state keeps its selections when their handles are
// deleted or reused, and RestoreDC brings them back (ORACLES.md). Slot-backed
// references remember the generation of the object they selected so that
// deleting it replaces only that object's current selection.
type objectRef struct {
	slot       uint32
	generation uint64
	stock      bool
	pen        *gdiPen
	brush      *gdiBrush
	font       *gdiFont
}

func stockRef(v any) objectRef {
	r := objectRef{stock: true}
	switch o := v.(type) {
	case *gdiPen:
		r.pen = o
	case *gdiBrush:
		r.brush = o
	case *gdiFont:
		r.font = o
	}
	return r
}

type playObject struct {
	pen     *gdiPen
	brush   *gdiBrush
	font    *gdiFont
	palette *gdiPalette
	region  []Rect // WMF region object in logical units; nil if not a region
}

// ivy: empty reports whether the slot holds no object.
func (o *playObject) empty() bool {
	return o.pen == nil && o.brush == nil && o.font == nil && o.palette == nil && o.region == nil
}

type gdiPen struct {
	null           bool
	style          uint32 // PS_* line style (low nibble)
	geometric      bool
	width          float64 // logical units; zero for cosmetic pens
	color          uint32
	cap            LineCap
	join           LineJoin
	dashes         []float64
	brush          *gdiBrush // non-nil for extended pens with non-solid brushes
	unsupportedWhy string
}

type gdiBrush struct {
	style, color, hatch uint32
	info, bits, packed  []byte
	usage               uint32
	pattern             image.Image
	mono                *monoPattern // monochrome pattern colored by the DC
	monoDIB             bool         // EMR_CREATEMONOBRUSH, decoded on first use
	unsupportedWhy      string
}

func stockPen(color uint32, null bool) *gdiPen {
	return &gdiPen{null: null, color: color, cap: CapRound, join: JoinRound}
}
func stockBrush(style, color uint32) *gdiBrush { return &gdiBrush{style: style, color: color} }

var (
	stockBrushes = [...]*gdiBrush{stockBrush(0, 0xffffff), stockBrush(0, 0xc0c0c0), stockBrush(0, 0x808080), stockBrush(0, 0x404040), stockBrush(0, 0), stockBrush(1, 0)}
	stockPens    = [...]*gdiPen{stockPen(0xffffff, false), stockPen(0, false), stockPen(0, true)}
)

func newPen(v Pen, extended bool) *gdiPen {
	g := &gdiPen{style: v.Style & 15, color: v.Color, cap: CapRound, join: JoinRound}
	if g.style == 5 {
		g.null = true
		return g
	}
	// PenStyle carries end cap and join bits for every pen record kind
	// (MS-WMF 2.1.1.23, MS-EMF 2.1.25).
	switch v.Style & 0xf00 {
	case 0:
	case 0x100:
		g.cap = CapSquare
	case 0x200:
		g.cap = CapFlat
	default:
		g.unsupportedWhy = "pen end cap"
	}
	switch v.Style & 0xf000 {
	case 0:
	case 0x1000:
		g.join = JoinBevel
	case 0x2000:
		g.join = JoinMiter
	default:
		g.unsupportedWhy = "pen line join"
	}
	if v.Style&^0x1ffff != 0 || v.Style&0xf0 != 0 {
		g.unsupportedWhy = "pen style flags"
	}
	if extended {
		g.geometric = v.Style&0x10000 != 0
		if g.geometric {
			g.width = v.Width
		}
		switch v.BrushStyle {
		case 0:
		case 1:
			g.null = true
		case 2:
			g.brush = &gdiBrush{style: 2, color: v.Color, hatch: v.Hatch}
		default:
			g.unsupportedWhy = "pattern pen brush"
		}
		for i := 0; i < v.Dashes.Len(); i++ {
			g.dashes = append(g.dashes, float64(v.Dashes.At(i)))
		}
		if g.style == 7 && len(g.dashes) == 0 {
			g.unsupportedWhy = "empty user pen style"
		}
	} else {
		// MS-WMF 3.1.4.2: a zero-width pen is one pixel wide at every scale.
		// LogPen widths are geometric with round caps and joins.
		w := math.Abs(v.Width)
		if w > 0 {
			g.geometric = true
			g.width = w
		}
	}
	if g.style > 8 {
		g.unsupportedWhy = "pen line style"
	}
	if g.style == 8 && g.geometric {
		g.unsupportedWhy = "PS_ALTERNATE geometric pen"
	}
	return g
}

func newBrush(r Record, v any, limits DecodeLimits) *gdiBrush {
	switch b := v.(type) {
	case Brush:
		g := &gdiBrush{style: b.Style, color: b.Color, hatch: b.Hatch}
		switch b.Style {
		case 0, 1, 2:
		default:
			g.unsupportedWhy = "brush style"
		}
		return g
	case PatternBrush:
		if r.Type == EMRCreateMonoBrush {
			// GDI creates no monochrome brush from a top-down DIB, so the
			// slot stays empty and selecting it keeps the previous brush
			// (ORACLES.md).
			if len(b.Info) >= 12 && u32(b.Info) >= 40 && int32(u32(b.Info[8:])) < 0 {
				return nil
			}
			// Monochrome brushes take text/background colors from the DC.
			return &gdiBrush{style: 3, info: b.Info, bits: b.Bits, usage: b.Usage, monoDIB: true}
		}
		return &gdiBrush{style: 5, info: b.Info, bits: b.Bits, usage: b.Usage}
	case PackedPatternBrush:
		if b.Style == 3 {
			// BS_PATTERN carries a Bitmap16 (MS-WMF 2.3.4.8).
			bm, err := parseBitmap16(b.Data, limits)
			if err != nil {
				// ivy: writers such as Stata put a DIB in place of the
				// Bitmap16. A monochrome DIB takes its colors from the
				// device context, as the Bitmap16 does, and a DIB of more
				// colors is a DIB pattern.
				if bits, ok := packedDIBBitCount(b.Data); ok {
					if bits == 1 {
						return &gdiBrush{style: 3, packed: b.Data, usage: b.Usage, monoDIB: true}
					}
					return &gdiBrush{style: 5, packed: b.Data, usage: b.Usage}
				}
				return &gdiBrush{style: 3, unsupportedWhy: "malformed Bitmap16 pattern brush"}
			}
			m, why := monoFromBitmap16(bm)
			return &gdiBrush{style: 3, mono: m, unsupportedWhy: why}
		}
		return &gdiBrush{style: 5, packed: b.Data, usage: b.Usage}
	case BitmapPatternBrush:
		m, why := monoFromBitmap16(b.Bitmap)
		return &gdiBrush{style: 3, mono: m, unsupportedWhy: why}
	}
	return nil
}

// ivy: packedDIBBitCount returns the bits of a pixel of a packed DIB, and
// false if data does not start with a BITMAPCOREHEADER or a
// BITMAPINFOHEADER of a later version.
func packedDIBBitCount(data []byte) (uint16, bool) {
	if len(data) < 12 {
		return 0, false
	}
	switch size := u32(data); {
	case size == 12:
		return u16(data[10:]), true
	case size >= 40 && len(data) >= 16:
		return u16(data[14:]), true
	}
	return 0, false
}

// pageMatrix maps page (logical, after world transform) to device units.
func (dc *deviceContext) pageMatrix() Matrix {
	sx, sy := dc.viewportExt.X/dc.windowExt.X, dc.viewportExt.Y/dc.windowExt.Y
	return Matrix{M11: sx, M22: sy, Dx: dc.viewportOrg.X - dc.windowOrg.X*sx, Dy: dc.viewportOrg.Y - dc.windowOrg.Y*sy}
}

// toDestination maps logical coordinates to destination coordinates.
func (p *player) toDestination(r Record) (Matrix, error) {
	m := p.dc.world.Then(p.dc.pageMatrix()).Then(p.base)
	if !m.Finite() {
		return Matrix{}, malformed(r.Offset, "non-finite playback transform")
	}
	return m, nil
}

// advanced reports whether GM_ADVANCED semantics are in effect. EMF records
// no graphics mode; a non-identity world transform implies GM_ADVANCED
// because GDI only records world transforms in that mode. WMF is always
// GM_COMPATIBLE.
func (p *player) advanced() bool { return p.dc.world != Identity() }

var fixedUnitsPerMM = [...]float64{2: 10, 3: 100, 4: 100 / 25.4, 5: 1000 / 25.4, 6: 1440 / 25.4}

// setMapMode follows MS-WMF 2.1.1.16. Invalid modes are ignored, as the GDI
// call they record would fail.
func (p *player) setMapMode(mode uint32) {
	dc := &p.dc
	switch {
	case mode == 1:
		dc.windowExt, dc.viewportExt = Point{1, 1}, Point{1, 1}
	case mode >= 2 && mode <= 6:
		u := fixedUnitsPerMM[mode]
		dc.windowExt = Point{u, u}
		dc.viewportExt = Point{1 / p.pixelMM.X, -1 / p.pixelMM.Y}
	case mode == 7 || mode == 8:
		// Extents are retained when switching to a scalable mode.
	default:
		return
	}
	dc.mapMode = mode
	p.isotropic()
}

// isotropic adjusts the viewport so one logical unit has the same physical
// size on both axes, keeping the smaller scale (MS-WMF 2.1.1.16).
func (p *player) isotropic() {
	dc := &p.dc
	if dc.mapMode != 7 {
		return
	}
	xs := math.Abs(dc.viewportExt.X/dc.windowExt.X) * p.pixelMM.X
	ys := math.Abs(dc.viewportExt.Y/dc.windowExt.Y) * p.pixelMM.Y
	if xs > ys {
		dc.viewportExt.X = math.Copysign(math.Abs(dc.windowExt.X)*ys/p.pixelMM.X, dc.viewportExt.X)
	} else if ys > xs {
		dc.viewportExt.Y = math.Copysign(math.Abs(dc.windowExt.Y)*xs/p.pixelMM.Y, dc.viewportExt.Y)
	}
}

// setExtent applies Set*Ext and Scale*Ext. Extents only change in scalable
// mapping modes, and zero extents are ignored, as GDI rejects those calls.
func (p *player) setExtent(window bool, v Point) {
	if p.dc.mapMode != 7 && p.dc.mapMode != 8 || v.X == 0 || v.Y == 0 || !finite(v.X) || !finite(v.Y) {
		return
	}
	if window {
		p.dc.windowExt = v
	} else {
		p.dc.viewportExt = v
	}
	p.isotropic()
}

func (p *player) restore(n int32) {
	var level int
	if n < 0 {
		level = len(p.saved) + int(n)
	} else {
		level = int(n) - 1
	}
	// Stream has already validated the level against its own saved count.
	p.dc = p.saved[level]
	p.saved = p.saved[:level]
}

// beyondTable reports the EMF object index equal to the header's Handles
// count. MS-EMF 3.1.1.1 sizes the table for it, but Windows counts the
// reserved index zero among the Handles: creating an object there fails, and
// records that use the index do nothing (ORACLES.md).
func (p *player) beyondTable(id uint32) bool {
	return p.format == EMF && uint64(id)+1 == uint64(len(p.objects))
}

func (p *player) create(r Record, id uint32, body any) error {
	if p.beyondTable(id) {
		return nil // drawn as Windows draws it, so not reported
	}
	var o playObject
	switch v := body.(type) {
	case Pen:
		o.pen = newPen(v, r.Format == EMF && r.Type == EMRExtCreatePen)
	case Brush, PatternBrush, PackedPatternBrush, BitmapPatternBrush:
		o.brush = newBrush(r, v, p.options.Stream.Decoding)
	case Font:
		o.font = newFont(v, r.Format)
	case Palette:
		entries, err := p.readEntries(r, v.Entries, v.Count)
		if err != nil {
			return err
		}
		o.palette = &gdiPalette{entries: entries}
	case WMFRegion:
		o.region = wmfRegionRects(v)
		if o.region == nil {
			o.region = []Rect{}
		}
	}
	// Stream has checked the handle against the table size.
	p.objects[id] = o
	return nil
}

func (p *player) selectObject(r Record, id uint32) error {
	if id&0x80000000 != 0 && p.format == EMF {
		n := id & 0x7fffffff
		switch {
		case n <= 5:
			p.dc.brush = stockRef(stockBrushes[n])
		case n >= 6 && n <= 8:
			p.dc.pen = stockRef(stockPens[n-6])
		case n == 18:
			p.dc.brush = stockRef(stockBrushes[0]) // DC_BRUSH defaults to white
		case n == 19:
			p.dc.pen = stockRef(stockPens[1]) // DC_PEN defaults to black
		case stockFonts[n] != nil:
			p.dc.font = stockRef(stockFonts[n])
		}
		return nil
	}
	o := p.objects[id]
	ref := objectRef{slot: id, generation: p.generations[id], pen: o.pen, brush: o.brush, font: o.font}
	switch {
	case o.pen != nil:
		p.dc.pen = ref
	case o.brush != nil:
		p.dc.brush = ref
	case o.font != nil:
		p.dc.font = ref
	case o.region != nil:
		// Selecting a WMF region replaces the clipping region.
		return p.selectRegionClip(r, o.region)
	}
	return nil
}

// deleteObject empties a slot. A current selection of the deleted object
// falls back to the default stock object (MS-EMF 3.1.1.1); selections held by
// saved states are kept.
func (p *player) deleteObject(id uint32) {
	gen := p.generations[id]
	p.objects[id] = playObject{}
	p.generations[id]++
	deleted := func(r objectRef) bool { return !r.stock && r.slot == id && r.generation == gen }
	if deleted(p.dc.pen) {
		p.dc.pen = stockRef(stockPens[1])
	}
	if deleted(p.dc.brush) {
		p.dc.brush = stockRef(stockBrushes[0])
	}
	if deleted(p.dc.font) {
		p.dc.font = stockRef(stockFonts[systemFont])
	}
	if s := p.dc.palette; s.selected && s.slot == id && s.generation == gen {
		p.dc.palette = paletteRef{}
	}
}

// clip combines area with the current clipping region.
func (p *player) combineClip(r Record, op ClipOp, area Path, rule FillRule) error {
	base := p.dc.clip
	if op == ClipReplace {
		base = nil
	}
	var depth uint32
	if base != nil {
		depth = base.depth
	}
	if uint64(depth)+p.metaSteps() >= uint64(p.options.MaxClipSteps) {
		return failure(r.Offset, "clip region steps", ErrLimit)
	}
	p.dc.clip = &ClipRegion{Base: base, Op: op, Area: area, Rule: rule, depth: depth + 1}
	return nil
}

func (p *player) metaSteps() uint64 {
	var n uint64
	for _, m := range p.dc.meta {
		n += uint64(m.depth)
	}
	return n
}

func (p *player) offsetClip(r Record, v Point) error {
	if p.dc.clip == nil {
		return nil
	}
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	o := Matrix{M11: m.M11, M12: m.M12, M21: m.M21, M22: m.M22}.Apply(v)
	if uint64(p.dc.clip.depth)+p.metaSteps() >= uint64(p.options.MaxClipSteps) {
		return failure(r.Offset, "clip region steps", ErrLimit)
	}
	p.dc.clip = &ClipRegion{Base: p.dc.clip, Op: ClipOffset, Offset: o, depth: p.dc.clip.depth + 1}
	return nil
}

// setMetaRegion intersects the metaregion with the clipping region and resets
// the clipping region (MS-EMF 2.3.2).
func (p *player) setMetaRegion() {
	if p.dc.clip == nil {
		return
	}
	meta := make([]*ClipRegion, len(p.dc.meta), len(p.dc.meta)+1)
	copy(meta, p.dc.meta)
	p.dc.meta = append(meta, p.dc.clip)
	p.dc.clip = nil
}

func (p *player) selectRegionClip(r Record, rects []Rect) error {
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	path, ok := rectsPath(rects, m, p.options.MaxPathPoints)
	if !ok {
		return failure(r.Offset, "clip region points", ErrLimit)
	}
	return p.combineClip(r, ClipReplace, path, NonZero)
}

func (p *player) currentClip() Clip {
	if p.dc.clip == nil {
		return Clip(p.dc.meta[:len(p.dc.meta):len(p.dc.meta)])
	}
	c := make(Clip, len(p.dc.meta)+1)
	copy(c, p.dc.meta)
	c[len(p.dc.meta)] = p.dc.clip
	return c
}
