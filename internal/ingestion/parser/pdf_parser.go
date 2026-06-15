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

type PDFParser struct {
	ParserType string // DeepDoc, PaddleOCR, MinerU
	Model      string // DeepDoc@buildin@ragflow
	LibType    string // pdf_oxide, used by DeepDoc
}

func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

func (p *PDFParser) Parse(filename string, data []byte) error {
	fmt.Printf("Parsing PDF file: %s\n", filename)
	return nil
}

func (p *PDFParser) String() string {
	return "PDFParser"
}
