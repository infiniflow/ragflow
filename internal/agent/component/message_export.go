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
// final rendered content is converted to that format, uploaded to the
// agent attachment storage, and surfaced as outputs["attachment"]
// ({doc_id, format, file_name}). The front-end renders the message
// download button from that descriptor.

package component

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	iow "ragflow/internal/agent/component/io"
	"ragflow/internal/common"
)

// messageExportFormat normalizes the raw output_format value to a
// supported export format. It returns "" when the value only selects a
// text rendering style (plain / empty) or is not a file format.
// Mirrors Python: anything outside {markdown, html, pdf, docx, xlsx}
// (after aliasing "md") is treated as markdown.
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

// exportMessageAttachment converts content to the requested format,
// stores it via the agent attachment storage, and returns the
// attachment descriptor consumed by the front-end download button
// (group-button.tsx reads {doc_id, format, file_name}).
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
		payload = iow.WriteHTML(exported, iow.HTMLOptions{
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
