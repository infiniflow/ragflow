package parser

import "encoding/json"

// This file owns PDF position decoding. The parser boundary, the vision
// cropper and the chunker all read PDF positions through these helpers, so
// they agree on what a position row is and on which wire shapes are
// supported.
//
// Supported shapes:
//   - a matrix of 5-tuples: [][]any, []any of rows, [][]float64; a page cell
//     may be a scalar or a list (the raw layout.SectionsToJSON form, also
//     seen in external/historical payloads), ordered by its first page;
//   - one flat 5-tuple: []float64, the legacy wire form some engines (TCADP)
//     still emit;
//   - a flat sequence of multiple 5-tuples (TCADP/external producers), grouped
//     by five.
//
// A row is [page, left, right, top, bottom]. The page is 1-based.

// PDFPositionRows decodes the position rows carried by raw. Rows that are not
// a 5-tuple are skipped; a flat tuple counts as one row.
func PDFPositionRows(raw any) [][]any {
	switch v := raw.(type) {
	case [][]any:
		return filterPositionRows(v, func(row []any) []any { return row })
	case []any:
		out := make([][]any, 0, len(v))
		for _, rawRow := range v {
			switch row := rawRow.(type) {
			case []any:
				if len(row) == 5 {
					out = append(out, row)
				}
			case []float64:
				if len(row) == 5 {
					out = append(out, floatRowToAny(row))
				}
			}
		}
		return out
	case [][]float64:
		out := make([][]any, 0, len(v))
		for _, row := range v {
			if len(row) == 5 {
				out = append(out, floatRowToAny(row))
			}
		}
		return out
	case []float64:
		if len(v) == 5 {
			return [][]any{floatRowToAny(v)}
		}
	}
	return nil
}

// PDFPositionRowsFromJSON decodes a PDF position matrix carried as raw JSON.
// It accepts the nested matrix form (a page cell may be a scalar or a list),
// a single flat 5-tuple, and a flat sequence of multiple 5-tuples grouped by
// five, and returns rows of five floats. Rows shorter than five values, rows
// with non-numeric coordinates, and flat sequences whose length is not a
// multiple of five (ambiguous tuple boundaries) are skipped.
func PDFPositionRowsFromJSON(raw json.RawMessage) [][]float64 {
	if len(raw) == 0 {
		return nil
	}
	var rows [][]any
	if err := json.Unmarshal(raw, &rows); err == nil {
		out := make([][]float64, 0, len(rows))
		for _, row := range rows {
			if floats, ok := positionRowFloats(row); ok {
				out = append(out, floats)
			}
		}
		return out
	}
	var flat []any
	if err := json.Unmarshal(raw, &flat); err != nil || len(flat) < 5 || len(flat)%5 != 0 {
		return nil
	}
	out := make([][]float64, 0, len(flat)/5)
	for i := 0; i < len(flat); i += 5 {
		if floats, ok := positionRowFloats(flat[i : i+5]); ok {
			out = append(out, floats)
		}
	}
	return out
}

// positionRowFloats decodes one [page,left,right,top,bottom] row for numeric
// consumers (ordering, crop-region checks). A list page cell is ordered by
// its first page. The verbatim []any form is kept by the chunker merge for
// consumers that need the original cell.
func positionRowFloats(row []any) ([]float64, bool) {
	if len(row) < 5 {
		return nil, false
	}
	page, ok := positionPageFloat(row[0])
	if !ok {
		return nil, false
	}
	out := make([]float64, 5)
	out[0] = page
	for i := 1; i < 5; i++ {
		v, ok := positionFloat(row[i])
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// positionPageFloat decodes a page cell: a scalar number or a list whose
// first entry is numeric.
func positionPageFloat(raw any) (float64, bool) {
	if v, ok := positionFloat(raw); ok {
		return v, true
	}
	if pages, ok := raw.([]any); ok && len(pages) > 0 {
		return positionPageFloat(pages[0])
	}
	return 0, false
}

func filterPositionRows(rows [][]any, keep func([]any) []any) [][]any {
	out := make([][]any, 0, len(rows))
	for _, row := range rows {
		if len(row) == 5 {
			out = append(out, keep(row))
		}
	}
	return out
}

func floatRowToAny(row []float64) []any {
	out := make([]any, len(row))
	for i, v := range row {
		out[i] = v
	}
	return out
}
