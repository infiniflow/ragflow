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

package orchestrator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/prompts"
)

// Unified Sufficient Context Agent.
//
// Mirrors Python orchestrator/sufficient_context.py.
//
// The SCA performs ONE review pass over (1) each claim's intermediate draft and
// (2) the overall draft assembled from them, and returns a unified verdict:
// is_sufficient / confidence / contradictions / reasoning / claims / sub_queries.
// It replaced an earlier two-call split (global verdict + per-claim groundedness)
// whose trigger bands were complementary, so the three-part review rarely
// happened in one shot.
//
// The SCA reviews ONLY the claims' reports, never the raw retrieved chunks.
// Each report is an evidence-backed finding already distilled from that claim's
// evidence; sending raw chunks ballooned the prompt to 20k-100k chars, which
// degraded the LLM (empty claims) or timed it out.

const (
	// scaClaimsContextMax caps the rendered claims context (all per-claim
	// reports + the overall draft). Sized to match the action session's context
	// budget (maxToolResponseChars * 4) so a table-bearing evidence anchor plus
	// several claim reports still fit.
	scaClaimsContextMax = 48000
	// scaEvidenceAnchorChars is the max chars of each cited snippet kept as an
	// evidence anchor, so the SCA can verify a draft against real retrieved text
	// without a token blow-up.
	scaEvidenceAnchorChars = 300
	// scaMaxAnchorsPerClaim bounds anchors per claim (Python: `if len(anchors) >= 3`).
	scaMaxAnchorsPerClaim = 3
	// feedbackMax bounds the missing pieces folded into the boost feedback string.
	feedbackMax = 4
)

var reHintToken = regexp.MustCompile(`[A-Za-z0-9_\x{4e00}-\x{9fff}]{3,}`)

// ClaimDraft is one claim's intermediate draft plus the evidence it cited.
//
// EvidenceIDs are INDICES into Kbinfos.Chunks (see Kbinfos.Merge, which returns
// global indices) — NOT the chunk's chunk_id hash. Keying the anchor lookup by
// chunk_id would make every anchor a miss and silently disable the "SCA reviews
// the retrieved snippets" guard.
type ClaimDraft struct {
	ID          string
	Draft       string
	EvidenceIDs []string
}

// SubQuery is one step-by-step sub-question with its coverage verdict (Q-CARE).
// Unsatisfied entries carry the concrete missing fact + a search hint — the
// precise "what is missing / where to search next" signal the next round
// consumes.
type SubQuery struct {
	SubQuery    string
	Satisfied   bool
	MissingFact string
	SearchHint  string
}

// MissingPiece mirrors Python's {"what", "search_hint"}.
type MissingPiece struct {
	What       string
	SearchHint string
}

// ClaimVerdict is the SCA's per-claim verdict.
type ClaimVerdict struct {
	Grounded           bool
	Ungrounded         []string
	MissingInformation []MissingPiece
}

// SCAResult is the unified verdict. An empty result means "no new signal",
// which callers must treat as neither sufficient nor insufficient.
type SCAResult struct {
	IsSufficient   bool
	Confidence     float64
	Contradictions []string
	Reasoning      string
	SubQueries     []SubQuery
	Claims         map[string]ClaimVerdict
}

// SCADeps are the dependencies of the SCA.
type SCADeps struct {
	KB *harness.Kbinfos
	// Model generates the JSON verdict.
	Model JSONModel
	// Prompts supplies the "sca_select" template.
	Prompts harness.PromptLoader
}

// SufficientContextAgent mirrors Python sufficient_context_agent.
//
// claims carries per-claim evidence (not the global union): rendering only the
// snippets each claim cited keeps the prompt small (~1-3 chunks per claim) so
// the LLM does not degrade, while the cited snippets still carry the
// answer-bearing facts.
//
// Returns an empty SCAResult when unavailable (no claims, no model, or a
// failure) — callers treat that as "no new signal".
func SufficientContextAgent(ctx context.Context, deps SCADeps, question string, claims []ClaimDraft) SCAResult {
	if len(claims) == 0 || deps.Model == nil {
		return SCAResult{}
	}
	// Python wraps the review in @in_phase("sca").
	ctx, done := harness.Phase(ctx, harness.PhaseSCA)
	defer done()

	claimsContext := renderClaimContext(claims, deps.KB)
	if claimsContext == "" || claimsContext == "(no claim drafts)" {
		return SCAResult{}
	}
	overallDraft := renderOverallDraft(claims)

	prompt := prompts.Render(deps.Prompts, "sca_select", "", map[string]string{
		"question":       question,
		"claims_context": claimsContext,
		"overall_draft":  overallDraft,
	})
	_LOG.Printf("[SCA] unified review of %s (reports only, %d chars; overall draft %d chars)",
		fmtClaimCount(len(claims)), len(claimsContext), len(overallDraft))

	result, err := deps.Model.GenJSON(ctx, prompt)
	if err != nil {
		_LOG.Printf("[SCA] unified review failed: %v", err)
		return SCAResult{}
	}
	data := coerceDict(result)
	if data == nil {
		_LOG.Printf("[SCA] no usable response; treating as no signal")
		return SCAResult{}
	}

	// Per-claim verdicts.
	claimsOut := map[string]ClaimVerdict{}
	if rawClaims, ok := data["claims"].([]any); ok {
		for _, item := range rawClaims {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			cid := strings.TrimSpace(fmt.Sprint(m["claim_id"]))
			if cid == "" {
				continue
			}
			var ungrounded []string
			for _, u := range asSlice(m["ungrounded_assertions"]) {
				if um, ok := u.(map[string]any); ok {
					if s := strings.TrimSpace(fmt.Sprint(firstNonEmpty(um["assertion"], um["reason"]))); s != "" {
						ungrounded = append(ungrounded, s)
					}
				} else if s := strings.TrimSpace(fmt.Sprint(u)); s != "" {
					ungrounded = append(ungrounded, s)
				}
			}
			claimsOut[cid] = ClaimVerdict{
				Grounded:           truthy(m["grounded"]),
				Ungrounded:         ungrounded,
				MissingInformation: parseMissingInformation(m["missing_information"]),
			}
		}
	}

	// Structured sub-query coverage (Q-CARE).
	var subQueries []SubQuery
	for _, sq := range asSlice(data["sub_queries"]) {
		m, ok := sq.(map[string]any)
		if !ok {
			continue
		}
		sqText := strings.TrimSpace(fmt.Sprint(m["sub_query"]))
		if sqText == "" {
			continue
		}
		entry := SubQuery{SubQuery: sqText, Satisfied: truthy(m["satisfied"])}
		if !entry.Satisfied {
			entry.MissingFact = strings.TrimSpace(fmt.Sprint(m["missing_fact"]))
			entry.SearchHint = strings.TrimSpace(fmt.Sprint(m["search_hint"]))
		}
		subQueries = append(subQueries, entry)
	}

	isSufficient := truthy(data["is_sufficient"])

	// Failsafe: the SCA judged the context insufficient but returned an EMPTY
	// claims array (a known degradation on very long prompts). Without a gap, the
	// orchestrator would abandon with "I don't have enough information" despite
	// having retrieved useful snippets. Harvest top-level missing_information, and
	// fall back to the un-verified drafts themselves as coarse gaps.
	if !isSufficient && len(claimsOut) == 0 {
		topMissing := parseMissingInformation(data["missing_information"])
		if len(topMissing) > 0 {
			claimsOut["_global"] = ClaimVerdict{Grounded: false, MissingInformation: topMissing}
			_LOG.Printf("[SCA] insufficient with empty claims; using %d top-level missing piece(s) as the re-search gap.", len(topMissing))
		} else {
			var draftGaps []MissingPiece
			for _, c := range claims {
				if d := strings.TrimSpace(c.Draft); d != "" {
					draftGaps = append(draftGaps, MissingPiece{What: d, SearchHint: d})
				}
			}
			if len(draftGaps) > 0 {
				claimsOut["_global"] = ClaimVerdict{Grounded: false, MissingInformation: draftGaps}
				_LOG.Printf("[SCA] insufficient with no structured gap; deriving %d coarse gap(s) from the drafts.", len(draftGaps))
			}
		}
	}

	return SCAResult{
		IsSufficient:   isSufficient,
		Confidence:     clamp(data["confidence"]),
		Contradictions: stringList(data["contradictions"]),
		Reasoning:      strings.TrimSpace(fmt.Sprint(data["reasoning"])),
		SubQueries:     subQueries,
		Claims:         claimsOut,
	}
}

// Boost mirrors Python to_boost: adapt the unified verdict into the
// decision-ladder boost dict, preserving the contract
// (is_sufficient / confidence / missing / contradictions / feedback /
// followups) that the existing ladder consumes unchanged.
func (s SCAResult) Boost(fallbackFollowups []string) Boost {
	var missing []string
	for _, g := range s.Claims {
		for _, mi := range g.MissingInformation {
			w := strings.TrimSpace(mi.What)
			if w != "" && !containsStr(missing, w) {
				missing = append(missing, w)
			}
		}
	}
	feedback := ""
	if len(missing) > 0 {
		n := min(len(missing), feedbackMax)
		feedback = "missing: " + strings.Join(missing[:n], "; ")
	}
	return Boost{
		IsSufficient:   s.IsSufficient,
		Confidence:     s.Confidence,
		Missing:        missing,
		Contradictions: s.Contradictions,
		Followups:      fallbackFollowups,
		Feedback:       feedback,
		SubQueries:     s.SubQueries,
	}
}

// Boost is the decision-ladder input produced by SCAResult.Boost.
type Boost struct {
	IsSufficient   bool
	Confidence     float64
	Missing        []string
	Contradictions []string
	Followups      []string
	Feedback       string
	// SubQueries is the structured Q-CARE coverage signal consumed by replan.
	SubQueries []SubQuery
}

// ToGrounded mirrors Python to_grounded: adapt the unified verdict into the
// `grounded` dict consumed by replan and the ungrounded-veto path
// ({claim_id: {grounded, ungrounded, missing_information}}).
func (s SCAResult) ToGrounded() map[string]ClaimVerdict { return s.Claims }

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

// renderClaimContext mirrors Python _render_claim_context: each claim's report
// PLUS a brief evidence anchor (the first line of each cited snippet), so the
// SCA can verify the draft is grounded in real retrieved text.
func renderClaimContext(claims []ClaimDraft, kb *harness.Kbinfos) string {
	if len(claims) == 0 {
		return "(no claim drafts)"
	}
	// Key by INDEX into kbinfos chunks (see the ClaimDraft doc comment).
	id2chunk := map[string]map[string]any{}
	if kb != nil {
		for i, c := range kb.Chunks {
			id2chunk[fmt.Sprint(i)] = c
		}
	}
	var blocks []string
	used := 0
	for _, c := range claims {
		if c.Draft == "" {
			continue
		}
		block := fmt.Sprintf("Claim %s (draft):\n%s", c.ID, c.Draft)
		used += len(block) + 2
		var anchors []string
		for _, eid := range c.EvidenceIDs {
			ck, ok := id2chunk[strings.TrimSpace(eid)]
			if !ok {
				continue
			}
			txt := strings.TrimSpace(scaChunkText(ck))
			if txt == "" {
				continue
			}
			// Table chunks contribute their FULL text: hint-token windowing is
			// unreliable for tables (the draft rarely contains the row's entity
			// names) and head-truncation hides answer rows that sit mid-table.
			if ex := boundedExcerpt(txt, c.Draft, scaEvidenceAnchorChars); ex != "" {
				anchors = append(anchors, ex)
			}
			if len(anchors) >= scaMaxAnchorsPerClaim {
				break
			}
		}
		if len(anchors) > 0 {
			anchorText := strings.Join(anchors, " | ")
			block += "\n  Evidence: " + anchorText
			used += len(anchorText) + 4
		}
		blocks = append(blocks, block)
		if used >= scaClaimsContextMax {
			break
		}
	}
	if len(blocks) == 0 {
		return "(no claim drafts)"
	}
	return strings.Join(blocks, "\n\n")
}

// renderOverallDraft mirrors Python _render_overall_draft: assemble the
// problem-level "rough draft" by concatenating each claim's report, so the SCA
// can judge whether the context lets the model answer end-to-end — including
// cross-claim synthesis that per-claim review would miss.
func renderOverallDraft(claims []ClaimDraft) string {
	if len(claims) == 0 {
		return "(no overall draft)"
	}
	var parts []string
	for _, c := range claims {
		if d := strings.TrimSpace(c.Draft); d != "" {
			parts = append(parts, fmt.Sprintf("[Claim %s] %s", c.ID, d))
		}
	}
	if len(parts) == 0 {
		return "(no overall draft)"
	}
	draft := strings.Join(parts, "\n")
	if len(draft) > scaClaimsContextMax {
		draft = draft[:scaClaimsContextMax]
	}
	return draft
}

// scaChunkText mirrors Python sufficient_context.py:_render_claim_context: prefer the reranked
// "content_with_weight" over the raw "content", then fall back to "chunk". This
// differs from harness.chunkText (which prefers content and keeps a Go-only
// "text" fallback): the SCA evidence anchor must read the SAME text Python did
// so the groundedness guard resolves the same snippets.
func scaChunkText(c map[string]any) string {
	if v, ok := c["content_with_weight"]; ok && v != nil {
		if s := fmt.Sprint(v); s != "" {
			return s
		}
	}
	if v, ok := c["content"]; ok && v != nil {
		if s := fmt.Sprint(v); s != "" {
			return s
		}
	}
	if v, ok := c["chunk"]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// boundedExcerpt mirrors Python _bounded_excerpt: a bounded window around a
// term from the draft. Table text is returned whole.
func boundedExcerpt(text, hints string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if isTableText(text) {
		return text
	}
	if maxChars < 80 {
		maxChars = 80
	}
	// Work in runes (code points), exactly like Python's str[:] / len() / find():
	// a byte-based slice would mis-count and split a multibyte (e.g. CJK) rune,
	// emitting invalid UTF-8.
	r := []rune(text)
	lower := strings.ToLower(text)
	start := -1
	for _, tok := range reHintToken.FindAllString(hints, -1) {
		if pos := strings.Index(lower, strings.ToLower(tok)); pos >= 0 {
			// pos is a byte offset in text; convert to a rune index.
			start = len([]rune(text[:pos]))
			break
		}
	}
	n := len(r)
	if start < 0 {
		if n <= maxChars {
			return text
		}
		tail := maxChars / 2
		return string(r[:maxChars-tail]) + " … " + string(r[n-tail:])
	}
	half := maxChars / 2
	left := max(0, start-half)
	right := min(n, left+maxChars)
	left = max(0, right-maxChars)
	prefix, suffix := "", ""
	if left > 0 {
		prefix = "…"
	}
	if right < n {
		suffix = "…"
	}
	return prefix + string(r[left:right]) + suffix
}

// isTableText mirrors Python _is_table_text: a corpus-neutral table detector —
// HTML table markup, or >=3 pipe rows.
func isTableText(text string) bool {
	t := strings.ToLower(text)
	if strings.Contains(t, "<table") || strings.Contains(t, "<tr") {
		return true
	}
	pipeRows := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.Count(line, "|") >= 2 {
			pipeRows++
		}
	}
	return pipeRows >= 3
}

// ---------------------------------------------------------------------------
// Coercion helpers
// ---------------------------------------------------------------------------

// coerceDict mirrors Python _coerce_dict: tolerate model format drift. The
// reviewer occasionally replies with a bare array or a JSON string; previously
// any non-dict response was dropped, so the SCA produced NO signal on those
// rounds and replan/rewrite silently stopped.
func coerceDict(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case []any:
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				return m
			}
		}
		return nil
	case string:
		if parsed := harness.ExtractJSON(t); parsed != nil {
			return coerceDict(parsed)
		}
		return nil
	}
	return nil
}

// clamp mirrors Python _clamp: coerce to [0,1], defaulting to 1.0 on failure.
func clamp(v any) float64 {
	f, ok := toFloat(v)
	if !ok {
		return 1.0
	}
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// truthy mirrors Python's builtin bool() on the raw JSON value, exactly as the
// Python sufficient_context path evaluates these fields (e.g.
// bool(item.get("grounded")), bool(result.get("is_sufficient"))). Python's bool()
// treats ANY non-empty string as truthy — including the literal "false" or "0" —
// because they are non-empty. We must not string-parse booleans here, or a model
// that returns grounded:"false" as a string would wrongly collapse to False while
// Python keeps it True. Only an empty string / 0 / nil / False is falsy.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case nil:
		return false
	}
	return true
}

func parseMissingInformation(v any) []MissingPiece {
	var out []MissingPiece
	for _, m := range asSlice(v) {
		if mm, ok := m.(map[string]any); ok {
			what := strings.TrimSpace(fmt.Sprint(mm["what"]))
			hint := strings.TrimSpace(fmt.Sprint(mm["search_hint"]))
			if what != "" || hint != "" {
				out = append(out, MissingPiece{What: what, SearchHint: hint})
			}
		} else if s := strings.TrimSpace(fmt.Sprint(m)); s != "" {
			out = append(out, MissingPiece{What: s})
		}
	}
	return out
}

func stringList(v any) []string {
	var out []string
	for _, item := range asSlice(v) {
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func asSlice(v any) []any {
	list, _ := v.([]any)
	return list
}

func firstNonEmpty(v ...any) string {
	for _, item := range v {
		if item == nil {
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
			return s
		}
	}
	return ""
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		return harness.ParseFloat(n)
	}
	return 0, false
}
