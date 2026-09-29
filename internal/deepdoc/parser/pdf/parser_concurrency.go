package pdf

import (
	"context"
	"image"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/utility"
)

// ── Process inference budget ─────────────────────────────────────────────
//
// Real page parallelism exposes the parser to multiplicative DeepDoc inference
// fan-out (DLA + TSR per table region + OCR per region). Every ONNX session runs
// single-threaded (see the intraOpThreads constant in the native package), so the
// threads DeepDoc inference occupies in this process are exactly the number of
// Runs in flight at once — one number to bound.
//
// That number is the process inference budget. It is (a) registered with the
// native gate every inference call passes through (native.SetInferenceLimit,
// called by the server's backend wiring) and (b) used to size the page worker
// pool. It is configurable: the server resolves it once at start from
// CLI > env (RAGFLOW_DEEPDOC_INFERENCE_CONCURRENCY) > config
// (ingestor.inference_concurrency) > default 1, then injects it with
// SetDeepDocConcurrency. Callers read it via DeepDocConcurrency(); they never
// have to reason about the precedence themselves.
//
// Native PDFium access (RenderPage / ExtractChars / PageSize / outlines)
// is serialized by a process-wide mutex in package pdfsync, shared by
// both the cgo pdfium binding and the Rust pdf_oxide binding — PDFium is
// not thread-safe for any call, even across different documents, so the
// mutex (not a per-Parser limiter) is the correct guard. See
// pdfsync/pdfsync.go.

// deepDocInferenceConcurrency is the process-wide DeepDoc ONNX inference budget,
// set once at server start via SetDeepDocConcurrency. It is the maximum number
// of Runs in flight; each Run is single-threaded (intraOpThreads = 1 in the
// native package), so it is also the number of cores inference may occupy.
// The default (1) is the conservative floor; the server overrides it from
// CLI/env/config at boot via SetDeepDocConcurrency.
var deepDocInferenceConcurrency = 1

// SetDeepDocConcurrency sets the process inference budget. It is called exactly
// once at server boot after CLI/env/config resolution. Non-positive values are
// clamped to 1.
func SetDeepDocConcurrency(n int) {
	deepDocInferenceConcurrency = max(1, n)
}

// DeepDocConcurrency returns how many DeepDoc ONNX Runs this process may have in
// flight at once — its inference budget. Sessions run single-threaded, so this
// is also the number of threads inference occupies.
func DeepDocConcurrency() int {
	return deepDocInferenceConcurrency
}

// ── Process-wide page concurrency (N) ──────────────────────────────────────
//
// PageConcurrency (N) is the total number of PDF pages parsed concurrently
// across the whole process. It is the size of the shared page
// worker pool (see parserPageWorkerPool) and is resolved once at server start
// via SetPageConcurrency from CLI > env > config (ingestor.page_concurrency) >
// default(2). It is deliberately independent of the process inference budget
// (DeepDocConcurrency): a page worker that is not currently holding an
// inference slot only queues a rendered bitmap while it waits, so N governs
// page-level scheduling and memory, not inference throughput. The CLI/env/config
// resolver in cmd validates N against [MinPageConcurrency, MaxPageConcurrency]
// and fails fast on out-of-range values; the setter below clamps defensively so
// an already-validated value is never distorted by a stray caller.
const (
	minPageConcurrency = 1
	maxPageConcurrency = 16
)

var pageConcurrency = 2

// SetPageConcurrency sets the per-document page parallelism (N). It is called
// exactly once at server boot after CLI/env/config resolution; the resolved
// value is already within [1, 16], and any out-of-range input is clamped here
// as a last-resort safety net (the setter never shrinks below 1).
func SetPageConcurrency(n int) {
	if n < minPageConcurrency {
		n = minPageConcurrency
	}
	if n > maxPageConcurrency {
		n = maxPageConcurrency
	}
	pageConcurrency = n
}

// PageConcurrency returns the per-document page parallelism (N).
func PageConcurrency() int {
	return pageConcurrency
}

// ── Page worker pool ─────────────────────────────────────────────────────

// pageTask holds the per-page work handed to the shared worker pool.
type pageTask struct {
	parser      *Parser
	engine      pdf.PDFEngine
	pageNumber  int
	docAnalyzer pdf.DocAnalyzer
	tb          pdf.TableBuilder
	// progress reports this run's page completions; the worker fires the
	// caller's callback as soon as its page finishes.
	progress *pageProgress
}

// pageProgress serialises page-completion reporting for one runPageWorkers
// run. The counter and the callback share one lock so completions are reported
// in order and never concurrently — the contract ParserConfig.OnPageDone
// documents — even though the pages themselves run on parallel workers.
//
// Reporting from the worker is what ties the callback to the page that just
// finished. The collection loop cannot run until every page has been
// submitted, and SubmitTo blocks once the worker queue is full, so on a
// document larger than the pool's worker+queue capacity a collector-side
// callback stays silent while the pool drains the overflow and then reports
// every completion behind it in one burst.
type pageProgress struct {
	mu     sync.Mutex
	done   int
	total  int
	onDone func(done, total int)
}

// finish records one completed page and reports it. It is a no-op when no
// callback is configured (the default), so an unwatched parse pays nothing.
func (p *pageProgress) finish() {
	if p.onDone == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	p.onDone(p.done, p.total)
}

const pageWorkerQueueFactor = 4

var (
	pagePoolOnce sync.Once
	pagePool     *utility.WorkerPool[pageTask, pageResult]
)

// defaultPageWorkerCount sizes the shared page worker pool from the
// process-wide page concurrency (N), resolved once at server start via
// SetPageConcurrency. N is independent of the process inference budget: page
// workers beyond DeepDocConcurrency() simply queue rendered bitmaps while they
// wait for an inference slot, so sizing the pool to N never over-subscribes
// inference.
func defaultPageWorkerCount() int {
	return PageConcurrency()
}

func parserPageWorkerPool() *utility.WorkerPool[pageTask, pageResult] {
	pagePoolOnce.Do(func() {
		workers := defaultPageWorkerCount()
		if workers <= 0 {
			workers = 1
		}
		pagePool = utility.NewWorkerPool(workers, workers*pageWorkerQueueFactor,
			func(ctx context.Context, task pageTask) (pageResult, error) {
				defer task.progress.finish()
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
// The pool is sized to the per-document page concurrency (N) at server start;
// this setter exists for runtime tuning and the throughput benchmark. N is
// independent of the process inference budget: a page worker not holding an
// inference slot only queues a rendered bitmap while it waits for one, so a
// larger pool does not over-subscribe inference. A size of zero or less is
// floored to 1, matching the Resize contract.
func SetPageWorkerPoolSize(workers int) {
	if workers <= 0 {
		workers = 1
	}
	parserPageWorkerPool().Resize(workers)
}

// ── Wrapped calls used by the parser pipeline ─────────────────────────────
//
// These wrappers guard health and shape, not the process inference budget: that
// budget is enforced inside the native backend, at the one boundary every ONNX
// Run passes through (see native.inference_limit.go), so a call site cannot
// escape it by not going through a wrapper.

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

// inferDLA invokes the per-page DLA call for a healthy analyzer. Page workers
// and enrichOnePageWithDeepDoc callers route through this wrapper so an
// unavailable analyzer degrades to "no regions" instead of an error.
func (p *Parser) inferDLA(ctx context.Context, doc pdf.DocAnalyzer, pageImg image.Image) ([]pdf.DLARegion, error) {
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	return doc.DLA(ctx, pageImg)
}

// reportPageInferenceFailure logs one page-local inference failure (DLA, TSR or
// OCR). A failure raised while the parse context is cancelled is the stop path,
// not a fault: cancelling terminates every in-flight ONNX Run, and the native
// session answers with the runtime's terminate-flag error (or ctx.Err()), which
// carries no context.Canceled to match on. Those pages log at debug instead of
// warning once per page; any other failure keeps its per-page warning.
func reportPageInferenceFailure(ctx context.Context, msg string, page int, err error) {
	if ctx.Err() != nil {
		common.Debug(msg, zap.Int("page", page), zap.Error(err))
		return
	}
	common.Warn(msg, zap.Int("page", page), zap.Error(err))
}

// inferTSR invokes TSR for a single cropped table region.
func (p *Parser) inferTSR(ctx context.Context, tb pdf.TableBuilder, cropped image.Image) ([]pdf.TSRCell, error) {
	if tb == nil {
		return nil, nil
	}
	return tb.DetectCells(ctx, cropped)
}

// inferOCRDetect invokes OCR detection for a healthy analyzer. ocrMergeChars and
// ocrDetectAndRecognize callers funnel through this helper.
func (p *Parser) inferOCRDetect(ctx context.Context, doc pdf.DocAnalyzer, pageImg image.Image) ([]pdf.OCRBox, error) {
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	return doc.OCRDetect(ctx, pageImg)
}

// inferOCRRecognize invokes OCR recognition for a healthy analyzer.
// Per-region OCR fallback paths (buildTextBoxes) use this wrapper.
func (p *Parser) inferOCRRecognize(ctx context.Context, doc pdf.DocAnalyzer, cropped image.Image) ([]pdf.OCRText, error) {
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	return doc.OCRRecognize(ctx, cropped)
}

// batchRecognizer is an OPTIONAL capability a pdf.DocAnalyzer may implement to
// recognize a page's OCR crops in one batched forward pass (see
// NativeAnalyzer.OCRRecognizeBatch). The production in-process backend
// implements it; test doubles (MockDocAnalyzer, replay
// analyzer) and any analyzer that prefers per-crop recognition do not.
// Callers MUST fall back to inferOCRRecognize when the analyzer does not
// implement it, so adding this capability never forces changes onto mocks or
// the parity-replay analyzer (which routes recognition by the box index
// stamped in ctx and is inherently per-crop).
type batchRecognizer interface {
	OCRRecognizeBatch(ctx context.Context, imgs []image.Image) ([][]pdf.OCRText, error)
}

// inferOCRRecognizeBatch recognizes a batch of crops in one call. It is only
// safe to call after a type assertion confirms doc implements batchRecognizer.
// The whole batch is a single ONNX Run, so it costs the process budget one
// inference slot rather than one per crop — a throughput win as well. An empty
// slice returns nil without touching the analyzer.
func (p *Parser) inferOCRRecognizeBatch(ctx context.Context, doc pdf.DocAnalyzer, crops []image.Image) ([][]pdf.OCRText, error) {
	br, ok := doc.(batchRecognizer)
	if !ok || len(crops) == 0 {
		return nil, nil
	}
	if doc == nil || !doc.Health() {
		return nil, nil
	}
	return br.OCRRecognizeBatch(ctx, crops)
}

// docSupportsBatchOCR reports whether doc implements the optional batched OCR
// recognition capability. Used by the OCR loop to choose between one batched
// Run and a per-crop fallback without paying for a redundant type assertion.
func (p *Parser) docSupportsBatchOCR(doc pdf.DocAnalyzer) bool {
	_, ok := doc.(batchRecognizer)
	return ok
}
