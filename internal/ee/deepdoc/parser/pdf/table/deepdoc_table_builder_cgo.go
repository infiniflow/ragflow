//go:build cgo

package table

import (
	pdfparser "ragflow/internal/deepdoc/parser/pdf"
	inference "ragflow/internal/deepdoc/parser/pdf/inference"
	pdft "ragflow/internal/deepdoc/parser/pdf/type"
)

// EE model label taxonomies.
// DLA: 10 classes with duplicates (matching EE Docker TSR endpoint).
var eeDLALabels = []string{
	pdft.LayoutTypeText, pdft.LayoutTypeTitle, pdft.LayoutTypeText, pdft.LayoutTypeReference,
	pdft.LayoutTypeFigure, pdft.DLALabelFigureCaption,
	pdft.LayoutTypeTable, pdft.DLALabelTableCaption, pdft.DLALabelTableCaption,
	pdft.LayoutTypeEquation, pdft.DLALabelFigureCaption,
}

// TSR: 2-class separator lines (v=vertical, h=horizontal).
var eeTSRLabels = []string{"v", "h"}

// NewDeepDocTableBuilder creates an EE TableBuilder backed by the DeepDoc service.
// If doc is a *DeepDocClient, its label tables are set to the EE taxonomy.
func NewDeepDocTableBuilder(doc pdft.DocAnalyzer) pdft.TableBuilder {
	if c, ok := doc.(*inference.InferenceClient); ok {
		c.DLALabels = eeDLALabels
		c.TSRLabels = eeTSRLabels
	}
	return &DeepDocTableBuilder{doc: doc}
}

// init registers the EE TableBuilder factory for ModelEE.
func init() {
	pdfparser.RegisterTableBuilder(NewDeepDocTableBuilder)
}
