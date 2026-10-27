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

// PageChunker emits one chunk per upstream slide or page.
// It is the faithful Go port of the Python `presentation` chunk method
// (rag/app/presentation.py), whose docstring states: "Every page will
// be treated as a chunk."
//
// Unlike TokenChunker (which merges slides into a single chunk) or
// OneChunker (which collapses many slides into one), PageChunker
// keeps each slide as the unit of chunking. Text and tables from the same
// slide are combined in order; images remain separate media chunks.
package chunker

import (
	"context"
	"encoding/json"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"

	"gorm.io/gorm"
)

const ComponentNamePageChunker = "PageChunker"

type pageChunkerParam struct{}

func (p *pageChunkerParam) Update(conf map[string]any) {}

func (pageChunkerParam) Defaults() pageChunkerParam {
	return pageChunkerParam{}
}

func (pageChunkerParam) Validate() error { return nil }

type PageChunkerComponent struct {
	name  string
	param pageChunkerParam
}

func NewPageChunker(params map[string]any) (runtime.Component, error) {
	p := pageChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &PageChunkerComponent{
		name:  ComponentNamePageChunker,
		param: p,
	}, nil
}
func (c *PageChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *PageChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *PageChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

// invoke groups slide content and preserves other upstream items.
func (c *PageChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return inputErrorOutput(err), nil
	}

	items := slideItems(upstream.JSONResult, upstream.Chunks)
	if len(items) == 0 {
		return emptyOutputs(), nil
	}
	return chunkOutputs(items), nil
}

// slideItems combines consecutive items with the same slide number, while
// preserving unnumbered items and slide images as independent chunks.
func slideItems(items, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	if len(items) == 0 {
		items = chunks
	}
	var result []schema.ChunkDoc
	for i := 0; i < len(items); {
		var slide int
		if json.Unmarshal(items[i].Extra["slide_number"], &slide) != nil || slide <= 0 {
			result = append(result, items[i])
			i++
			continue
		}
		end := i + 1
		for end < len(items) {
			var next int
			if json.Unmarshal(items[end].Extra["slide_number"], &next) != nil || next != slide {
				break
			}
			end++
		}
		var page *schema.ChunkDoc
		merged := false
		var images []schema.ChunkDoc
		var tableIDs []string
		for _, item := range items[i:end] {
			if itemDocType(item) == "image" || item.Image != "" || item.ImgID != "" {
				images = append(images, item)
				continue
			}
			if item.TableData != nil {
				var id string
				if json.Unmarshal(item.Extra["source_table_id"], &id) == nil && id != "" {
					tableIDs = append(tableIDs, id)
				}
			}
			if page == nil {
				first := cloneChunkDoc(item)
				page = &first
			} else {
				merged = true
				if page.Text != "" && item.Text != "" && !strings.HasSuffix(page.Text, "\n") && !strings.HasPrefix(item.Text, "\n") {
					page.Text += "\n"
				}
				page.Text += item.Text
				mergeGeneralMetadata(page, item)
			}
		}
		if page != nil {
			if merged {
				page.DocType = "text"
				page.TableData = nil
				page.TKNums = nil
			}
			if len(tableIDs) > 1 {
				encoded, _ := json.Marshal(tableIDs)
				page.Extra["source_table_ids"] = encoded
			}
			result = append(result, *page)
		}
		result = append(result, images...)
		i = end
	}
	return result
}

func init() {
	MustRegisterChunker(ComponentNamePageChunker)
}
