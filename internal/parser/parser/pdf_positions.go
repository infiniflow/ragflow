package parser

import "encoding/json"

// This file owns PDF position decoding. The parser boundary, the vision
// cropper and the chunker all read PDF positions through these helpers, so
// they agree on what a position row is and on which wire shapes are
// supported.
//
// Supported shapes:
//   - a matrix of 5-tuples: [][]any, []any of rows, [][]float64;
//   - one flat 5-tuple: []float64, the legacy wire form some engines (TCADP)
//     still emit.
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
// It accepts the matrix form and the legacy flat 5-tuple and returns rows of
// five floats. Rows shorter than five values are skipped.
func PDFPositionRowsFromJSON(raw json.RawMessage) [][]float64 {
	if len(raw) == 0 {
		return nil
	}
	var matrix [][]float64
	if err := json.Unmarshal(raw, &matrix); err == nil {
		out := make([][]float64, 0, len(matrix))
		for _, row := range matrix {
			if len(row) >= 5 {
				out = append(out, row)
			}
		}
		return out
	}
	var flat []float64
	if err := json.Unmarshal(raw, &flat); err == nil && len(flat) >= 5 {
		return [][]float64{flat}
	}
	return nil
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
