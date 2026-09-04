package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// claimExtractionWorkers bounds how many claim-extraction LLM calls run at once.
// Batches are independent, so a large document would otherwise open one
// round-trip per batch and trip the provider's rate limit; this keeps the
// fan-out predictable and mirrors the Python side's _claim_limiter default.
const claimExtractionWorkers = 4

// claimBatchSize is how many chunks one extraction call covers, mirroring Python
// _CLAIM_BATCH_SIZE (raptor.py). Every chunk in the batch is a TARGET and the
// model attributes each claim by chunk id.
const claimBatchSize = 4

// Retry budget for a transient LLM failure (rate limit, 5xx, timeout), mirroring
// Python raptor._extract_claim_for_chunk: 3 attempts, 2s then 4s of back-off.
const claimMaxAttempts = 3

// claimRetryBaseDelay is a var (not a const) purely so tests can shorten the
// wait; production always uses the 2s base.
var claimRetryBaseDelay = 2 * time.Second

// EvidenceGateMode decides what happens to a claim whose evidence all failed to
// locate. Mirrors Python _EVIDENCE_GATE_MODES.
type EvidenceGateMode string

const (
	// EvidenceGateSoft keeps the claim with its evidence dropped — the default,
	// because enumeration / multi-hop questions degrade badly when facts are
	// discarded (see claim_evidence.md §4.2).
	EvidenceGateSoft EvidenceGateMode = "soft"
	// EvidenceGateHard drops the claim along with its evidence, the behaviour
	// the ISC paper validated on single-document dialogue QA.
	EvidenceGateHard EvidenceGateMode = "hard"
)

// evidenceGateDefault is used when the configured mode is absent or unknown.
const evidenceGateDefault = EvidenceGateSoft

// ParseEvidenceGateMode maps a configured gate mode onto EvidenceGateMode,
// falling back to the default for anything unrecognised (mirrors Python
// _struct_evidence_gate_mode).
func ParseEvidenceGateMode(raw any) EvidenceGateMode {
	s, ok := raw.(string)
	if !ok {
		return evidenceGateDefault
	}
	switch EvidenceGateMode(strings.ToLower(strings.TrimSpace(s))) {
	case EvidenceGateSoft:
		return EvidenceGateSoft
	case EvidenceGateHard:
		return EvidenceGateHard
	}
	return evidenceGateDefault
}

// claimExtractionPrompt mirrors the Python prompt in
// rag/advanced_rag/knowlege_compile/raptor.py (_CLAIM_EXTRACTION_PROMPT). The
// two must stay in sync: a divergence here silently changes what the tree is
// summarised from, which then makes Python-derived and Go-derived trees
// incomparable.
const claimExtractionPrompt = `## Task
You are a high-recall claim harvester for the TARGET chunks below. For EACH
TARGET chunk, extract EVERY explicit atomic claim that chunk supports: factual
assertions AND explicitly expressed opinions, beliefs, judgments, assessments,
recommendations, preferences, intentions, predictions, and hypotheses. Do not
rank claims, summarize, merge claims, or omit a claim because it seems minor.

A claim must be:
- Self-contained: readable without the surrounding text, with named subjects.
  Never write "the article", "the document", "it says", or a bare pronoun.
  Resolve the referent to its name when attribution makes the claim clearer.
- Faithful: stated only if that TARGET chunk supports it. Never invent,
  strengthen, or infer. Preserve modality, uncertainty, negation, and
  attribution exactly.
- Atomic: exactly one fact. Split compound sentences.

## Response Format
Reply with a single JSON object: {"items": [{"type": "claim", "name": "<the claim, one sentence>", "description": "<optional restatement for retrieval>", "source_chunk_ids": ["<CHUNK_ID it was taken from>"], "evidence": [{"quote": "<the verbatim source sentence>", "chunk_id": "<CHUNK_ID it was taken from>"}]}, ...]}.

Rules:
- ` + "`evidence.quote`" + ` MUST be a CONTIGUOUS verbatim substring of the chunk cited
  by ` + "`evidence.chunk_id`" + `: same words, same order, no paraphrase, no truncation,
  no added words. A quote that cannot be found verbatim is rejected downstream,
  so never restate.
- ` + "`evidence.chunk_id`" + ` and ` + "`source_chunk_ids`" + ` MUST identify the exact chunk the
  claim and quote came from. Never cross-attribute a quote to a different chunk.
- Keep each quote concise and under 240 characters.
- For tables, infoboxes and bullet lists, the quote MUST be the raw cell/row
  text exactly as it appears — keep its separators and order. Do NOT turn a
  table row like "Starring | Penn Badgley | Elizabeth Lail" into a sentence
  like "Starring Penn Badgley Elizabeth Lail"; that is a paraphrase and will
  be rejected.
- Preserve numbers, units, dates, names and qualifiers exactly.
- Distribute claims across all TARGET chunks; do not skip a chunk because you
  reached the cap — the cap is per chunk, not per batch.
- If a TARGET chunk contains no extractable assertion, emit no claim for it.
- Keep claims in the same language as the source.
Return JSON only, no commentary.`

// claimEntry is one (chunk id, text) pair in a claim-extraction batch.
type claimEntry struct {
	id   string
	text string
}

// renderClaimSource builds the user prompt body for one claim-extraction call.
// Every chunk in the batch is a TARGET, so the model harvests claims for all of
// them in one call. Each chunk carries its own id, which both pins the claim
// attribution and lets the validation gate check the quote against the right
// text (mirrors Python _render_claim_source).
func renderClaimSource(targets []claimEntry) string {
	var b strings.Builder
	b.WriteString("## Source Text\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "[CHUNK_ID: %s (TARGET)]\n%s\n[END_CHUNK]\n", t.id, t.text)
	}
	b.WriteString("\n## Output (JSON only):")
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
// Chunks are packed into batches of claimBatchSize (mirroring Python
// _CLAIM_BATCH_SIZE) and each batch is harvested by one LLM call. Every chunk in
// a batch is a TARGET and the model reports which chunk each claim came from
// through source_chunk_ids; the validation gate then checks each quote against
// that chunk's text.
//
// The calls are independent, so a bounded worker pool runs several at once —
// mirroring the Python side, which fans every batch out under _claim_limiter
// (default 4) instead of extracting serially. claimExtractionWorkers caps
// in-flight work so a large document cannot open hundreds of concurrent
// round-trips against the provider's rate limit.
//
// Extraction is best-effort: a failing batch is skipped and simply contributes
// no claims, which makes the tree fall back to raw text for it. A missing LLM
// client disables extraction entirely.
func ExtractClaimsForChunks(ctx context.Context, deps common.Deps, llmID string, chunks []common.Chunk, mode EvidenceGateMode) map[string][]Claim {
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

	batches := packClaimBatches(entries)
	claimsByChunk := make(map[string][]Claim)
	total := len(entries)
	workers := claimExtractionWorkers
	if workers > len(batches) {
		workers = len(batches)
	}

	var (
		// mu guards claimsByChunk and serialises the progress callback, which
		// is supplied by the caller and is not required to be goroutine-safe.
		mu      sync.Mutex
		wg      sync.WaitGroup
		started int
	)
	sem := make(chan struct{}, workers)
	for i := range batches {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Take a slot before doing any work; if the context is already
			// done, drop out instead of queueing a call that would be
			// cancelled mid-flight.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			batch := batches[i]

			mu.Lock()
			started += len(batch)
			// Report progress *before* the LLM call so the UI moves during the
			// extraction loop instead of freezing at "building tree". Progress
			// counts chunks (the user-visible unit), not batches, and workers
			// finish out of order, so this is a started count.
			runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf("tree-template: extracting claims for chunk %d/%d", started, total))
			mu.Unlock()

			batchClaims := extractClaimsForBatch(ctx, deps, llmID, batch, mode)
			if len(batchClaims) == 0 {
				return
			}

			mu.Lock()
			defer mu.Unlock()
			for id, claims := range batchClaims {
				claimsByChunk[id] = append(claimsByChunk[id], claims...)
			}
		}(i)
	}
	wg.Wait()
	if len(claimsByChunk) == 0 {
		return nil
	}
	return claimsByChunk
}

// extractClaimsForBatch runs one claim-extraction call for a batch of chunks and
// returns the surviving claims keyed by the chunk they came from.
//
// It is safe for concurrent use: all state is local, and the caller owns the map
// the results are merged into. A batch that fails after its retries, yields no
// parseable items, or yields nothing attributable contributes no claims, which
// makes the tree fall back to raw text for it.
func extractClaimsForBatch(ctx context.Context, deps common.Deps, llmID string, batch []claimEntry, mode EvidenceGateMode) map[string][]Claim {
	batchIDs := make(map[string]bool, len(batch))
	textByID := make(map[string]string, len(batch))
	for _, t := range batch {
		batchIDs[t.id] = true
		textByID[t.id] = t.text
	}

	raw, err := chatWithClaimRetry(ctx, deps, llmID, renderClaimSource(batch), batch)
	if err != nil {
		log.Printf("tree: claim extraction skipped for batch %s: %v", claimBatchLabel(batch), err)
		return nil
	}

	items := parseClaimItems(raw)
	if len(items) == 0 {
		return nil
	}

	claims := make([]Claim, 0, len(items))
	for _, it := range items {
		it.Type = "claim"
		// Keep only ids that are actually in this batch: a hallucinated id
		// points at text the model never saw, so its quote could never be
		// validated. Missing attribution is recovered from the verified
		// evidence below rather than guessed here.
		src := make([]string, 0, len(it.SourceChunkIDs))
		for _, id := range it.SourceChunkIDs {
			if batchIDs[id] {
				src = append(src, id)
			}
		}
		it.SourceChunkIDs = src
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
	// Validate only against this batch's text: a gate that could scan the whole
	// document would let a quote match an unrelated chunk that merely happens to
	// contain the same sentence, and would then attribute the claim to it.
	claims, verified, rejected := ValidateClaims(claims, textByID, mode)
	log.Printf("tree: claim extraction batch=%s chunks=%d claims=%d emitted_evidence=%d verified=%d rejected=%d",
		claimBatchLabel(batch), len(batch), len(claims), emitted, verified, rejected)

	// Recover attribution from the evidence that survived the gate: the chunk a
	// quote was located in IS where the claim came from. A claim that neither the
	// model nor its evidence can place is dropped — filing it under an arbitrary
	// chunk would cite text it never came from (mirrors Python, which drops the
	// claim instead of defaulting to the batch's first chunk).
	out := make(map[string][]Claim)
	for _, c := range claims {
		id := ""
		if len(c.SourceChunkIDs) > 0 {
			id = c.SourceChunkIDs[0]
		} else {
			for _, e := range c.Evidence {
				if batchIDs[e.ChunkID] {
					id = e.ChunkID
					break
				}
			}
		}
		if id == "" {
			log.Printf("tree: dropped claim with no attributable source: %s", c.Name)
			continue
		}
		c.SourceChunkIDs = []string{id}
		out[id] = append(out[id], c)
	}
	return out
}

// packClaimBatches groups chunks into fixed-size batches, mirroring Python
// _pack_claim_batches / _CLAIM_BATCH_SIZE.
func packClaimBatches(entries []claimEntry) [][]claimEntry {
	bs := claimBatchSize
	if bs < 1 {
		bs = 1
	}
	out := make([][]claimEntry, 0, (len(entries)+bs-1)/bs)
	for i := 0; i < len(entries); i += bs {
		end := i + bs
		if end > len(entries) {
			end = len(entries)
		}
		out = append(out, entries[i:end])
	}
	return out
}

// claimBatchLabel renders a batch's chunk ids for logs.
func claimBatchLabel(batch []claimEntry) string {
	ids := make([]string, 0, len(batch))
	for _, t := range batch {
		ids = append(ids, t.id)
	}
	return strings.Join(ids, ",")
}

// chatWithClaimRetry issues one claim-extraction call, retrying transient
// provider failures (rate limits, 5xx, timeouts) with exponential back-off,
// mirroring Python raptor._extract_claim_for_chunk. A cancelled context is never
// retried.
func chatWithClaimRetry(ctx context.Context, deps common.Deps, llmID, prompt string, batch []claimEntry) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= claimMaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		resp, err := deps.Chat.Chat(ctx, common.ChatRequest{
			LLMID:           llmID,
			SystemPrompt:    claimExtractionPrompt,
			UserPrompt:      prompt,
			JSONMode:        true,
			DisableThinking: true,
		})
		if err == nil {
			if resp == nil {
				return "", nil
			}
			return resp.Content, nil
		}
		lastErr = err
		if !isRetryableClaimErr(err) || attempt == claimMaxAttempts {
			break
		}
		delay := claimRetryBaseDelay * time.Duration(1<<(attempt-1))
		log.Printf("tree: claim extraction retry %d/%d after %s for batch %s: %v",
			attempt, claimMaxAttempts, delay, claimBatchLabel(batch), err)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return "", lastErr
}

// isRetryableClaimErr mirrors Python _RETRYABLE_LLM_ERR: capacity and transport
// failures that back-off can clear, as opposed to a malformed request that no
// amount of waiting would fix.
func isRetryableClaimErr(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"rate limit", "429", "tpm limit", "too many requests", "requests per minute",
		"server", "503", "502", "504", "500", "unavailable", "timeout", "timed out",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
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
// that cannot be found. mode decides what happens to a claim left with no
// verified evidence: soft keeps it with the evidence removed (discarding the
// claim too would silently lose facts, which hurts enumeration-style
// questions), hard drops the claim.
//
// Returns the surviving claims plus (verified, rejected) counts for the gate
// log. Mirrors Python _struct_apply_evidence_gate.
func ValidateClaims(claims []Claim, textByID map[string]string, mode EvidenceGateMode) ([]Claim, int, int) {
	verified, rejected := 0, 0
	if mode != EvidenceGateSoft && mode != EvidenceGateHard {
		mode = evidenceGateDefault
	}
	if len(textByID) == 0 {
		return claims, verified, rejected
	}
	survivors := make([]Claim, 0, len(claims))
	for i := range claims {
		c := &claims[i]
		if len(c.Evidence) == 0 {
			survivors = append(survivors, *c)
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
				candidates = make([]string, 0, len(textByID))
				for id := range textByID {
					candidates = append(candidates, id)
				}
				// Map iteration order is random, so sort: with two chunks
				// holding the same sentence the gate must pick the same one on
				// every run, otherwise a recompile repoints the quote.
				sort.Strings(candidates)
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
			survivors = append(survivors, *c)
			continue
		}
		rejected++
		c.Evidence = nil
		if mode == EvidenceGateSoft {
			survivors = append(survivors, *c)
		}
	}
	return survivors, verified, rejected
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
	if endNorm >= len(idxMap) {
		return 0, 0, false
	}
	// End is the original offset just past the match: the offset of the next
	// normalized byte, or the text length when the match runs to the end.
	// Adding 1 to the last matched byte instead would cut a multi-byte rune in
	// half whenever the match ends inside one.
	end := len(text)
	if endNorm+1 < len(idxMap) {
		end = idxMap[endNorm+1]
	}
	return idxMap[pos], end, true
}

// normalizeForMatch collapses whitespace runs, returning the normalized text
// and a byte-aligned map: idxMap[j] is the offset in the original text of byte
// j of the normalized text.
//
// The map is indexed with byte offsets, because that is what strings.Index
// returns. Recording one entry per rune misaligned every lookup as soon as the
// text held a multi-byte rune, so CJK quotes resolved to the wrong span — or
// failed to match at all.
func normalizeForMatch(text string) (string, []int) {
	var b strings.Builder
	b.Grow(len(text))
	idxMap := make([]int, 0, len(text))
	prevSpace := false
	for i := 0; i < len(text); {
		c := text[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				idxMap = append(idxMap, i)
			}
			prevSpace = true
			i++
			continue
		}
		// Copy the rune byte for byte and record one entry per byte, so idxMap
		// stays byte-aligned with the normalized string even for multi-byte
		// runes (CJK, emoji, accented characters).
		_, size := utf8.DecodeRuneInString(text[i:])
		if size <= 0 {
			size = 1
		}
		for k := 0; k < size; k++ {
			b.WriteByte(text[i+k])
			idxMap = append(idxMap, i+k)
		}
		i += size
		prevSpace = false
	}
	return b.String(), idxMap
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
					// Plain quotes, not %q: Go's %q escapes embedded quotes and
					// non-printables, which Python's f'- {name}\n  Evidence:
					// "{quote}"' does not — the two runtimes would feed the
					// abstraction layer different text for the same claim.
					lines = append(lines, fmt.Sprintf("- %s\n  Evidence: \"%s\"", name, c.Evidence[0].Quote))
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
