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

package splitter

import (
	"fmt"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
)

// SlicePDF extracts a self-contained valid sub-PDF containing pages from
// startPage to endPage (0-indexed inclusive).
func SlicePDF(pdfData []byte, startPage, endPage int) ([]byte, error) {
	if len(pdfData) == 0 {
		return nil, fmt.Errorf("splitter: empty pdf data")
	}
	if startPage < 0 || endPage < startPage {
		return nil, fmt.Errorf("splitter: invalid page range [%d, %d]", startPage, endPage)
	}
	return pdfium.ExtractPageRange(pdfData, startPage, endPage)
}
