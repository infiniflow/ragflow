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
	"strings"

	"ragflow/internal/utility"
)

// TryParser optionally identifies a format before full parsing by inspecting
// recognition structures. FileTypeOTHER means unknown: keep the requested type.
type TryParser interface {
	TryParse(data []byte) utility.FileType
}

// GetTextOnlyParser selects a parser and effective file type for readable-text
// extraction: previews, attachments, agent uploads and CLI parsing.
// Embedded media is skipped even when extraction is nested inside an ingestion
// run (email attachments).
// These paths also work without an injected run media cache.
func GetTextOnlyParser(fileType utility.FileType, data []byte) (ParseResultProducer, utility.FileType, error) {
	return selectParser(fileType, data, true)
}

// GetParser selects the ingestion parser and effective file type from the
// requested type and contents. Unrecognized contents keep the requested type.
func GetParser(fileType utility.FileType, data []byte) (ParseResultProducer, utility.FileType, error) {
	return selectParser(fileType, data, false)
}

func selectParser(fileType utility.FileType, data []byte, textOnly bool) (ParseResultProducer, utility.FileType, error) {
	p, err := newParser(fileType, textOnly)
	if err != nil {
		return nil, fileType, err
	}
	if probe, ok := p.(TryParser); ok {
		if actual := probe.TryParse(data); actual != "" && actual != utility.FileTypeOTHER && actual != fileType {
			p, err = newParser(actual, textOnly)
			return p, actual, err
		}
	}
	return p, fileType, nil
}

// parserFactories holds extra FileType→parser factory mappings registered by
// editions. Lookups consult this map before the built-in switch so an edition
// can add parsers for its own file types without the community switch changing.
var parserFactories = map[utility.FileType]func(textOnly bool) (ParseResultProducer, error){}

// parseMethodFactories holds parser overrides keyed by file type and method.
var parseMethodFactories = map[utility.FileType]map[string]func(textOnly bool) (ParseResultProducer, error){}

// RegisterParseMethodFactory registers a parser factory for a specific
// file type and method pair. Register factories during package initialization.
// Methods are matched case-insensitively.
func RegisterParseMethodFactory(ft utility.FileType, method string, factory func(textOnly bool) (ParseResultProducer, error)) {
	method = strings.ToLower(strings.TrimSpace(method))
	if ft == "" || method == "" || factory == nil {
		return
	}
	byMethod := parseMethodFactories[ft]
	if byMethod == nil {
		byMethod = map[string]func(textOnly bool) (ParseResultProducer, error){}
		parseMethodFactories[ft] = byMethod
	}
	byMethod[method] = factory
}

// HasParseMethod reports whether a FileType + parse_method override exists.
func HasParseMethod(ft utility.FileType, method string) bool {
	method = strings.ToLower(strings.TrimSpace(method))
	if method == "" {
		return false
	}
	_, ok := parseMethodFactories[ft][method]
	return ok
}

// GetParserWithMethod selects a registered method-specific parser. When the
// method is not registered it falls back to the ordinary FileType route, which
// owns validation/fallback for that family.
func GetParserWithMethod(ft utility.FileType, method string, data []byte, textOnly bool) (ParseResultProducer, utility.FileType, error) {
	method = strings.ToLower(strings.TrimSpace(method))
	if factory, ok := parseMethodFactories[ft][method]; ok {
		p, err := factory(textOnly)
		if err != nil {
			return nil, ft, err
		}
		return p, ft, nil
	}
	return selectParser(ft, data, textOnly)
}

// RegisterParserFactory registers a parser factory for a FileType not handled by
// the community switch. Enterprise editions call this from init() to inject
// parsers for their extra file types.
func RegisterParserFactory(ft utility.FileType, factory func(textOnly bool) (ParseResultProducer, error)) {
	if ft == "" {
		return
	}
	parserFactories[ft] = factory
}

// newParser owns the format routes for both ingestion and text extraction.
func newParser(fileType utility.FileType, textOnly bool) (ParseResultProducer, error) {
	if factory, ok := parserFactories[fileType]; ok {
		return factory(textOnly)
	}
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
		return NewXLSParser("")
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
		p, err := NewMarkdownParser(GoMarkdown)
		if err != nil {
			return nil, err
		}
		p.textOnly = textOnly
		p.mediaRefs = !textOnly
		return p, nil
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
