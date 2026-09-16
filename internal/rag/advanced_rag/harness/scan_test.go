package harness

import (
	"strconv"
	"strings"
	"testing"
)

// TestScanReadingListCoversEveryDeclaredTerm pins the round-robin admission: the list is
// capped, so the order it is filled in decides which act words it covers.
//
// Measured (2026-09-16, 三国/关羽, one run): four act words filled the whole 100-passage
// list (30+29+26+15) and the other six contributed nothing at all, because the cap was
// already reached when their turn came. The names the runs keep missing are worded with
// those rare words (劈管亥于马下), so a cap spent in declaration order is a cap spent away
// from exactly the passages the sweep was built to reach.
func TestScanReadingListCoversEveryDeclaredTerm(t *testing.T) {
	terms := []string{"杀", "斩", "诛", "刺", "劈", "砍"}
	results := make([]SweepResult, 0, len(terms))
	for _, term := range terms {
		chunks := make([]map[string]any, 0, 40)
		for i := 0; i < 40; i++ {
			chunks = append(chunks, scanChunk(term+strconv.Itoa(i), term+"于马下"))
		}
		results = append(results, SweepResult{Term: term, Chunks: chunks})
	}
	kb := &Kbinfos{}
	if added := kb.AddScanResults(results); added != ScanItemsMax {
		t.Fatalf("added = %d, want the cap %d", added, ScanItemsMax)
	}
	perTerm := map[string]int{}
	for _, it := range kb.scanItems {
		perTerm[it.Term]++
	}
	if len(perTerm) != len(terms) {
		t.Fatalf("the reading list covers %d act word(s), want all %d: %v", len(perTerm), len(terms), perTerm)
	}
	floor := ScanItemsMax / len(terms)
	for _, term := range terms {
		if perTerm[term] < floor {
			t.Errorf("term %q got %d passage(s), want at least %d (one per pass across the terms)", term, perTerm[term], floor)
		}
	}
}

func scanChunk(id, text string) map[string]any {
	return map[string]any{"chunk_id": id, "content": text}
}

// TestScanLedgerTracksCoverage pins the sweep's bookkeeping: the act words a
// direction declared, the passages the runtime matched for them (deduped — a passage
// two terms both matched is ONE reading), and how many have been handed to a session.
func TestScanLedgerTracksCoverage(t *testing.T) {
	kb := &Kbinfos{}
	added := kb.DeclareScanTerms([]string{"斩", "杀", " ", "斩", "诛"})
	if len(added) != 3 || added[0] != "斩" {
		t.Fatalf("declared = %v, want 斩/杀/诛 (duplicates and blanks dropped)", added)
	}
	// Over the cap: extra terms are refused rather than silently growing the sweep.
	many := []string{}
	for i := 0; i < ScanTermsMax+4; i++ {
		many = append(many, "t"+strconv.Itoa(i))
	}
	kb2 := &Kbinfos{}
	if got := kb2.DeclareScanTerms(many); len(got) != ScanTermsMax {
		t.Fatalf("declared %d terms, want the cap %d", len(got), ScanTermsMax)
	}

	if n := kb.AddScanItems("斩", []map[string]any{
		scanChunk("c1", "云长提华雄之头"),
		scanChunk("c2", "刀起处，蔡阳头已落地"),
	}); n != 2 {
		t.Fatalf("added = %d, want 2", n)
	}
	// The same passage under a second term is not a second reading.
	if n := kb.AddScanItems("杀", []map[string]any{
		scanChunk("c1", "云长提华雄之头"),
		scanChunk("c3", "关公刀起，秦琪头落"),
	}); n != 1 {
		t.Fatalf("added = %d, want 1 (c1 was already in the reading list)", n)
	}

	matched, read := kb.ScanCoverage()
	if matched != 3 || read != 0 {
		t.Fatalf("coverage = %d/%d, want 0/3 read before anything is handed over", read, matched)
	}

	// Handing over IS the read, and the batch respects its limit.
	items := kb.UnreadScan(2)
	if len(items) != 2 || items[0].ChunkID != "c1" {
		t.Fatalf("unread = %v, want the first two passages in order", items)
	}
	if matched, read = kb.ScanCoverage(); matched != 3 || read != 2 {
		t.Fatalf("coverage = %d/%d, want 2/3 after one batch", read, matched)
	}
	if got := kb.UnreadScan(0); got != nil {
		t.Fatalf("unread(0) = %v, want nothing", got)
	}
	if items = kb.UnreadScan(2); len(items) != 1 || items[0].ChunkID != "c3" {
		t.Fatalf("unread = %v, want the remaining passage", items)
	}
	if matched, read = kb.ScanCoverage(); read != matched {
		t.Fatalf("coverage = %d/%d, want complete", read, matched)
	}
}

// TestScanBlockNamesTheFactsAndNobodyElse pins the division of labour in the block a
// session reads: the runtime supplies "this passage matches the act and you have not
// read it" and the model decides who is in it — the runtime names no member, and it
// says so when a passage attributes the deed to someone else.
func TestScanBlockNamesTheFactsAndNobodyElse(t *testing.T) {
	kb := &Kbinfos{}
	kb.DeclareScanTerms([]string{"斩"})
	kb.AddScanItems("斩", []map[string]any{scanChunk("c1", "关公马快，赶上文丑，脑后一刀")})

	block := kb.scanBlock(1)
	for _, want := range []string{"[scan]", `[term "斩"]`, "[ID:c1]", "脑后一刀", "SOMEONE ELSE"} {
		if !strings.Contains(block, want) {
			t.Errorf("scan block = %q, missing %q", block, want)
		}
	}
	if strings.Contains(block, "文丑 is a member") {
		t.Errorf("scan block must not name members: %q", block)
	}
	// Handed over once: the second turn gets nothing, and the block says the item was
	// read rather than handing it twice.
	if again := kb.scanBlock(1); again != "" {
		t.Errorf("scan block = %q, want nothing left to hand over", again)
	}
}

// TestSessionRecordCarriesScanCoverage pins the enumeration's stop rule reaching the
// model: the record line says how much of the corpus's matches for the declared act
// have been read, so "am I done?" is a number rather than an appetite.
func TestSessionRecordCarriesScanCoverage(t *testing.T) {
	kb := &Kbinfos{}
	kb.DeclareScanTerms([]string{"斩", "杀"})
	kb.AddScanItems("斩", []map[string]any{scanChunk("c1", "a"), scanChunk("c2", "b")})
	kb.AddScanItems("杀", []map[string]any{scanChunk("c3", "c")})
	kb.UnreadScan(1)

	rec := CollectSessionRecord(State{}, kb)
	if rec.ScanMatched != 3 || rec.ScanRead != 1 {
		t.Fatalf("record coverage = %d/%d, want 1/3", rec.ScanRead, rec.ScanMatched)
	}
	if len(rec.ScanTerms) != 2 || rec.ScanTerms[0] != "斩" {
		t.Fatalf("record terms = %v, want the declared act words", rec.ScanTerms)
	}
	line := rec.Line()
	for _, want := range []string{"scan=1/3 read", "unread matches"} {
		if !strings.Contains(line, want) {
			t.Errorf("record line = %q, missing %q", line, want)
		}
	}
	// Complete coverage drops the warning but keeps the counts.
	kb.UnreadScan(10)
	line = CollectSessionRecord(State{}, kb).Line()
	if !strings.Contains(line, "scan=3/3 read") || strings.Contains(line, "unread matches") {
		t.Errorf("record line = %q, want complete coverage with no warning", line)
	}
}
