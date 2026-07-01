//go:build !cgo || !office

package parser

import "fmt"

// OfficeOxide is the lib_type identifier for office_oxide backend.
const OfficeOxide = "office_oxide"

type DOCParser struct {
	libType string
}

func NewDOCParser(libType string) (*DOCParser, error) {
	return nil, fmt.Errorf("DOC parser requires CGO (office_oxide)")
}

func (p *DOCParser) Parse(_ string, _ []byte) error {
	return fmt.Errorf("DOC parser requires CGO (office_oxide)")
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

func (p *DOCXParser) Parse(_ string, _ []byte) error {
	return fmt.Errorf("DOCX parser requires CGO (office_oxide)")
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

func (p *XLSParser) Parse(_ string, _ []byte) error {
	return fmt.Errorf("XLS parser requires CGO (office_oxide)")
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

func (p *XLSXParser) Parse(_ string, _ []byte) error {
	return fmt.Errorf("XLSX parser requires CGO (office_oxide)")
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

func (p *PPTParser) Parse(_ string, _ []byte) error {
	return fmt.Errorf("PPT parser requires CGO (office_oxide)")
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

func (p *PPTXParser) Parse(_ string, _ []byte) error {
	return fmt.Errorf("PPTX parser requires CGO (office_oxide)")
}

func (p *PPTXParser) String() string {
	return "PPTXParser(no-cgo)"
}
