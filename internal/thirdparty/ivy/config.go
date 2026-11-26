package ivy

import (
	"context"
	"io/fs"
	"log/slog"
)

// Config is the configuration that every format shares. A program gives it
// to a format with the option WithConfig of the format. The zero Config is
// valid.
type Config struct {
	// Password opens a file that a password protects (D12).
	Password string
	// PasswordFunc asks for a password. ivy calls it once for each attempt,
	// from attempt 1, while the password is wrong. It stops when the function
	// returns an error.
	PasswordFunc func(ctx context.Context, attempt int) (string, error)
	// Decrypt decodes a resource that DRM with a key protects. Only the
	// module drm gives such a function (D12). The format passes the resource
	// and its license, and it never decodes the DRM itself.
	Decrypt func(ctx context.Context, scheme string, license, data []byte) ([]byte, error)
	// Logger receives the log of the format. A nil Logger logs nothing.
	Logger *slog.Logger
	// Limits bound the work on one document.
	Limits Limits
	// Cache shares the fonts and the ICC profiles between documents. A nil
	// Cache gives each document a cache of its own.
	Cache *Cache
	// SystemFonts lets a format that lays out text, such as an ebook, use
	// the fonts of the system, after the fonts that the document embeds and
	// the fonts that ivy bundles. Without it, ivy reads no font of the
	// system, and a page looks the same on each machine (D23).
	SystemFonts bool
	// Fonts holds font files that a format that lays out text uses after
	// the fonts that the document embeds, and before the fonts of the
	// system, such as fonts.FS of the module github.com/xo/ivy/fonts. A
	// font of Fonts also replaces a font of Microsoft Office with the same
	// metrics that the document names and does not embed, such as Carlito
	// for Calibri (D24). The format reads TrueType, OpenType, WOFF and
	// WOFF2 files within Limits, and ignores the other files.
	Fonts fs.FS
	// TeX holds the files of TeX that a LaTeX document reads by name, such
	// as its class and its packages: a tree of a TeX distribution, or a
	// folder of .cls and .sty files. The format latex finds a file by its
	// base name, anywhere in the tree, within Limits. The module of ivy
	// holds no file of LaTeX (D62, D65).
	TeX fs.FS
	// Paper is the page of a document whose file gives none, such as a
	// text file, an HTML file or an ebook. The zero Paper keeps the default
	// of the format, US Letter in portrait (D32).
	Paper Paper
}

// Paper is the size of a page in points, 1/72 inch. The zero Paper is the
// default of the format.
type Paper struct {
	Width, Height float32
}

// The common papers, in portrait.
var (
	Letter = Paper{Width: 612, Height: 792}
	Legal  = Paper{Width: 612, Height: 1008}
	A4     = Paper{Width: 595.28, Height: 841.89}
	A5     = Paper{Width: 419.53, Height: 595.28}
)

// Landscape returns p with its longer side across.
func (p Paper) Landscape() Paper {
	if p.Height > p.Width {
		p.Width, p.Height = p.Height, p.Width
	}
	return p
}

// Portrait returns p with its longer side down.
func (p Paper) Portrait() Paper {
	if p.Width > p.Height {
		p.Width, p.Height = p.Height, p.Width
	}
	return p
}

// Log returns the logger of the configuration. It returns a logger that
// discards each record when Logger is nil.
func (cfg Config) Log() *slog.Logger {
	if cfg.Logger != nil {
		return cfg.Logger
	}
	return slog.New(slog.DiscardHandler)
}

// The default limits.
const (
	// DefaultMaxMemory is the default of Limits.MaxMemory: 512 MiB.
	DefaultMaxMemory = 512 << 20
	// DefaultMaxPixels is the default of Limits.MaxPixels: 256 million
	// pixels, such as an image of 16384 by 16384.
	DefaultMaxPixels = 1 << 28
	// DefaultMaxObjects is the default of Limits.MaxObjects.
	DefaultMaxObjects = 1 << 20
	// DefaultMaxDepth is the default of Limits.MaxDepth.
	DefaultMaxDepth = 64
)

// Limits bound the work on one document, because each file is input from a
// stranger (D5). A field that is zero or less takes its default. The context
// bounds the time.
type Limits struct {
	// MaxMemory is the number of bytes that the document can allocate for
	// one decoded part, such as one image or one entry of an archive.
	MaxMemory int64
	// MaxPixels is the number of pixels of the largest image that the
	// document decodes.
	MaxPixels int64
	// MaxObjects is the number of objects, entries or records that the
	// document reads.
	MaxObjects int
	// MaxDepth is the depth of nesting that the document follows.
	MaxDepth int
}

// WithDefaults returns the limits, with the default in each field that is
// zero or less.
func (lim Limits) WithDefaults() Limits {
	if lim.MaxMemory <= 0 {
		lim.MaxMemory = DefaultMaxMemory
	}
	if lim.MaxPixels <= 0 {
		lim.MaxPixels = DefaultMaxPixels
	}
	if lim.MaxObjects <= 0 {
		lim.MaxObjects = DefaultMaxObjects
	}
	if lim.MaxDepth <= 0 {
		lim.MaxDepth = DefaultMaxDepth
	}
	return lim
}

// Cache holds the fonts of the system, the fonts that documents embed, and
// the transforms of ICC profiles, and shares them between documents. It is
// safe for concurrent use. There is no global cache.
//
// The formats of phase 1 hold no fonts and no ICC profiles, so the cache is
// empty until the fonts of phase 2.
type Cache struct {
	_ struct{}
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{}
}
