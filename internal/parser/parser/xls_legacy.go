//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"fmt"
	"io"

	"github.com/yamitzky/xlrd-go/xlrd"
)

// biffSource is the reader contract parseBIFF8Bytes consumes. The primary
// implementation is go-excel-reader (BIFF8); xlrdBIFFSource adapts xlrd-go
// for BIFF5 and older workbooks it rejects.
type biffSource interface {
	NextSheet() bool
	SheetName() string
	SheetCount() int
	Read() bool
	FieldCount() int
	GetValue(col int) any
	RowIndex() int
	Err() error
	io.Closer
}

// xlrdBIFFSource presents an xlrd workbook through the streaming cursor the
// BIFF8 reader exposes. xlrd indexes sheets and rows directly, so the cursor
// state lives here.
type xlrdBIFFSource struct {
	book  *xlrd.Book
	sheet *xlrd.Sheet
	row   int
	err   error
}

func newXlrdBIFFSource(data []byte) (_ *xlrdBIFFSource, err error) {
	// xlrd indexes malformed BIFF records without bounds checks in places;
	// report panics as parse errors instead of crashing the parser.
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("open legacy BIFF workbook: %v", p)
		}
	}()
	bk, err := xlrd.OpenWorkbook("", &xlrd.OpenWorkbookOptions{FileContents: data})
	if err != nil {
		return nil, fmt.Errorf("open legacy BIFF workbook: %w", err)
	}
	return &xlrdBIFFSource{book: bk, row: -1}, nil
}

func (s *xlrdBIFFSource) NextSheet() bool {
	idx := 0
	if s.sheet != nil {
		idx = s.indexOf(s.sheet) + 1
	}
	for ; idx < s.book.NSheets; idx++ {
		sh, err := s.book.SheetByIndex(idx)
		if err == nil && sh != nil {
			s.sheet = sh
			s.row = -1
			return true
		}
		if err != nil && s.err == nil {
			s.err = err
		}
	}
	return false
}

func (s *xlrdBIFFSource) indexOf(target *xlrd.Sheet) int {
	for i := 0; i < s.book.NSheets; i++ {
		if sh, err := s.book.SheetByIndex(i); err == nil && sh == target {
			return i
		}
	}
	return -1
}

func (s *xlrdBIFFSource) SheetName() string {
	if s.sheet == nil {
		return ""
	}
	return s.sheet.Name
}

func (s *xlrdBIFFSource) SheetCount() int { return s.book.NSheets }

func (s *xlrdBIFFSource) Read() bool {
	if s.sheet == nil {
		return false
	}
	s.row++
	return s.row < s.sheet.NRows
}

func (s *xlrdBIFFSource) FieldCount() int {
	if s.sheet == nil {
		return 0
	}
	return s.sheet.NCols
}

func (s *xlrdBIFFSource) RowIndex() int { return s.row }

func (s *xlrdBIFFSource) Err() error { return s.err }

func (s *xlrdBIFFSource) Close() error { return nil }

func (s *xlrdBIFFSource) GetValue(col int) any {
	if s.sheet == nil || s.row < 0 || s.row >= s.sheet.NRows || col < 0 || col >= s.sheet.NCols {
		return nil
	}
	v := s.sheet.CellValue(s.row, col)
	switch s.sheet.CellType(s.row, col) {
	case xlrd.XL_CELL_DATE:
		if f, ok := v.(float64); ok {
			if t, err := xlrd.XldateAsDatetime(f, s.book.Datemode); err == nil {
				return t
			}
		}
		return nil
	case xlrd.XL_CELL_EMPTY, xlrd.XL_CELL_BLANK, xlrd.XL_CELL_ERROR:
		return nil
	case xlrd.XL_CELL_TEXT:
		str, _ := v.(string)
		return str
	default:
		return v
	}
}
