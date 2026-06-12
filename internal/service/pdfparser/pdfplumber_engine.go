package pdfparser

// pdfplumberEngine adapts pdfplumber_adapter.go's Document to the PDFEngine interface.
// The heavy lifting (pdf_oxide interaction, char extraction, rendering) lives in
// pdfplumber_adapter.go, which is maintained as a standalone adapter layer.
type pdfplumberEngine struct {
	doc *Document
}

// NewPDFPlumberEngine creates a PDFEngine backed by pdf_oxide via the adapter layer.
func NewPDFPlumberEngine(pdfBytes []byte) (PDFEngine, error) {
	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		return nil, err
	}
	return &pdfplumberEngine{doc: doc}, nil
}

func (e *pdfplumberEngine) ExtractChars(pageNum int) ([]TextChar, error) {
	chars, err := e.doc.GetDedupePageChars(pageNum, 0.5)
	if err != nil {
		return nil, err
	}
	result := make([]TextChar, len(chars))
	for i, c := range chars {
		result[i] = TextChar{
			X0:         c.X0,
			X1:         c.X1,
			Top:        c.Top,
			Bottom:     c.Bottom,
			Text:       c.Text,
			FontName:   c.Fontname,
			FontSize:   c.Size,
			PageNumber: pageNum,
		}
	}
	return result, nil
}

func (e *pdfplumberEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	result, err := e.doc.RenderPage(pageNum, dpi)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (e *pdfplumberEngine) ExtractTables(pageNum int) ([]TableRegion, error) {
	return e.doc.ExtractTableCells(pageNum)
}

func (e *pdfplumberEngine) PageCount() (int, error) { return e.doc.PageCount() }

func (e *pdfplumberEngine) Close() error { e.doc.Close(); return nil }
