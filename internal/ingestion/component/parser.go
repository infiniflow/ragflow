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
//     Invoke / Inputs / Outputs and registration under
//     runtime.CategoryIngestion.
//
//   - Per-page parallelism is delegated to the parser backends
//     (e.g. internal/deepdoc/parser/pdf fans out one worker per
//     page and assembles the results in page order). This
//     component only reshapes the parser output into the
//     schema.Page layout and keeps the deterministic, page-number
//     sorted merge contract (plan §8 R8) that the downstream
//     chunker / tokenizer rely on for stable chunk IDs.
//
//   - Progress (start/done callback) and elapsed-time stamping
//     (_created_time / _elapsed_time) are owned by the canvas
//     framework (internal/agent/canvas/node_body.go realComponentBody),
//     which wraps every component Invoke. This component does not call
//     those helpers itself. See internal/agent/runtime/helpers.go.
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
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

const ComponentNameParser = "Parser"

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
	Setups map[string]schema.ParserSetup
	Param  schema.ParserParam
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
//	  "pdf":                  map[string]any,
//	  "docx":                 map[string]any,
//	  ...
//	  "allowed_output_format": map[string][]string,
//	}
//
// Errors here surface as canvas compile failures so a malformed
// param is caught at build time rather than mid-run.
func NewParserComponent(params map[string]any) (runtime.Component, error) {
	p := schema.ParserParam{}.Defaults()
	s := defaultSetups()
	if params == nil {
		return &ParserComponent{Setups: s, Param: p}, nil
	}
	for k, raw := range params {
		if k == "outputs" {
			continue
		}
		ftCfg, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := s[k]; !exists {
			s[k] = schema.ParserSetup{}
		}
		for fk, fv := range ftCfg {
			s[k][fk] = fv
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
	return &ParserComponent{Setups: s, Param: p}, nil
}

func defaultSetups() map[string]schema.ParserSetup {
	return map[string]schema.ParserSetup{
		"pdf": {
			"parse_method":          "deepdoc",
			"lang":                  "Chinese",
			"flatten_media_to_text": false,
			"remove_toc":            false,
			"remove_header_footer":  false,
			"suffix":                []string{"pdf"},
			"output_format":         "json",
		},
		"spreadsheet": {
			"parse_method":          "deepdoc",
			"flatten_media_to_text": false,
			"output_format":         "html",
			"suffix":                []string{"xls", "xlsx", "csv"},
		},
		"doc": {
			"remove_toc":           false,
			"remove_header_footer": false,
			"suffix":               []string{"doc"},
			"output_format":        "json",
		},
		"docx": {
			"flatten_media_to_text": false,
			"remove_toc":            false,
			"remove_header_footer":  false,
			"suffix":                []string{"docx"},
			"output_format":         "json",
		},
		"markdown": {
			"flatten_media_to_text": false,
			"suffix":                []string{"md", "markdown", "mdx"},
			"remove_toc":            false,
			"output_format":         "json",
		},
		"text&code": {
			"suffix": []string{
				"txt", "py", "js", "java", "c", "cpp", "h", "php",
				"go", "ts", "sh", "cs", "kt", "sql",
			},
			"output_format": "json",
		},
		"html": {
			"suffix":               []string{"htm", "html"},
			"remove_toc":           false,
			"remove_header_footer": false,
			"output_format":        "json",
		},
		"slides": {
			"parse_method":  "deepdoc",
			"suffix":        []string{"pptx", "ppt"},
			"output_format": "json",
		},
		"image": {
			"parse_method":  "ocr",
			"llm_id":        "",
			"lang":          "Chinese",
			"system_prompt": "",
			"suffix":        []string{"jpg", "jpeg", "png", "gif"},
			"output_format": "json",
		},
		"email": {
			"suffix": []string{"eml", "msg"},
			"fields": []string{
				"from", "to", "cc", "bcc", "date", "subject",
				"body", "attachments", "metadata",
			},
			"output_format": "text",
		},
		"audio": {
			"suffix": []string{
				"da", "wave", "wav", "mp3", "aac", "flac", "ogg",
				"aiff", "au", "midi", "wma", "realaudio", "vqf",
				"oggvorbis", "ape",
			},
			"output_format": "text",
		},
		"video": {
			"suffix":        []string{"mp4", "avi", "mkv"},
			"output_format": "text",
			"prompt":        "",
		},
		"epub": {
			"suffix":        []string{"epub"},
			"output_format": "json",
		},
		"json": {
			"suffix":        []string{"json", "jsonl", "ldjson"},
			"output_format": "json",
		},
	}
}

// Inputs returns the static parameter metadata. The component
// reads the following from the inputs map at Invoke time:
//
//	binary    ([]byte, optional) — file bytes from upstream File.
//	                                When absent, Parser resolves
//	                                them from bucket/path or doc_id.
//	doc_id    (string, optional) — document ID used for naming and,
//	                                when binary is absent, storage lookup.
func (c *ParserComponent) Inputs() map[string]string {
	return map[string]string{
		"binary": "Optional file bytes ([]byte). When absent, Parser resolves them from bucket/path or doc_id.",
		"doc_id": "Optional document ID (string). Used for downstream correlation and doc_id-driven storage lookup.",
		"bucket": "Optional storage bucket override. Used when binary is absent.",
		"path":   "Optional storage object key override. Used when binary is absent.",
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
// Per-page parallelism and aggregation now live in the parser
// backends (e.g. internal/deepdoc/parser/pdf fans out one worker
// per page and assembles the results in page order), so this
// component does no goroutine fan-out of its own.
//
// DETERMINISTIC MERGE (plan §8 R8): after the page slice is built,
// it is sorted by PageNumber. This guarantees the same input
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

	// Inject run-level metadata from Globals into inputs so media
	// dispatch branches (audio/image/video) can resolve tenant_id.
	// The File component upstream does not emit tenant_id; the pipeline
	// runner seeds it into CanvasState.Globals, and the Parser must pull
	// it back into the local inputs map for the dispatch functions.
	if tid := globals.GlobalOrInput(ctx, inputs, "tenant_id", ""); tid != "" {
		inputs["tenant_id"] = tid
	}

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
	if _, hasSetup := c.Setups[fileTypeFam]; hasSetup {
		if _, verr := resolveOutputFormat(fileTypeFam, c.Setups, c.Param.AllowedOutputFormat); verr != nil {
			return nil, verr
		}
	}

	dispatched, handledVision, visionErr := maybeDispatchPDFVision(fileTypeExt, filename, binary, inputs, c.Setups)
	if visionErr != nil {
		return nil, visionErr
	}

	var handledMedia bool
	if !handledVision {
		// Video dispatch: IMAGE2TEXT vision chat.
		// Mirrors Python's _video().
		dispatched, handledMedia, visionErr = maybeDispatchVideo(fileTypeExt, filename, binary, inputs, c.Setups)
		if visionErr != nil {
			return nil, visionErr
		}
	}
	var handledImage bool
	if !handledVision && !handledMedia {
		// Image/Picture dispatch: OCR + IMAGE2TEXT vision describe.
		// Mirrors Python's rag/app/picture.py:chunk() image branch.
		dispatched, handledImage, visionErr = maybeDispatchImage(fileTypeExt, filename, binary, inputs, c.Setups)
		if visionErr != nil {
			return nil, visionErr
		}
	}
	var handledAudio bool
	if !handledVision && !handledMedia && !handledImage {
		// Audio dispatch: SPEECH2TEXT transcription.
		// Mirrors Python's rag/app/audio.py:chunk().
		dispatched, handledAudio, visionErr = maybeDispatchAudio(fileTypeExt, filename, binary, inputs, c.Setups)
		if visionErr != nil {
			return nil, visionErr
		}
	}
	if !handledVision && !handledMedia && !handledImage && !handledAudio {
		dispatched = dispatchParse(fileTypeExt, filename, binary, c.Setups)
		dispatched = hydrateEmptyDispatchPayload(dispatched, binary)

		// DOCX vision figure enhancement: enrich the markdown
		// with LLM-generated descriptions of embedded images.
		// Mirrors Python's vision_figure_parser_docx_wrapper_naive.
		dispatched, _, _ = maybeDispatchDOCXVision(fileTypeExt, dispatched, inputs, c.Setups)

		// Markdown vision figure enhancement: enrich parsed
		// markdown JSON items with LLM-generated descriptions of
		// referenced images (![alt](url)). Mirrors Python's
		// enhance_media_sections_with_vision in _markdown.
		dispatched, _, _ = maybeDispatchMarkdownVision(fileTypeExt, dispatched, inputs)
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

	// 4. Build the page slice sequentially. Per-page parallelism now
	//    lives in the parser backends (e.g. internal/deepdoc/parser/pdf
	//    fans out one worker per page and assembles in page order), so
	//    this component only reshapes the parser output. The DETERMINISTIC
	//    MERGE (plan §8 R8) keeps pages sorted by PageNumber so the
	//    downstream chunker / tokenizer get stable chunk IDs.
	parsed, err := buildPagesFromBytes(ctx, pages, dispatched.DocType)
	if err != nil {
		return nil, fmt.Errorf("Parser: %w", err)
	}
	sortPagesByNumber(parsed)
	out := buildParserOutputs(parsed, dispatched, filename, fileTypeExt)
	// Forward the storage references so a downstream chunker can
	// re-acquire the source PDF and crop section images on demand,
	// instead of carrying the binary across the component boundary.
	if docID != "" {
		out["doc_id"] = docID
	}
	if bucket, _ := getString(inputs, "bucket"); bucket != "" {
		out["bucket"] = bucket
	}
	if path, _ := getString(inputs, "path"); path != "" {
		out["path"] = path
	}
	// Publish the resolved run-level metadata into the workflow-wide
	// CanvasState.Globals bag so downstream components read it from ctx
	// instead of relying on this output re-emitting it. The Go runtime
	// forwards only this explicit output to the next node, so shared
	// fields must live in Globals.
	globals.PublishGlobals(ctx, out)
	// Progress (_created_time / _elapsed_time stamping, start/done
	// callbacks) is owned by the canvas framework (realComponentBody),
	// not by this component, so we return the work result directly.
	return out, nil
}

// buildPagesFromBytes reshapes already-prepared page bytes into the
// schema.Page layout the downstream chunker consumes. The per-page
// parse (including any parallelism) now lives in the parser backends
// (internal/parser/parser and internal/deepdoc/parser/pdf); this
// component only wraps the bytes into pages and honors context
// cancellation so an abandoned run does not keep reshaping pages.
//
// The function is format-agnostic: it does not resolve parsers or
// inspect file families — it only carries the raw bytes under the
// "text" key with the given doc_type_kwd (defaults to "text" when
// empty), matching the shape downstream readers expect from the
// raw-text fallback and dispatch paths.
func buildPagesFromBytes(ctx context.Context, pages [][]byte, docType string) ([]schema.Page, error) {
	if docType == "" {
		docType = "text"
	}
	out := make([]schema.Page, 0, len(pages))
	for _, raw := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, schema.Page{
			"text":         string(raw),
			"doc_type_kwd": docType,
		})
	}
	return out, nil
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
		return FetchBinary(ctx, bucket, path)
	}
	if docID, ok := getString(inputs, "doc_id"); ok && docID != "" {
		ref, err := ResolveDocumentStorage(docID)
		if err != nil {
			return nil, fmt.Errorf("Parser: resolve doc_id %q: %w", docID, err)
		}
		return FetchBinary(ctx, ref.Bucket, ref.Path)
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
