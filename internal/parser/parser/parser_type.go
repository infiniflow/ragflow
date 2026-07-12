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
		return NewXLSXParser(), nil
	case utility.FileTypeXLS:
		return NewXLSParser(), nil
	case utility.FileTypeDOCX:
		return NewDOCXParser(), nil
	case utility.FileTypeDOC:
		return NewDOCParser(), nil
	case utility.FileTypePDF:
		return NewPDFParser(), nil
	case utility.FileTypeHTML:
		return NewHTMLParser(), nil
	case utility.FileTypeMarkdown:
		return NewMarkdownParser(), nil
	case utility.FileTypeTXT:
		return NewTextParser(), nil
	default:
		return nil, fmt.Errorf("unsupported file type: %s", fileType)
	}
}
