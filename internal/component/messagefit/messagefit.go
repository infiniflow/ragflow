//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (License); you may not
//  use this file except in compliance with the License. You may obtain a
//  copy of the License at http://www.apache.org/licenses/LICENSE-2.0
//

// Package messagefit trims a message list so its total token count fits
// within a budget. It mirrors Python's rag/prompts/generator.py:message_fit_in.
//
// The package is shared by the agent canvas LLM component and the ingestion
// Extractor component. Callers convert their message type to []Message,
// call Fit, then write the results back.
package messagefit

import (
	"ragflow/internal/tokenizer"
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

// Fit trims msgs so its total token count fits within maxTokens.
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
//     - Single message → trim to maxTokens directly.
//
// maxTokens <= 0 is treated as 8192 (Python's default).
// Returns the token count after fitting. msgs is modified in place:
// entries that were dropped have their Content set to "".
func Fit(msgs []Message, maxTokens int) int {
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	if len(msgs) == 0 {
		return 0
	}

	// Step 1: everything fits (an exact budget match counts as fitting).
	if total := countTokens(msgs); total <= maxTokens {
		return total
	}

	// Step 2: keep all system + last non-system.
	kept := make([]int, 0, len(msgs))
	lastNonSystem := -1
	for i := range msgs {
		if msgs[i].Role == "system" {
			kept = append(kept, i)
		} else {
			lastNonSystem = i
		}
	}
	if lastNonSystem >= 0 {
		kept = append(kept, lastNonSystem)
	}
	if len(kept) == 0 {
		return 0
	}

	keptMsgs := make([]Message, len(kept))
	for i, idx := range kept {
		keptMsgs[i] = msgs[idx]
	}
	if total := countTokens(keptMsgs); total <= maxTokens {
		// Zero out the dropped entries so the caller can filter them.
		zeroDropped(msgs, kept)
		return total
	}

	// Step 3: trim proportionally.
	if len(kept) == 1 {
		msgs[kept[0]].Content = tokenizer.TrimContentToTokenLimit(msgs[kept[0]].Content, maxTokens)
		zeroDropped(msgs, kept)
		return countTokens(msgs[kept[0] : kept[0]+1])
	}

	// kept[:len(kept)-1] are the retained system messages; the last entry
	// is the final non-system message.
	sysIdxs := kept[:len(kept)-1]
	lastIdx := kept[len(kept)-1]
	ll := 0
	for _, idx := range sysIdxs {
		ll += tokenizer.NumTokensFromString(msgs[idx].Content)
	}
	ll2 := tokenizer.NumTokensFromString(msgs[lastIdx].Content)
	total := ll + ll2
	if total <= 0 {
		zeroDropped(msgs, kept)
		return 0
	}

	if float64(ll)/float64(total) > 0.8 {
		// System dominates: preserve the last message and give the
		// remaining budget to the system messages.
		preserved := min(ll2, maxTokens)
		msgs[lastIdx].Content = tokenizer.TrimContentToTokenLimit(msgs[lastIdx].Content, preserved)
		trimSystems(msgs, sysIdxs, max(0, maxTokens-preserved))
	} else {
		preserved := min(ll, maxTokens)
		trimSystems(msgs, sysIdxs, preserved)
		msgs[lastIdx].Content = tokenizer.TrimContentToTokenLimit(msgs[lastIdx].Content, max(0, maxTokens-preserved))
	}
	zeroDropped(msgs, kept)
	return countTokensMulti(msgs, kept)
}

// trimSystems trims each retained system message so their combined token
// count fits within budget. The budget is allocated in proportion to each
// message's original token count, with the last message taking any remainder
// so the total never exceeds budget.
func trimSystems(msgs []Message, sysIdxs []int, budget int) {
	if len(sysIdxs) == 0 {
		return
	}
	if budget <= 0 {
		for _, idx := range sysIdxs {
			msgs[idx].Content = ""
		}
		return
	}
	total := 0
	for _, idx := range sysIdxs {
		total += tokenizer.NumTokensFromString(msgs[idx].Content)
	}
	if total <= 0 {
		return
	}
	remaining := budget
	for i, idx := range sysIdxs {
		limit := remaining
		if i < len(sysIdxs)-1 {
			tokens := tokenizer.NumTokensFromString(msgs[idx].Content)
			limit = int(float64(budget) * float64(tokens) / float64(total))
			if limit > remaining {
				limit = remaining
			}
		}
		msgs[idx].Content = tokenizer.TrimContentToTokenLimit(msgs[idx].Content, limit)
		remaining -= tokenizer.NumTokensFromString(msgs[idx].Content)
	}
}

func countTokens(msgs []Message) int {
	total := 0
	for i := range msgs {
		total += tokenizer.NumTokensFromString(msgs[i].Content)
	}
	return total
}

func countTokensMulti(msgs []Message, indices []int) int {
	total := 0
	for _, i := range indices {
		total += tokenizer.NumTokensFromString(msgs[i].Content)
	}
	return total
}

// zeroDropped sets Content to "" for every index in msgs that is not in kept.
func zeroDropped(msgs []Message, kept []int) {
	keptSet := make(map[int]struct{}, len(kept))
	for _, i := range kept {
		keptSet[i] = struct{}{}
	}
	for i := range msgs {
		if _, ok := keptSet[i]; !ok {
			msgs[i].Content = ""
		}
	}
}
