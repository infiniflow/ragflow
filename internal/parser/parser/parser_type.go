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

	"ragflow/internal/utility"
)

// GetTextOnlyParser returns a parser for readable-text extraction: previews,
// attachments, agent uploads and CLI parsing. Embedded media is skipped even
// when the extraction is nested inside an ingestion run (email attachments).
// These paths also work without an injected run media cache.
func GetTextOnlyParser(fileType utility.FileType) (ParseResultProducer, error) {
	return newParser(fileType, true)
}

// GetParser returns the ingestion parser for the given file type.
func GetParser(fileType utility.FileType) (ParseResultProducer, error) {
	return newParser(fileType, false)
}

// newParser owns the format routes for both ingestion and text extraction.
func newParser(fileType utility.FileType, textOnly bool) (ParseResultProducer, error) {
	switch fileType {
	case utility.FileTypePPTX:
		p := NewPPTXParser()
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypePPT:
		p := NewPPTParser()
		p.pptx.textOnly = textOnly
		return p, nil
	case utility.FileTypeXLSX:
		p, err := NewXLSXParser("")
		if err != nil {
			return nil, err
		}
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypeXLS:
		p, err := NewXLSParser("")
		if err != nil {
			return nil, err
		}
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypeCSV:
		return NewCSVParser(), nil
	case utility.FileTypeDOCX:
		p := NewDOCXParser()
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypeDOC:
		p := NewDOCParser()
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypePDF:
		return NewPDFParser(), nil
	case utility.FileTypeHTML:
		p := NewHTMLParser()
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypeMarkdown:
		return NewMarkdownParser(GoMarkdown)
	case utility.FileTypeTXT:
		return NewTextParser(), nil
	case utility.FileTypeEPUB:
		p := NewEPUBParser()
		p.textOnly = textOnly
		return p, nil
	case utility.FileTypeJSON:
		return NewJSONParser(), nil
	case utility.FileTypeEMAIL:
		return NewEmailParser(), nil
	case utility.FileTypeVISUAL:
		return NewPictureParser(), nil
	case utility.FileTypeAURAL:
		return NewAudioParser(), nil
	default:
		return nil, fmt.Errorf("unsupported file type: %s", fileType)
	}
}
