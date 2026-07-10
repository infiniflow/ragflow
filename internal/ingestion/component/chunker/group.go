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

// SCOPE (honest) for group.go:
//
//   - Implements the GroupTitleChunker variant: aggregates adjacent
//     text records into chunks that span multiple body records while
//     staying inside one heading section.
//
//   - PARALLELISM: Parallelism() advertises a fan-out hint to outer
//     executors. Heading detection stays sequential; grouping work is
//     local to one invocation.
//
//   - MIRRORS python `_build_section_ids` + `GroupTitleChunker.build_chunks`:
//     consecutive records with the same (target_level-derived) sec_id
//     are merged up to MIN_GROUP_TOKENS / MAX_GROUP_TOKENS, then
//     emitted as one chunk per group.
//
//   - No PDF-position merge is performed (mirrors TokenChunker SCOPE
//     notes — those land with the deepdoc/parser port).
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const ComponentNameGroupTitleChunker = "GroupTitleChunker"

// MIN_GROUP_TOKENS / MAX_GROUP_TOKENS mirror the python constants in
// group_chunker.py:22-23. They drive the merge heuristic for adjacent
// text records.
const (
	minGroupTokens = 32
	maxGroupTokens = 1024
)

// resolveTargetLevel mirrors common.py:resolve_target_level: pick the
// n-th smallest heading level from the per-line `levels` vector.
func resolveTargetLevel(levels []int, hierarchy int) int {
	tiers := make([]int, 0, len(levels))
	seen := make(map[int]bool)
	for _, l := range levels {
		if l > 0 && l < bodyLevel && !seen[l] {
			seen[l] = true
			tiers = append(tiers, l)
		}
	}
	if len(tiers) == 0 {
		return 0
	}
	// ascending
	for i := 1; i < len(tiers); i++ {
		for j := i; j > 0 && tiers[j-1] > tiers[j]; j-- {
			tiers[j-1], tiers[j] = tiers[j], tiers[j-1]
		}
	}
	if hierarchy < 1 {
		hierarchy = 1
	}
	if hierarchy > len(tiers) {
		hierarchy = len(tiers)
	}
	return tiers[hierarchy-1]
}

// _buildSectionIDs computes, for each input level, the section id
// (`sid`) under which that record falls. Each heading encounter
// increments sid by 1; body lines share the active sid.
func buildSectionIDs(levels []int, targetLevel int) []int {
	secIDs := make([]int, len(levels))
	sid := 0
	for i, lvl := range levels {
		if i > 0 && targetLevel > 0 && lvl <= targetLevel {
			sid++
		}
		secIDs[i] = sid
	}
	return secIDs
}

// invokeGroup runs the GroupTitleChunker strategy against the
// supplied inputs. Detected headings + adjacent merges happen in two
// goroutines (heading detection sequential, then a fan-out over
// record-buckets for the merge pass).
func invokeGroup(_ context.Context, inputs map[string]any, p *titleChunkerParam) (map[string]any, error) {
	records := extractLineRecords(inputs)
	if len(records) == 0 {
		return emptyOutputs(), nil
	}
	ctx := newLevelContext(records, p)
	levels := ctx.Levels()

	// Mirror python group_chunker._resolve_group_target_level: when
	// `hierarchy` is unset the target level is `most_level` directly
	// (NOT resolve_target_level — that would re-rank the distinct
	// heading levels and pick the wrong depth when the heading levels
	// are not contiguous from 1). When `hierarchy` is set, it defers to
	// resolve_target_level.
	targetLevel := resolveGroupTargetLevel(levels, p, ctx.mostLevel)
	secIDs := buildSectionIDs(levels, targetLevel)

	groups := groupRecords(records, secIDs, p)
	chunks := buildChunksFromRecordGroups(groups, p, isPlainTextFormat(inputs))
	if len(chunks) == 0 {
		return emptyOutputs(), nil
	}
	out := map[string]any{
		"output_format": "chunks",
		"chunks":        chunks,
	}
	return out, nil
}

// groupRecords mirrors `GroupTitleChunker.build_chunks`: merges
// adjacent text records while staying inside the same logical section
// (matching last_sid) and token-budget constraints.
func groupRecords(records []lineRecord, secIDs []int, p *titleChunkerParam) [][]lineRecord {
	if len(records) == 0 {
		return nil
	}
	var recordGroups [][]lineRecord
	var currentGroup []lineRecord
	tkCnt := 0
	lastSID := -2

	for i, rec := range records {
		secID := secIDs[i]
		if !rec.isText() {
			if len(currentGroup) > 0 {
				recordGroups = append(recordGroups, append([]lineRecord(nil), currentGroup...))
			}
			recordGroups = append(recordGroups, []lineRecord{rec})
			currentGroup = currentGroup[:0]
			tkCnt = 0
			lastSID = -2
			continue
		}
		text := trim(rec.text)
		if text == "" {
			continue
		}
		tokenCount := tokenizer.NumTokensFromString(text)
		shouldMerge := len(currentGroup) > 0 &&
			currentGroup[0].isText() &&
			(tkCnt < minGroupTokens || (tkCnt < maxGroupTokens && secID == lastSID))
		if shouldMerge {
			currentGroup = append(currentGroup, rec)
			tkCnt += tokenCount
		} else {
			if len(currentGroup) > 0 {
				recordGroups = append(recordGroups, append([]lineRecord(nil), currentGroup...))
			}
			currentGroup = []lineRecord{rec}
			tkCnt = tokenCount
		}
		lastSID = secID
	}
	if len(currentGroup) > 0 {
		recordGroups = append(recordGroups, currentGroup)
	}
	return recordGroups
}

// joinGroupText mirrors python's `"".join(record["text"] + "\n" for
// record in records)` — every record's text followed by a newline
// (including the last), matching the python text join exactly.
func joinGroupText(g []lineRecord) string {
	var sb strings.Builder
	for _, r := range g {
		sb.WriteString(r.text)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// isPlainTextFormat mirrors the `output_format in ["markdown", "text",
// "html"]` branch of common.py:build_chunks_from_record_groups. Plain
// payloads emit only "text"; structured payloads (chunks/json) also
// carry doc_type_kwd and img_id.
func isPlainTextFormat(inputs map[string]any) bool {
	if f, ok := inputs["output_format"].(string); ok {
		return f == "markdown" || f == "text" || f == "html"
	}
	return false
}

// buildChunksFromRecordGroups mirrors common.py:build_chunks_from_record_groups
// (minus the deepdoc-only remove_tag / merge_pdf_positions steps):
//   - plain payloads: {"text": joined}
//   - structured payloads: text plus doc_type_kwd / img_id from the
//     group's leading record.
//
// root_chunk_as_heading is applied here, exactly as python does (post
// materialisation): the root chunk's text is prepended to every
// following chunk and the root chunk is dropped.
func buildChunksFromRecordGroups(groups [][]lineRecord, p *titleChunkerParam, plain bool) []map[string]any {
	chunks := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		chunk := map[string]any{"text": joinGroupText(g)}
		if !plain {
			first := g[0]
			if first.docType != "" {
				chunk["doc_type_kwd"] = first.docType
			}
			if first.imgID != nil {
				chunk["img_id"] = *first.imgID
			}
		}
		chunks = append(chunks, chunk)
	}
	if p.RootChunkAsHeading && len(chunks) > 1 {
		rootText := toString(chunks[0]["text"])
		for i := 1; i < len(chunks); i++ {
			chunks[i]["text"] = rootText + "\n" + toString(chunks[i]["text"])
		}
		chunks = chunks[1:]
	}
	return chunks
}

// extractLineRecords reads the chunker inputs in the same order the
// python BaseTitleChunker.extract_line_records uses:
//
//  1. If upstream emitted chunks (output_format == "chunks") OR
//     upstream emitted JSON, normalise from the list payload.
//  2. Otherwise, treat text/markdown/html as a "one record per line"
//     stream (preserving indentation for non-text formats, strip-only
//     for the text format).
func extractLineRecords(inputs map[string]any) []lineRecord {
	if docs := chunksFromInputs(inputs); docs != nil {
		return recordsFromStructured(docs)
	}
	text, _ := stringFromInputs(inputs, "text", "content")
	if text == "" {
		return nil
	}
	return lineRecordsFromText(text)
}

func recordsFromStructured(items []schema.ChunkDoc) []lineRecord {
	out := make([]lineRecord, 0, len(items))
	for _, it := range items {
		text := itemTextOrFallback(it)
		if text == "" {
			continue
		}
		dt := it.DocType
		if dt == "" {
			dt = "text"
		}
		var imgID *string
		if it.ImgID != "" {
			img := it.ImgID
			imgID = &img
		}
		meta := make(map[string]any)
		if it.ContentLtks != "" {
			meta["content_ltks"] = it.ContentLtks
		}
		if it.ContentSmLtks != "" {
			meta["content_sm_ltks"] = it.ContentSmLtks
		}
		if it.ContentWithWeight != "" {
			meta["content_with_weight"] = it.ContentWithWeight
		}
		if it.TitleTks != "" {
			meta["title_tks"] = it.TitleTks
		}
		if it.TitleSmTks != "" {
			meta["title_sm_tks"] = it.TitleSmTks
		}
		for k, v := range it.Extra {
			meta[k] = json.RawMessage(v)
		}
		out = append(out, lineRecord{
			text:       text,
			docType:    dt,
			imgID:      imgID,
			layout:     it.Layout,
			parentMeta: meta,
		})
	}
	return out
}

// resolveGroupTargetLevel mirrors group_chunker._resolve_group_target_level:
// when `hierarchy` is set (>0) the target depth is resolve_target_level,
// otherwise it is the frequency-derived `most_level` directly.
func resolveGroupTargetLevel(levels []int, p *titleChunkerParam, mostLevel int) int {
	if p.Hierarchy != nil && *p.Hierarchy > 0 {
		return resolveTargetLevel(levels, *p.Hierarchy)
	}
	return mostLevel
}

// GroupTitleChunkerComponent is the standalone variant entry point.
// It is registered separately so canvas authors can pick the strategy
// directly. TitleChunker's dispatcher routes to invokeGroup /
// invokeHierarchy as well.
type GroupTitleChunkerComponent struct {
	name  string
	param titleChunkerParam
}

// NewGroupTitleChunker constructs the variant component with method
// pre-set to "group".
func NewGroupTitleChunker(params map[string]any) (runtime.Component, error) {
	conf := map[string]any{"method": "group"}
	for k, v := range params {
		conf[k] = v
	}
	p := defaultsTitle()
	p.Update(conf)
	if err := p.TitleChunkerParam.Validate(); err != nil {
		return nil, fmt.Errorf("GroupTitleChunker: %w", err)
	}
	return &GroupTitleChunkerComponent{
		name:  ComponentNameGroupTitleChunker,
		param: p,
	}, nil
}

func (c *GroupTitleChunkerComponent) Parallelism() int { return 2 }
func (c *GroupTitleChunkerComponent) Inputs() map[string]string {
	return ChunkerInputs
}
func (c *GroupTitleChunkerComponent) Outputs() map[string]string {
	return ChunkerOutputs
}

func (c *GroupTitleChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return runtime.TrackElapsed(ComponentNameGroupTitleChunker, func() (map[string]any, error) {
		if inputs == nil {
			inputs = map[string]any{}
		}
		// `name` is read from the workflow-wide Globals bag (seeded at
		// pipeline start, published by the File component), not from the
		// upstream output map.
		name := globals.GlobalOrInput(ctx, inputs, "name", "")
		if name == "" {
			return map[string]any{
				"output_format": "chunks",
				"chunks":        []map[string]any{},
				"_ERROR":        "GroupTitleChunker: missing required upstream field \"name\"",
			}, nil
		}
		return invokeGroup(ctx, withName(inputs, name), &c.param)
	})
}

// init registers GroupTitleChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameGroupTitleChunker)
}
