// Package emf opens Windows metafiles: EMF, EMF+ and WMF, and their
// compressed forms EMZ and WMZ.
//
// A metafile is one picture, so a document has one page. The package plays
// the records of the file with internal/metafile, which reads them with a
// copy of gowemf (D24) and draws them onto the display list of ivy (D10).
// A record that the package cannot draw, such as a flood fill, is left out,
// and the page returns an *ivy.PartialError (D5).
//
// The text of a metafile names its fonts, and a metafile does not embed
// them. The package draws the text with the Go fonts, or with the fonts of
// the system when the field SystemFonts of ivy.Config is set (D23).
package emf

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/xo/okage"

	"ragflow/internal/thirdparty/ivy"
	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/metafile"
)

// Format describes the package for the registry of a program.
var Format = ivy.Format{
	Name: "emf",
	Mimes: []string{
		"image/emf", "image/x-emf", "application/emf", "application/x-emf",
		"image/wmf", "image/x-wmf", "application/wmf", "application/x-wmf", "application/x-msmetafile",
	},
	Exts: []string{"emf", "emz", "wmf", "wmz"},
	Open: func(ctx context.Context, src ivy.Source, cfg ivy.Config) (ivy.Document, error) {
		return Open(ctx, src, WithConfig(cfg))
	},
}

// Option is an option of Open.
type Option func(*options)

type options struct {
	cfg ivy.Config
}

// WithConfig sets the configuration that every format shares.
func WithConfig(cfg ivy.Config) Option {
	return func(o *options) {
		o.cfg = cfg
	}
}

// Document is an open metafile.
type Document struct {
	pic  *metafile.Picture
	page *Page
}

// The capabilities of a document.
var (
	_ ivy.Pager     = (*Document)(nil)
	_ ivy.Describer = (*Document)(nil)
)

// Open opens a metafile. It reads the whole file into memory, and
// decompresses an EMZ or a WMZ file. It returns an error that wraps
// ivy.ErrFormat if src is not a metafile, and one that wraps
// ivy.ErrUnsupported for a WMF file without a placeable header and without a
// window, whose size it cannot know.
func Open(ctx context.Context, src ivy.Source, opts ...Option) (d *Document, err error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("opening metafile: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			d, err = nil, fmt.Errorf("opening metafile: panic: %v: %w", r, ivy.ErrFormat)
		}
	}()
	lim := o.cfg.Limits.WithDefaults()
	size := src.Size()
	if size <= 0 {
		return nil, fmt.Errorf("metafile of %d bytes: %w", size, ivy.ErrFormat)
	}
	if size > lim.MaxMemory {
		return nil, fmt.Errorf("metafile of %d bytes: %w", size, ivy.ErrLimit)
	}
	data := make([]byte, size)
	if _, err := src.ReadAt(data, 0); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("reading metafile: %w", err)
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		if data, err = gunzip(data, lim); err != nil {
			return nil, err
		}
	}
	pic, err := metafile.Open(data, metafile.Options{Config: o.cfg})
	if err != nil {
		return nil, err
	}
	d = &Document{pic: pic}
	d.page = &Page{pic: pic, log: o.cfg.Log()}
	return d, nil
}

// gunzip decompresses an EMZ or a WMZ file, up to the limit of the memory.
func gunzip(data []byte, lim ivy.Limits) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decompressing metafile: %w: %w", ivy.ErrFormat, err)
	}
	out, err := io.ReadAll(io.LimitReader(zr, lim.MaxMemory+1))
	if err != nil {
		return nil, fmt.Errorf("decompressing metafile: %w: %w", ivy.ErrFormat, err)
	}
	if int64(len(out)) > lim.MaxMemory {
		return nil, fmt.Errorf("metafile of more than %d bytes: %w", lim.MaxMemory, ivy.ErrLimit)
	}
	return out, nil
}

// Close satisfies the ivy.Document interface.
func (d *Document) Close() error {
	return nil
}

// PageCount satisfies the ivy.Pager interface. A metafile has one page.
func (d *Document) PageCount(context.Context) (int, error) {
	return 1, nil
}

// Page satisfies the ivy.Pager interface.
func (d *Document) Page(_ context.Context, n int) (ivy.Page, error) {
	if n != 0 {
		return nil, fmt.Errorf("page %d: %w", n, ivy.ErrPageRange)
	}
	return d.page, nil
}

// Describe satisfies the ivy.Describer interface. An EMF file can describe
// itself with the name of the program that made it and the name of the
// picture. A WMF file says nothing.
func (d *Document) Describe(context.Context) (ivy.Info, error) {
	program, title := d.pic.Description()
	return ivy.Info{Title: title, Producer: program}, nil
}

// Page is the picture of a metafile.
type Page struct {
	pic *metafile.Picture
	log *slog.Logger
}

// Size satisfies the ivy.Page interface. It is the frame of an EMF file, or
// the bounds of the placeable header of a WMF file.
func (p *Page) Size() (width, height float32) {
	return p.pic.Size()
}

// The interfaces that a Page implements.
var (
	_ ivy.Page       = (*Page)(nil)
	_ display.Drawer = (*Page)(nil)
)

// Render satisfies the ivy.Page interface.
func (p *Page) Render(ctx context.Context, dst *okage.Pixmap, t okage.Transform) error {
	return p.Draw(ctx, display.NewOkage(dst, t), 1)
}

// Draw satisfies the display.Drawer interface (D58). The picture draws
// nothing into a pixmap, so it does not read res.
func (p *Page) Draw(ctx context.Context, d display.Device, _ float32) error {
	w, h := p.pic.Size()
	r, ok := okage.RectFromXYWH(0, 0, w, h)
	if !ok {
		return fmt.Errorf("rendering metafile of size %gx%g: %w", w, h, ivy.ErrFormat)
	}
	err := p.pic.Draw(ctx, d, r)
	if err != nil {
		p.log.Debug("emf: page drawn in part", "err", err)
	}
	return err
}
