//go:build cgo

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pdf

import (
	"fmt"

	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
)

// PlainText extracts plain text per page using the pdf_oxide native engine.
// It is the cgo implementation of the parser package's plain_text PDF
// strategy. The returned items carry the keys text/doc_type_kwd/page_number
// so the parser package can wrap them with pdfItemsToResult unchanged.
func PlainText(data []byte) ([]map[string]any, int, error) {
	if len(data) == 0 {
		return nil, 0, nil
	}
	doc, err := pdfoxide.OpenBytes(data)
	if err != nil {
		return nil, 0, fmt.Errorf("deepdoc/pdf: plain_text open: %w", err)
	}
	defer doc.Close()

	pageCount, err := doc.PageCount()
	if err != nil {
		return nil, 0, fmt.Errorf("deepdoc/pdf: plain_text page count: %w", err)
	}
	items := make([]map[string]any, 0, pageCount)
	for page := 0; page < pageCount; page++ {
		text, err := doc.GetPageText(page)
		if err != nil {
			return nil, 0, fmt.Errorf("deepdoc/pdf: plain_text page %d: %w", page+1, err)
		}
		items = append(items, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
			"page_number":  page + 1,
		})
	}
	return items, pageCount, nil
}
