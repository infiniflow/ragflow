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

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// CoverageResolvePrompt is the whole instruction of the enumeration's LAST node.
//
// It is written around the QUESTION rather than around one kind of answer: the same step
// runs on "which awards did X win" and on "who did X kill", so what a line has to do is
// state a member of the set the question asks for. Nothing here decides what counts as a
// member — that judgement stays the model's.
const CoverageResolvePrompt = "You are given numbered evidence lines from one fixed corpus, each with the chunk id it came from.\n" +
	"For EVERY line decide one thing: does this line itself state a member of the set the question asks for — and if it does, what is that member called?\n" +
	"Judge only what the line says, never what you know from elsewhere. A mention that is not a member (a plan, a promise, a denial, a pursuit, something that did not happen, somebody else's action, or a member the line does not name) is NOT one.\n" +
	"Answer with JSON only, and put EVERY line number in exactly one of the two lists:\n" +
	`{"members": [{"i": <line number>, "name": "<the member the line states>"}], "not_members": [<line numbers>]}` + "\n" +
	"Use the line's own words for the name. Never invent a name, and never answer for a line you were not given."

// ResolvedMember is a member the last node named, with the line it was named from.
type ResolvedMember struct {
	Name    string
	ChunkID string
	Quote   string
}

// ResolveStats is what one resolve produced, for the run log: how many calls it took,
// how many lines were put in front of the model, how many got a verdict, and how many
// would still be UNKNOWN if the answer were written now.
type ResolveStats struct {
	Batches  int
	Asked    int
	Answered int
	Unknown  int
	Failed   int
	// Unjudged are the chunk ids that got no verdict — the windows a failed call left, the lines
	// the model skipped, the windows the cap kept out. The count alone says "the list may be
	// short"; the ids say WHICH passages nobody read, which is what a reader can check.
	Unjudged []string
}

// resolveJob is ONE window as a unit of work: the index it holds in the caller's window list
// (which is what a citation is resolved from) and the window itself.
type resolveJob struct {
	idx int
	w   CoverageWindow
}

// batchResult is one CALL's outcome: the members it named, which of its jobs were judged (as
// a member or explicitly not), and whether the call failed. The indices are the CALLER's
// window indices, so a later pass can re-ask exactly the ones that got no verdict.
type batchResult struct {
	members []ResolvedMember
	judged  []int
	failed  bool
}

// The resolve's bounds. A batch is small because ONE prompt carrying every candidate is the
// failure this node exists to remove: a single call that carries them all reaches its clock
// and answers nothing, losing a whole enumeration's worth of members to one timeout.
//
// These bound ONE budget, not three independent knobs: this node has to be able to judge
// coverageWindowsMax windows inside CoverageResolveTimeoutS, and its capacity is
// workers × (clock / the slowest call it can expect) × batch. The cap and the capacity are
// the SAME number on purpose (6 × (30/5) × 8 = 288): a cap above the capacity is a ceiling
// the point-of-naming step can never clear, which makes the member set a function of provider
// latency instead of a function of the corpus, and a cap below it drops windows the
// enumeration already paid a search for (they are reported UNKNOWN: nobody judged them).
const (
	coverageResolveBatch      = 8
	coverageResolveWorkers    = 6
	coverageResolveQuoteChars = 300
	// coverageResolvePasses bounds the re-asking: the first pass puts every window in front of
	// the model, and each pass after it re-asks ONLY the windows that got no verdict — a call
	// that failed, or lines the reply left out. Work that already has a verdict is never sent
	// again, so the set closes without paying twice for the windows that were answered.
	coverageResolvePasses = 5
)

// ResolveCoverage is the enumeration's LAST node: every window gets a verdict, and the
// members are the lines that state one.
//
// Three things are deliberate:
//
//   - EVERY window is asked about, in batches, in parallel. The node is not allowed to
//     skip a candidate in silence: a batch that fails or a line the model does not answer
//     is counted as UNKNOWN and reported, so "nobody looked" can never be read as "the
//     corpus does not say it".
//   - A member's citation is the LINE'S chunk id, never the reply's: the model only has
//     to get a line number right, so a fabricated source cannot reach the record.
//   - The member's NAME is kept as the model wrote it from the line, and members are
//     deduped case-insensitively — one source spells one member several ways, and a set
//     counts entities, not spellings.
func ResolveCoverage(ctx context.Context, model SessionModel, question string, cov Coverage, set CoverageSet) ([]ResolvedMember, ResolveStats) {
	stats := ResolveStats{Asked: len(set.Windows)}
	if model == nil || set.Empty() {
		stats.Unknown = stats.Asked
		return nil, stats
	}
	windows := set.Windows
	// Windows beyond the cap are never put in front of the model, so they are
	// UNKNOWN for the same reason a failed batch is: nobody judged them.
	dropped := 0
	if len(windows) > coverageWindowsMax {
		dropped = len(windows) - coverageWindowsMax
		windows = windows[:coverageWindowsMax]
	}
	stats.Asked = len(windows)

	// EVERY window is a unit of work that must end with exactly one verdict, so the set is
	// CLOSED: the first pass asks about all of them, and each pass after it re-asks only the
	// ones that got none — a call that failed, or lines the reply left out. Answering is what
	// makes a member set a function of the corpus instead of a function of which call happened
	// to come back in time.
	pending := make([]resolveJob, 0, len(windows))
	for i, w := range windows {
		pending = append(pending, resolveJob{idx: i, w: w})
	}
	judged := make([]bool, len(windows))
	seen := map[string]int{}
	var members []ResolvedMember

	for pass := 0; pass < coverageResolvePasses && len(pending) > 0; pass++ {
		if ctx.Err() != nil {
			break
		}
		out := make([]batchResult, (len(pending)+coverageResolveBatch-1)/coverageResolveBatch)
		sem := make(chan struct{}, coverageResolveWorkers)
		var wg sync.WaitGroup
		for b := range out {
			lo := b * coverageResolveBatch
			hi := min(lo+coverageResolveBatch, len(pending))
			part := pending[lo:hi]
			wg.Add(1)
			go func(b int, part []resolveJob) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				out[b] = resolveCoverageBatch(ctx, model, question, cov, part)
			}(b, part)
		}
		wg.Wait()

		for _, r := range out {
			stats.Batches++
			if r.failed {
				stats.Failed++
			}
			for _, idx := range r.judged {
				if !judged[idx] {
					judged[idx] = true
					stats.Answered++
				}
			}
			for _, m := range r.members {
				key := strings.ToLower(strings.TrimSpace(m.Name))
				if key == "" {
					continue
				}
				if i, dup := seen[key]; dup {
					// The member is already known; a second passage is worth keeping when the
					// first came without one (evidence is what makes a member answerable).
					if members[i].ChunkID == "" && m.ChunkID != "" {
						members[i] = m
					}
					continue
				}
				seen[key] = len(members)
				members = append(members, m)
			}
		}
		next := pending[:0:0]
		for _, j := range pending {
			if !judged[j.idx] {
				next = append(next, j)
			}
		}
		pending = next
	}
	// Every window is UNKNOWN unless a verdict named it: the windows a failed call left, the
	// lines the model skipped, and the windows the cap kept out.
	stats.Unknown = dropped + (stats.Asked - stats.Answered)
	for i, w := range windows {
		if !judged[i] {
			stats.Unjudged = appendUnique(stats.Unjudged, w.ChunkID)
		}
	}
	if len(set.Windows) > coverageWindowsMax {
		for _, w := range set.Windows[coverageWindowsMax:] {
			stats.Unjudged = appendUnique(stats.Unjudged, w.ChunkID)
		}
	}
	return members, stats
}

// CoverageVocabPrompt asks for the vocabulary of THE SOURCE, never for the vocabulary of the
// language: the passages are what answers it, and a word that is not in them does not qualify.
const CoverageVocabPrompt = `You are given passages from ONE source document.

The direction under study is the deed named by these words — the act they stand for:
%WORDS%

List the words THIS SOURCE uses for that deed. A word qualifies only if it ACTUALLY APPEARS in the
passages below: never list a word you know from the language but cannot see in the text. Include
every distinct phrasing the passages use for it, including unusual and indirect ones.

Answer with JSON only: {"words": ["<word>", "<word>"]}
Every entry is copied EXACTLY as it appears in a passage, at most %MAX% entries. No prose.`

// The probe's bounds: how many passages it reads, how much of each, and how many words it may
// bring back. The call is small on purpose — it is read once per enumeration, against a point of
// naming that judges hundreds of lines — and every word it reports is then checked against the
// very passages it was shown.
const (
	coverageVocabSamples     = 12
	coverageVocabSampleChars = 320
	coverageVocabWords       = 24
)

// induceActWords reads the deed's vocabulary OUT OF THE CORPUS — one call, before the enumeration
// filters with it.
//
// Why this exists: the planner declares the deed's words BEFORE any passage has been read (see the
// initialize prompt's "the words the SOURCE itself uses for that deed" — asked of a text nobody
// has looked at yet). Measured 2026-09-18 (三国演义, 1718 chunks, "关羽杀了多少有姓名的人物"):
// the plan declared seven words, and 程远志 appears ZERO times in that run's whole log, although
// 第一回 states his death as "被云长刀起处，挥为两段" — actor form and deed in ONE chunk, and the
// actor's own name recalls it. It was dropped by the act-word filter, for a phrasing the plan had
// not written down.
//
// The fix is not a kill-verb list in the code. Such a list is bound to one language and one corpus
// ("挥为两段" survives no change of source, and nothing fails loudly when it stops matching), and
// it would sit UPSTREAM of the node whose job is to judge the deed ("nothing here decides what a
// kill is, or which words mean one" — coverage_step.go). The words the source uses have to be read
// out of the passages this run already holds — and then CHECKED: a word no passage contains is the
// model's memory of the language, so it is dropped rather than trusted (the validation below).
//
// samples is the text of the passages the caller showed it; a reported word is kept only when one
// of them actually carries it.
func induceActWords(ctx context.Context, model SessionModel, declared []string, passages string, samples []string) []string {
	if model == nil || len(declared) == 0 || passages == "" || len(samples) == 0 || ctx.Err() != nil {
		return nil
	}
	head := strings.Replace(CoverageVocabPrompt, "%WORDS%", strings.Join(declared, " / "), 1)
	head = strings.Replace(head, "%MAX%", fmt.Sprint(coverageVocabWords), 1)
	reply, err := model.Complete(ctx, []schema.Message{
		*schema.SystemMessage(head),
		*schema.UserMessage(passages),
	}, nil)
	if err != nil || reply == nil {
		return nil
	}
	obj := coverageJSONObject(reply.Content)
	if obj == nil {
		return nil
	}
	var out []string
	for _, raw := range coverageAnyList(obj["words"]) {
		w, _ := raw.(string)
		w = strings.TrimSpace(w)
		// A phrase, not a sentence; and present in what the model was shown.
		if w == "" || len([]rune(w)) > 16 || strings.ContainsAny(w, " \t\n") {
			continue
		}
		if !coverageCarriedByAny(samples, w) {
			continue
		}
		out = appendUnique(out, w)
		if len(out) >= coverageVocabWords {
			break
		}
	}
	return out
}

// coverageCarriedByAny reports whether one of the passages actually carries the word — the check
// that makes the induced vocabulary a reading of the corpus instead of a claim about it.
func coverageCarriedByAny(samples []string, word string) bool {
	for _, s := range samples {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

// resolveCoverageBatch puts ONE batch of jobs in front of the model and returns the members
// it named out of that batch. The verdicts it reports are keyed by the CALLER's window
// indices, so the caller can re-ask exactly the jobs that came back with none.
func resolveCoverageBatch(ctx context.Context, model SessionModel, question string, cov Coverage, part []resolveJob) batchResult {
	res := batchResult{}
	if ctx.Err() != nil {
		return res
	}
	reply, err := model.Complete(ctx, []schema.Message{
		*schema.SystemMessage(CoverageResolvePrompt),
		*schema.UserMessage(coverageResolveQuestion(question, cov, part)),
	}, nil)
	if err != nil || reply == nil {
		res.failed = true
		return res
	}
	obj := coverageJSONObject(reply.Content)
	if obj == nil {
		res.failed = true
		return res
	}
	for _, raw := range coverageAnyList(obj["members"]) {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		i, ok := coverageInt(entry["i"])
		name := strings.TrimSpace(anyString(entry["name"]))
		if !ok || name == "" || i < 0 || i >= len(part) {
			continue
		}
		res.judged = append(res.judged, part[i].idx)
		res.members = append(res.members, ResolvedMember{
			Name:    name,
			ChunkID: part[i].w.ChunkID,
			Quote:   truncateRunes(part[i].w.Quote, coverageResolveQuoteChars),
		})
	}
	for _, raw := range coverageAnyList(obj["not_members"]) {
		if i, ok := coverageInt(raw); ok && i >= 0 && i < len(part) {
			res.judged = append(res.judged, part[i].idx)
		}
	}
	return res
}

// coverageResolveQuestion renders one call's user message: the question, the actor's
// declared forms, and this batch's windows on their own numbered lines. The numbering is
// BATCH-LOCAL and starts at 0 in every call — that is the whole interface the model has to
// get right, and the caller maps a line number back to its window.
func coverageResolveQuestion(question string, cov Coverage, part []resolveJob) string {
	var b strings.Builder
	b.WriteString("Question: " + question + "\n")
	if actors := cov.Actors(); len(actors) > 0 {
		b.WriteString("Actor (the forms this direction named): " + strings.Join(actors, " / ") + "\n")
	}
	if len(cov.Acts) > 0 {
		b.WriteString("Act words this direction declared: " + strings.Join(cov.Acts, " / ") + "\n")
	}
	b.WriteString("Evidence lines:\n")
	for i, j := range part {
		w := j.w
		quote := truncateRunes(w.Quote, coverageResolveQuoteChars)
		b.WriteString("- [" + strconv.Itoa(i) + "] chunk_id=" + w.ChunkID + " \"" + quote + "\"\n")
	}
	b.WriteString("\nAnswer with the JSON object and nothing else.")
	return b.String()
}

// coverageJSONObject reads the JSON object out of a reply, which models wrap in thinking
// preamble and Markdown fences.
func coverageJSONObject(content string) map[string]any {
	text := reFence.ReplaceAllString(content, "")
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(text[start:end+1]), &obj); err != nil {
		return nil
	}
	return obj
}

// coverageAnyList reads a JSON list out of a decoded object. `encoding/json` decodes every
// JSON array into []any (a []map[string]any case here was unreachable), so anything else is
// treated as "no list" rather than guessed at.
func coverageAnyList(v any) []any {
	if list, ok := v.([]any); ok {
		return list
	}
	return nil
}

// coverageInt reads a line number, which a model may hand back as a number or a string.
func coverageInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), t == float64(int(t))
	case int:
		return t, true
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}
