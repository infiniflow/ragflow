package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// claimExtractionPrompt mirrors the Python prompt in
// rag/advanced_rag/knowlege_compile/raptor.py (_CLAIM_EXTRACTION_PROMPT). The
// two must stay in sync: a divergence here silently changes what the tree is
// summarised from, which then makes Python-derived and Go-derived trees
// incomparable.
const claimExtractionPrompt = `## Task
You are a high-recall claim harvester for the TARGET chunk below. Extract EVERY
explicit atomic claim the TARGET supports: factual assertions AND explicitly
expressed opinions, beliefs, judgments, assessments, recommendations,
preferences, intentions, predictions, and hypotheses. Do not rank claims,
summarize, merge claims, or omit a claim because it seems minor.

A claim must be:
- Self-contained: readable without the surrounding text, with named subjects.
  Never write "the article", "the document", "it says", or a bare pronoun.
  Resolve the referent to its name when attribution makes the claim clearer.
- Faithful: stated only if the TARGET supports it. Never invent, strengthen,
  or infer. Preserve modality, uncertainty, negation, and attribution exactly.
- Atomic: exactly one fact. Split compound sentences.

CONTEXT chunks (if any) are provided only to resolve names and references;
never extract a claim from them and never quote from them.

## Response Format
Reply with a single JSON object: {"items": [{"type": "claim", "name": "<the claim, one sentence>", "description": "<optional restatement for retrieval>", "evidence": [{"quote": "<the verbatim source sentence supporting this claim>", "chunk_id": "TARGET_ID"}]}, ...]}.

Rules:
- ` + "`evidence.quote`" + ` MUST be a CONTIGUOUS verbatim substring of the TARGET chunk:
  same words, same order, no paraphrase, no truncation, no added words. A
  quote that cannot be found verbatim is rejected downstream, so never restate.
- Keep each quote concise and under 240 characters.
- For tables, infoboxes and bullet lists, the quote MUST be the raw cell/row
  text exactly as it appears — keep its separators and order. Do NOT turn a
  table row like "Starring | Penn Badgley | Elizabeth Lail" into a sentence
  like "Starring Penn Badgley Elizabeth Lail"; that is a paraphrase and will
  be rejected.
- ` + "`evidence.chunk_id`" + ` MUST be TARGET_ID, never a CONTEXT id.
- Preserve numbers, units, dates, names and qualifiers exactly.
- Return at most 30 claims; do not omit a claim for importance.
- If the TARGET contains no extractable assertion, return {"items": []}.
- Keep the claim in the same language as the source.
Return JSON only, no commentary.`

// claimEntry is one (chunk id, text) pair in a claim-extraction batch.
type claimEntry struct {
	id   string
	text string
}

// renderClaimSource builds the user prompt body for one claim-extraction call.
// The target chunk is marked so the model knows what to extract and quote from;
// context chunks (when any) are marked as reference-only.
func renderClaimSource(targetID, targetText string, context []claimEntry) string {
	var b strings.Builder
	b.WriteString("## Source Text\n")
	fmt.Fprintf(&b, "[CHUNK_ID: %s (TARGET)]\n%s\n[END_CHUNK]", targetID, targetText)
	for _, c := range context {
		fmt.Fprintf(&b, "\n\n[CHUNK_ID: %s (CONTEXT — reference only)]\n%s\n[END_CHUNK]", c.id, c.text)
	}
	b.WriteString("\n\n## Output (JSON only):")
	return b.String()
}

// Claim is one extracted claim with the verbatim evidence that backs it.
type Claim struct {
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	Type           string        `json:"type"`
	SourceChunkIDs []string      `json:"source_chunk_ids"`
	Evidence       []EvidenceRef `json:"evidence,omitempty"`
}

// EvidenceRef is a verbatim quote plus where it was located. Offsets are only
// known after validation, so a claim parsed from the model carries a zero span
// until ValidateClaims resolves it against the source text.
type EvidenceRef struct {
	Quote   string `json:"quote"`
	ChunkID string `json:"chunk_id"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

// ExtractClaimsForChunks extracts claim/evidence pairs for the layer-0 chunks
// about to be clustered.
//
// It returns claims keyed by the chunk they came from, so the tree builder can
// look up a cluster's claims by its member chunk ids and summarise from those
// instead of from truncated raw text (see buildClusterContent vs
// buildClaimContent).
//
// Each chunk is extracted with its own LLM call: source chunks average ~2000
// tokens, so packing several together made the model drop or mis-attribute
// claims. One chunk per call keeps every target under the model's attention and
// the quote check exact (mirrors the Python extract_claims_for_chunks).
//
// Extraction is best-effort: a failing chunk is skipped and simply contributes
// no claims, which makes the tree fall back to raw text for it. A missing LLM
// client disables extraction entirely.
func ExtractClaimsForChunks(ctx context.Context, deps common.Deps, llmID string, chunks []common.Chunk) map[string][]Claim {
	if deps.Chat == nil || len(chunks) == 0 {
		return nil
	}

	var entries []claimEntry
	for _, c := range chunks {
		if c.ID == "" || strings.TrimSpace(c.Text) == "" {
			continue
		}
		entries = append(entries, claimEntry{id: c.ID, text: c.Text})
	}
	if len(entries) == 0 {
		return nil
	}

	claimsByChunk := make(map[string][]Claim)
	total := len(entries)
	for i, target := range entries {
		// Report per-chunk progress *before* the LLM call so the UI moves during
		// the long extraction loop instead of freezing at "building tree". One
		// chunk per call keeps each target under the model's attention and the
		// quote check exact (mirrors the Python extract_claims_for_chunks).
		runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf("tree-template: extracting claims for chunk %d/%d", i+1, total))

		prompt := renderClaimSource(target.id, target.text, nil)

		resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
			LLMID:           llmID,
			SystemPrompt:    claimExtractionPrompt,
			UserPrompt:      prompt,
			JSONMode:        true,
			DisableThinking: true,
		})
		if err != nil {
			log.Printf("tree: claim extraction skipped for chunk %s: %v", target.id, err)
			continue
		}
		var raw string
		if resp != nil {
			raw = resp.Content
		}
		items := parseClaimItems(raw)
		if len(items) == 0 {
			continue
		}

		claims := make([]Claim, 0, len(items))
		for _, it := range items {
			it.Type = "claim"
			// One chunk per call, so the claim is always attributed to that
			// chunk — the chunk id is filled by code, never by the model (the
			// model only sees a system-internal id it would otherwise have to
			// guess). The evidence gate validates quotes against this chunk's
			// text, so attribution stays exact.
			it.SourceChunkIDs = []string{target.id}
			if strings.TrimSpace(it.Description) == "" {
				it.Description = it.Name
			}
			claims = append(claims, it)
		}

		emitted := 0
		for _, c := range claims {
			if len(c.Evidence) > 0 {
				emitted++
			}
		}
		// Validate only against the target's text — context chunks were never
		// supposed to be quoted.
		verified, rejected := ValidateClaims(claims, map[string]string{target.id: target.text})
		log.Printf("tree: claim extraction chunk=%s claims=%d emitted_evidence=%d verified=%d rejected=%d",
			target.id, len(claims), emitted, verified, rejected)
		claimsByChunk[target.id] = append(claimsByChunk[target.id], claims...)
	}
	if len(claimsByChunk) == 0 {
		return nil
	}
	return claimsByChunk
}

// parseClaimItems tolerates the model wrapping JSON in prose or fences.
func parseClaimItems(raw string) []Claim {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 && j+1 < len(s) {
		s = s[:j+1]
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")

	var out struct {
		Items []Claim `json:"items"`
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	items := make([]Claim, 0, len(out.Items))
	for _, it := range out.Items {
		if strings.TrimSpace(it.Name) == "" {
			continue
		}
		items = append(items, it)
	}
	return items
}

// ValidateClaims locates every quote in the chunk it cites and drops the ones
// that cannot be found. A claim whose evidence is all dropped is kept (soft
// mode) with the evidence removed — discarding the claim too would silently
// lose facts, which hurts enumeration-style questions.
//
// Returns (verified, rejected) counts for the gate log.
func ValidateClaims(claims []Claim, textByID map[string]string) (int, int) {
	verified, rejected := 0, 0
	if len(textByID) == 0 {
		return verified, rejected
	}
	for i := range claims {
		c := &claims[i]
		if len(c.Evidence) == 0 {
			continue
		}
		kept := make([]EvidenceRef, 0, len(c.Evidence))
		for _, e := range c.Evidence {
			if strings.TrimSpace(e.Quote) == "" {
				continue
			}
			// Prefer the cited chunk; fall back to scanning the batch when the
			// model cited nothing (or something outside it).
			candidates := []string{e.ChunkID}
			if textByID[e.ChunkID] == "" {
				candidates = nil
				for id := range textByID {
					candidates = append(candidates, id)
				}
			}
			for _, id := range candidates {
				start, end, ok := LocateEvidence(e.Quote, textByID[id])
				if !ok {
					continue
				}
				kept = append(kept, EvidenceRef{Quote: e.Quote, ChunkID: id, Start: start, End: end})
				break
			}
		}
		if len(kept) > 0 {
			verified += len(kept)
			c.Evidence = kept
			continue
		}
		rejected++
		c.Evidence = nil
	}
	return verified, rejected
}

// LocateEvidence finds quote in text using whitespace-normalized matching,
// returning offsets against the ORIGINAL text so the span can be sliced back
// out. Normalizing tolerates the reflowing a model introduces when it copies a
// sentence across a line break.
func LocateEvidence(quote, text string) (int, int, bool) {
	if quote == "" || text == "" {
		return 0, 0, false
	}
	nText, idxMap := normalizeForMatch(text)
	nQuote, _ := normalizeForMatch(quote)
	if nQuote == "" {
		return 0, 0, false
	}
	pos := strings.Index(nText, nQuote)
	if pos < 0 {
		return 0, 0, false
	}
	endNorm := pos + len(nQuote) - 1
	return idxMap[pos], idxMap[endNorm] + 1, true
}

// normalizeForMatch collapses whitespace runs, returning the normalized text and
// a map from each normalized index back to its index in the original text.
func normalizeForMatch(text string) (string, []int) {
	chars := make([]rune, 0, len(text))
	idxMap := make([]int, 0, len(text))
	prevSpace := false
	for i, ch := range text {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			if prevSpace {
				continue
			}
			chars = append(chars, ' ')
			idxMap = append(idxMap, i)
			prevSpace = true
			continue
		}
		chars = append(chars, ch)
		idxMap = append(idxMap, i)
		prevSpace = false
	}
	return string(chars), idxMap
}

// BuildClaimContent renders a cluster's member claims as the summary input.
//
// Each claim is rendered with the verbatim quote that backs it, so the
// abstraction above sees the facts AND their grounding rather than a reflowed
// paraphrase. Chunks without claims contribute their own text, so a cluster is
// never left empty just because extraction produced nothing.
func BuildClaimContent(texts []string, chunkIDs []string, pointIdxs []int, claimsByChunk map[string][]Claim) string {
	if len(claimsByChunk) == 0 {
		return ""
	}
	var parts []string
	for _, idx := range pointIdxs {
		if idx < 0 || idx >= len(chunkIDs) {
			continue
		}
		var rendered string
		if claims, ok := claimsByChunk[chunkIDs[idx]]; ok && len(claims) > 0 {
			var lines []string
			for _, c := range claims {
				name := strings.TrimSpace(c.Name)
				if name == "" {
					continue
				}
				if len(c.Evidence) > 0 && strings.TrimSpace(c.Evidence[0].Quote) != "" {
					lines = append(lines, fmt.Sprintf("- %s\n  Evidence: %q", name, c.Evidence[0].Quote))
				} else {
					lines = append(lines, "- "+name)
				}
			}
			rendered = strings.Join(lines, "\n")
		}
		if rendered == "" {
			if idx < len(texts) {
				rendered = texts[idx]
			}
		}
		if rendered != "" {
			parts = append(parts, rendered)
		}
	}
	return strings.Join(parts, "\n")
}
