package parser

import "image"

// MediaBox is a unified representation of a parsed PDF element.
// It can be a text section, a table, or a figure.
type MediaBox struct {
	Text       string
	LayoutType string      // "text", "table", "figure", "equation", "header", "footer", "number", "reference"
	DocTypeKwd string      // "text", "table", "image" — assigned during post-processing
	Image      image.Image // nil for plain text boxes
}

// Boxes is a slice of MediaBox used as input/output for post-processing steps.
type Boxes []MediaBox

// StepFn processes boxes and returns the filtered/modified result.
// Steps should not retain references to the input slice.
type StepFn func(boxes Boxes) Boxes
