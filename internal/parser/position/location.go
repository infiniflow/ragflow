// Package position owns source geometry shared by parsers and ingestion.
package position

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

// Location keeps the coordinate vocabulary and its source identity together.
// A location contains either paginated geometry or spreadsheet geometry.
type Location struct {
	Page        *PageLocation        `json:"page,omitempty"`
	Spreadsheet *SpreadsheetLocation `json:"spreadsheet,omitempty"`
}

// PageLocation separates index geometry from preview geometry. Number is
// optional parser page metadata, independent of boxes spanning several pages.
type PageLocation struct {
	Number       *int  `json:"number,omitempty"`
	IndexBoxes   []Box `json:"index_boxes,omitempty"`
	PreviewBoxes []Box `json:"preview_boxes,omitempty"`
}

// Box retains the parser's coordinate space and a one-based page number.
type Box struct {
	Page   int     `json:"page"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}

// SpreadsheetLocation binds sheet identity to discrete cell selections.
type SpreadsheetLocation struct {
	Sheet  string      `json:"sheet,omitempty"`
	Index  *int        `json:"index,omitempty"`
	Ranges []CellRange `json:"ranges,omitempty"`
}

// CellRange uses one-based inclusive sheet, row and column indices.
type CellRange struct {
	SheetIndex int `json:"sheet_index"`
	RowStart   int `json:"row_start"`
	RowEnd     int `json:"row_end"`
	ColStart   int `json:"col_start"`
	ColEnd     int `json:"col_end"`
}

func (l *Location) Validate() error {
	if l == nil {
		return nil
	}
	if l.Page != nil && l.Spreadsheet != nil {
		return fmt.Errorf("source location has both page and spreadsheet geometry")
	}
	if l.Page != nil {
		for _, boxes := range [][]Box{l.Page.IndexBoxes, l.Page.PreviewBoxes} {
			for _, box := range boxes {
				for _, v := range []float64{box.Left, box.Right, box.Top, box.Bottom} {
					if math.IsNaN(v) || math.IsInf(v, 0) {
						return fmt.Errorf("source location has non-finite coordinates")
					}
				}
			}
		}
	}
	if l.Spreadsheet != nil && len(l.Spreadsheet.Ranges) > 0 {
		s := l.Spreadsheet
		index := s.Ranges[0].SheetIndex
		if s.Index != nil {
			index = *s.Index
		}
		for _, r := range s.Ranges {
			if r.SheetIndex != index {
				return fmt.Errorf("spreadsheet location contains conflicting sheet identities")
			}
		}
	}
	return nil
}

func (l *Location) Clone() *Location {
	if l == nil {
		return nil
	}
	out := *l
	if l.Page != nil {
		p := *l.Page
		p.IndexBoxes = append([]Box(nil), p.IndexBoxes...)
		p.PreviewBoxes = append([]Box(nil), p.PreviewBoxes...)
		if p.Number != nil {
			n := *p.Number
			p.Number = &n
		}
		out.Page = &p
	}
	if l.Spreadsheet != nil {
		s := *l.Spreadsheet
		s.Ranges = append([]CellRange(nil), s.Ranges...)
		if s.Index != nil {
			n := *s.Index
			s.Index = &n
		}
		out.Spreadsheet = &s
	}
	return &out
}

func (l *Location) IsSpreadsheet() bool { return l != nil && l.Spreadsheet != nil }

func (l *Location) HasCoordinates() bool {
	return l != nil && ((l.Page != nil && (len(l.Page.IndexBoxes) > 0 || len(l.Page.PreviewBoxes) > 0)) || (l.Spreadsheet != nil && len(l.Spreadsheet.Ranges) > 0))
}

// Preview uses the explicit preview geometry, falling back to index boxes.
func (l *Location) Preview() []Box {
	if l == nil || l.Page == nil {
		return nil
	}
	if len(l.Page.PreviewBoxes) > 0 {
		return l.Page.PreviewBoxes
	}
	return l.Page.IndexBoxes
}

func (l *Location) SameSheet(other *Location) bool {
	if !l.IsSpreadsheet() || !other.IsSpreadsheet() {
		return false
	}
	a, b := l.Spreadsheet, other.Spreadsheet
	if a.Index != nil || b.Index != nil {
		return a.Index != nil && b.Index != nil && *a.Index == *b.Index
	}
	if len(a.Ranges) > 0 && len(b.Ranges) > 0 {
		return a.Ranges[0].SheetIndex == b.Ranges[0].SheetIndex
	}
	return a.Sheet != "" && a.Sheet == b.Sheet
}

// SelectRows changes only a spreadsheet's ranges, retaining its identity.
func (l *Location) SelectRows(indices []int) *Location {
	out := l.Clone()
	if !out.IsSpreadsheet() {
		return out
	}
	out.Spreadsheet.Ranges = nil
	for _, i := range indices {
		if i < 0 || i >= len(l.Spreadsheet.Ranges) {
			continue
		}
		out.Spreadsheet.Ranges = append(out.Spreadsheet.Ranges, l.Spreadsheet.Ranges[i])
	}
	return out
}

// WithoutIndex retains identity and preview geometry when a derived chunk
// cannot be assigned an exact source selection.
func (l *Location) WithoutIndex() *Location {
	out := l.Clone()
	if out == nil {
		return nil
	}
	if out.Page != nil {
		out.Page.IndexBoxes = nil
	}
	if out.Spreadsheet != nil {
		out.Spreadsheet.Ranges = nil
	}
	return out
}

// SlicePage slices paginated geometry by accumulated vertical height. Sheet
// ranges remain discrete and are never interpolated as page rectangles.
func (l *Location) SlicePage(start, end float64) *Location {
	out := l.Clone()
	if out == nil || out.Page == nil {
		return out
	}
	out.Page.IndexBoxes = sliceBoxes(l.Page.IndexBoxes, start, end)
	out.Page.PreviewBoxes = sliceBoxes(l.Page.PreviewBoxes, start, end)
	return out
}

func sliceBoxes(boxes []Box, start, end float64) []Box {
	start = max(0, start)
	end = min(1, end)
	if start >= end {
		return append([]Box(nil), boxes...)
	}
	total := 0.0
	for _, b := range boxes {
		if b.Bottom <= b.Top {
			return append([]Box(nil), boxes...)
		}
		total += b.Bottom - b.Top
	}
	if total == 0 {
		return nil
	}
	var out []Box
	cum := 0.0
	for _, b := range boxes {
		h := b.Bottom - b.Top
		lo, hi := max(cum, start*total), min(cum+h, end*total)
		if lo < hi {
			b.Bottom = b.Top + hi - cum
			b.Top += lo - cum
			out = append(out, b)
		}
		cum += h
	}
	return out
}

// Append adds geometry in source order to an owned destination. A nil
// destination clones the source; callers clone parser locations before using
// them as destinations. The appended source is never modified or aliased.
func (l *Location) Append(other *Location) *Location {
	if l == nil {
		return other.Clone()
	}
	if other == nil {
		return l
	}
	if (l.Page == nil && l.Spreadsheet == nil) || (other.Page == nil && other.Spreadsheet == nil) {
		return &Location{}
	}
	out := l
	if out.Page != nil && other.Page != nil {
		out.Page.IndexBoxes = append(out.Page.IndexBoxes, other.Page.IndexBoxes...)
		out.Page.PreviewBoxes = append(out.Page.PreviewBoxes, other.Page.PreviewBoxes...)
		if out.Page.Number == nil && other.Page.Number != nil {
			n := *other.Page.Number
			out.Page.Number = &n
		}
		return out
	}
	if l.SameSheet(other) {
		out.Spreadsheet.Ranges = append(out.Spreadsheet.Ranges, other.Spreadsheet.Ranges...)
		return out
	}
	return &Location{}
}

// MergePage preserves the sorted, deduplicated geometry of General/Title.
func (l *Location) MergePage(other *Location) *Location {
	out := l.Clone().Append(other)
	if out != nil && out.Page != nil {
		out.Page.IndexBoxes = sortedBoxes(out.Page.IndexBoxes)
		out.Page.PreviewBoxes = sortedBoxes(out.Page.PreviewBoxes)
	}
	return out
}

func sortedBoxes(boxes []Box) []Box {
	seen := make(map[Box]bool, len(boxes))
	out := make([]Box, 0, len(boxes))
	for _, b := range boxes {
		if !seen[b] {
			seen[b] = true
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return Less(out[i], out[j]) })
	return out
}

func Less(a, b Box) bool {
	if a.Page != b.Page {
		return a.Page < b.Page
	}
	if a.Top != b.Top {
		return a.Top < b.Top
	}
	return a.Left < b.Left
}

// IndexMatrix is the existing storage/API coordinate contract.
func (l *Location) IndexMatrix() [][]float64 {
	if l == nil {
		return nil
	}
	var out [][]float64
	if l.Page != nil {
		for _, b := range l.Page.IndexBoxes {
			out = append(out, []float64{float64(b.Page), b.Left, b.Right, b.Top, b.Bottom})
		}
	}
	if l.Spreadsheet != nil {
		for _, r := range l.Spreadsheet.Ranges {
			out = append(out, []float64{float64(r.SheetIndex), float64(r.RowStart), float64(r.RowEnd), float64(r.ColStart), float64(r.ColEnd)})
		}
	}
	return out
}

// FromMap decodes the canonical component payload, never inferring a type
// from the length of a coordinate tuple.
func FromMap(m map[string]any) (*Location, error) {
	v, ok := m["source_location"]
	if !ok || v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var l *Location
	if err = json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	return l, l.Validate()
}

// FromParserMap normalizes the backend's position fields at the parser seam.
// Runtime component outputs use source_location, not these backend keys.
func FromParserMap(m map[string]any) *Location {
	if _, ok := m["source_location"]; ok {
		l, err := FromMap(m)
		if err == nil {
			return l
		}
		return nil
	}
	sheet, _ := m["sheet"].(string)
	index := numberPointer(m["sheet_index"])
	if sheet != "" || index != nil {
		s := &SpreadsheetLocation{Sheet: sheet, Index: index}
		for _, row := range matrix(m["positions"]) {
			if index != nil && int(row[0]) != *index {
				s.Ranges = nil
				return &Location{Spreadsheet: s}
			}
			s.Ranges = append(s.Ranges, CellRange{int(row[0]), int(row[1]), int(row[2]), int(row[3]), int(row[4])})
		}
		location := &Location{Spreadsheet: s}
		if location.Validate() != nil {
			s.Ranges = nil
		}
		return location
	}
	p := &PageLocation{Number: numberPointer(m["page_number"]), IndexBoxes: boxes(matrix(m["positions"])), PreviewBoxes: boxes(matrix(m["_pdf_positions"]))}
	if p.Number == nil && len(p.IndexBoxes) == 0 && len(p.PreviewBoxes) == 0 {
		return nil
	}
	return &Location{Page: p}
}

func numberPointer(v any) *int {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var n float64
	if json.Unmarshal(b, &n) != nil {
		return nil
	}
	i := int(n)
	return &i
}

func matrix(v any) [][]float64 {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var rows [][]float64
	if json.Unmarshal(b, &rows) != nil {
		rows = nil
		var flat []float64
		if json.Unmarshal(b, &flat) == nil && len(flat)%5 == 0 {
			for i := 0; i < len(flat); i += 5 {
				rows = append(rows, flat[i:i+5])
			}
		} else {
			// Some parser responses attach one rectangle to several pages.
			var rawRows [][]json.RawMessage
			if json.Unmarshal(b, &rawRows) != nil {
				return nil
			}
			for _, rawRow := range rawRows {
				if len(rawRow) != 5 {
					return nil
				}
				var pages []float64
				if json.Unmarshal(rawRow[0], &pages) != nil {
					var page float64
					if json.Unmarshal(rawRow[0], &page) != nil {
						return nil
					}
					pages = []float64{page}
				}
				coordinates := make([]float64, 4)
				for i := range coordinates {
					if json.Unmarshal(rawRow[i+1], &coordinates[i]) != nil {
						return nil
					}
				}
				for _, page := range pages {
					rows = append(rows, append([]float64{page}, coordinates...))
				}
			}
		}
	}
	var out [][]float64
	for _, row := range rows {
		if len(row) != 5 {
			return nil
		}
		valid := true
		for _, v := range row {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				valid = false
			}
		}
		if valid {
			out = append(out, row)
		}
	}
	return out
}

func boxes(rows [][]float64) []Box {
	var out []Box
	for _, r := range rows {
		out = append(out, Box{int(r[0]), r[1], r[2], r[3], r[4]})
	}
	return out
}
