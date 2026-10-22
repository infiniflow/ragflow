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

package component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

// TOC extraction builds one table of contents per document. The retrieval
// path reads it in internal/service/toc_enhancer.go.
const (
	tocBatchTokenCap      = 1024
	tocInputUtilization   = 0.5
	tocMaxTitles          = 512
	tocTitleMaxWords      = 22
	tocTitleMaxWordsPrune = 12
	tocJSONAttempts       = 2
	// tocPageNum sorts the TOC chunk after every content chunk.
	tocPageNum = 100000000
	// tocChunkIDSeed derives the TOC chunk id from the document id alone, so
	// a retried run overwrites the TOC row instead of adding a second one. The
	// NUL byte keeps the seed apart from the text of any real chunk.
	tocChunkIDSeed = "\x00toc"
)

var tocNumberOnlyTitleRE = regexp.MustCompile(`^[0-9,.()/ -]+$`)

// tocEntry is one stored TOC item. The stored level is a numeric string.
type tocEntry struct {
	Level string   `json:"level"`
	Title string   `json:"title"`
	IDs   []string `json:"ids,omitempty"`
}

// tocTitle is a heading the model found, with the chunk index it reported.
type tocTitle struct {
	title   string
	chunkID any
}

// buildTOCChunk returns the TOC chunk for the document, or nil when the
// model finds no headings.
func (c *ExtractorComponent) buildTOCChunk(ctx context.Context, db *gorm.DB, in extractorInputs, docID string, w progress.Window) (map[string]any, error) {
	texts := make([]string, len(in.chunks))
	for i, ck := range in.chunks {
		texts[i] = extractorChunkText(ck)
	}
	entries, err := c.runTOCFromText(ctx, db, in, texts, w)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entries); err != nil {
		return nil, err
	}
	return map[string]any{
		"text":          strings.TrimSuffix(buf.String(), "\n"),
		"id":            common.ChunkID(docID, tocChunkIDSeed),
		"toc_kwd":       "toc",
		"available_int": 0,
		"page_num_int":  []int{tocPageNum},
	}, nil
}

// runTOCFromText finds the headings in batches of chunks, then asks the model
// for the level of each heading. Each entry covers the chunks from its own
// chunk up to the chunk of the next entry. The last entry runs to the last
// chunk.
func (c *ExtractorComponent) runTOCFromText(ctx context.Context, db *gorm.DB, in extractorInputs, texts []string, w progress.Window) ([]tocEntry, error) {
	loader := prompts.EmbeddedPromptLoader{}
	systemPrompt, err := loader.Load("toc_from_text_system")
	if err != nil {
		return nil, err
	}
	userTemplate, err := loader.Load("toc_from_text_user")
	if err != nil {
		return nil, err
	}
	levelsPrompt, err := loader.Load("assign_toc_levels")
	if err != nil {
		return nil, err
	}

	budget := tocBatchTokenCap
	if ctxLen := extractorContextLength(ctx, db, in.llmID); ctxLen > 0 {
		budget = min(int(float64(ctxLen)*tocInputUtilization)-tokenizer.NumTokensFromString(userTemplate+systemPrompt), tocBatchTokenCap)
		if budget <= 0 {
			return nil, fmt.Errorf("model context of %d tokens leaves no room for chunk text", ctxLen)
		}
	}
	batches, err := tocBatchPrompts(texts, budget)
	if err != nil {
		return nil, err
	}

	temp := 0.0
	tin := extractorInputs{llmID: in.llmID, modelID: in.modelID, temperature: &temp}
	results := make([][]any, len(batches))
	var skipped atomic.Int64
	futs := make([]utility.WorkerPoolFuture[extractorJob, struct{}], 0, len(batches))
	for i, batch := range batches {
		i := i
		userPrompt := strings.Replace(userTemplate, "{{text}}", batch, 1)
		fn := func() error {
			parsed, err := c.genTOCJSON(ctx, db, tin, systemPrompt, userPrompt)
			if err != nil {
				return resolveChunkCallError(ctx, i, "toc", fmt.Errorf("batch %d: %w", i, err), false, &skipped)
			}
			results[i], _ = parsed.([]any)
			return nil
		}
		f, err := extractorPool.Submit(ctx, fn)
		if err != nil {
			return nil, err
		}
		futs = append(futs, f)
	}
	if err := awaitFutures(ctx, futs, w); err != nil {
		return nil, err
	}
	if err := checkAllChunksSkipped("toc", len(futs), skipped.Load()); err != nil {
		return nil, err
	}

	var raw []any
	for _, r := range results {
		raw = append(raw, r...)
	}
	titles, err := filterTOCTitles(raw, tokenizer.New(in.lang))
	if err != nil {
		return nil, err
	}
	if len(titles) == 0 {
		return nil, nil
	}

	names := make([]string, len(titles))
	for i, t := range titles {
		names[i] = t.title
	}
	namesJSON, err := json.Marshal(names)
	if err != nil {
		return nil, err
	}
	parsed, err := c.genTOCJSON(ctx, db, tin, levelsPrompt, string(namesJSON))
	if err != nil {
		return nil, err
	}
	levels, _ := parsed.([]any)
	return mergeTOCLevels(levels, titles, in.chunks), nil
}

// tocBatchPrompts packs chunks into batches of about budget tokens, one
// {"<index>": text} JSON line per chunk. A chunk is never split, even when it
// alone exceeds the budget.
func tocBatchPrompts(texts []string, budget int) ([]string, error) {
	var batches []string
	var lines []string
	tokens := 0
	for i, text := range texts {
		t := tokenizer.NumTokensFromString(text)
		if len(lines) > 0 && tokens+t > budget {
			batches = append(batches, strings.Join(lines, "\n"))
			lines, tokens = nil, 0
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(map[string]string{strconv.Itoa(i): text}); err != nil {
			return nil, err
		}
		lines = append(lines, strings.TrimSuffix(buf.String(), "\n"))
		tokens += t
	}
	if len(lines) > 0 {
		batches = append(batches, strings.Join(lines, "\n"))
	}
	return batches, nil
}

// genTOCJSON parses the model reply as JSON. When the reply is not valid JSON,
// it asks once more with the reply and the parse error appended. It returns
// nil when no attempt parses.
func (c *ExtractorComponent) genTOCJSON(ctx context.Context, db *gorm.DB, in extractorInputs, systemPrompt, userPrompt string) (any, error) {
	var errs string
	for attempt := 0; attempt < tocJSONAttempts; attempt++ {
		ans, err := c.callText(ctx, db, in, systemPrompt, userPrompt)
		if err != nil {
			return nil, err
		}
		repaired, err := kccommon.RepairJSONText(ans)
		if err == nil {
			var out any
			if err = json.Unmarshal([]byte(repaired), &out); err == nil {
				return out, nil
			}
		}
		errs += err.Error()
		userPrompt += "\nGenerated JSON is as following:\n" + ans + "\nBut exception while loading:\n" + errs + "\nPlease reconsider and correct it."
	}
	common.Warn("extractor: toc: model reply is not valid JSON; ignoring it",
		zap.String("component", componentNameExtractor),
		zap.String("error", errs),
	)
	return nil, nil
}

// filterTOCTitles drops empty titles, long sentences and number-only titles
// from the model output. The number-only rule also drops the "-1" placeholder
// the prompt asks for on chunks with no heading.
func filterTOCTitles(raw []any, tok tokenizer.Tokenizer) ([]tocTitle, error) {
	maxWords := tocTitleMaxWords
	if len(raw) > tocMaxTitles {
		maxWords = tocTitleMaxWordsPrune
	}
	var out []tocTitle
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title, _ := m["title"].(string)
		if title == "" {
			continue
		}
		tks, err := tok.Tokenize(title)
		if err != nil {
			return nil, fmt.Errorf("tokenize title: %w", err)
		}
		if len(strings.Split(tks, " ")) > maxWords {
			continue
		}
		if tocNumberOnlyTitleRE.MatchString(title) {
			continue
		}
		out = append(out, tocTitle{title: title, chunkID: m["chunk_id"]})
	}
	return out, nil
}

// mergeTOCLevels pairs each level item with the title at the same index and
// maps the reported chunk index to chunk ids. An item with an unusable level
// is dropped, because the TOC reader rejects the whole TOC on a non-numeric
// level.
func mergeTOCLevels(levels []any, titles []tocTitle, chunks []map[string]any) []tocEntry {
	type item struct {
		level int
		title string
		src   tocTitle
	}
	var items []item
	for i := 0; i < len(levels) && i < len(titles); i++ {
		var levelRaw, title any
		switch v := levels[i].(type) {
		case map[string]any:
			levelRaw, title = v["level"], v["title"]
		case []any:
			if len(v) < 2 {
				continue
			}
			levelRaw, title = v[0], v[1]
		default:
			continue
		}
		level := 0
		if levelRaw != nil {
			n, ok := tocInt(levelRaw)
			if !ok {
				continue
			}
			level = n
		}
		titleStr, ok := title.(string)
		if !ok {
			titleStr = titles[i].title
		}
		items = append(items, item{level: level, title: titleStr, src: titles[i]})
	}

	maxLevel := math.MinInt
	for _, it := range items {
		maxLevel = max(maxLevel, it.level)
	}
	prune := len(items) > tocMaxTitles
	var kept []item
	for _, it := range items {
		if prune && it.level >= maxLevel {
			continue
		}
		kept = append(kept, it)
	}

	entries := make([]tocEntry, len(kept))
	for i, it := range kept {
		entries[i] = tocEntry{Level: strconv.Itoa(it.level), Title: it.title}
		idx, ok := tocInt(it.src.chunkID)
		if !ok || idx < 0 || idx >= len(chunks) {
			continue
		}
		entries[i].IDs = appendChunkID(nil, chunks[idx])
		end := len(chunks) - 1
		if i < len(kept)-1 {
			next, ok := tocInt(kept[i+1].src.chunkID)
			if !ok {
				continue
			}
			end = next
		}
		for j := idx + 1; j <= end && j < len(chunks); j++ {
			entries[i].IDs = appendChunkID(entries[i].IDs, chunks[j])
		}
	}
	return entries
}

func appendChunkID(ids []string, ck map[string]any) []string {
	if id := chunkCacheID(ck); id != "" {
		ids = append(ids, id)
	}
	return ids
}

// tocInt reads an integer the model wrote as a JSON number or a string.
func tocInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, false
		}
		return int(n), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	default:
		return 0, false
	}
}
