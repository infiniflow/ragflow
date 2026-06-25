package parser

import (
	"context"
	"image"
)

// TableBuilder encapsulates TSR model-specific cell detection and grouping.
// Each TSR model implements its own Builder, producing a unified row-column
// grid consumed by the shared downstream pipeline.
type TableBuilder interface {
	// Name returns the model identifier for logging and diagnostics.
	Name() string

	// DetectCells detects all cells from a cropped table image.
	// The Label field on returned TSRCells is consumed only by the Builder
	// itself during GroupCells; shared code does not depend on Label semantics.
	DetectCells(ctx context.Context, cropped image.Image) ([]TSRCell, error)

	// GroupCells groups cells into a row-column grid (pure computation, no I/O).
	GroupCells(cells []TSRCell) [][]TSRCell
}
