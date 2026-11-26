// Package font is the font layer of D10. It loads the fonts that a document
// embeds, maps characters to glyphs, and gives the outline of a glyph, its
// advance, and its vertical advance and origin for vertical text.
//
// On 2026-11-16 it reads TrueType, OpenType and CFF fonts with go-text (D7).
// Type 1 and Type 3, and the substitute of a font that a document names and
// does not embed, come with PDF in phase 3.
package font

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	gofont "github.com/xo/okage/typesetting/font"
	"github.com/xo/okage/typesetting/font/opentype"

	"github.com/xo/okage"
	"ragflow/internal/thirdparty/ivy"
)

// GID is the index of a glyph in a font.
type GID = uint32

// Face is a font. Its methods are safe for concurrent use.
type Face struct {
	mu       sync.Mutex
	face     *gofont.Face
	upem     float32
	outlines map[GID]*okage.Path
	// bitmaps holds the bitmaps that Bitmap decoded, and nil for a glyph
	// without one.
	bitmaps map[GID]*Bitmap
	// src is the font file of the face, and hasSrc is false when the face
	// does not know it.
	src    Source
	hasSrc bool
}

// Load reads the font in data, which is TrueType, OpenType or CFF in an
// OpenType wrapper. For a collection, index selects the font. It returns an
// error that wraps ivy.ErrFormat if data is not such a font. The face keeps
// data as its Source, with the length of each table cut to the end of
// data.
func Load(data []byte, index int) (f *Face, err error) {
	data, err = clampDirectory(data)
	if err != nil {
		return nil, err
	}
	// go-text can panic on a malformed font, and a font is input from a
	// stranger (D5, D18)
	defer func() {
		if r := recover(); r != nil {
			f, err = nil, fmt.Errorf("reading font: %v: %w", r, ivy.ErrFormat)
		}
	}()
	faces, err := gofont.ParseTTC(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("reading font: %w: %w", ivy.ErrFormat, err)
	}
	if index < 0 || index >= len(faces) {
		return nil, fmt.Errorf("font %d of a collection of %d: %w", index, len(faces), ivy.ErrFormat)
	}
	face := faces[index]
	upem := float32(face.Upem())
	if upem <= 0 {
		upem = 1000
	}
	return &Face{
		face:     face,
		upem:     upem,
		outlines: make(map[GID]*okage.Path),
		src:      Source{Data: data, Index: index},
		hasSrc:   true,
	}, nil
}

// FromFace returns the font of a face of go-text that is already parsed,
// with its variations, such as a face that the layout shaped its text with.
// The caller must not use the face of go-text at the same time as the
// returned font.
func FromFace(face *gofont.Face) *Face {
	upem := float32(face.Upem())
	if upem <= 0 {
		upem = 1000
	}
	return &Face{face: face, upem: upem, outlines: make(map[GID]*okage.Path)}
}

// FromFaceSource returns the font of a face of go-text, as FromFace does,
// and keeps src, the font file of the face, for a writer that embeds it
// (D58).
func FromFaceSource(face *gofont.Face, src Source) *Face {
	f := FromFace(face)
	f.src, f.hasSrc = src, len(src.Data) > 0
	return f
}

// Source returns the font file of the face. It returns false when the face
// does not know it, such as a face of FromFace.
func (f *Face) Source() (Source, bool) {
	return f.src, f.hasSrc
}

// clampDirectory returns data, with the length of each table of each font cut
// to the end of data. go-text reserves the length that the directory gives
// for a table before it reads the table, so a table of a declared length of
// 4 GB in a small file makes it reserve 4 GB. A writer of fonts can also
// declare a last table a few bytes longer than the file, so a table past the
// end is cut, not refused. data is copied before a change.
func clampDirectory(data []byte) ([]byte, error) {
	bad := fmt.Errorf("reading font: bad table directory: %w", ivy.ErrFormat)
	if len(data) < 12 {
		return nil, bad
	}
	size := uint64(len(data))
	offsets := []uint64{0}
	if string(data[:4]) == "ttcf" {
		n := uint64(binary.BigEndian.Uint32(data[8:]))
		if 12+4*n > size {
			return nil, bad
		}
		offsets = offsets[:0]
		for i := range n {
			offsets = append(offsets, uint64(binary.BigEndian.Uint32(data[12+4*i:])))
		}
	}
	copied := false
	for _, off := range offsets {
		if off+12 > size {
			return nil, bad
		}
		n := uint64(binary.BigEndian.Uint16(data[off+4:]))
		if off+12+16*n > size {
			return nil, bad
		}
		for i := range n {
			rec := off + 12 + 16*i
			start := uint64(binary.BigEndian.Uint32(data[rec+8:]))
			length := uint64(binary.BigEndian.Uint32(data[rec+12:]))
			if start+length <= size {
				continue
			}
			if !copied {
				data, copied = bytes.Clone(data), true
			}
			binary.BigEndian.PutUint32(data[rec+12:], uint32(size-min(start, size)))
		}
	}
	return data, nil
}

// Glyph returns the glyph of the character r, and false if the font has none.
func (f *Face) Glyph(r rune) (gid GID, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// go-text can panic on a malformed font (D5).
	defer func() {
		if recover() != nil {
			gid, ok = 0, false
		}
	}()
	g, ok := f.face.NominalGlyph(r)
	return GID(g), ok
}

// Advance returns the horizontal advance of the glyph, in ems. It returns
// 0 when the font holds no advance for the glyph.
func (f *Face) Advance(gid GID) (adv float32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// go-text panics on a font whose hmtx table holds no metrics (D5).
	defer func() {
		if recover() != nil {
			adv = 0
		}
	}()
	return f.face.HorizontalAdvance(opentype.GID(gid)) / f.upem
}

// VerticalAdvance returns the vertical advance of the glyph, in ems, as a
// length down the page. It comes from the tables vhea and vmtx, or from the
// height of the font when the font has no vmtx table. It returns 0 when the
// font holds no vertical advance for the glyph.
func (f *Face) VerticalAdvance(gid GID) (adv float32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// go-text can panic on a malformed font (D5).
	defer func() {
		if recover() != nil {
			adv = 0
		}
	}()
	// go-text gives the vertical advance with y up, so it is negative
	return max(-f.face.VerticalAdvance(opentype.GID(gid))/f.upem, 0)
}

// VerticalOrigin returns the vertical origin of the glyph, in ems: the
// point of the glyph that vertical text puts at the pen. x is half of the
// horizontal advance, and y is the height above the baseline, with y up. y
// comes from the VORG table, or from the vmtx and the glyf tables, or else
// from the height of the glyph. It returns false when the font cannot give
// the origin.
func (f *Face) VerticalOrigin(gid GID) (x, y float32, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// go-text can panic on a malformed font (D5).
	defer func() {
		if recover() != nil {
			x, y, ok = 0, 0, false
		}
	}()
	x, y = f.face.GlyphVOrigin(opentype.GID(gid))
	return x / f.upem, y / f.upem, true
}

// Outline returns the outline of the glyph, in ems, with y down, from the
// origin of the glyph on the baseline. It returns nil for a glyph without an
// outline, such as a space. The outlines are kept, and a caller must not
// change one.
func (f *Face) Outline(gid GID) *okage.Path {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.outlines[gid]; ok {
		return p
	}
	p := f.outline(gid)
	f.outlines[gid] = p
	return p
}

// outline builds the outline of the glyph, from the table glyf, CFF or
// CFF2. It reads no embedded bitmap. GlyphData of go-text gives the bitmap
// of a glyph first when the font has one, as Calibri, Cambria and Lucida
// Console have in EBDT, and that bitmap holds no outline.
func (f *Face) outline(gid GID) (p *okage.Path) {
	// go-text can panic on a malformed font (D5).
	defer func() {
		if recover() != nil {
			p = nil
		}
	}()
	outline, ok := f.face.GlyphDataOutline(opentype.GID(gid))
	if !ok || len(outline.Segments) == 0 {
		return nil
	}
	scale := 1 / f.upem
	pt := func(p opentype.SegmentPoint) (float32, float32) {
		return p.X * scale, -p.Y * scale
	}
	pb := okage.NewPathBuilder()
	for _, s := range outline.Segments {
		switch s.Op {
		case opentype.SegmentOpMoveTo:
			pb.MoveTo(pt(s.Args[0]))
		case opentype.SegmentOpLineTo:
			pb.LineTo(pt(s.Args[0]))
		case opentype.SegmentOpQuadTo:
			x1, y1 := pt(s.Args[0])
			x, y := pt(s.Args[1])
			pb.QuadTo(x1, y1, x, y)
		case opentype.SegmentOpCubeTo:
			x1, y1 := pt(s.Args[0])
			x2, y2 := pt(s.Args[1])
			x, y := pt(s.Args[2])
			pb.CubicTo(x1, y1, x2, y2, x, y)
		}
	}
	path, ok := pb.Finish()
	if !ok {
		return nil
	}
	return path
}
