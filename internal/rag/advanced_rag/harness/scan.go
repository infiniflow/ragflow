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
	"strings"
)

// ---------------------------------------------------------------------------
// The COVERAGE SWEEP of an enumeration.
//
// An enumeration ("how many named people did X kill") is bounded by what the model
// can REMEMBER while a corpus holds the truth: measured (2026-09-16, 三国/关羽, four
// runs of the same question) the answer missing 2-4 members each time, and the
// missing names differed every run — 程远志 / 管亥 / 荀正 / 车胄 appeared ZERO times in
// one run's whole log, because no session ever named them. A model that does not
// think of a name cannot retrieve it, and a member nobody retrieved is a member
// nobody records.
//
// So the sweep is driven by the CORPUS instead: a direction DECLARES the words its
// source uses for the act (斩/杀/诛/劈/砍 — declared, never inferred from a slot's
// text), the runtime asks the store for each of them once, and the passages that come
// back become the session's reading list. The division of labour stays the usual one:
// the runtime supplies the fact "this text matches the act and you have not read it",
// the model decides who is in it.
//
// Coverage is what replaces the "two batches added nothing" stop rule: matched and
// read are counts over a fixed candidate set, so "complete" is a property of the
// corpus rather than of the model's patience.
// ---------------------------------------------------------------------------

// Bounds on the sweep. A declared term is a word the SOURCE uses, so a handful of
// them covers the act; the per-term and total caps keep the reading list small
// enough for a session to finish inside its clock.
const (
	// ScanTermsMax caps how many act terms one direction may declare. Ten rather than
	// six because a source words one deed many ways (斩 / 杀 / 劈 / 挥为两段 …), and the
	// phrasing a term does not cover is a passage the sweep never sees.
	ScanTermsMax = 10
	// ScanPerTerm is how many matching passages one term's sweep keeps.
	//
	// Measured (2026-09-16, 三国/关羽): at 12 the three busiest act words returned
	// exactly 12 each — the cap, not the corpus — and the tail of a term's matches is
	// where the members nobody thought of sit (杨龄's passage is one of the later
	// "砍" matches).
	ScanPerTerm = 30
	// ScanItemsMax caps the whole reading list.
	ScanItemsMax = 100
	// ScanBatchPerTurn is how many unread matching passages one turn hands over.
	//
	// Measured (2026-09-16, 三国/关羽): at 2 the round read 38 of the 56 passages the
	// sweep matched before its sessions converged — the reading is the work, and two
	// passages a turn is less than a session's turns can carry. Four doubles the
	// throughput while keeping one turn's reading list smaller than the evidence a
	// turn already carries.
	//
	// Eight, after the next measurement: a round hands over a batch only on a turn that
	// ENDED IN A TOOL RESULT (the block rides on the tool message), and the run's three
	// sessions produced 12 such hand-overs between them over two rounds — 12 × 4 = 48 of
	// the 100 matched passages read, with 52 never opened when the clock closed the run
	// ("scan-unread=60 … only 15s left; closing out"). The hand-overs are what the round
	// actually has; the batch is the number to raise, because turning a session's
	// non-tool turns into carriers would mean appending runtime text to a message the
	// model wrote.
	ScanBatchPerTurn = 8
)

// ScanItem is one passage the sweep matched for a declared act term, with whether a
// session has been shown it.
type ScanItem struct {
	Term    string
	ChunkID string
	Chunk   map[string]any
	Read    bool
}

// ScanSweeper is the one capability the sweep needs: ask the store for a term
// WITHOUT the model asking, and page the corpus for it.
//
// It is a small optional interface rather than a ToolExecutor method so the sweep
// stays a runtime job with its own seam: an executor that cannot sweep (a test
// double, a deployment whose store is unavailable) simply leaves coverage at zero
// and the round behaves exactly as it did before.
type ScanSweeper interface {
	// ScanTerm returns up to limit passages that MATCH term for subject, the way a
	// keyword leg would: term is the act word, subject the (declared, possibly empty)
	// actor it is about. Implementations must return only passages that carry term —
	// a sweep's whole value is that its matches are real.
	ScanTerm(ctx context.Context, subject, term string, limit int) []map[string]any
}

// DeclareScanTerms records the act words a direction declared and returns the ones
// that were new. Empty and duplicate terms are dropped; the list is capped.
func (k *Kbinfos) DeclareScanTerms(terms []string) []string {
	if k == nil {
		return nil
	}
	var added []string
	k.scanMu.Lock()
	defer k.scanMu.Unlock()
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" || len(k.scanTerms) >= ScanTermsMax {
			continue
		}
		dup := false
		for _, have := range k.scanTerms {
			if have == t {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		k.scanTerms = append(k.scanTerms, t)
		added = append(added, t)
	}
	return added
}

// DeclaredScanTerms returns the act words declared so far.
func (k *Kbinfos) DeclaredScanTerms() []string {
	if k == nil {
		return nil
	}
	k.scanMu.Lock()
	defer k.scanMu.Unlock()
	return append([]string(nil), k.scanTerms...)
}

// SweepResult is one declared term's sweep: the term, and the passages that matched it.
type SweepResult struct {
	Term   string
	Chunks []map[string]any
}

// AddScanItems records one term's sweep result (see AddScanResults).
func (k *Kbinfos) AddScanItems(term string, chunks []map[string]any) int {
	return k.AddScanResults([]SweepResult{{Term: term, Chunks: chunks}})
}

// AddScanResults records every declared term's sweep into the reading list, one
// passage per term per pass, de-duped by chunk id (a passage two terms both matched is
// ONE reading, not two). It returns how many were added.
//
// The round-robin order is the point, because the list is capped. A cap filled in
// declaration order covers only the terms that happened to be declared first: measured
// (2026-09-16, 三国/关羽) four words filled the whole 100-passage list (30+29+26+15) and
// the remaining six — the rare act words, whose passages are worded in the source's own
// phrasing, which is exactly where the members nobody thought of sit — contributed
// nothing at all, because the cap was already reached when their turn came. Taking one
// from each in turn spends the same 100 passages on coverage of every act word the
// direction declared.
func (k *Kbinfos) AddScanResults(results []SweepResult) int {
	if k == nil || len(results) == 0 {
		return 0
	}
	k.scanMu.Lock()
	defer k.scanMu.Unlock()
	if k.scanIndex == nil {
		k.scanIndex = map[string]int{}
	}
	added := 0
	for depth := 0; ; depth++ {
		progressed := false
		for _, r := range results {
			if depth >= len(r.Chunks) {
				continue
			}
			progressed = true
			c := r.Chunks[depth]
			if c == nil {
				continue
			}
			id := ChunkIDOf(c)
			if id == "" {
				continue
			}
			if _, ok := k.scanIndex[id]; ok {
				continue
			}
			if len(k.scanItems) >= ScanItemsMax {
				return added
			}
			k.scanIndex[id] = len(k.scanItems)
			k.scanItems = append(k.scanItems, ScanItem{Term: r.Term, ChunkID: id, Chunk: c})
			added++
		}
		if !progressed {
			break
		}
	}
	return added
}

// UnreadScan HANDS OVER up to limit passages no session has been shown, and marks
// them read: being shown one IS the read (the division of labour — the runtime
// knows what it delivered, the model decides what is in it).
func (k *Kbinfos) UnreadScan(limit int) []ScanItem {
	if k == nil || limit <= 0 {
		return nil
	}
	k.scanMu.Lock()
	defer k.scanMu.Unlock()
	out := make([]ScanItem, 0, limit)
	for i := range k.scanItems {
		if k.scanItems[i].Read {
			continue
		}
		k.scanItems[i].Read = true
		out = append(out, k.scanItems[i])
		if len(out) >= limit {
			break
		}
	}
	return out
}

// ScanCoverage is (matched, read) over the sweep's candidate set. Unread matching
// passages with budget left are what an enumeration still owes the answer.
func (k *Kbinfos) ScanCoverage() (matched, read int) {
	if k == nil {
		return 0, 0
	}
	k.scanMu.Lock()
	defer k.scanMu.Unlock()
	for _, it := range k.scanItems {
		if it.Read {
			read++
		}
	}
	return len(k.scanItems), read
}

// scanBlock renders the sweep's unread passages for one session turn, or "" when
// there is nothing left to read. It marks what it hands over as read.
//
// The wording separates the two facts the model needs and the two it must not
// confuse: the passage MATCHES the act word and the model has NOT read it (facts),
// versus who the member is (the model's reading — the runtime names nobody).
func (k *Kbinfos) scanBlock(limit int) string {
	if k == nil || limit <= 0 {
		return ""
	}
	items := k.UnreadScan(limit)
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[scan] unread passages the corpus matched for the act words this direction declared. ")
	b.WriteString("Name every person these passages attribute to the act — each as a `kind:\"members\"` item with its chunk_id — ")
	b.WriteString("and say so when a passage attributes it to SOMEONE ELSE (that is how a name that is not a member is excluded).")
	for _, it := range items {
		text := strings.TrimSpace(ChunkTextOf(it.Chunk))
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "\n- [term %q] [ID:%s] %s", it.Term, it.ChunkID, truncateRunes(text, scanPassageRunes))
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

// scanPassageRunes bounds one handed-over passage. A sweep's passages are read for
// NAMES, so a passage's neighbourhood is what matters, not its whole text.
const scanPassageRunes = 400
