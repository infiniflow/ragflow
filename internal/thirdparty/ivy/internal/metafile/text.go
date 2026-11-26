package metafile

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/xo/okage"
	"github.com/xo/okage/svg"
	gofont "github.com/xo/okage/typesetting/font"
	"github.com/xo/okage/typesetting/shaping"

	"ragflow/internal/thirdparty/ivy"
	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/font"
	"ragflow/internal/thirdparty/ivy/internal/fontdb"
	"ragflow/internal/thirdparty/ivy/internal/gowemf"
)

// maxFaces is the number of realized fonts that the cache keeps.
const maxFaces = 64

// faces is the cache of the realized fonts of all the pictures, by database
// and face. A font of the database is parsed once.
var faces = struct {
	sync.Mutex
	m map[faceKey]*realFace
}{m: make(map[faceKey]*realFace)}

// faceKey identifies a face of a database.
type faceKey struct {
	db *svg.FontDatabase
	id svg.FontID
}

// realFace is a face of a database, with its metrics in ems.
type realFace struct {
	face *font.Face
	// family is the first family name of the face.
	family string
	italic bool
	// ascent and descent are the extents of the face. avg is the average
	// width of a character.
	ascent, descent, avg float32
	// underline and strike are the positions of the lines below and above
	// the baseline, and thick is their thickness.
	underline, strike, thick float32
	// shapeFace is a face of its own for the shaper, which shapeMu guards
	// with the shaper. It is nil if the face cannot be read again.
	shapeMu   *sync.Mutex
	shapeFace *gofont.Face
	shaper    *shaping.HarfbuzzShaper
}

// fontSet chooses the faces of the fonts that the records of a picture ask
// for.
type fontSet struct {
	db *svg.FontDatabase
}

// newFontSet returns the fonts of a picture: db, or the fonts of the
// program when db is nil.
func newFontSet(db *svg.FontDatabase, cfg ivy.Config) *fontSet {
	if db == nil {
		db = fontdb.Of(cfg)
	}
	return &fontSet{db: db}
}

// genericFamily returns the generic family of a font of GDI, from its name or
// its pitch and family (MS-WMF 2.2.2.14).
func genericFamily(req gowemf.FontRequest) svg.FontFamilyKind {
	name := strings.ToLower(req.FaceName)
	switch {
	case strings.Contains(name, "mono"), strings.Contains(name, "courier"), strings.Contains(name, "consol"),
		strings.Contains(name, "lucida console"), req.PitchAndFamily&3 == 1:
		return svg.FontFamilyMonospace
	case strings.Contains(name, "times"), strings.Contains(name, "roman"), strings.Contains(name, "serif") && !strings.Contains(name, "sans"),
		strings.Contains(name, "cambria"), strings.Contains(name, "georgia"), strings.Contains(name, "garamond"):
		return svg.FontFamilySerif
	}
	switch req.PitchAndFamily & 0xf0 {
	case 0x10:
		return svg.FontFamilySerif
	case 0x30:
		return svg.FontFamilyMonospace
	case 0x40:
		return svg.FontFamilyCursive
	case 0x50:
		return svg.FontFamilyFantasy
	}
	switch req.Stock {
	case 10, 11, 16:
		return svg.FontFamilyMonospace
	}
	return svg.FontFamilySansSerif
}

// face returns the face for a font request, or nil if the database has no
// font.
func (fs *fontSet) face(req gowemf.FontRequest) *realFace {
	q := svg.FontQuery{Weight: 400, Stretch: svg.FontStretchNormal, Style: svg.FontStyleNormal}
	if req.Weight > 0 {
		q.Weight = uint16(min(max(req.Weight, 100), 900))
	}
	if req.Italic {
		q.Style = svg.FontStyleItalic
	}
	if name := strings.TrimSpace(req.FaceName); name != "" {
		if id, ok := fontdb.Find(fs.db, name, q.Weight, req.Italic); ok {
			return realize(fs.db, id)
		}
		q.Families = append(q.Families, svg.FontFamily{Kind: svg.FontFamilyNamed, Name: name})
		// a font of Office that the program does not have takes the font
		// with the same metrics of ivy.Config.Fonts, such as Carlito for
		// Calibri (D24)
		if sub, ok := font.Substitute(name); ok {
			q.Families = append(q.Families, svg.FontFamily{Kind: svg.FontFamilyNamed, Name: sub})
		}
	}
	q.Families = append(q.Families, svg.FontFamily{Kind: genericFamily(req)}, svg.FontFamily{Kind: svg.FontFamilySansSerif})
	id, ok := fs.db.Query(q)
	if !ok {
		return nil
	}
	return realize(fs.db, id)
}

// realize returns the realized face id of db, from the cache.
func realize(db *svg.FontDatabase, id svg.FontID) *realFace {
	key := faceKey{db, id}
	faces.Lock()
	defer faces.Unlock()
	if f, ok := faces.m[key]; ok {
		return f
	}
	f := load(db, id)
	if len(faces.m) >= maxFaces {
		clear(faces.m)
	}
	faces.m[key] = f
	return f
}

// load returns the face id of db, with its metrics. The face shares the
// font that db parses once (okage D57). It returns nil if the face cannot be
// read.
func load(db *svg.FontDatabase, id svg.FontID) (f *realFace) {
	defer func() {
		// go-text can panic on a malformed font
		if recover() != nil {
			f = nil
		}
	}()
	info, ok := db.Face(id)
	if !ok {
		return nil
	}
	face, ok := db.NewFace(id, nil)
	if !ok {
		return nil
	}
	upem := float32(face.Upem())
	if upem <= 0 {
		upem = 1000
	}
	f = &realFace{italic: info.Style != svg.FontStyleNormal, ascent: 0.8, descent: 0.2, avg: 0.5}
	if len(info.Families) > 0 {
		f.family = info.Families[0].Name
	}
	if ext, ok := face.FontHExtents(); ok && ext.Ascender-ext.Descender > 0 {
		f.ascent, f.descent = ext.Ascender/upem, -ext.Descender/upem
	}
	if gid, ok := face.NominalGlyph('x'); ok {
		if adv := face.HorizontalAdvance(gid) / upem; adv > 0 {
			f.avg = adv
		}
	}
	f.underline = -face.LineMetric(gofont.UnderlinePosition) / upem
	f.thick = face.LineMetric(gofont.UnderlineThickness) / upem
	f.strike = face.LineMetric(gofont.StrikethroughPosition) / upem
	if f.thick <= 0 {
		f.thick = 0.05
	}
	if f.underline <= 0 {
		f.underline = 0.1
	}
	if f.strike <= 0 {
		f.strike = 0.25
	}
	if src, ok := fontdb.Source(db, id, nil); ok {
		f.face = font.FromFaceSource(face, src)
	} else {
		f.face = font.FromFace(face)
	}
	if sf, ok := db.NewFace(id, nil); ok {
		f.shapeMu, f.shapeFace, f.shaper = new(sync.Mutex), sf, new(shaping.HarfbuzzShaper)
	}
	return f
}

// textFont is the face and the size of the text of a run.
type textFont struct {
	*realFace
	// em is the size of the font in the units of the text space, and
	// scaleX scales its glyphs across, for a font with a width.
	em, scaleX float32
}

// font realizes the font of a run. It returns false if no font is there.
func (b *backend) font(req gowemf.FontRequest) (textFont, bool) {
	f := b.pic.fonts.face(req)
	if f == nil {
		return textFont{}, false
	}
	tf := textFont{realFace: f, scaleX: 1}
	switch h := float32(math.Abs(req.Height)); {
	case req.Height < 0:
		tf.em = h
	case req.Height > 0:
		// the height of the cell, with the internal leading
		tf.em = h / (f.ascent + f.descent)
	default:
		tf.em = 12
	}
	if w := float32(math.Abs(req.Width)); w > 0 && tf.em > 0 {
		tf.scaleX = w / (f.avg * tf.em)
	}
	if !(tf.em > 0) || math.IsInf(float64(tf.em), 0) || !(tf.scaleX > 0) || math.IsInf(float64(tf.scaleX), 0) {
		return textFont{}, false
	}
	return tf, true
}

// glyphs returns the glyph of each element of a run, and its advance in the
// units of the text space. The second unit of a surrogate pair has the glyph
// of its pair and no advance, and draws nothing.
func (tf textFont) glyphs(run gowemf.TextRun) ([]font.GID, []float64, []bool) {
	n := len(run.Text)
	gids := make([]font.GID, n)
	adv := make([]float64, n)
	draw := make([]bool, n)
	for i := 0; i < n; i++ {
		var gid font.GID
		if run.Glyphs {
			gid = font.GID(run.Text[i])
		} else {
			r := rune(run.Text[i])
			if utf16.IsSurrogate(r) && i+1 < n {
				if p := utf16.DecodeRune(r, rune(run.Text[i+1])); p != '�' {
					r = p
				}
			}
			if run.Levels != nil && run.Levels[i]%2 == 1 {
				r = mirror(r)
			}
			gid, _ = tf.face.Glyph(r)
		}
		gids[i] = gid
		adv[i] = float64(tf.face.Advance(gid) * tf.em * tf.scaleX)
		draw[i] = true
		if !run.Glyphs && utf16.IsSurrogate(rune(run.Text[i])) && i+1 < n && utf16.IsSurrogate(rune(run.Text[i+1])) {
			gids[i+1] = gid
			i++
		}
	}
	return gids, adv, draw
}

// runText returns the text of the UTF-16 units of a run in UTF-8, and the
// byte of the text where each unit starts, with the length of the text at
// the end. The second unit of a surrogate pair starts at the end of the
// pair. A lone surrogate is U+FFFD.
func runText(units []uint16) (string, []int) {
	b := make([]byte, 0, len(units))
	offsets := make([]int, len(units)+1)
	for i := 0; i < len(units); i++ {
		offsets[i] = len(b)
		r := rune(units[i])
		if utf16.IsSurrogate(r) && i+1 < len(units) {
			if p := utf16.DecodeRune(r, rune(units[i+1])); p != unicode.ReplacementChar {
				b = utf8.AppendRune(b, p)
				offsets[i+1] = len(b)
				i++
				continue
			}
		}
		b = utf8.AppendRune(b, r)
	}
	offsets[len(units)] = len(b)
	return string(b), offsets
}

// mirrors are the characters that bidirectional text mirrors at an odd
// level, the brackets of the Bidi_Mirrored property.
var mirrors = map[rune]rune{
	'(': ')', ')': '(', '<': '>', '>': '<', '[': ']', ']': '[', '{': '}', '}': '{',
	'«': '»', '»': '«', '‹': '›', '›': '‹',
}

// mirror returns the mirrored form of r.
func mirror(r rune) rune {
	if m, ok := mirrors[r]; ok {
		return m
	}
	return r
}

// MeasureText satisfies the gowemf.TextBackend interface.
func (b *backend) MeasureText(run gowemf.TextRun) (gowemf.TextMetrics, error) {
	if err := b.ctx.Err(); err != nil {
		return gowemf.TextMetrics{}, fmt.Errorf("drawing metafile: %w", err)
	}
	tf, ok := b.font(run.Font)
	if !ok {
		return gowemf.TextMetrics{Advances: make([]float64, len(run.Text))}, nil
	}
	adv := []float64(nil)
	if shaped, ok := tf.shape(run); ok {
		adv = shaped.advances
	} else {
		_, adv, _ = tf.glyphs(run)
	}
	return gowemf.TextMetrics{
		Ascent:   float64(tf.ascent * tf.em),
		Descent:  float64(tf.descent * tf.em),
		Advances: adv,
	}, nil
}

// DrawText satisfies the gowemf.TextBackend interface. A run of the glyphs of
// a font that the database does not hold draws nothing, because the indexes
// of the glyphs belong to that font.
func (b *backend) DrawText(run gowemf.TextRun, clip gowemf.Clip) error {
	if err := b.begin(); err != nil {
		return err
	}
	shader := b.shader(run.Paint)
	tf, ok := b.font(run.Font)
	if shader == nil || !ok || len(run.Origins) < len(run.Text) {
		return nil
	}
	if run.Glyphs && !strings.EqualFold(tf.family, strings.TrimSpace(run.Font.FaceName)) {
		b.skip("glyph indexes of font " + run.Font.FaceName)
		return nil
	}
	// text is the text of the run for a writer of PDF, and offsets holds
	// the byte of text where each element starts (D58). A run of glyph
	// indexes has no text.
	var text string
	var offsets []int
	if !run.Glyphs {
		text, offsets = runText(run.Text)
	}
	// rangeOf returns the bytes of text from the element i to the element
	// end
	rangeOf := func(i, end int) (int, int) {
		if offsets == nil {
			return 0, 0
		}
		return offsets[i], offsets[end]
	}
	var glyphs []display.Glyph
	if shaped, ok := tf.shape(run); ok {
		// the glyphs of a cluster are placed from the origin of its first
		// element, and the cluster covers the elements up to the next
		// element with glyphs
		for i, cluster := range shaped.glyphs {
			if len(cluster) == 0 {
				continue
			}
			end := i + 1
			for end < len(shaped.glyphs) && len(shaped.glyphs[end]) == 0 {
				end++
			}
			start, stop := rangeOf(i, end)
			o := run.Origins[i]
			for _, g := range cluster {
				g.X += float32(o.X)
				g.Y += float32(o.Y)
				g.Start, g.End = start, stop
				glyphs = append(glyphs, g)
			}
		}
	} else {
		gids, _, draw := tf.glyphs(run)
		glyphs = make([]display.Glyph, 0, len(gids))
		for i, gid := range gids {
			if draw[i] {
				o := run.Origins[i]
				start, end := rangeOf(i, i+1)
				glyphs = append(glyphs, display.Glyph{ID: gid, X: float32(o.X), Y: float32(o.Y), Start: start, End: end})
			}
		}
	}
	glyph := okage.TransformFromScale(tf.scaleX, 1)
	if run.Font.Italic && !tf.italic {
		// a face that is not italic slants for an italic font
		glyph = glyph.PostConcat(okage.TransformFromSkew(-0.2, 0))
	}
	if o := run.Font.Orientation; o != 0 {
		// the orientation turns each glyph counterclockwise, and y is down
		glyph = glyph.PostRotate(float32(-o * 180 / math.Pi))
	}
	b.clipped(clip, func() {
		b.d.Save()
		defer b.d.Restore()
		b.d.Concat(transform(run.Transform))
		b.d.FillGlyphs(display.GlyphRun{Face: tf.face, Size: tf.em, Glyphs: glyphs, Transform: glyph, Text: text}, shader)
		var width float64
		for _, a := range run.Advances {
			width += a
		}
		line := func(y float32) {
			r, ok := okage.RectFromXYWH(float32(run.Left), y, float32(width), max(tf.thick*tf.em, 0.5))
			if ok {
				b.d.FillPath(okage.PathFromRect(r), okage.FillRuleWinding, shader)
			}
		}
		if run.Font.Underline {
			line(tf.underline * tf.em)
		}
		if run.Font.StrikeOut {
			line(-tf.strike * tf.em)
		}
	})
	return nil
}
