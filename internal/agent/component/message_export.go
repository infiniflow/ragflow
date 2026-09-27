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

// Message attachment export — the Go port of Python message.py's
// `_convert_content`: when the Message component declares a file
// `output_format` (the "Download file type" selector in the UI), the
// resolved content is converted to that format, uploaded to the
// agent attachment storage, and surfaced as outputs["attachment"]
// ({doc_id, format, file_name}). The front-end renders the message
// download button from that descriptor.

package component

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"

	iow "ragflow/internal/agent/component/io"
	"ragflow/internal/common"
)

// messageExportFormat normalizes the raw output_format value to a
// supported export format. It returns "" when the value only selects a
// text rendering style (plain / empty) or is not a file format, in
// which case no attachment is exported. This deliberately deviates
// from the Python runtime, which coerces any unrecognized non-empty
// format to markdown: a plain rendering should not fabricate a
// download, so only explicit file formats produce an attachment.
func messageExportFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "markdown", "md":
		return "markdown"
	case "html":
		return "html"
	case "docx":
		return "docx"
	case "xlsx":
		return "xlsx"
	case "pdf":
		return "pdf"
	default:
		return ""
	}
}

// exportMessageAttachment converts the resolved (un-rendered) markdown
// content to the requested format, stores it via the agent attachment
// storage, and returns the attachment descriptor consumed by the
// front-end download button (group-button.tsx reads {doc_id, format,
// file_name}). Each format writer owns its own markdown handling, the
// same contract as Python's pypandoc/pandas conversion.
//
// Errors are returned to the caller, which logs them and continues —
// a failed export must not break the message itself (Python wraps the
// whole conversion in try/except).
func exportMessageAttachment(ctx context.Context, format, content string) (map[string]any, error) {
	exported := strings.TrimSpace(common.StripThinkTrailing(content))
	if exported == "" {
		return nil, nil
	}

	var payload []byte
	var err error
	switch format {
	case "html":
		// Convert the markdown body to an HTML fragment first, as
		// pypandoc does (format="markdown" → html); WriteHTML only
		// adds the document wrapper around the fragment.
		payload = iow.WriteHTML(markdownToHTML(exported), iow.HTMLOptions{
			FontSize:   defaultDocsFontSize,
			FontFamily: defaultHTMLFontFamily,
		})
	case "docx":
		payload, err = iow.WriteDOCX(exported, iow.DOCXOptions{
			FontSize:      defaultDocsFontSize,
			CJKFontFamily: defaultDOCXFontFamily,
		})
	case "pdf":
		payload, err = iow.WritePDF(exported, iow.PDFOptions{
			FontSize:       defaultDocsFontSize,
			FontFamily:     defaultPDFFontFamily,
			AddPageNumbers: true,
		})
	case "xlsx":
		payload, err = iow.WriteXLSX(exported, iow.XLSXOptions{})
	default: // markdown
		payload = iow.WriteMarkdown(exported, iow.MarkdownOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("convert to %s: %w", format, err)
	}

	docID := uuid.New().String()
	stored, err := storeAgentAttachment(ctx, docID, payload)
	if err != nil {
		return nil, fmt.Errorf("store attachment: %w", err)
	}
	if !stored {
		// No tenant/storage context (e.g. unit tests): nothing was
		// persisted, so do not advertise a download that would 404.
		return nil, nil
	}
	return map[string]any{
		"doc_id":    docID,
		"format":    format,
		"file_name": fmt.Sprintf("%s.%s", docID[:8], format),
	}, nil
}

// markdownToHTML renders markdown source to an HTML fragment, the Go
// counterpart of pypandoc's markdown→html conversion in Python's
// _convert_content: headings, emphasis and tables keep their structure
// in the exported file instead of shipping as literal text. Raw HTML
// in the source passes through (pandoc's raw_html default). On a
// conversion error the plain content is kept rather than dropping the
// export.
func markdownToHTML(content string) string {
	var buf bytes.Buffer
	md := goldmark.New(goldmark.WithRendererOptions(gmhtml.WithUnsafe()))
	if err := md.Convert([]byte(content), &buf); err != nil {
		return content
	}
	return buf.String()
}
