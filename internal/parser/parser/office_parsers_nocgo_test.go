//go:build !cgo

package parser

import (
	"errors"
	"testing"
)

func TestOfficeParsers_ParseWithResult_NoCGO(t *testing.T) {
	cases := []struct {
		name string
		res  ParseResult
	}{
		{name: "docx", res: (&DOCXParser{}).ParseWithResult("a.docx", nil)},
		{name: "doc", res: (&DOCParser{}).ParseWithResult("a.doc", nil)},
		{name: "xlsx", res: (&XLSXParser{}).ParseWithResult("a.xlsx", nil)},
		{name: "xls", res: (&XLSParser{}).ParseWithResult("a.xls", nil)},
		{name: "pptx", res: (&PPTXParser{}).ParseWithResult("a.pptx", nil)},
		{name: "ppt", res: (&PPTParser{}).ParseWithResult("a.ppt", nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.res.Err == nil {
				t.Fatal("want ErrOfficeCGORequired, got nil")
			}
			if !errors.Is(tc.res.Err, ErrOfficeCGORequired) {
				t.Fatalf("err = %v, want wraps ErrOfficeCGORequired", tc.res.Err)
			}
		})
	}
}
