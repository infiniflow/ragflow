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

package chunker

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

const ComponentNameGeneralChunker = "GeneralChunker"

type generalChunkerParam struct {
	ChunkTokenSize     int
	Delimiters         []string
	OverlappedPercent  float64
	ChildrenDelimiters []string
	TableContextSize   int
	ImageContextSize   int
}

func (generalChunkerParam) Defaults() generalChunkerParam {
	return generalChunkerParam{
		ChunkTokenSize:     512,
		Delimiters:         []string{"\n"},
		ChildrenDelimiters: []string{},
	}
}

func (p *generalChunkerParam) Update(conf map[string]any) {
	if value, ok := schema.NumericFromAny(conf["chunk_token_size"]); ok {
		p.ChunkTokenSize = int(value)
	}
	if value, ok := conf["delimiters"].([]any); ok {
		p.Delimiters = stringListFromAny(value)
	} else if value, ok := conf["delimiters"].([]string); ok {
		p.Delimiters = append([]string(nil), value...)
	}
	if value, ok := conf["overlapped_percent"]; ok {
		p.OverlappedPercent = schema.NormalizeOverlappedPercent(value)
	}
	if value, ok := conf["children_delimiters"].([]any); ok {
		p.ChildrenDelimiters = stringListFromAny(value)
	} else if value, ok := conf["children_delimiters"].([]string); ok {
		p.ChildrenDelimiters = append([]string(nil), value...)
	}
	if value, ok := schema.NumericFromAny(conf["table_context_size"]); ok {
		p.TableContextSize = max(0, int(value))
	}
	if value, ok := schema.NumericFromAny(conf["image_context_size"]); ok {
		p.ImageContextSize = max(0, int(value))
	}
}

// GeneralChunkerComponent owns the general ingestion chunking policy and
// selects its format-specific strategy from the Parser's canonical file_type.
type GeneralChunkerComponent struct {
	param generalChunkerParam
}

func NewGeneralChunker(params map[string]any) (runtime.Component, error) {
	param := generalChunkerParam{}.Defaults()
	param.Update(params)
	return &GeneralChunkerComponent{param: param}, nil
}

func (c *GeneralChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *GeneralChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *GeneralChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return nil, fmt.Errorf("GeneralChunker: decode input: %w", err)
	}
	if strings.TrimSpace(upstream.FileType) == "" {
		return nil, fmt.Errorf("GeneralChunker: file_type is required")
	}

	switch generalStrategyForFileType(upstream.FileType) {
	case generalStrategyPDF:
		return c.chunkPDF(ctx, db, upstream)
	case generalStrategyDOCX:
		return c.chunkDOCX(ctx, upstream)
	case generalStrategyMarkdown:
		return c.chunkMarkdown(ctx, upstream)
	case generalStrategySpreadsheet:
		return c.chunkSpreadsheet(ctx, upstream)
	default:
		return c.chunkGeneral(ctx, upstream)
	}
}

type generalStrategy uint8

const (
	generalStrategyText generalStrategy = iota
	generalStrategyPDF
	generalStrategyDOCX
	generalStrategyMarkdown
	generalStrategySpreadsheet
)

func generalStrategyForFileType(fileType string) generalStrategy {
	switch fileType {
	case "pdf":
		return generalStrategyPDF
	case "docx":
		return generalStrategyDOCX
	case "md":
		return generalStrategyMarkdown
	case "xls", "xlsx", "csv":
		return generalStrategySpreadsheet
	default:
		return generalStrategyText
	}
}

func (c *GeneralChunkerComponent) chunkPDF(ctx context.Context, _ *gorm.DB, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkDOCX(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkMarkdown(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkSpreadsheet(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	return c.chunkGeneral(ctx, upstream)
}

func (c *GeneralChunkerComponent) chunkGeneral(ctx context.Context, upstream schema.ChunkerFromUpstream) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("GeneralChunker: %w", err)
	}
	units := upstream.JSONResult
	if upstream.OutputFormat == schema.PayloadFormatChunks {
		units = upstream.Chunks
	}
	if len(units) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(units), nil
}

func init() {
	MustRegisterChunker(ComponentNameGeneralChunker)
}
