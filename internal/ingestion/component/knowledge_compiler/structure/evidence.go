package structure

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Evidence-gate mode: what happens to a payload whose evidence quotes all
// failed to locate. Mirrors Python _EVIDENCE_GATE_MODES /
// _struct_evidence_gate_mode.
const (
	EvidenceGateSoft = "soft"
	EvidenceGateHard = "hard"

	evidenceGateDefault = EvidenceGateSoft
)

// EvidenceGateMode maps the template config's evidence_gate_mode onto the
// supported modes, defaulting to soft for anything unrecognised (mirrors
// Python _struct_evidence_gate_mode).
func EvidenceGateMode(parserConfig map[string]any) string {
	if parserConfig == nil {
		return evidenceGateDefault
	}
	mode, _ := parserConfig["evidence_gate_mode"].(string)
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == EvidenceGateSoft || mode == EvidenceGateHard {
		return mode
	}
	return evidenceGateDefault
}

// outputFieldNames normalises a section's "output_fields" to the keys it
// declares, accepting both spellings: a bare string, and the object the prompt
// renderer requires (it also needs "shape" and "description").
//
// Reading only the string form is not a style preference — renderTypeFields
// skips a bare string outright, so an object is the only spelling that reaches
// the model. A reader that accepted only strings could therefore never agree
// with what the prompt actually asked for.
func outputFieldNames(cfg map[string]any) []string {
	names := []string{}
	for _, raw := range configOutputFields(cfg) {
		switch f := raw.(type) {
		case string:
			if n := strings.TrimSpace(f); n != "" {
				names = append(names, n)
			}
		case map[string]any:
			if n := strings.TrimSpace(firstStringOf(f["name"])); n != "" {
				names = append(names, n)
			}
		}
	}
	return names
}

// sectionExpectsEvidence reports whether a template section ("entity" or
// "relation") declares an output field named evidence. The entity stage is
// validated unconditionally, so its declared flag is only ever read to explain
// in the progress log which side of the prompt asked for quotes.
func sectionExpectsEvidence(parserConfig map[string]any, section string) bool {
	if parserConfig == nil {
		return false
	}
	fields, _ := common.Get(parserConfig, section).(map[string]any)
	if fields == nil {
		return false
	}
	for _, name := range outputFieldNames(fields) {
		if strings.EqualFold(name, "evidence") {
			return true
		}
	}
	return false
}

// RelationExpectsEvidence reports whether the template asks relation payloads
// for evidence ("relation.output_fields" contains "evidence"). Mirrors Python
// _struct_relation_expects_evidence: gating relations unconditionally would
// strip the field from templates that never asked for it.
func RelationExpectsEvidence(parserConfig map[string]any) bool {
	return sectionExpectsEvidence(parserConfig, "relation")
}

// EntityExpectsEvidence reports whether the template asks entity payloads for
// evidence. The entity stage is validated either way; naming the template's
// intent is what lets the progress log separate "the prompt never asked" from
// "the quotes did not survive".
func EntityExpectsEvidence(parserConfig map[string]any) bool {
	return sectionExpectsEvidence(parserConfig, "entity")
}

// batchTextByID maps chunk id → chunk text for the gate, using the same id
// synthesis as PackBatch (an empty id becomes "chunk-<n>"). Keep the two in
// sync: the gate validates quotes against exactly the text the prompt showed.
func batchTextByID(batch []common.Chunk) map[string]string {
	out := make(map[string]string, len(batch))
	for i, c := range batch {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			id = fmt.Sprintf("chunk-%d", i+1)
		}
		text := common.FirstNonEmpty(c.Text, c.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out[id] = text
	}
	return out
}

// locateEvidence finds quote in text using whitespace-normalized matching and
// returns offsets against the ORIGINAL text so the span can be sliced back
// out. The offset map is byte-aligned (mirrors tree/claims.go LocateEvidence
// and Python _struct_locate_evidence): a rune-indexed map misaligned every
// lookup past the first multi-byte rune, so CJK quotes resolved to the wrong
// span.
func locateEvidence(quote, text string) (int, int, bool) {
	if quote == "" || text == "" {
		return 0, 0, false
	}
	nText, idxMap := normalizeEvidenceText(text)
	nQuote, _ := normalizeEvidenceText(quote)
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
	end := len(text)
	if endNorm+1 < len(idxMap) {
		end = idxMap[endNorm+1]
	}
	return idxMap[pos], end, true
}

// normalizeEvidenceText collapses whitespace runs, returning the normalized
// text and a byte-aligned map: idxMap[j] is the offset in the original text of
// byte j of the normalized text.
func normalizeEvidenceText(text string) (string, []int) {
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
		// Copy the rune byte for byte and record one entry per byte, so the
		// map stays byte-aligned with the normalized string even for
		// multi-byte runes (CJK, emoji, accented characters).
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

// ValidatePayloadEvidence locates every evidence quote in the chunk it cites
// and drops the ones that cannot be found, mirroring Python
// _struct_apply_evidence_gate. mode decides whether a payload left with no
// verified evidence survives (soft: evidence dropped, payload kept) or is
// dropped (hard).
//
// Returns the surviving payloads plus (verified, rejected) counts. Payloads
// are filtered in place where possible; callers must use the returned slice.
func ValidatePayloadEvidence(
	payloads []map[string]any,
	textByID map[string]string,
	mode string,
) ([]map[string]any, int, int) {
	survivors, verified, rejected, _ := validatePayloadEvidence(payloads, textByID, mode)
	return survivors, verified, rejected
}

// validatePayloadEvidence is ValidatePayloadEvidence plus the samples of the
// quotes it rejected. Sampling is deliberately not behind a flag: the counts
// alone cannot say why a quote failed, and that question has to be answerable
// from an ordinary run — a diagnostic that only works when someone remembers to
// start the ingestor with an extra option is a diagnostic nobody uses. It stays
// affordable because a stage samples at most evidenceSampleLimit rejections and
// stops scanning once it reaches that.
func validatePayloadEvidence(
	payloads []map[string]any,
	textByID map[string]string,
	mode string,
) ([]map[string]any, int, int, []EvidenceRejectSample) {
	verified, rejected := 0, 0
	var samples []EvidenceRejectSample
	if mode != EvidenceGateSoft && mode != EvidenceGateHard {
		mode = evidenceGateDefault
	}
	if len(textByID) == 0 {
		return payloads, verified, rejected, nil
	}

	survivors := make([]map[string]any, 0, len(payloads))
	for _, payload := range payloads {
		rawEvidence, _ := payload["evidence"].([]any)
		if len(rawEvidence) == 0 {
			survivors = append(survivors, payload)
			continue
		}

		kept := make([]any, 0, len(rawEvidence))
		for _, entryAny := range rawEvidence {
			entry, ok := entryAny.(map[string]any)
			if !ok {
				continue
			}
			quote, _ := entry["quote"].(string)
			if strings.TrimSpace(quote) == "" {
				continue
			}
			// Prefer the cited chunk; fall back to scanning the batch. The
			// fallback candidates are sorted because map iteration order is
			// random — with two chunks holding the same sentence the gate must
			// pick the same one on every run.
			cited, _ := entry["chunk_id"].(string)
			candidates := []string{}
			if cited != "" && textByID[cited] != "" {
				candidates = append(candidates, cited)
			} else {
				for id := range textByID {
					candidates = append(candidates, id)
				}
				sort.Strings(candidates)
			}
			located := false
			for _, id := range candidates {
				start, end, ok := locateEvidence(quote, textByID[id])
				if !ok {
					continue
				}
				kept = append(kept, map[string]any{
					"quote":    quote,
					"chunk_id": id,
					"start":    start,
					"end":      end,
				})
				located = true
				break
			}
			if !located {
				samples = appendEvidenceSample(samples, quote, cited, textByID)
			}
		}

		if len(kept) > 0 {
			verified += len(kept)
			payload["evidence"] = kept
			survivors = append(survivors, payload)
			continue
		}
		rejected++
		delete(payload, "evidence")
		if mode == EvidenceGateSoft {
			survivors = append(survivors, payload)
		}
	}
	return survivors, verified, rejected, samples
}

// EvidenceGateStats is what one evidence-gate pass observed for one stage
// (entity or relation) of one batch. The gate returns only the survivors, so
// without these counts a template whose model emitted no quotes at all looks
// exactly like one whose quotes were every one rejected — soft mode drops the
// field without a trace.
type EvidenceGateStats struct {
	// Payloads is how many payloads the stage produced.
	Payloads int
	// WithEvidence and QuotesIn are what arrived: payloads carrying a non-empty
	// evidence list, and the quotes across them.
	WithEvidence int
	QuotesIn     int
	// QuotesKept are the quotes located in the batch's source text.
	QuotesKept int
	// PayloadsLost are the payloads left with no verified quote: soft mode keeps
	// them without the field, hard mode drops them.
	PayloadsLost int
	// Validated is false when there was no source text to validate against, so
	// the counts describe what arrived and nothing was removed.
	Validated bool
	// Samples describes a bounded number of the rejected quotes. The counts
	// above cannot say WHY a quote failed: a mis-cited chunk and a paraphrased
	// sentence are identical in every number.
	Samples []EvidenceRejectSample
}

// EvidenceRejectSample is one quote the gate could not locate, carrying the one
// fact that separates the two ways that happens.
type EvidenceRejectSample struct {
	// Quote is the quote flattened onto one line and truncated for the log;
	// QuoteLen is its full length in runes, so truncation cannot pass off an
	// over-long quote as a short one.
	Quote    string
	QuoteLen int
	// Cited is the chunk id the model named, CitedKnown whether that id was part
	// of the batch at all.
	Cited      string
	CitedKnown bool
	// Elsewhere names a chunk the quote DOES exist in when it is not the cited
	// one. Non-empty means the text is in the batch and the model mis-cited the
	// chunk; empty means the batch does not contain that text at all, so the
	// quote was paraphrased, truncated or stitched together.
	Elsewhere string
	// Chunks is how many chunks the batch held, which bounds what an empty
	// Elsewhere can mean.
	Chunks int
}

// evidenceSampleLimit bounds the samples one stage of one document keeps, so a
// document with many batches still reports as one readable message.
const evidenceSampleLimit = 20

// Merge folds one batch's stats into a stage's running total.
func (s EvidenceGateStats) Merge(o EvidenceGateStats) EvidenceGateStats {
	return EvidenceGateStats{
		Payloads:     s.Payloads + o.Payloads,
		WithEvidence: s.WithEvidence + o.WithEvidence,
		QuotesIn:     s.QuotesIn + o.QuotesIn,
		QuotesKept:   s.QuotesKept + o.QuotesKept,
		PayloadsLost: s.PayloadsLost + o.PayloadsLost,
		Validated:    s.Validated || o.Validated,
		Samples:      mergeEvidenceSamples(s.Samples, o.Samples),
	}
}

// mergeEvidenceSamples concatenates two sample lists under the shared limit.
func mergeEvidenceSamples(a, b []EvidenceRejectSample) []EvidenceRejectSample {
	if len(a) >= evidenceSampleLimit {
		return a
	}
	if room := evidenceSampleLimit - len(a); len(b) > room {
		b = b[:room]
	}
	out := make([]EvidenceRejectSample, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}

// Summary renders the counts for a progress message. It leads with the ratio
// that answers the question the survivors cannot: quotes in versus payloads
// carrying them.
func (s EvidenceGateStats) Summary() string {
	if !s.Validated {
		return fmt.Sprintf("carried %d/%d payload(s), %d quote(s) in, not validated (no source text)",
			s.WithEvidence, s.Payloads, s.QuotesIn)
	}
	return fmt.Sprintf("carried %d/%d payload(s), %d quote(s) in, %d kept, %d payload(s) with none verified",
		s.WithEvidence, s.Payloads, s.QuotesIn, s.QuotesKept, s.PayloadsLost)
}

// SampleDetail renders the sampled rejections one per line, or "" when nothing
// was rejected. The "elsewhere" column is the point of the whole thing: it is
// the only view that distinguishes text the gate threw away although it was
// there (the model named the wrong chunk) from text that was never there.
func (s EvidenceGateStats) SampleDetail() string {
	if len(s.Samples) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d quote(s) could not be located, %d sampled (elsewhere=<chunk> = the text IS in the batch but not in the chunk the model cited; elsewhere=- = the batch does not contain the text):",
		s.PayloadsLost, len(s.Samples))
	for i, sample := range s.Samples {
		fmt.Fprintf(&b, "\n  %d. cited=%s elsewhere=%s chars=%d chunks=%d :: %s",
			i+1, sampleCitation(sample), sampleElsewhere(sample), sample.QuoteLen, sample.Chunks, sample.Quote)
	}
	return b.String()
}

func sampleCitation(s EvidenceRejectSample) string {
	switch {
	case s.Cited == "":
		return "(none)"
	case s.CitedKnown:
		return s.Cited
	default:
		return s.Cited + " (not a batch chunk)"
	}
}

func sampleElsewhere(s EvidenceRejectSample) string {
	if s.Elsewhere == "" {
		return "-"
	}
	return s.Elsewhere
}

// appendEvidenceSample records one rejected quote under the shared limit.
func appendEvidenceSample(samples []EvidenceRejectSample, quote, cited string, textByID map[string]string) []EvidenceRejectSample {
	if len(samples) >= evidenceSampleLimit {
		return samples
	}
	return append(samples, EvidenceRejectSample{
		Quote:      flattenEvidenceQuote(quote),
		QuoteLen:   utf8.RuneCountInString(quote),
		Cited:      cited,
		CitedKnown: cited != "" && textByID[cited] != "",
		Elsewhere:  evidenceElsewhere(quote, cited, textByID),
		Chunks:     len(textByID),
	})
}

// evidenceElsewhere reports a chunk other than the cited one that holds the
// quote, or "" when none does. Candidates are sorted so the answer stays stable
// across runs when two chunks hold the same sentence.
func evidenceElsewhere(quote, cited string, textByID map[string]string) string {
	ids := make([]string, 0, len(textByID))
	for id := range textByID {
		if id != cited {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, _, ok := locateEvidence(quote, textByID[id]); ok {
			return id
		}
	}
	return ""
}

// flattenEvidenceQuote puts a quote on one line and truncates it for the log.
// QuoteLen keeps the true length, so truncation cannot pass off an over-long
// quote as a short one.
func flattenEvidenceQuote(quote string) string {
	const limit = 160
	flat := strings.Join(strings.Fields(quote), " ")
	runes := []rune(flat)
	if len(runes) <= limit {
		return flat
	}
	return string(runes[:limit]) + "…"
}

// countEvidence returns the payloads carrying a non-empty evidence list and the
// quotes across them — the input side the gate's own counts cannot report.
func countEvidence(payloads []map[string]any) (payloadsWithEvidence, quotes int) {
	for _, payload := range payloads {
		entries, _ := payload["evidence"].([]any)
		if len(entries) == 0 {
			continue
		}
		payloadsWithEvidence++
		quotes += len(entries)
	}
	return payloadsWithEvidence, quotes
}

// EvidenceArrived reports what a stage produced without validating anything. Use
// it for a stage the template does not ask to carry evidence: the counts still
// say whether the model emitted quotes, which is the only way that stage's
// absence of evidence can be attributed.
func EvidenceArrived(payloads []map[string]any) EvidenceGateStats {
	stats := EvidenceGateStats{Payloads: len(payloads)}
	stats.WithEvidence, stats.QuotesIn = countEvidence(payloads)
	return stats
}

// GateEvidence validates one stage's payloads against the batch text and reports
// what the gate did, so a caller can tell "the model emitted no quotes" from
// "every quote failed to locate" — and, through the attached samples, why a
// quote that did arrive could not be located. An empty textByID leaves the
// payloads untouched (ValidatePayloadEvidence's own no-op) and the stats
// unvalidated.
func GateEvidence(payloads []map[string]any, textByID map[string]string, mode string) ([]map[string]any, EvidenceGateStats) {
	stats := EvidenceArrived(payloads)
	stats.Validated = len(textByID) > 0
	survivors, verified, rejected, samples := validatePayloadEvidence(payloads, textByID, mode)
	stats.QuotesKept = verified
	stats.PayloadsLost = rejected
	stats.Samples = samples
	return survivors, stats
}
