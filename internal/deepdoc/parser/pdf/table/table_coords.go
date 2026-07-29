package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── coordinate space conversion helpers ──────────────────────────────

// CellToPageSpace converts from crop-pixel space to page-global 72-DPI space.
func CellToPageSpace(c pdf.TSRCell, cropOffX, cropOffY, scale float64) pdf.TSRCell {
	return pdf.TSRCell{
		X0: (c.X0 + cropOffX) / scale, Y0: (c.Y0 + cropOffY) / scale,
		X1: (c.X1 + cropOffX) / scale, Y1: (c.Y1 + cropOffY) / scale,
		Text: c.Text, Label: c.Label,
	}
}

// CellAddOffset applies a crop offset to cell coordinates (stays in pixel space).
func CellAddOffset(c pdf.TSRCell, offX, offY float64) pdf.TSRCell {
	return pdf.TSRCell{
		X0: c.X0 + offX, Y0: c.Y0 + offY, X1: c.X1 + offX, Y1: c.Y1 + offY,
		Text: c.Text, Label: c.Label,
	}
}

// CellSliceToPageSpace converts a slice of cells from crop-pixel to page DPI space.
func CellSliceToPageSpace(cells []pdf.TSRCell, cropOffX, cropOffY, scale float64) []pdf.TSRCell {
	out := make([]pdf.TSRCell, len(cells))
	for i, c := range cells {
		out[i] = CellToPageSpace(c, cropOffX, cropOffY, scale)
	}
	return out
}

// BoxToCropSpace converts a pdf.TextBox from PDF-point space to crop-pixel space.
func BoxToCropSpace(b pdf.TextBox, scale, cropOffX, cropOffY float64) pdf.TextBox {
	return pdf.TextBox{
		X0: b.X0*scale - cropOffX, X1: b.X1*scale - cropOffX,
		Top: b.Top*scale - cropOffY, Bottom: b.Bottom*scale - cropOffY,
		Text: b.Text,
	}
}

// CopyBoxAnnotations copies the DLA/TSR annotation fields from src to dst.
func CopyBoxAnnotations(dst, src *pdf.TextBox) {
	dst.R = src.R
	dst.C = src.C
	dst.RTop = src.RTop
	dst.RBott = src.RBott
	dst.H = src.H
	dst.HTop = src.HTop
	dst.HBott = src.HBott
	dst.HLeft = src.HLeft
	dst.HRight = src.HRight
	dst.CLeft = src.CLeft
	dst.CRight = src.CRight
	dst.SP = src.SP
}
