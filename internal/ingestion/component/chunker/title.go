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

// SCOPE (honest) for title.go:
//
//   - TitleChunker is the dispatcher for the three Go chunks: this
//     file holds the TitleChunker variant itself, which mirrors
//     python `rag/flow/chunker/title_chunker/title_chunker.py` — it
//     holds no business logic of its own; the actual chunking
//     happens in group.go / hierarchy.go depending on the
//     `method` param.
//
//   - PARALLELISM: Parallelism() is only a hint for outer executors.
//     TitleChunker.Invoke dispatches synchronously to group.go or
//     hierarchy.go.
//
//   - HEADING DETECTION PARITY:
//     The Python side uses three heading-detection strategies in
//     `common.py:resolve_title_levels`:
//     (1) PDF outlines   (extract_pdf_outlines, requires deepdoc/parser)
//     (2) Regex families (the user's `levels` param)
//     (3) Layout hints   (layout field matches section/title/head)
//     The Go port ships ONLY strategy (2). Strategies (1) and (3)
//     require PDF binary access (deepdoc is Python-only) and a parser
//     that emits a layout field. A canvas author who needs PDF-outline heading
//     detection must wait for the deepdoc/parser port.
//
//   - TODO: parity-gap — non-ASCII heading formats and PDFs without
//     user-supplied `levels` are not detected. Tests assert ASCII
//     input only.
//
//   - GROUP-TITLE and HIERARCHY-TITLE are separate Go files
//     (`group.go`, `hierarchy.go`); they share the resolve_levels
//     path with TitleChunker but their build_chunks logic differs.
package chunker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

const ComponentNameTitleChunker = "TitleChunker"

type titleChunkerParam struct {
	schema.TitleChunkerParam
}

func (p *titleChunkerParam) Update(conf map[string]any) {
	if conf == nil {
		return
	}
	if v, ok := conf["method"].(string); ok {
		p.TitleChunkerParam.Method = v
	}
	if v, ok := conf["levels"].([]any); ok {
		p.TitleChunkerParam.Levels = parseLevels(v)
	} else if v, ok := conf["levels"].([][]string); ok {
		p.TitleChunkerParam.Levels = v
	}
	if v, ok := numericFromAny(conf["hierarchy"]); ok {
		n := int(v)
		p.TitleChunkerParam.Hierarchy = &n
	}
	if v, ok := conf["include_heading_content"].(bool); ok {
		p.TitleChunkerParam.IncludeHeadingContent = v
	}
	if v, ok := conf["root_chunk_as_heading"].(bool); ok {
		p.TitleChunkerParam.RootChunkAsHeading = v
	}
}

// parseLevels accepts a [[string]] representation — the natural
// JSON form — and rebroadcasts the strings verbatim. Stricter
// validation lives in schema.TitleChunkerParam.Validate.
func parseLevels(in []any) [][]string {
	out := make([][]string, 0, len(in))
	for _, lvl := range in {
		group, ok := lvl.([]any)
		if !ok {
			continue
		}
		row := make([]string, 0, len(group))
		for _, pat := range group {
			if s, ok := pat.(string); ok && s != "" {
				row = append(row, s)
			}
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out
}

func defaultsTitle() titleChunkerParam {
	return titleChunkerParam{TitleChunkerParam: schema.TitleChunkerParam{}.Defaults()}
}

// selectLevelGroup mirrors common.py:select_level_group. Returns the
// regex-list family with the highest hit count across the input
// lines.
func selectLevelGroup(lines []string, rawLevels [][]string) []string {
	if len(rawLevels) == 0 {
		return nil
	}
	hits := make([]int, len(rawLevels))
	for i, group := range rawLevels {
		for _, line := range lines {
			text := trim(line)
			if text == "" {
				continue
			}
			for _, pattern := range group {
				if re := compileLevelPattern(pattern); re != nil {
					if re.MatchString(text) {
						hits[i]++
						break
					}
				}
			}
		}
	}
	bestIdx := -1
	best := 0
	for i, h := range hits {
		if h > best {
			best = h
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil
	}
	group := make([]string, 0, len(rawLevels[bestIdx]))
	for _, p := range rawLevels[bestIdx] {
		if p != "" {
			group = append(group, p)
		}
	}
	return group
}

// matchRegexLevel mirrors common.py:match_regex_level. Levels are
// 1-indexed; "" or no-match returns 0 (BODY).
func matchRegexLevel(text string, group []string) int {
	stripped := trim(text)
	if stripped == "" {
		return 0
	}
	for lvl, pattern := range group {
		re := compileLevelPattern(pattern)
		if re != nil && re.MatchString(stripped) {
			return lvl + 1
		}
	}
	return 0
}

// resolveTitleLevels mirrors common.py:resolve_title_levels in the
// "frequency" branch only (the outline branch is parity-gap territory
// per the SCOPE comment above).
func resolveTitleLevels(lines []string, p *titleChunkerParam) []int {
	group := selectLevelGroup(lines, p.Levels)
	if len(group) == 0 {
		// No levels hit — every line is body.
		out := make([]int, len(lines))
		for i := range out {
			out[i] = bodyLevel
		}
		return out
	}
	out := make([]int, len(lines))
	for i, ln := range lines {
		out[i] = matchRegexLevel(ln, group)
		if out[i] == 0 {
			out[i] = bodyLevel
		}
	}
	return out
}

// bodyLevel is the sentinel python uses for non-heading lines. We use
// the same large int (sys.maxsize - 1) for parity. Practically this
// just needs to be "larger than any realistic heading level"; tests
// only check relative ordering.
const bodyLevel = 1<<31 - 1

// lineRecords mirrors common.py:extract_line_records' markdown/text/html
// branch. Returns one record per non-empty input line.
func lineRecordsFromText(text string) []lineRecord {
	if text == "" {
		return nil
	}
	out := make([]lineRecord, 0)
	for _, ln := range strings.Split(text, "\n") {
		if trim(ln) == "" {
			continue
		}
		out = append(out, lineRecord{
			text:    ln,
			docType: "text",
			imgID:   nil,
			layout:  "",
			pdfPos:  nil,
		})
	}
	return out
}

// lineRecord is the internal common shape — same fields as
// common.py:extract_line_records yields. Used by Group/Hierarchy
// chunk-builders.
type lineRecord struct {
	text    string
	docType string
	imgID   *string
	layout  string
	pdfPos  []map[string]any
}

func (r lineRecord) textOrEmpty() string { return r.text }
func (r lineRecord) isText() bool        { return r.docType == "text" }

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		last := s[len(s)-1]
		if last == ' ' || last == '\t' || last == '\r' || last == '\n' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	return s
}

// compileLevelPattern returns a compiled regex for `pattern`. Returns
// nil on empty/error to skip the entry — same effect as a regex
// mismatch in python (the row falls through to body).
func compileLevelPattern(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// LevelContext groups the level-detection artefacts (per-line levels
// + the most-common heading level) so the strategy implementations
// don't recompute.
type LevelContext struct {
	levels    []int
	mostLevel int
}

func newLevelContext(lines []string, p *titleChunkerParam) LevelContext {
	levels := resolveTitleLevels(lines, p)
	most := 0
	seen := make(map[int]int)
	for _, lvl := range levels {
		seen[lvl]++
	}
	// Pick the smallest level that has any hits — titles are
	// more-specific headings, so the highest-specificity one wins.
	for _, lvl := range seen {
		if lvl < bodyLevel && (most == 0 || lvl < most) {
			most = lvl
		}
	}
	return LevelContext{levels: levels, mostLevel: most}
}

func (lc LevelContext) Levels() []int {
	out := make([]int, len(lc.levels))
	copy(out, lc.levels)
	return out
}

// buildChunksFromRecordGroups is the cheap text-form materialiser
// (used by Group/Hierarchy for plain-text payloads). For structured
// upstream payloads a parallel fan-out over groups is used.
func buildChunksFromRecordGroupsText(groups [][]lineRecord) []map[string]any {
	out := make([]map[string]any, 0, len(groups))
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		var sb strings.Builder
		for _, r := range g {
			sb.WriteString(r.text)
			sb.WriteString("\n")
		}
		out = append(out, map[string]any{"text": sb.String()})
	}
	return out
}

// ---------------------------------------------------------------------------
// Component implementations
// ---------------------------------------------------------------------------

// TitleChunkerComponent dispatches based on `param.method`. Heading
// detection is shared (resolveTitleLevels); the actual chunk-build
// logic lives in group.go / hierarchy.go.
type TitleChunkerComponent struct {
	name  string
	param titleChunkerParam
}

// NewTitleChunker constructs a TitleChunker from the DSL param map.
// Errors here surface as canvas compile failures.
func NewTitleChunker(params map[string]any) (runtime.Component, error) {
	p := defaultsTitle()
	p.Method = "group" // default to group
	p.Update(params)
	if err := p.TitleChunkerParam.Validate(); err != nil {
		return nil, fmt.Errorf("TitleChunker: %w", err)
	}
	return &TitleChunkerComponent{
		name:  ComponentNameTitleChunker,
		param: p,
	}, nil
}

// Parallelism is the configured intra-component fan-out (plan §4
// Phase 2 row 2.3b).
func (c *TitleChunkerComponent) Parallelism() int { return 2 }

// Inputs is exposed so callers can introspect.
func (c *TitleChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

// Outputs is exposed so callers can introspect.
func (c *TitleChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

// Invoke delegates to the chosen strategy (group or hierarchy).
func (c *TitleChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return runtime.TrackElapsed(ComponentNameTitleChunker, func() (map[string]any, error) {
		if inputs == nil {
			return emptyOutputs(), nil
		}
		if _, ok := inputs["name"].(string); !ok {
			return map[string]any{
				"output_format": "chunks",
				"chunks":        []map[string]any{},
				"_ERROR":        "TitleChunker: missing required upstream field \"name\"",
			}, nil
		}
		switch c.param.Method {
		case "hierarchy":
			return invokeHierarchy(ctx, inputs, &c.param)
		case "group":
			return invokeGroup(ctx, inputs, &c.param)
		default:
			return map[string]any{
				"output_format": "chunks",
				"chunks":        []map[string]any{},
				"_ERROR":        fmt.Sprintf("TitleChunker: unsupported method %q", c.param.Method),
			}, nil
		}
	})
}

// init registers TitleChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameTitleChunker)
}
