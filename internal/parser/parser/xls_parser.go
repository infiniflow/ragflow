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
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type XLSParser struct {
	libType                        string
	ParseMethod                    string
	OutputFormat                   string
	ChunkRows                      int
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
}

func NewXLSParser(libType string) (*XLSParser, error) {
	if libType == "" {
		libType = "excelize"
	}
	return &XLSParser{
		libType:                        libType,
		ChunkRows:                      defaultTableChunkRows,
		TCADPTableResultType:           "1",
		TCADPMarkdownImageResponseType: "1",
	}, nil
}

func (p *XLSParser) String() string {
	return "XLSParser"
}

func (p *XLSParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["parse_method"].(string); ok && v != "" {
		p.ParseMethod = v
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	p.ChunkRows = decodeChunkRows(setup)
	if v, ok := setup["tcadp_apiserver"].(string); ok && v != "" {
		p.TCADPAPIServer = v
	}
	if v, ok := setup["tcadp_api_key"].(string); ok {
		p.TCADPAPIKey = v
	}
	if v, ok := setup["table_result_type"].(string); ok && v != "" {
		p.TCADPTableResultType = v
	}
	if v, ok := setup["markdown_image_response_type"].(string); ok && v != "" {
		p.TCADPMarkdownImageResponseType = v
	}
}

func (p *XLSParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	method := normalizeXLSXParseMethod(p.ParseMethod)
	switch method {
	case "tcadp":
		return parseWithTCADP(
			ctx, filename, data, "XLS",
			p.TCADPAPIServer, p.TCADPAPIKey,
			p.TCADPTableResultType, p.TCADPMarkdownImageResponseType,
			p.OutputFormat,
		)
	case "", "excelize":
		// Continue with the local Excelize parser.
	default:
		return ParseResult{
			Err: fmt.Errorf("unsupported XLS parse method: %q", p.ParseMethod),
		}
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("xls open: %w", err)}
	}
	defer f.Close()

	sheets := f.GetSheetList()
	chunkRows := p.ChunkRows
	if chunkRows <= 0 {
		chunkRows = defaultTableChunkRows
	}

	var html strings.Builder
	var warnings []string
	for _, sheet := range sheets {
		rendered, sheetWarnings, err := renderSheetTables(f, sheet, chunkRows)
		if err != nil {
			return ParseResult{Err: fmt.Errorf("xls parse: %w", err)}
		}
		html.WriteString(rendered)
		warnings = append(warnings, sheetWarnings...)
	}

	return ParseResult{
		OutputFormat: "html",
		File:         map[string]any{"name": filename, "format": "xls"},
		HTML:         html.String(),
		Warnings:     warnings,
	}
}
