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
	"encoding/csv"
	"io"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// CountSpreadsheetRows estimates how many logical rows a table document contains.
// XLSX sums all worksheet rows so task pagination covers every sheet.
func CountSpreadsheetRows(name string, binary []byte) int {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		if rows, err := countXLSXRows(binary); err == nil {
			return rows
		}
	case ".csv", ".tsv", ".txt":
		return countDelimitedRows(name, binary)
	}
	return 0
}

func countDelimitedRows(name string, binary []byte) int {
	reader := csv.NewReader(bytes.NewReader(binary))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	if strings.EqualFold(filepath.Ext(name), ".tsv") {
		reader.Comma = '\t'
	}
	rows := 0
	for {
		_, err := reader.Read()
		if err == nil {
			rows++
			continue
		}
		if err == io.EOF {
			break
		}
		rows += bytes.Count(binary, []byte{'\n'})
		if len(binary) > 0 && binary[len(binary)-1] != '\n' {
			rows++
		}
		break
	}
	return rows
}

func countXLSXRows(binary []byte) (int, error) {
	book, err := excelize.OpenReader(bytes.NewReader(binary))
	if err != nil {
		return 0, err
	}
	defer book.Close()

	totalRows := 0
	for _, sheet := range book.GetSheetList() {
		rows, err := book.GetRows(sheet)
		if err != nil {
			return 0, err
		}
		totalRows += len(rows)
	}
	return totalRows, nil
}
