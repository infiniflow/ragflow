package ivy

import (
	"context"
	"image"
	"io"
	"iter"
	"time"

	"github.com/xo/okage"
)

// Source is the input of a document. *bytes.Reader, *strings.Reader and
// *io.SectionReader satisfy it. For an *os.File, use io.NewSectionReader.
//
// The caller owns the source. A document reads it until Close, and Close
// does not close it. A document reads the source from more than one
// goroutine, so ReadAt must be safe for concurrent use, as the contract of
// io.ReaderAt says.
type Source interface {
	io.ReaderAt
	Size() int64
}

// Document is an open file. Each format returns its own type, and the type
// also satisfies the capabilities that the format has: Pager, Previewer and
// Describer. A Document is safe for concurrent use.
type Document interface {
	// Close releases the caches of the document. It does not close the
	// source. A second call does nothing.
	Close() error
}

// Pager is a document with pages.
type Pager interface {
	// PageCount returns the number of pages. For a flow of text, such as an
	// EPUB, it lays out the whole document on its first call.
	PageCount(ctx context.Context) (int, error)
	// Page returns page n, from 0. It returns ErrPageRange past the last
	// page. For a flow of text, it lays out only as far as page n.
	Page(ctx context.Context, n int) (Page, error)
}

// Previewer is a document that carries images of its content (D11).
type Previewer interface {
	// Previews lists the previews. Each one decodes only on demand.
	Previews(ctx context.Context) iter.Seq2[Preview, error]
}

// Describer is a document that says what it is: its title, its authors and
// its dates.
type Describer interface {
	Describe(ctx context.Context) (Info, error)
}

// Page is one page of a document. Render is safe for concurrent use when
// each call has its own pixmap.
type Page interface {
	// Size returns the size of the page in points, 1/72 of an inch, after
	// its rotation. A page of a raster image has one point for each pixel.
	Size() (width, height float32)
	// Render draws the page onto dst. t maps the points of the page onto the
	// pixels of dst. Render draws over what dst holds, so a program fills
	// the background first. If Render draws the page only in part, it
	// returns a *PartialError, and dst holds the part.
	Render(ctx context.Context, dst *okage.Pixmap, t okage.Transform) error
}

// Preview is one image that a document carries.
type Preview struct {
	// Kind is the kind of the preview.
	Kind PreviewKind
	// Size is the size in pixels, or zero if the file does not say it before
	// the decode.
	Size image.Point
	// Decode decodes the image.
	Decode func(ctx context.Context) (image.Image, error)
}

// PreviewKind is the kind of a preview.
type PreviewKind int

// The kinds of preview.
const (
	// Thumbnail is a small image of a page.
	Thumbnail PreviewKind = iota
	// Icon is the icon of an application or of a volume.
	Icon
	// Cover is the cover of a book.
	Cover
	// Background is the background of a disk image.
	Background
)

// String satisfies the fmt.Stringer interface.
func (kind PreviewKind) String() string {
	switch kind {
	case Thumbnail:
		return "thumbnail"
	case Icon:
		return "icon"
	case Cover:
		return "cover"
	case Background:
		return "background"
	}
	return "unknown"
}

// Info is what a document says about itself. A field is empty when the
// document does not say it.
type Info struct {
	Title    string
	Authors  []string
	Subject  string
	Language string
	Created  time.Time
	Modified time.Time
	// Producer is the program or the device that made the document.
	Producer string
	// Publisher is the publisher of the document, such as the publisher of
	// a book.
	Publisher string
	// Identifier is an identifier of the document that its file gives, such
	// as an ISBN, a UUID or an ASIN, as the file writes it.
	Identifier string
}
