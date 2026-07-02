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
	"context"
	"fmt"
	"ragflow/internal/deepdoc/parser/pdf"
	i "ragflow/internal/deepdoc/parser/pdf/inference"
	t "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/parser/post"
)

type PDFParser struct {
	ParserType string // DeepDoc, PaddleOCR, MinerU
	Model      string // DeepDoc@buildin@ragflow
	LibType    string // pdf_oxide, used by DeepDoc
}

func NewPDFParser() *PDFParser {
	return &PDFParser{}
}

func (p *PDFParser) Parse(filename string, data []byte) error {
	return nil
}

func (p *PDFParser) String() string {
	return "PDFParser"
}

// ParseWithDeepDoc runs the full PDF parsing pipeline with explicit configuration.
// The underlying engine (pdfium/pdf_oxide) requires CGO; without CGO this returns an error.
func (p *PDFParser) ParseWithDeepDoc(ctx context.Context, filename string, data []byte, config t.ParserConfig) ([]map[string]any, error) {
	client, err := i.NewClient("http://localhost:9390")
	if err != nil {
		return nil, fmt.Errorf("inference client not available: http://localhost:9390 %w", err)
	}

	parser := pdf.NewParser(config)
	result, err := parser.Parse(ctx, data, client)
	if err != nil {
		return nil, fmt.Errorf("pdf parse file %s, error: %w", filename, err)
	}

	pipeCfg := buildPipelineConfig(make(map[string]any))
	if err := post.PostProcess(ctx, result, pipeCfg); err != nil {
		return nil, fmt.Errorf("pdf post-process: %w", err)
	}

	return sectionsToMaps(result.Sections), nil
}

// sectionsToMaps converts []t.Section to the output format []map[string]any,
// matching Python's bbox dict layout from parser.py:_pdf with output_format="json".
func sectionsToMaps(sections []t.Section) []map[string]any {
	if len(sections) == 0 {
		return nil
	}
	out := make([]map[string]any, len(sections))
	for idx, sec := range sections {
		m := map[string]any{
			"text":         sec.Text,
			"doc_type_kwd": sec.DocTypeKwd,
			"img_id":       "",
		}
		if sec.Image != "" {
			m["image"] = sec.Image // base64-encoded image data
		}
		if len(sec.Positions) > 0 {
			m["positions"] = sec.Positions
		}
		out[idx] = m
	}
	return out
}

// buildPipelineConfig converts parser_config values to post.PipelineConfig.
func buildPipelineConfig(parserConfig map[string]any) post.PipelineConfig {
	cfg := post.PipelineConfig{}
	if v, ok := parserConfig["page_width"]; ok {
		cfg[post.ConfigKeyPageWidth] = v
	}
	if v, ok := parserConfig["remove_toc"]; ok {
		cfg[post.ConfigKeyRemoveTOC] = v
	}
	if v, ok := parserConfig["flatten_media_to_text"]; ok {
		cfg[post.ConfigKeyFlattenMediaToText] = v
	}
	if v, ok := parserConfig["tenant_id"]; ok {
		cfg[post.ConfigKeyTenantID] = v
	}
	if v, ok := parserConfig["vlm_llm_id"]; ok {
		cfg[post.ConfigKeyVLMLLMID] = v
	}
	cfg[post.ConfigKeyZoom] = float64(3)
	return cfg
}
