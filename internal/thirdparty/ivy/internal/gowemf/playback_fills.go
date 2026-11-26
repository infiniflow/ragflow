// Copied from github.com/mgilbir/gowemf, file playback_fills.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
)

// gdiPalette is a logical palette object. Entries are owned; palette updates
// modify the object in place, as they do in GDI, so every selection of it
// sees them.
type gdiPalette struct {
	entries   []paletteEntry
	corrected bool // EMR_COLORCORRECTPALETTE applied an ICM correction
}

type paletteEntry struct {
	color color.NRGBA
	flags byte
}

// paletteRef selects a logical palette. Like objectRef it holds the palette
// itself, which palette records still update through its slot; the
// generation identifies the selection a deletion replaces.
type paletteRef struct {
	pal        *gdiPalette
	slot       uint32
	generation uint64
	selected   bool
}

// Palette entries are PALETTEENTRY values: red, green, blue, flags. MS-WMF
// 2.2.2.13 specifies this order; MS-EMF 2.2.18 draws LogPaletteEntry as
// reserved, blue, green, red, but EMR palette records serialize the same GDI
// structure (see ORACLES.md for the oracle evidence).
func (p *player) readEntries(r Record, b []byte, n uint32) ([]paletteEntry, error) {
	if err := p.spendPixels(r, int(n), 1); err != nil {
		return nil, err
	}
	out := make([]paletteEntry, n)
	for i := range out {
		if 4*i+3 < len(b) {
			e := b[4*i:]
			out[i] = paletteEntry{color.NRGBA{e[0], e[1], e[2], 255}, e[3]}
		} else {
			out[i] = paletteEntry{color.NRGBA{A: 255}, 0}
		}
	}
	return out, nil
}

func (p *player) palette(slot uint32) *gdiPalette {
	if uint64(slot) < uint64(len(p.objects)) {
		return p.objects[slot].palette
	}
	return nil
}

// selectedPalette returns the logical palette in the device context, or nil
// when the default palette is selected.
func (p *player) selectedPalette() *gdiPalette {
	return p.dc.palette.pal
}

func (p *player) selectPalette(id uint32) {
	if p.format == EMF && id == 0x8000000f {
		p.dc.palette = paletteRef{} // DEFAULT_PALETTE
		return
	}
	if p.beyondTable(id) {
		return
	}
	p.dc.palette = paletteRef{pal: p.palette(id), slot: id, generation: p.generations[id], selected: true}
}

// updatePalette applies SetPaletteEntries, ResizePalette and AnimatePalette.
// Stream has resolved WMF updates to the selected palette and checked ranges.
func (p *player) updatePalette(r Record, v Palette, op string) error {
	pal := p.palette(v.Handle)
	if pal == nil && p.beyondTable(v.Handle) {
		return nil
	}
	if pal == nil {
		return malformed(r.Offset, "palette update target")
	}
	switch op {
	case "resize":
		entries, err := p.readEntries(r, nil, v.Count)
		if err != nil {
			return err
		}
		copy(entries, pal.entries)
		pal.entries = entries
	case "set", "animate":
		next, err := p.readEntries(r, v.Entries, v.Count)
		if err != nil {
			return err
		}
		for i, e := range next {
			k := int(v.Start) + i
			if k >= len(pal.entries) {
				return malformed(r.Offset, "palette entry range")
			}
			// AnimatePalette replaces only entries reserved for animation.
			if op == "animate" && pal.entries[k].flags&1 == 0 {
				continue
			}
			pal.entries[k] = e
		}
	}
	return nil
}

// color resolves a COLORREF. MS-WMF 2.2.2.8 requires a zero high byte; GDI's
// PALETTERGB (0x02) form is the plain RGB value on a true-color playback
// device, and PALETTEINDEX (0x01) selects an entry of the logical palette.
func (p *player) color(r Record, ref uint32) (color.NRGBA, error) {
	switch ref >> 24 {
	case 0, 2:
		return color.NRGBA{byte(ref), byte(ref >> 8), byte(ref >> 16), 255}, nil
	case 1:
		pal := p.selectedPalette()
		if pal == nil {
			return color.NRGBA{}, p.unsupported(r, "PALETTEINDEX color without a selected logical palette")
		}
		if pal.corrected {
			return color.NRGBA{}, p.unsupported(r, "ICM-corrected palette")
		}
		i := int(ref & 0xffff)
		if i >= len(pal.entries) {
			return color.NRGBA{}, p.unsupported(r, "PALETTEINDEX beyond the logical palette")
		}
		return pal.entries[i].color, nil
	}
	return color.NRGBA{}, p.unsupported(r, "COLORREF with a reserved high byte")
}

// logicalPalette returns the colors DIB_PAL_COLORS bitmaps index.
func (p *player) logicalPalette(r Record, usage uint32) ([]color.NRGBA, error) {
	switch usage {
	case 0:
		// ivy: the selected palette, for a packed DIB without its color
		// table
		if pal := p.selectedPalette(); pal != nil && !pal.corrected {
			out := make([]color.NRGBA, len(pal.entries))
			for i, e := range pal.entries {
				out[i] = e.color
			}
			return out, nil
		}
		return nil, nil
	case 1:
		pal := p.selectedPalette()
		if pal == nil {
			return nil, p.unsupported(r, "DIB_PAL_COLORS without a selected logical palette")
		}
		if pal.corrected {
			return nil, p.unsupported(r, "ICM-corrected palette")
		}
		out := make([]color.NRGBA, len(pal.entries))
		for i, e := range pal.entries {
			out[i] = e.color
		}
		return out, nil
	}
	return nil, p.unsupported(r, "DIB_PAL_INDICES system palette indexes")
}

// monoPattern is a monochrome pattern whose colors come from the device
// context: 0 bits in the text color and 1 bits in the background color, as
// GDI's CreatePatternBrush documents for monochrome bitmaps.
type monoPattern struct {
	w, h   int
	bits   []bool
	cache  *image.NRGBA
	fg, bg color.NRGBA
}

func (m *monoPattern) image(p *player, r Record, fg, bg color.NRGBA) (*image.NRGBA, error) {
	if m.cache != nil && m.fg == fg && m.bg == bg {
		return m.cache, nil
	}
	if err := p.spendPixels(r, m.w, m.h); err != nil {
		return nil, err
	}
	im := image.NewNRGBA(image.Rect(0, 0, m.w, m.h))
	for i, one := range m.bits {
		c := fg
		if one {
			c = bg
		}
		im.SetNRGBA(i%m.w, i/m.w, c)
	}
	m.cache, m.fg, m.bg = im, fg, bg
	return im, nil
}

// monoIndexPalette lets a monochrome DIB be parsed as two palette indexes.
var monoIndexPalette = []color.NRGBA{{A: 255}, {255, 255, 255, 255}}

// monoBrushPattern decodes an EMR_CREATEMONOBRUSH bitmap, whose colors the
// DC supplies. As GDI does (ORACLES.md), a DIB_PAL_INDICES bitmap's bits are
// used as they are, a DIB_RGB_COLORS bitmap's bits are inverted unless its
// second color is the brighter, and a DIB_PAL_COLORS bitmap paints only the
// text color.
func (p *player) monoBrushPattern(r Record, g *gdiBrush) (*monoPattern, error) {
	// The bits do not depend on the color table; only DIB_RGB_COLORS reads it.
	usage, logical := uint32(2), monoIndexPalette
	if g.usage == 0 {
		usage, logical = 0, nil
	}
	var d *DIB
	var err error
	if g.packed != nil {
		// ivy: a DIB in place of the Bitmap16 of a WMF pattern brush.
		d, err = ParsePackedDIB(g.packed, usage, logical, p.options.Images)
	} else {
		d, err = ParseDIB(g.info, g.bits, usage, logical, p.options.Images)
	}
	if err == nil {
		err = p.spendPixels(r, d.width, d.height)
	}
	var bits []bool
	if err == nil {
		bits, err = d.monoBits()
	}
	if err != nil {
		return nil, p.imageError(r, err)
	}
	invert := g.usage == 0 && (len(d.palette) < 2 || !brighter(d.palette[1], d.palette[0]))
	for i := range bits {
		switch g.usage {
		case 0:
			bits[i] = bits[i] != invert
		case 1:
			bits[i] = false
		}
	}
	return &monoPattern{w: d.width, h: d.height, bits: bits}, nil
}

// brighter reports whether a is brighter than b by the unweighted sum of
// their channels, the measure GDI's monochrome conversion was seen to use.
func brighter(a, b color.NRGBA) bool {
	return int(a.R)+int(a.G)+int(a.B) > int(b.R)+int(b.G)+int(b.B)
}

func parseBitmap16(data []byte, limits DecodeLimits) (Bitmap16, error) {
	c := cursor{b: data, limits: limits.defaults()}
	v := c.bitmap16(false)
	return v, c.err
}

// monoFromBitmap16 accepts the device-independent 1-plane, 1-bit form of a
// Bitmap16; other depths depend on the recording device.
func monoFromBitmap16(b Bitmap16) (*monoPattern, string) {
	if b.Planes != 1 || b.BitCount != 1 {
		return nil, "device-dependent Bitmap16 pattern brush"
	}
	w, h := int(b.Width), int(b.Height)
	m := &monoPattern{w: w, h: h, bits: make([]bool, w*h)}
	for y := 0; y < h; y++ {
		row := b.Bits[y*int(b.WidthBytes):]
		for x := 0; x < w; x++ {
			m.bits[y*w+x] = row[x/8]>>uint(7-x%8)&1 != 0
		}
	}
	return m, ""
}

// regionRects collects the rectangles of an EMF RegionData object.
func regionRects(v Region) []Rect {
	out := make([]Rect, int(v.Count))
	for i := range out {
		out[i] = v.RectangleAt(i)
	}
	return out
}

// wmfRegionRects expands WMF scans: each pair of endpoints is a rectangle
// [left,right) x [top,bottom) in logical units (MS-WMF 2.2.2.21).
func wmfRegionRects(v WMFRegion) []Rect {
	var out []Rect
	for _, s := range v.Scans {
		for i := 0; i+1 < s.Endpoints.Len(); i += 2 {
			out = append(out, Rect{int32(s.Endpoints.At(i)), int32(s.Top), int32(s.Endpoints.At(i + 1)), int32(s.Bottom)})
		}
	}
	return out
}

func rectsPath(rects []Rect, m Matrix, limit uint64) (Path, bool) {
	b := pathBuilder{limit: limit}
	s := shape{&b, m}
	for _, q := range rects {
		l, t, r, bt := normalize(q)
		if l == r || t == bt {
			continue
		}
		s.moveTo(Point{l, t})
		s.lineTo(Point{r, t})
		s.lineTo(Point{r, bt})
		s.lineTo(Point{l, bt})
		b.close()
	}
	return b.path, !b.err
}

// paintRegion fills a region with a brush (FillRgn, PaintRgn and the WMF
// equivalents). A non-nil frame draws only the region's border of frame.X by
// frame.Y logical units (FrameRgn); an empty border draws nothing.
// ivy: invertRegion inverts the destination in a region, the raster
// operation DSTINVERT, if the backend can read its destination.
func (p *player) invertRegion(c Command, rects []Rect) error {
	r := c.Source
	rb, ok := backendAs[RasterBackend](p.backend)
	if !ok {
		return p.unsupported(r, "region inversion reads the destination")
	}
	if err := p.drawable(r, c.ColorState); err != nil {
		return err
	}
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	area, ok := rectsPath(rects, m, p.options.MaxPathPoints)
	if !ok {
		return failure(r.Offset, "region points", ErrLimit)
	}
	if len(area.Verbs) == 0 {
		return nil
	}
	return rb.DrawRaster(RasterDraw{Operation: 0x55, Area: area}, p.currentClip())
}

func (p *player) paintRegion(c Command, rects []Rect, brush *gdiBrush, frame *Point) error {
	r := c.Source
	if err := p.drawable(r, c.ColorState); err != nil {
		return err
	}
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	paint, err := p.brushPaint(r, brush, m, true)
	if err != nil || paint == nil {
		return err
	}
	area, ok := rectsPath(rects, m, p.options.MaxPathPoints)
	if !ok {
		return failure(r.Offset, "region points", ErrLimit)
	}
	if len(area.Verbs) == 0 {
		return nil
	}
	clip := p.currentClip()
	if frame != nil {
		if frame.X == 0 || frame.Y == 0 {
			return nil
		}
		// The frame is R minus R eroded by the brush box, which equals R
		// intersected with the complement of R dilated by that box.
		border, err := p.dilatedComplement(r, rects, math.Abs(frame.X), math.Abs(frame.Y))
		if err != nil {
			return err
		}
		path, ok := rectsPath(border, m, p.options.MaxPathPoints)
		if !ok {
			return failure(r.Offset, "region frame points", ErrLimit)
		}
		clip = append(clip[:len(clip):len(clip)], &ClipRegion{Op: ClipReplace, Area: path, Rule: NonZero, depth: 1})
	}
	return p.backend.FillPath(area, NonZero, *paint, clip)
}

// dilatedComplement returns the complement of a rectangle union within its
// bounds grown by (w,h), with every complement rectangle grown by (w,h).
// The sweep's work is charged to a per-Play budget.
func (p *player) dilatedComplement(r Record, rects []Rect, w, h float64) ([]Rect, error) {
	if len(rects) == 0 {
		return nil, nil
	}
	pad := func(v float64) int32 { return int32(math.Ceil(v)) }
	minX, minY, maxX, maxY := int64(math.MaxInt32), int64(math.MaxInt32), int64(math.MinInt32), int64(math.MinInt32)
	var ys []int64
	for _, q := range rects {
		l, t, rr, b := normalize(q)
		minX, maxX = min(minX, int64(l)), max(maxX, int64(rr))
		minY, maxY = min(minY, int64(t)), max(maxY, int64(b))
		ys = append(ys, int64(t), int64(b))
	}
	wx, hy := int64(pad(w)), int64(pad(h))
	box := [4]int64{minX - wx, minY - hy, maxX + wx, maxY + hy}
	ys = append(ys, box[1], box[3])
	sort.Slice(ys, func(i, j int) bool { return ys[i] < ys[j] })
	var out []Rect
	grow := func(l, t, rr, b int64) {
		out = append(out, Rect{clamp32(l - wx), clamp32(t - hy), clamp32(rr + wx), clamp32(b + hy)})
	}
	type span struct{ l, r int64 }
	for i := 0; i+1 < len(ys); i++ {
		t, b := ys[i], ys[i+1]
		if t == b {
			continue
		}
		p.budget.regionWork += uint64(len(rects))
		if p.budget.regionWork > regionWorkLimit {
			return nil, failure(r.Offset, "region frame work", ErrLimit)
		}
		var spans []span
		for _, q := range rects {
			l, qt, rr, qb := normalize(q)
			if int64(qt) <= t && int64(qb) >= b && l < rr {
				spans = append(spans, span{int64(l), int64(rr)})
			}
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].l < spans[j].l })
		x := box[0]
		for _, s := range spans {
			if s.l > x {
				grow(x, t, s.l, b)
			}
			x = max(x, s.r)
		}
		if x < box[2] {
			grow(x, t, box[2], b)
		}
	}
	return out, nil
}

const regionWorkLimit = 16_000_000

func clamp32(v int64) int32 {
	return int32(max(math.MinInt32, min(math.MaxInt32, v)))
}

// GradientTriangle is one triangle of a Gouraud-shaded mesh in destination
// coordinates. Colors are interpolated linearly in destination space. EMF
// meshes carry MS-EMF TriVertex's 16-bit channels and are opaque, because
// GradientFill ignores the vertex alpha (MS-EMF 2.2.26); EMF+ path gradient
// meshes carry 8-bit colors scaled to 16 bits and one alpha shared by every
// vertex of the mesh.
type GradientTriangle struct {
	Points [3]Point
	Colors [3]color.NRGBA64
}

// GradientBackend is a Backend that can fill smoothly shaded meshes. Rectangle
// gradients arrive as two triangles each, whose interpolation is exactly
// linear along the gradient axis. Backends without it get EMR_GRADIENTFILL
// reported unsupported.
type GradientBackend interface {
	Backend
	FillGradient(mesh []GradientTriangle, clip Clip) error
}

func (p *player) gradient(c Command, v Gradient) error {
	r := c.Source
	gb, ok := backendAs[GradientBackend](p.backend)
	if !ok {
		return p.unsupported(r, "gradient fill")
	}
	if err := p.drawable(r, c.ColorState); err != nil {
		return err
	}
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	vertex := func(i int) (Point, color.NRGBA64) {
		g := v.Vertices.At(int(v.Indexes.At(i)))
		return g.Point, color.NRGBA64{g.Red, g.Green, g.Blue, 0xffff}
	}
	var mesh []GradientTriangle
	if v.Mode == 2 {
		n := v.Indexes.Len() / 3
		if err := p.spendPixels(r, n, 1); err != nil {
			return err
		}
		mesh = make([]GradientTriangle, 0, n)
		for i := 0; i < n; i++ {
			var t GradientTriangle
			for k := 0; k < 3; k++ {
				q, col := vertex(3*i + k)
				t.Points[k], t.Colors[k] = m.Apply(q), col
			}
			mesh = append(mesh, t)
		}
	} else {
		n := v.Indexes.Len() / 2
		if err := p.spendPixels(r, 2*n, 1); err != nil {
			return err
		}
		mesh = make([]GradientTriangle, 0, 2*n)
		for i := 0; i < n; i++ {
			ul, c0 := vertex(2 * i)
			lr, c1 := vertex(2*i + 1)
			ur, ll := Point{lr.X, ul.Y}, Point{ul.X, lr.Y}
			// RECT_H interpolates from the left edge to the right edge, RECT_V
			// from the top edge to the bottom edge.
			cur, cll := c1, c0
			if v.Mode == 1 {
				cur, cll = c0, c1
			}
			a, b, cc, d := m.Apply(ul), m.Apply(ur), m.Apply(lr), m.Apply(ll)
			mesh = append(mesh, GradientTriangle{[3]Point{a, b, cc}, [3]color.NRGBA64{c0, cur, c1}}, GradientTriangle{[3]Point{a, cc, d}, [3]color.NRGBA64{c0, c1, cll}})
		}
	}
	if len(mesh) == 0 {
		return nil
	}
	return gb.FillGradient(mesh, p.currentClip())
}

func (p *player) regionObject(r Record, slot uint32) ([]Rect, error) {
	if uint64(slot) >= uint64(len(p.objects)) || p.objects[slot].region == nil {
		return nil, malformed(r.Offset, "region object reference")
	}
	return p.objects[slot].region, nil
}

func (p *player) brushObject(r Record, id uint32) (*gdiBrush, error) {
	if p.format == EMF && id&0x80000000 != 0 {
		n := id & 0x7fffffff
		if n == 18 {
			return stockBrushes[0], nil
		}
		if n > 5 {
			return nil, malformed(r.Offset, "stock brush reference")
		}
		return stockBrushes[n], nil
	}
	if p.beyondTable(id) {
		return nil, nil
	}
	if uint64(id) >= uint64(len(p.objects)) || p.objects[id].brush == nil {
		return nil, malformed(r.Offset, fmt.Sprintf("brush object %d", id))
	}
	return p.objects[id].brush, nil
}
