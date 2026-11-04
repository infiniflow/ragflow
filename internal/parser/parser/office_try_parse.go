package parser

import (
	"archive/zip"
	"bytes"

	"github.com/richardlehane/mscfb"
	"ragflow/internal/utility"
)

func (*DOCParser) TryParse(data []byte) utility.FileType  { return tryParseOffice(data) }
func (*DOCXParser) TryParse(data []byte) utility.FileType { return tryParseOffice(data) }
func (*XLSParser) TryParse(data []byte) utility.FileType  { return tryParseOffice(data) }
func (*XLSXParser) TryParse(data []byte) utility.FileType { return tryParseOffice(data) }
func (*PPTParser) TryParse(data []byte) utility.FileType  { return tryParseOffice(data) }
func (*PPTXParser) TryParse(data []byte) utility.FileType { return tryParseOffice(data) }

// tryParseOffice reads container directories without extracting document bodies.
// A container signature alone is insufficient to identify an Office format.
func tryParseOffice(data []byte) utility.FileType {
	actual := utility.FileTypeOTHER
	accept := func(candidate utility.FileType) bool {
		if actual != utility.FileTypeOTHER && actual != candidate {
			return false
		}
		actual = candidate
		return true
	}
	switch {
	case bytes.HasPrefix(data, []byte{'P', 'K', 3, 4}):
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return utility.FileTypeOTHER
		}
		hasContentTypes := false
		for _, f := range r.File {
			switch f.Name {
			case "[Content_Types].xml":
				hasContentTypes = true
			case "word/document.xml":
				if !accept(utility.FileTypeDOCX) {
					return utility.FileTypeOTHER
				}
			case "xl/workbook.xml":
				if !accept(utility.FileTypeXLSX) {
					return utility.FileTypeOTHER
				}
			case "ppt/presentation.xml":
				if !accept(utility.FileTypePPTX) {
					return utility.FileTypeOTHER
				}
			}
		}
		if !hasContentTypes {
			return utility.FileTypeOTHER
		}
	case bytes.HasPrefix(data, CompoundDocumentSignature):
		r, err := mscfb.New(bytes.NewReader(data))
		if err != nil {
			return utility.FileTypeOTHER
		}
		for _, f := range r.File {
			if len(f.Path) != 0 || f.FileInfo().IsDir() {
				continue
			}
			switch f.Name {
			case "EncryptedPackage", "EncryptionInfo":
				return utility.FileTypeOTHER
			case "WordDocument":
				if !accept(utility.FileTypeDOC) {
					return utility.FileTypeOTHER
				}
			case "Workbook", "Book":
				if !accept(utility.FileTypeXLS) {
					return utility.FileTypeOTHER
				}
			case "PowerPoint Document":
				if !accept(utility.FileTypePPT) {
					return utility.FileTypeOTHER
				}
			}
		}
	}
	return actual
}
