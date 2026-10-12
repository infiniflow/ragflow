package position

// ProjectPublic replaces internal source geometry with the existing preview
// response fields. It belongs at public debug/API boundaries, not between
// ingestion components.
func ProjectPublic(chunk map[string]any) error {
	l, err := FromMap(chunk)
	if err != nil {
		return err
	}
	delete(chunk, "source_location")
	if l == nil {
		return nil
	}
	if matrix := l.IndexMatrix(); len(matrix) > 0 {
		chunk["positions"] = matrix
	}
	if l.Page != nil {
		if len(l.Page.PreviewBoxes) > 0 {
			var matrix [][]float64
			for _, box := range l.Page.PreviewBoxes {
				matrix = append(matrix, []float64{float64(box.Page), box.Left, box.Right, box.Top, box.Bottom})
			}
			chunk["_pdf_positions"] = matrix
		}
		if l.Page.Number != nil {
			chunk["page_number"] = *l.Page.Number
		}
	}
	if l.Spreadsheet != nil {
		if l.Spreadsheet.Sheet != "" {
			chunk["sheet"] = l.Spreadsheet.Sheet
		}
		if l.Spreadsheet.Index != nil {
			chunk["sheet_index"] = *l.Spreadsheet.Index
		}
	}
	return nil
}
