//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (License); you may not
//  use this file except in compliance with the License. You may obtain a
//  copy of the License at http://www.apache.org/licenses/LICENSE-2.0
//

package tokenizer

import (
	"slices"
)

// Message is the minimal representation the fitter needs. Both the agent's
// schema.Message and ingestion's eschema.Message convert to this.
type Message struct {
	// Role is "system", "user", "assistant", etc. Only "system" receives
	// special treatment during fitting.
	Role string
	// Content is the text content that may be truncated.
	Content string
}

// Fit trims msgs so the kept messages fit within budget. budget is the
// caller-chosen token ceiling for the whole conversation — the agent LLM and
// ingestion Extractor both pass the chat model's context window
// (content_length), not the generation cap (max_output).
//
// It returns the kept messages in original order (trimmed when necessary),
// their original indices into msgs, and the kept messages' total token count.
// msgs itself is never modified, and dropped entries are simply absent from
// kept/keptIdx — no empty-content sentinel is used. A message that is kept but
// trimmed to empty (e.g. the system share collapses to 0 when the last message
// alone fills the budget) is still reported as kept, mirroring Python.
//
// Strategy (mirrors Python's message_fit_in, with two deliberate tweaks:
// an exact budget match counts as fitting, and the system share is spread
// across every retained system message instead of only the first):
//  1. If everything fits, return as-is.
//  2. Keep all system messages + the last non-system message, drop the
//     rest; if that fits, return.
//  3. If still over, trim proportionally:
//     - System dominates (>80% of tokens) → preserve the last message,
//     give the remaining budget to the system messages.
//     - Otherwise → preserve the system messages, give the remaining
//     budget to the last.
//     - Single message → trim to budget directly.
//
// budget <= 0 is treated as 8192 (Python's default).
func Fit(msgs []Message, budget int) (kept []Message, keptIdx []int, count int) {
	if budget <= 0 {
		budget = 8192
	}
	if len(msgs) == 0 {
		return nil, nil, 0
	}

	// Step 1: everything fits (an exact budget match counts as fitting).
	if total := countTokens(msgs); total <= budget {
		kept = slices.Clone(msgs)
		keptIdx = make([]int, len(msgs))
		for i := range keptIdx {
			keptIdx[i] = i
		}
		return kept, keptIdx, total
	}

	// Step 2: keep all system + last non-system.
	kept = make([]Message, 0, len(msgs))
	keptIdx = make([]int, 0, len(msgs))
	lastNonSystem := -1
	for i := range msgs {
		if msgs[i].Role == "system" {
			kept = append(kept, msgs[i])
			keptIdx = append(keptIdx, i)
		} else {
			lastNonSystem = i
		}
	}
	if lastNonSystem >= 0 {
		kept = append(kept, msgs[lastNonSystem])
		keptIdx = append(keptIdx, lastNonSystem)
	}
	if len(kept) == 0 {
		return nil, nil, 0
	}
	if total := countTokens(kept); total <= budget {
		return kept, keptIdx, total
	}

	// Step 3: trim proportionally.
	if len(kept) == 1 {
		kept[0].Content = TrimContentToTokenLimit(kept[0].Content, budget)
		return kept, keptIdx, countTokens(kept)
	}

	// Only system messages were retained (no non-system message): spread the
	// whole budget across every retained system message.
	if lastNonSystem < 0 {
		trimSystems(kept, budget)
		return kept, keptIdx, countTokens(kept)
	}

	// kept[:len(kept)-1] are the retained system messages; the last entry
	// is the final non-system message.
	sys := kept[:len(kept)-1]
	last := &kept[len(kept)-1]
	ll := 0
	for i := range sys {
		ll += NumTokensFromString(sys[i].Content)
	}
	ll2 := NumTokensFromString(last.Content)
	total := ll + ll2
	if total <= 0 {
		return kept, keptIdx, 0
	}

	if float64(ll)/float64(total) > 0.8 {
		// System dominates: preserve the last message and give the
		// remaining budget to the system messages.
		preserved := min(ll2, budget)
		last.Content = TrimContentToTokenLimit(last.Content, preserved)
		trimSystems(sys, max(0, budget-preserved))
	} else {
		preserved := min(ll, budget)
		trimSystems(sys, preserved)
		last.Content = TrimContentToTokenLimit(last.Content, max(0, budget-preserved))
	}
	return kept, keptIdx, countTokens(kept)
}

// trimSystems trims each system message so their combined token count fits
// within budget. The budget is allocated in proportion to each message's
// original token count, with the last message taking any remainder so the
// total never exceeds budget.
func trimSystems(sys []Message, budget int) {
	if len(sys) == 0 {
		return
	}
	if budget <= 0 {
		for i := range sys {
			sys[i].Content = ""
		}
		return
	}
	total := 0
	for i := range sys {
		total += NumTokensFromString(sys[i].Content)
	}
	if total <= 0 {
		return
	}
	remaining := budget
	for i := range sys {
		limit := remaining
		if i < len(sys)-1 {
			tokens := NumTokensFromString(sys[i].Content)
			limit = int(float64(budget) * float64(tokens) / float64(total))
			if limit > remaining {
				limit = remaining
			}
		}
		sys[i].Content = TrimContentToTokenLimit(sys[i].Content, limit)
		remaining -= NumTokensFromString(sys[i].Content)
	}
}

func countTokens(msgs []Message) int {
	total := 0
	for i := range msgs {
		total += NumTokensFromString(msgs[i].Content)
	}
	return total
}
