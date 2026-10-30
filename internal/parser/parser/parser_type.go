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

// GetTextOnlyParser returns the parser used by callers that extract readable
// text outside an ingestion run: file previews, chat attachments, agent
// uploads and CLI parsing. These callers have no run media cache, so formats
// whose normal parser writes image references return a text-only variant that
// skips embedded images instead of failing on them.
func GetTextOnlyParser(fileType utility.FileType) (ParseResultProducer, error) {
	switch fileType {
	case utility.FileTypeHTML:
		return NewHTMLTextParser(), nil
	case utility.FileTypeEPUB:
		return NewEPUBTextParser(), nil
	case utility.FileTypeDOCX:
		return NewDOCXTextParser(), nil
	case utility.FileTypeDOC:
		return NewDOCTextParser(), nil
	case utility.FileTypePPTX:
		return NewPPTXTextParser(), nil
	case utility.FileTypePPT:
		return NewPPTTextParser(), nil
	case utility.FileTypeXLSX:
		return NewXLSXTextParser(), nil
	case utility.FileTypeXLS:
		return NewXLSTextParser(), nil
	default:
		return GetParser(fileType)
	}
}

// GetParser returns the ParseResultProducer for the given file type.
// Each format picks a single backend — the Python dispatcher does
// not pass any lib_type knob for these families, so the Go side
// mirrors that with one constructor per format. Constructors mirror
// the NewPDFParser shape (no error return): none of them have inputs
// that could fail, so the error slot is dead weight.
func GetParser(fileType utility.FileType) (ParseResultProducer, error) {
	switch fileType {
	case utility.FileTypePPTX:
		return NewPPTXParser(), nil
	case utility.FileTypePPT:
		return NewPPTParser(), nil
	case utility.FileTypeXLSX:
		return NewXLSXParser("")
	case utility.FileTypeXLS:
		return NewXLSParser("")
	case utility.FileTypeCSV:
		return NewCSVParser(), nil
	case utility.FileTypeDOCX:
		return NewDOCXParser(), nil
	case utility.FileTypeDOC:
		return NewDOCParser(), nil
	case utility.FileTypePDF:
		return NewPDFParser(), nil
	case utility.FileTypeHTML:
		return NewHTMLParser(), nil
	case utility.FileTypeMarkdown:
		return NewMarkdownParser(GoMarkdown)
	case utility.FileTypeTXT:
		return NewTextParser(), nil
	case utility.FileTypeEPUB:
		return NewEPUBParser(), nil
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
