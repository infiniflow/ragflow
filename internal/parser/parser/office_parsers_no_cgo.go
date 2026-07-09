//go:build !cgo

package parser

import (
	"errors"
	"fmt"
)

// OfficeOxide is the lib_type identifier for office_oxide backend.
const OfficeOxide = "office_oxide"

// ErrOfficeCGORequired is returned by ParseWithResult on every
// office-parser family (DOC / DOCX / PPT / PPTX / XLS / XLSX)
// when the build is not CGO-enabled. The CGO build's
// implementation captures the office_oxide PlainText / ToMarkdown
// output; this stub mirrors that surface so the package compiles
// and existing tests pass.
var ErrOfficeCGORequired = errors.New("parser: office family requires CGO (office_oxide)")

// docxParseWithResultNoCGO is the no-CGO stub for the DOCX
// family. The CGO build's implementation lives in docx_parser.go
// under //go:build cgo.
func (p *DOCXParser) ParseWithResult(filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: docx", ErrOfficeCGORequired),
	}
}

func (p *DOCParser) ParseWithResult(filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: doc", ErrOfficeCGORequired),
	}
}

func (p *XLSParser) ParseWithResult(filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: xls", ErrOfficeCGORequired),
	}
}

func (p *XLSXParser) ParseWithResult(filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: xlsx", ErrOfficeCGORequired),
	}
}

func (p *PPTParser) ParseWithResult(filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: ppt", ErrOfficeCGORequired),
	}
}

func (p *PPTXParser) ParseWithResult(filename string, _ []byte) ParseResult {
	return ParseResult{
		File: map[string]any{"name": filename},
		Err:  fmt.Errorf("%w: pptx", ErrOfficeCGORequired),
	}
}

type DOCParser struct {
	libType string
}

func NewDOCParser(libType string) (*DOCParser, error) {
	return nil, fmt.Errorf("DOC parser requires CGO (office_oxide)")
}

func (p *DOCParser) String() string {
	return "DOCParser(no-cgo)"
}

type DOCXParser struct {
	libType string
}

func NewDOCXParser(libType string) (*DOCXParser, error) {
	return nil, fmt.Errorf("DOCX parser requires CGO (office_oxide)")
}

func (p *DOCXParser) String() string {
	return "DOCXParser(no-cgo)"
}

type XLSParser struct {
	libType string
}

func NewXLSParser(libType string) (*XLSParser, error) {
	return nil, fmt.Errorf("XLS parser requires CGO (office_oxide)")
}

func (p *XLSParser) String() string {
	return "XLSParser(no-cgo)"
}

type XLSXParser struct {
	libType string
}

func NewXLSXParser(libType string) (*XLSXParser, error) {
	return nil, fmt.Errorf("XLSX parser requires CGO (office_oxide)")
}

func (p *XLSXParser) String() string {
	return "XLSXParser(no-cgo)"
}

type PPTParser struct {
	libType string
}

func NewPPTParser(libType string) (*PPTParser, error) {
	return nil, fmt.Errorf("PPT parser requires CGO (office_oxide)")
}

func (p *PPTParser) String() string {
	return "PPTParser(no-cgo)"
}

type PPTXParser struct {
	libType string
}

func NewPPTXParser(libType string) (*PPTXParser, error) {
	return nil, fmt.Errorf("PPTX parser requires CGO (office_oxide)")
}

func (p *PPTXParser) String() string {
	return "PPTXParser(no-cgo)"
}
