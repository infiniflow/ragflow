//go:build !cgo

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
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PlainText extracts plain text per page using the pure-Go ledongthuc/pdf
// engine. It mirrors the cgo pdf_oxide implementation so the parser package's
// plain_text strategy keeps working under !cgo builds without native
// libraries. The returned items carry the keys text/doc_type_kwd/page_number
// so the parser package can wrap them with pdfItemsToResult unchanged.
func PlainText(data []byte) ([]map[string]any, int, error) {
	if len(data) == 0 {
		return nil, 0, nil
	}
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, 0, fmt.Errorf("deepdoc/pdf: plain_text open: %w", err)
	}
	pageCount := pdfReader.NumPage()
	items := make([]map[string]any, 0, pageCount)
	for pageNum := 1; pageNum <= pageCount; pageNum++ {
		p := pdfReader.Page(pageNum)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			return nil, 0, fmt.Errorf("deepdoc/pdf: plain_text page %d: %w", pageNum, err)
		}
		items = append(items, map[string]any{
			"text":         strings.TrimRight(text, "\n"),
			"doc_type_kwd": "text",
			"page_number":  pageNum,
		})
	}
	return items, pageCount, nil
}
