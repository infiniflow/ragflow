// Package metafile draws a Windows metafile onto the display list of D10.
//
// A metafile is a list of the drawing records of Windows GDI or GDI+. The
// package reads WMF, EMF and EMF+ files with the copy of gowemf in
// internal/gowemf (D24), and plays each record onto a display.Device. It
// draws paths, fills, strokes, hatches, patterns, gradients, clips,
// transforms, bitmaps and text. It draws the raster operations of GDI, which
// combine a bitmap or a brush with the page, and the mix modes, which
// combine a pen or a brush with the page, with blend modes. The text uses
// the fonts of the program, as the layout of the ebooks does: the fonts of
// ivy.Config.Fonts, then the Go fonts. A font of Office that the program
// does not have takes the font of ivy.Config.Fonts with the same metrics,
// such as Carlito for Calibri (D24). The shaper of okage shapes the text of
// the complex scripts, such as Arabic and Devanagari, and the text with
// combining marks.
//
// The format package emf opens a metafile as a document. The Office formats
// draw a metafile that a document embeds with Picture.Draw.
package metafile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"unicode/utf16"

	"github.com/xo/okage"
	"github.com/xo/okage/svg"

	"ragflow/internal/thirdparty/ivy"
	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/gowemf"
)

// Kind is the kind of a metafile.
type Kind int

// The kinds of metafile.
const (
	// WMF is a Windows Metafile of 16 bits.
	WMF Kind = iota + 1
	// EMF is an Enhanced Metafile.
	EMF
	// EMFPlus is an Enhanced Metafile that holds records of EMF+. A file of
	// EMF+ Dual also holds the records of GDI for the same picture.
	EMFPlus
)

// String satisfies the fmt.Stringer interface.
func (k Kind) String() string {
	switch k {
	case WMF:
		return "WMF"
	case EMF:
		return "EMF"
	case EMFPlus:
		return "EMF+"
	}
	return "unknown"
}

// Options are the options of Open.
type Options struct {
	// Config gives the limits, the logger, and the fields Fonts and
	// SystemFonts.
	Config ivy.Config
	// Fonts are the fonts of the text, or nil for the fonts of the program:
	// the fonts of Config.Fonts, then the Go fonts, then the fonts of the
	// system when Config.SystemFonts is set.
	Fonts *svg.FontDatabase
}

// Picture is a metafile that Open read. It is safe for concurrent use.
type Picture struct {
	data      []byte
	header    gowemf.Header
	placeable *gowemf.PlaceableHeader
	lim       ivy.Limits
	log       *slog.Logger
	fonts     *fontSet
	// width and height are the size of the picture in points.
	width, height float64

	// mu guards recs, the last playbacks of the picture, from the oldest.
	mu   sync.Mutex
	recs []*recording
}

// maxRecordings is the number of the playbacks that a picture keeps.
const maxRecordings = 4

// recording is a playback of a picture into a rectangle, as a display list.
// The playback does not depend on the device, so Draw replays it for each
// later draw into the same rectangle. A sheet draws a picture once for each
// row that it covers, and a page in bands draws it once for each band.
type recording struct {
	dst  okage.Rect
	list *display.List
	err  error
}

// Open reads the framing of the metafile in data, and its size. It reads no
// drawing record yet. Picture keeps data, so the caller must not change it.
// It returns an error that wraps ivy.ErrFormat if data is not a metafile.
func Open(data []byte, opts Options) (p *Picture, err error) {
	defer func() {
		if r := recover(); r != nil {
			p, err = nil, fmt.Errorf("reading metafile: panic: %v: %w", r, ivy.ErrFormat)
		}
	}()
	lim := opts.Config.Limits.WithDefaults()
	if int64(len(data)) > lim.MaxMemory {
		return nil, fmt.Errorf("metafile of %d bytes: %w", len(data), ivy.ErrLimit)
	}
	header, err := gowemf.Walk(data, framingLimits(lim), nil)
	if err != nil {
		return nil, wrap("reading metafile", err)
	}
	p = &Picture{
		data:   data,
		header: header,
		lim:    lim,
		log:    opts.Config.Log(),
		fonts:  newFontSet(opts.Fonts, opts.Config),
	}
	if err := p.size(); err != nil {
		return nil, err
	}
	return p, nil
}

// framingLimits returns the limits of gowemf on the framing of a file.
func framingLimits(lim ivy.Limits) gowemf.Limits {
	return gowemf.Limits{
		MaxBytes:       uint64(lim.MaxMemory),
		MaxRecordBytes: uint64(min(lim.MaxMemory, 16<<20)),
		MaxRecords:     uint64(lim.MaxObjects),
	}
}

// size sets the size of the picture. A WMF file without a placeable header
// takes its bounds from the window of its records.
func (p *Picture) size() error {
	h := p.header
	w, ht, err := h.SizeInPoints()
	switch {
	case err == nil:
	case h.EMF != nil:
		// an empty frame: the bounds in the pixels of the reference device
		e := h.EMF
		if e.Device.X <= 0 || e.Device.Y <= 0 || e.Millimeters.X <= 0 || e.Millimeters.Y <= 0 {
			return fmt.Errorf("emf without a reference device: %w", ivy.ErrFormat)
		}
		b := e.Bounds
		w = (float64(b.Right) - float64(b.Left) + 1) * float64(e.Millimeters.X) / float64(e.Device.X) * 72 / 25.4
		ht = (float64(b.Bottom) - float64(b.Top) + 1) * float64(e.Millimeters.Y) / float64(e.Device.Y) * 72 / 25.4
	case h.WMF != nil:
		place, err := windowBounds(p.data, p.lim)
		if err != nil {
			return err
		}
		p.placeable = place
		r := place.Bounds
		scale := 72 / float64(place.UnitsPerInch)
		w = math.Abs(float64(r.Right)-float64(r.Left)) * scale
		ht = math.Abs(float64(r.Bottom)-float64(r.Top)) * scale
	default:
		return wrap("reading metafile size", err)
	}
	if !(w > 0 && ht > 0) || math.IsInf(w, 0) || math.IsInf(ht, 0) {
		return fmt.Errorf("metafile of size %gx%g: %w", w, ht, ivy.ErrFormat)
	}
	p.width, p.height = w, ht
	return nil
}

// The records of WMF that set the window, which is the frame of a WMF file
// without a placeable header (MS-WMF 2.3.5.30 and 2.3.5.31).
const (
	metaSetWindowOrg = 0x020b
	metaSetWindowExt = 0x020c
)

// windowBounds returns the placeable header of a WMF file without one: the
// window that its first SetWindowOrg and SetWindowExt records set. The
// header has 96 units to the inch, the pixels of a screen, or 1440, the
// twips that many programs use, when 96 makes the picture wider than 100
// inches. MS-WMF gives no size to such a file, so this is a guess.
func windowBounds(data []byte, lim ivy.Limits) (*gowemf.PlaceableHeader, error) {
	var org, ext [2]int16
	var hasExt bool
	errFound := errors.New("found")
	_, err := gowemf.Walk(data, framingLimits(lim), func(r gowemf.Record) error {
		if r.Format != gowemf.WMF || len(r.Data) < 4 {
			return nil
		}
		y, x := int16(uint16(r.Data[0])|uint16(r.Data[1])<<8), int16(uint16(r.Data[2])|uint16(r.Data[3])<<8)
		switch r.Type & 0xffff {
		case metaSetWindowOrg:
			org = [2]int16{x, y}
		case metaSetWindowExt:
			ext, hasExt = [2]int16{x, y}, true
			return errFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errFound) {
		return nil, wrap("reading wmf window", err)
	}
	if !hasExt || ext[0] == 0 || ext[1] == 0 {
		return nil, fmt.Errorf("wmf without a placeable header or a window extent: %w", ivy.ErrUnsupported)
	}
	place := &gowemf.PlaceableHeader{
		Bounds: gowemf.Rect{
			Left:   int32(org[0]),
			Top:    int32(org[1]),
			Right:  int32(org[0]) + int32(ext[0]),
			Bottom: int32(org[1]) + int32(ext[1]),
		},
		UnitsPerInch: 96,
	}
	if max(math.Abs(float64(ext[0])), math.Abs(float64(ext[1])))/96 > 100 {
		place.UnitsPerInch = 1440
	}
	return place, nil
}

// Kind returns the kind of the metafile.
func (p *Picture) Kind() Kind {
	switch {
	case p.header.EMFPlus != nil:
		return EMFPlus
	case p.header.EMF != nil:
		return EMF
	}
	return WMF
}

// Size returns the size of the picture in points, 1/72 of an inch.
func (p *Picture) Size() (width, height float32) {
	return float32(p.width), float32(p.height)
}

// Description returns the two strings of the description of an EMF file:
// the name of the program that made it, and the name of the picture. They
// are empty for a WMF file, and for an EMF file without a description.
func (p *Picture) Description() (program, title string) {
	if p.header.EMF == nil {
		return "", ""
	}
	b := p.header.EMF.Description
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	var parts []string
	start := 0
	for i, c := range u {
		if c == 0 {
			parts = append(parts, string(utf16.Decode(u[start:i])))
			start = i + 1
		}
	}
	if start < len(u) {
		parts = append(parts, string(utf16.Decode(u[start:])))
	}
	if len(parts) > 0 {
		program = parts[0]
	}
	if len(parts) > 1 {
		title = parts[1]
	}
	return program, title
}

// Draw plays the records of the picture onto d, into the rectangle dst of
// the user space of d. It changes no state of d that outlives the call.
// Draw records the playback into a display list, and replays the list onto
// d. A later draw into the same rectangle replays the list again.
//
// If Draw leaves out a record that the package cannot draw, or stops at a
// malformed record after it drew a part, it returns an *ivy.PartialError,
// and d holds the part. It returns the error of ctx when ctx ends.
func (p *Picture) Draw(ctx context.Context, d display.Device, dst okage.Rect) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("drawing metafile: %w", err)
	}
	rec := p.recording(dst)
	if rec == nil {
		list := display.NewRecording()
		err := p.play(ctx, list, dst)
		if ctx.Err() != nil {
			return err
		}
		rec = &recording{dst: dst, list: list, err: err}
		p.mu.Lock()
		if len(p.recs) >= maxRecordings {
			p.recs = slices.Delete(p.recs, 0, len(p.recs)-maxRecordings+1)
		}
		p.recs = append(p.recs, rec)
		p.mu.Unlock()
	}
	rec.list.Replay(d)
	return rec.err
}

// recording returns the playback of the picture into dst, or nil.
func (p *Picture) recording(dst okage.Rect) *recording {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, rec := range p.recs {
		if rec.dst == dst {
			return rec
		}
	}
	return nil
}

// play plays the records of the picture onto d, into dst.
func (p *Picture) play(ctx context.Context, d display.Device, dst okage.Rect) (err error) {
	b := newBackend(ctx, d, dst, p)
	d.Save()
	defer func() {
		b.close()
		d.Restore()
		if r := recover(); r != nil {
			err = &ivy.PartialError{Err: fmt.Errorf("drawing metafile: panic: %v: %w", r, ivy.ErrFormat)}
		}
	}()
	ansi := uint8(0)
	opts := gowemf.PlayOptions{
		Stream: gowemf.StreamOptions{
			Framing: framingLimits(p.lim),
			Decoding: gowemf.DecodeLimits{
				MaxNesting:     uint32(min(p.lim.MaxDepth, 256)),
				MaxObjectBytes: uint64(min(p.lim.MaxMemory, 16<<20)),
			},
		},
		Images: gowemf.ImageLimits{
			MaxBytes:        uint64(min(p.lim.MaxMemory, 16<<20)),
			MaxPixels:       uint64(p.lim.MaxPixels),
			MaxDecodedBytes: uint64(p.lim.MaxMemory),
		},
		Destination: gowemf.Box{
			X:      float64(dst.X()),
			Y:      float64(dst.Y()),
			Width:  float64(dst.Width()),
			Height: float64(dst.Height()),
		},
		Placeable:      p.placeable,
		ColorTransform: gowemf.NewColorTransform,
		// ANSI text in DEFAULT_CHARSET takes the code page 1252, the code
		// page of Windows in English
		DefaultCharSet: &ansi,
		// the custom caps and the arrows of EMF+ lines take the geometry
		// that gowemf gives them. MS-EMFPLUS does not define it, and a
		// review of a document needs the arrows of its lines (D5)
		CustomLineCaps:     true,
		MaxImagePixels:     uint64(p.lim.MaxPixels),
		MaxMetafileDepth:   uint32(min(p.lim.MaxDepth, 8)),
		MaxEmbeddedRecords: uint64(p.lim.MaxObjects),
		Unsupported:        b.unsupported,
	}
	_, err = gowemf.Play(p.data, opts, b)
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("drawing metafile: %w", ctx.Err())
	case err != nil && b.drew:
		return &ivy.PartialError{Err: wrap("drawing metafile", err)}
	case err != nil:
		return wrap("drawing metafile", err)
	case b.skipped > 0:
		return &ivy.PartialError{Err: fmt.Errorf("drawing metafile: %d records left out, the first for %s: %w",
			b.skipped, b.firstSkip, ivy.ErrUnsupported)}
	}
	return nil
}

// wrap wraps an error of gowemf with the sentinel error of ivy that matches
// it.
func wrap(op string, err error) error {
	switch {
	case errors.Is(err, gowemf.ErrLimit):
		return fmt.Errorf("%s: %w: %w", op, ivy.ErrLimit, err)
	case errors.Is(err, gowemf.ErrUnsupported):
		return fmt.Errorf("%s: %w: %w", op, ivy.ErrUnsupported, err)
	case errors.Is(err, gowemf.ErrFormat), errors.Is(err, gowemf.ErrMalformed):
		return fmt.Errorf("%s: %w: %w", op, ivy.ErrFormat, err)
	}
	if _, ok := errors.AsType[ivy.Error](err); ok {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w: %w", op, ivy.ErrFormat, err)
}
