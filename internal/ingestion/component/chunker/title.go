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
//     TitleChunker.Invoke dispatches synchronously to group.go or
//     hierarchy.go.
//
//   - HEADING DETECTION PARITY:
//     The Python side uses two heading-detection strategies in
//     `common.py:resolve_title_levels`:
//     (1) PDF outlines   (extract_pdf_outlines, requires deepdoc/parser)
//     (2) Regex families (the user's `levels` param) with a layout-hint
//     fallback (layout field matches section/title/head).
//     The Go port ships strategy (2) IN FULL, including the
//     match_layout_level fallback ported from common.py (Gap C closed):
//     a non-regex text record whose layout flags it as a
//     section/title/head and whose text passes not_title is promoted to
//     fallback_level = len(selected_group) + 1. Strategy (1) —
//     PDF-outline detection — still requires deepdoc/parser binary
//     access and remains the only parity gap.
//
//   - The Go port has NO hardcoded BULLET_PATTERN fallback. Heading
//     detection relies entirely on the user-supplied `levels` param
//     (which templates carry as comprehensive regex families) paired
//     with the layout-hint fallback. A PDF without matching levels
//     produces BODY_LEVEL-only records — Python's BULLET_PATTERN-based
//     tree_merge / hierarchical_merge would still find structure.
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
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/globals"
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
			// Mirror common.py:select_level_group — a bullet-looking
			// line cannot count as a heading hit for any family.
			if notBullet(text) {
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
	// Mirror common.py:match_regex_level — a bullet-looking line is not
	// a heading regardless of the regex family.
	if notBullet(stripped) {
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

// notBulletPatterns mirrors rag/nlp.not_bullet. A line matching any of
// these looks like a numbered/bulleted list item rather than a section
// heading, so it must not be promoted to a title level by the regex
// families.
var notBulletPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^0`),
	regexp.MustCompile(`^[0-9]+ +[0-9~个只-]`),
	regexp.MustCompile(`^[0-9]+\.{2,}`),
	regexp.MustCompile(`^[0-9]+(\.[0-9]+){2,}[的中]`),
}

// notBullet mirrors rag/nlp.not_bullet: true when the line looks like a
// numbered/bulleted list entry rather than a genuine section heading.
func notBullet(s string) bool {
	for _, re := range notBulletPatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// notTitlePatterns mirrors rag/nlp.not_title:
//   - notTitleException: `第...条` is always a title (never "not a title").
//   - notTitlePunct: a body line typically carries one of these punctuation
//     marks, so their presence flags the line as body text.
var (
	notTitleException = regexp.MustCompile(`^第[零一二三四五六七八九十百0-9]+条`)
	notTitlePunct     = regexp.MustCompile(`[,;，。；！!]`)
	layoutHeadingRe   = regexp.MustCompile(`(?i)(section|title|head)`)
	numericOnly       = regexp.MustCompile(`^[0-9]+$`)
)

// notTitle mirrors rag/nlp.not_title. Returns true when the line looks
// like body text rather than a section heading, so layout-based heading
// detection must skip it.
func notTitle(s string) bool {
	if notTitleException.MatchString(s) {
		return false
	}
	if len(strings.Fields(s)) > 12 || (!strings.Contains(s, " ") && utf8.RuneCountInString(s) >= 32) {
		return true
	}
	return notTitlePunct.MatchString(s)
}

// beforeAt mirrors python's `text.split("@")[0]` (used by
// match_layout_level / not_title): the part of the line before the first
// "@" separator, whitespace-trimmed.
func beforeAt(s string) string {
	if i := strings.Index(s, "@"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// matchLayoutLevel mirrors common.py:match_layout_level. When the
// record's layout field flags it as a section/title/head and the text is
// title-like (not not_title), the record is promoted to `fallback_level`
// (common.py:resolve_frequency_levels sets this to len(level_group) + 1).
// Otherwise it stays BODY_LEVEL.
func matchLayoutLevel(text, layout string, fallbackLevel int) int {
	if layoutHeadingRe.MatchString(layout) && !notTitle(beforeAt(text)) {
		return fallbackLevel
	}
	return bodyLevel
}

// resolveTitleLevels mirrors common.py:resolve_frequency_levels over the
// full record stream. It is the "frequency" branch of
// common.py:resolve_title_levels (the outline branch is parity-gap
// territory per the SCOPE comment above).
//
// For each record:
//   - a non-text record is pinned to BODY_LEVEL directly (python skips
//     regex/layout detection for doc_type_kwd != "text");
//   - a text record first tries the selected regex group (match_regex_level);
//     on no match it falls back to match_layout_level (layout hint), else
//     BODY_LEVEL.
//
// fallback_level is len(selected_group) + 1, exactly as in python.
// isColonTitle mirrors Python make_colon_as_title intent: returns true
// when a line ends with colon, has sentence-ending punctuation before
// the colon, and the text between them is at least 32 runes long.
func isColonTitle(s string) bool {
	if s == "" {
		return false
	}
	if !strings.HasSuffix(s, ":") && !strings.HasSuffix(s, "：") {
		return false
	}
	body := strings.TrimSuffix(s, ":")
	body = strings.TrimSuffix(body, "：")
	if body == s {
		return false
	}
	lastPunct := strings.LastIndexAny(body, "。？！!?;；.")
	if lastPunct < 0 {
		return false
	}
	// Use rune-aware slicing: body[lastPunct+1] would be wrong for CJK
	// punctuation (e.g. "。" is 3 bytes in UTF-8). Decode the rune at
	// lastPunct to get the correct byte width and skip the full rune.
	_, runeLen := utf8.DecodeRuneInString(body[lastPunct:])
	between := strings.TrimSpace(body[lastPunct+runeLen:])
	return utf8.RuneCountInString(between) >= 32
}

func resolveTitleLevels(records []lineRecord, p *titleChunkerParam) []int {
	lines := make([]string, len(records))
	for i, r := range records {
		lines[i] = r.text
	}
	group := selectLevelGroup(lines, p.Levels)
	fallbackLevel := len(group) + 1
	out := make([]int, len(records))
	for i, rec := range records {
		if !rec.isText() {
			out[i] = bodyLevel
			continue
		}
		// Python tree_merge short/numeric line filter:
		// sections = [s for s in sections if len(s.split("@")[0].strip()) > 1
		//             and not re.match(r"[0-9]+$", s.split("@")[0].strip())]
		if text := beforeAt(rec.text); utf8.RuneCountInString(text) <= 1 || numericOnly.MatchString(text) {
			out[i] = bodyLevel
			continue
		}
		if lvl := matchRegexLevel(rec.text, group); lvl != 0 {
			out[i] = lvl
			continue
		}
		if rec.ckType == "heading" {
			out[i] = fallbackLevel
			continue
		}
		// Python make_colon_as_title: promote lines ending with colon
		// that have sentence-ending punctuation before it and at least
		// 32 chars between the punctuation and the colon.
		if text := beforeAt(rec.text); isColonTitle(text) {
			out[i] = fallbackLevel
			continue
		}
		out[i] = matchLayoutLevel(rec.text, rec.layout, fallbackLevel)
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
	text       string
	docType    string
	imgID      *string
	layout     string
	ckType     string
	pdfPos     []map[string]any
	parentMeta map[string]any
}

func (r lineRecord) textOrEmpty() string { return r.text }
func (r lineRecord) isText() bool        { return r.docType == "text" }

// trim mirrors python's str.strip(): remove leading/trailing Unicode
// whitespace. Used for emptiness checks and regex matching so the Go
// port matches python's `text.strip()` exactly.
func trim(s string) string {
	return strings.TrimSpace(s)
}

// compileLevelPattern returns a compiled regex for `pattern`. Returns
// nil on empty/error to skip the entry — same effect as a regex
// mismatch in python (the row falls through to body).
//
// Python's match_regex_level / select_level_group use `re.match`, which
// anchors at the START of the string (not the end). We mirror that by
// prepending `^` (unless the caller already anchored) so Go's
// MatchString behaves exactly like re.match.
func compileLevelPattern(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	anchored := pattern
	if !strings.HasPrefix(pattern, "^") {
		anchored = "^(?:" + pattern + ")"
	}
	re, err := regexp.Compile(anchored)
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

func newLevelContext(records []lineRecord, p *titleChunkerParam) LevelContext {
	levels := resolveTitleLevels(records, p)
	// most_level is the most-frequent non-body heading level
	// (common.py:resolve_frequency_levels). Python computes this via
	// Counter(levels).most_common() over the heading levels only. Walk
	// levels in input order so ties resolve to the first-encountered
	// level, matching python's insertion-order tie-break.
	counts := make(map[int]int)
	for _, lvl := range levels {
		if lvl < bodyLevel {
			counts[lvl]++
		}
	}
	most := 0
	best := 0
	for _, lvl := range levels {
		if c := counts[lvl]; c > best {
			best = c
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

// Inputs is exposed so callers can introspect.
func (c *TitleChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

// Outputs is exposed so callers can introspect.
func (c *TitleChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

// Invoke delegates to the chosen strategy (group or hierarchy).
func (c *TitleChunkerComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
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
}

// init registers TitleChunker under CategoryIngestion.
func init() {
	MustRegisterChunker(ComponentNameTitleChunker)
}
