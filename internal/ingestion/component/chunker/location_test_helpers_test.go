package chunker

import (
	"encoding/json"
	"reflect"

	"ragflow/internal/parser/position"
)

// locationJSON exposes geometry for existing matrix expectations. It never
// supplies a missing location or derives geometry from chunk text.
func locationJSON(l *position.Location, preview bool) json.RawMessage {
	if l == nil {
		return nil
	}
	rows := l.IndexMatrix()
	if preview {
		rows = nil
		if l.Page != nil {
			for _, b := range l.Page.PreviewBoxes {
				rows = append(rows, []float64{float64(b.Page), b.Left, b.Right, b.Top, b.Bottom})
			}
		}
	}
	if len(rows) == 0 {
		return nil
	}
	b, _ := json.Marshal(rows)
	return b
}

func locationPage(l *position.Location) *int {
	if l == nil || l.Page == nil {
		return nil
	}
	return l.Page.Number
}

func locationField(chunk map[string]any, key string) any {
	l, err := position.FromMap(chunk)
	if err != nil {
		return nil
	}
	if l == nil {
		return chunk[key]
	}
	switch key {
	case "positions":
		return l.IndexMatrix()
	case "_pdf_positions":
		var rows [][]float64
		if l.Page != nil {
			for _, b := range l.Page.PreviewBoxes {
				rows = append(rows, []float64{float64(b.Page), b.Left, b.Right, b.Top, b.Bottom})
			}
		}
		return rows
	case "sheet":
		if l.Spreadsheet != nil {
			return l.Spreadsheet.Sheet
		}
	case "sheet_index":
		if l.Spreadsheet != nil && l.Spreadsheet.Index != nil {
			return float64(*l.Spreadsheet.Index)
		}
	case "page_number":
		if l.Page != nil && l.Page.Number != nil {
			return float64(*l.Page.Number)
		}
	}
	return nil
}

func locationFieldPresent(chunk map[string]any, key string) (any, bool) {
	v := locationField(chunk, key)
	if v == nil {
		return nil, false
	}
	r := reflect.ValueOf(v)
	if (r.Kind() == reflect.Slice || r.Kind() == reflect.Pointer) && r.IsNil() {
		return nil, false
	}
	return v, true
}

func slicePageJSON(raw json.RawMessage, start, end float64) json.RawMessage {
	l := position.FromParserMap(map[string]any{"positions": raw})
	return locationJSON(l.SlicePage(start, end), false)
}

func slicePageMatrixForTest(v any, start, end float64) any {
	l := position.FromParserMap(map[string]any{"positions": v})
	if l == nil {
		return nil
	}
	return l.SlicePage(start, end).IndexMatrix()
}

func pageMatrixLess(a, b []float64) bool {
	return position.Less(position.Box{Page: int(a[0]), Left: a[1], Top: a[3]}, position.Box{Page: int(b[0]), Left: b[1], Top: b[3]})
}

func setLocationField(chunk map[string]any, key string, value any) {
	_ = position.ProjectPublic(chunk)
	if value == nil {
		delete(chunk, key)
	} else {
		chunk[key] = value
	}
	l := position.FromParserMap(chunk)
	for _, k := range []string{"positions", "_pdf_positions", "sheet", "sheet_index", "page_number"} {
		delete(chunk, k)
	}
	chunk["source_location"] = l
}
