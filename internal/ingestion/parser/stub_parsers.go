//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package parser

// stubPDFParser is a placeholder for PDF-based parsers whose real
// implementations are being built by colleagues. It satisfies the
// FileParser interface so GetParserByID can return a non-nil parser
// for every valid parser_id, unblocking TaskHandler dispatch.
type stubPDFParser struct{ name string }

func (p *stubPDFParser) Parse(filename string, data []byte) error { return nil }
func (p *stubPDFParser) String() string                            { return p.name }

func NewNaivePDFParser() FileParser    { return &stubPDFParser{"NaivePDFParser"} }
func NewPaperPDFParser() FileParser    { return &stubPDFParser{"PaperPDFParser"} }
func NewBookPDFParser() FileParser     { return &stubPDFParser{"BookPDFParser"} }
func NewManualPDFParser() FileParser   { return &stubPDFParser{"ManualPDFParser"} }
func NewLawsPDFParser() FileParser     { return &stubPDFParser{"LawsPDFParser"} }
func NewQAPDFParser() FileParser       { return &stubPDFParser{"QAPDFParser"} }
func NewResumePDFParser() FileParser   { return &stubPDFParser{"ResumePDFParser"} }
func NewPicturePDFParser() FileParser  { return &stubPDFParser{"PicturePDFParser"} }
func NewOnePDFParser() FileParser      { return &stubPDFParser{"OnePDFParser"} }
func NewTagPDFParser() FileParser      { return &stubPDFParser{"TagPDFParser"} }
func NewKGPDFParser() FileParser       { return &stubPDFParser{"KGPDFParser"} }
func NewAudioParser() FileParser       { return &stubPDFParser{"AudioParser"} }
func NewEmailParser() FileParser       { return &stubPDFParser{"EmailParser"} }
func NewPresentationParser() FileParser { return &stubPDFParser{"PresentationParser"} }
func NewTableParser() FileParser       { return &stubPDFParser{"TableParser"} }
