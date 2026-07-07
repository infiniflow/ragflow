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
)

type PPTParser struct {
	libType string
}

func NewPPTParser(libType string) (*PPTParser, error) {
	switch libType {
	case OfficeOxide:
		return &PPTParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported PPT library type: %s", libType)
	}
}

func (p *PPTParser) String() string {
	return "PPTParser"
}

// ParseWithResult delegates to PPTXParser's structured output
// for the legacy PPT format. The two file families differ only
// in the binary container; the python parser.py:slides branch
// treats them uniformly.
func (p *PPTParser) ParseWithResult(filename string, data []byte) ParseResult {
	delegate, err := NewPPTXParser(OfficeOxide)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("ppt delegate: %w", err)}
	}
	res := delegate.ParseWithResult(filename, data)
	if res.File != nil {
		res.File["format"] = "ppt"
	}
	return res
}
