//go:build !cgo

package parser

import (
	"context"
	"errors"
	"testing"
)

func TestOfficeParsers_ParseWithResult_NoCGO(t *testing.T) {
	ctx := t.Context()
	cases := []struct {
		name string
		res  ParseResult
	}{
		{name: "docx", res: (&DOCXParser{}).ParseWithResult(ctx, "a.docx", nil)},
		{name: "doc", res: (&DOCParser{}).ParseWithResult(ctx, "a.doc", nil)},
		{name: "pptx", res: (&PPTXParser{}).ParseWithResult(ctx, "a.pptx", nil)},
		{name: "ppt", res: (&PPTParser{}).ParseWithResult(ctx, "a.ppt", nil)},
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
