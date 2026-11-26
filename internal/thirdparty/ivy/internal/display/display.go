// Package display is the display list of D10. A page description, such as
// XPS or PDF, emits commands to a Device. The device that draws with okage is
// the main one, and a List records the commands for the tests.
//
// The commands follow the operators of PDF: save and restore the state,
// concatenate a transform, fill and stroke a path, clip, draw an image, fill
// a stencil mask, fill a run of glyphs, and begin and end a group. A group
// has an opacity, a blend mode and a soft mask. A path, a clip, an image and
// a shader are in the coordinates of the user space at the call: the device
// maps them with the current transform.
package display

import (
	"context"
	"fmt"
	"image"
	"strings"
	"unicode/utf8"

	"github.com/xo/okage"
	"ragflow/internal/thirdparty/ivy/internal/font"
)

// Device receives the commands of a page.
type Device interface {
	// Save pushes the transform and the clip.
	Save()
	// Restore pops the transform and the clip that the last Save pushed.
	Restore()
	// Concat multiplies the current transform by t, so that t applies
	// first.
	Concat(t okage.Transform)
	// FillPath fills p with the shader.
	FillPath(p *okage.Path, rule okage.FillRule, shader okage.Shader)
	// StrokePath strokes p with the shader.
	StrokePath(p *okage.Path, stroke okage.Stroke, shader okage.Shader)
	// ClipPath intersects the clip with p.
	ClipPath(p *okage.Path, rule okage.FillRule)
	// DrawImage draws img. t maps the pixels of img, counted from the corner
	// of its bounds, onto the user space. The pixel (x, y) covers the unit
	// square from (x, y) to (x+1, y+1).
	DrawImage(img image.Image, t okage.Transform, opts ImageOptions)
	// FillMask fills the shader through a stencil mask. t places the pixels
	// of mask as DrawImage does. The alpha of each pixel is the coverage.
	FillMask(mask *image.Alpha, t okage.Transform, opts ImageOptions, shader okage.Shader)
	// FillGlyphs fills the outlines of a run of glyphs with the shader.
	FillGlyphs(run GlyphRun, shader okage.Shader)
	// BeginGroup starts a group, which draws to a layer of its own.
	BeginGroup(g Group)
	// EndGroup draws the layer of the group onto its parent, with the
	// opacity, the blend mode and the soft mask of the group.
	EndGroup()
}

// Drawer is a page that draws onto a device of the display list. A writer,
// such as the PDF writer of pdfenc, takes the commands of the page from it
// (D58).
type Drawer interface {
	// Draw draws the page onto d, in points with y down, as Render draws
	// it with the identity transform. res is the resolution, in pixels for
	// each point, of a part that the page draws into a pixmap first, such
	// as a tiling pattern of PDF.
	Draw(ctx context.Context, d Device, res float32) error
}

// ImageOptions are the options of DrawImage and FillMask.
type ImageOptions struct {
	// Opacity multiplies the alpha of the image, from 0 to 1.
	Opacity float32
	// Smooth samples the image with bilinear filtering. Without it, the
	// device takes the nearest pixel, and the pixels of a scaled image stay
	// sharp.
	Smooth bool
}

// Group is a group that BeginGroup starts.
//
// The zero Group draws its layer at an opacity of 0. A caller sets Opacity
// to 1 for a group without transparency.
type Group struct {
	// Opacity is the opacity of the layer, from 0 to 1.
	Opacity float32
	// Blend is the blend mode of the layer onto its parent. The zero value,
	// okage.BlendModeClear, is not a mode of PDF, and a device draws it as
	// okage.BlendModeSourceOver.
	Blend okage.BlendMode
	// Isolated starts the layer transparent, without the backdrop of the
	// parent. Without it, the objects of the group blend with the backdrop
	// of the parent, and the device removes the backdrop from the layer
	// before it draws the layer onto the parent (ISO 32000-2, 11.6.3).
	// This changes the result only for an object with a blend mode other
	// than the normal one.
	Isolated bool
	// Knockout makes each object of the group draw over the backdrop of the
	// group, not over the earlier objects of the group. Where an object
	// covers a pixel, it replaces the earlier objects (ISO 32000-2, 11.4.8).
	// A nested group is one object.
	Knockout bool
	// Mask is the soft mask of the group, or nil.
	Mask *Mask
}

// Mask is a soft mask. Its content is drawn through a device, and each pixel
// gives the coverage of the group: its alpha, or its luminosity over a
// backdrop.
type Mask struct {
	// Type is okage.MaskTypeAlpha or okage.MaskTypeLuminance.
	Type okage.MaskType
	// Backdrop is the color under the content of a luminosity mask. The
	// zero Color is transparent, which gives the same mask as black.
	Backdrop okage.Color
	// Transfer is the transfer function of the mask, or nil. Each value v
	// of the mask becomes Transfer[v].
	Transfer *[256]uint8
	// Content draws the content of the mask onto d. The user space of d is
	// the user space at BeginGroup. A device can call Content at BeginGroup
	// or at EndGroup, and more than once, so each call must draw the same
	// content.
	Content func(d Device)
	// Key identifies the content, or is nil. A device can reuse the mask that
	// it drew for an earlier mask with the same key and the same transform.
	// A Key must be comparable.
	Key any
}

// TextFiller is a device that fills the outlines of glyphs with a finer
// coverage than FillPath gives. An OkageDevice and a List are text fillers.
type TextFiller interface {
	// FillText fills p, the outlines of one or more glyphs, with the
	// shader.
	FillText(p *okage.Path, rule okage.FillRule, shader okage.Shader)
}

// FillText fills p, the outlines of one or more glyphs, with the shader. It
// calls FillText of d when d is a TextFiller, and FillPath of d otherwise. A
// format that draws the outline of each glyph itself, such as PDF, calls it
// in place of FillPath.
func FillText(d Device, p *okage.Path, rule okage.FillRule, shader okage.Shader) {
	if tf, ok := d.(TextFiller); ok {
		tf.FillText(p, rule, shader)
		return
	}
	d.FillPath(p, rule, shader)
}

// StrokeAdjuster is a device that can stroke a path with the automatic
// stroke adjustment of PDF, the entry SA of a graphics state. An OkageDevice
// and a List are stroke adjusters.
type StrokeAdjuster interface {
	// StrokeAdjusted strokes p with the shader, as StrokePath does. It rounds
	// the width of the stroke to whole pixels, at least 1, and moves each
	// horizontal or vertical line onto the grid of the pixels, so that a
	// thin line covers whole pixels.
	StrokeAdjusted(p *okage.Path, stroke okage.Stroke, shader okage.Shader)
}

// StrokeAdjusted strokes p with the stroke adjustment of PDF. It calls
// StrokeAdjusted of d when d is a StrokeAdjuster, and StrokePath of d
// otherwise.
func StrokeAdjusted(d Device, p *okage.Path, stroke okage.Stroke, shader okage.Shader) {
	if sa, ok := d.(StrokeAdjuster); ok {
		sa.StrokeAdjusted(p, stroke, shader)
		return
	}
	d.StrokePath(p, stroke, shader)
}

// Linker is a device that takes the links and the anchors of a page (D58).
// The marks of a page carry them, so a device that draws pixels, such as an
// OkageDevice, does not implement it. A List records them.
type Linker interface {
	// LinkURI makes r, in the user space, a link to uri.
	LinkURI(r okage.Rect, uri string)
	// LinkAnchor makes r, in the user space, a link to the anchor name of
	// the document.
	LinkAnchor(r okage.Rect, name string)
	// Anchor names the point x, y of the page, in the user space.
	Anchor(name string, x, y float32)
}

// Bookmarker is a device that takes the bookmarks of a page (D58). A List
// records them.
type Bookmarker interface {
	// Bookmark adds an entry of the outline at x, y of the page, in the
	// user space, at level, from 1. open shows its children.
	Bookmark(level int, label string, x, y float32, open bool)
}

// LinkURI calls LinkURI of d when d is a Linker.
func LinkURI(d Device, r okage.Rect, uri string) {
	if l, ok := d.(Linker); ok {
		l.LinkURI(r, uri)
	}
}

// LinkAnchor calls LinkAnchor of d when d is a Linker.
func LinkAnchor(d Device, r okage.Rect, name string) {
	if l, ok := d.(Linker); ok {
		l.LinkAnchor(r, name)
	}
}

// Anchor calls Anchor of d when d is a Linker.
func Anchor(d Device, name string, x, y float32) {
	if l, ok := d.(Linker); ok {
		l.Anchor(name, x, y)
	}
}

// Bookmark calls Bookmark of d when d is a Bookmarker.
func Bookmark(d Device, level int, label string, x, y float32, open bool) {
	if b, ok := d.(Bookmarker); ok {
		b.Bookmark(level, label, x, y, open)
	}
}

// PageAnchor returns the name of the anchor at x, y of page n, from 0, in
// points with y down. A format names a destination with it when the file
// gives the destination no name of its own, such as a link to a page of a
// PDF or a DjVu file. The target page calls Anchor with the same name.
func PageAnchor(n int, x, y float32) string {
	return fmt.Sprintf("%s%d-%g-%g", PageAnchorPrefix, n+1, x, y)
}

// PageAnchorPrefix starts the name of each anchor of PageAnchor.
const PageAnchorPrefix = "ivy-page-"

// GlyphRun is a run of glyphs of one font and one size.
type GlyphRun struct {
	Face *font.Face
	// Size is the size of the font, an em, in the units of the user space.
	Size float32
	// Glyphs are the glyphs, with the position of each origin.
	Glyphs []Glyph
	// Transform is applied to each glyph, at the scale of the size, before
	// its position, such as a skew for a slanted face.
	Transform okage.Transform
	// Text is the text of the run, or empty when the format does not know
	// it. A writer of PDF gives it to a reader that searches or copies the
	// text (D58). The okage device ignores it.
	Text string
}

// Glyph is one glyph of a run.
type Glyph struct {
	ID   font.GID
	X, Y float32
	// Start and End are the bytes of the Text of the run that the cluster
	// of the glyph covers. The glyphs of one cluster have the same range.
	// Both are 0 when the run has no text.
	Start, End int
}

// TextRanges reports whether the run has a text of valid UTF-8, and
// whether the range of each glyph is a range of the text that starts and
// ends at the bounds of its characters.
func (run GlyphRun) TextRanges() bool {
	if run.Text == "" || !utf8.ValidString(run.Text) {
		return false
	}
	boundary := func(i int) bool {
		return i == len(run.Text) || (i >= 0 && i < len(run.Text) && utf8.RuneStart(run.Text[i]))
	}
	for _, g := range run.Glyphs {
		if g.Start < 0 || g.Start > g.End || g.End > len(run.Text) || !boundary(g.Start) || !boundary(g.End) {
			return false
		}
	}
	return true
}

// Path returns the outlines of the run as one path, or nil if no glyph has an
// outline.
func (run GlyphRun) Path() *okage.Path {
	pb := okage.NewPathBuilder()
	glyph := run.Transform.PreScale(run.Size, run.Size)
	for _, g := range run.Glyphs {
		outline := run.Face.Outline(g.ID)
		if outline == nil {
			continue
		}
		t := glyph.PostConcat(okage.TransformFromTranslate(g.X, g.Y))
		// Transform changes its path in place, and the outline is shared, so
		// the run transforms a copy
		if p, ok := Clone(outline).Transform(t); ok {
			pb.PushPath(p)
		}
	}
	p, ok := pb.Finish()
	if !ok {
		return nil
	}
	return p
}

// Clone returns a copy of p. A path of okage changes in place when it
// transforms, so a shared path is cloned first.
func Clone(p *okage.Path) *okage.Path {
	pb := okage.NewPathBuilder()
	pb.PushPath(p)
	c, ok := pb.Finish()
	if !ok {
		return p
	}
	return c
}

// RectTransform returns the transform that DrawImage takes to draw an image
// of size into the rectangle dst.
func RectTransform(size image.Point, dst okage.Rect) okage.Transform {
	if size.X <= 0 || size.Y <= 0 {
		return okage.TransformFromTranslate(dst.X(), dst.Y())
	}
	return okage.TransformFromTranslate(dst.X(), dst.Y()).
		PreScale(dst.Width()/float32(size.X), dst.Height()/float32(size.Y))
}

// blendMode returns the blend mode that a device draws for b.
func blendMode(b okage.BlendMode) okage.BlendMode {
	if b == okage.BlendModeClear {
		return okage.BlendModeSourceOver
	}
	return b
}

// List records the commands that it receives, and replays them onto another
// device. Its String lists the commands, one on each line, for the tests.
//
// A List records the content of a soft mask when it receives the group of
// the mask, as a list of its own. A list does not call back into the code
// that recorded it, so goroutines can replay one list onto devices of their
// own at the same time.
type List struct {
	cmds []func(Device)
	ops  []string
	// quiet leaves out the names of the commands, which only String reads.
	quiet bool
}

// NewRecording returns a list that keeps the commands without their names,
// for a page that a program replays: its String is empty.
func NewRecording() *List {
	return &List{quiet: true}
}

// The interfaces that a List implements.
var (
	_ Device         = (*List)(nil)
	_ TextFiller     = (*List)(nil)
	_ StrokeAdjuster = (*List)(nil)
	_ Linker         = (*List)(nil)
	_ Bookmarker     = (*List)(nil)
)

// record keeps one command, and the name that op gives, unless the list is
// quiet.
func (l *List) record(op func() string, cmd func(Device)) {
	if !l.quiet {
		l.ops = append(l.ops, op())
	}
	l.cmds = append(l.cmds, cmd)
}

// Replay sends the commands to d, in order.
func (l *List) Replay(d Device) {
	for _, cmd := range l.cmds {
		cmd(d)
	}
}

// String lists the commands, one on each line.
func (l *List) String() string {
	return strings.Join(l.ops, "\n")
}

// Save satisfies the Device interface.
func (l *List) Save() {
	l.record(func() string { return "save" }, func(d Device) { d.Save() })
}

// Restore satisfies the Device interface.
func (l *List) Restore() {
	l.record(func() string { return "restore" }, func(d Device) { d.Restore() })
}

// Concat satisfies the Device interface.
func (l *List) Concat(t okage.Transform) {
	l.record(func() string { return fmt.Sprintf("concat %v", t) }, func(d Device) { d.Concat(t) })
}

// FillPath satisfies the Device interface.
func (l *List) FillPath(p *okage.Path, rule okage.FillRule, shader okage.Shader) {
	l.record(func() string { return fmt.Sprintf("fill %v %v", bounds(p), rule) }, func(d Device) { d.FillPath(p, rule, shader) })
}

// StrokePath satisfies the Device interface.
func (l *List) StrokePath(p *okage.Path, stroke okage.Stroke, shader okage.Shader) {
	l.record(func() string { return fmt.Sprintf("stroke %v %g", bounds(p), stroke.Width) }, func(d Device) { d.StrokePath(p, stroke, shader) })
}

// StrokeAdjusted satisfies the StrokeAdjuster interface. The replay calls
// StrokeAdjusted of this package with the device of the replay.
func (l *List) StrokeAdjusted(p *okage.Path, stroke okage.Stroke, shader okage.Shader) {
	l.record(func() string { return fmt.Sprintf("stroke adjusted %v %g", bounds(p), stroke.Width) },
		func(d Device) { StrokeAdjusted(d, p, stroke, shader) })
}

// ClipPath satisfies the Device interface.
func (l *List) ClipPath(p *okage.Path, rule okage.FillRule) {
	l.record(func() string { return fmt.Sprintf("clip %v %v", bounds(p), rule) }, func(d Device) { d.ClipPath(p, rule) })
}

// DrawImage satisfies the Device interface.
func (l *List) DrawImage(img image.Image, t okage.Transform, opts ImageOptions) {
	l.record(func() string { return fmt.Sprintf("image %v %v %g", img.Bounds().Size(), t, opts.Opacity) },
		func(d Device) { d.DrawImage(img, t, opts) })
}

// FillMask satisfies the Device interface.
func (l *List) FillMask(mask *image.Alpha, t okage.Transform, opts ImageOptions, shader okage.Shader) {
	l.record(func() string { return fmt.Sprintf("mask %v %v %g", mask.Bounds().Size(), t, opts.Opacity) },
		func(d Device) { d.FillMask(mask, t, opts, shader) })
}

// FillGlyphs satisfies the Device interface.
func (l *List) FillGlyphs(run GlyphRun, shader okage.Shader) {
	l.record(func() string { return fmt.Sprintf("glyphs %d %g", len(run.Glyphs), run.Size) }, func(d Device) { d.FillGlyphs(run, shader) })
}

// FillText satisfies the TextFiller interface. The replay calls FillText
// of this package with the device of the replay.
func (l *List) FillText(p *okage.Path, rule okage.FillRule, shader okage.Shader) {
	l.record(func() string { return fmt.Sprintf("fill text %v %v", bounds(p), rule) }, func(d Device) { FillText(d, p, rule, shader) })
}

// BeginGroup satisfies the Device interface. It records the content of the
// soft mask of g at once, into a list of its own, which the replay draws.
func (l *List) BeginGroup(g Group) {
	if g.Mask != nil && g.Mask.Content != nil {
		sub := &List{quiet: l.quiet}
		g.Mask.Content(sub)
		mask := *g.Mask
		mask.Content = sub.Replay
		g.Mask = &mask
	}
	op := func() string {
		op := fmt.Sprintf("group %g %v", g.Opacity, blendMode(g.Blend))
		if g.Isolated {
			op += " isolated"
		}
		if g.Knockout {
			op += " knockout"
		}
		if g.Mask != nil {
			op += " mask " + g.Mask.Type.String()
		}
		return op
	}
	l.record(op, func(d Device) { d.BeginGroup(g) })
}

// EndGroup satisfies the Device interface.
func (l *List) EndGroup() {
	l.record(func() string { return "end group" }, func(d Device) { d.EndGroup() })
}

// LinkURI satisfies the Linker interface. The replay calls LinkURI of this
// package with the device of the replay.
func (l *List) LinkURI(r okage.Rect, uri string) {
	l.record(func() string { return fmt.Sprintf("link %v %q", rectString(r), uri) }, func(d Device) { LinkURI(d, r, uri) })
}

// LinkAnchor satisfies the Linker interface. The replay calls LinkAnchor of
// this package with the device of the replay.
func (l *List) LinkAnchor(r okage.Rect, name string) {
	l.record(func() string { return fmt.Sprintf("link %v #%q", rectString(r), name) }, func(d Device) { LinkAnchor(d, r, name) })
}

// Anchor satisfies the Linker interface. The replay calls Anchor of this
// package with the device of the replay.
func (l *List) Anchor(name string, x, y float32) {
	l.record(func() string { return fmt.Sprintf("anchor %q %.0f %.0f", name, x, y) }, func(d Device) { Anchor(d, name, x, y) })
}

// Bookmark satisfies the Bookmarker interface. The replay calls Bookmark of
// this package with the device of the replay.
func (l *List) Bookmark(level int, label string, x, y float32, open bool) {
	l.record(func() string { return fmt.Sprintf("bookmark %d %q %.0f %.0f %v", level, label, x, y, open) },
		func(d Device) { Bookmark(d, level, label, x, y, open) })
}

// rectString returns r, rounded, for the list of commands.
func rectString(r okage.Rect) string {
	return fmt.Sprintf("[%.0f %.0f %.0f %.0f]", r.Left(), r.Top(), r.Right(), r.Bottom())
}

// bounds returns the bounds of p, rounded, for the list of commands.
func bounds(p *okage.Path) string {
	if p == nil {
		return "empty"
	}
	b := p.Bounds()
	return fmt.Sprintf("[%.0f %.0f %.0f %.0f]", b.Left(), b.Top(), b.Right(), b.Bottom())
}
