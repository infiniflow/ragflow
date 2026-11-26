// Copied from github.com/mgilbir/gowemf, file playback_records.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"fmt"
	"image/color"
	"math"
)

func (p *player) dispatch(c Command) error {
	r := c.Source
	if r.Format == EMFPlus {
		return p.plusDispatch(c)
	}
	if c.HasObjectID {
		return p.create(r, c.ObjectID, c.Body)
	}
	if r.Format == WMF {
		return p.wmf(c)
	}
	return p.emf(c)
}

func (p *player) wmf(c Command) error {
	r, body := c.Source, c.Body
	switch r.Type & 255 {
	case 0x00, 0x05, 0x35, 0x31, 0x26:
		// EOF, SetRelAbs (ignored by MS-WMF), RealizePalette, mapper flags
		// and escapes.
		return nil
	case 0x34:
		p.selectPalette(body.(Value).Value)
	case 0x37:
		return p.updatePalette(r, body.(Palette), "set")
	case 0x36:
		return p.updatePalette(r, body.(Palette), "animate")
	case 0x39:
		return p.updatePalette(r, body.(Palette), "resize")
	case 0x2e:
		p.dc.textAlign = body.(Value).Value
	case 0x09:
		p.dc.textColor = body.(Value).Value
	case 0x08:
		p.dc.charExtra = body.(SignedValue).Value
	case 0x0a:
		v := body.(TextJustification)
		p.dc.breakExtra, p.dc.breakCount = v.Extra, v.Count
	case 0x1e:
		return p.save(r)
	case 0x27:
		p.restore(body.(SignedValue).Value)
		return nil
	case 0x2d:
		return p.selectObject(r, body.(Value).Value)
	case 0xf0:
		p.deleteObject(body.(Value).Value)
		return nil
	case 0x02:
		p.dc.bkMode = body.(Value).Value
	case 0x03:
		p.setMapMode(body.(Value).Value)
	case 0x04:
		p.dc.rop2 = body.(Value).Value
	case 0x06:
		p.setPolyFill(body.(Value).Value)
	case 0x07:
		p.dc.stretchMode = body.(Value).Value
	case 0x49:
		p.dc.layout = body.(Value).Value
	case 0x01:
		p.dc.bkColor = body.(Value).Value
	case 0x0b:
		p.dc.windowOrg = body.(PointRecord).Point
	case 0x0c:
		p.setExtent(true, body.(PointRecord).Point)
	case 0x0d:
		p.dc.viewportOrg = body.(PointRecord).Point
	case 0x0e:
		p.setExtent(false, body.(PointRecord).Point)
	case 0x0f:
		v := body.(PointRecord).Point
		p.dc.windowOrg = Point{p.dc.windowOrg.X + v.X, p.dc.windowOrg.Y + v.Y}
	case 0x11:
		v := body.(PointRecord).Point
		p.dc.viewportOrg = Point{p.dc.viewportOrg.X + v.X, p.dc.viewportOrg.Y + v.Y}
	case 0x10, 0x12:
		p.scaleExtent(r.Type&255 == 0x10, body.(Scale))
	case 0x14:
		return p.moveTo(r, body.(PointRecord).Point)
	case 0x13:
		return p.lineTo(c, body.(PointRecord).Point)
	case 0x20:
		return p.offsetClip(r, body.(PointRecord).Point)
	case 0x15, 0x16:
		op := ClipIntersect
		if r.Type&255 == 0x15 {
			op = ClipDifference
		}
		return p.clipRect(r, body.(RectRecord).Rect, op)
	case 0x18:
		return p.drawShape(c, shapeEllipse, body.(RectRecord).Rect, Point{}, Point{}, Point{})
	case 0x1b:
		return p.drawShape(c, shapeRectangle, body.(RectRecord).Rect, Point{}, Point{}, Point{})
	case 0x1c:
		v := body.(RoundRect)
		return p.drawShape(c, shapeRoundRect, v.Rect, v.Corner, Point{}, Point{})
	case 0x17, 0x1a, 0x30:
		v := body.(Arc)
		kind := map[uint32]int{0x17: shapeArc, 0x1a: shapePie, 0x30: shapeChord}[r.Type&255]
		return p.drawShape(c, kind, v.Rect, Point{}, v.Start, v.End)
	case 0x1f:
		return p.setPixel(c, body.(Pixel))
	case 0x24:
		return p.poly(c, polyPolygon, body.(Poly))
	case 0x25:
		return p.poly(c, polyPolyline, body.(Poly))
	case 0x38:
		return p.poly(c, polyPolygon, body.(Poly))
	case 0x32:
		return p.text(c, body.(Text), false)
	case 0x40, 0x41, 0x43, 0x33, 0x1d:
		v := body.(PackedDIBTransfer)
		b := blit{r: r, rop: v.RasterOperation, dest: v.Destination, destSize: v.DestinationSize, src: v.Source, srcSize: v.SourceSize, packed: v.DIB, hasBitmap: !v.DeviceSource && len(v.DIB) != 0, usage: v.Usage, colorState: c.ColorState}
		if v.DeviceSource {
			// The no-bitmap DibBitBlt/DibStretchBlt forms copy from the
			// playback surface itself for source raster operations.
			b.deviceBitmapWhy = "device-to-device bitmap transfer"
		}
		switch r.Type & 255 {
		case 0x33:
			b.rop, b.deviceSize, b.lowerLeftOrigin, b.scanned = 0x00cc0020, true, true, true
			b.startScan, b.scans = v.StartScan, v.Scans
		case 0x43:
			b.lowerLeftOrigin = true
		case 0x1d:
			b.hasBitmap = false
		}
		return p.blit(b)
	case 0x22, 0x23:
		v := body.(Bitmap16Transfer)
		why := "device-dependent Bitmap16 transfer"
		if v.DeviceSource {
			why = "device-to-device bitmap transfer"
		}
		return p.blit(blit{r: r, rop: v.RasterOperation, dest: v.Destination, destSize: v.DestinationSize, src: v.Source, srcSize: v.SourceSize, hasBitmap: !v.DeviceSource, deviceBitmapWhy: why, colorState: c.ColorState})
	case 0x2c:
		// ivy: Stream passes a slot without an object, which selects no
		// region and removes the clipping region.
		if id := body.(Value).Value; uint64(id) < uint64(len(p.objects)) && p.objects[id].empty() {
			p.dc.clip = nil
			return nil
		}
		rects, err := p.regionObject(r, body.(Value).Value)
		if err != nil {
			return err
		}
		return p.selectRegionClip(r, rects)
	case 0x21:
		return p.text(c, body.(Text), false)
	case 0x19, 0x48:
		return p.unsupported(r, "flood fill")
	case 0x28, 0x29:
		v := body.(RegionPaint)
		rects, err := p.regionObject(r, v.Region)
		if err != nil {
			return err
		}
		brush, err := p.brushObject(r, v.Brush)
		if err != nil {
			return err
		}
		var frame *Point
		if r.Type&255 == 0x29 {
			frame = &v.Frame
		}
		return p.paintRegion(c, rects, brush, frame)
	case 0x2b:
		rects, err := p.regionObject(r, body.(Value).Value)
		if err != nil {
			return err
		}
		return p.paintRegion(c, rects, p.dc.brush.brush, nil)
	case 0x2a:
		// ivy: a backend that reads its destination inverts the region.
		rects, err := p.regionObject(r, body.(Value).Value)
		if err != nil {
			return err
		}
		return p.invertRegion(c, rects)
	default:
		return p.unsupported(r, fmt.Sprintf("WMF record 0x%04x", r.Type))
	}
	return nil
}

func (p *player) emf(c Command) error {
	r, body := c.Source, c.Body
	switch r.Type {
	case EMRHeader, EMREOF, EMRComment, EMRSetMapperFlags, EMRRealizePalette,
		EMRSetICMMode, EMRCreateColorSpace, EMRCreateColorSpaceW, EMRSetColorSpace,
		EMRDeleteColorSpace, EMRSetColorAdjustment, EMRSetICMProfileA,
		EMRSetICMProfileW, EMRColorMatchToTargetW:
		// Color state is not drawn here; ColorState is checked by every
		// drawing operation.
		return nil
	case EMRSelectPalette:
		p.selectPalette(body.(Value).Value)
	case EMRSetPaletteEntries:
		return p.updatePalette(r, body.(Palette), "set")
	case EMRResizePalette:
		return p.updatePalette(r, body.(Palette), "resize")
	case EMRColorCorrectPalette:
		if pal := p.palette(body.(Palette).Handle); pal != nil {
			pal.corrected = true
		}
	case EMRSetTextAlign:
		p.dc.textAlign = body.(Value).Value
	case EMRSetTextColor:
		p.dc.textColor = body.(Value).Value
	case EMRSetTextJustification:
		v := body.(TextJustification)
		p.dc.breakExtra, p.dc.breakCount = v.Extra, v.Count
	case EMRSaveDC:
		return p.save(r)
	case EMRRestoreDC:
		p.restore(body.(SignedValue).Value)
		return nil
	case EMRSelectObject:
		return p.selectObject(r, body.(Value).Value)
	case EMRDeleteObject:
		p.deleteObject(body.(Value).Value)
		return nil
	case EMRSetBkMode:
		p.dc.bkMode = body.(Value).Value
	case EMRSetBkColor:
		p.dc.bkColor = body.(Value).Value
	case EMRSetMapMode:
		p.setMapMode(body.(Value).Value)
	case EMRSetROP2:
		p.dc.rop2 = body.(Value).Value
	case EMRSetPolyFillMode:
		p.setPolyFill(body.(Value).Value)
	case EMRSetStretchBltMode:
		p.dc.stretchMode = body.(Value).Value
	case EMRSetLayout:
		p.dc.layout = body.(Value).Value
	case EMRSetArcDirection:
		if v := body.(Value).Value; v == 1 || v == 2 {
			p.dc.arcDir = v
		}
	case EMRSetMiterLimit:
		p.dc.miterLimit = body.(FloatValue).Value
	case EMRSetBrushOrgEx:
		p.dc.brushOrg = body.(PointRecord).Point
	case EMRSetWindowOrgEx:
		p.dc.windowOrg = body.(PointRecord).Point
	case EMRSetViewportOrgEx:
		p.dc.viewportOrg = body.(PointRecord).Point
	case EMRSetWindowExtEx:
		p.setExtent(true, body.(PointRecord).Point)
	case EMRSetViewportExtEx:
		p.setExtent(false, body.(PointRecord).Point)
	case EMRScaleWindowExtEx, EMRScaleViewportExtEx:
		p.scaleExtent(r.Type == EMRScaleWindowExtEx, body.(Scale))
	case EMRSetWorldTransform:
		return p.setWorld(r, body.(Transform).Matrix)
	case EMRModifyWorldTransform:
		v := body.(Transform)
		switch v.Mode {
		case 1:
			p.dc.world = Identity()
		case 2:
			return p.setWorld(r, v.Matrix.Then(p.dc.world))
		case 3:
			return p.setWorld(r, p.dc.world.Then(v.Matrix))
		case 4:
			return p.setWorld(r, v.Matrix)
		}
	case EMRMoveToEx:
		return p.moveTo(r, body.(PointRecord).Point)
	case EMRLineTo:
		return p.lineTo(c, body.(PointRecord).Point)
	case EMRSetPixelV:
		return p.setPixel(c, body.(Pixel))
	case EMRRectangle:
		return p.drawShape(c, shapeRectangle, body.(RectRecord).Rect, Point{}, Point{}, Point{})
	case EMREllipse:
		return p.drawShape(c, shapeEllipse, body.(RectRecord).Rect, Point{}, Point{}, Point{})
	case EMRRoundRect:
		v := body.(RoundRect)
		return p.drawShape(c, shapeRoundRect, v.Rect, v.Corner, Point{}, Point{})
	case EMRArc, EMRChord, EMRPie:
		v := body.(Arc)
		kind := map[uint32]int{EMRArc: shapeArc, EMRChord: shapeChord, EMRPie: shapePie}[r.Type]
		return p.drawShape(c, kind, v.Rect, Point{}, v.Start, v.End)
	case EMRArcTo:
		return p.arcTo(c, body.(Arc))
	case EMRAngleArc:
		return p.angleArc(c, body.(AngleArc))
	case EMRPolygon, EMRPolygon16, EMRPolyPolygon, EMRPolyPolygon16:
		return p.poly(c, polyPolygon, body.(Poly))
	case EMRPolyline, EMRPolyline16, EMRPolyPolyline, EMRPolyPolyline16:
		return p.poly(c, polyPolyline, body.(Poly))
	case EMRPolyBezier, EMRPolyBezier16:
		return p.poly(c, polyBezier, body.(Poly))
	case EMRPolyBezierTo, EMRPolyBezierTo16:
		return p.poly(c, polyBezierTo, body.(Poly))
	case EMRPolylineTo, EMRPolylineTo16:
		return p.poly(c, polylineTo, body.(Poly))
	case EMRPolyDraw, EMRPolyDraw16:
		return p.poly(c, polyDraw, body.(Poly))
	case EMRBeginPath:
		p.path = &pathBuilder{limit: p.options.MaxPathPoints}
		p.constructing = true
	case EMREndPath:
		p.constructing = false
		return p.pathError(r)
	case EMRCloseFigure:
		p.path.close()
	case EMRAbortPath:
		p.path, p.constructing = nil, false
	case EMRFlattenPath:
		p.path.flatten()
		return p.pathError(r)
	case EMRWidenPath:
		return p.unsupported(r, "WidenPath")
	case EMRFillPath, EMRStrokePath, EMRStrokeAndFillPath:
		path := p.path
		p.path = nil
		return p.renderPath(c, path, r.Type != EMRStrokePath, r.Type != EMRFillPath)
	case EMRSelectClipPath:
		path := p.path
		p.path = nil
		mode := body.(Value).Value
		if mode < 1 || mode > 5 {
			return malformed(r.Offset, "clip path mode")
		}
		return p.combineClip(r, ClipOp(mode), path.closeAll(), p.fillRule())
	case EMRIntersectClipRect:
		return p.clipRect(r, body.(RectRecord).Rect, ClipIntersect)
	case EMRExcludeClipRect:
		return p.clipRect(r, body.(RectRecord).Rect, ClipDifference)
	case EMROffsetClipRgn:
		return p.offsetClip(r, body.(PointRecord).Point)
	case EMRSetMetaRgn:
		p.setMetaRegion()
	case EMRExtSelectClipRgn:
		return p.selectClipRegion(r, body.(Region))
	case EMRBitBlt, EMRStretchBlt, EMRAlphaBlend, EMRTransparentBlt:
		v := body.(RasterTransfer)
		t := v.SourceTransform
		b := blit{r: r, rop: v.Operation, operation: v.Operation, dest: v.Destination, destSize: v.DestinationSize, src: v.Source, srcSize: v.SourceSize, info: v.Info, bits: v.Bits, hasBitmap: len(v.Info) != 0, usage: v.Usage, sourceTransform: &t, colorState: c.ColorState}
		if r.Type == EMRAlphaBlend {
			b.kind = blitAlpha
		} else if r.Type == EMRTransparentBlt {
			b.kind = blitTransparent
		}
		return p.blit(b)
	case EMRStretchDIBits:
		v := body.(BitmapTransfer)
		return p.blit(blit{r: r, rop: v.RasterOperation, dest: v.Destination, destSize: v.DestinationSize, src: v.Source, srcSize: v.SourceSize, info: v.Info, bits: v.Bits, hasBitmap: len(v.Info) != 0, usage: v.Usage, lowerLeftOrigin: true, colorState: c.ColorState})
	case EMRSetDIBitsToDevice:
		v := body.(BitmapTransfer)
		return p.blit(blit{r: r, rop: 0x00cc0020, dest: v.Destination, src: v.Source, srcSize: v.SourceSize, info: v.Info, bits: v.Bits, hasBitmap: len(v.Info) != 0, usage: v.Usage, deviceSize: true, lowerLeftOrigin: true, scanned: true, startScan: v.StartScan, scans: v.Scans, colorState: c.ColorState})
	case EMRMaskBlt, EMRPlgBlt:
		v := body.(MaskedBitmapTransfer)
		t := v.SourceTransform
		b := blit{r: r, rop: v.ROP4 & 0x00ffffff, dest: v.Destination, destSize: v.DestinationSize, src: v.Source, srcSize: v.SourceSize, info: v.SourceInfo, bits: v.SourceBits, hasBitmap: len(v.SourceInfo) != 0, usage: v.SourceUsage, sourceTransform: &t, colorState: c.ColorState, maskPresent: len(v.MaskInfo) != 0}
		if r.Type == EMRPlgBlt {
			b.rop = 0x00cc0020 // PlgBlt has no raster operation; it copies source pixels.
			b.plg = []Point{v.DestinationPoints.At(0), v.DestinationPoints.At(1), v.DestinationPoints.At(2)}
		}
		return p.blit(b)
	case EMRExtTextOutA, EMRExtTextOutW, EMRSmallTextOut:
		v := body.(Text)
		return p.text(c, v, v.Unicode)
	case EMRPolyTextOutA, EMRPolyTextOutW:
		return p.polyText(c, body.(PolyText), r.Type == EMRPolyTextOutW)
	case EMRExtFloodFill:
		return p.unsupported(r, "flood fill")
	case EMRFillRgn, EMRFrameRgn, EMRPaintRgn:
		v := body.(EMFRegionPaint)
		brush := p.dc.brush.brush
		if v.HasBrush {
			var err error
			if brush, err = p.brushObject(r, v.Brush); err != nil || brush == nil {
				return err
			}
		}
		var frame *Point
		if r.Type == EMRFrameRgn {
			frame = &v.Frame
		}
		return p.paintRegion(c, regionRects(v.Region), brush, frame)
	case EMRInvertRgn:
		// ivy: a backend that reads its destination inverts the region.
		return p.invertRegion(c, regionRects(body.(EMFRegionPaint).Region))
	case EMRGradientFill:
		return p.gradient(c, body.(Gradient))
	default:
		return p.unsupported(r, fmt.Sprintf("EMF record %d", r.Type))
	}
	return nil
}

func (p *player) save(r Record) error {
	// Stream enforces MaxSavedStates before delivering SaveDC.
	p.saved = append(p.saved, p.dc)
	return nil
}

func (p *player) setPolyFill(v uint32) {
	if v == 1 || v == 2 {
		p.dc.polyFill = v
	}
}
func (p *player) fillRule() FillRule {
	if p.dc.polyFill == 2 {
		return NonZero
	}
	return EvenOdd
}

func (p *player) scaleExtent(window bool, s Scale) {
	e := p.dc.viewportExt
	if window {
		e = p.dc.windowExt
	}
	p.setExtent(window, Point{e.X * float64(s.XNum) / float64(s.XDenom), e.Y * float64(s.YNum) / float64(s.YDenom)})
}

func (p *player) setWorld(r Record, m Matrix) error {
	if !m.Finite() {
		return malformed(r.Offset, "non-finite world transform")
	}
	p.dc.world = m
	return nil
}

func (p *player) pathError(r Record) error {
	if p.path != nil && p.path.err {
		return failure(r.Offset, "path points", ErrLimit)
	}
	return nil
}

// target returns the builder receiving geometry: the open path bracket, or a
// new builder for immediate drawing.
func (p *player) target() *pathBuilder {
	if p.constructing {
		return p.path
	}
	return &pathBuilder{limit: p.options.MaxPathPoints}
}

// moveTo starts a new figure in a path bracket.
func (p *player) moveTo(r Record, v Point) error {
	p.dc.position = v
	if !p.constructing {
		return nil
	}
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	p.path.moveTo(m.Apply(v))
	return p.pathError(r)
}

// startFigure ensures a figure continues from the current position.
func (p *player) startFigure(s shape) {
	if !s.b.open {
		s.moveTo(p.dc.position)
	}
}

func (p *player) lineTo(c Command, v Point) error {
	m, err := p.toDestination(c.Source)
	if err != nil {
		return err
	}
	b := p.target()
	s := shape{b, m}
	p.startFigure(s)
	s.lineTo(v)
	p.dc.position = v
	return p.finish(c, b, false, true)
}

const (
	polyPolygon = iota
	polyPolyline
	polyBezier
	polyBezierTo
	polylineTo
	polyDraw
)

func (p *player) poly(c Command, kind int, v Poly) error {
	m, err := p.toDestination(c.Source)
	if err != nil {
		return err
	}
	b := p.target()
	s := shape{b, m}
	n := v.Points.Len()
	switch kind {
	case polyPolygon, polyPolyline:
		counts := []int{n}
		if v.Counts.Len() != 0 {
			counts = counts[:0]
			for i := 0; i < v.Counts.Len(); i++ {
				counts = append(counts, int(v.Counts.At(i)))
			}
		}
		i := 0
		for _, k := range counts {
			if k == 0 {
				continue
			}
			s.moveTo(v.Points.At(i))
			for j := 1; j < k; j++ {
				s.lineTo(v.Points.At(i + j))
			}
			if kind == polyPolygon {
				b.close()
			}
			// Polyline and Polygon neither use nor update the current
			// position; a later "To" record starts a new figure there.
			b.open = false
			i += k
		}
		return p.finish(c, b, kind == polyPolygon, true)
	case polyBezier:
		s.moveTo(v.Points.At(0))
		for i := 1; i+2 < n; i += 3 {
			s.cubicTo(v.Points.At(i), v.Points.At(i+1), v.Points.At(i+2))
		}
		b.open = false
	case polyBezierTo:
		p.startFigure(s)
		for i := 0; i+2 < n; i += 3 {
			s.cubicTo(v.Points.At(i), v.Points.At(i+1), v.Points.At(i+2))
		}
		p.dc.position = v.Points.At(n - 1)
	case polylineTo:
		if n == 0 {
			return nil
		}
		p.startFigure(s)
		for i := 0; i < n; i++ {
			s.lineTo(v.Points.At(i))
		}
		p.dc.position = v.Points.At(n - 1)
	case polyDraw:
		if err := p.polyDraw(c.Source, s, v); err != nil {
			return err
		}
	}
	return p.finish(c, b, false, true)
}

// polyDraw follows MS-EMF 2.1.26 point types. A Bézier needs three
// consecutive PT_BEZIERTO points; PT_CLOSEFIGURE may accompany a LineTo or the
// last point of a Bézier.
func (p *player) polyDraw(r Record, s shape, v Poly) error {
	n := v.Points.Len()
	for i := 0; i < n; i++ {
		t := v.Types[i]
		switch t &^ 1 {
		case 6:
			if t&1 != 0 {
				return malformed(r.Offset, "PolyDraw close flag on MoveTo")
			}
			s.moveTo(v.Points.At(i))
		case 2:
			p.startFigure(s)
			s.lineTo(v.Points.At(i))
		case 4:
			if i+2 >= n || v.Types[i]&1 != 0 || v.Types[i+1]&^1 != 4 || v.Types[i+1]&1 != 0 || v.Types[i+2]&^1 != 4 {
				return malformed(r.Offset, "PolyDraw Bézier points")
			}
			p.startFigure(s)
			s.cubicTo(v.Points.At(i), v.Points.At(i+1), v.Points.At(i+2))
			i += 2
			t = v.Types[i]
		default:
			return malformed(r.Offset, "PolyDraw point type")
		}
		p.dc.position = v.Points.At(i)
		if t&1 != 0 {
			s.b.close()
		}
	}
	return nil
}

func (p *player) arcTo(c Command, v Arc) error {
	m, err := p.toDestination(c.Source)
	if err != nil {
		return err
	}
	b := p.target()
	p.startFigure(shape{b, m})
	bs := p.boxSpace(b, m)
	end := p.closedShape(bs, shapeArcTo, v.Rect, Point{}, v.Start, v.End, Point{})
	// The box-space mapping is an invertible axis-aligned scale or identity.
	inv := bs.toBox
	p.dc.position = Point{(end.X - inv.Dx) / inv.M11, (end.Y - inv.Dy) / inv.M22}
	return p.finish(c, b, false, true)
}

// angleArc draws a line from the current position to the start of a circular
// arc. AngleArc is not affected by the arc direction; positive sweeps are
// counterclockwise. Sweeps beyond one turn retrace the circle and are bounded.
func (p *player) angleArc(c Command, v AngleArc) error {
	m, err := p.toDestination(c.Source)
	if err != nil {
		return err
	}
	if !finite(v.StartAngle) || !finite(v.SweepAngle) {
		return malformed(c.Source.Offset, "AngleArc angle")
	}
	b := p.target()
	s := shape{b, m}
	rad := Point{float64(v.Radius), float64(v.Radius)}
	a := v.StartAngle * math.Pi / 180
	sweep := v.SweepAngle * math.Pi / 180
	if math.Abs(sweep) > 4*math.Pi {
		sweep = math.Copysign(2*math.Pi+math.Mod(math.Abs(sweep), 2*math.Pi), sweep)
	}
	p.startFigure(s)
	s.lineTo(ellipsePoint(v.Center, rad, a))
	s.arc(v.Center, rad, a, sweep)
	p.dc.position = ellipsePoint(v.Center, rad, a+sweep)
	return p.finish(c, b, false, true)
}

func (p *player) drawShape(c Command, kind int, box Rect, corner, start, end Point) error {
	m, err := p.toDestination(c.Source)
	if err != nil {
		return err
	}
	b := p.target()
	bs := p.boxSpace(b, m)
	var inset Point
	if pen := p.dc.pen.pen; pen != nil && !pen.null && pen.geometric && pen.style == 6 {
		// The pen is transformed with the box space's axes.
		inset = Point{pen.width * math.Abs(bs.toBox.M11), pen.width * math.Abs(bs.toBox.M22)}
	}
	p.closedShape(bs, kind, box, corner, start, end, inset)
	// Arc leaves its figure open but does not update the current position.
	b.open = false
	return p.finish(c, b, kind != shapeArc, true)
}

// setPixel is not a path operation, so it draws even inside a path bracket.
func (p *player) setPixel(c Command, v Pixel) error {
	if err := p.drawable(c.Source, c.ColorState); err != nil {
		return err
	}
	col, err := p.color(c.Source, v.Color)
	if err != nil {
		return err
	}
	d := p.dc.world.Then(p.dc.pageMatrix())
	if !d.Finite() {
		return malformed(c.Source.Offset, "non-finite playback transform")
	}
	o := d.Apply(v.Point)
	o = Point{math.Floor(o.X), math.Floor(o.Y)}
	b := pathBuilder{limit: 4}
	s := shape{&b, p.base}
	s.moveTo(o)
	s.lineTo(Point{o.X + 1, o.Y})
	s.lineTo(Point{o.X + 1, o.Y + 1})
	s.lineTo(Point{o.X, o.Y + 1})
	b.close()
	return p.backend.FillPath(b.path, NonZero, Paint{Kind: PaintSolid, Color: col}, p.currentClip())
}

// finish draws immediate geometry. Path-bracket geometry is retained.
func (p *player) finish(c Command, b *pathBuilder, fill, stroke bool) error {
	if b.err {
		return failure(c.Source.Offset, "path points", ErrLimit)
	}
	if p.constructing {
		return nil
	}
	return p.render(c, b.path, fill, stroke)
}

func (p *player) renderPath(c Command, b *pathBuilder, fill, stroke bool) error {
	path := b.path
	if fill {
		path = b.closeAll()
	}
	return p.render(c, path, fill, stroke)
}

func (p *player) render(c Command, path Path, fill, stroke bool) error {
	if len(path.Verbs) == 0 {
		return nil
	}
	if err := p.drawable(c.Source, c.ColorState); err != nil {
		return err
	}
	m, err := p.toDestination(c.Source)
	if err != nil {
		return err
	}
	clip := p.currentClip()
	// ivy: a backend that reads its destination fills with a mix mode that
	// combines the brush with the destination, such as R2_MASKPEN.
	rb, raster := backendAs[RasterBackend](p.backend)
	if fill && raster && rop2ReadsDestination(p.dc.rop2) {
		paint, err := p.brushPaint(c.Source, p.dc.brush.brush, m, false)
		if err != nil {
			return err
		}
		if paint != nil {
			if err := rb.DrawRaster(RasterDraw{Operation: rop2Raster(p.dc.rop2), Area: path, Pattern: paint}, clip); err != nil {
				return err
			}
		}
		fill = false
	}
	if fill {
		paint, err := p.brushPaint(c.Source, p.dc.brush.brush, m, true)
		if err != nil {
			return err
		}
		if paint != nil {
			if err := p.backend.FillPath(path, p.fillRule(), *paint, clip); err != nil {
				return err
			}
		}
	}
	if stroke {
		// ivy: a backend that reads its destination strokes with a mix mode
		// that combines the pen with the destination, such as R2_XORPEN.
		readsDst := raster && rop2ReadsDestination(p.dc.rop2)
		s, err := p.stroke(c.Source, m, !readsDst)
		if err != nil {
			return err
		}
		if s != nil && readsDst {
			if s.Gap != nil {
				// the gaps of a styled pen in OPAQUE mode take the
				// background color, which the operation must not combine
				return p.unsupported(c.Source, fmt.Sprintf("ROP2 %d on a styled pen with an opaque background", p.dc.rop2))
			}
			return rb.DrawRaster(RasterDraw{Operation: rop2Raster(p.dc.rop2), Area: path, Pattern: &s.Paint, Stroke: s}, clip)
		}
		if s != nil {
			return p.backend.StrokePath(path, *s, clip)
		}
	}
	return nil
}

// drawable rejects state that would make any output unfaithful.
func (p *player) drawable(r Record, cs *ColorPlaybackState) error {
	if p.dc.layout&1 != 0 {
		return p.unsupported(r, "right-to-left layout")
	}
	if cs != nil && cs.ICMMode == 2 && (!cs.Source.IsSRGB() || cs.OutputProfile != nil || cs.Proof != nil) {
		return p.unsupported(r, "ICM color conversion")
	}
	return nil
}

// rop2 applies the foreground mix mode. Only modes independent of the
// destination are supported; R2_NOP draws nothing.
func (p *player) rop2(r Record, paint *Paint) (*Paint, error) {
	invert := func(c color.NRGBA) color.NRGBA { return color.NRGBA{^c.R, ^c.G, ^c.B, c.A} }
	set := func(c color.NRGBA) {
		paint.Color = c
		if paint.Background != nil {
			bg := c
			paint.Background = &bg
		}
	}
	switch p.dc.rop2 {
	case 13:
		return paint, nil
	case 11:
		return nil, nil
	}
	if paint.Kind == PaintPattern {
		return nil, p.unsupported(r, fmt.Sprintf("ROP2 %d with a pattern brush", p.dc.rop2))
	}
	switch p.dc.rop2 {
	case 1:
		set(color.NRGBA{A: 255})
	case 16:
		set(color.NRGBA{255, 255, 255, 255})
	case 4:
		paint.Color = invert(paint.Color)
		if paint.Background != nil {
			bg := invert(*paint.Background)
			paint.Background = &bg
		}
	default:
		return nil, p.unsupported(r, fmt.Sprintf("ROP2 %d", p.dc.rop2))
	}
	return paint, nil
}

// ivy: rop2ReadsDestination reports whether the mix mode rop2 (MS-WMF
// 2.1.1.4) combines the pen or the brush with the destination. R2_NOP (11)
// leaves the destination as it is, and draws nothing.
func rop2ReadsDestination(rop2 uint32) bool {
	return rop2 >= 1 && rop2 <= 16 && rop2 != 11 && rop2Raster(rop2).UsesDestination()
}

// ivy: rop2Raster returns the ternary raster operation of the mix mode
// rop2, with the pen or the brush as the pattern. The mix mode R2_x is the
// truth table x-1 of the bits P<<1|D.
func rop2Raster(rop2 uint32) RasterOperation {
	var op RasterOperation
	for i := uint(0); i < 8; i++ {
		pd := i>>2&1<<1 | i&1
		if (rop2-1)>>pd&1 != 0 {
			op |= 1 << i
		}
	}
	return op
}

// brushPaint resolves a brush. The returned paint is nil for BS_NULL or when
// the mix mode draws nothing. Hatch and pattern grids are device pixels
// anchored at the logical brush origin (MS-EMF 2.3.11.12).
func (p *player) brushPaint(r Record, g *gdiBrush, m Matrix, applyROP2 bool) (*Paint, error) {
	if g == nil || g.style == 1 {
		return nil, nil
	}
	if g.unsupportedWhy != "" {
		return nil, p.unsupported(r, g.unsupportedWhy)
	}
	paint := &Paint{Kind: PaintSolid}
	var err error
	switch g.style {
	case 0:
		paint.Color, err = p.color(r, g.color)
	case 2:
		switch {
		case g.hatch <= 5:
			paint.Kind, paint.Hatch = PaintHatch, g.hatch
			paint.Color, err = p.color(r, g.color)
			if err == nil && p.dc.bkMode == 2 {
				var bg color.NRGBA
				bg, err = p.color(r, p.dc.bkColor)
				paint.Background = &bg
			}
		case g.hatch == 6:
			paint.Color, err = p.color(r, g.color)
		case g.hatch == 10:
			paint.Color, err = p.color(r, p.dc.bkColor)
		default:
			return nil, p.unsupported(r, fmt.Sprintf("hatch style %d", g.hatch))
		}
	case 3:
		if g.monoDIB && g.mono == nil {
			mono, err := p.monoBrushPattern(r, g)
			if err != nil {
				return nil, err
			}
			g.mono = mono
		}
		fg, err := p.color(r, p.dc.textColor)
		if err != nil {
			return nil, err
		}
		bg, err := p.color(r, p.dc.bkColor)
		if err != nil {
			return nil, err
		}
		im, err := g.mono.image(p, r, fg, bg)
		if err != nil {
			return nil, err
		}
		paint.Kind, paint.Pattern = PaintPattern, im
	case 5:
		logical, err := p.logicalPalette(r, g.usage)
		if err != nil {
			return nil, err
		}
		// A DIB_PAL_COLORS pattern takes its colors from the palette selected
		// when it is used, so only DIB_RGB_COLORS patterns are cached.
		pattern := g.pattern
		if pattern == nil || g.usage != 0 {
			var d *DIB
			if g.packed != nil {
				d, err = ParsePackedDIB(g.packed, g.usage, logical, p.options.Images)
			} else {
				d, err = ParseDIB(g.info, g.bits, g.usage, logical, p.options.Images)
			}
			if err == nil {
				err = p.spendPixels(r, d.width, d.height)
			}
			if err == nil {
				pattern, err = d.ImageWithColorTransform(p.options.ColorTransform)
			}
			if err != nil {
				return nil, p.imageError(r, err)
			}
			if g.usage == 0 {
				g.pattern = pattern
			}
		}
		paint.Kind, paint.Pattern = PaintPattern, opaqueImage{pattern}
	default:
		return nil, p.unsupported(r, fmt.Sprintf("brush style %d", g.style))
	}
	if err != nil {
		return nil, err
	}
	if paint.Kind != PaintSolid {
		o := p.dc.world.Then(p.dc.pageMatrix()).Apply(p.dc.brushOrg)
		paint.PatternTransform = Matrix{M11: 1, M22: 1, Dx: o.X, Dy: o.Y}.Then(p.base)
		if !paint.PatternTransform.Finite() {
			return nil, malformed(r.Offset, "non-finite brush origin")
		}
	}
	if !applyROP2 {
		return paint, nil
	}
	return p.rop2(r, paint)
}

// stroke resolves the selected pen. A geometric pen is a circle of its
// width in logical units, transformed with the world and page mapping: an
// anisotropic mapping draws it as an ellipse, as Windows does in EMF and WMF
// (ORACLES.md).
//
// ivy: applyROP2 applies the mix mode to the paint of the pen, as
// brushPaint does.
func (p *player) stroke(r Record, m Matrix, applyROP2 bool) (*Stroke, error) {
	g := p.dc.pen.pen
	if g == nil || g.null {
		return nil, nil
	}
	if g.unsupportedWhy != "" {
		return nil, p.unsupported(r, g.unsupportedWhy)
	}
	var paint *Paint
	var err error
	if g.brush != nil {
		paint, err = p.brushPaint(r, g.brush, m, false)
	} else {
		paint = &Paint{Kind: PaintSolid}
		paint.Color, err = p.color(r, g.color)
	}
	if err != nil || paint == nil {
		return nil, err
	}
	if applyROP2 {
		if paint, err = p.rop2(r, paint); err != nil || paint == nil {
			return nil, err
		}
	}
	s := &Stroke{Paint: *paint, Cap: g.cap, Join: g.join, MiterLimit: p.dc.miterLimit, Transform: Identity(), PixelCenter: Point{p.base.M11 / 2, p.base.M22 / 2}}
	switch g.style {
	case 0, 6:
		s.Dash = DashSolid
	case 7:
		s.Dash, s.Dashes = DashUser, append([]float64(nil), g.dashes...)
	default:
		s.Dash = DashStyle(g.style + 1)
	}
	if s.Dash != DashSolid && p.dc.bkMode == 2 {
		gap := Paint{Kind: PaintSolid}
		if gap.Color, err = p.color(r, p.dc.bkColor); err != nil {
			return nil, err
		}
		s.Gap = &gap
	}
	switch {
	case !g.geometric || g.width <= 0:
		s.Hairline = true
	default:
		s.Width = g.width
		s.Transform = linear(m)
	}
	if !finite(s.Width) {
		return nil, malformed(r.Offset, "non-finite pen width")
	}
	return s, nil
}

func (p *player) clipRect(r Record, v Rect, op ClipOp) error {
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	l, t, rr, b := normalize(v)
	path := pathBuilder{limit: 4}
	s := shape{&path, m}
	s.moveTo(Point{l, t})
	s.lineTo(Point{rr, t})
	s.lineTo(Point{rr, b})
	s.lineTo(Point{l, b})
	path.close()
	return p.combineClip(r, op, path.path, NonZero)
}

// selectClipRegion applies EMR_EXTSELECTCLIPRGN. MS-EMF 2.3.2.2 specifies
// the region in logical units; an omitted RGN_COPY region resets the clip.
func (p *player) selectClipRegion(r Record, v Region) error {
	if v.Mode == 5 && v.Count == 0 && v.Rectangles == nil {
		p.dc.clip = nil
		return nil
	}
	m, err := p.toDestination(r)
	if err != nil {
		return err
	}
	path := pathBuilder{limit: p.options.MaxPathPoints}
	s := shape{&path, m}
	for i := 0; i < int(v.Count); i++ {
		q := v.RectangleAt(i)
		l, t, rr, b := normalize(q)
		s.moveTo(Point{l, t})
		s.lineTo(Point{rr, t})
		s.lineTo(Point{rr, b})
		s.lineTo(Point{l, b})
		path.close()
	}
	if path.err {
		return failure(r.Offset, "clip region points", ErrLimit)
	}
	return p.combineClip(r, ClipOp(v.Mode), path.path, NonZero)
}
