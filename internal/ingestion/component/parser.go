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
//     (pdf, Markdown, text&code, html, spreadsheet, slides, doc,
//     docx, image, audio, video, email, epub) — see parser.py
//     function_map at line ~1273. The Go port is LANDed and LIVE
//     in production for the families the ingestor claims (see
//     cmd/ragflow_server.go: Ingestor.supportedTypes =
//     ["pdf","docx","txt"]); those run their real parsers,
//     including the cgo-gated office variants via office_oxide.
//     Families not yet ported fall through to the raw-text path
//     below rather than printing skeletons.
//
//   - For any family NOT yet ported (its Go parser returns no real
//     data), the component uses a "raw text" fallback: it treats
//     the input binary as UTF-8 and slices it into 1 page (or N
//     pages when the upstream signals a page boundary with a literal
//     "\f" form feed). This is the conservative, observable
//     behaviour for UNPORTED families only; ported families run
//     their real parsers.
//
//   - The Python side's "image2id" pipeline (parser.py:1317-1329)
//     that uploads embedded images to MinIO is not replicated —
//     the schema layer carries images as opaque map values, and
//     the upload step is the responsibility of a separate
//     side-effect component (out of scope for Phase 2.2).
//
//   - The Python _param.check() business validation
//     (parse_method whitelist, conditional lang checks) is mirrored
//     by (*ParserComponent).Check() below, which NewParserComponent
//     runs at construction time. The Python flow check() also
//     validates audio/video vlm.llm_id, but Go media_dispatch uses
//     tenant default models (resolveTenantModelByType) rather than
//     setup["vlm"]["llm_id"], so that check is intentionally omitted.
//
//   - NO PERSISTENCE: parsed pages live only in the per-run
//     output map, exactly as the schema.Page type is intended.
package component

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
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
// upstream "binary" payload and returns structured parser outputs.
//
// The instance is safe for concurrent invocation: each Invoke call
// builds its own per-batch goroutine tree and merges results in
// the goroutine that returned from Invoke. The static Param is
// read-only after construction.
type ParserComponent struct {
	Setups map[string]schema.ParserSetup
}

// NewParserComponent constructs a Parser from a DSL param map.
// The default setups are overlaid with the supplied values. Historical
// output_format values are accepted but normalized to JSON so downstream
// components consume one parser output protocol.
//
// Param map shape (all keys optional):
//
//	{
//	  "pdf":                  map[string]any,
//	  "docx":                 map[string]any,
//	  ...
//	}
//
// Errors here surface as canvas compile failures so a malformed
// param is caught at build time rather than mid-run.
func NewParserComponent(params map[string]any) (runtime.Component, error) {
	s := defaultSetups()
	if params == nil {
		normalizeParserOutputFormats(s)
		return &ParserComponent{Setups: s}, nil
	}
	for k, raw := range params {
		if k == "outputs" || k == "allowed_output_format" {
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
	normalizeParserOutputFormats(s)
	pc := &ParserComponent{Setups: s}
	if err := pc.Check(); err != nil {
		return nil, fmt.Errorf("parser: %w", err)
	}
	return pc, nil
}

func normalizeParserOutputFormats(setups map[string]schema.ParserSetup) {
	for _, setup := range setups {
		setup["output_format"] = "json"
	}
}

func cloneParserSetups(setups map[string]schema.ParserSetup) map[string]schema.ParserSetup {
	cloned := make(map[string]schema.ParserSetup, len(setups))
	for family, setup := range setups {
		clonedSetup := make(schema.ParserSetup, len(setup)+1)
		for key, value := range setup {
			clonedSetup[key] = value
		}
		cloned[family] = clonedSetup
	}
	normalizeParserOutputFormats(cloned)
	return cloned
}

// Check mirrors the applicable subset of Python ParserParam.check()
// (rag/flow/parser/parser.py:251-321). Runs at construction time so
// a malformed DSL surfaces as a canvas compile failure rather than a
// mid-run error. Returns the first validation error encountered
// (Python raises ValueError on the first failure).
//
// NOT covered here (intentional):
//   - audio/video vlm.llm_id: Go media_dispatch uses tenant default
//     models (resolveTenantModelByType), not setup["vlm"]["llm_id"].
//     The Python flow check() for vlm.llm_id does not apply — Go
//     never reads that field, and validating it would block every
//     valid audio/video pipeline (see ingestion_pipeline_audio.json).
func (c *ParserComponent) Check() error {
	// PDF family (parser.py:252-261).
	if pdf, ok := c.Setups["pdf"]; ok {
		pm, _ := pdf["parse_method"].(string)
		if pm == "" {
			return errors.New("parse method abnormal. does not support empty value")
		}
		pmLower := strings.ToLower(pm)
		pdfWhitelist := []string{
			"deepdoc", "plain_text", "mineru", "monkeyocrv2", "docling",
			"opendataloader", "tcadp parser", "paddleocr", "somark",
		}
		if !containsString(pdfWhitelist, pmLower) {
			// Non-whitelist parse_method is treated as a VLM method,
			// which requires lang (Python parser.py:257-258).
			if lang, _ := pdf["lang"].(string); lang == "" {
				return errors.New("PDF VLM language does not support empty value")
			}
		}
	}
	// image family (parser.py:283-287).
	if img, ok := c.Setups["image"]; ok {
		pm, _ := img["parse_method"].(string)
		// OCR mode does not need a VLM language; any other value does.
		if pm != "ocr" {
			if lang, _ := img["lang"].(string); lang == "" {
				return errors.New("image VLM language does not support empty value")
			}
		}
	}
	return nil
}

// containsString reports whether s is in list. Used by Check() for
// whitelist membership tests; kept unexported and local to this file
// to avoid polluting the package namespace.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
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
			"output_format":         "json",
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
			"output_format": "json",
		},
		"audio": {
			"suffix": []string{
				"da", "wave", "wav", "mp3", "aac", "flac", "ogg",
				"aiff", "au", "midi", "wma", "realaudio", "vqf",
				"oggvorbis", "ape",
			},
			"output_format": "json",
		},
		"video": {
			"suffix":        []string{"mp4", "avi", "mkv"},
			"output_format": "json",
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
//	name          string  — carried over from the upstream file/document
//	                        name (or doc_id when no name is available).
//	file_type     string  — canonical parser-resolved file extension.
//	output_format string  — always "json".
//	lang          string  — language for tokenization.
//	_ERROR        string  — populated when the component short-
//	                        circuits with an error message
//	                        (mirrors Python set_output("_ERROR", ...)).
func (c *ParserComponent) Outputs() map[string]string {
	return map[string]string{
		"name":          "string: the upstream file/document name (or doc_id when no name is available).",
		"file_type":     "string: canonical parser-resolved file extension.",
		"output_format": "string: always \"json\".",
		"lang":          "string: the language for tokenization (e.g. English, Dutch, Chinese).",
		"_ERROR":        "string: set on short-circuit errors.",
	}
}

// Invoke runs the parser against the upstream "binary" payload.
//
// Returns:
//
//	{
//	  "name":           string (from inputs["doc_id"]),
//	  "output_format": "json",
//	  "lang":           string (from inputs["lang"]; e.g. English, Dutch),
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
func (c *ParserComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	// 1. Decode the binary input.
	binary, err := readParserBinary(ctx, db, inputs)
	if err != nil {
		return nil, err
	}
	docID, _ := inputs["doc_id"].(string)
	filename := parserInputName(inputs, docID)
	setups := cloneParserSetups(c.Setups)

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
	fileTypeExt := fileTypeFromInputs(inputs)

	dispatched, handledVision, visionErr := maybeDispatchPDFVision(ctx, db, fileTypeExt, filename, binary, inputs, setups)
	if visionErr != nil {
		return nil, visionErr
	}

	var handledMedia bool
	if !handledVision {
		// Video dispatch: IMAGE2TEXT vision chat.
		// Mirrors Python's _video().
		dispatched, handledMedia, visionErr = maybeDispatchVideo(ctx, db, fileTypeExt, filename, binary, inputs, setups)
		if visionErr != nil {
			return nil, visionErr
		}
	}
	var handledImage bool
	if !handledVision && !handledMedia {
		// Image/Picture dispatch: OCR + IMAGE2TEXT vision describe.
		// Mirrors Python's rag/app/picture.py:chunk() image branch.
		dispatched, handledImage, visionErr = maybeDispatchImage(ctx, db, fileTypeExt, filename, binary, inputs, setups)
		if visionErr != nil {
			return nil, visionErr
		}
	}
	var handledAudio bool
	if !handledVision && !handledMedia && !handledImage {
		// Audio dispatch: SPEECH2TEXT transcription.
		// Mirrors Python's rag/app/audio.py:chunk().
		dispatched, handledAudio, visionErr = maybeDispatchAudio(ctx, db, fileTypeExt, filename, binary, inputs, setups)
		if visionErr != nil {
			return nil, visionErr
		}
	}
	if !handledVision && !handledMedia && !handledImage && !handledAudio {
		dispatched = dispatchParse(ctx, fileTypeExt, filename, binary, setups)

		// Vision figure enhancement: on the JSON output path,
		// append vision-model descriptions to embedded image and
		// table items. Mirrors Python's enhance_media_sections_with_vision
		// (rag/flow/parser/utils.py:162, called at parser.py:772/978/1115).
		// Errors (including context cancellation) are intentionally
		// discarded — enhancement is best-effort, matching Python's
		// try/except pass pattern.
		dispatched, _, _ = maybeDispatchVisionEnhancement(ctx, db, fileTypeExt, dispatched, inputs, setups)
	}
	// Known/supported families must fail loudly when dispatch or
	// parsing breaks. Only unknown families keep the raw-text fallback.
	if dispatched.Err != nil && fileTypeExt != utility.FileTypeOTHER {
		return nil, dispatched.Err
	}
	reportParserWarnings(ctx, dispatched.Warnings)

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("parser: %w", err)
	}
	lang, _ := getString(inputs, "lang")
	out := buildParserOutputs(dispatched, filename, fileTypeExt, binary, lang)
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
	// Debug log: summarize parser output for pipeline debugging.
	if dispatched.OutputFormat == "json" {
		common.Debug("parser stage output",
			zap.String("component", "Parser"),
			zap.String("output_format", "json"),
			zap.Int("json_items", len(dispatched.JSON)),
		)
	} else if dispatched.OutputFormat != "" {
		common.Debug("parser stage output",
			zap.String("component", "Parser"),
			zap.String("output_format", dispatched.OutputFormat),
		)
	}
	// Progress (_created_time / _elapsed_time stamping, start/done
	// callbacks) is owned by the canvas framework (realComponentBody),
	// not by this component, so we return the work result directly.
	return out, nil
}

func reportParserWarnings(ctx context.Context, warnings []string) {
	for _, warning := range warnings {
		runtime.ReportProgressMessage(ctx, "Parser", "WARNING: "+warning)
	}
}

// --- input helpers ---

// readParserBinary pulls the "binary" payload out of the inputs
// map. The accepted shapes are:
//
//	[]byte          — the in-process caller's normal form
//	string          — UTF-8 text (JSON callers' normal form)
//	nil / absent    — returns an empty page (not an error)
//
// A non-UTF-8 string is rejected with a clear error so a caller
// that mistakenly hands a base64 string sees the failure
// immediately (mirrors pipeline_chunker's "no try-base64" rule).
func readParserBinary(ctx context.Context, db *gorm.DB, inputs map[string]any) ([]byte, error) {
	if inputs == nil {
		return nil, nil
	}
	if b, ok := inputs["binary"].([]byte); ok {
		return b, nil
	}
	if s, ok := inputs["binary"].(string); ok {
		if !utf8.ValidString(s) {
			return nil, errors.New(
				"parser: binary string is not valid UTF-8. " +
					"Text-page mode only accepts UTF-8 text input")
		}
		return []byte(s), nil
	}
	bucket, _ := getString(inputs, "bucket")
	path, _ := getString(inputs, "path")
	if bucket != "" && path != "" {
		return FetchBinary(ctx, bucket, path)
	}
	if docID, ok := getString(inputs, "doc_id"); ok && docID != "" {
		ref, err := ResolveDocumentStorage(ctx, db, docID)
		if err != nil {
			return nil, fmt.Errorf("parser: resolve doc_id %q: %w", docID, err)
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
