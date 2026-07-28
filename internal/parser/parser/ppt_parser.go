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

import "context"

// PPTParser delegates to PPTXParser which handles both OLE binary
// (.ppt) and OOXML (.pptx) containers uniformly
type PPTParser struct {
	pptx PPTXParser
}

func NewPPTParser() *PPTParser {
	return &PPTParser{pptx: PPTXParser{format: "ppt"}}
}

func (p *PPTParser) String() string {
	return "PPTParser"
}

// ConfigureFromSetup forwards the slides-family setup map to the
// underlying PPTXParser so both .ppt and .pptx containers share
// the same TCADP configuration
func (p *PPTParser) ConfigureFromSetup(setup map[string]any) {
	p.pptx.ConfigureFromSetup(setup)
}

// ParseWithResult delegates to PPTXParser's structured output
// for the legacy PPT format using the "ppt" container format
// hint (OLE binary). The two file families differ only in the
// binary container; the python parser.py:slides branch treats
// them uniformly.
func (p *PPTParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	return p.pptx.ParseWithResult(ctx, filename, data)
}
