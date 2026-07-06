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

// Parser dispatch validates the requested output format, resolves the
// parser backend, and returns the structured ParseWithResult payload.
//
// `parse_method` is carried through file metadata for downstream
// consumers, while the actual backend work stays in
// internal/parser/parser/*.

package component

import (
	"fmt"
	"maps"
	"strings"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

// parserDispatchResult is the typed outcome of dispatchParse. The
// component's Invoke translates it into the runtime output map.
//
// OutputFormat is the wire format the parser actually emitted. It
// always matches setups[fileType].output_format on success; on a
// format-mismatch failure (the whitelist rejects it) OutputFormat is
// empty and Err is non-nil.
//
// File is the per-parser file metadata and may be nil.
//
// Payload holds exactly one populated field per the ParseResult
// contract (see internal/parser/parser/parse_result.go).
type parserDispatchResult struct {
	OutputFormat string
	File         map[string]any
	JSON         []map[string]any
	Markdown     string
	Text         string
	HTML         string
	Err          error
}

type parserSetupConfigurer interface {
	ConfigureFromSetup(setup map[string]any)
}

func resolveParserFamily(fileType utility.FileType) string {
	if family := pythonFamilyName(string(fileType)); family != "" {
		return family
	}
	return string(fileType)
}

func configureParserFromSetups(p any, fileType utility.FileType, setups map[string]schema.ParserSetup) {
	cfg, ok := p.(parserSetupConfigurer)
	if !ok {
		return
	}
	family := resolveParserFamily(fileType)
	setup, ok := setups[family]
	if !ok {
		return
	}
	cfg.ConfigureFromSetup(map[string]any(setup))
}

// resolveOutputFormat picks the wire format for this run. The
// Python side asks the setup, then checks the value is in
// allowed_output_format[fileType]. We mirror that exact sequence:
//
//  1. setups[fileType].output_format (or "" when absent),
//  2. if absent, default to "text" (the most permissive option
//     that every family accepts),
//  3. if absent and the family has no allowed_output_format entry,
//     return "" (the component falls back to text-page mode
//     without validating).
//
// The whitelist check returns "" + Err when the requested format is
// not in the allowed set; this is the validation Python raises
// via check_empty / check_valid_value, surfaced as an error so the
// component short-circuits with _ERROR rather than emitting a
// payload the downstream chunker cannot consume.
func resolveOutputFormat(family string, setups map[string]schema.ParserSetup, allowed map[string][]string) (string, error) {
	setup, ok := setups[family]
	if !ok {
		// Family not configured — text-page mode; no validation.
		return "", nil
	}
	format, _ := setup["output_format"].(string)
	if format == "" {
		format = "text"
	}
	allowedList, ok := allowed[family]
	if !ok || len(allowedList) == 0 {
		// No whitelist entry — accept what the setup asked for.
		return format, nil
	}
	for _, candidate := range allowedList {
		if strings.EqualFold(candidate, format) {
			return format, nil
		}
	}
	return "", fmt.Errorf(
		"Parser: output_format %q for %q is not in allowed_output_format %v",
		format, family, allowedList,
	)
}

// resolveLibType returns the lib_type argument for parser.GetParser.
// The Python side does not pass an explicit lib_type for the
// Markdown / HTML / Office families — the parser constructor picks
// the only available backend. Mirroring that, when setups[…].lib_type
// is unset we leave the field empty and let parser.GetParser
// surface a structured error if no backend is wired.
//
// `parse_method` is preserved on the dispatch result so callers can
// tell the difference between "explicit OCR" and "default DeepDOC"
// without re-reading setups.
func resolveLibType(fileType utility.FileType, setups map[string]schema.ParserSetup) (libType, parseMethod string) {
	family := resolveParserFamily(fileType)
	setup, ok := setups[family]
	if !ok {
		return "", ""
	}
	if s, ok := setup["lib_type"].(string); ok {
		libType = s
	}
	if s, ok := setup["parse_method"].(string); ok {
		parseMethod = s
	}
	return libType, parseMethod
}

// dispatchParse resolves the parser for the given fileType and invokes
// its structured ParseWithResult contract.
//
// The function NEVER returns a partial result. On error the result
// is the zero value (OutputFormat == "" + Err != nil). Callers can
// detect the success/failure boundary on the OutputFormat alone.
//
// fileType may be utility.FileTypeOTHER when the upstream did not
// supply a filename; the dispatch then takes text-page mode
// without consulting parser.GetParser.
func dispatchParse(fileType utility.FileType, filename string, data []byte, setups map[string]schema.ParserSetup) parserDispatchResult {
	if fileType == utility.FileTypeOTHER {
		// Unknown / unset family. The component treats the bytes
		// as text pages; the existing logic
		// (splitIntoPages + fan-out) handles it. We return no
		// result here so the caller routes to that path.
		return parserDispatchResult{}
	}

	libType, parseMethod := resolveLibType(fileType, setups)

	// Resolve a parser via the GetParser entry point. Any error here
	// is the caller's fault (libType unsupported for this family);
	// surface it as Err so Invoke can set _ERROR.
	p, err := parser.GetParser(fileType, map[string]string{"lib_type": libType})
	if err != nil {
		return parserDispatchResult{Err: fmt.Errorf("Parser: resolve %q: %w", fileType, err)}
	}
	configureParserFromSetups(p, fileType, setups)

	res := p.ParseWithResult(filename, data)
	if res.Err != nil {
		return parserDispatchResult{Err: fmt.Errorf("Parser: %q: %w", fileType, res.Err)}
	}
	// Carry the configured parse_method on the file metadata so
	// downstream consumers can read which provider ran.
	if parseMethod != "" {
		if res.File == nil {
			res.File = map[string]any{}
		}
		res.File["parse_method"] = parseMethod
	}
	return parserDispatchResult{
		OutputFormat: res.OutputFormat,
		File:         res.File,
		JSON:         res.JSON,
		Markdown:     res.Markdown,
		Text:         res.Text,
		HTML:         res.HTML,
	}
}

// fileTypeFromInputs derives the parser-library extension form
// (utility.FileType) from the upstream inputs. The result is the
// value passed to parser.GetParser, whose switch arms are keyed
// off the utility constants.
//
// Resolution order:
//
//  1. inputs["file_type"] — explicit family hint from the upstream
//     File component. We accept either the extension ("md", "docx")
//     or the python family name ("markdown"); both are normalised
//     to the extension form via the pythonFamilyName / familyToExt
//     lookup tables below.
//  2. inputs["file"].name — fall back to the filename so a caller
//     that only supplies the path is still routed correctly.
//  3. inputs["name"] — last-resort filename.
//  4. utility.FileTypeOTHER — text-page mode.
//
// The function never errors; unknown / absent filenames degrade to
// FileTypeOTHER so the component's raw-text branch picks them up.
func fileTypeFromInputs(inputs map[string]any) utility.FileType {
	if inputs == nil {
		return utility.FileTypeOTHER
	}
	if raw, ok := inputs["file_type"].(string); ok && raw != "" {
		if ft := familyToExt(pythonFamilyName(strings.ToLower(raw))); ft != utility.FileTypeOTHER {
			return ft
		}
		// Direct extension match — handles "md", "docx", etc.
		if ft := utility.GetFileType("x." + strings.ToLower(raw)); ft != utility.FileTypeOTHER {
			return ft
		}
	}
	if m, ok := inputs["file"].(map[string]any); ok {
		if name, ok := m["name"].(string); ok && name != "" {
			return utility.GetFileType(name)
		}
	}
	if name, ok := inputs["name"].(string); ok && name != "" {
		return utility.GetFileType(name)
	}
	return utility.FileTypeOTHER
}

// familyToExt maps the python family name back to the utility
// extension form. Returns FileTypeOTHER for families whose parser
// isn't yet wired (audio, video, image, email, epub, …).
func familyToExt(family string) utility.FileType {
	switch family {
	case "pdf":
		return utility.FileTypePDF
	case "doc":
		return utility.FileTypeDOC
	case "docx":
		return utility.FileTypeDOCX
	case "slides":
		return utility.FileTypePPTX // pptx arm picks the slide parser
	case "spreadsheet":
		return utility.FileTypeXLSX
	case "html":
		return utility.FileTypeHTML
	case "markdown":
		return utility.FileTypeMarkdown
	case "text&code":
		return utility.FileTypeTXT
	}
	return utility.FileTypeOTHER
}

// pythonFamilyName normalises a free-form file-type hint to the
// python family identifier used by schema.ParserParam.Setups.
// Returns "" when the hint is unknown.
func pythonFamilyName(raw string) string {
	switch raw {
	case "pdf":
		return "pdf"
	case "doc":
		return "doc"
	case "docx":
		return "docx"
	case "ppt":
		return "slides"
	case "pptx":
		return "slides"
	case "xls":
		return "spreadsheet"
	case "xlsx":
		return "spreadsheet"
	case "csv":
		return "spreadsheet"
	case "html", "htm":
		return "html"
	case "md", "markdown", "mdx":
		return "markdown"
	case "jpg", "jpeg", "png", "gif":
		return "image"
	case "eml", "msg":
		return "email"
	case "epub":
		return "epub"
	case "txt", "py", "js", "java", "c", "cpp", "h", "php",
		"go", "ts", "sh", "cs", "kt", "sql":
		return "text&code"
	case "mp4", "avi", "mkv":
		return "video"
	case "wav", "mp3", "aac", "flac", "ogg":
		return "audio"
	}
	return ""
}

// jsonItemsToPages reshapes a parsed JSON payload into the
// schema.Page layout the chunker side consumes. Each JSON item
// becomes one page carrying `text`, `doc_type_kwd`, and any
// ck_type / image / positions fields the parser attached. The page
// number is assigned sequentially so the deterministic-merge
// contract in plan §8 R8 holds.
func jsonItemsToPages(items []map[string]any) []schema.Page {
	out := make([]schema.Page, 0, len(items))
	for i, it := range items {
		page := schema.Page{}
		maps.Copy(page, it)
		// page_number anchors the deterministic-merge sort; the
		// parser-supplied number (if any) takes precedence so
		// PDF / DOCX outputs that already carry `page_number`
		// survive the round-trip.
		if _, ok := page["page_number"]; !ok {
			page["page_number"] = i
		}
		if _, ok := page["doc_type_kwd"]; !ok {
			page["doc_type_kwd"] = "text"
		}
		out = append(out, page)
	}
	return out
}

// pagesFromDispatch extracts the per-page bytes from a parsed
// schema.Page slice so the existing fan-out / merge path can run
// unchanged. Pages without a `text` field emit an empty buffer
// (the merge step treats them as zero-length pages).
func pagesFromDispatch(pages []schema.Page) [][]byte {
	out := make([][]byte, 0, len(pages))
	for _, p := range pages {
		var buf []byte
		if s, ok := p["text"].(string); ok {
			buf = []byte(s)
		}
		out = append(out, buf)
	}
	return out
}

// buildParserOutputs assembles the runtime output map from the
// merged pages slice AND the dispatch result (when the dispatch
// succeeded). The output shape:
//
//   - pages          []schema.Page — sorted by PageNumber
//   - name           string        — from the upstream file/document name
//     (or doc_id when no filename is available)
//   - output_format  string        — the dispatch's OutputFormat,
//     or "text" for the raw-text
//     fallback
//   - json | markdown | text | html — the dispatched payload on
//     the matching family key (only
//     populated on a structured
//     dispatch)
//   - file           map[string]any — the parser-enriched file
//     metadata, when present
//
// This mirrors the Python Parser component's `set_output()` calls
// at rag/flow/parser/parser.py:_invoke — the downstream chunker
// / tokenizer / extractor components read the matching family
// key, with "pages" as the universal fallback shape.
func buildParserOutputs(parsed []schema.Page, dispatched parserDispatchResult, name string, fileType utility.FileType) map[string]any {
	out := map[string]any{
		"pages": toAnyPages(parsed),
		"name":  name,
	}
	if dispatched.Err == nil && dispatched.OutputFormat != "" {
		out["output_format"] = dispatched.OutputFormat
		switch dispatched.OutputFormat {
		case "json":
			out["json"] = dispatched.JSON
		case "markdown":
			out["markdown"] = dispatched.Markdown
		case "html":
			out["html"] = dispatched.HTML
		case "text":
			out["text"] = dispatched.Text
		}
		if dispatched.File != nil {
			out["file"] = dispatched.File
		}
		return out
	}
	// Raw-text fallback path: emit output_format = "text" so a
	// chunker branching on the format key still sees a sane value.
	out["output_format"] = "text"
	_ = fileType // reserved for a future "raw_text per-family" extension
	return out
}

func hydrateEmptyDispatchPayload(dispatched parserDispatchResult, binary []byte) parserDispatchResult {
	if dispatched.Err != nil || len(binary) == 0 {
		return dispatched
	}
	switch dispatched.OutputFormat {
	case "json":
		if len(dispatched.JSON) == 0 {
			dispatched.JSON = pagesToJSONItems(splitIntoPages(binary))
		}
	case "text":
		if dispatched.Text == "" {
			dispatched.Text = string(binary)
		}
	}
	return dispatched
}

func pagesToJSONItems(pages [][]byte) []map[string]any {
	if len(pages) == 0 {
		pages = splitIntoPages(nil)
	}
	out := make([]map[string]any, 0, len(pages))
	for _, page := range pages {
		text := string(page)
		if text == "" {
			continue
		}
		out = append(out, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
		})
	}
	return out
}

func parserInputName(inputs map[string]any, docID string) string {
	if inputs != nil {
		if name, ok := inputs["name"].(string); ok && name != "" {
			return name
		}
		if m, ok := inputs["file"].(map[string]any); ok {
			if name, ok := m["name"].(string); ok && name != "" {
				return name
			}
		}
	}
	return docID
}
