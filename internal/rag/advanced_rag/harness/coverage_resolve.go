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
}

// batchResult is one batch's outcome: the members it named, which of its lines were
// judged (as a member or explicitly not), and whether the call failed.
type batchResult struct {
	members []ResolvedMember
	answers map[int]bool
	failed  bool
}

// The resolve's bounds. A batch is small because ONE prompt carrying every candidate is
// the failure this node exists to remove: measured (2026-09-16, 三国/关羽) a single call
// carrying 28 candidates hit its 30s clock and answered nothing (`asked=28 answered=0`),
// which is a whole enumeration's worth of members lost to one timeout.
const (
	coverageResolveBatch      = 8
	coverageResolveWorkers    = 3
	coverageResolveQuoteChars = 300
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
//     deduped case-insensitively — one source spells one member several ways (Mona Lisa /
//     mona lisa, 关羽 / 关公), and a set counts entities, not spellings.
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

	batches := (len(windows) + coverageResolveBatch - 1) / coverageResolveBatch
	out := make([]batchResult, batches)

	sem := make(chan struct{}, coverageResolveWorkers)
	var wg sync.WaitGroup
	for b := 0; b < batches; b++ {
		lo := b * coverageResolveBatch
		hi := min(lo+coverageResolveBatch, len(windows))
		part := windows[lo:hi]
		wg.Add(1)
		go func(b int, part []CoverageWindow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[b] = resolveCoverageBatch(ctx, model, question, cov, part)
		}(b, part)
	}
	wg.Wait()

	seen := map[string]int{}
	var members []ResolvedMember
	for _, r := range out {
		stats.Batches++
		if r.failed {
			stats.Failed++
			continue
		}
		stats.Answered += len(r.answers)
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
	// Every window is UNKNOWN unless a verdict named it: the windows a failed batch
	// carried, the lines the model skipped, and the windows the cap kept out.
	stats.Unknown = dropped + (stats.Asked - stats.Answered)
	return members, stats
}

// resolveCoverageBatch puts ONE batch of windows in front of the model and returns the
// members it named out of that batch.
func resolveCoverageBatch(ctx context.Context, model SessionModel, question string, cov Coverage, part []CoverageWindow) batchResult {
	res := batchResult{answers: map[int]bool{}}
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
		res.answers[i] = true
		res.members = append(res.members, ResolvedMember{
			Name:    name,
			ChunkID: part[i].ChunkID,
			Quote:   truncateRunes(part[i].Quote, coverageResolveQuoteChars),
		})
	}
	for _, raw := range coverageAnyList(obj["not_members"]) {
		if i, ok := coverageInt(raw); ok && i >= 0 && i < len(part) {
			res.answers[i] = true
		}
	}
	return res
}

// coverageResolveQuestion renders one call's user message: the question, the actor's
// declared forms, and this batch's windows on their own numbered lines.
func coverageResolveQuestion(question string, cov Coverage, part []CoverageWindow) string {
	var b strings.Builder
	b.WriteString("Question: " + question + "\n")
	if actors := cov.Actors(); len(actors) > 0 {
		b.WriteString("Actor (the forms this direction named): " + strings.Join(actors, " / ") + "\n")
	}
	if len(cov.Acts) > 0 {
		b.WriteString("Act words this direction declared: " + strings.Join(cov.Acts, " / ") + "\n")
	}
	b.WriteString("Evidence lines:\n")
	for i, w := range part {
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
