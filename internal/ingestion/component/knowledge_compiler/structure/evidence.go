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

// RelationExpectsEvidence reports whether the template asks relation payloads
// for evidence ("relation.output_fields" contains "evidence"). Mirrors Python
// _struct_relation_expects_evidence: gating relations unconditionally would
// strip the field from templates that never asked for it.
func RelationExpectsEvidence(parserConfig map[string]any) bool {
	if parserConfig == nil {
		return false
	}
	relations, _ := common.Get(parserConfig, "relation").(map[string]any)
	if relations == nil {
		return false
	}
	for _, f := range configOutputFields(relations) {
		if s, ok := f.(string); ok && strings.EqualFold(strings.TrimSpace(s), "evidence") {
			return true
		}
	}
	return false
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
	verified, rejected := 0, 0
	if mode != EvidenceGateSoft && mode != EvidenceGateHard {
		mode = evidenceGateDefault
	}
	if len(textByID) == 0 {
		return payloads, verified, rejected
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
			candidates := []string{}
			if cited, _ := entry["chunk_id"].(string); cited != "" && textByID[cited] != "" {
				candidates = append(candidates, cited)
			} else {
				for id := range textByID {
					candidates = append(candidates, id)
				}
				sort.Strings(candidates)
			}
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
				break
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
	return survivors, verified, rejected
}
