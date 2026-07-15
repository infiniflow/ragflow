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

package utility

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestCountSpreadsheetRowsUsesSummedXLSXRows(t *testing.T) {
	book := excelize.NewFile()
	t.Cleanup(func() { _ = book.Close() })
	if err := book.SetSheetName("Sheet1", "First"); err != nil {
		t.Fatalf("rename first sheet: %v", err)
	}
	if _, err := book.NewSheet("Second"); err != nil {
		t.Fatalf("create second sheet: %v", err)
	}
	for row := 1; row <= 3; row++ {
		if err := book.SetCellInt("First", "A"+strconv.Itoa(row), int64(row)); err != nil {
			t.Fatalf("write first sheet: %v", err)
		}
	}
	for row := 1; row <= 5; row++ {
		if err := book.SetCellInt("Second", "A"+strconv.Itoa(row), int64(row)); err != nil {
			t.Fatalf("write second sheet: %v", err)
		}
	}

	var buf bytes.Buffer
	if err := book.Write(&buf); err != nil {
		t.Fatalf("write workbook: %v", err)
	}

	if got := CountSpreadsheetRows("book.xlsx", buf.Bytes()); got != 8 {
		t.Fatalf("CountSpreadsheetRows() = %d, want 8", got)
	}
}
