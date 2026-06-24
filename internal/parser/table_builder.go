package parser

import "image"

// TableBuilder encapsulates TSR model-specific cell detection and grouping.
// Each TSR model implements its own Builder, producing a unified row-column
// grid consumed by the shared downstream pipeline.
type TableBuilder interface {
	// Name returns the model identifier for logging and diagnostics.
	Name() string

	// DetectCells detects all cells from a cropped table image.
	// The Label field on returned TSRCells is consumed only by the Builder
	// itself during GroupCells; shared code does not depend on Label semantics.
	DetectCells(cropped image.Image) ([]TSRCell, error)

	// GroupCells groups cells into a row-column grid.
	// DeepDoc: groups by Label strings ("table row", "table column header", etc).
	// Hierarchical models: computes the grid by cross-referencing row and column cells.
	// Returns grid[row][col] consumed directly by annotateTableBoxes and constructTable.
	GroupCells(cells []TSRCell) [][]TSRCell
}
