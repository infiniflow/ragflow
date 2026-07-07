//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package component — Parser component (Phase 2.2 of
// port-rag-flow-pipeline-to-go.md §4).
//
// SCOPE (honest):
//
//   - WHAT IS PORTED:
//
//   - The component's lifecycle contract: NewParserComponent /
//     Invoke / Parallelism / Inputs / Outputs and registration
//     under runtime.CategoryIngestion.
//
//   - The Python fan-out pattern from rag/flow/parser/parser.py
//     (parallel page parsing, deterministic merge by page number,
//     see plan §8 R8) — the Go implementation uses
//     golang.org/x/sync/errgroup with up to 4 goroutines and
//     bounds each fan-out batch by a "page_size" input (default
//     = ceil(total_pages / 4)).
//
//   - TrackProgress (start/done callback), WithTimeout (60s per
//     page-batch) and TrackElapsed (_created_time / _elapsed_time
//     stamping) — see internal/agent/runtime/helpers.go for the
//     helpers, plan §1 background.
//
//   - WHAT IS NOT YET PORTED:
//
//   - The Python component dispatches to 13 file-format branches
//     (pdf, markdown, text&code, html, spreadsheet, slides, doc,
//     docx, image, audio, video, email, epub) — see parser.py
//     function_map at line ~1273. The Go counterparts in
//     internal/parser/parser/ are SKELETONS that print to
//     stdout and return nil. The cgo-gated office variants
//     (docx, doc, ppt, pptx, xls, xlsx) call office_oxide but
//     discard the result.
//
//   - Until the parser package returns real data, the Go Parser
//     component uses a "raw text" fallback: it treats the input
//     binary as UTF-8 and slices it into 1 page (or N pages
//     when the upstream signals a page boundary with a literal
//     "\f" form feed). This is the conservative, observable
//     behaviour until the real format dispatch lands.
//
//   - The Python side's "image2id" pipeline (parser.py:1317-1329)
//     that uploads embedded images to MinIO is not replicated —
//     the schema layer carries images as opaque map values, and
//     the upload step is the responsibility of a separate
//     side-effect component (out of scope for Phase 2.2).
//
//   - The Python _param.check() business validation
//     (parse_method whitelist, output_format whitelist, etc.) is
//     not replicated. The component trusts the param block
//     passed in at construction time; invalid values surface as
//     runtime errors in the chosen parser branch.
//
//   - NO PERSISTENCE: parsed pages live only in the per-run
//     output map, exactly as the schema.Page type is intended.
package component

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

const ComponentNameParser = "Parser"

// parserParallelism is the fan-out degree for the Parser component.
// Matches the plan §2 AD-5a choice ("Parser: 4 (parallel page
// parsing)"). Used by the pipeline runner when it needs to know how
// many goroutines the component is willing to absorb.
const parserParallelism = 4

// parserPageBatchTimeout is the per-batch timeout. Mirrors the
// Python component's `@timeout(60)` decorator on the page parse
// branch. WithTimeout collapses the dual-layer
// asyncio.wait_for / @timeout model into a single context, see
// plan §8 R1.
const parserPageBatchTimeout = 60 * time.Second

// pageFormFeed is the byte that text-page mode treats as a
// hard page boundary. Matches the ASCII form feed (\f, 0x0C) — the
// same convention used by the Python TxtParser and by most
// "page-segmented text" codecs.
const pageFormFeed = '\f'

// ParserComponent runs the configured parser branch against the
// upstream "binary" payload and returns a deterministic, page-
// sorted slice of schema.Page values.
//
// The instance is safe for concurrent invocation: each Invoke call
// builds its own per-batch goroutine tree and merges results in
// the goroutine that returned from Invoke. The static Param is
// read-only after construction.
type ParserComponent struct {
	// Param is the static configuration from schema.ParserParam.
	// Kept as a value (not a pointer) so callers can pass literals
	// and the component makes its own copy.
	Param schema.ParserParam
}

// NewParserComponent constructs a Parser from a DSL param map.
// The map is decoded into schema.ParserParam.Defaults() and then
// overlaid with the supplied values. This matches the Python
// "default + override" pattern in parser.py:ParserParam.__init__.
//
// Param map shape (all keys optional; missing keys fall back to
// schema.ParserParam.Defaults() values):
//
//	{
//	  "setups":               map[string]map[string]any,
//	  "allowed_output_format": map[string][]string,
//	}
//
// Errors here surface as canvas compile failures so a malformed
// param is caught at build time rather than mid-run.
func NewParserComponent(params map[string]any) (runtime.Component, error) {
	p := schema.ParserParam{}.Defaults()
	if params == nil {
		return &ParserComponent{Param: p}, nil
	}
	// Setups — best-effort decode. A type mismatch in a single
	// setup entry drops just that entry; the rest of the table
	// remains usable. This matches the python behaviour of
	// accepting whatever shape the JSON loader hands back.
	if rawSetups, ok := params["setups"].(map[string]any); ok {
		for fileType, raw := range rawSetups {
			setupMap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if p.Setups == nil {
				p.Setups = make(map[string]schema.ParserSetup)
			}
			p.Setups[fileType] = schema.ParserSetup(setupMap)
		}
	}
	if rawAllowed, ok := params["allowed_output_format"].(map[string]any); ok {
		allowed := make(map[string][]string, len(rawAllowed))
		for fileType, raw := range rawAllowed {
			list, ok := raw.([]any)
			if !ok {
				continue
			}
			formats := make([]string, 0, len(list))
			for _, item := range list {
				if s, ok := item.(string); ok {
					formats = append(formats, s)
				}
			}
			allowed[fileType] = formats
		}
		p.AllowedOutputFormat = allowed
	}
	return &ParserComponent{Param: p}, nil
}

// Parallelism declares the goroutine fan-out degree. The pipeline
// runner uses this to decide how many worker slots the component
// can absorb. We return 4 to match the Python asyncio.gather
// pattern that fans one batch per page-range.
func (c *ParserComponent) Parallelism() int { return parserParallelism }

// Inputs returns the static parameter metadata. The component
// reads the following from the inputs map at Invoke time:
//
//	binary    ([]byte, optional) — file bytes from upstream File.
//	                                When absent, Parser resolves
//	                                them from bucket/path or doc_id.
//	doc_id    (string, optional) — document ID used for naming and,
//	                                when binary is absent, storage lookup.
//	page_size (int, optional)    — pages per goroutine for
//	                                fan-out. Defaults to
//	                                ceil(totalPages / Parallelism).
func (c *ParserComponent) Inputs() map[string]string {
	return map[string]string{
		"binary":    "Optional file bytes ([]byte). When absent, Parser resolves them from bucket/path or doc_id.",
		"doc_id":    "Optional document ID (string). Used for downstream correlation and doc_id-driven storage lookup.",
		"bucket":    "Optional storage bucket override. Used when binary is absent.",
		"path":      "Optional storage object key override. Used when binary is absent.",
		"page_size": "Optional integer. Pages per goroutine for fan-out. Default: ceil(totalPages / 4).",
	}
}

// Outputs returns the public surface that downstream ingestion
// components (Chunker, Tokenizer, Extractor) can wire into.
//
//	pages   []schema.Page — sorted by PageNumber. Deterministic
//	                        merge per plan §8 R8.
//	name    string        — carried over from the upstream file/document
//	                        name (or doc_id when no name is available).
//	output_format string  — "text" when emitting text pages,
//	                        otherwise the parser-selected wire
//	                        format.
//	_ERROR  string        — populated when the component short-
//	                        circuits with an error message
//	                        (mirrors Python set_output("_ERROR", ...)).
func (c *ParserComponent) Outputs() map[string]string {
	return map[string]string{
		"pages":         "[]schema.Page: parsed pages sorted by PageNumber.",
		"name":          "string: the upstream file/document name (or doc_id when no name is available).",
		"output_format": "string: the active output format (\"text\" when emitting text pages).",
		"_ERROR":        "string: set on short-circuit errors.",
	}
}

// Invoke runs the parser against the upstream "binary" payload.
//
// Returns:
//
//	{
//	  "pages":          []schema.Page (sorted by PageNumber),
//	  "name":           string (from inputs["doc_id"]),
//	  "output_format": "text",
//	  "_created_time":  RFC3339Nano (via TrackElapsed),
//	  "_elapsed_time":  float64 seconds (via TrackElapsed),
//	}
//
// The fan-out is bounded by Parallelism() goroutines. Each
// goroutine parses its page-batch under a derived timeout
// (WithTimeout, 60s). The first error cancels the errgroup
// context; siblings observe ctx.Done() and abandon their work.
//
// DETERMINISTIC MERGE (plan §8 R8): after fan-out, the page slice
// is sorted by PageNumber. This guarantees the same input
// produces byte-identical output across runs and is the contract
// that downstream Chunker / Tokenizer rely on for stable chunk
// IDs (chunks that span pages must reference adjacent PageNumbers
// in input order).
func (c *ParserComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// 1. Decode the binary input.
	binary, err := readParserBinary(ctx, inputs)
	if err != nil {
		return nil, err
	}
	docID, _ := inputs["doc_id"].(string)
	filename := parserInputName(inputs, docID)

	// 2. Resolve the file family from the inputs. When the family
	//    is known, dispatchParse returns a typed parser payload.
	//    Otherwise the component stays in text-page mode.
	//
	// We track TWO forms:
	//
	//   - fileTypeExt  — the utility.FileType extension form ("md",
	//     "docx", ...). Used by parser.GetParser, whose switch
	//     arms are keyed off the utility constants.
	//
	//   - fileTypeFam  — the python-side family name ("markdown",
	//     "docx", ...). Used by setups[fileType] and
	//     allowed_output_format[fileType] lookups, which are keyed
	//     off the python family identifiers in schema.ParserParam.
	//
	// For most families the two forms coincide; the divergence
	// exists for markdown ("md" vs "markdown") and slides
	// ("ppt"/"pptx" vs "slides") and is intentional — the python
	// ParserParam collapses the slide family into a single key.
	fileTypeExt := fileTypeFromInputs(inputs)
	fileTypeFam := pythonFamilyName(string(fileTypeExt))

	// 2a. Validate the requested output_format against the
	//     family-specific allowed_output_format whitelist. We do
	//     this even when no setups entry exists so a misconfigured
	//     DSL surfaces as _ERROR instead of a silent fallback.
	if _, hasSetup := c.Param.Setups[fileTypeFam]; hasSetup {
		if _, verr := resolveOutputFormat(fileTypeFam, c.Param.Setups, c.Param.AllowedOutputFormat); verr != nil {
			return nil, verr
		}
	}

	dispatched, handledVision, visionErr := maybeDispatchPDFVision(fileTypeExt, filename, binary, inputs, c.Param.Setups)
	if visionErr != nil {
		return nil, visionErr
	}
	if !handledVision {
		dispatched = dispatchParse(fileTypeExt, filename, binary, c.Param.Setups)
		dispatched = hydrateEmptyDispatchPayload(dispatched, binary)
	}
	// Known/supported families must fail loudly when dispatch or
	// parsing breaks. Only unknown families keep the raw-text fallback.
	if dispatched.Err != nil && fileTypeExt != utility.FileTypeOTHER {
		return nil, dispatched.Err
	}

	// 3. Build the legacy `pages` slice. When the dispatch path
	//    produced a JSON payload, we re-shape it into the page
	//    layout the chunker side consumes (`{text, doc_type_kwd,
	//    page_number?}`); when the dispatch produced a string
	//    payload we emit a single page carrying the rendered text;
	//    otherwise we slice the binary on ASCII form-feed and
	//    treat the input as text pages.
	var pages [][]byte
	var dispatchedPages []schema.Page
	switch {
	case dispatched.Err == nil && dispatched.OutputFormat == "json" && len(dispatched.JSON) > 0:
		dispatchedPages = jsonItemsToPages(dispatched.JSON)
		pages = pagesFromDispatch(dispatchedPages)
	case dispatched.Err == nil && dispatched.OutputFormat != "":
		var text string
		switch dispatched.OutputFormat {
		case "markdown":
			text = dispatched.Markdown
		case "html":
			text = dispatched.HTML
		case "text":
			text = dispatched.Text
		}
		pages = [][]byte{[]byte(text)}
	default:
		pages = splitIntoPages(binary)
		if len(pages) == 0 {
			pages = [][]byte{nil}
		}
	}
	totalPages := len(pages)

	// 4. Fan-out: split pages into batches of pageSize. The
	//    default pageSize is ceil(totalPages / Parallelism),
	//    matching the plan §2 AD-5a target.
	pageSize := resolvePageSize(inputs, totalPages)
	batches := splitIntoBatches(pages, pageSize)

	// 5. Drive the fan-out from TrackProgress (which delivers the
	//    start/done/fail callback sequence); stamp
	//    _created_time / _elapsed_time on the result via
	//    TrackElapsed without re-running the work.
	var out map[string]any
	progressErr := runtime.TrackProgress(ComponentNameParser, nil, func() error {
		parsed, err := fanOutAndMerge(ctx, batches, parserParallelism)
		if err != nil {
			return err
		}
		// Sort by PageNumber — DETERMINISTIC MERGE (plan §8 R8).
		sortPagesByNumber(parsed)
		out = buildParserOutputs(parsed, dispatched, filename, fileTypeExt)
		return nil
	})
	if progressErr != nil {
		return nil, fmt.Errorf("Parser: %w", progressErr)
	}
	// Stamp _created_time / _elapsed_time. We pass a closure
	// that returns the pre-built `out` so the helper does not
	// re-execute the fan-out.
	return runtime.TrackElapsed(ComponentNameParser, func() (map[string]any, error) {
		return out, nil
	})
}

// fanOutAndMerge parses each batch in parallel and concatenates
// the per-batch results. The first error cancels the errgroup
// context; siblings see ctx.Done() and abandon their parse
// (returning ctx.Err()).
//
// Concurrency model: at most `parallelism` goroutines run
// concurrently. errgroup.WithContext provides the cancel-on-
// first-error behaviour, and golang.org/x/sync/errgroup is
// already in go.mod (line 59).
func fanOutAndMerge(parent context.Context, batches [][][]byte, parallelism int) ([]schema.Page, error) {
	if len(batches) == 0 {
		return nil, nil
	}
	if parallelism < 1 {
		parallelism = 1
	}
	g, ctx := errgroup.WithContext(parent)
	g.SetLimit(parallelism)

	// One slot per batch; we collect the results in order so the
	// caller can sort the merged slice by PageNumber without
	// needing a mutex.
	results := make([][]schema.Page, len(batches))
	for i, batch := range batches {
		i, batch := i, batch
		g.Go(func() error {
			pages, err := parseBatch(ctx, batch)
			if err != nil {
				return err
			}
			results[i] = pages
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	// Flatten in batch order. The caller is responsible for
	// sorting by PageNumber — we deliberately do NOT sort here
	// so the per-batch order is visible to tests.
	total := 0
	for _, r := range results {
		total += len(r)
	}
	merged := make([]schema.Page, 0, total)
	for _, r := range results {
		merged = append(merged, r...)
	}
	return merged, nil
}

// parseBatch parses a single batch of pages under a derived
// 60-second timeout. The batch is the unit of fan-out: if a
// batch exceeds its timeout, ONLY that batch errors; siblings
// see ctx.Done() and abandon their work (errgroup cancel
// cascades).
//
// runtime.WithTimeout returns just an error; we capture the
// result pages in a closure-scoped variable and read it back
// after Wait. This keeps the helper at its single-purpose
// signature (ctx, fn -> error) without growing the runtime API.
func parseBatch(ctx context.Context, batch [][]byte) ([]schema.Page, error) {
	var pages []schema.Page
	err := runtime.WithTimeout(ctx, parserPageBatchTimeout, func(ctx context.Context) error {
		pages = make([]schema.Page, 0, len(batch))
		for _, raw := range batch {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			// Text-page mode: the bytes are already page text.
			pages = append(pages, schema.Page{
				"text":         string(raw),
				"doc_type_kwd": "text",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pages, nil
}

// --- input helpers ---

// readParserBinary pulls the "binary" payload out of the inputs
// map. The accepted shapes are:
//
//	[]byte          — the in-process caller's normal form
//	string          — UTF-8 text (json callers' normal form)
//	nil / absent    — returns an empty page (not an error)
//
// A non-UTF-8 string is rejected with a clear error so a caller
// that mistakenly hands a base64 string sees the failure
// immediately (mirrors pipeline_chunker's "no try-base64" rule).
func readParserBinary(ctx context.Context, inputs map[string]any) ([]byte, error) {
	if inputs == nil {
		return nil, nil
	}
	if b, ok := inputs["binary"].([]byte); ok {
		return b, nil
	}
	if s, ok := inputs["binary"].(string); ok {
		if !utf8.ValidString(s) {
			return nil, errors.New(
				"Parser: binary string is not valid UTF-8. " +
					"Text-page mode only accepts UTF-8 text input.")
		}
		return []byte(s), nil
	}
	bucket, _ := getString(inputs, "bucket")
	path, _ := getString(inputs, "path")
	if bucket != "" && path != "" {
		return fetchBinary(ctx, bucket, path)
	}
	if docID, ok := getString(inputs, "doc_id"); ok && docID != "" {
		ref, err := resolveDocumentStorage(docID)
		if err != nil {
			return nil, fmt.Errorf("Parser: resolve doc_id %q: %w", docID, err)
		}
		return fetchBinary(ctx, ref.Bucket, ref.Path)
	}
	return nil, nil
}

// splitIntoPages segments the input bytes on ASCII form-feed
// (\f, 0x0C). An input with no form-feeds becomes a single page
// (the whole input). Empty pages are dropped — the python
// TxtParser skips empty splits the same way.
func splitIntoPages(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	// Fast path: no form-feeds → single page.
	if !containsFormFeed(b) {
		return [][]byte{b}
	}
	parts := strings.Split(string(b), string(pageFormFeed))
	out := make([][]byte, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, []byte(p))
	}
	return out
}

// containsFormFeed is a tiny specialised byte-search to avoid
// pulling in bytes.Index for one call site.
func containsFormFeed(b []byte) bool {
	for _, c := range b {
		if c == pageFormFeed {
			return true
		}
	}
	return false
}

// splitIntoBatches partitions the page slice into batches of
// `size` consecutive pages. A non-positive size collapses to
// one batch.
func splitIntoBatches(pages [][]byte, size int) [][][]byte {
	if size < 1 {
		size = len(pages)
	}
	if size < 1 {
		return nil
	}
	batches := make([][][]byte, 0, (len(pages)+size-1)/size)
	for i := 0; i < len(pages); i += size {
		end := i + size
		if end > len(pages) {
			end = len(pages)
		}
		batch := make([][]byte, end-i)
		copy(batch, pages[i:end])
		batches = append(batches, batch)
	}
	return batches
}

// resolvePageSize returns the inputs["page_size"] value when
// valid, otherwise ceil(totalPages / Parallelism). A page_size
// of 0 or 1 is treated as "use the default" so a caller that
// sets page_size=1 to mean "no batching" still fans out across
// `Parallelism` goroutines.
func resolvePageSize(inputs map[string]any, totalPages int) int {
	if inputs != nil {
		if v, ok := inputs["page_size"].(int); ok && v > 1 {
			return v
		}
		if v, ok := inputs["page_size"].(int64); ok && v > 1 {
			return int(v)
		}
		if v, ok := inputs["page_size"].(float64); ok && v > 1 {
			return int(v)
		}
	}
	if totalPages < 1 {
		return 1
	}
	// ceil(totalPages / Parallelism)
	size := (totalPages + parserParallelism - 1) / parserParallelism
	if size < 1 {
		size = 1
	}
	return size
}

// sortPagesByNumber orders pages by their PageNumber key
// ascending. Pages without a PageNumber key (or with a non-int
// value) sort to the END so the deterministic contract is
// "numbered pages first, then unnumbered" — this matches the
// Python component's loop order (it processes pages in input
// order, not in PageNumber order, but the Go merge is
// intentionally stricter so the test can assert exact byte
// equality across runs).
func sortPagesByNumber(pages []schema.Page) {
	sort.SliceStable(pages, func(i, j int) bool {
		pi, oki := numericPageNumber(pages[i])
		pj, okj := numericPageNumber(pages[j])
		switch {
		case oki && okj:
			return pi < pj
		case oki:
			return true // i is numbered, j is not
		case okj:
			return false
		default:
			return false // stable
		}
	})
}

func numericPageNumber(p schema.Page) (int, bool) {
	if p == nil {
		return 0, false
	}
	v, ok := p["page_number"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// toAnyPages is a tiny adapter that hands the page slice to
// the output map as `any`. We use it instead of a direct cast
// so the type stays `[]schema.Page` in the Go source and the
// output map value type is `any` — matching the runtime.Component
// contract.
func toAnyPages(pages []schema.Page) any { return pages }

// init registers Parser under CategoryIngestion per plan §4
// Phase 2.2. The factory is a thin closure that decodes the
// DSL param map; the static Metadata is derived from
// Inputs()/Outputs() on a zero-value instance.
func init() {
	pc := &ParserComponent{}
	runtime.MustRegister(ComponentNameParser, runtime.CategoryIngestion,
		func(_ string, params map[string]any) (runtime.Component, error) {
			return NewParserComponent(params)
		},
		runtime.Metadata{
			Version: "1.0.0",
			Inputs:  pc.Inputs(),
			Outputs: pc.Outputs(),
		})
}
