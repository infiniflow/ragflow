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

// Output-format renderer. The Message component exposes a
// `output_format` field that selects between "html", "markdown",
// and "plain" rendering of the resolved content + downloads list.
// This file is the renderer; it stays dependency-free (no
// html/template or blackfriday) and ships in lockstep with the
// message.go scaffold without dragging in new third-party deps.
//
// Render conventions:
//   - "html"     — wrap in a minimal <div> + escape HTML; downloads
//                  become <a href="..." download="...">filename</a>
//   - "markdown" — pass through verbatim; downloads become
//                  [filename](url) links. Python also passes
//                  markdown through with light normalization; we
//                  match that for parity.
//   - "plain"    — strip HTML tags (best-effort) and present
//                  downloads as "filename (url)" lines. Default
//                  when the field is unset, matching Python's
//                  "no renderer" fallback.
//
// The renderer is intentionally pure (no I/O) so the message
// component can call it inside Stream() chunks without blocking
// on a downstream service.

package component

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// OutputFormat is the value type of the `output_format` field on
// the Message component. The string constants match the Python
// DSL field values.
type OutputFormat string

const (
	OutputFormatHTML     OutputFormat = "html"
	OutputFormatMarkdown OutputFormat = "markdown"
	OutputFormatMD       OutputFormat = "md"
	OutputFormatPlain    OutputFormat = "plain"
	// OutputFormatEmpty means "no renderer" — content passes through
	// as-is. Python's default. The string value differs from "" only
	// in the way the user expressed the choice ("none" vs unset).
	OutputFormatEmpty OutputFormat = ""
)

// DownloadInfo is the normalized shape of an extracted download
// entry. Mirrors agent/component/message.py:_is_download_info
// (the {doc_id, filename, mime_type} tuple).
type DownloadInfo struct {
	DocID                        string `json:"doc_id"`
	Filename                     string `json:"filename"`
	MimeType                     string `json:"mime_type"`
	URL                          string `json:"url,omitempty"`
	Content                      string `json:"content,omitempty"`
	Size                         int    `json:"size,omitempty"`
	PreviewURL                   string `json:"preview_url,omitempty"`
	IncludeDownloadInfoInContent bool   `json:"include_download_info_in_content,omitempty"`
}

// RenderRequest is the renderer input. Text is the resolved
// message body; Downloads is the list of extracted attachment
// descriptors. The renderer is pure — the caller decides where
// the rendered string goes.
type RenderRequest struct {
	Format    OutputFormat
	Text      string
	Downloads []DownloadInfo
}

// Render applies the format to the request. Unknown formats
// fall back to plain text so downstream nodes always see a
// non-empty string.
func Render(req RenderRequest) string {
	format := req.Format
	if format == OutputFormatEmpty {
		format = OutputFormatPlain
	}
	body := req.Text
	var dlBlock string
	if len(req.Downloads) > 0 {
		dlBlock = renderDownloads(format, req.Downloads)
	}
	switch format {
	case OutputFormatHTML:
		return wrapHTML(body, dlBlock)
	case OutputFormatMarkdown, OutputFormatMD:
		return joinMarkdown(body, dlBlock)
	default:
		return joinPlain(body, dlBlock)
	}
}

func renderDownloads(format OutputFormat, dls []DownloadInfo) string {
	switch format {
	case OutputFormatHTML:
		var b strings.Builder
		b.WriteString(`<ul class="rf-downloads">`)
		for _, d := range dls {
			b.WriteString(`<li><a href="`)
			b.WriteString(html.EscapeString(d.URL))
			b.WriteString(`" download="`)
			b.WriteString(html.EscapeString(d.Filename))
			b.WriteString(`" type="`)
			b.WriteString(html.EscapeString(d.MimeType))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(d.Filename))
			b.WriteString("</a></li>")
		}
		b.WriteString("</ul>")
		return b.String()
	case OutputFormatMarkdown, OutputFormatMD:
		var b strings.Builder
		for _, d := range dls {
			b.WriteString("- [")
			b.WriteString(d.Filename)
			b.WriteString("](")
			b.WriteString(d.URL)
			b.WriteString(")\n")
		}
		return strings.TrimRight(b.String(), "\n")
	default:
		var b strings.Builder
		for _, d := range dls {
			b.WriteString(d.Filename)
			b.WriteString(" (")
			b.WriteString(d.URL)
			b.WriteString(")\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}
}

func wrapHTML(body, dlBlock string) string {
	if dlBlock == "" {
		return "<div class=\"rf-message\">" + html.EscapeString(body) + "</div>"
	}
	return "<div class=\"rf-message\">" + html.EscapeString(body) + dlBlock + "</div>"
}

func joinMarkdown(body, dlBlock string) string {
	if dlBlock == "" {
		return body
	}
	return body + "\n\n" + dlBlock
}

func joinPlain(body, dlBlock string) string {
	if dlBlock == "" {
		return body
	}
	return body + "\n" + dlBlock
}

// htmlTagRe is the loose "best-effort" HTML stripper used by the
// plain renderer. It removes paired and unpaired tags without
// attempting to keep attribute content. This matches the
// pragmatic behaviour in Python's `_stringify_message_value`
// fallback path: "if markdown → use as-is, if plain → strip tags".
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// StripHTMLTags removes HTML tags from s. Used by callers that
// want a "plain" preview of an HTML-rendered body (e.g. console
// logging). Public so the message component can reuse it.
func StripHTMLTags(s string) string {
	return strings.TrimSpace(htmlTagRe.ReplaceAllString(s, ""))
}

// IsDownloadInfo mirrors the Python `_is_download_info` static
// method. A value is a download descriptor iff it is a map carrying
// the three canonical keys (doc_id, filename, mime_type). Other
// keys are allowed and ignored.
func IsDownloadInfo(value any) bool {
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, k := range []string{"doc_id", "filename", "mime_type"} {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// ExtractDownloads walks a single input value (string, map, list)
// and returns the download descriptors it carries. The Python
// version does the same walk recursively on message-value trees;
// we keep the same recursive shape so the Go port's semantics
// match. Returns an empty slice when nothing is found.
func ExtractDownloads(value any) []DownloadInfo {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil
		}
		return ExtractDownloads(parsed)
	case map[string]any:
		if IsDownloadInfo(v) {
			return []DownloadInfo{downloadFromMap(v)}
		}
		var out []DownloadInfo
		for _, item := range v {
			out = append(out, ExtractDownloads(item)...)
		}
		return out
	case map[string]string:
		m := make(map[string]any, len(v))
		for key, value := range v {
			m[key] = value
		}
		return ExtractDownloads(m)
	case []any:
		var out []DownloadInfo
		for _, item := range v {
			out = append(out, ExtractDownloads(item)...)
		}
		return out
	case []DownloadInfo:
		return v
	}
	return nil
}

func appendUniqueDownloads(dst []DownloadInfo, src []DownloadInfo) []DownloadInfo {
	for _, candidate := range src {
		duplicate := false
		for _, existing := range dst {
			if candidate.DocID != "" && candidate.DocID == existing.DocID {
				duplicate = true
				break
			}
			if candidate.DocID == "" && candidate.Filename == existing.Filename && candidate.URL == existing.URL {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, candidate)
		}
	}
	return dst
}

func downloadInfoString(value any) bool {
	switch v := value.(type) {
	case string:
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return false
		}
		return isDownloadInfoValue(parsed)
	default:
		return isDownloadInfoValue(v)
	}
}

func isDownloadInfoValue(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		return IsDownloadInfo(v)
	case map[string]string:
		for _, k := range []string{"doc_id", "filename", "mime_type"} {
			if _, ok := v[k]; !ok {
				return false
			}
		}
		return true
	case DownloadInfo:
		return v.DocID != "" && v.Filename != "" && v.MimeType != ""
	}
	return false
}

func downloadFromMap(m map[string]any) DownloadInfo {
	d := DownloadInfo{}
	if s, ok := m["doc_id"].(string); ok {
		d.DocID = s
	}
	if s, ok := m["filename"].(string); ok {
		d.Filename = s
	}
	if s, ok := m["mime_type"].(string); ok {
		d.MimeType = s
	}
	if s, ok := m["url"].(string); ok {
		d.URL = s
	}
	if d.URL == "" {
		if s, ok := m["download"].(string); ok {
			d.URL = s
		}
	}
	if s, ok := m["preview_url"].(string); ok {
		d.PreviewURL = s
		if d.URL == "" {
			d.URL = s
		}
	}
	if s, ok := m["content"].(string); ok {
		d.Content = s
	}
	if f, ok := m["size"].(float64); ok {
		d.Size = int(f)
	} else if i, ok := m["size"].(int); ok {
		d.Size = i
	}
	if b, ok := m["include_download_info_in_content"].(bool); ok {
		d.IncludeDownloadInfoInContent = b
	}
	return d
}
