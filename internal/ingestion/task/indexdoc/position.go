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

package indexdoc

// positionsToIntMatrix groups a flat position array by five and truncates each
// value to int — the stored shape of position_int. It returns nil for an empty
// array or one whose length is not a multiple of five: malformed input is
// dropped, never reinterpreted.
func positionsToIntMatrix(positions []float64) [][]int {
	if len(positions) == 0 || len(positions)%5 != 0 {
		return nil
	}
	matrix := make([][]int, 0, len(positions)/5)
	for i := 0; i < len(positions); i += 5 {
		matrix = append(matrix, []int{
			int(positions[i]), int(positions[i+1]), int(positions[i+2]),
			int(positions[i+3]), int(positions[i+4]),
		})
	}
	return matrix
}

// addPDFPositions stores PDF layout boxes: positions is a flat []float64
// grouped by five as [page, left, right, top, bottom] and is written to
// page_num_int / top_int / position_int. Page numbers are ALREADY 1-indexed on
// entry — the 0→1 conversion happens once, at the parser boundary
// (normalizePDFPageNumber for the DeepDoc PDF path; TCADP writes 1-indexed
// directly) — so this is a passthrough and must NOT add +1, or the PDF path
// would double-increment.
//
// Mirrors Python: rag.nlp.add_positions() (Python adds +1 because its callers
// feed 0-indexed values; the Go pipeline normalizes earlier).
func addPDFPositions(chunk map[string]any, positions []float64) {
	matrix := positionsToIntMatrix(positions)
	if matrix == nil {
		return
	}
	pageNumInt := make([]int, 0, len(matrix))
	topInt := make([]int, 0, len(matrix))
	for _, box := range matrix {
		pageNumInt = append(pageNumInt, box[0])
		topInt = append(topInt, box[3])
	}
	chunk["page_num_int"] = pageNumInt
	chunk["top_int"] = topInt
	chunk["position_int"] = matrix
}

// addSpreadsheetPositions stores spreadsheet cell ranges: positions is a flat
// []float64 grouped by five as [sheet, rowStart, rowEnd, colStart, colEnd] and
// is written to position_int only — that field is the document preview's
// coordinate carrier (the chunk API serves "positions" from it, and the Excel
// preview reads the five values positionally as sheet / rows / columns).
//
// page_num_int and top_int are PDF layout fields: deriving them from these
// tuples would store a sheet number as a page and a column as a top, and would
// clobber the row index the QA chunker wrote to top_int (the same field
// Python's beAdoc/beAdocDocx set from row_num for its spreadsheet chunks,
// which never carry positions at all).
func addSpreadsheetPositions(chunk map[string]any, positions []float64) {
	if matrix := positionsToIntMatrix(positions); matrix != nil {
		chunk["position_int"] = matrix
	}
}
