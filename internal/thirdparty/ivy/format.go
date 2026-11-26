package ivy

import (
	"cmp"
	"context"
	"errors"
	"image"
	"image/color"
	"iter"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/xo/okage"
)

// Format describes a format package for the registry of a program. ivy holds
// no registry (D6). Each format package exports a Format value, and a program
// builds its own registry from the values of the packages that it imports.
type Format struct {
	// Name is the name of the format, such as "psd".
	Name string
	// Mimes are the mime types of the format.
	Mimes []string
	// Exts are the extensions of the format, without the leading dot.
	Exts []string
	// Open opens a document with the default options of the format and the
	// configuration cfg.
	Open func(ctx context.Context, src Source, cfg Config) (Document, error)
}

// Pages iterates over the pages of p, in order, until the last page. It
// stops after the first error.
func Pages(ctx context.Context, p Pager) iter.Seq2[Page, error] {
	return func(yield func(Page, error) bool) {
		for n := 0; ; n++ {
			page, err := p.Page(ctx, n)
			switch {
			case errors.Is(err, ErrPageRange):
				return
			case err != nil:
				yield(nil, err)
				return
			case !yield(page, nil):
				return
			}
		}
	}
}

// BestPreview decodes the preview of d that is closest in size to want, and
// prefers one that is larger. A zero want asks for the largest preview. If
// the decode of a preview fails, BestPreview tries the next one. It returns
// ErrNoPreview if d is not a Previewer or lists no preview.
func BestPreview(ctx context.Context, d Document, want image.Point) (image.Image, error) {
	previewer, ok := d.(Previewer)
	if !ok {
		return nil, ErrNoPreview
	}
	var previews []Preview
	for preview, err := range previewer.Previews(ctx) {
		if err != nil {
			return nil, err
		}
		previews = append(previews, preview)
	}
	if len(previews) == 0 {
		return nil, ErrNoPreview
	}
	sortPreviews(previews, want)
	var last error
	for _, preview := range previews {
		img, err := preview.Decode(ctx)
		if err == nil {
			return img, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		last = err
	}
	return nil, last
}

// sortPreviews sorts the previews in the order that BestPreview tries them:
// first the smallest one that covers want, then the larger ones, then the
// smaller ones from the largest, and last the ones of unknown size.
func sortPreviews(previews []Preview, want image.Point) {
	rank := func(p Preview) (int, int) {
		side := max(p.Size.X, p.Size.Y)
		switch {
		case side == 0:
			return 3, 0
		case want == (image.Point{}):
			return 1, -side
		case p.Size.X >= want.X && p.Size.Y >= want.Y:
			return 0, side
		default:
			return 2, -side
		}
	}
	slices.SortStableFunc(previews, func(a, b Preview) int {
		ag, ak := rank(a)
		bg, bk := rank(b)
		if c := cmp.Compare(ag, bg); c != 0 {
			return c
		}
		return cmp.Compare(ak, bk)
	})
}

// RenderImage renders p at dpi dots per inch over bg, and returns the image
// (D11). A nil bg leaves the background transparent. A page of a raster
// image renders at its own size at 72 dots per inch.
//
// A large page renders in bands of rows, on up to one goroutine for each
// CPU and at most 8 (D47). Each band calls Render with a pixmap of its own,
// so Render must be safe for concurrent use, as D11 says. The pixels do not
// depend on the number of CPUs.
//
// If the page is drawn only in part, RenderImage returns the image and a
// *PartialError.
func RenderImage(ctx context.Context, p Page, dpi float32, bg color.Color) (image.Image, error) {
	return renderImage(ctx, p, dpi, bg, min(runtime.GOMAXPROCS(0), maxBandWorkers))
}

// renderImage is RenderImage, with workers goroutines for the bands.
func renderImage(ctx context.Context, p Page, dpi float32, bg color.Color, workers int) (image.Image, error) {
	if dpi <= 0 || math.IsNaN(float64(dpi)) || math.IsInf(float64(dpi), 0) {
		return nil, ErrUnsupported
	}
	scale := dpi / 72
	w, h := p.Size()
	pw, ph := math.Ceil(float64(w*scale)), math.Ceil(float64(h*scale))
	if pw < 1 || ph < 1 {
		return nil, ErrFormat
	}
	if pw*ph > DefaultMaxPixels || pw > math.MaxInt32 || ph > math.MaxInt32 {
		return nil, ErrLimit
	}
	dst, ok := okage.NewPixmap(uint32(pw), uint32(ph))
	if !ok {
		return nil, ErrLimit
	}
	if bg != nil {
		dst.Fill(okageColor(bg))
	}
	err := renderBands(ctx, p, dst, scale, workers)
	if err != nil {
		if _, partial := errors.AsType[*PartialError](err); !partial {
			return nil, err
		}
	}
	return dst, err
}

// RenderPages renders the pages of p whose numbers are in pages, from 0, at
// dpi over bg, as RenderImage does, and yields the image and the error of
// each in the order of pages (D47). It renders up to one page for each CPU
// at the same time, and the bands of each page one after the other, so the
// pixels are the same as those of RenderImage. It renders at most as many
// pages ahead of the page that it yields as it has goroutines. A page past
// the last page yields an error that wraps ErrPageRange. When the loop
// stops early, RenderPages stops its goroutines before it returns.
func RenderPages(ctx context.Context, p Pager, pages []int, dpi float32, bg color.Color) iter.Seq2[image.Image, error] {
	return func(yield func(image.Image, error) bool) {
		if len(pages) == 0 {
			return
		}
		// The deferred calls run in the reverse order: cancel stops the
		// goroutines, and then Wait waits for them.
		var wg sync.WaitGroup
		defer wg.Wait()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		type result struct {
			img image.Image
			err error
		}
		workers := min(runtime.GOMAXPROCS(0), len(pages))
		// results holds a channel for each page, and slots bounds the pages
		// that are rendered and not yet yielded.
		results := make([]chan result, len(pages))
		for i := range results {
			results[i] = make(chan result, 1)
		}
		slots := make(chan struct{}, workers)
		var next atomic.Int64
		for range workers {
			wg.Go(func() {
				for {
					select {
					case slots <- struct{}{}:
					case <-ctx.Done():
						return
					}
					i := int(next.Add(1)) - 1
					if i >= len(pages) {
						<-slots
						return
					}
					var r result
					page, err := p.Page(ctx, pages[i])
					if err != nil {
						r.err = err
					} else {
						r.img, r.err = renderImage(ctx, page, dpi, bg, 1)
					}
					results[i] <- r
				}
			})
		}
		for i := range pages {
			var r result
			select {
			case r = <-results[i]:
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			}
			<-slots
			if !yield(r.img, r.err) {
				return
			}
		}
	}
}

// okageColor converts c to a color of okage, which is not premultiplied.
func okageColor(c color.Color) okage.Color {
	n, _ := color.NRGBAModel.Convert(c).(color.NRGBA)
	return okage.ColorFromRGBA8(n.R, n.G, n.B, n.A)
}

// The bands of renderBands.
const (
	// minBandPixels is the size of the smallest page, in pixels, that
	// renderBands splits into bands.
	minBandPixels = 1 << 18
	// bandRows is the height of a band. It does not depend on the number of
	// CPUs, so that a page gives the same pixels on each computer.
	bandRows = 256
	// bandMargin is the number of rows that a band draws more on each side.
	bandMargin = 32
	// maxBandWorkers bounds the goroutines of a page, because each band runs
	// the content of the page again, and holds its memory.
	maxBandWorkers = 8
)

// renderBands renders p onto dst at scale (D47). A page of minBandPixels
// pixels or more renders as horizontal bands of bandRows rows, on workers
// goroutines. Each band draws the page into a pixmap of its own, with
// bandMargin rows more above and below and the transform moved up by its
// top, and then copies its own rows into dst. The bands do not depend on
// workers, so a page gives the same pixels on each computer. Page.Render is
// safe for concurrent use when each call has its own pixmap (D11).
// renderBands returns the first error that is not a *PartialError, or else
// the first *PartialError, in the order of the bands.
func renderBands(ctx context.Context, p Page, dst *okage.Pixmap, scale float32, workers int) error {
	w, h := int(dst.Width()), int(dst.Height())
	n := (h + bandRows - 1) / bandRows
	if n < 2 || w*h < minBandPixels {
		return p.Render(ctx, dst, okage.TransformFromScale(scale, scale))
	}
	workers = max(min(workers, n), 1)
	// The bands start from a copy of dst, because a band draws over the
	// rows of its neighbors, which they write at the same time.
	data := dst.DataMut()
	start := slices.Clone(data)
	errs := make([]error, n)
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				errs[i] = renderBand(ctx, p, start, data, w, h, i, scale)
			}
		})
	}
	wg.Wait()
	var partial error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if _, ok := errors.AsType[*PartialError](err); !ok {
			return err
		}
		if partial == nil {
			partial = err
		}
	}
	return partial
}

// renderBand renders band i of a page of w by h pixels into data, the
// pixels of the page. The band starts from the pixels of start, the pixels
// of the page before the bands, and writes only its own rows of data. It
// draws bandMargin rows more above and below, which it does not keep, so
// that a drawing near its edges is the same as in the middle of a band.
func renderBand(ctx context.Context, p Page, start, data []byte, w, h, i int, scale float32) error {
	y0, y1 := i*bandRows, min((i+1)*bandRows, h)
	m0, m1 := max(y0-bandMargin, 0), min(y1+bandMargin, h)
	band, ok := okage.NewPixmap(uint32(w), uint32(m1-m0))
	if !ok {
		return ErrLimit
	}
	copy(band.DataMut(), start[m0*w*4:m1*w*4])
	err := p.Render(ctx, band, okage.TransformFromRow(scale, 0, 0, scale, 0, -float32(m0)))
	copy(data[y0*w*4:y1*w*4], band.Data()[(y0-m0)*w*4:(y1-m0)*w*4])
	return err
}
