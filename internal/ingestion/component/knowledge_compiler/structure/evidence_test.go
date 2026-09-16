package structure

import (
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestEvidenceGateModeParsing(t *testing.T) {
	cases := []struct {
		in   map[string]any
		want string
	}{
		{map[string]any{"evidence_gate_mode": "hard"}, EvidenceGateHard},
		{map[string]any{"evidence_gate_mode": "SOFT"}, EvidenceGateSoft},
		{map[string]any{"evidence_gate_mode": "bogus"}, EvidenceGateSoft},
		{map[string]any{}, EvidenceGateSoft},
		{nil, EvidenceGateSoft},
	}
	for _, c := range cases {
		if got := EvidenceGateMode(c.in); got != c.want {
			t.Fatalf("EvidenceGateMode(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRelationExpectsEvidence(t *testing.T) {
	withEvidence := map[string]any{
		"relation": map[string]any{"output_fields": []any{"evidence", "other"}},
	}
	without := map[string]any{
		"relation": map[string]any{"output_fields": []any{"other"}},
	}
	if !RelationExpectsEvidence(withEvidence) {
		t.Fatal("relation.output_fields with evidence should be detected")
	}
	if RelationExpectsEvidence(without) {
		t.Fatal("relation.output_fields without evidence must not be gated")
	}
	if RelationExpectsEvidence(nil) {
		t.Fatal("nil config must not be gated")
	}
}

func TestValidatePayloadEvidenceSoftKeepsPayloadWithoutEvidence(t *testing.T) {
	textByID := map[string]string{"c1": "The capital is Paris."}
	payloads := []map[string]any{{
		"name":     "invented",
		"evidence": []any{map[string]any{"quote": "a sentence that does not exist", "chunk_id": "c1"}},
	}}
	kept, verified, rejected := ValidatePayloadEvidence(payloads, textByID, EvidenceGateSoft)
	if verified != 0 || rejected != 1 {
		t.Fatalf("got verified=%d rejected=%d, want 0/1", verified, rejected)
	}
	if len(kept) != 1 {
		t.Fatalf("soft mode keeps the payload, got %d", len(kept))
	}
	if _, has := kept[0]["evidence"]; has {
		t.Fatal("rejected evidence must be removed")
	}
}

func TestValidatePayloadEvidenceHardDropsThePayload(t *testing.T) {
	textByID := map[string]string{"c1": "The capital is Paris."}
	payloads := []map[string]any{{
		"name":     "invented",
		"evidence": []any{map[string]any{"quote": "a sentence that does not exist", "chunk_id": "c1"}},
	}}
	kept, verified, rejected := ValidatePayloadEvidence(payloads, textByID, EvidenceGateHard)
	if verified != 0 || rejected != 1 {
		t.Fatalf("got verified=%d rejected=%d, want 0/1", verified, rejected)
	}
	if len(kept) != 0 {
		t.Fatalf("hard mode drops the payload, got %d", len(kept))
	}
}

func TestValidatePayloadEvidenceLocatesAndStampsOffsets(t *testing.T) {
	src := "The   capital   is   Paris."
	textByID := map[string]string{"c1": src}
	payloads := []map[string]any{{
		"name":     "capital",
		"evidence": []any{map[string]any{"quote": "The capital is Paris.", "chunk_id": "c1"}},
	}}
	kept, verified, _ := ValidatePayloadEvidence(payloads, textByID, EvidenceGateSoft)
	if verified != 1 {
		t.Fatalf("verified=%d, want 1", verified)
	}
	ev := kept[0]["evidence"].([]any)[0].(map[string]any)
	s, _ := ev["start"].(int)
	e, _ := ev["end"].(int)
	if got := src[s:e]; got != src {
		t.Fatalf("span %d:%d sliced %q, want the original sentence", s, e, got)
	}
}

func TestValidatePayloadEvidenceFallsBackToSortedScan(t *testing.T) {
	// The model cited nothing: the gate scans the batch. Sorting keeps the
	// choice deterministic across runs even when two chunks hold the sentence.
	textByID := map[string]string{"zz": "alpha text", "aa": "alpha text"}
	payloads := []map[string]any{{
		"name":     "dup",
		"evidence": []any{map[string]any{"quote": "alpha text"}},
	}}
	kept, verified, _ := ValidatePayloadEvidence(payloads, textByID, EvidenceGateSoft)
	if verified != 1 {
		t.Fatalf("verified=%d, want 1", verified)
	}
	ev := kept[0]["evidence"].([]any)[0].(map[string]any)
	if id := ev["chunk_id"]; id != "aa" {
		t.Fatalf("fallback chunk_id = %v, want the first sorted candidate", id)
	}
}

func TestLocateEvidenceMultibyteOffsets(t *testing.T) {
	// A byte offset from strings.Index runs ahead of the rune offset, so a map
	// indexed per rune resolves every lookup past the first multi-byte rune to
	// the wrong span. Mirrors the tree package's regression test.
	src := "前言。中国的首都是北京。尾声。"
	quote := "中国的首都是北京。"
	start, end, ok := locateEvidence(quote, src)
	if !ok {
		t.Fatalf("CJK quote not located in %q", src)
	}
	if got := src[start:end]; got != quote {
		t.Fatalf("span sliced %q, want %q", got, quote)
	}

	// A match running to the end of the text must end at len(text).
	tail := "前言。中国的首都是北京。"
	start, end, ok = locateEvidence(quote, tail)
	if !ok {
		t.Fatal("quote at the end of the text not located")
	}
	if end != len(tail) {
		t.Fatalf("end = %d, want len(text) = %d", end, len(tail))
	}
	if got := tail[start:end]; got != quote {
		t.Fatalf("span sliced %q, want %q", got, quote)
	}
}

func TestBatchTextByIDUsesPackBatchIDSynthesis(t *testing.T) {
	// The gate must validate against exactly the text the prompt showed, so its
	// id synthesis matches PackBatch's (empty id → "chunk-<n>", index-based).
	got := batchTextByID([]common.Chunk{
		{ID: "", Text: "alpha"},
		{ID: "", Text: "beta"},
		{ID: "", Text: "   "},
	})
	if len(got) != 2 {
		t.Fatalf("blank chunks skipped: %v", got)
	}
	if got["chunk-1"] != "alpha" || got["chunk-2"] != "beta" {
		t.Fatalf("ids must match PackBatch: %v", got)
	}
}
