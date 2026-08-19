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

package harness

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"

	"gorm.io/gorm"
	"ragflow/internal/agent/chat"
)

// LLM Sufficient Context AutoRater (mirrors Python sufficiency_llm.py +
// rag/prompts/sufficiency_select.md). It is the *primary* sufficiency judge in
// the decision-ladder design: it decides whether the retrieved evidence supports
// a plausible answer, and on "insufficient" returns concrete missing information
// that becomes follow-up search queries.

// sufficiencySelectPrompt mirrors rag/prompts/sufficiency_select.md.
const sufficiencySelectPrompt = `You are an information retrieval evaluation expert. Determine whether the retrieved content is sufficient to answer the user's question(s), following the "Sufficient Context" criterion:

The CONTEXT is sufficient to answer the question if and only if a PLAUSIBLE answer can be inferred from it — that is, the retrieved content either directly contains or logically entails an answer to the question. The answer does NOT need to be proven correct; it only needs to be a reasonable, supportable answer. If the context cannot be used to infer any plausible answer, it is INSUFFICIENT.

Each retrieved chunk is labeled with an integer ID on a line like ` + "`ID: 3`" + `.

User question(s):
%s

Retrieved content:
%s

Reasoning procedure (do this step-by-step before answering):
1. Identify the REQUIRED ENTITIES or key facts that a plausible answer to the question must involve.
2. For each required entity, check whether the retrieved content provides evidence about it. Record this in "coverage".
3. Check for multi-hop inference: if answering requires combining facts not present in the context, or inferring a connection the context does not state, that is NOT inferable from the context.
4. Check whether the context is ambiguous: if it could support multiple mutually exclusive plausible answers and nothing in the context lets you distinguish them, mark it insufficient.
5. Note any internally conflicting figures/statements in the context ("contradictions").
6. Decide whether a plausible answer can be inferred; give your confidence in that decision.

Output format (JSON):
{
    "Sufficient Context": true/false,
    "is_sufficient": true/false,
    "required_entities": ["Entity 1", "Entity 2"],
    "coverage": {"Entity 1": true, "Entity 2": false},
    "missing_information": ["Missing information 1", "Missing information 2"],
    "contradictions": ["conflicting figures/statements if any"],
    "confidence": 0.0,
    "reasoning": "Step-by-step reasoning for the judgment",
    "useful_chunk_ids": [0, 3, 7]
}

Requirements:
1. ` + "`Sufficient Context`" + ` / ` + "`is_sufficient`" + ` must be true if and only if a plausible answer can be inferred from the context (per the definition above). A missing detail that a reasonable answer would still require makes it false.
2. If not sufficient, list the concrete ` + "`missing_information`" + `.
3. ` + "`coverage`" + ` must mark, for each required entity, whether the context provides evidence about it. Missing required entities belong in ` + "`missing_information`" + `.
4. ` + "`confidence`" + ` (0-1): how confident you are in your sufficiency decision. 0.9-1.0 if the context clearly supports or clearly fails a plausible answer; 0.5-0.7 if evidence is partial or ambiguous; below 0.5 if you cannot tell.
5. ` + "`contradictions`" + `: list any internally conflicting figures/statements that would make a single answer ambiguous. Empty array when none.
6. ` + "`useful_chunk_ids`" + ` must contain ONLY the integer IDs (taken from the ` + "`ID:`" + ` labels above) of chunks that provide information useful for answering the question(s). Exclude irrelevant or redundant chunks. Use an empty array when none are useful.
7. The ` + "`missing_information`" + ` should only be filled when insufficient, otherwise an empty array.
8. The ` + "`reasoning`" + ` should be concise and clear.`

const (
	maxEvidenceChunksLLM = 24
	maxChunkCharsLLM     = 800
	maxEvidenceCharsLLM  = 24000
)

type sufficiencySelectResult struct {
	SufficientContext bool            `json:"Sufficient Context"`
	IsSufficient      bool            `json:"is_sufficient"`
	RequiredEntities  []string        `json:"required_entities"`
	Coverage          map[string]bool `json:"coverage"`
	MissingInfo       []string        `json:"missing_information"`
	Contradictions    []string        `json:"contradictions"`
	Confidence        float64         `json:"confidence"`
	Reasoning         string          `json:"reasoning"`
	UsefulChunkIDs    []int           `json:"useful_chunk_ids"`
}

var reNarrowTokens = regexp.MustCompile(`[a-zA-Z0-9]+|[\x{4e00}-\x{9fff}]+`)

// narrowKeywords mirrors Python _narrow_keywords: language-agnostic keywords for
// snippet narrowing (numeric tokens, latin len>=4, CJK character bigrams).
func narrowKeywords(question string) []string {
	tokens := reNarrowTokens.FindAllString(strings.ToLower(question), -1)
	var kw []string
	for _, t := range tokens {
		if isDigits(t) {
			kw = append(kw, t)
		} else if containsLatin(t) {
			if len(t) >= 4 {
				kw = append(kw, t)
			}
		} else {
			// CJK run → character bigrams
			rs := []rune(t)
			for i := 0; i+1 < len(rs); i++ {
				kw = append(kw, string(rs[i:i+2]))
			}
		}
	}
	return kw
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func containsLatin(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

// renderEvidenceMD renders the cited evidence chunks with "ID: n" markers,
// mirroring Python _evidence_md. Prefers the chunks referenced by evidenceIDs;
// falls back to a bounded prefix of the pool when none are given.
func renderEvidenceMD(kb *Kbinfos, evidenceIDs []int, keywords []string) string {
	if kb == nil || len(kb.Chunks) == 0 {
		return ""
	}
	var picked []int
	if len(evidenceIDs) > 0 {
		seen := map[int]bool{}
		for _, eid := range evidenceIDs {
			if eid >= 0 && eid < len(kb.Chunks) && !seen[eid] {
				seen[eid] = true
				picked = append(picked, eid)
			}
			if len(picked) >= maxEvidenceChunksLLM {
				break
			}
		}
	}
	if len(picked) == 0 {
		n := len(kb.Chunks)
		if n > maxEvidenceChunksLLM {
			n = maxEvidenceChunksLLM
		}
		for i := 0; i < n; i++ {
			picked = append(picked, i)
		}
	}

	var blocks []string
	used := 0
	for _, idx := range picked {
		c := kb.Chunks[idx]
		raw := chunkText(c)
		title := chunkDoc(c)
		if len(keywords) > 0 {
			if narrowed := narrowSnippetSafe(raw, keywords); narrowed != "" {
				raw = narrowed
			}
		}
		if len(raw) > maxChunkCharsLLM {
			raw = raw[:maxChunkCharsLLM]
		}
		if used+len(raw) > maxEvidenceCharsLLM {
			break
		}
		blocks = append(blocks, fmt.Sprintf("ID: %d | %s\n%s", idx, title, raw))
		used += len(raw) + 8
	}
	return strings.Join(blocks, "\n\n")
}

// narrowSnippetSafe mirrors Python _narrow_snippet_safe: keep keyword-bearing
// sentences (plus one neighbour) only when keywords cover a meaningful share.
// Returns "" to signal "keep the whole chunk".
func narrowSnippetSafe(content string, kw []string) string {
	sents := splitSentences(content)
	if len(sents) <= 3 {
		return ""
	}
	lower := make([]string, len(sents))
	for i, s := range sents {
		lower[i] = strings.ToLower(s)
	}
	var hitIdx []int
	for i, s := range lower {
		for _, k := range kw {
			if strings.Contains(s, k) {
				hitIdx = append(hitIdx, i)
				break
			}
		}
	}
	if len(hitIdx) < 2 {
		return ""
	}
	keep := map[int]bool{}
	for _, i := range hitIdx {
		for j := i - 1; j <= i+1; j++ {
			if j >= 0 && j < len(sents) {
				keep[j] = true
			}
		}
	}
	// Preserve order.
	var out []string
	for i := 0; i < len(sents); i++ {
		if keep[i] {
			out = append(out, sents[i])
		}
	}
	return strings.Join(out, " ")
}

// Sentence splitting mirrors Python tools/search.py _split_sentences:
//   - terminators are KEPT on their sentence (。！？；!?; plus a digit-guarded
//     English period so "3.14" / "v1.2" do not split);
//   - table blocks (HTML <table> and markdown tables) are ATOMIC — never split
//     internally.
//
// Go's RE2 lacks lookbehind, so the digit-guard is handled by a manual scan.
var (
	reHTMLTable = regexp.MustCompile(`(?is)<table\b[^>]*>.*?</table>`)
	reMDTable   = regexp.MustCompile("(?m)^[ \t]*\\|?[^\n]*\\|[^\n]*\r?\n[ \t]*\\|?[ \t]*:?-{1,}:?[ \t]*(?:\\|[ \t]*:?-{1,}:?[ \t]*)+\\|?[ \t]*\r?\n(?:[ \t]*\\|?[^\n]*\\|[^\n]*\r?\n?)*")
)

func splitSentences(text string) []string {
	if text == "" {
		return nil
	}
	// Collect non-overlapping table spans (HTML + markdown), in order.
	var spans [][2]int
	spans = append(spans, matchSpans(reHTMLTable, text)...)
	spans = append(spans, matchSpans(reMDTable, text)...)
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var merged [][2]int
	lastEnd := -1
	for _, s := range spans {
		if s[0] < lastEnd {
			continue
		}
		merged = append(merged, s)
		lastEnd = s[1]
	}

	var sents []string
	pos := 0
	for _, m := range merged {
		if m[0] > pos {
			sents = append(sents, splitPlainSentences(text[pos:m[0]])...)
		}
		if block := strings.TrimSpace(text[m[0]:m[1]]); block != "" {
			sents = append(sents, block)
		}
		pos = m[1]
	}
	if pos < len(text) {
		sents = append(sents, splitPlainSentences(text[pos:])...)
	}
	return sents
}

func matchSpans(re *regexp.Regexp, text string) [][2]int {
	matches := re.FindAllStringIndex(text, -1)
	out := make([][2]int, 0, len(matches))
	for _, m := range matches {
		out = append(out, [2]int{m[0], m[1]})
	}
	return out
}

// splitPlainSentences splits plain text (no table blocks) into sentences,
// keeping each terminator attached and guarding decimal periods. Operates on
// runes; rune indices == byte indices for the ASCII terminators we emit.
func splitPlainSentences(text string) []string {
	rs := []rune(text)
	var sents []string
	start := 0
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if !isSentTerminator(r) {
			continue
		}
		// ASCII period guarded against decimals (digit on BOTH sides).
		if r == '.' && i > 0 && i+1 < len(rs) && isASCIIDigit(rs[i-1]) && isASCIIDigit(rs[i+1]) {
			continue
		}
		// Consume a run of terminators (e.g. "。！？" or "...").
		j := i + 1
		for j < len(rs) && isSentTerminator(rs[j]) && rs[j] != '.' {
			j++
		}
		seg := strings.TrimSpace(string(rs[start:j]))
		if seg != "" {
			sents = append(sents, seg)
		}
		start = j
		i = j - 1
	}
	if start < len(rs) {
		if tail := strings.TrimSpace(string(rs[start:])); tail != "" {
			sents = append(sents, tail)
		}
	}
	return sents
}

func isSentTerminator(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '.':
		return true
	}
	return false
}

func isASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// LLMSufficiencyBoost mirrors Python llm_sufficiency_boost: run the AutoRater on
// the cited evidence and return an AutoRating. Returns nil when no LLM judge is
// available or the verdict is already clear (SUFFICIENT/UNANSWERABLE).
func LLMSufficiencyBoost(ctx context.Context, db *gorm.DB, question string, verdict *SufficiencyVerdict, kb *Kbinfos, evidenceIDs []int) *AutoRating {
	if verdict == nil {
		return nil
	}
	switch verdict.Status {
	case "USEFUL_BUT_INCOMPLETE", "INSUFFICIENT", "CONFLICTING":
		// boost applicable
	default:
		return nil
	}
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return nil
	}
	evidenceMD := renderEvidenceMD(kb, evidenceIDs, narrowKeywords(question))
	if evidenceMD == "" {
		return nil
	}
	prompt := fmt.Sprintf(sufficiencySelectPrompt, question, evidenceMD)
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: prompt},
		},
	})
	if err != nil {
		return nil
	}
	var res sufficiencySelectResult
	if err := unmarshalModelJSON(resp.Content, &res); err != nil {
		return nil
	}
	isSuff := res.IsSufficient || res.SufficientContext
	missing := filterNonEmpty(res.MissingInfo)
	contradictions := filterNonEmpty(res.Contradictions)
	confidence := clamp01(res.Confidence)
	rating := &AutoRating{
		IsSufficient:   isSuff,
		Confidence:     confidence,
		Missing:        missing,
		Contradictions: contradictions,
	}
	// Phase-2: when the AutoRater says insufficient with concrete gaps, generate
	// complementary follow-up search queries for the next round (mirrors Python
	// gen_followups → multi_queries_gen). This is the missing-piece feedback loop.
	if !isSuff && len(missing) > 0 {
		rating.Followups = genFollowups(ctx, db, question, missing, evidenceMD)
	}
	return rating
}

// multiQueriesGenPrompt mirrors rag/prompts/multi_queries_gen.md.
const multiQueriesGenPrompt = `You are a query optimization expert.
The user's original query failed to retrieve sufficient information;
please generate multiple complementary improved questions and corresponding queries.

Original query:
%s

Original question:
%s

Currently, retrieved content:
%s

Missing information:
%s

Please generate 2-3 complementary queries to help find the missing information. These queries should:
1. Focus on different missing information points.
2. Use different expressions.
3. Avoid being identical to the original query.
4. Remain concise and clear.

Output format (JSON):
{
    "reasoning": "Explanation of query generation strategy",
    "questions": [
        {"question": "Improved question 1", "query": "Improved query 1"}
    ]
}

Requirements:
1. Questions array contains 1-3 questions and corresponding queries.
2. Each question length is between 5-200 characters.
3. Each query length is between 1-5 keywords.
4. Each query MUST be in the same language as the retrieved content in.
5. DO NOT generate question and query that is similar to the original query.
6. Reasoning explains the generation strategy.`

type multiQueriesResult struct {
	Reasoning string             `json:"reasoning"`
	Questions []multiQueriesItem `json:"questions"`
}

type multiQueriesItem struct {
	Question string `json:"question"`
	Query    string `json:"query"`
}

// genFollowups mirrors Python gen_followups → multi_queries_gen: generate
// complementary follow-up search queries for the missing information. Returns
// the "query or question" strings the research agent injects as targeted
// follow-up searches (mirrors agentic.py:98).
func genFollowups(ctx context.Context, db *gorm.DB, question string, missing []string, evidenceMD string) []string {
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return nil
	}
	// Fit evidence (mirrors _fit_evidence: bounded truncation already done by
	// renderEvidenceMD, so reuse it verbatim).
	missingStr := "\n - " + strings.Join(missing, "\n - ")
	prompt := fmt.Sprintf(multiQueriesGenPrompt, question, question, evidenceMD, missingStr)
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: prompt},
		},
	})
	if err != nil {
		return nil
	}
	var res multiQueriesResult
	if err := unmarshalModelJSON(resp.Content, &res); err != nil {
		return nil
	}
	var out []string
	for _, q := range res.Questions {
		v := strings.TrimSpace(q.Query)
		if v == "" {
			v = strings.TrimSpace(q.Question)
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func filterNonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
