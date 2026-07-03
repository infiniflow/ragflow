//go:build cgo

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

type DOCParser struct {
	libType string
}

func NewDOCParser(libType string) (*DOCParser, error) {
	switch libType {
	case OfficeOxide:
		return &DOCParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported DOC library type: %s", libType)
	}
}

func (p *DOCParser) String() string {
	return "DOCParser"
}

// ParseWithResult captures the office_oxide PlainText output for
// the DOC family. Python parser.py routes .doc through tika;
// the Go side uses office_oxide which supports DOC via PlainText.
// OutputFormat="text" — the python side falls back to text for
// legacy DOC files since structured extraction is unreliable.
func (p *DOCParser) ParseWithResult(filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "doc")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("doc open: %w", err)}
	}
	defer doc.Close()

	text, err := doc.PlainText()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("doc plain-text: %w", err)}
	}

	return ParseResult{
		OutputFormat: "text",
		File:         map[string]any{"name": filename, "format": "doc"},
		Text:         text,
	}
}
