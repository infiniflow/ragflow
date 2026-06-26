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

type XLSXParser struct {
	libType string
}

func NewXLSXParser(libType string) (*XLSXParser, error) {
	switch libType {
	case OfficeOxide:
		return &XLSXParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported XLSX library type: %s", libType)
	}
}

func (p *XLSXParser) Parse(filename string, data []byte) error {
	fmt.Printf("Parsing XLSX file: %s\n", filename)
	switch p.libType {
	case OfficeOxide:
		return p.OfficeOxideParse(data)
	default:
		return fmt.Errorf("unsupported XLSX library type: %s", p.libType)
	}
}

func (p *XLSXParser) OfficeOxideParse(data []byte) error {
	doc, err := officeOxide.OpenFromBytes(data, "xlsx")
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

func (p *XLSXParser) String() string {
	return "XLSXParser"
}
