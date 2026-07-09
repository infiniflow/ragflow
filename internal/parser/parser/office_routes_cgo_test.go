//go:build cgo

package parser

import (
	"testing"

	"ragflow/internal/utility"
)

func TestGetParser_RoutesOfficeFamilies(t *testing.T) {
	cases := []struct {
		name     string
		fileType utility.FileType
	}{
		{name: "docx", fileType: utility.FileTypeDOCX},
		{name: "doc", fileType: utility.FileTypeDOC},
		{name: "pptx", fileType: utility.FileTypePPTX},
		{name: "ppt", fileType: utility.FileTypePPT},
		{name: "xlsx", fileType: utility.FileTypeXLSX},
		{name: "xls", fileType: utility.FileTypeXLS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := GetParser(tc.fileType, map[string]string{"lib_type": OfficeOxide})
			if err != nil {
				t.Fatalf("GetParser(%q): %v", tc.fileType, err)
			}
			if _, ok := p.(ParseResultProducer); !ok {
				t.Fatalf("GetParser(%q) returned %T, want ParseResultProducer", tc.fileType, p)
			}
		})
	}
}
