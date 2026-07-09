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

import "fmt"

// stubParser is a placeholder for parsers whose real implementation lives
// in the DeepDoc inference service. The stub exists only to satisfy the
// factory dispatch contract.
type stubParser struct{ name string }

func (p *stubParser) ParseWithResult(filename string, data []byte) ParseResult {
	return ParseResult{Err: fmt.Errorf("%s: remote deepdoc dispatch not yet wired", p.name)}
}

// GetParserByID returns a parser for the given parser_id (matches Python FACTORY dispatch).
// Most parser_ids handle PDF files with different strategies; the strategy is encoded
// in the returned parser implementation. Stub implementations are replaced as
// colleagues deliver real parsers.
func GetParserByID(parserID string) (ParseResultProducer, error) {
	switch parserID {
	case "naive", "general":
		return NewNaivePDFParser(), nil
	case "paper":
		return NewPaperPDFParser(), nil
	case "book":
		return NewBookPDFParser(), nil
	case "presentation":
		return NewPresentationParser(), nil
	case "manual":
		return NewManualPDFParser(), nil
	case "laws":
		return NewLawsPDFParser(), nil
	case "qa":
		return NewQAPDFParser(), nil
	case "table":
		return NewTableParser(), nil
	case "resume":
		return NewResumePDFParser(), nil
	case "picture":
		return NewPicturePDFParser(), nil
	case "one":
		return NewOnePDFParser(), nil
	case "audio":
		return NewAudioParser(), nil
	case "email":
		return NewEmailParser(), nil
	case "tag":
		return NewTagPDFParser(), nil
	case "knowledge_graph":
		return NewKGPDFParser(), nil
	default:
		return nil, fmt.Errorf("unknown parser_id: %s", parserID)
	}
}

// Stub constructors for each parser type.
func NewNaivePDFParser() *stubParser    { return &stubParser{name: "naive"} }
func NewPaperPDFParser() *stubParser    { return &stubParser{name: "paper"} }
func NewBookPDFParser() *stubParser     { return &stubParser{name: "book"} }
func NewPresentationParser() *stubParser  { return &stubParser{name: "presentation"} }
func NewManualPDFParser() *stubParser   { return &stubParser{name: "manual"} }
func NewLawsPDFParser() *stubParser     { return &stubParser{name: "laws"} }
func NewQAPDFParser() *stubParser       { return &stubParser{name: "qa"} }
func NewTableParser() *stubParser       { return &stubParser{name: "table"} }
func NewResumePDFParser() *stubParser   { return &stubParser{name: "resume"} }
func NewPicturePDFParser() *stubParser  { return &stubParser{name: "picture"} }
func NewOnePDFParser() *stubParser      { return &stubParser{name: "one"} }
func NewAudioParser() *stubParser       { return &stubParser{name: "audio"} }
func NewEmailParser() *stubParser       { return &stubParser{name: "email"} }
func NewTagPDFParser() *stubParser      { return &stubParser{name: "tag"} }
func NewKGPDFParser() *stubParser       { return &stubParser{name: "knowledge_graph"} }
