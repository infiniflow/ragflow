//go:build cgo && office

//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"fmt"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

const (
	OfficeOxide string = "office_oxide"
)

type DOCXParser struct {
	libType string
}

func NewDOCXParser(libType string) (*DOCXParser, error) {
	switch libType {
	case OfficeOxide:
		return &DOCXParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported DOCX library type: %s", libType)
	}
}

func (p *DOCXParser) Parse(filename string, data []byte) error {

	switch p.libType {
	case OfficeOxide:
		return p.OfficeOxideParse(data)
	default:
		return fmt.Errorf("unsupported DOCX library type: %s", p.libType)
	}
}

func (p *DOCXParser) OfficeOxideParse(data []byte) error {

	doc, err := officeOxide.OpenFromBytes(data, "docx")
	if err != nil {
		return err
	}
	defer doc.Close()

	docFormat, err := doc.Format()
	if err != nil {
		return err
	}

	fmt.Println("Document format:", docFormat)

	docContext, err := doc.PlainText()
	if err != nil {
		return err
	}
	fmt.Println("Document context:", docContext)

	md, err := doc.ToMarkdown()
	if err != nil {
		return err
	}
	fmt.Println("Document Markdown:", md)
	return nil
}

// ParseWithResult captures the office_oxide ToMarkdown output
// instead of discarding it. Returns OutputFormat="markdown" with
// the rendered Markdown on the matching key. Mirrors the python
// parser.py:Docx branch which uses rag.app.naive.Docx to render
// markdown; the Go side delegates to office_oxide for now and
// will switch to a native Markdown renderer in a follow-up.
func (p *DOCXParser) ParseWithResult(filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "docx")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("docx open: %w", err)}
	}
	defer doc.Close()

	md, err := doc.ToMarkdown()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("docx to-markdown: %w", err)}
	}

	return ParseResult{
		OutputFormat: "markdown",
		File:         map[string]any{"name": filename, "format": "docx"},
		Markdown:     md,
	}
}

func (p *DOCXParser) String() string {
	return "DOCXParser"
}
