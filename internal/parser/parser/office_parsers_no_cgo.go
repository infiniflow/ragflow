//go:build !cgo

package parser

import (
	"context"
	"errors"
	"fmt"
)

// ErrOfficeCGORequired is returned by ParseWithResult on every
// office-parser family (DOC / DOCX / PPT / PPTX)
// when the build is not CGO-enabled. The CGO build's
// implementation captures the office_oxide PlainText / ToMarkdown
// output; this stub mirrors that surface so the package compiles
// and existing tests pass. The error is surfaced at parse time
// rather than at construction time, matching the NewPDFParser
// shape used by the rest of the package.
var ErrOfficeCGORequired = errors.New("parser: office family requires CGO (office_oxide)")

func (p *DOCXParser) ParseWithResult(_ context.Context, filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: docx", ErrOfficeCGORequired),
	}
}

func (p *DOCParser) ParseWithResult(_ context.Context, filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: doc", ErrOfficeCGORequired),
	}
}

func (p *PPTParser) ParseWithResult(_ context.Context, filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: ppt", ErrOfficeCGORequired),
	}
}

func (p *PPTXParser) ParseWithResult(_ context.Context, filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: pptx", ErrOfficeCGORequired),
	}
}

type DOCParser struct{}

func NewDOCParser() *DOCParser {
	return &DOCParser{}
}

func (p *DOCParser) String() string {
	return "DOCParser(no-cgo)"
}

type DOCXParser struct{}

func NewDOCXParser() *DOCXParser {
	return &DOCXParser{}
}

func (p *DOCXParser) ConfigureFromSetup(setup map[string]any) {
	// No-op in the no-CGO stub: the real implementation in
	// docx_parser.go reads output_format from setup.
}

func (p *DOCXParser) String() string {
	return "DOCXParser(no-cgo)"
}

type PPTParser struct{}

func NewPPTParser() *PPTParser {
	return &PPTParser{}
}

func (p *PPTParser) String() string {
	return "PPTParser(no-cgo)"
}

type PPTXParser struct{}

func NewPPTXParser() *PPTXParser {
	return &PPTXParser{}
}

func (p *PPTXParser) String() string {
	return "PPTXParser(no-cgo)"
}
