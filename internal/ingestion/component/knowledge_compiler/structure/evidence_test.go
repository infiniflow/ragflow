package structure

import (
	"strings"
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

// The object spelling is the only one that reaches the model, so a reader that
// recognised only the bare string could never agree with the prompt: the
// template would ask for quotes and the gate would then refuse to check them.
func TestRelationExpectsEvidenceReadsTheDeclaredObjectForm(t *testing.T) {
	objectForm := map[string]any{
		"relation": map[string]any{"output_fields": []any{
			map[string]any{"name": "evidence", "type": "list", "shape": `[{"quote": "x"}]`},
		}},
	}
	if !RelationExpectsEvidence(objectForm) {
		t.Fatal("an output_fields object named evidence must be gated")
	}
	objectWithout := map[string]any{
		"relation": map[string]any{"output_fields": []any{
			map[string]any{"name": "score", "type": "float"},
		}},
	}
	if RelationExpectsEvidence(objectWithout) {
		t.Fatal("an unrelated output_fields object must not be gated")
	}
	// A nameless object declares no key, and a padded name is the same key.
	padded := map[string]any{
		"relation": map[string]any{"output_fields": []any{
			map[string]any{"type": "list"},
			map[string]any{"name": "  Evidence  ", "type": "list"},
		}},
	}
	if !RelationExpectsEvidence(padded) {
		t.Fatal("a padded name must still match")
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

func TestEntityExpectsEvidence(t *testing.T) {
	cfg := map[string]any{
		"entity":   map[string]any{"output_fields": []any{map[string]any{"name": "evidence", "type": "list"}}},
		"relation": map[string]any{"output_fields": []any{map[string]any{"name": "score"}}},
	}
	if !EntityExpectsEvidence(cfg) {
		t.Fatal("entity.output_fields naming evidence must be reported as declared")
	}
	if RelationExpectsEvidence(cfg) {
		t.Fatal("sections must not be conflated: relation declares no evidence")
	}
	if EntityExpectsEvidence(nil) {
		t.Fatal("nil config must not declare evidence")
	}
	if !RelationExpectsEvidence(map[string]any{
		"relation": map[string]any{"output_fields": []any{map[string]any{"name": "evidence"}}},
	}) {
		t.Fatal("the shared reader must still serve the relation flag")
	}
}

// The gate reports only survivors, so a stage whose quotes all failed to locate
// used to be indistinguishable from one the model never annotated. These counts
// are the difference the progress log needs.
func TestGateEvidenceReportsWhatArrivedAndWhatSurvived(t *testing.T) {
	textByID := map[string]string{"c1": "The capital is Paris. Paris is in France."}
	payloads := []map[string]any{
		{"name": "paris", "evidence": []any{
			map[string]any{"quote": "Paris is in France.", "chunk_id": "c1"},
			map[string]any{"quote": "a paraphrased sentence", "chunk_id": "c1"},
		}},
		{"name": "lyon"},
		{"name": "nice", "evidence": []any{map[string]any{"quote": "nowhere in the text", "chunk_id": "c1"}}},
	}

	survivors, stats := GateEvidence(payloads, textByID, EvidenceGateSoft)

	if len(survivors) != 3 {
		t.Fatalf("soft mode keeps every payload, got %d", len(survivors))
	}
	if stats.Payloads != 3 || stats.WithEvidence != 2 || stats.QuotesIn != 3 {
		t.Fatalf("carried counts = %+v, want 3 payload(s)/2 carrying/3 quote(s)", stats)
	}
	if stats.QuotesKept != 1 || stats.PayloadsLost != 1 {
		t.Fatalf("gate outcome = %+v, want 1 kept quote and 1 payload left with none", stats)
	}
	if !stats.Validated {
		t.Fatal("a gate with source text must report itself as validated")
	}
	if _, has := survivors[0]["evidence"]; !has {
		t.Fatal("the locatable quote must survive")
	}
	if _, has := survivors[2]["evidence"]; has {
		t.Fatal("the payload that lost its only quote must be stripped of the field")
	}
}

func TestGateEvidenceWithoutSourceTextOnlyCounts(t *testing.T) {
	payloads := []map[string]any{
		{"name": "x", "evidence": []any{map[string]any{"quote": "q"}}},
	}
	survivors, stats := GateEvidence(payloads, nil, EvidenceGateSoft)
	if len(survivors) != 1 {
		t.Fatalf("nothing may be removed without source text, got %d", len(survivors))
	}
	if stats.Validated || stats.QuotesKept != 0 || stats.PayloadsLost != 0 {
		t.Fatalf("stats = %+v, want unvalidated with no outcome", stats)
	}
	if stats.WithEvidence != 1 || stats.QuotesIn != 1 {
		t.Fatalf("stats must still report what arrived: %+v", stats)
	}
	if _, has := survivors[0]["evidence"]; !has {
		t.Fatal("an unvalidated stage must not strip the field")
	}
	if s := stats.Summary(); !strings.Contains(s, "not validated") {
		t.Fatalf("summary must not claim a verdict without source text: %q", s)
	}
}

func TestEvidenceGateStatsMergeAndSummary(t *testing.T) {
	total := EvidenceGateStats{Payloads: 3, WithEvidence: 2, QuotesIn: 4, QuotesKept: 3, PayloadsLost: 1, Validated: true}
	total = total.Merge(EvidenceArrived([]map[string]any{{"name": "z"}}))
	if total.Payloads != 4 || total.WithEvidence != 2 || total.QuotesIn != 4 {
		t.Fatalf("merge lost the carried counts: %+v", total)
	}
	if total.QuotesKept != 3 || total.PayloadsLost != 1 || !total.Validated {
		t.Fatalf("merge lost the gate outcome: %+v", total)
	}
	// An empty arrival must not mark a validated stage unvalidated.
	empty := EvidenceArrived(nil)
	if merged := (EvidenceGateStats{Validated: true}).Merge(empty); !merged.Validated {
		t.Fatal("merge must keep a validated stage validated")
	}
	summary := total.Summary()
	for _, want := range []string{"carried 2/4 payload(s)", "4 quote(s) in", "3 kept", "1 payload(s) with none verified"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q missing %q", summary, want)
		}
	}
}

// The counts say how many quotes died; only the samples say why. These two
// rejections produce identical statistics yet have opposite causes, which is
// exactly what the sample has to separate.
func TestRejectedEvidenceSamplesSeparateMisCitationFromRewriting(t *testing.T) {
	textByID := map[string]string{
		"c1": "Paris is the capital of France.",
		"c2": "Berlin is the capital of Germany.",
	}
	payloads := []map[string]any{
		// Real text, wrong chunk: the sentence exists, in c2 rather than c1.
		{"name": "berlin", "evidence": []any{
			map[string]any{"quote": "Berlin is the capital of Germany.", "chunk_id": "c1"},
		}},
		// No such text anywhere in the batch.
		{"name": "munich", "evidence": []any{
			map[string]any{"quote": "Munich is the capital of Bavaria.", "chunk_id": "c2"},
		}},
	}
	survivors, verified, rejected, samples := validatePayloadEvidence(payloads, textByID, EvidenceGateSoft)
	if len(survivors) != 2 || verified != 0 || rejected != 2 {
		t.Fatalf("got survivors=%d verified=%d rejected=%d, want 2/0/2", len(survivors), verified, rejected)
	}
	if len(samples) != 2 {
		t.Fatalf("sampled %d rejection(s), want 2", len(samples))
	}

	misCited := samples[0]
	if misCited.Elsewhere != "c2" {
		t.Fatalf("mis-cited quote: elsewhere=%q, want c2 — the text IS in the batch", misCited.Elsewhere)
	}
	if !misCited.CitedKnown || misCited.Cited != "c1" {
		t.Fatalf("mis-cited quote: cited=%q known=%t, want c1/true", misCited.Cited, misCited.CitedKnown)
	}
	if misCited.Chunks != 2 {
		t.Fatalf("mis-cited quote: chunks=%d, want 2", misCited.Chunks)
	}

	rewritten := samples[1]
	if rewritten.Elsewhere != "" {
		t.Fatalf("rewritten quote: elsewhere=%q, want empty — the text is nowhere in the batch", rewritten.Elsewhere)
	}
	if want := len("Munich is the capital of Bavaria."); rewritten.QuoteLen != want {
		t.Fatalf("rewritten quote: chars=%d, want %d", rewritten.QuoteLen, want)
	}
}

// A rejection must be explained by an ordinary run. Needing an extra startup
// option would mean the diagnosis is only available to whoever remembered to
// pass it, which is the same as not having one.
func TestGateEvidenceAlwaysAttachesRejectionSamples(t *testing.T) {
	textByID := map[string]string{"c1": "alpha"}
	payloads := []map[string]any{
		{"name": "x", "evidence": []any{map[string]any{"quote": "missing", "chunk_id": "c1"}}},
	}

	_, stats := GateEvidence(payloads, textByID, EvidenceGateSoft)
	if len(stats.Samples) != 1 {
		t.Fatalf("a rejection must be sampled without any flag, got %d sample(s)", len(stats.Samples))
	}
	detail := stats.SampleDetail()
	if !strings.Contains(detail, "missing") {
		t.Fatalf("detail must quote the rejected text: %q", detail)
	}
	if !strings.Contains(detail, "elsewhere=-") {
		t.Fatalf("detail must render the empty elsewhere column: %q", detail)
	}
	if stats.Summary() == "" {
		t.Fatal("the counts line must survive alongside the samples")
	}
}

// A stage that rejected nothing must not grow a message of its own.
func TestGateEvidenceKeepsQuietWhenNothingIsRejected(t *testing.T) {
	textByID := map[string]string{"c1": "The capital is Paris."}
	payloads := []map[string]any{
		{"name": "paris", "evidence": []any{map[string]any{"quote": "The capital is Paris.", "chunk_id": "c1"}}},
	}
	_, stats := GateEvidence(payloads, textByID, EvidenceGateSoft)
	if len(stats.Samples) != 0 {
		t.Fatalf("nothing was rejected, got %d sample(s)", len(stats.Samples))
	}
	if detail := stats.SampleDetail(); detail != "" {
		t.Fatalf("SampleDetail must render nothing: %q", detail)
	}
}

// A chunk id the model invented is worth flagging: it says the model was not
// actually reading the ids the prompt showed it.
func TestRejectedEvidenceSampleFlagsAnUnknownCitation(t *testing.T) {
	textByID := map[string]string{"c1": "alpha", "c2": "beta"}
	payloads := []map[string]any{
		{"name": "x", "evidence": []any{map[string]any{"quote": "nowhere", "chunk_id": "c9"}}},
	}
	_, _, _, samples := validatePayloadEvidence(payloads, textByID, EvidenceGateSoft)
	if len(samples) != 1 {
		t.Fatalf("sampled %d rejection(s), want 1", len(samples))
	}
	if samples[0].CitedKnown {
		t.Fatalf("c9 is not a batch chunk, CitedKnown must be false: %+v", samples[0])
	}
	detail := EvidenceGateStats{PayloadsLost: 1, Samples: samples}.SampleDetail()
	if !strings.Contains(detail, "cited=c9 (not a batch chunk)") {
		t.Fatalf("detail must flag the invented citation: %q", detail)
	}
}

// A truncated sample must not make an over-long quote look short: QuoteLen
// carries the true length so "the model copied the whole paragraph" stays
// visible.
func TestRejectedEvidenceSampleKeepsFullLengthWhenTruncated(t *testing.T) {
	long := strings.Repeat("a", 400)
	textByID := map[string]string{"c1": "alpha"}
	payloads := []map[string]any{
		{"name": "x", "evidence": []any{map[string]any{"quote": long, "chunk_id": "c1"}}},
	}
	_, _, _, samples := validatePayloadEvidence(payloads, textByID, EvidenceGateSoft)
	if len(samples) != 1 {
		t.Fatalf("sampled %d rejection(s), want 1", len(samples))
	}
	if samples[0].QuoteLen != 400 {
		t.Fatalf("QuoteLen = %d, want the untruncated 400", samples[0].QuoteLen)
	}
	if got := len([]rune(samples[0].Quote)); got != 161 {
		t.Fatalf("rendered quote = %d rune(s), want 160 plus the ellipsis", got)
	}
}

func TestEvidenceSamplesMergeUnderTheLimit(t *testing.T) {
	many := func(n int) []EvidenceRejectSample {
		out := make([]EvidenceRejectSample, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, EvidenceRejectSample{Quote: "q", QuoteLen: 1})
		}
		return out
	}

	total := EvidenceGateStats{Samples: many(evidenceSampleLimit - 1)}
	total = total.Merge(EvidenceGateStats{Samples: many(5)})
	if len(total.Samples) != evidenceSampleLimit {
		t.Fatalf("merged samples = %d, want the %d limit", len(total.Samples), evidenceSampleLimit)
	}
	if got := (EvidenceGateStats{Samples: many(evidenceSampleLimit)}).Merge(EvidenceGateStats{Samples: many(3)}); len(got.Samples) != evidenceSampleLimit {
		t.Fatalf("merge past the limit = %d, want %d", len(got.Samples), evidenceSampleLimit)
	}
	if detail := (EvidenceGateStats{}).SampleDetail(); detail != "" {
		t.Fatalf("no samples must render nothing, got %q", detail)
	}
}
