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

// TagChunker ports the Python `tag` chunk method (rag/app/tag.py).
//
// The Python chunk() reads an Excel/csv/txt file with two logical
// columns — content and tags (no header) — and emits ONE chunk per
// row, each carrying:
//
//	content_with_weight = content          (verbatim)
//	tag_kwd             = [t for t in tags.split(",") if t]  (".\" -> "_")
//
// The Go ingestion pipeline feeds the chunker a parsed payload rather
// than the raw file, so TagChunker mirrors the sibling QAChunker and
// extracts (content, tags) pairs from every accepted upstream format:
//
//   - Text / Markdown / CSV-style txt → delimiter-based pairing
//     (tab wins over comma, matching tag.py's detectDelimiter).
//   - HTML (spreadsheet → table)         → first two <td> columns.
//   - JSON (parsed doc items)            → per-item text pairing.
//
// Every pair becomes one chunk with content_with_weight = content and
// tag_kwd = the comma-split, dot-sanitised tag list.
package chunker

import (
	"context"
	"fmt"
	"html"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const ComponentNameTagChunker = "TagChunker"

type tagChunkerParam struct{}

func (p *tagChunkerParam) Update(conf map[string]any) {}

func (tagChunkerParam) Defaults() tagChunkerParam { return tagChunkerParam{} }

func (tagChunkerParam) Validate() error { return nil }

type TagChunkerComponent struct {
	name  string
	param tagChunkerParam
}

func NewTagChunker(params map[string]any) (runtime.Component, error) {
	p := tagChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &TagChunkerComponent{
		name:  ComponentNameTagChunker,
		param: p,
	}, nil
}
func (c *TagChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *TagChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *TagChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *TagChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        fmt.Sprintf("Input error: %v", err),
		}, nil
	}

	var pairs []tagPair
	switch upstream.OutputFormat {
	case schema.PayloadFormatHTML:
		pairs = extractTagTable(stringPtrVal(upstream.HTMLResult))
	case schema.PayloadFormatMarkdown:
		pairs = extractTagText(stringPtrVal(upstream.MarkdownResult))
	case schema.PayloadFormatText:
		pairs = extractTagText(stringPtrVal(upstream.TextResult))
	default:
		pairs = extractTagJSON(upstream.JSONResult)
	}

	chunks := make([]schema.ChunkDoc, 0, len(pairs))
	for _, pair := range pairs {
		content := strings.TrimSpace(pair.Content)
		if content == "" {
			continue
		}
		contentLTKS, _ := tokenizer.Tokenize(content)
		contentSMLTKS, _ := tokenizer.FineGrainedTokenize(contentLTKS)
		chunk := schema.ChunkDoc{
			ContentWithWeight: content,
			DocType:           "text",
			ContentLtks:       contentLTKS,
			ContentSmLtks:     contentSMLTKS,
			TagKwd:            splitTagKwd(pair.Tags),
		}
		chunks = append(chunks, chunk)
	}

	return chunkOutputs(chunks), nil
}

// tagPair is a (content, tags) row extracted from the upstream payload.
type tagPair struct {
	Content string
	Tags    string
}

// extractTagText ports tag.py:60-89 (txt) and tag.py:91-113 (csv):
// every deformed line is accumulated into the running content; a line that
// splits into exactly two fields is emitted as a (content+prefix, tags)
// pair and the accumulator is reset. Delimiter selection (tab vs comma)
// reuses the same detectDelimiter/splitQA helpers as QAChunker.
func extractTagText(text string) []tagPair {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	delimiter := detectDelimiter(lines)

	var pairs []tagPair
	content := ""
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := splitQA(line, delimiter)
		if len(parts) != 2 {
			content += "\n" + line
			continue
		}
		content += "\n" + parts[0]
		pairs = append(pairs, tagPair{Content: content, Tags: parts[1]})
		content = ""
	}
	return pairs
}

// extractTagTable ports the spreadsheet (xlsx/csv → html table) path.
// Python tag.py reads these via the Excel parser's (q, a) pairs, which
// are exactly the first two columns of each row — so we take the same
// two leading cells per <tr>.
func extractTagTable(htmlStr string) []tagPair {
	if htmlStr == "" {
		return nil
	}
	rows := htmlTR.FindAllStringSubmatch(htmlStr, -1)
	pairs := make([]tagPair, 0, len(rows))
	for _, row := range rows {
		cells := htmlTD.FindAllStringSubmatch(row[1], -1)
		var texts []string
		for _, cell := range cells {
			t := html.UnescapeString(htmlTag.ReplaceAllString(cell[1], ""))
			t = strings.TrimSpace(t)
			if t != "" {
				texts = append(texts, t)
			}
		}
		if len(texts) >= 2 {
			pairs = append(pairs, tagPair{Content: texts[0], Tags: texts[1]})
		}
	}
	return pairs
}

// extractTagJSON ports the JSON/structured upstream path: each item's
// text is run through the same delimiter pairing as the text path.
func extractTagJSON(items []schema.ChunkDoc) []tagPair {
	var pairs []tagPair
	for _, item := range items {
		txt, _ := itemText(item)
		if txt == "" {
			continue
		}
		pairs = append(pairs, extractTagText(txt)...)
	}
	return pairs
}

// splitTagKwd ports tag.py:beAdoc's tag_kwd construction: split on
// comma, drop empties, and replace "." with "_" (the storage layer
// rejects dots in keyword values).
func splitTagKwd(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.ReplaceAll(p, ".", "_")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	MustRegisterChunker(ComponentNameTagChunker)
}
