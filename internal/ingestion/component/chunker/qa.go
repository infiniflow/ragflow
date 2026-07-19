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

// QAChunker extracts question-answer pairs from parsed content.
//
// Input formats and extraction strategies:
//   - Text (txt, csv)  → delimiter-based Q&A (comma or tab)
//   - Markdown (md)    → heading-based Q&A
//   - HTML (xlsx/xls)  → table-based Q&A (first two columns)
//   - JSON (pdf, docx) → delimiter-based on structured text sections
//
// Every Q&A pair becomes a single chunk with content_with_weight
// formatted as "Question: {q}\tAnswer: {a}".
package chunker

import (
	"context"
	"encoding/csv"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/parser"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const ComponentNameQAChunker = "QAChunker"

type qaChunkerParam struct {
	Lang string `json:"lang,omitempty"`
}

func (p *qaChunkerParam) Update(conf map[string]any) {
	if v, ok := conf["lang"]; ok {
		if s, ok := v.(string); ok {
			p.Lang = s
		}
	}
}

func (qaChunkerParam) Defaults() qaChunkerParam { return qaChunkerParam{} }

func (qaChunkerParam) Validate() error { return nil }

type QAChunkerComponent struct {
	name  string
	param qaChunkerParam
}

func NewQAChunker(params map[string]any) (runtime.Component, error) {
	p := qaChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &QAChunkerComponent{
		name:  ComponentNameQAChunker,
		param: p,
	}, nil
}
func (c *QAChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *QAChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *QAChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *QAChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
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

	qPrefix, aPrefix := "问题：", "回答："
	eng := strings.EqualFold(c.param.Lang, "english") || c.param.Lang == ""
	if eng {
		qPrefix, aPrefix = "Question: ", "Answer: "
	}

	var qaPairs []qaPair
	var isMarkdown bool
	switch upstream.OutputFormat {
	case schema.PayloadFormatHTML:
		qaPairs = extractQATable(stringPtrVal(upstream.HTMLResult))
	case schema.PayloadFormatMarkdown:
		qaPairs = extractQAMarkdown(stringPtrVal(upstream.MarkdownResult))
		isMarkdown = true
	case schema.PayloadFormatText:
		qaPairs = extractQAText(stringPtrVal(upstream.TextResult))
	default:
		qaPairs = extractQAJSON(upstream.JSONResult)
	}

	chunks := make([]schema.ChunkDoc, 0, len(qaPairs))
	for _, pair := range qaPairs {
		contentLTKS, _ := tokenizer.Tokenize(pair.Question)
		contentSMLTKS, _ := tokenizer.FineGrainedTokenize(contentLTKS)
		answer := rmQAPrefix(pair.Answer)
		if isMarkdown {
			answer = renderMarkdown(answer)
		}
		chunk := schema.ChunkDoc{
			ContentWithWeight: fmt.Sprintf("%s%s\t%s%s", qPrefix, rmQAPrefix(pair.Question), aPrefix, answer),
			DocType:           "text",
			ContentLtks:       contentLTKS,
			ContentSmLtks:     contentSMLTKS,
		}
		chunks = append(chunks, chunk)
	}

	return chunkOutputs(chunks), nil
}

func renderMarkdown(s string) string {
	mdParser := parser.NewWithExtensions(parser.CommonExtensions | parser.Tables)
	output := markdown.ToHTML([]byte(s), mdParser, nil)
	return string(output)
}

type qaPair struct {
	Question string
	Answer   string
}

var rmQAPrefixRe = regexp.MustCompile(`(?i)^(问题|答案|回答|user|assistant|Q|A|Question|Answer|问|答)[ \t]*(?:[:：]|\t)[ \t]*`)

func rmQAPrefix(txt string) string {
	return strings.TrimSpace(rmQAPrefixRe.ReplaceAllString(txt, ""))
}

func stringPtrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ---------------------------------------------------------------------------
// HTML / spreadsheet QA extraction
// ---------------------------------------------------------------------------

var htmlTR = regexp.MustCompile(`(?i)<tr[^>]*>(.*?)</tr>`)
var htmlTD = regexp.MustCompile(`(?i)<t[dh][^>]*>(.*?)</t[dh]>`)
var htmlTag = regexp.MustCompile(`<[^>]+>`)

func extractQATable(htmlStr string) []qaPair {
	if htmlStr == "" {
		return nil
	}
	rows := htmlTR.FindAllStringSubmatch(htmlStr, -1)
	pairs := make([]qaPair, 0, len(rows))
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
			pairs = append(pairs, qaPair{Question: texts[0], Answer: texts[1]})
		}
	}
	return pairs
}

// ---------------------------------------------------------------------------
// Markdown QA extraction
// ---------------------------------------------------------------------------

var mdHeading = regexp.MustCompile(`^(#*)`)

func extractQAMarkdown(md string) []qaPair {
	if md == "" {
		return nil
	}
	lines := strings.Split(md, "\n")
	var pairs []qaPair
	var questionStack []string
	var levelStack []int
	var answer []string
	codeBlock := false

	flushAnswer := func() {
		joined := strings.TrimSpace(strings.Join(answer, "\n"))
		if joined != "" && len(questionStack) > 0 {
			sumQ := strings.Join(questionStack, "\n")
			pairs = append(pairs, qaPair{Question: sumQ, Answer: joined})
		}
		answer = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			codeBlock = !codeBlock
		}
		if codeBlock {
			answer = append(answer, line)
			continue
		}

		m := mdHeading.FindStringSubmatch(line)
		level := len(m[1])
		if level == 0 || level > 6 {
			answer = append(answer, line)
			continue
		}

		flushAnswer()
		question := strings.TrimSpace(line[level:])

		for len(levelStack) > 0 && level <= levelStack[len(levelStack)-1] {
			questionStack = questionStack[:len(questionStack)-1]
			levelStack = levelStack[:len(levelStack)-1]
		}
		questionStack = append(questionStack, question)
		levelStack = append(levelStack, level)
	}
	flushAnswer()
	return pairs
}

// ---------------------------------------------------------------------------
// Text / delimiter-based QA extraction (txt, csv)
// ---------------------------------------------------------------------------

func extractQAText(text string) []qaPair {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")

	delimiter := detectDelimiter(lines)

	var pairs []qaPair
	var question, answer string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := splitQA(line, delimiter)
		if len(parts) != 2 {
			if question != "" {
				answer += "\n" + line
			}
			continue
		}
		if question != "" && answer != "" {
			pairs = append(pairs, qaPair{Question: strings.TrimSpace(question), Answer: strings.TrimSpace(answer)})
		}
		question = parts[0]
		answer = parts[1]
	}
	if question != "" {
		pairs = append(pairs, qaPair{Question: strings.TrimSpace(question), Answer: strings.TrimSpace(answer)})
	}
	return pairs
}

func detectDelimiter(lines []string) string {
	comma, tab := 0, 0
	for _, line := range lines {
		if len(strings.Split(line, ",")) == 2 {
			comma++
		}
		if len(strings.Split(line, "\t")) == 2 {
			tab++
		}
	}
	if tab >= comma {
		return "\t"
	}
	return ","
}

func splitQA(line, delimiter string) []string {
	if delimiter == "\t" {
		parts := strings.Split(line, "\t")
		if len(parts) == 2 {
			return parts
		}
		return []string{line}
	}
	r := csv.NewReader(strings.NewReader(line))
	r.Comma = ','
	r.LazyQuotes = true
	records, err := r.Read()
	if err != nil || len(records) != 2 {
		return []string{line}
	}
	return records
}

// ---------------------------------------------------------------------------
// JSON / structured QA extraction
// ---------------------------------------------------------------------------

func extractQAJSON(items []schema.ChunkDoc) []qaPair {
	var pairs []qaPair
	for _, item := range items {
		txt, _ := itemText(item)
		if txt == "" {
			continue
		}
		tmp := extractQAText(txt)
		pairs = append(pairs, tmp...)
	}
	return pairs
}

func init() {
	MustRegisterChunker(ComponentNameQAChunker)
}
