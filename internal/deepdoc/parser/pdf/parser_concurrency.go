package pdf

import (
	"context"
	"image"
	"runtime"
	"sync"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/utility"
)

// ── Internal concurrency guards ──────────────────────────────────────────
//
// Real page parallelism exposes the parser to multiplicative DeepDoc
// inference fan-out (DLA + TSR per table region + OCR per region).
// deepInfLimiter (deepdoc inference limiter) bounds the worst-case
// concurrent inference work a single document can drive. It is
// parser-owned, not user-configurable, so callers do not have to reason
// about the knob.
//
// Native PDFium access (RenderPage / ExtractChars / PageSize / outlines)
// is serialized by a process-wide mutex in package pdfsync, shared by
// both the cgo pdfium binding and the Rust pdf_oxide binding — PDFium is
// not thread-safe for any call, even across different documents, so the
// mutex (not a per-Parser limiter) is the correct guard. See
// pdfsync/pdfsync.go.

const (
	// defaultdeepInfCapacity bounds DeepDoc (deepdoc) inference calls
	// (DLA, TSR, OCR).
	defaultdeepInfCapacity = 8
)

// deepInfLimiter bounds concurrent DeepDoc (deepdoc) inference calls emitted
// by per-page workers. acquire blocks until a slot is available or the
// context is cancelled; release must be called in a defer.
type deepInfLimiter struct {
	sem chan struct{}
}

func newdeepInfLimiter(capacity int) *deepInfLimiter {
	if capacity <= 0 {
		capacity = defaultdeepInfCapacity
	}
	return &deepInfLimiter{sem: make(chan struct{}, capacity)}
}

func (l *deepInfLimiter) acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case l.sem <- struct{}{}:
		return nil
	}
}

func (l *deepInfLimiter) release() {
	if l == nil {
		return
	}
	<-l.sem
}

// withSlot acquires a slot, runs fn, and releases the slot. fn's error
// is propagated unchanged.
func (l *deepInfLimiter) withSlot(ctx context.Context, fn func() error) error {
	if err := l.acquire(ctx); err != nil {
		return err
	}
	defer l.release()
	return fn()
}

// ── Parser-owned limiter accessors ────────────────────────────────────────

// pageTask holds the per-page work handed to the shared worker pool.
type pageTask struct {
	parser      *Parser
	engine      pdf.PDFEngine
	pageNumber  int
	docAnalyzer pdf.DocAnalyzer
	tb          pdf.TableBuilder
}

const pageWorkerQueueFactor = 4

var (
	pagePoolOnce sync.Once
	pagePool     *utility.WorkerPool[pageTask, pageResult]
)

func parserPageWorkerPool() *utility.WorkerPool[pageTask, pageResult] {
	pagePoolOnce.Do(func() {
		workers := runtime.GOMAXPROCS(0) * 2
		if workers <= 0 {
			workers = 1
		}
		pagePool = utility.NewWorkerPool(workers, workers*pageWorkerQueueFactor,
			func(ctx context.Context, task pageTask) (pageResult, error) {
				return task.parser.processPage(ctx, task.engine, task.pageNumber,
					task.docAnalyzer, task.tb), nil
			})
	})
	return pagePool
}

// PageWorkerPoolStats returns process-wide stats for the shared PDF page worker pool.
func PageWorkerPoolStats() utility.WorkerPoolStats {
	return parserPageWorkerPool().Stats()
}

// SetPageWorkerPoolSize adjusts the process-wide PDF page worker pool size.
func SetPageWorkerPoolSize(workers int) {
	parserPageWorkerPool().Resize(workers)
}

// limiters returns the parser's lazily-initialized DeepDoc inference
// limiter. The returned limiter is shared across ParseRaw calls (and
// per-page workers) so test or production callers that reuse a Parser
// do not pile up extra slots. Creation is guarded by deepInfOnce so a
// Parser shared across goroutines initializes the slot channel once.
func (p *Parser) limiters() *deepInfLimiter {
	p.deepInfOnce.Do(func() {
		p.deepInf = newdeepInfLimiter(0)
	})
	return p.deepInf
}

// ── Wrapped calls used by the parser pipeline ─────────────────────────────

// renderPageToImage renders a page at the default DLA DPI. Native PDFium
// access inside the engine is serialized by the process-wide pdfsync.Mu
// (see pdfsync/pdfsync.go), so no per-Parser engine limiter is needed here.
func (p *Parser) renderPageToImage(ctx context.Context, eng pdf.PDFEngine, pageNum int) (image.Image, error) {
	return RenderPageToImage(eng, pageNum)
}

// renderAtDPI invokes the engine's DPI-parameterized render path. As with
// renderPageToImage, native PDFium serialization is handled by pdfsync.Mu.
func (p *Parser) renderAtDPI(ctx context.Context, eng pdf.PDFEngine, pageNum int, dpi float64) (image.Image, error) {
	return eng.RenderPageImage(pageNum, dpi)
}

// inferDLA acquires the inference limiter slot before invoking the
// per-page DLA call. Page workers and enrichOnePageWithDeepDoc callers
// route through this wrapper so DLA fan-out cannot overwhelm the
// DeepDoc service independently of the page worker pool size.
func (p *Parser) inferDLA(ctx context.Context, doc pdf.DocAnalyzer, pageImg image.Image) ([]pdf.DLARegion, error) {
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	var regions []pdf.DLARegion
	err := p.limiters().withSlot(ctx, func() error {
		r, callErr := doc.DLA(ctx, pageImg)
		if callErr != nil {
			return callErr
		}
		regions = r
		return nil
	})
	return regions, err
}

// inferTSR acquires the inference limiter slot before invoking TSR for
// a single cropped table region. TSR can be called once per detected
// DLA table region, so without this wrapper the worst-case
// (DlaRegion × pool worker count) fan-out could overwhelm the service.
func (p *Parser) inferTSR(ctx context.Context, tb pdf.TableBuilder, cropped image.Image) ([]pdf.TSRCell, error) {
	if tb == nil {
		return nil, nil
	}
	var cells []pdf.TSRCell
	err := p.limiters().withSlot(ctx, func() error {
		c, callErr := tb.DetectCells(ctx, cropped)
		if callErr != nil {
			return callErr
		}
		cells = c
		return nil
	})
	return cells, err
}

// inferOCRDetect routes doc.OCRDetect through the inference limiter.
// ocrMergeChars and ocrDetectAndRecognize callers should funnel
// through this helper.
func (p *Parser) inferOCRDetect(ctx context.Context, doc pdf.DocAnalyzer, pageImg image.Image) ([]pdf.OCRBox, error) {
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	var boxes []pdf.OCRBox
	err := p.limiters().withSlot(ctx, func() error {
		b, callErr := doc.OCRDetect(ctx, pageImg)
		if callErr != nil {
			return callErr
		}
		boxes = b
		return nil
	})
	return boxes, err
}

// inferOCRRecognize routes doc.OCRRecognize through the inference
// limiter. Per-region OCR fallback paths (buildTextBoxes, ocrTableCells)
// should use this wrapper so the per-region fan-out is bounded.
func (p *Parser) inferOCRRecognize(ctx context.Context, doc pdf.DocAnalyzer, cropped image.Image) ([]pdf.OCRText, error) {
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	var texts []pdf.OCRText
	err := p.limiters().withSlot(ctx, func() error {
		t, callErr := doc.OCRRecognize(ctx, cropped)
		if callErr != nil {
			return callErr
		}
		texts = t
		return nil
	})
	return texts, err
}
