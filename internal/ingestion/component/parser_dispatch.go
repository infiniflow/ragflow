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

// Parser dispatch resolves the parser backend and returns the structured
// ParseWithResult payload for component-boundary normalization.
//
// `parse_method` is carried through file metadata for downstream
// consumers, while the actual backend work stays in
// internal/parser/parser/*.

package component

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// parserDispatchResult is the typed outcome of dispatchParse. The
// component's Invoke translates it into the runtime output map.
//
// OutputFormat is the wire format the parser actually emitted. ParserComponent
// normalizes setup values to JSON before dispatch.
//
// File is the per-parser file metadata and may be nil.
//
// JSON is the canonical payload. Markdown, Text, and HTML capture non-JSON
// backend responses until buildParserOutputs normalizes them.
type parserDispatchResult struct {
	OutputFormat string
	File         map[string]any
	JSON         []map[string]any
	Markdown     string
	Text         string
	HTML         string
	Warnings     []string
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
//
// `parse_method` is captured from setups so callers can tell the
// difference between "explicit OCR" and "default DeepDOC" without
// re-reading setups. lib_type is no longer threaded through: the
// Python dispatcher picks a single backend per family and the Go
// constructors mirror that.
func dispatchParse(ctx context.Context, fileType utility.FileType, filename string, data []byte, setups map[string]schema.ParserSetup) parserDispatchResult {
	if fileType == utility.FileTypeOTHER {
		// Unknown / unset family. The component treats the bytes
		// as text pages; splitIntoPages handles it. We return no
		// result here so the caller routes to that path.
		return parserDispatchResult{}
	}

	var parseMethod string
	if setup, ok := setups[resolveParserFamily(fileType)]; ok {
		if s, ok := setup["parse_method"].(string); ok {
			parseMethod = s
		}
	}

	p, err := parser.GetParser(fileType)
	if err != nil {
		return parserDispatchResult{Err: fmt.Errorf("parser: resolve %q: %w", fileType, err)}
	}
	configureParserFromSetups(p, fileType, setups)

	res := p.ParseWithResult(ctx, filename, data)
	if res.Err != nil {
		return parserDispatchResult{Err: fmt.Errorf("parser: %q: %w", fileType, res.Err)}
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
		Warnings:     res.Warnings,
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
//     or the python family name ("markdown"); both are normalized
//     to the extension form via the pythonFamilyName / familyToExt
//     lookup tables below.
//  2. inputs["name"] — the resolved source filename.
//  3. inputs["file"].name — fallback for callers that only supply a
//     file descriptor.
//  4. utility.FileTypeOTHER — text-page mode.
//
// The function never errors; unknown / absent filenames degrade to
// FileTypeOTHER so the component's raw-text branch picks them up.
func fileTypeFromInputs(inputs map[string]any) utility.FileType {
	if inputs == nil {
		return utility.FileTypeOTHER
	}
	if raw, ok := inputs["file_type"].(string); ok && raw != "" {
		lower := strings.ToLower(raw)
		// csv is a spreadsheet-family member but uses its own
		// dedicated parser rather than the xlsx/xls path.
		if lower == "csv" {
			return utility.FileTypeCSV
		}
		// Direct extension match first — handles exact hints like
		// "xls", "ppt", "doc", "docx", etc. This must run before
		// the family look-up so that legacy binary extensions
		// aren't collapsed to OOXML types (e.g. "xls" → XLSX).
		if ft := utility.GetFileType("x." + lower); ft != utility.FileTypeOTHER {
			return ft
		}
		// Family-name lookup catches python-side family identifiers
		// ("slides", "spreadsheet", "text&code") that aren't valid
		// file extensions.
		if ft := familyToExt(pythonFamilyName(lower)); ft != utility.FileTypeOTHER {
			return ft
		}
	}
	if name, ok := inputs["name"].(string); ok && name != "" {
		return utility.GetFileType(name)
	}
	if m, ok := inputs["file"].(map[string]any); ok {
		if name, ok := m["name"].(string); ok && name != "" {
			return utility.GetFileType(name)
		}
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
		return utility.FileTypePPTX
	case "spreadsheet":
		return utility.FileTypeXLSX
	case "csv":
		return utility.FileTypeCSV
	case "html":
		return utility.FileTypeHTML
	case "markdown":
		return utility.FileTypeMarkdown
	case "text&code":
		return utility.FileTypeTXT
	case "epub":
		return utility.FileTypeEPUB
	case "json":
		return utility.FileTypeJSON
	case "video":
		return utility.FileTypeVIDEO
	case "email":
		return utility.FileTypeEMAIL
	case "audio":
		return utility.FileTypeAURAL
	case "picture", "image", "visual":
		return utility.FileTypeVISUAL
	}
	return utility.FileTypeOTHER
}

// pythonFamilyName normalises a free-form file-type hint to the
// Python family identifier used by ParserComponent setups.
// Returns "" when the hint is unknown.
func pythonFamilyName(raw string) string {
	switch raw {
	case "pdf":
		return "pdf"
	case "doc":
		return "doc"
	case "docx":
		return "docx"
	case "ppt", "pptx", "slides":
		return "slides"
	case "xls", "xlsx", "spreadsheet":
		return "spreadsheet"
	case "csv":
		return "spreadsheet"
	case "html", "htm":
		return "html"
	case "md", "markdown", "mdx":
		return "markdown"
	case "epub":
		return "epub"
	case "json", "jsonl", "ldjson":
		return "json"
	case "txt", "py", "js", "java", "c", "cpp", "h", "php",
		"go", "ts", "sh", "cs", "kt", "sql":
		return "text&code"
	case "mp4", "avi", "mkv", "mov", "webm", "flv",
		"mpeg", "mpg", "wmv", "3gp", "3gpp", "video":
		return "video"
	case "eml", "msg", "email":
		return "email"
	case "da", "wave", "wav", "mp3", "aac", "flac", "ogg",
		"aiff", "au", "midi", "wma", "ape", "alac", "wv", "opus", "aural":
		return "audio"
	case "visual", "picture", "image",
		"png", "jpg", "jpeg", "gif", "bmp", "tiff", "tif",
		"webp", "svg", "ico", "avif", "heic", "apng":
		return "image"
	}
	return ""
}

// ParserFileFamily normalises a free-form file-type/extension hint to the
// python-side family identifier used as the key into a Parser component's
// setups (e.g. "pdf", "docx", "slides", "text&code"). It is the exported
// entry point for callers outside this package (e.g. the ingestion task
// executor that injects the debug page cap into override_params) that need
// to build the canonical ParserConfig[cpnID][family]["pages"] shape.
func ParserFileFamily(ext string) string {
	return pythonFamilyName(ext)
}

// buildParserOutputs assembles the runtime output map from the
// dispatch result (when the dispatch succeeded) or raw binary fallback.
// The output shape:
//
//   - name           string        — from the upstream file/document name
//     (or doc_id when no filename is available)
//   - file_type      string        — normalized parser routing type
//   - output_format  string        — always "json"
//   - json           []map[string]any — normalized parser items
//   - file           map[string]any — the parser-enriched file
//     metadata, when present
func buildParserOutputs(ctx context.Context, dispatched parserDispatchResult, name string, fileType utility.FileType, rawBinary []byte, lang string) map[string]any {
	out := map[string]any{
		"name": name,
	}
	if fileType != "" && fileType != utility.FileTypeOTHER {
		out["file_type"] = string(fileType)
	}
	if lang != "" {
		out["lang"] = lang
	}
	if dispatched.Err == nil && dispatched.OutputFormat != "" {
		out["output_format"] = "json"
		out["json"] = normalizeParserJSON(ctx, name, dispatched)
		if dispatched.File != nil {
			out["file"] = dispatched.File
		}
		return out
	}
	// Raw-text fallback path: emit one JSON item per page.
	rawPages := splitIntoPages(rawBinary)
	if len(rawPages) == 0 {
		rawPages = [][]byte{nil}
	}
	fallbackItems := make([]map[string]any, 0, len(rawPages))
	for _, pageBytes := range rawPages {
		txt := string(pageBytes)
		fallbackItems = append(fallbackItems, parser.NewTextJSONItem(txt))
	}
	out["output_format"] = "json"
	out["json"] = fallbackItems
	return out
}

func normalizeParserJSON(ctx context.Context, filename string, dispatched parserDispatchResult) []map[string]any {
	if len(dispatched.JSON) > 0 {
		return dispatched.JSON
	}
	if dispatched.Markdown != "" {
		return parseMarkdownToJSONItems(ctx, filename, dispatched.Markdown)
	}
	if dispatched.HTML != "" {
		res := parser.NewHTMLParser().ParseWithResult(ctx, filename, []byte(dispatched.HTML))
		if res.Err == nil && len(res.JSON) > 0 {
			return res.JSON
		}
		if res.Err != nil {
			warnParserNormalizationFallback(filename, "html", res.Err)
		} else {
			warnParserNormalizationFallback(filename, "html", fmt.Errorf("parser returned no JSON items"))
		}
		return []map[string]any{parser.NewTextJSONItem(dispatched.HTML)}
	}
	if dispatched.Text != "" {
		return []map[string]any{parser.NewTextJSONItem(dispatched.Text)}
	}
	return []map[string]any{}
}

func warnParserNormalizationFallback(filename, source string, err error) {
	common.Warn("parser normalization fell back to text",
		zap.String("filename", filename),
		zap.String("normalized_from", source),
		zap.Error(err),
	)
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
